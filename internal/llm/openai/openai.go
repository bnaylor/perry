package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bnaylor/perry/internal/llm"
)

// Provider implements llm.Provider for OpenAI-compatible chat API.
// POST {baseURL}/v1/chat/completions
type Provider struct {
	baseURL string
	apiKey  string
}

func New(baseURL, apiKey string) *Provider {
	return &Provider{baseURL: baseURL, apiKey: apiKey}
}

func (p *Provider) Name() string {
	return "openai"
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	msgs := make([]chatMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = chatMessage{Role: m.Role, Content: m.Content}
	}

	body := chatRequest{
		Model:       req.Model,
		Messages:    msgs,
		Stream:      false,
		Temperature: req.Temperature,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	url := p.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("openai: API error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return llm.CompletionResponse{}, fmt.Errorf("openai: no choices in response")
	}

	return llm.CompletionResponse{
		Content: chatResp.Choices[0].Message.Content,
		Model:   req.Model,
		Usage: llm.Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
		},
	}, nil
}
