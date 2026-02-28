package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildProviderMap(t *testing.T) {
	providers := BuildProviderMap(ProviderConfig{
		AnthropicKey: "test-anthropic-key",
		GeminiKey:    "test-gemini-key",
		OllamaURL:    "http://localhost:11434",
	})
	assert.Contains(t, providers, "anthropic")
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "ollama")
	assert.Contains(t, providers, "mock")
	assert.Equal(t, "anthropic", providers["anthropic"].Name())
	assert.Equal(t, "google", providers["google"].Name())
	assert.Equal(t, "ollama", providers["ollama"].Name())
	assert.Equal(t, "mock", providers["mock"].Name())
}

func TestBuildProviderMapDefaults(t *testing.T) {
	providers := BuildProviderMap(ProviderConfig{})
	assert.Contains(t, providers, "anthropic")
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "ollama")
	assert.Contains(t, providers, "mock")
}

func TestBuildProviderMapCustomOllamaURL(t *testing.T) {
	providers := BuildProviderMap(ProviderConfig{
		OllamaURL: "http://gpu-box:11434",
	})
	assert.Contains(t, providers, "ollama")
}
