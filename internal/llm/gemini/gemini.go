package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bnaylor/perry/internal/llm"
)

// Provider implements llm.Provider for Gemini generateContent API.
// POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={key}
//
// Key behaviors:
// - System message -> system_instruction.parts
// - Role mapping: "assistant" -> "model" (Gemini convention)
// - Messages as contents with parts[].text structure
// - API key in query string, not header
// - baseURL field for test override
type Provider struct {
	apiKey  string
	baseURL string
}

// New creates a new Gemini provider with the given API key.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: "https://generativelanguage.googleapis.com",
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string {
	return "google"
}

// Gemini API request/response types.
type (
	geminiRequest struct {
		SystemInstruction *geminiContent    `json:"system_instruction,omitempty"`
		Contents          []geminiContent   `json:"contents"`
		GenerationConfig  *generationConfig `json:"generationConfig,omitempty"`
	}

	geminiContent struct {
		Role  string       `json:"role,omitempty"`
		Parts []geminiPart `json:"parts"`
	}

	geminiPart struct {
		Text string `json:"text"`
	}

	generationConfig struct {
		MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
		Temperature     float64 `json:"temperature,omitempty"`
	}

	geminiResponse struct {
		Candidates    []geminiCandidate `json:"candidates"`
		UsageMetadata geminiUsage       `json:"usageMetadata"`
	}

	geminiCandidate struct {
		Content geminiContent `json:"content"`
	}

	geminiUsage struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	}
)

// Complete sends a completion request to the Gemini API.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	greq := geminiRequest{}

	// Separate system messages and content messages.
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			greq.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
			continue
		}
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		greq.Contents = append(greq.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	// Set generation config if temperature or max tokens specified.
	if req.Temperature != 0 || req.MaxTokens != 0 {
		greq.GenerationConfig = &generationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		}
	}

	body, err := json.Marshal(greq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: send request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var gresp geminiResponse
	if err := json.Unmarshal(respBody, &gresp); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: unmarshal response: %w", err)
	}

	if len(gresp.Candidates) == 0 || len(gresp.Candidates[0].Content.Parts) == 0 {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: empty response")
	}

	return llm.CompletionResponse{
		Content: gresp.Candidates[0].Content.Parts[0].Text,
		Model:   req.Model,
		Usage: llm.Usage{
			InputTokens:  gresp.UsageMetadata.PromptTokenCount,
			OutputTokens: gresp.UsageMetadata.CandidatesTokenCount,
		},
	}, nil
}
