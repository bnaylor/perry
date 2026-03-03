package dispatch

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func defaultConfig() Config {
	return Config{
		Providers: []ProviderInstanceConfig{
			{Name: "anthropic", Type: "anthropic"},
			{Name: "google", Type: "google"},
			{Name: "ollama", Type: "ollama"},
		},
		Defaults: map[string]RouteConfig{
			"strategist":       {Tier: "cloud", Provider: "anthropic", Model: "claude-3-7-sonnet-20260219"},
			"researcher":       {Tier: "cloud", Provider: "google", Model: "gemini-3.1-pro"},
			"coder":            {Tier: "local", Provider: "ollama", Model: "qwen3:14b"},
			"auditor_semantic": {Tier: "cloud", Provider: "anthropic", Model: "claude-3-7-sonnet-20260219"},
		},
		Escalation: EscalationConfig{
			MaxLocalAttempts: 3,
			PromoteTo:        RouteConfig{Tier: "cloud", Provider: "anthropic", Model: "claude-3-7-sonnet-20260219"},
		},
		Fallback: RouteConfig{Tier: "local", Provider: "ollama", Model: "qwen3:14b"},
	}
}

func TestRouteDefaultCoder(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	decision, err := d.Route(context.Background(), tk, "coder")
	require.NoError(t, err)
	assert.Equal(t, "local", decision.Tier)
	assert.Equal(t, "ollama", decision.Provider)
	assert.Equal(t, "qwen3:14b", decision.Model)
}

func TestRouteDefaultStrategist(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	decision, err := d.Route(context.Background(), tk, "strategist")
	require.NoError(t, err)
	assert.Equal(t, "cloud", decision.Tier)
	assert.Equal(t, "anthropic", decision.Provider)
}

func TestRouteEscalatesAfterRetries(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	// Simulate 3 failed coding->auditing cycles
	for i := 0; i < 3; i++ {
		tk.RecordTransition(task.StateCoding, task.StateAuditing, "submit")
		tk.RecordTransition(task.StateAuditing, task.StateCoding, "revision")
	}

	decision, err := d.Route(context.Background(), tk, "coder")
	require.NoError(t, err)
	assert.Equal(t, "cloud", decision.Tier)
	assert.Equal(t, "anthropic", decision.Provider)
	assert.Contains(t, decision.Reason, "escalated")
}

func TestRouteUnknownRole(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	_, err := d.Route(context.Background(), tk, "nonexistent")
	assert.Error(t, err)
}

func TestRouteWithCostFallback(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	// Large research task triggering cost fallback (> $0.10)
	decision, err := d.RouteWithCost(context.Background(), tk, "researcher", 400000, 10.0)
	require.NoError(t, err)
	assert.Equal(t, "local", decision.Tier)
	assert.Equal(t, "ollama", decision.Provider)
	assert.Contains(t, decision.Reason, "cost fallback")
}

func TestRouteWithCostNoFallback(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	// Small researcher task ($0.005)
	decision, err := d.RouteWithCost(context.Background(), tk, "researcher", 10000, 10.0)
	require.NoError(t, err)
	assert.Equal(t, "cloud", decision.Tier)
	assert.Equal(t, "google", decision.Provider)
}
