package providers

import (
	"testing"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/stretchr/testify/assert"
)

func TestBuildProviderMap(t *testing.T) {
	providers := BuildProviderMap(dispatch.Config{
		Providers: []dispatch.ProviderInstanceConfig{
			{Name: "anthropic", Type: "anthropic"},
			{Name: "google", Type: "google"},
			{Name: "ollama", Type: "ollama"},
			{Name: "custom", Type: "openai", BaseURL: "http://test:1234"},
		},
	})
	assert.Contains(t, providers, "anthropic")
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "ollama")
	assert.Contains(t, providers, "custom")
	assert.Contains(t, providers, "mock")
	assert.Equal(t, "anthropic", providers["anthropic"].Name())
	assert.Equal(t, "google", providers["google"].Name())
	assert.Equal(t, "ollama", providers["ollama"].Name())
	assert.Equal(t, "openai", providers["custom"].Name())
	assert.Equal(t, "mock", providers["mock"].Name())
}

func TestBuildProviderMapDefaults(t *testing.T) {
	providers := BuildProviderMap(dispatch.Config{})
	assert.Contains(t, providers, "mock")
}

func TestBuildProviderMapCustomOllamaURL(t *testing.T) {
	providers := BuildProviderMap(dispatch.Config{
		Providers: []dispatch.ProviderInstanceConfig{
			{Name: "gpu-node", Type: "ollama", BaseURL: "http://gpu-box:11434"},
		},
	})
	assert.Contains(t, providers, "gpu-node")
}
