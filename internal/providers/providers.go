package providers

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/llm/anthropic"
	"github.com/bnaylor/perry/internal/llm/gemini"
	"github.com/bnaylor/perry/internal/llm/ollama"
)

const defaultOllamaURL = "http://localhost:11434"

type ProviderConfig struct {
	AnthropicKey string
	GeminiKey    string
	OllamaURL    string
}

func BuildProviderMap(cfg ProviderConfig) map[string]llm.Provider {
	ollamaURL := cfg.OllamaURL
	if ollamaURL == "" {
		ollamaURL = defaultOllamaURL
	}
	return map[string]llm.Provider{
		"anthropic": anthropic.New(cfg.AnthropicKey),
		"google":    gemini.New(cfg.GeminiKey),
		"ollama":    ollama.New(ollamaURL),
		"mock":      llm.NewMockProvider("mock", "mock response"),
	}
}
