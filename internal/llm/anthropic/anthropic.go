package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bnaylor/perry/internal/llm"
)

const defaultBaseURL = "https://api.anthropic.com"

// Provider implements llm.Provider for the Anthropic Messages API.
// POST https://api.anthropic.com/v1/messages
//
// Key behaviors:
//   - System message extracted from Messages array to top-level "system" field
//   - Always sets max_tokens (default 4096 if not specified -- Anthropic requires it)
//   - Extras map passed through to request body
//   - baseURL field (unexported) allows test override
type Provider struct {
	apiKey  string
	baseURL string
}

// New creates a new Anthropic provider with the given API key.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
	}
}

// Name returns "anthropic".
func (p *Provider) Name() string {
	return "anthropic"
}

// anthropicMessage is the message format for the Anthropic API.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicRequest is the request body for the Anthropic Messages API.
type anthropicRequest struct {
	Model     string             `json:"model"`
	Messages  []anthropicMessage `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
}

// anthropicResponse is the response from the Anthropic Messages API.
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Complete sends a completion request to the Anthropic Messages API.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	// Extract system message and build messages array.
	var system string
	var messages []anthropicMessage
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			system = msg.Content
		} else {
			messages = append(messages, anthropicMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// Default max_tokens to 4096 if not set (Anthropic requires this field).
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	// Build request body. Use a map so we can merge Extras and conditionally
	// include the system field.
	body := map[string]any{
		"model":      req.Model,
		"messages":   messages,
		"max_tokens": maxTokens,
	}
	if system != "" {
		body["system"] = system
	}
	for k, v := range req.Extras {
		body[k] = v
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: send request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	// Extract text content.
	var content string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			content = block.Text
			break
		}
	}

	return llm.CompletionResponse{
		Content: content,
		Model:   apiResp.Model,
		Usage: llm.Usage{
			InputTokens:  apiResp.Usage.InputTokens,
			OutputTokens: apiResp.Usage.OutputTokens,
		},
	}, nil
}
