package llm

import "context"

// ModelInfo represents metadata about an LLM model.
type ModelInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities"` // e.g., "generateContent", "chat", "embeddings"
}

// Provider is the interface for all LLM backends.
type Provider interface {
	// Name returns the provider identifier (e.g., "anthropic", "google", "ollama").
	Name() string

	// Complete sends a completion request and returns the response.
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)

	// ListModels returns a list of available models for this provider.
	ListModels(ctx context.Context) ([]ModelInfo, error)
}
