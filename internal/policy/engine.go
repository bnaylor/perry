package policy

import "fmt"

// Config holds all policy rules loaded from config file.
type Config struct {
	AllowedDependencies []string `yaml:"allowed_dependencies"`
	ProhibitedImports   []string `yaml:"prohibited_imports"`
	MaxTokensPerTask    int      `yaml:"max_tokens_per_task"`
	MaxCostPerTask      float64  `yaml:"max_cost_per_task"`
	MaxDailyBudget      float64  `yaml:"max_daily_budget"`
	MaxMonthlyBudget    float64  `yaml:"max_monthly_budget"`
}

// Decision is the result of a policy check.
type Decision struct {
	Allowed bool
	Action  string // "allow", "escalate", "deny"
	Reason  string
}

// Engine evaluates deterministic policy rules.
type Engine struct {
	config           Config
	allowedDepsIndex map[string]bool
	prohibitedIndex  map[string]bool
}

// NewEngine creates a policy engine from config.
func NewEngine(cfg Config) *Engine {
	depsIdx := make(map[string]bool, len(cfg.AllowedDependencies))
	for _, d := range cfg.AllowedDependencies {
		depsIdx[d] = true
	}
	prohibIdx := make(map[string]bool, len(cfg.ProhibitedImports))
	for _, p := range cfg.ProhibitedImports {
		prohibIdx[p] = true
	}
	return &Engine{
		config:           cfg,
		allowedDepsIndex: depsIdx,
		prohibitedIndex:  prohibIdx,
	}
}

// CheckDependency returns whether a package is on the allowlist.
func (e *Engine) CheckDependency(pkg string) Decision {
	if e.allowedDepsIndex[pkg] {
		return Decision{Allowed: true, Action: "allow"}
	}
	return Decision{
		Allowed: false,
		Action:  "escalate",
		Reason:  fmt.Sprintf("package %q not on approved list", pkg),
	}
}

// CheckBudget returns whether token/cost usage is within limits.
func (e *Engine) CheckBudget(tokens int, cost float64) Decision {
	if tokens > e.config.MaxTokensPerTask {
		return Decision{
			Allowed: false,
			Action:  "escalate",
			Reason:  fmt.Sprintf("token usage %d exceeds limit %d", tokens, e.config.MaxTokensPerTask),
		}
	}
	if cost > e.config.MaxCostPerTask {
		return Decision{
			Allowed: false,
			Action:  "escalate",
			Reason:  fmt.Sprintf("cost $%.2f exceeds limit $%.2f", cost, e.config.MaxCostPerTask),
		}
	}
	return Decision{Allowed: true, Action: "allow"}
}

// CheckProhibitedImports returns any imports that match the prohibited list.
func (e *Engine) CheckProhibitedImports(imports []string) []string {
	var violations []string
	for _, imp := range imports {
		if e.prohibitedIndex[imp] {
			violations = append(violations, imp)
		}
	}
	return violations
}

// MaxCost returns the configured maximum cost per task.
func (e *Engine) MaxCost() float64 {
	return e.config.MaxCostPerTask
}

// MaxDailyBudget returns the configured maximum daily budget.
func (e *Engine) MaxDailyBudget() float64 {
	return e.config.MaxDailyBudget
}

// MaxMonthlyBudget returns the configured maximum monthly budget.
func (e *Engine) MaxMonthlyBudget() float64 {
	return e.config.MaxMonthlyBudget
}
