package dispatch

import (
	"context"
	"fmt"

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
