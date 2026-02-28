package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/config"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/orchestrator"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/providers"
	"github.com/bnaylor/perry/internal/task"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// Get task description from args
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: perry <task description>\n")
		os.Exit(1)
	}
	taskDesc := os.Args[1]

	// Load config
	routingCfg, err := config.LoadRoutingConfig("configs/routing.yaml")
	if err != nil {
		slog.Error("failed to load routing config", "error", err)
		os.Exit(1)
	}
	policyCfg, err := config.LoadPolicyConfig("configs/policy.yaml")
	if err != nil {
		slog.Error("failed to load policy config", "error", err)
		os.Exit(1)
	}

	// Build provider map from environment
	providerMap := providers.BuildProviderMap(providers.ProviderConfig{
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		GeminiKey:    os.Getenv("GEMINI_API_KEY"),
		OllamaURL:    os.Getenv("OLLAMA_URL"),
	})

	// Register real agents
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewStrategist(),
		agent.RoleResearcher: agent.NewResearcher(),
		agent.RoleCoder:      agent.NewCoder(),
		agent.RoleAuditor:    agent.NewAuditor(),
	}

	// Build orchestrator
	fsmMachine := fsm.New()
	fsmMachine.OnTransition(func(tk *task.Task, from, to task.State) {
		fmt.Printf("  %s → %s\n", from, to)
	})

	orch := orchestrator.NewOrchestrator(orchestrator.Config{
		FSM:        fsmMachine,
		Store:      task.NewMemStore(),
		Runner:     agent.NewRunner(agents, providerMap),
		Dispatcher: dispatch.New(routingCfg),
		Policy:     policy.NewEngine(policyCfg),
		Audit:      audit.NewPipeline(), // deterministic gates wired in Phase 3
		Executor:   executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:     notary.NewMockNotary(true),
	})

	// Submit and run
	fmt.Println("perry: secure agentic platform")
	fmt.Println("================================")
	fmt.Printf("Task: %s\n\n", taskDesc)

	tk, err := orch.Submit(ctx, taskDesc, "cli-user")
	if err != nil {
		slog.Error("failed to submit task", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Task %s submitted\n\n", tk.ID)
	fmt.Println("Running through state machine:")

	for tk.State != task.StateCompleted && tk.State != task.StateFailed && tk.State != task.StateHumanReview {
		if err := orch.Step(ctx, tk); err != nil {
			slog.Error("step failed", "error", err, "state", tk.State)
			os.Exit(1)
		}
	}

	fmt.Printf("\nFinal state: %s (%d transitions)\n", tk.State, len(tk.History))
}
