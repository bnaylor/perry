// internal/policy/engine_test.go
package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckDependencyAllowed(t *testing.T) {
	e := NewEngine(Config{
		AllowedDependencies: []string{"requests", "json", "pandas"},
	})
	result := e.CheckDependency("requests")
	assert.True(t, result.Allowed)
}

func TestCheckDependencyDenied(t *testing.T) {
	e := NewEngine(Config{
		AllowedDependencies: []string{"requests", "json"},
	})
	result := e.CheckDependency("subprocess")
	assert.False(t, result.Allowed)
	assert.Equal(t, "escalate", result.Action)
}

func TestCheckBudgetWithinLimits(t *testing.T) {
	e := NewEngine(Config{
		MaxTokensPerTask: 100000,
		MaxCostPerTask:   5.00,
	})
	result := e.CheckBudget(50000, 2.50)
	assert.True(t, result.Allowed)
}

func TestCheckBudgetExceeded(t *testing.T) {
	e := NewEngine(Config{
		MaxTokensPerTask: 100000,
		MaxCostPerTask:   5.00,
	})
	result := e.CheckBudget(150000, 2.50)
	assert.False(t, result.Allowed)
	assert.Equal(t, "escalate", result.Action)
	assert.Contains(t, result.Reason, "token")
}

func TestCheckBudgetCostExceeded(t *testing.T) {
	e := NewEngine(Config{
		MaxTokensPerTask: 100000,
		MaxCostPerTask:   5.00,
	})
	result := e.CheckBudget(50000, 7.00)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "cost")
}

func TestImmutableSafetyRules(t *testing.T) {
	e := NewEngine(Config{
		ProhibitedImports: []string{"eval", "exec", "os.system"},
	})
	violations := e.CheckProhibitedImports([]string{"json", "os.system", "requests"})
	assert.Len(t, violations, 1)
	assert.Equal(t, "os.system", violations[0])
}
