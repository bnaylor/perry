package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bnaylor/perry/internal/llm"
)

// Provider implements llm.Provider for OpenAI-compatible APIs.
type Provider struct {
	baseURL string
	apiKey  string
}

// New creates a new OpenAI provider with the given base URL and API key.
func New(baseURL, apiKey string) *Provider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &Provider{baseURL: baseURL, apiKey: apiKey}
}

// Name returns "openai".
func (p *Provider) Name() string {
	return "openai"
}

// Complete sends a completion request to the OpenAI API.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{}, fmt.Errorf("openai: Complete not yet implemented")
}

// ListModels fetches the list of available models from the OpenAI API.
func (p *Provider) ListModels(ctx context.Context) ([]llm.ModelInfo, error) {
	url := p.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}

	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: list models API error %d: %s", resp.StatusCode, string(body))
	}

	var listResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}

	var models []llm.ModelInfo
	for _, m := range listResp.Data {
		models = append(models, llm.ModelInfo{
			Name:         m.ID,
			Capabilities: []string{"chat"},
		})
	}
	return models, nil
}
