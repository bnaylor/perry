// internal/orchestrator/orchestrator_test.go
package orchestrator

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func allPassingPipeline() *audit.Pipeline {
	return audit.NewPipeline(&passingGate{})
}

type passingGate struct{}

func (g *passingGate) Name() string { return "mock" }
func (g *passingGate) Run(_ context.Context, _ audit.AuditInput) audit.GateResult {
	return audit.GateResult{Pass: true, Gate: "mock"}
}

func testOrchestrator() *Orchestrator {
	provider := llm.NewMockProvider("test-model", "mock response")
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewMockAgent(agent.RoleStrategist),
		agent.RoleResearcher: agent.NewMockAgent(agent.RoleResearcher),
		agent.RoleCoder:      agent.NewMockAgent(agent.RoleCoder),
		agent.RoleAuditor:    agent.NewMockAgent(agent.RoleAuditor),
	}
	return NewOrchestrator(Config{
		FSM:   fsm.New(),
		Store: task.NewMemStore(),
		Runner: agent.NewRunner(agents, map[string]llm.Provider{
			"mock": provider,
		}),
		Dispatcher: dispatch.New(dispatch.Config{
			Defaults: map[string]dispatch.RouteConfig{
				"strategist":       {Tier: "cloud", Provider: "mock", Model: "test"},
				"researcher":       {Tier: "cloud", Provider: "mock", Model: "test"},
				"coder":            {Tier: "local", Provider: "mock", Model: "test"},
				"auditor_semantic": {Tier: "cloud", Provider: "mock", Model: "test"},
			},
			Escalation: dispatch.EscalationConfig{MaxLocalAttempts: 3, PromoteTo: dispatch.RouteConfig{Tier: "cloud", Provider: "mock", Model: "test"}},
		}),
		Policy:   policy.NewEngine(policy.Config{MaxTokensPerTask: 100000, MaxCostPerTask: 5.0}),
		Audit:    allPassingPipeline(),
		Executor: executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:   notary.NewMockNotary(true),
	})
}

func TestOrchestratorSubmitTask(t *testing.T) {
	orch := testOrchestrator()
	ctx := context.Background()

	tk, err := orch.Submit(ctx, "Build a weather CLI", "user-1")
	require.NoError(t, err)
	assert.NotEmpty(t, tk.ID)
	assert.Equal(t, task.StateSubmitted, tk.State)
}

func TestOrchestratorStepThroughHappyPath(t *testing.T) {
	orch := testOrchestrator()
	ctx := context.Background()

	tk, err := orch.Submit(ctx, "Build a weather CLI", "user-1")
	require.NoError(t, err)

	// Step through the entire happy path
	expectedStates := []task.State{
		task.StatePlanning,
		task.StateResearching,
		task.StatePacketValidation,
		task.StateCoding,
		task.StateAuditing,
		task.StateExecuting,
		task.StateOutputReview,
		task.StateCompleted,
	}

	for _, expected := range expectedStates {
		err = orch.Step(ctx, tk)
		require.NoError(t, err, "step to %s should succeed", expected)
		assert.Equal(t, expected, tk.State)
	}
}

func TestOrchestratorPacketAuditRejectsInvalidPacket(t *testing.T) {
	provider := llm.NewMockProvider("test-model", "mock response")
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewMockAgent(agent.RoleStrategist),
		agent.RoleResearcher: agent.NewMockAgent(agent.RoleResearcher),
		agent.RoleCoder:      agent.NewMockAgent(agent.RoleCoder),
		agent.RoleAuditor:    agent.NewMockAgent(agent.RoleAuditor),
	}

	orch := NewOrchestrator(Config{
		FSM:   fsm.New(),
		Store: task.NewMemStore(),
		Runner: agent.NewRunner(agents, map[string]llm.Provider{
			"mock": provider,
		}),
		Dispatcher: dispatch.New(dispatch.Config{
			Defaults: map[string]dispatch.RouteConfig{
				"strategist":       {Tier: "cloud", Provider: "mock", Model: "test"},
				"researcher":       {Tier: "cloud", Provider: "mock", Model: "test"},
				"coder":            {Tier: "local", Provider: "mock", Model: "test"},
				"auditor_semantic": {Tier: "cloud", Provider: "mock", Model: "test"},
			},
			Escalation: dispatch.EscalationConfig{MaxLocalAttempts: 3, PromoteTo: dispatch.RouteConfig{Tier: "cloud", Provider: "mock", Model: "test"}},
		}),
		Policy:      policy.NewEngine(policy.Config{MaxTokensPerTask: 100000, MaxCostPerTask: 5.0}),
		Audit:       allPassingPipeline(),
		PacketAudit: audit.NewPipeline(&rejectingGate{}),
		Executor:    executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:      notary.NewMockNotary(true),
	})

	ctx := context.Background()
	tk, err := orch.Submit(ctx, "test packet rejection", "user-1")
	require.NoError(t, err)

	// Step to PACKET_VALIDATION
	for tk.State != task.StatePacketValidation {
		require.NoError(t, orch.Step(ctx, tk))
	}

	// Packet audit rejects → should go back to RESEARCHING
	err = orch.Step(ctx, tk)
	require.NoError(t, err)
	assert.Equal(t, task.StateResearching, tk.State)
}

type rejectingGate struct{}

func (g *rejectingGate) Name() string { return "reject" }
func (g *rejectingGate) Run(_ context.Context, _ audit.AuditInput) audit.GateResult {
	return audit.GateResult{Pass: false, Gate: "reject", Findings: []string{"packet invalid"}}
}

func TestOrchestratorStepAtCompletedIsNoop(t *testing.T) {
	orch := testOrchestrator()
	ctx := context.Background()

	tk, _ := orch.Submit(ctx, "test", "user-1")

	// Run to completion
	for tk.State != task.StateCompleted {
		require.NoError(t, orch.Step(ctx, tk))
	}

	// Stepping again should be a no-op
	err := orch.Step(ctx, tk)
	require.NoError(t, err)
	assert.Equal(t, task.StateCompleted, tk.State)
}
