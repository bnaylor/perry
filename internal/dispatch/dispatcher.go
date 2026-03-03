package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnaylor/perry/internal/llm"
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
	// Fallback is the local tier to use if cloud cost is too high.
	Fallback RouteConfig `yaml:"fallback"`
}

// Decision is the routing result.
type Decision struct {
	Tier     string
	Provider string
	Model    string
	Reason   string
}

// Dispatcher routes tasks to compute tiers.
type Dispatcher struct {
	config Config
}

// New creates a dispatcher from config.
func New(cfg Config) *Dispatcher {
	return &Dispatcher{config: cfg}
}

// Route decides where to run an agent for a given task and role.
func (d *Dispatcher) Route(_ context.Context, tk *task.Task, role string) (Decision, error) {
	defaults, ok := d.config.Defaults[role]
	if !ok {
		return Decision{}, fmt.Errorf("no routing config for role %q", role)
	}

	// Check if the task has exhausted local retries (only applies to local-tier roles)
	if defaults.Tier == "local" {
		retries := tk.RetryCount(task.StateCoding, task.StateAuditing)
		if retries >= d.config.Escalation.MaxLocalAttempts {
			promo := d.config.Escalation.PromoteTo
			return Decision{
				Tier:     promo.Tier,
				Provider: promo.Provider,
				Model:    promo.Model,
				Reason:   fmt.Sprintf("escalated after %d local attempts", retries),
			}, nil
		}
	}

	return Decision{
		Tier:     defaults.Tier,
		Provider: defaults.Provider,
		Model:    defaults.Model,
		Reason:   "default routing",
	}, nil
}

// RouteWithCost applies cost-based fallback logic before choosing a provider.
func (d *Dispatcher) RouteWithCost(ctx context.Context, tk *task.Task, role string, inputTokens int, maxCost float64) (Decision, error) {
	decision, err := d.Route(ctx, tk, role)
	if err != nil {
		return decision, err
	}

	// Only apply cost fallback if we're currently routed to cloud.
	if decision.Tier != "cloud" {
		return decision, nil
	}

	// Estimate cost (assume output is 25% of input)
	outputTokens := inputTokens / 4
	cost, err := llm.EstimateCost(decision.Model, inputTokens, outputTokens)
	if err != nil {
		// If we can't estimate cost, stick with the original decision but log a warning.
		return decision, nil
	}

	// If cost is > 80% of total budget, or if the role is "easy" (auditor/researcher)
	// and cost is > $0.10, fallback to local.
	isEasyRole := strings.Contains(role, "auditor") || strings.Contains(role, "researcher")
	
	shouldFallback := cost > (maxCost * 0.8)
	if !shouldFallback && isEasyRole && cost > 0.10 {
		shouldFallback = true
	}

	if shouldFallback && d.config.Fallback.Provider != "" {
		return Decision{
			Tier:     d.config.Fallback.Tier,
			Provider: d.config.Fallback.Provider,
			Model:    d.config.Fallback.Model,
			Reason:   fmt.Sprintf("cost fallback (est. $%.4f > threshold)", cost),
		}, nil
	}

	return decision, nil
}
