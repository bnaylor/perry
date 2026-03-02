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

// Provider implements llm.Provider for Anthropic Messages API.
type Provider struct {
	apiKey  string
	baseURL string
}

// New creates a new Anthropic provider with the given API key.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: "https://api.anthropic.com",
	}
}

// Name returns "anthropic".
func (p *Provider) Name() string {
	return "anthropic"
}

// Anthropic API request/response types.
type (
	anthropicMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	anthropicRequest struct {
		Model     string             `json:"model"`
		MaxTokens int                `json:"max_tokens"`
		Messages  []anthropicMessage `json:"messages"`
		System    string             `json:"system,omitempty"`
	}

	anthropicResponse struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
)

// Complete sends a completion request to the Anthropic Messages API.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	areq := anthropicRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
	}
	if areq.MaxTokens == 0 {
		areq.MaxTokens = 4096
	}

	// Separate system message and user/assistant messages.
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			areq.System = msg.Content
			continue
		}
		areq.Messages = append(areq.Messages, anthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	body, err := json.Marshal(areq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var aresp anthropicResponse
	if err := json.Unmarshal(respBody, &aresp); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	if len(aresp.Content) == 0 {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: empty response")
	}

	return llm.CompletionResponse{
		Content: aresp.Content[0].Text,
		Model:   req.Model,
		Usage: llm.Usage{
			InputTokens:  aresp.Usage.InputTokens,
			OutputTokens: aresp.Usage.OutputTokens,
		},
	}, nil
}

// ListModels is not yet implemented for Anthropic.
func (p *Provider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	return nil, fmt.Errorf("anthropic: ListModels not yet implemented")
}
