package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
)

// ProviderInstanceConfig defines a specific instance of an LLM provider.
type ProviderInstanceConfig struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"` // "anthropic", "google", "ollama", "openai"
	BaseURL    string `yaml:"base_url,omitempty"`
	APIKeyEnv  string `yaml:"api_key_env,omitempty"`
	ModelAlias string `yaml:"model_alias,omitempty"`
}

// RouteConfig specifies where to run an agent.
type RouteConfig struct {
	Tier     string `yaml:"tier"`     // "local" or "cloud"
	Provider string `yaml:"provider"` // refers to a named instance in Providers list
	Model    string `yaml:"model"`
}

// EscalationConfig defines when to promote from local to cloud.
type EscalationConfig struct {
	MaxLocalAttempts int         `yaml:"max_local_attempts"`
	PromoteTo        RouteConfig `yaml:"promote_to"`
}

// Config holds dispatch routing configuration.
type Config struct {
	Providers  []ProviderInstanceConfig `yaml:"providers"`
	Defaults   map[string]RouteConfig   `yaml:"defaults"`
	Escalation EscalationConfig         `yaml:"escalation"`
	Fallback   RouteConfig              `yaml:"fallback"`
}

// Decision is the routing result.
type Decision struct {
	Tier     string
	Provider string
	Model    string
	Reason   string
}

// RouteFilter evaluates and potentially overrides a routing decision.
type RouteFilter interface {
	Filter(ctx context.Context, tk *task.Task, role string, inputTokens int, d Decision) (Decision, error)
}

// Dispatcher routes tasks to compute tiers using a pipeline of filters.
type Dispatcher struct {
	config  Config
	filters []RouteFilter
}

// New creates a dispatcher with the given config and filters.
func New(cfg Config, filters ...RouteFilter) *Dispatcher {
	return &Dispatcher{
		config:  cfg,
		filters: filters,
	}
}

// Route decides the initial provider for a given task and role.
func (d *Dispatcher) Route(ctx context.Context, tk *task.Task, role string) (Decision, error) {
	defaults, ok := d.config.Defaults[role]
	if !ok {
		return Decision{}, fmt.Errorf("no routing config for role %q", role)
	}

	decision := Decision{
		Tier:     defaults.Tier,
		Provider: defaults.Provider,
		Model:    defaults.Model,
		Reason:   "default routing",
	}

	// 1. Initial Escalation logic (Local -> Cloud)
	if defaults.Tier == "local" {
		retries := tk.RetryCount(task.StateCoding, task.StateAuditing)
		if retries >= d.config.Escalation.MaxLocalAttempts {
			promo := d.config.Escalation.PromoteTo
			decision = Decision{
				Tier:     promo.Tier,
				Provider: promo.Provider,
				Model:    promo.Model,
				Reason:   fmt.Sprintf("escalated after %d local attempts", retries),
			}
		}
	}

	return decision, nil
}

// RouteWithFilters applies the filter pipeline to a routing decision.
func (d *Dispatcher) RouteWithFilters(ctx context.Context, tk *task.Task, role string, inputTokens int) (Decision, error) {
	decision, err := d.Route(ctx, tk, role)
	if err != nil {
		return decision, err
	}

	for _, filter := range d.filters {
		var err error
		decision, err = filter.Filter(ctx, tk, role, inputTokens, decision)
		if err != nil {
			return decision, err
		}
	}

	return decision, nil
}

// --- Filter Implementations ---

// CostFilter redirects to local if a cloud call is too expensive.
type CostFilter struct {
	MaxTaskCost float64
	Fallback    RouteConfig
}

func (f *CostFilter) Filter(ctx context.Context, tk *task.Task, role string, inputTokens int, d Decision) (Decision, error) {
	if d.Tier != "cloud" {
		return d, nil
	}

	outputTokens := inputTokens / 4
	cost, err := llm.EstimateCost(d.Model, inputTokens, outputTokens)
	if err != nil {
		return d, nil // skip if unknown
	}

	isEasyRole := strings.Contains(role, "auditor") || strings.Contains(role, "researcher")
	shouldFallback := cost > (f.MaxTaskCost * 0.8)
	if !shouldFallback && isEasyRole && cost > 0.10 {
		shouldFallback = true
	}

	if shouldFallback && f.Fallback.Provider != "" {
		return Decision{
			Tier:     f.Fallback.Tier,
			Provider: f.Fallback.Provider,
			Model:    f.Fallback.Model,
			Reason:   fmt.Sprintf("cost fallback (est. $%.4f > threshold)", cost),
		}, nil
	}

	return d, nil
}

// BudgetHealthFilter redirects to local if total budget is nearly exhausted.
type BudgetHealthFilter struct {
	Billing  *audit.BillingAuditor
	Policy   *policy.Engine
	Fallback RouteConfig
}

func (f *BudgetHealthFilter) Filter(ctx context.Context, tk *task.Task, role string, inputTokens int, d Decision) (Decision, error) {
	if d.Tier != "cloud" || f.Billing == nil || f.Policy == nil {
		return d, nil
	}

	health, err := f.Billing.AssessHealth(ctx, f.Policy.MaxDailyBudget(), f.Policy.MaxMonthlyBudget())
	if err == nil && !health.IsHealthy {
		return Decision{
			Tier:     f.Fallback.Tier,
			Provider: f.Fallback.Provider,
			Model:    f.Fallback.Model,
			Reason:   fmt.Sprintf("budget fallback (Daily %.1f%%, Monthly %.1f%%)", health.DailyPercent, health.MonthlyPercent),
		}, nil
	}

	return d, nil
}
