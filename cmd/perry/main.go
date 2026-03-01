package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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
	"github.com/bnaylor/perry/internal/storage"
	"github.com/bnaylor/perry/internal/task"
	dkclient "github.com/docker/docker/client"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// Parse flags
	dbPath := flag.String("db-path", ".perry/perry.db", "path to SQLite database")
	flag.Parse()

	// Get task description from remaining args
	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: perry [--db-path path] <task description>\n")
		os.Exit(1)
	}
	taskDesc := flag.Arg(0)

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

	// Build provider map from routing config
	providerMap := providers.BuildProviderMap(routingCfg)

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

	// Build audit pipelines
	scriptDir := "python"
	codePipeline := audit.NewPipeline(
		audit.NewASTGate(scriptDir, policyCfg.AllowedDependencies),
		audit.NewSecretsGate(scriptDir),
	)
	packetPipeline := audit.NewPipeline(
		audit.NewSchemaValidationGate(scriptDir),
		audit.NewContentScanGate(scriptDir),
	)

	// Build executor — Docker if available, mock fallback
	var exec executor.Executor
	dockerClient, err := dkclient.NewClientWithOpts(dkclient.FromEnv, dkclient.WithAPIVersionNegotiation())
	if err != nil {
		slog.Warn("Docker not available, using mock executor", "error", err)
		exec = executor.NewMockExecutor(executor.Result{Success: true, Output: "mock-result", ExitCode: 0})
	} else {
		exec = executor.NewDockerExecutor(dockerClient, "python:3.12-slim", executor.DefaultSandboxLimits())
	}

	// Open storage
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		slog.Error("failed to create database directory", "error", err)
		os.Exit(1)
	}
	store, err := storage.NewStore(*dbPath)
	if err != nil {
		slog.Error("failed to open storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	orch := orchestrator.NewOrchestrator(orchestrator.Config{
		FSM:         fsmMachine,
		Store:       store,
		Runner:      agent.NewRunner(agents, providerMap),
		Dispatcher:  dispatch.New(routingCfg),
		Policy:      policy.NewEngine(policyCfg),
		Audit:       codePipeline,
		PacketAudit: packetPipeline,
		Executor:    exec,
		Notary:      notary.NewMockNotary(true),
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
