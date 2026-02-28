package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRoutingConfig(t *testing.T) {
	content := `
defaults:
  strategist:
    tier: cloud
    provider: anthropic
    model: claude-sonnet-4-6
  coder:
    tier: local
    provider: ollama
    model: qwen2.5-coder:32b
escalation:
  max_local_attempts: 3
  promote_to:
    tier: cloud
    provider: anthropic
    model: claude-sonnet-4-6
`
	path := writeTemp(t, "routing.yaml", content)
	cfg, err := LoadRoutingConfig(path)
	require.NoError(t, err)
	assert.Equal(t, "cloud", cfg.Defaults["strategist"].Tier)
	assert.Equal(t, "anthropic", cfg.Defaults["strategist"].Provider)
	assert.Equal(t, "claude-sonnet-4-6", cfg.Defaults["strategist"].Model)
	assert.Equal(t, "local", cfg.Defaults["coder"].Tier)
	assert.Equal(t, 3, cfg.Escalation.MaxLocalAttempts)
	assert.Equal(t, "anthropic", cfg.Escalation.PromoteTo.Provider)
}

func TestLoadPolicyConfig(t *testing.T) {
	content := `
allowed_dependencies:
  - json
  - requests
prohibited_imports:
  - eval
  - exec
max_tokens_per_task: 500000
max_cost_per_task: 10.00
`
	path := writeTemp(t, "policy.yaml", content)
	cfg, err := LoadPolicyConfig(path)
	require.NoError(t, err)
	assert.Contains(t, cfg.AllowedDependencies, "requests")
	assert.Contains(t, cfg.ProhibitedImports, "eval")
	assert.Equal(t, 500000, cfg.MaxTokensPerTask)
	assert.Equal(t, 10.0, cfg.MaxCostPerTask)
}

func TestLoadRoutingConfigFileNotFound(t *testing.T) {
	_, err := LoadRoutingConfig("/nonexistent/file.yaml")
	assert.Error(t, err)
}

func TestLoadPolicyConfigFileNotFound(t *testing.T) {
	_, err := LoadPolicyConfig("/nonexistent/file.yaml")
	assert.Error(t, err)
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}
