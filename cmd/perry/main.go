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
	listModels := flag.Bool("models", false, "list available models from all providers and exit")
	showBilling := flag.Bool("bill", false, "show current billing cycle spend and exit")
	flag.Parse()

	// Open storage (needed for billing check too)
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

	// Load policy (needed for budget thresholds)
	policyCfg, err := config.LoadPolicyConfig("configs/policy.yaml")
	if err != nil {
		slog.Error("failed to load policy config", "error", err)
		os.Exit(1)
	}
	policyEngine := policy.NewEngine(policyCfg)

	if *showBilling {
		billing := audit.NewBillingAuditor(store)
		daily, monthly, err := billing.CheckBudgetStatus(ctx)
		if err != nil {
			slog.Error("failed to fetch billing status", "error", err)
			os.Exit(1)
		}
		fmt.Println("Perry: Billing & Cognitive Logistics")
		fmt.Println("====================================")
		fmt.Printf("Daily Spend:   $%7.4f / $%7.4f (%.1f%%)\n", daily, policyEngine.MaxDailyBudget(), (daily/policyEngine.MaxDailyBudget())*100)
		fmt.Printf("Monthly Spend: $%7.4f / $%7.4f (%.1f%%)\n", monthly, policyEngine.MaxMonthlyBudget(), (monthly/policyEngine.MaxMonthlyBudget())*100)
		
		health, _ := billing.AssessHealth(ctx, policyEngine.MaxDailyBudget(), policyEngine.MaxMonthlyBudget())
		status := "HEALTHY"
		if !health.IsHealthy {
			status = "WARNING: AGGRESSIVE LOCAL ROUTING ACTIVE"
		}
		fmt.Printf("System Status: %s\n", status)
		os.Exit(0)
	}

	// Load routing
	routingCfg, err := config.LoadRoutingConfig("configs/routing.yaml")
	if err != nil {
		slog.Error("failed to load routing config", "error", err)
		os.Exit(1)
	}

	// Build provider map from routing config
	providerMap := providers.BuildProviderMap(routingCfg)

	if *listModels {
		fmt.Println("Available Models by Provider:")
		fmt.Println("==============================")
		for name, p := range providerMap {
			fmt.Printf("\nProvider: %s\n", name)
			models, err := p.ListModels(ctx)
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			for _, m := range models {
				fmt.Printf("  - %-30s (%v)\n", m.Name, m.Capabilities)
			}
		}
		os.Exit(0)
	}

	// Get task description from remaining args
	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Usage: perry [--db-path path] [--models] [--bill] <task description>\n")
		os.Exit(1)
	}
	taskDesc := flag.Arg(0)

	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist:    agent.NewStrategist(),
		agent.RoleResearcher:    agent.NewResearcher(),
		agent.RoleCoder:         agent.NewCoder(),
		agent.RoleAuditor:       agent.NewAuditor(),
		agent.RoleShadowAuditor: agent.NewShadowAuditor(),
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

	// Build Dispatcher with Filters
	billingAuditor := audit.NewBillingAuditor(store)
	dispatcher := dispatch.New(routingCfg,
		&dispatch.CostFilter{MaxTaskCost: policyEngine.MaxCost(), Fallback: routingCfg.Fallback},
		&dispatch.BudgetHealthFilter{Billing: billingAuditor, Policy: policyEngine, Fallback: routingCfg.Fallback},
	)

	orch := orchestrator.NewOrchestrator(orchestrator.Config{
		FSM:         fsmMachine,
		Store:       store,
		Runner:      agent.NewRunner(agents, providerMap),
		Dispatcher:  dispatcher,
		Policy:      policyEngine,
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
