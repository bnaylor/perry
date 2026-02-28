package providers

import (
	"os"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/llm/anthropic"
	"github.com/bnaylor/perry/internal/llm/gemini"
	"github.com/bnaylor/perry/internal/llm/ollama"
	"github.com/bnaylor/perry/internal/llm/openai"
)

func BuildProviderMap(cfg dispatch.Config) map[string]llm.Provider {
	m := make(map[string]llm.Provider)

	for _, p := range cfg.Providers {
		var provider llm.Provider
		apiKey := ""
		if p.APIKeyEnv != "" {
			apiKey = os.Getenv(p.APIKeyEnv)
		}

		switch p.Type {
		case "anthropic":
			provider = anthropic.New(apiKey)
		case "google":
			provider = gemini.New(apiKey)
		case "ollama":
			url := p.BaseURL
			if url == "" {
				url = "http://localhost:11434"
			}
			provider = ollama.New(url)
		case "openai":
			provider = openai.New(p.BaseURL, apiKey)
		case "mock":
			provider = llm.NewMockProvider(p.Name, "mock response")
		default:
			// Unknown provider type, skip or log warning
			continue
		}
		m[p.Name] = provider
	}

	// Always add mock for testing if not present
	if _, ok := m["mock"]; !ok {
		m["mock"] = llm.NewMockProvider("mock", "mock response")
	}

	return m
}
