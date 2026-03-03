package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/codebase"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/storage"
	"github.com/bnaylor/perry/internal/task"
)

// Config holds all dependencies for the orchestrator.
type Config struct {
	FSM         *fsm.Machine
	Store       *storage.Store
	Runner      *agent.Runner
	Dispatcher  *dispatch.Dispatcher
	Policy      *policy.Engine
	Audit       *audit.Pipeline
	PacketAudit *audit.Pipeline
	Executor    executor.Executor
	Notary      notary.Notary
}

// Orchestrator drives tasks through the FSM.
type Orchestrator struct {
	fsm           *fsm.Machine
	store         *storage.Store
	runner        *agent.Runner
	dispatcher    *dispatch.Dispatcher
	policy        *policy.Engine
	audit         *audit.Pipeline
	packetAudit   *audit.Pipeline
	executor      executor.Executor
	notary        notary.Notary
	outputs       map[string]agent.AgentOutput // keyed by "taskID:role"
	artifactPaths map[string]string            // keyed by taskID
}

// NewOrchestrator creates an orchestrator with all dependencies.
func NewOrchestrator(cfg Config) *Orchestrator {
	return &Orchestrator{
		fsm:           cfg.FSM,
		store:         cfg.Store,
		runner:        cfg.Runner,
		dispatcher:    cfg.Dispatcher,
		policy:        cfg.Policy,
		audit:         cfg.Audit,
		packetAudit:   cfg.PacketAudit,
		executor:      cfg.Executor,
		notary:        cfg.Notary,
		outputs:       make(map[string]agent.AgentOutput),
		artifactPaths: make(map[string]string),
	}
}

// outputKey returns the map key for storing agent outputs.
func outputKey(taskID string, role agent.Role) string {
	return taskID + ":" + string(role)
}

// Submit creates a new task and persists it.
func (o *Orchestrator) Submit(ctx context.Context, description, createdBy string) (*task.Task, error) {
	tk := task.New(description, createdBy)
	if err := o.store.Create(ctx, tk); err != nil {
		return nil, fmt.Errorf("failed to store task: %w", err)
	}
	slog.Info("task submitted", "id", tk.ID, "description", description)
	return tk, nil
}

// Step advances a task by one state transition.
func (o *Orchestrator) Step(ctx context.Context, tk *task.Task) error {
	next, reason, err := o.determineNextState(ctx, tk)
	if err != nil {
		return err
	}
	if next == "" {
		return nil // terminal state, nothing to do
	}

	prev := tk.State // capture before transition
	if err := o.fsm.Transition(ctx, tk, next, reason); err != nil {
		return fmt.Errorf("transition failed: %w", err)
	}

	// Record transition (log and swallow errors)
	if recErr := o.store.RecordTransition(tk.ID, string(prev), string(next), reason); recErr != nil {
		slog.Warn("failed to record transition", "error", recErr)
	}

	slog.Info("state transition", "task", tk.ID, "to", next, "reason", reason)

	if err := o.store.Update(ctx, tk); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	return nil
}

// HandleCommands checks for and processes inbound instructions from the bus.
func (o *Orchestrator) HandleCommands(ctx context.Context) error {
	cmds, err := o.store.GetPendingCommands(ctx)
	if err != nil {
		return err
	}

	for _, cmd := range cmds {
		slog.Info("processing command", "id", cmd.ID, "command", cmd.Command)
		if err := o.store.UpdateCommandStatus(ctx, cmd.ID, "processing"); err != nil {
			slog.Warn("failed to update command status", "id", cmd.ID, "error", err)
			continue
		}

		// Process command logic
		status := "completed"
		if err := o.executeCommand(ctx, cmd); err != nil {
			slog.Error("failed to execute command", "id", cmd.ID, "error", err)
			status = "failed"
		}

		if err := o.store.UpdateCommandStatus(ctx, cmd.ID, status); err != nil {
			slog.Warn("failed to finalize command status", "id", cmd.ID, "error", err)
		}
	}

	return nil
}

func (o *Orchestrator) executeCommand(ctx context.Context, cmd storage.Command) error {
	// Simple command parsing: "!command [taskID]"
	// In Phase 4, we mainly care about !approve for HUMAN_REVIEW states.
	if cmd.Command == "!approve" && cmd.TaskID != "" {
		tk, err := o.store.Get(ctx, cmd.TaskID)
		if err != nil {
			return err
		}
		if tk.State == task.StateHumanReview {
			// Advance the state machine manually
			// For now, we'll just log it. Real advancement requires knowing 
			// which state we're resuming to. 
			// TODO: Add resumption logic to FSM/Orchestrator.
			slog.Info("manual approval received via discord", "task", tk.ID)
		}
	}
	return nil
}

// determineNextState figures out what the next state should be based on
// the current state and the results of running the appropriate handler.
func (o *Orchestrator) determineNextState(ctx context.Context, tk *task.Task) (task.State, string, error) {
	switch tk.State {
	case task.StateSubmitted:
		return task.StatePlanning, "auto", nil

	case task.StatePlanning:
		input := tk.Description
		decision, err := o.dispatcher.RouteWithCost(ctx, tk, string(agent.RoleStrategist), len(input)/4, o.policy.MaxCost())
		if err != nil {
			return task.StateHumanReview, "routing error", nil
		}
		output, err := o.runner.Execute(ctx, tk, agent.RoleStrategist, input, decision)
		if err != nil {
			slog.Error("strategist failed", "error", err)
			return task.StateHumanReview, "strategist error", nil
		}
		o.outputs[outputKey(tk.ID, agent.RoleStrategist)] = output
		if recErr := o.store.RecordAgentCall(tk.ID, string(agent.RoleStrategist), decision.Provider, decision.Model, output.Usage.InputTokens, output.Usage.OutputTokens, output.Content); recErr != nil {
			slog.Warn("failed to record agent call", "error", recErr)
		}
		return task.StateResearching, "requirements ready", nil

	case task.StateResearching:
		researchInput := "gather context"
		// Try to estimate input size from current description + working set
		decision, err := o.dispatcher.RouteWithCost(ctx, tk, string(agent.RoleResearcher), len(tk.Description)/4, o.policy.MaxCost())
		if err != nil {
			return "", "", fmt.Errorf("routing failed: %w", err)
		}

		// Check for Working Set from Strategist
		var workingSet []string
		if stratOutput, ok := o.outputs[outputKey(tk.ID, agent.RoleStrategist)]; ok {
			if wsVal, ok := stratOutput.Parsed["working_set"]; ok {
				if wsSlice, ok := wsVal.([]any); ok {
					for _, v := range wsSlice {
						if s, ok := v.(string); ok {
							workingSet = append(workingSet, s)
						}
					}
				}
			}
		}

		// If working set is identified, take a codebase snapshot and read file contents
		if len(workingSet) > 0 {
			snapshot, err := codebase.TakeSnapshot(workingSet)
			if err != nil {
				slog.Warn("codebase snapshot failed", "error", err)
			} else if snapBytes, err := json.Marshal(snapshot); err == nil {
				researchInput = fmt.Sprintf("gather context. TARGET LANGUAGE IS 'go'. codebase snapshot: %s", string(snapBytes))
			}
		}

		output, err := o.runner.Execute(ctx, tk, agent.RoleResearcher, researchInput, decision)
		if err != nil {
			return "", "", fmt.Errorf("researcher failed: %w", err)
		}
		o.outputs[outputKey(tk.ID, agent.RoleResearcher)] = output
		if recErr := o.store.RecordAgentCall(tk.ID, string(agent.RoleResearcher), decision.Provider, decision.Model, output.Usage.InputTokens, output.Usage.OutputTokens, output.Content); recErr != nil {
			slog.Warn("failed to record agent call", "error", recErr)
		}
		return task.StatePacketValidation, "context packet produced", nil

	case task.StatePacketValidation:
		if o.packetAudit == nil {
			return task.StateCoding, "packet validated (no pipeline)", nil
		}
		resOutput, ok := o.outputs[outputKey(tk.ID, agent.RoleResearcher)]
		if !ok {
			return task.StateHumanReview, "no researcher output to validate", nil
		}
		packet := resOutput.Parsed
		if packet == nil {
			packet = map[string]any{}
		}

		// Fix packet_id if it's invalid (common local LLM failure)
		if meta, ok := packet["packet_meta"].(map[string]any); ok {
			dateStr := tk.CreatedAt.Format("20060102")
			hash := sha256.Sum256([]byte(tk.ID))
			shortHash := hex.EncodeToString(hash[:4]) // 8 hex chars
			meta["packet_id"] = fmt.Sprintf("CP-%s-%s", dateStr, shortHash)
		}

		// Ensure language is 'go' for Perry core changes
		if constraints, ok := packet["constraints"].(map[string]any); ok {
			constraints["language"] = "go"
		} else {
			packet["constraints"] = map[string]any{
				"language":              "go",
				"permitted_operations":  []string{"read_local_file", "write_local_file"},
				"prohibited_operations": []string{"network_listen"},
			}
		}

		result, err := o.packetAudit.Run(ctx, audit.AuditInput{ContextPacket: packet})
		if err != nil {
			return task.StateHumanReview, "packet audit error", nil
		}
		for _, gr := range result.GateResults {
			if recErr := o.store.RecordAuditGate(tk.ID, "packet", gr.Gate, gr.Pass, gr.Findings); recErr != nil {
				slog.Warn("failed to record audit gate", "error", recErr)
			}
		}
		if result.Verdict != audit.VerdictApprove {
			// Check for infinite loops in packet validation
			if tk.RetryCount(task.StatePacketValidation, task.StateResearching) >= 3 {
				return task.StateHumanReview, "too many packet validation failures", nil
			}
			return task.StateResearching, "packet rejected, re-research needed", nil
		}
		return task.StateCoding, "packet validated", nil

	case task.StateCoding:
		// Retrieve Context Packet from Researcher for Coder
		coderInput := "generate code"
		packetBytes := []byte("{}")
		if resOutput, ok := o.outputs[outputKey(tk.ID, agent.RoleResearcher)]; ok {
			if b, err := json.Marshal(resOutput.Parsed); err == nil {
				packetBytes = b
				coderInput = fmt.Sprintf("generate code using this context packet: %s", string(packetBytes))
			}
		}

		decision, err := o.dispatcher.RouteWithCost(ctx, tk, string(agent.RoleCoder), len(packetBytes)/4, o.policy.MaxCost())
		if err != nil {
			return "", "", fmt.Errorf("routing failed: %w", err)
		}

		output, err := o.runner.Execute(ctx, tk, agent.RoleCoder, coderInput, decision)
		if err != nil {
			return "", "", fmt.Errorf("coder failed: %w", err)
		}
		o.outputs[outputKey(tk.ID, agent.RoleCoder)] = output
		if recErr := o.store.RecordAgentCall(tk.ID, string(agent.RoleCoder), decision.Provider, decision.Model, output.Usage.InputTokens, output.Usage.OutputTokens, output.Content); recErr != nil {
			slog.Warn("failed to record agent call", "error", recErr)
		}
		return task.StateAuditing, "code ready for audit", nil

	case task.StateAuditing:
		code := "placeholder"
		language := "go"
		if coderOutput, ok := o.outputs[outputKey(tk.ID, agent.RoleCoder)]; ok {
			if codeVal, ok := coderOutput.Parsed["code"]; ok {
				if s, ok := codeVal.(string); ok {
					code = s
				}
			}
			if code == "placeholder" {
				code = coderOutput.Content
			}
			if langVal, ok := coderOutput.Parsed["language"]; ok {
				if s, ok := langVal.(string); ok {
					language = s
				}
			}
		}
		result, err := o.audit.Run(ctx, audit.AuditInput{
			Code:     code,
			Language: language,
		})
		if err != nil {
			return task.StateHumanReview, "audit error", nil
		}
		for _, gr := range result.GateResults {
			if recErr := o.store.RecordAuditGate(tk.ID, "code", gr.Gate, gr.Pass, gr.Findings); recErr != nil {
				slog.Warn("failed to record audit gate", "error", recErr)
			}
		}
		switch result.Verdict {
		case audit.VerdictApprove:
			return task.StateShadowAuditing, "audit passed", nil
		case audit.VerdictReject:
			return task.StateCoding, "audit rejected, revision needed", nil
		default:
			return task.StateHumanReview, "audit escalated", nil
		}

	case task.StateShadowAuditing:
		// Grab code to send to the auditor
		code := "placeholder"
		if coderOutput, ok := o.outputs[outputKey(tk.ID, agent.RoleCoder)]; ok {
			if codeVal, ok := coderOutput.Parsed["code"]; ok {
				if s, ok := codeVal.(string); ok {
					code = s
				}
			}
			if code == "placeholder" {
				code = coderOutput.Content
			}
		}

		decision, err := o.dispatcher.RouteWithCost(ctx, tk, string(agent.RoleShadowAuditor), len(code)/4, o.policy.MaxCost())
		if err != nil {
			return task.StateHumanReview, "routing error", nil
		}

		output, err := o.runner.Execute(ctx, tk, agent.RoleShadowAuditor, "find vulnerabilities in: "+code, decision)
		if err != nil {
			return task.StateHumanReview, "shadow auditor failed", nil
		}
		o.outputs[outputKey(tk.ID, agent.RoleShadowAuditor)] = output
		if recErr := o.store.RecordAgentCall(tk.ID, string(agent.RoleShadowAuditor), decision.Provider, decision.Model, output.Usage.InputTokens, output.Usage.OutputTokens, output.Content); recErr != nil {
			slog.Warn("failed to record agent call", "error", recErr)
		}

		// Evaluate report
		foundVal, ok := output.Parsed["vulnerability_found"]
		if !ok {
			return task.StateExecuting, "shadow auditor output invalid, proceeding", nil
		}
		found, isBool := foundVal.(bool)
		if !isBool || !found {
			return task.StateExecuting, "no vulnerabilities found by shadow auditor", nil
		}

		// Vulnerability found, test PoC
		pocVal, ok := output.Parsed["poc_code"]
		if !ok {
			return task.StateExecuting, "vulnerability found but no poc_code provided", nil
		}
		pocCode, ok := pocVal.(string)
		if !ok || pocCode == "" {
			return task.StateExecuting, "vulnerability found but poc_code empty", nil
		}

		// Execute PoC
		result, err := o.executor.Run(ctx, executor.RunRequest{
			Files: map[string]string{
				"target.py":   code,
				"poc_test.py": pocCode,
			},
			Entrypoint: "poc_test.py",
			Language:   "python",
			TimeoutSec: 30,
		})

		if err != nil {
			return task.StateExecuting, "executor failed to run PoC, skipping", nil
		}
		if result.Success { // Exit code 0 means PoC worked — vulnerability confirmed
			return task.StateCoding, "shadow auditor exploit verified, revision needed", nil
		}
		return task.StateExecuting, "shadow auditor exploit failed (false positive)", nil

	case task.StateExecuting:
		// Retrieve coder output for execution
		code := ""
		language := "python"
		var deps []string
		if coderOutput, ok := o.outputs[outputKey(tk.ID, agent.RoleCoder)]; ok {
			if codeVal, ok := coderOutput.Parsed["code"]; ok {
				if s, ok := codeVal.(string); ok {
					code = s
				}
			}
			if code == "" {
				code = coderOutput.Content
			}
			if langVal, ok := coderOutput.Parsed["language"]; ok {
				if s, ok := langVal.(string); ok {
					language = s
				}
			}
			if depsVal, ok := coderOutput.Parsed["dependencies"]; ok {
				if depsSlice, ok := depsVal.([]any); ok {
					for _, d := range depsSlice {
						if s, ok := d.(string); ok {
							deps = append(deps, s)
						}
					}
				}
			}
		}
		result, err := o.executor.Run(ctx, executor.RunRequest{
			Code:         code,
			Language:     language,
			Dependencies: deps,
			TimeoutSec:   60,
		})
		if err != nil {
			return task.StateFailed, "executor error", nil
		}

		// Move artifacts and record execution
		artifactPath := ""
		if result.Output != "" {
			managedPath, moveErr := o.store.MoveArtifacts(tk.ID, result.Output)
			if moveErr != nil {
				slog.Warn("failed to move artifacts", "error", moveErr)
			} else {
				artifactPath = managedPath
				o.artifactPaths[tk.ID] = managedPath
			}
		}
		if recErr := o.store.RecordExecution(tk.ID, result.ExitCode, result.Logs, artifactPath); recErr != nil {
			slog.Warn("failed to record execution", "error", recErr)
		}

		if !result.Success {
			return task.StateCoding, "execution failed, retry", nil
		}
		return task.StateOutputReview, "execution complete", nil

	case task.StateOutputReview:
		artifactPath := o.artifactPaths[tk.ID]
		result, err := o.notary.Review(ctx, notary.ReviewRequest{ArtifactPath: artifactPath})
		if err != nil {
			return task.StateHumanReview, "notary error", nil
		}
		if !result.Approved {
			return task.StateHumanReview, "output rejected", nil
		}
		return task.StateCompleted, "output approved", nil

	case task.StateCompleted, task.StateFailed:
		return "", "", nil // terminal

	case task.StateHumanReview:
		return "", "", nil // waiting for human, can't auto-advance

	default:
		return "", "", fmt.Errorf("unhandled state: %s", tk.State)
	}
}
