package orchestrator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
)

// Config holds all dependencies for the orchestrator.
type Config struct {
	FSM        *fsm.Machine
	Store      task.Store
	Runner     *agent.Runner
	Dispatcher *dispatch.Dispatcher
	Policy     *policy.Engine
	Audit      *audit.Pipeline
	Executor   executor.Executor
	Notary     notary.Notary
}

// Orchestrator drives tasks through the FSM.
type Orchestrator struct {
	fsm        *fsm.Machine
	store      task.Store
	runner     *agent.Runner
	dispatcher *dispatch.Dispatcher
	policy     *policy.Engine
	audit      *audit.Pipeline
	executor   executor.Executor
	notary     notary.Notary
}

// NewOrchestrator creates an orchestrator with all dependencies.
func NewOrchestrator(cfg Config) *Orchestrator {
	return &Orchestrator{
		fsm:        cfg.FSM,
		store:      cfg.Store,
		runner:     cfg.Runner,
		dispatcher: cfg.Dispatcher,
		policy:     cfg.Policy,
		audit:      cfg.Audit,
		executor:   cfg.Executor,
		notary:     cfg.Notary,
	}
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

	if err := o.fsm.Transition(ctx, tk, next, reason); err != nil {
		return fmt.Errorf("transition failed: %w", err)
	}

	slog.Info("state transition", "task", tk.ID, "to", next, "reason", reason)

	if err := o.store.Update(ctx, tk); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
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
		_, err := o.runner.Execute(ctx, tk, agent.RoleStrategist, tk.Description)
		if err != nil {
			return task.StateHumanReview, "strategist error", nil
		}
		return task.StateResearching, "requirements ready", nil

	case task.StateResearching:
		_, err := o.runner.Execute(ctx, tk, agent.RoleResearcher, "gather context")
		if err != nil {
			return "", "", fmt.Errorf("researcher failed: %w", err)
		}
		return task.StatePacketValidation, "context packet produced", nil

	case task.StatePacketValidation:
		// In phase 1, validation is a pass-through. Real validation comes with Python tooling.
		return task.StateCoding, "packet validated", nil

	case task.StateCoding:
		_, err := o.runner.Execute(ctx, tk, agent.RoleCoder, "generate code")
		if err != nil {
			return "", "", fmt.Errorf("coder failed: %w", err)
		}
		return task.StateAuditing, "code ready for audit", nil

	case task.StateAuditing:
		result, err := o.audit.Run(ctx, audit.AuditInput{Code: "placeholder"})
		if err != nil {
			return task.StateHumanReview, "audit error", nil
		}
		switch result.Verdict {
		case audit.VerdictApprove:
			return task.StateExecuting, "audit passed", nil
		case audit.VerdictReject:
			return task.StateCoding, "audit rejected, revision needed", nil
		default:
			return task.StateHumanReview, "audit escalated", nil
		}

	case task.StateExecuting:
		result, err := o.executor.Run(ctx, executor.RunRequest{Code: "placeholder"})
		if err != nil {
			return task.StateFailed, "executor error", nil
		}
		if !result.Success {
			return task.StateCoding, "execution failed, retry", nil
		}
		return task.StateOutputReview, "execution complete", nil

	case task.StateOutputReview:
		result, err := o.notary.Review(ctx, notary.ReviewRequest{ArtifactPath: "placeholder"})
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
