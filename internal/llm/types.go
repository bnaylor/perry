package llm

// Message represents a single message in a conversation.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// CompletionRequest is the provider-agnostic request.
type CompletionRequest struct {
	Model       string         `json:"model,omitempty"`
	Messages    []Message      `json:"messages"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Extras      map[string]any `json:"extras,omitempty"` // provider-specific
}

// CompletionResponse is the provider-agnostic response.
type CompletionResponse struct {
	Content  string         `json:"content"`
	Model    string         `json:"model"`
	Usage    Usage          `json:"usage"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
