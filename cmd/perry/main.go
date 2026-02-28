// cmd/perry/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/orchestrator"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// Build all components with mocks
	provider := llm.NewMockProvider("mock-model", "mock LLM output")
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewMockAgent(agent.RoleStrategist),
		agent.RoleResearcher: agent.NewMockAgent(agent.RoleResearcher),
		agent.RoleCoder:      agent.NewMockAgent(agent.RoleCoder),
		agent.RoleAuditor:    agent.NewMockAgent(agent.RoleAuditor),
	}

	fsmMachine := fsm.New()
	fsmMachine.OnTransition(func(tk *task.Task, from, to task.State) {
		fmt.Printf("  %s → %s\n", from, to)
	})

	orch := orchestrator.NewOrchestrator(orchestrator.Config{
		FSM:    fsmMachine,
		Store:  task.NewMemStore(),
		Runner: agent.NewRunner(agents, provider),
		Dispatcher: dispatch.New(dispatch.Config{
			Defaults: map[string]dispatch.RouteConfig{
				"strategist":       {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
				"researcher":       {Tier: "cloud", Provider: "google", Model: "gemini-2.5-pro"},
				"coder":            {Tier: "local", Provider: "ollama", Model: "qwen2.5-coder:32b"},
				"auditor_semantic": {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
			},
			Escalation: dispatch.EscalationConfig{
				MaxLocalAttempts: 3,
				PromoteTo:        dispatch.RouteConfig{Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
			},
		}),
		Policy: policy.NewEngine(policy.Config{
			MaxTokensPerTask: 500000,
			MaxCostPerTask:   10.0,
		}),
		Audit:    audit.NewPipeline(), // no gates yet — auto-pass
		Executor: executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:   notary.NewMockNotary(true),
	})

	// Submit and run a task
	fmt.Println("perry: secure agentic platform")
	fmt.Println("================================")
	fmt.Println()

	tk, err := orch.Submit(ctx, "Build a Python script that fetches weather data and saves it as JSON", "user-1")
	if err != nil {
		slog.Error("failed to submit task", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Task %s submitted\n\n", tk.ID)
	fmt.Println("Running through state machine:")

	for tk.State != task.StateCompleted && tk.State != task.StateFailed {
		if err := orch.Step(ctx, tk); err != nil {
			slog.Error("step failed", "error", err, "state", tk.State)
			os.Exit(1)
		}
	}

	fmt.Printf("\nFinal state: %s (%d transitions)\n", tk.State, len(tk.History))
}
