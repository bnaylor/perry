package llm

import "context"

// Provider is the interface for all LLM backends.
type Provider interface {
	// Name returns the provider identifier (e.g., "anthropic", "google", "ollama").
	Name() string

	// Complete sends a completion request and returns the response.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
