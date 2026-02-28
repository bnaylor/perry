// internal/dispatch/dispatcher_test.go
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
		Defaults: map[string]RouteConfig{
			"strategist":       {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
			"researcher":       {Tier: "cloud", Provider: "google", Model: "gemini-2.5-pro"},
			"coder":            {Tier: "local", Provider: "ollama", Model: "qwen2.5-coder:32b"},
			"auditor_semantic": {Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
		},
		Escalation: EscalationConfig{
			MaxLocalAttempts: 3,
			PromoteTo:        RouteConfig{Tier: "cloud", Provider: "anthropic", Model: "claude-sonnet-4-6"},
		},
	}
}

func TestRouteDefaultCoder(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	decision, err := d.Route(context.Background(), tk, "coder")
	require.NoError(t, err)
	assert.Equal(t, "local", decision.Tier)
	assert.Equal(t, "ollama", decision.Provider)
	assert.Equal(t, "qwen2.5-coder:32b", decision.Model)
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
	assert.Contains(t, decision.Reason, "escalat")
}

func TestRouteUnknownRole(t *testing.T) {
	d := New(defaultConfig())
	tk := task.New("test", "user-1")

	_, err := d.Route(context.Background(), tk, "nonexistent")
	assert.Error(t, err)
}
