package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bnaylor/perry/internal/llm"
)

// Provider implements llm.Provider for Ollama chat API.
type Provider struct {
	baseURL string
}

// New creates a new Ollama provider with the given base URL.
func New(baseURL string) *Provider {
	return &Provider{baseURL: baseURL}
}

// Name returns "ollama".
func (p *Provider) Name() string {
	return "ollama"
}

// chatMessage is the Ollama message format.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatRequest is the Ollama /api/chat request body.
type chatRequest struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Stream   bool           `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

// chatResponse is the Ollama /api/chat response body.
type chatResponse struct {
	Model           string      `json:"model"`
	Message         chatMessage `json:"message"`
	PromptEvalCount int         `json:"prompt_eval_count"`
	EvalCount       int         `json:"eval_count"`
}

// Complete sends a completion request to the Ollama chat API.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	// Build messages — pass through as-is.
	msgs := make([]chatMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMessage{Role: m.Role, Content: m.Content}
	}

	// Build request body.
	body := chatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   false,
	}

	// Add temperature if set.
	if req.Temperature > 0 {
		body.Options = map[string]any{
			"temperature": req.Temperature,
		}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	return llm.CompletionResponse{
		Content: chatResp.Message.Content,
		Model:   chatResp.Model,
		Usage: llm.Usage{
			InputTokens:  chatResp.PromptEvalCount,
			OutputTokens: chatResp.EvalCount,
		},
	}, nil
}

// ListModels returns the list of models available on the Ollama node.
func (p *Provider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	resp, err := http.Get(p.baseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("ollama: list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama: list models API error %d: %s", resp.StatusCode, string(body))
	}

	var tagsResp struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("ollama: decode tags response: %w", err)
	}

	var models []llm.ModelInfo
	for _, m := range tagsResp.Models {
		models = append(models, llm.ModelInfo{
			Name:         m.Name,
			Capabilities: []string{"chat"}, // Ollama default
		})
	}
	return models, nil
}
