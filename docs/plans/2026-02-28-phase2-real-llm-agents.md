# Phase 2: Real LLM Agents — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace mock LLM providers and stub agents with real implementations so perry can accept a task, route it through real LLMs (Claude, Gemini, Ollama), and produce real structured output at each pipeline stage.

**Architecture:** Three thin HTTP-based LLM providers implement the existing `llm.Provider` interface. The Runner is upgraded from a single provider to a provider map. The Dispatcher's routing decisions drive provider selection. Real agent prompts produce structured JSON output. A YAML config loader replaces hardcoded config in `main.go`.

**Tech Stack:** Go 1.25, `net/http` for LLM API calls, `gopkg.in/yaml.v3` for config loading, `net/http/httptest` for provider unit tests.

**Design doc:** `docs/plans/2026-02-28-phase2-real-llm-agents-design.md`

---

## Batch 1: Foundation (Tasks 1–4)

### Task 1: Add Model Field to CompletionRequest

The Dispatcher produces a `Decision` with a `Model` field, but `CompletionRequest` has no `Model` field — the provider doesn't know which model to use. Add it.

**Files:**
- Modify: `internal/llm/types.go`
- Modify: `internal/llm/mock.go`
- Test: `internal/llm/types_test.go` (create)

**Step 1: Write the failing test**

Create `internal/llm/types_test.go`:

```go
package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompletionRequestHasModelField(t *testing.T) {
	req := CompletionRequest{
		Model:       "claude-sonnet-4-6",
		Messages:    []Message{{Role: "user", Content: "hello"}},
		MaxTokens:   1024,
		Temperature: 0.7,
	}
	assert.Equal(t, "claude-sonnet-4-6", req.Model)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ -run TestCompletionRequestHasModelField -v`
Expected: FAIL — `CompletionRequest` has no field `Model`

**Step 3: Add Model field to CompletionRequest**

In `internal/llm/types.go`, add `Model` as the first field:

```go
type CompletionRequest struct {
	Model       string         `json:"model,omitempty"`
	Messages    []Message      `json:"messages"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Extras      map[string]any `json:"extras,omitempty"`
}
```

**Step 4: Verify MockProvider records the Model field**

The `MockProvider.Complete` already records the full `CompletionRequest` in `m.Requests`, so the Model field is automatically captured. No changes needed to `mock.go`.

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/llm/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/llm/types.go internal/llm/types_test.go
git commit -m "feat(llm): add Model field to CompletionRequest"
```

---

### Task 2: Anthropic Provider

Implement the Anthropic Messages API provider. Uses direct HTTP — no SDK.

**Files:**
- Create: `internal/llm/anthropic/anthropic.go`
- Create: `internal/llm/anthropic/anthropic_test.go`

**Reference:** [Anthropic Messages API](https://docs.anthropic.com/en/api/messages) — `POST https://api.anthropic.com/v1/messages`

**Step 1: Write the failing test**

Create `internal/llm/anthropic/anthropic_test.go`:

```go
package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicProviderName(t *testing.T) {
	p := New("test-key")
	assert.Equal(t, "anthropic", p.Name())
}

func TestAnthropicComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		// System message should be extracted to top level
		assert.Equal(t, "You are helpful.", body["system"])
		// Messages should not contain system message
		msgs := body["messages"].([]any)
		assert.Len(t, msgs, 1)
		msg := msgs[0].(map[string]any)
		assert.Equal(t, "user", msg["role"])
		assert.Equal(t, "hello", msg["content"])

		assert.Equal(t, "test-model", body["model"])
		assert.Equal(t, float64(1024), body["max_tokens"])

		// Return mock response
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hello back!"},
			},
			"model": "test-model",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New("test-key")
	p.baseURL = server.URL // override for testing

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:     "test-model",
		Messages: []llm.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "hello"},
		},
		MaxTokens: 1024,
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello back!", resp.Content)
	assert.Equal(t, "test-model", resp.Model)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
}

func TestAnthropicNoSystemMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		// No system field when no system message provided
		_, hasSystem := body["system"]
		assert.False(t, hasSystem)

		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "response"},
			},
			"model": "m",
			"usage": map[string]any{"input_tokens": 1, "output_tokens": 1},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New("test-key")
	p.baseURL = server.URL

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:    "m",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	require.NoError(t, err)
}

func TestAnthropicAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"type":    "invalid_request_error",
				"message": "model not found",
			},
		})
	}))
	defer server.Close()

	p := New("test-key")
	p.baseURL = server.URL

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:    "bad-model",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

// Verify Provider interface compliance at compile time.
var _ llm.Provider = (*Provider)(nil)
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/anthropic/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement the Anthropic provider**

Create `internal/llm/anthropic/anthropic.go`:

```go
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
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// New creates an Anthropic provider.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{},
	}
}

func (p *Provider) Name() string { return "anthropic" }

func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	body := p.buildRequestBody(req)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := p.client.Do(httpReq)
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

	return p.parseResponse(respBody)
}

func (p *Provider) buildRequestBody(req llm.CompletionRequest) map[string]any {
	body := map[string]any{
		"model": req.Model,
	}

	// Extract system message — Anthropic puts it at top level, not in messages array.
	var messages []map[string]string
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			body["system"] = msg.Content
			continue
		}
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}
	body["messages"] = messages

	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	} else {
		body["max_tokens"] = 4096 // Anthropic requires max_tokens
	}

	if req.Temperature > 0 {
		body["temperature"] = req.Temperature
	}

	// Pass through any extras
	for k, v := range req.Extras {
		body[k] = v
	}

	return body
}

func (p *Provider) parseResponse(body []byte) (llm.CompletionResponse, error) {
	var raw struct {
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

	if err := json.Unmarshal(body, &raw); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("anthropic: parse response: %w", err)
	}

	var content string
	for _, block := range raw.Content {
		if block.Type == "text" {
			content = block.Text
			break
		}
	}

	return llm.CompletionResponse{
		Content: content,
		Model:   raw.Model,
		Usage: llm.Usage{
			InputTokens:  raw.Usage.InputTokens,
			OutputTokens: raw.Usage.OutputTokens,
		},
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/llm/anthropic/ -v`
Expected: ALL PASS (4 tests)

**Step 5: Commit**

```bash
git add internal/llm/anthropic/
git commit -m "feat(llm): add Anthropic Messages API provider"
```

---

### Task 3: Gemini Provider

Implement the Gemini `generateContent` REST API provider.

**Files:**
- Create: `internal/llm/gemini/gemini.go`
- Create: `internal/llm/gemini/gemini_test.go`

**Reference:** [Gemini API](https://ai.google.dev/api/generate-content) — `POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`

**Step 1: Write the failing test**

Create `internal/llm/gemini/gemini_test.go`:

```go
package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiProviderName(t *testing.T) {
	p := New("test-key")
	assert.Equal(t, "google", p.Name())
}

func TestGeminiComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/v1beta/models/test-model:generateContent")
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		// System instruction should be separate
		sysInstr := body["system_instruction"].(map[string]any)
		parts := sysInstr["parts"].([]any)
		assert.Equal(t, "You are helpful.", parts[0].(map[string]any)["text"])

		// Contents should have user/model roles (not user/assistant)
		contents := body["contents"].([]any)
		assert.Len(t, contents, 1)
		content := contents[0].(map[string]any)
		assert.Equal(t, "user", content["role"])

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "Hello from Gemini!"},
						},
					},
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     15,
				"candidatesTokenCount": 8,
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New("test-key")
	p.baseURL = server.URL

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "hello"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello from Gemini!", resp.Content)
	assert.Equal(t, "test-model", resp.Model)
	assert.Equal(t, 15, resp.Usage.InputTokens)
	assert.Equal(t, 8, resp.Usage.OutputTokens)
}

func TestGeminiRoleMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		contents := body["contents"].([]any)
		assert.Len(t, contents, 2)
		// "assistant" should be mapped to "model"
		assert.Equal(t, "user", contents[0].(map[string]any)["role"])
		assert.Equal(t, "model", contents[1].(map[string]any)["role"])

		resp := map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "ok"}}}},
			},
			"usageMetadata": map[string]any{"promptTokenCount": 1, "candidatesTokenCount": 1},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New("test-key")
	p.baseURL = server.URL

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "m",
		Messages: []llm.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hey"},
		},
	})
	require.NoError(t, err)
}

func TestGeminiAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "API key invalid"},
		})
	}))
	defer server.Close()

	p := New("bad-key")
	p.baseURL = server.URL

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:    "m",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

var _ llm.Provider = (*Provider)(nil)
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/gemini/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement the Gemini provider**

Create `internal/llm/gemini/gemini.go`:

```go
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

const defaultBaseURL = "https://generativelanguage.googleapis.com"

// Provider implements llm.Provider for the Gemini generateContent API.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// New creates a Gemini provider.
func New(apiKey string) *Provider {
	return &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{},
	}
}

func (p *Provider) Name() string { return "google" }

func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	body := p.buildRequestBody(req)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
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

	return p.parseResponse(respBody, req.Model)
}

func (p *Provider) buildRequestBody(req llm.CompletionRequest) map[string]any {
	body := map[string]any{}

	var contents []map[string]any
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			body["system_instruction"] = map[string]any{
				"parts": []map[string]any{{"text": msg.Content}},
			}
			continue
		}

		// Gemini uses "model" instead of "assistant"
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}

		contents = append(contents, map[string]any{
			"role":  role,
			"parts": []map[string]any{{"text": msg.Content}},
		})
	}
	body["contents"] = contents

	if req.Temperature > 0 || req.MaxTokens > 0 {
		genConfig := map[string]any{}
		if req.Temperature > 0 {
			genConfig["temperature"] = req.Temperature
		}
		if req.MaxTokens > 0 {
			genConfig["maxOutputTokens"] = req.MaxTokens
		}
		body["generationConfig"] = genConfig
	}

	return body
}

func (p *Provider) parseResponse(body []byte, model string) (llm.CompletionResponse, error) {
	var raw struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("gemini: parse response: %w", err)
	}

	var content string
	if len(raw.Candidates) > 0 && len(raw.Candidates[0].Content.Parts) > 0 {
		content = raw.Candidates[0].Content.Parts[0].Text
	}

	return llm.CompletionResponse{
		Content: content,
		Model:   model,
		Usage: llm.Usage{
			InputTokens:  raw.UsageMetadata.PromptTokenCount,
			OutputTokens: raw.UsageMetadata.CandidatesTokenCount,
		},
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/llm/gemini/ -v`
Expected: ALL PASS (4 tests)

**Step 5: Commit**

```bash
git add internal/llm/gemini/
git commit -m "feat(llm): add Gemini generateContent API provider"
```

---

### Task 4: Ollama Provider

Implement the Ollama chat completions API provider.

**Files:**
- Create: `internal/llm/ollama/ollama.go`
- Create: `internal/llm/ollama/ollama_test.go`

**Reference:** [Ollama API](https://github.com/ollama/ollama/blob/main/docs/api.md#generate-a-chat-completion) — `POST http://localhost:11434/api/chat`

**Step 1: Write the failing test**

Create `internal/llm/ollama/ollama_test.go`:

```go
package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaProviderName(t *testing.T) {
	p := New("http://localhost:11434")
	assert.Equal(t, "ollama", p.Name())
}

func TestOllamaComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/chat", r.URL.Path)

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))

		assert.Equal(t, "test-model", body["model"])
		assert.Equal(t, false, body["stream"])

		msgs := body["messages"].([]any)
		assert.Len(t, msgs, 2)
		assert.Equal(t, "system", msgs[0].(map[string]any)["role"])
		assert.Equal(t, "user", msgs[1].(map[string]any)["role"])

		resp := map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": "Hello from Ollama!",
			},
			"model": "test-model",
			"prompt_eval_count":    12,
			"eval_count":           6,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := New(server.URL)

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "hello"},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "Hello from Ollama!", resp.Content)
	assert.Equal(t, "test-model", resp.Model)
	assert.Equal(t, 12, resp.Usage.InputTokens)
	assert.Equal(t, 6, resp.Usage.OutputTokens)
}

func TestOllamaConnectionError(t *testing.T) {
	p := New("http://localhost:1") // nothing listening

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:    "m",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ollama")
}

func TestOllamaServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("model not found"))
	}))
	defer server.Close()

	p := New(server.URL)

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:    "bad-model",
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

var _ llm.Provider = (*Provider)(nil)
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ollama/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement the Ollama provider**

Create `internal/llm/ollama/ollama.go`:

```go
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

// Provider implements llm.Provider for the Ollama chat API.
type Provider struct {
	baseURL string
	client  *http.Client
}

// New creates an Ollama provider. baseURL is typically "http://localhost:11434".
func New(baseURL string) *Provider {
	return &Provider{
		baseURL: baseURL,
		client:  &http.Client{},
	}
}

func (p *Provider) Name() string { return "ollama" }

func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	body := p.buildRequestBody(req)

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: send request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	return p.parseResponse(respBody)
}

func (p *Provider) buildRequestBody(req llm.CompletionRequest) map[string]any {
	var messages []map[string]string
	for _, msg := range req.Messages {
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	body := map[string]any{
		"model":    req.Model,
		"messages": messages,
		"stream":   false, // we want the full response, not streaming
	}

	if req.Temperature > 0 {
		body["options"] = map[string]any{
			"temperature": req.Temperature,
		}
	}

	return body
}

func (p *Provider) parseResponse(body []byte) (llm.CompletionResponse, error) {
	var raw struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Model           string `json:"model"`
		PromptEvalCount int    `json:"prompt_eval_count"`
		EvalCount       int    `json:"eval_count"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return llm.CompletionResponse{}, fmt.Errorf("ollama: parse response: %w", err)
	}

	return llm.CompletionResponse{
		Content: raw.Message.Content,
		Model:   raw.Model,
		Usage: llm.Usage{
			InputTokens:  raw.PromptEvalCount,
			OutputTokens: raw.EvalCount,
		},
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/llm/ollama/ -v`
Expected: ALL PASS (4 tests)

**Step 5: Run all LLM package tests**

Run: `go test ./internal/llm/... -v`
Expected: ALL PASS across all four packages (llm, anthropic, gemini, ollama)

**Step 6: Commit**

```bash
git add internal/llm/ollama/
git commit -m "feat(llm): add Ollama chat API provider"
```

---

## Batch 2: Runner Upgrade + Agent Prompts (Tasks 5–8)

### Task 5: Upgrade Runner to Provider Map

The Runner currently takes a single `llm.Provider`. Upgrade it to a `map[string]llm.Provider` so the Dispatcher's routing decision can select which provider handles each call.

**Files:**
- Modify: `internal/agent/runner.go`
- Modify: `internal/agent/runner_test.go`

**Step 1: Write the failing tests**

Add to `internal/agent/runner_test.go` (keeping existing tests and updating them):

```go
package agent

import (
	"context"
	"testing"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerExecuteWithRouting(t *testing.T) {
	mockClaude := llm.NewMockProvider("claude", "strategist response")
	mockOllama := llm.NewMockProvider("ollama", "coder response")

	providers := map[string]llm.Provider{
		"anthropic": mockClaude,
		"ollama":    mockOllama,
	}

	agents := map[Role]Agent{
		RoleStrategist: NewMockAgent(RoleStrategist),
		RoleCoder:      NewMockAgent(RoleCoder),
	}

	runner := NewRunner(agents, providers)
	tk := task.New("test task", "user")

	// Route strategist to anthropic
	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan this", dispatch.Decision{
		Provider: "anthropic",
		Model:    "claude-sonnet-4-6",
	})
	require.NoError(t, err)
	assert.Equal(t, "strategist response", out.Content)
	assert.Equal(t, RoleStrategist, out.Role)
	// Verify model was set on the request
	require.Len(t, mockClaude.Requests, 1)
	assert.Equal(t, "claude-sonnet-4-6", mockClaude.Requests[0].Model)

	// Route coder to ollama
	out, err = runner.Execute(ctx(t), tk, RoleCoder, "write code", dispatch.Decision{
		Provider: "ollama",
		Model:    "qwen2.5-coder:32b",
	})
	require.NoError(t, err)
	assert.Equal(t, "coder response", out.Content)
	require.Len(t, mockOllama.Requests, 1)
	assert.Equal(t, "qwen2.5-coder:32b", mockOllama.Requests[0].Model)
}

func TestRunnerUnknownProvider(t *testing.T) {
	providers := map[string]llm.Provider{
		"mock": llm.NewMockProvider("m", "r"),
	}
	agents := map[Role]Agent{
		RoleStrategist: NewMockAgent(RoleStrategist),
	}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	_, err := runner.Execute(ctx(t), tk, RoleStrategist, "input", dispatch.Decision{
		Provider: "nonexistent",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no provider")
}

func TestRunnerUnknownRole(t *testing.T) {
	providers := map[string]llm.Provider{
		"mock": llm.NewMockProvider("m", "r"),
	}
	runner := NewRunner(map[Role]Agent{}, providers)
	tk := task.New("test", "user")

	_, err := runner.Execute(ctx(t), tk, RoleStrategist, "input", dispatch.Decision{
		Provider: "mock",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no agent")
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestRunnerExecuteWithRouting -v`
Expected: FAIL — `NewRunner` signature mismatch, `Execute` doesn't accept `Decision`

**Step 3: Update Runner**

Replace `internal/agent/runner.go`:

```go
package agent

import (
	"context"
	"fmt"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

// Runner executes agents by dispatching to LLM providers.
type Runner struct {
	agents    map[Role]Agent
	providers map[string]llm.Provider
}

// NewRunner creates an agent runner with a provider map.
func NewRunner(agents map[Role]Agent, providers map[string]llm.Provider) *Runner {
	return &Runner{agents: agents, providers: providers}
}

// Execute runs the agent for the given role using the routed provider.
func (r *Runner) Execute(ctx context.Context, tk *task.Task, role Role, input string, decision dispatch.Decision) (AgentOutput, error) {
	agent, ok := r.agents[role]
	if !ok {
		return AgentOutput{}, fmt.Errorf("no agent registered for role %q", role)
	}

	provider, ok := r.providers[decision.Provider]
	if !ok {
		return AgentOutput{}, fmt.Errorf("no provider %q in provider map", decision.Provider)
	}

	messages := agent.BuildMessages(tk, input)
	resp, err := provider.Complete(ctx, llm.CompletionRequest{
		Model:    decision.Model,
		Messages: messages,
	})
	if err != nil {
		return AgentOutput{}, fmt.Errorf("LLM call failed for %s: %w", role, err)
	}

	return AgentOutput{
		Role:    role,
		Content: resp.Content,
		Usage:   resp.Usage,
	}, nil
}
```

**Step 4: Update the orchestrator to pass routing decisions**

The orchestrator calls `runner.Execute` without a `Decision`. Update `internal/orchestrator/orchestrator.go` — in each `runner.Execute` call, first route through the dispatcher:

In the `determineNextState` method, update the `StatePlanning`, `StateResearching`, and `StateCoding` cases to route through the dispatcher. Here's the pattern for each:

```go
case task.StatePlanning:
    decision, err := o.dispatcher.Route(ctx, tk, string(agent.RoleStrategist))
    if err != nil {
        return task.StateHumanReview, "routing error", nil
    }
    _, err = o.runner.Execute(ctx, tk, agent.RoleStrategist, tk.Description, decision)
    if err != nil {
        return task.StateHumanReview, "strategist error", nil
    }
    return task.StateResearching, "requirements ready", nil

case task.StateResearching:
    decision, err := o.dispatcher.Route(ctx, tk, string(agent.RoleResearcher))
    if err != nil {
        return "", "", fmt.Errorf("routing failed: %w", err)
    }
    _, err = o.runner.Execute(ctx, tk, agent.RoleResearcher, "gather context", decision)
    if err != nil {
        return "", "", fmt.Errorf("researcher failed: %w", err)
    }
    return task.StatePacketValidation, "context packet produced", nil

case task.StateCoding:
    decision, err := o.dispatcher.Route(ctx, tk, string(agent.RoleCoder))
    if err != nil {
        return "", "", fmt.Errorf("routing failed: %w", err)
    }
    _, err = o.runner.Execute(ctx, tk, agent.RoleCoder, "generate code", decision)
    if err != nil {
        return "", "", fmt.Errorf("coder failed: %w", err)
    }
    return task.StateAuditing, "code ready for audit", nil
```

**Step 5: Run all tests**

Run: `go test ./internal/agent/ ./internal/orchestrator/ -v`
Expected: ALL PASS

Note: Existing orchestrator tests will need their `NewRunner` calls updated from `NewRunner(agents, provider)` to `NewRunner(agents, map[string]llm.Provider{"mock": provider})`, and `Execute` calls will need a `dispatch.Decision{Provider: "mock"}` added. Fix these compilation errors as they appear.

**Step 6: Commit**

```bash
git add internal/agent/runner.go internal/agent/runner_test.go internal/orchestrator/orchestrator.go
git commit -m "feat(agent): upgrade Runner to provider map with routing decisions"
```

---

### Task 6: Add Parsed Field to AgentOutput

Agents produce JSON. Add a `Parsed` field to `AgentOutput` and a helper that attempts JSON parsing of the content.

**Files:**
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/runner.go`
- Test: `internal/agent/agent_test.go` (create or extend)

**Step 1: Write the failing test**

Add to `internal/agent/runner_test.go`:

```go
func TestRunnerParsesJSON(t *testing.T) {
	jsonResp := `{"requirements": ["req1"], "complexity": "standard"}`
	providers := map[string]llm.Provider{
		"mock": llm.NewMockProvider("m", jsonResp),
	}
	agents := map[Role]Agent{
		RoleStrategist: NewMockAgent(RoleStrategist),
	}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan", dispatch.Decision{
		Provider: "mock",
		Model:    "m",
	})
	require.NoError(t, err)
	assert.Equal(t, jsonResp, out.Content)
	require.NotNil(t, out.Parsed)
	assert.Equal(t, "standard", out.Parsed["complexity"])
	reqs := out.Parsed["requirements"].([]any)
	assert.Equal(t, "req1", reqs[0])
}

func TestRunnerNonJSONContent(t *testing.T) {
	providers := map[string]llm.Provider{
		"mock": llm.NewMockProvider("m", "this is not JSON"),
	}
	agents := map[Role]Agent{
		RoleStrategist: NewMockAgent(RoleStrategist),
	}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan", dispatch.Decision{
		Provider: "mock",
		Model:    "m",
	})
	require.NoError(t, err)
	assert.Equal(t, "this is not JSON", out.Content)
	assert.Nil(t, out.Parsed) // graceful — no error, just nil
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestRunnerParsesJSON -v`
Expected: FAIL — `AgentOutput` has no `Parsed` field

**Step 3: Add Parsed field and JSON extraction**

In `internal/agent/agent.go`, add the `Parsed` field:

```go
type AgentOutput struct {
	Role    Role
	Content string
	Parsed  map[string]any // parsed JSON from Content (nil if not valid JSON)
	Usage   llm.Usage
}
```

In `internal/agent/runner.go`, after constructing the `AgentOutput`, attempt JSON parsing:

```go
import "encoding/json"

// In Execute, after building the output:
output := AgentOutput{
    Role:    role,
    Content: resp.Content,
    Usage:   resp.Usage,
}

// Attempt to parse as JSON — non-JSON content is not an error
var parsed map[string]any
if err := json.Unmarshal([]byte(resp.Content), &parsed); err == nil {
    output.Parsed = parsed
}

return output, nil
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/agent/agent.go internal/agent/runner.go internal/agent/runner_test.go
git commit -m "feat(agent): add Parsed JSON field to AgentOutput"
```

---

### Task 7: Real Agent Prompts — Strategist and Researcher

Replace mock agents with real system prompts that produce structured JSON output.

**Files:**
- Create: `internal/agent/strategist.go`
- Create: `internal/agent/researcher.go`
- Create: `internal/agent/strategist_test.go`
- Create: `internal/agent/researcher_test.go`

**Step 1: Write tests for Strategist**

Create `internal/agent/strategist_test.go`:

```go
package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrategistRole(t *testing.T) {
	s := NewStrategist()
	assert.Equal(t, RoleStrategist, s.Role())
}

func TestStrategistBuildMessages(t *testing.T) {
	s := NewStrategist()
	tk := task.New("Build a Python script that fetches weather data", "user-1")

	msgs := s.BuildMessages(tk, "Build a Python script that fetches weather data")

	// Should have system + user messages
	require.GreaterOrEqual(t, len(msgs), 2)

	// First message should be system prompt
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Strategist")
	assert.Contains(t, msgs[0].Content, "JSON")

	// Last message should be user message with the task
	last := msgs[len(msgs)-1]
	assert.Equal(t, "user", last.Role)
	assert.Contains(t, last.Content, "weather data")
}

func TestStrategistSystemPromptRequiresJSON(t *testing.T) {
	s := NewStrategist()
	tk := task.New("test", "user")

	msgs := s.BuildMessages(tk, "test")

	system := msgs[0].Content
	// Must instruct JSON output format
	assert.Contains(t, system, "requirements")
	assert.Contains(t, system, "complexity")
	assert.Contains(t, system, "task_summary")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestStrategist -v`
Expected: FAIL — `NewStrategist` doesn't exist

**Step 3: Implement Strategist**

Create `internal/agent/strategist.go`:

```go
package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const strategistSystemPrompt = `You are the Strategist agent in the Perry secure agentic platform. Your role is to translate user intent into clear, actionable requirements.

Given a task description, produce a JSON object with these fields:
- "requirements": array of specific, testable requirements
- "cuj_ids": array of Critical User Journey IDs (e.g., "CUJ-001") — assign sequentially
- "complexity": either "standard" or "complex" — "complex" means the task likely needs multiple files, external APIs, or careful error handling
- "task_summary": a one-sentence summary of what needs to be built

Respond ONLY with the JSON object. No markdown fencing, no explanatory text.`

// Strategist translates user intent into structured requirements.
type Strategist struct{}

// NewStrategist creates a Strategist agent.
func NewStrategist() *Strategist {
	return &Strategist{}
}

func (s *Strategist) Role() Role { return RoleStrategist }

func (s *Strategist) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: strategistSystemPrompt},
		{Role: "user", Content: input},
	}
}
```

**Step 4: Run Strategist tests**

Run: `go test ./internal/agent/ -run TestStrategist -v`
Expected: ALL PASS

**Step 5: Write tests for Researcher**

Create `internal/agent/researcher_test.go`:

```go
package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResearcherRole(t *testing.T) {
	r := NewResearcher()
	assert.Equal(t, RoleResearcher, r.Role())
}

func TestResearcherBuildMessages(t *testing.T) {
	r := NewResearcher()
	tk := task.New("fetch weather data", "user-1")

	// Input is the strategist's requirements JSON
	input := `{"requirements": ["fetch weather API"], "complexity": "standard", "task_summary": "weather fetcher"}`
	msgs := r.BuildMessages(tk, input)

	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Researcher")
	assert.Contains(t, msgs[0].Content, "Context Packet")
	assert.Contains(t, msgs[0].Content, "JSON")

	last := msgs[len(msgs)-1]
	assert.Equal(t, "user", last.Role)
	assert.Contains(t, last.Content, "weather")
}

func TestResearcherSystemPromptDefinesPacketSchema(t *testing.T) {
	r := NewResearcher()
	tk := task.New("test", "user")
	msgs := r.BuildMessages(tk, "{}")

	system := msgs[0].Content
	assert.Contains(t, system, "packet_meta")
	assert.Contains(t, system, "external_apis")
	assert.Contains(t, system, "constraints")
}
```

**Step 6: Implement Researcher**

Create `internal/agent/researcher.go`:

```go
package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const researcherSystemPrompt = `You are the Researcher agent in the Perry secure agentic platform. Your role is to gather external context and produce a structured Context Packet — the sole authorized channel for external information to enter the pipeline.

Given the Strategist's requirements, produce a JSON Context Packet with these fields:
- "packet_meta": {"generated_by": "researcher", "schema_version": "1.0"}
- "task_reference": {"task_summary": "...", "requirements": [...]}
- "external_apis": array of objects, each with {"name", "base_url", "auth_method", "endpoints": [{"path", "method", "purpose"}]}
- "constraints": {"language": "...", "min_python_version": "...", "allowed_dependencies": [...], "prohibited_patterns": [...]}
- "researcher_notes": {"summary": "...", "open_questions": [...], "recommendations": [...]}

The external_apis, constraints, and researcher_notes fields provide the Coder with everything it needs to generate working code without network access.

Respond ONLY with the JSON object. No markdown fencing, no explanatory text.`

// Researcher gathers context and produces a Context Packet.
type Researcher struct{}

// NewResearcher creates a Researcher agent.
func NewResearcher() *Researcher {
	return &Researcher{}
}

func (r *Researcher) Role() Role { return RoleResearcher }

func (r *Researcher) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: researcherSystemPrompt},
		{Role: "user", Content: input},
	}
}
```

**Step 7: Run all agent tests**

Run: `go test ./internal/agent/ -v`
Expected: ALL PASS

**Step 8: Commit**

```bash
git add internal/agent/strategist.go internal/agent/strategist_test.go internal/agent/researcher.go internal/agent/researcher_test.go
git commit -m "feat(agent): add Strategist and Researcher agents with real prompts"
```

---

### Task 8: Real Agent Prompts — Coder and Auditor

**Files:**
- Create: `internal/agent/coder.go`
- Create: `internal/agent/auditor.go`
- Create: `internal/agent/coder_test.go`
- Create: `internal/agent/auditor_test.go`

**Step 1: Write tests for Coder**

Create `internal/agent/coder_test.go`:

```go
package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCoderRole(t *testing.T) {
	c := NewCoder()
	assert.Equal(t, RoleCoder, c.Role())
}

func TestCoderBuildMessages(t *testing.T) {
	c := NewCoder()
	tk := task.New("build weather fetcher", "user-1")

	// Input is the Context Packet JSON
	input := `{"task_reference": {"task_summary": "weather fetcher"}, "external_apis": [], "constraints": {"language": "python"}}`
	msgs := c.BuildMessages(tk, input)

	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Coder")
	assert.Contains(t, msgs[0].Content, "files")
	assert.Contains(t, msgs[0].Content, "JSON")

	last := msgs[len(msgs)-1]
	assert.Equal(t, "user", last.Role)
	assert.Contains(t, last.Content, "weather")
}

func TestCoderSystemPromptDefinesOutputFormat(t *testing.T) {
	c := NewCoder()
	tk := task.New("test", "user")
	msgs := c.BuildMessages(tk, "{}")

	system := msgs[0].Content
	assert.Contains(t, system, "path")
	assert.Contains(t, system, "content")
	assert.Contains(t, system, "dependencies")
	assert.Contains(t, system, "explanation")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestCoder -v`
Expected: FAIL — `NewCoder` doesn't exist

**Step 3: Implement Coder**

Create `internal/agent/coder.go`:

```go
package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const coderSystemPrompt = `You are the Coder agent in the Perry secure agentic platform. You generate code from a Context Packet. You operate in a network-isolated environment — you cannot reach the internet. Everything you need is in the Context Packet.

Given a Context Packet, produce a JSON object with these fields:
- "files": array of objects, each with {"path": "relative/path.py", "content": "full file content"}
- "dependencies": array of package requirements (e.g., "requests>=2.28")
- "explanation": a brief explanation of the implementation approach

Rules:
- Generate complete, runnable code — no placeholders or TODOs
- Only use dependencies listed in the Context Packet's constraints.allowed_dependencies
- Follow the language and version specified in constraints
- Include error handling for external API calls
- Each file must be self-contained or properly import from other generated files

Respond ONLY with the JSON object. No markdown fencing, no explanatory text.`

// Coder generates code from a Context Packet.
type Coder struct{}

// NewCoder creates a Coder agent.
func NewCoder() *Coder {
	return &Coder{}
}

func (c *Coder) Role() Role { return RoleCoder }

func (c *Coder) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: coderSystemPrompt},
		{Role: "user", Content: input},
	}
}
```

**Step 4: Run Coder tests**

Run: `go test ./internal/agent/ -run TestCoder -v`
Expected: ALL PASS

**Step 5: Write tests for Auditor**

Create `internal/agent/auditor_test.go`:

```go
package agent

import (
	"testing"

	"github.com/bnaylor/perry/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditorRole(t *testing.T) {
	a := NewAuditor()
	assert.Equal(t, RoleAuditor, a.Role())
}

func TestAuditorBuildMessages(t *testing.T) {
	a := NewAuditor()
	tk := task.New("test", "user-1")

	input := `{"files": [{"path": "main.py", "content": "print('hello')"}]}`
	msgs := a.BuildMessages(tk, input)

	require.GreaterOrEqual(t, len(msgs), 2)
	assert.Equal(t, "system", msgs[0].Role)
	assert.Contains(t, msgs[0].Content, "Auditor")
	assert.Contains(t, msgs[0].Content, "verdict")
	assert.Contains(t, msgs[0].Content, "APPROVE")
	assert.Contains(t, msgs[0].Content, "REJECT")
	assert.Contains(t, msgs[0].Content, "ESCALATE")
}

func TestAuditorSystemPromptDefinesOutputFormat(t *testing.T) {
	a := NewAuditor()
	tk := task.New("test", "user")
	msgs := a.BuildMessages(tk, "{}")

	system := msgs[0].Content
	assert.Contains(t, system, "intent_alignment")
	assert.Contains(t, system, "security_review")
	assert.Contains(t, system, "findings")
}
```

**Step 6: Implement Auditor**

Create `internal/agent/auditor.go`:

```go
package agent

import (
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/task"
)

const auditorSystemPrompt = `You are the Auditor agent (semantic review layer) in the Perry secure agentic platform. You perform the LLM-powered stage of the audit pipeline, after deterministic gates (AST analysis, secrets scanning) have already passed.

Given the generated code and the original requirements, produce a JSON verdict:
- "verdict": one of "APPROVE", "REJECT", or "ESCALATE"
- "intent_alignment": {"pass": true/false, "notes": "does the code fulfill the stated requirements?"}
- "security_review": {"pass": true/false, "notes": "any security concerns?"}
- "findings": array of issue strings (empty if clean)

Rules:
- APPROVE: code fulfills requirements, no security issues
- REJECT: code has clear bugs, missing requirements, or security vulnerabilities
- ESCALATE: you're uncertain about a finding — defer to human review
- When in doubt, ESCALATE. Never APPROVE uncertain code.
- Focus on: intent alignment, dependency safety, data handling, error paths

Respond ONLY with the JSON object. No markdown fencing, no explanatory text.`

// Auditor performs semantic code review.
type Auditor struct{}

// NewAuditor creates an Auditor agent.
func NewAuditor() *Auditor {
	return &Auditor{}
}

func (a *Auditor) Role() Role { return RoleAuditor }

func (a *Auditor) BuildMessages(tk *task.Task, input string) []llm.Message {
	return []llm.Message{
		{Role: "system", Content: auditorSystemPrompt},
		{Role: "user", Content: input},
	}
}
```

**Step 7: Run all tests**

Run: `go test ./internal/agent/ -v`
Expected: ALL PASS

**Step 8: Commit**

```bash
git add internal/agent/coder.go internal/agent/coder_test.go internal/agent/auditor.go internal/agent/auditor_test.go
git commit -m "feat(agent): add Coder and Auditor agents with real prompts"
```

---

## Batch 3: Config + Wiring (Tasks 9–12)

### Task 9: YAML Config Loading

Create a config loader that reads YAML files into the existing dispatch and policy config structs.

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRoutingConfig(t *testing.T) {
	content := `
defaults:
  strategist:
    tier: cloud
    provider: anthropic
    model: claude-sonnet-4-6
  coder:
    tier: local
    provider: ollama
    model: qwen2.5-coder:32b
escalation:
  max_local_attempts: 3
  promote_to:
    tier: cloud
    provider: anthropic
    model: claude-sonnet-4-6
`
	path := writeTemp(t, "routing.yaml", content)
	cfg, err := LoadRoutingConfig(path)
	require.NoError(t, err)

	assert.Equal(t, "cloud", cfg.Defaults["strategist"].Tier)
	assert.Equal(t, "anthropic", cfg.Defaults["strategist"].Provider)
	assert.Equal(t, "claude-sonnet-4-6", cfg.Defaults["strategist"].Model)
	assert.Equal(t, "local", cfg.Defaults["coder"].Tier)
	assert.Equal(t, 3, cfg.Escalation.MaxLocalAttempts)
	assert.Equal(t, "anthropic", cfg.Escalation.PromoteTo.Provider)
}

func TestLoadPolicyConfig(t *testing.T) {
	content := `
allowed_dependencies:
  - json
  - requests
prohibited_imports:
  - eval
  - exec
max_tokens_per_task: 500000
max_cost_per_task: 10.00
`
	path := writeTemp(t, "policy.yaml", content)
	cfg, err := LoadPolicyConfig(path)
	require.NoError(t, err)

	assert.Contains(t, cfg.AllowedDependencies, "requests")
	assert.Contains(t, cfg.ProhibitedImports, "eval")
	assert.Equal(t, 500000, cfg.MaxTokensPerTask)
	assert.Equal(t, 10.0, cfg.MaxCostPerTask)
}

func TestLoadRoutingConfigFileNotFound(t *testing.T) {
	_, err := LoadRoutingConfig("/nonexistent/file.yaml")
	assert.Error(t, err)
}

func TestLoadPolicyConfigFileNotFound(t *testing.T) {
	_, err := LoadPolicyConfig("/nonexistent/file.yaml")
	assert.Error(t, err)
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — package doesn't exist

**Step 3: Implement config loader**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"

	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/policy"
	"gopkg.in/yaml.v3"
)

// LoadRoutingConfig reads a YAML file into dispatch.Config.
func LoadRoutingConfig(path string) (dispatch.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return dispatch.Config{}, fmt.Errorf("read routing config: %w", err)
	}

	var cfg dispatch.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return dispatch.Config{}, fmt.Errorf("parse routing config: %w", err)
	}

	return cfg, nil
}

// LoadPolicyConfig reads a YAML file into policy.Config.
func LoadPolicyConfig(path string) (policy.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return policy.Config{}, fmt.Errorf("read policy config: %w", err)
	}

	var cfg policy.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return policy.Config{}, fmt.Errorf("parse policy config: %w", err)
	}

	return cfg, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add YAML config loading for routing and policy"
```

---

### Task 10: Provider Map Builder

Create a helper that builds the provider map from environment variables, used by main.go.

**Files:**
- Create: `internal/llm/providers.go`
- Create: `internal/llm/providers_test.go`

**Step 1: Write the failing test**

Create `internal/llm/providers_test.go`:

```go
package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildProviderMap(t *testing.T) {
	providers := BuildProviderMap(ProviderConfig{
		AnthropicKey: "test-anthropic-key",
		GeminiKey:    "test-gemini-key",
		OllamaURL:    "http://localhost:11434",
	})

	assert.Contains(t, providers, "anthropic")
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "ollama")
	assert.Contains(t, providers, "mock")

	assert.Equal(t, "anthropic", providers["anthropic"].Name())
	assert.Equal(t, "google", providers["google"].Name())
	assert.Equal(t, "ollama", providers["ollama"].Name())
	assert.Equal(t, "mock", providers["mock"].Name())
}

func TestBuildProviderMapDefaults(t *testing.T) {
	providers := BuildProviderMap(ProviderConfig{})

	// Should still have all providers (empty keys are accepted — they'll error at call time)
	assert.Contains(t, providers, "anthropic")
	assert.Contains(t, providers, "google")
	assert.Contains(t, providers, "ollama")
	assert.Contains(t, providers, "mock")
}

func TestBuildProviderMapCustomOllamaURL(t *testing.T) {
	providers := BuildProviderMap(ProviderConfig{
		OllamaURL: "http://gpu-box:11434",
	})
	assert.Contains(t, providers, "ollama")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/llm/ -run TestBuildProviderMap -v`
Expected: FAIL — `BuildProviderMap` doesn't exist

**Step 3: Implement the builder**

Create `internal/llm/providers.go`:

```go
package llm

import (
	"github.com/bnaylor/perry/internal/llm/anthropic"
	"github.com/bnaylor/perry/internal/llm/gemini"
	"github.com/bnaylor/perry/internal/llm/ollama"
)

const defaultOllamaURL = "http://localhost:11434"

// ProviderConfig holds the credentials needed to build the provider map.
type ProviderConfig struct {
	AnthropicKey string
	GeminiKey    string
	OllamaURL    string
}

// BuildProviderMap creates a map of all available providers.
func BuildProviderMap(cfg ProviderConfig) map[string]Provider {
	ollamaURL := cfg.OllamaURL
	if ollamaURL == "" {
		ollamaURL = defaultOllamaURL
	}

	return map[string]Provider{
		"anthropic": anthropic.New(cfg.AnthropicKey),
		"google":    gemini.New(cfg.GeminiKey),
		"ollama":    ollama.New(ollamaURL),
		"mock":      NewMockProvider("mock", "mock response"),
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/llm/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/llm/providers.go internal/llm/providers_test.go
git commit -m "feat(llm): add BuildProviderMap helper for provider wiring"
```

---

### Task 11: Wire Real Agents in main.go

Replace mock agents and hardcoded config with real agents, YAML config loading, and the provider map.

**Files:**
- Modify: `cmd/perry/main.go`

**Step 1: Read current main.go**

Already known from exploration above.

**Step 2: Rewrite main.go**

Replace `cmd/perry/main.go` with:

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/bnaylor/perry/internal/agent"
	"github.com/bnaylor/perry/internal/audit"
	"github.com/bnaylor/perry/internal/config"
	"github.com/bnaylor/perry/internal/dispatch"
	"github.com/bnaylor/perry/internal/executor"
	"github.com/bnaylor/perry/internal/fsm"
	"github.com/bnaylor/perry/internal/llm"
	"github.com/bnaylor/perry/internal/notary"
	"github.com/bnaylor/perry/internal/orchestrator"
	"github.com/bnaylor/perry/internal/policy"
	"github.com/bnaylor/perry/internal/task"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx := context.Background()

	// Get task description from args
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: perry <task description>\n")
		os.Exit(1)
	}
	taskDesc := os.Args[1]

	// Load config
	routingCfg, err := config.LoadRoutingConfig("configs/routing.yaml")
	if err != nil {
		slog.Error("failed to load routing config", "error", err)
		os.Exit(1)
	}
	policyCfg, err := config.LoadPolicyConfig("configs/policy.yaml")
	if err != nil {
		slog.Error("failed to load policy config", "error", err)
		os.Exit(1)
	}

	// Build provider map from environment
	providers := llm.BuildProviderMap(llm.ProviderConfig{
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		GeminiKey:    os.Getenv("GEMINI_API_KEY"),
		OllamaURL:    os.Getenv("OLLAMA_URL"),
	})

	// Register real agents
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewStrategist(),
		agent.RoleResearcher: agent.NewResearcher(),
		agent.RoleCoder:      agent.NewCoder(),
		agent.RoleAuditor:    agent.NewAuditor(),
	}

	// Build orchestrator
	fsmMachine := fsm.New()
	fsmMachine.OnTransition(func(tk *task.Task, from, to task.State) {
		fmt.Printf("  %s → %s\n", from, to)
	})

	orch := orchestrator.NewOrchestrator(orchestrator.Config{
		FSM:        fsmMachine,
		Store:      task.NewMemStore(),
		Runner:     agent.NewRunner(agents, providers),
		Dispatcher: dispatch.New(routingCfg),
		Policy:     policy.NewEngine(policyCfg),
		Audit:      audit.NewPipeline(), // deterministic gates wired in Phase 3
		Executor:   executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:     notary.NewMockNotary(true),
	})

	// Submit and run
	fmt.Println("perry: secure agentic platform")
	fmt.Println("================================")
	fmt.Printf("Task: %s\n\n", taskDesc)

	tk, err := orch.Submit(ctx, taskDesc, "cli-user")
	if err != nil {
		slog.Error("failed to submit task", "error", err)
		os.Exit(1)
	}
	fmt.Printf("Task %s submitted\n\n", tk.ID)
	fmt.Println("Running through state machine:")

	for tk.State != task.StateCompleted && tk.State != task.StateFailed && tk.State != task.StateHumanReview {
		if err := orch.Step(ctx, tk); err != nil {
			slog.Error("step failed", "error", err, "state", tk.State)
			os.Exit(1)
		}
	}

	fmt.Printf("\nFinal state: %s (%d transitions)\n", tk.State, len(tk.History))
}
```

**Step 3: Build to verify compilation**

Run: `go build ./cmd/perry/`
Expected: Compiles successfully

**Step 4: Commit**

```bash
git add cmd/perry/main.go
git commit -m "feat(cli): wire real agents, config loading, and provider map"
```

---

### Task 12: Fix Orchestrator Tests + Full Test Suite

After the Runner signature change (Task 5) and the new wiring, existing orchestrator tests need updating.

**Files:**
- Modify: `internal/orchestrator/orchestrator_test.go`

**Step 1: Read current orchestrator tests**

Read `internal/orchestrator/orchestrator_test.go` to see existing tests.

**Step 2: Update test setup**

All calls to `agent.NewRunner(agents, provider)` need to become `agent.NewRunner(agents, map[string]llm.Provider{"mock": provider})`.

The orchestrator's `determineNextState` now calls `o.dispatcher.Route()` before `o.runner.Execute()`, so the dispatcher config in tests needs to route to `"mock"` provider.

Update the test helper to build a dispatcher that routes everything to `"mock"`:

```go
func testConfig(t *testing.T) orchestrator.Config {
	t.Helper()
	provider := llm.NewMockProvider("mock", "mock output")
	agents := map[agent.Role]agent.Agent{
		agent.RoleStrategist: agent.NewMockAgent(agent.RoleStrategist),
		agent.RoleResearcher: agent.NewMockAgent(agent.RoleResearcher),
		agent.RoleCoder:      agent.NewMockAgent(agent.RoleCoder),
		agent.RoleAuditor:    agent.NewMockAgent(agent.RoleAuditor),
	}

	return orchestrator.Config{
		FSM:   fsm.New(),
		Store: task.NewMemStore(),
		Runner: agent.NewRunner(agents, map[string]llm.Provider{
			"mock": provider,
		}),
		Dispatcher: dispatch.New(dispatch.Config{
			Defaults: map[string]dispatch.RouteConfig{
				"strategist":       {Tier: "cloud", Provider: "mock", Model: "mock"},
				"researcher":       {Tier: "cloud", Provider: "mock", Model: "mock"},
				"coder":            {Tier: "local", Provider: "mock", Model: "mock"},
				"auditor_semantic": {Tier: "cloud", Provider: "mock", Model: "mock"},
			},
			Escalation: dispatch.EscalationConfig{
				MaxLocalAttempts: 3,
				PromoteTo:        dispatch.RouteConfig{Tier: "cloud", Provider: "mock", Model: "mock"},
			},
		}),
		Policy:   policy.NewEngine(policy.Config{MaxTokensPerTask: 500000, MaxCostPerTask: 10.0}),
		Audit:    audit.NewPipeline(),
		Executor: executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
		Notary:   notary.NewMockNotary(true),
	}
}
```

**Step 3: Run the full test suite**

Run: `go test ./... -v`
Expected: ALL PASS across every package

**Step 4: Commit**

```bash
git add internal/orchestrator/orchestrator_test.go
git commit -m "fix(orchestrator): update tests for provider map and routing"
```

---

## Batch 4: Integration Tests + Polish (Tasks 13–15)

### Task 13: Integration Test Scaffolding

Create integration test files that hit real APIs when env vars are set, skipped otherwise.

**Files:**
- Create: `internal/llm/anthropic/integration_test.go`
- Create: `internal/llm/gemini/integration_test.go`
- Create: `internal/llm/ollama/integration_test.go`

**Step 1: Create Anthropic integration test**

Create `internal/llm/anthropic/integration_test.go`:

```go
package anthropic

import (
	"context"
	"os"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicIntegration(t *testing.T) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping integration test")
	}

	p := New(key)
	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "claude-haiku-4-5-20251001",
		Messages: []llm.Message{
			{Role: "user", Content: "Reply with exactly: INTEGRATION_OK"},
		},
		MaxTokens: 32,
	})

	require.NoError(t, err)
	assert.Contains(t, resp.Content, "INTEGRATION_OK")
	assert.Greater(t, resp.Usage.InputTokens, 0)
	assert.Greater(t, resp.Usage.OutputTokens, 0)
}
```

**Step 2: Create Gemini integration test**

Create `internal/llm/gemini/integration_test.go`:

```go
package gemini

import (
	"context"
	"os"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiIntegration(t *testing.T) {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		t.Skip("GEMINI_API_KEY not set — skipping integration test")
	}

	p := New(key)
	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []llm.Message{
			{Role: "user", Content: "Reply with exactly: INTEGRATION_OK"},
		},
	})

	require.NoError(t, err)
	assert.Contains(t, resp.Content, "INTEGRATION_OK")
	assert.Greater(t, resp.Usage.InputTokens, 0)
	assert.Greater(t, resp.Usage.OutputTokens, 0)
}
```

**Step 3: Create Ollama integration test**

Create `internal/llm/ollama/integration_test.go`:

```go
package ollama

import (
	"context"
	"os"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaIntegration(t *testing.T) {
	url := os.Getenv("OLLAMA_URL")
	if url == "" {
		t.Skip("OLLAMA_URL not set — skipping integration test")
	}

	model := os.Getenv("OLLAMA_TEST_MODEL")
	if model == "" {
		model = "llama3.2:1b" // small model for testing
	}

	p := New(url)
	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "user", Content: "Reply with exactly: INTEGRATION_OK"},
		},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.Content)
	t.Logf("Ollama response: %s", resp.Content)
}
```

**Step 4: Run tests (they should skip)**

Run: `go test ./internal/llm/... -v`
Expected: Integration tests SKIP, unit tests PASS

**Step 5: Commit**

```bash
git add internal/llm/anthropic/integration_test.go internal/llm/gemini/integration_test.go internal/llm/ollama/integration_test.go
git commit -m "test: add integration test scaffolding for all LLM providers"
```

---

### Task 14: JSON Extraction from Markdown-Fenced Responses

LLMs often wrap JSON in markdown code fences (```json ... ```). Add a helper that strips fences before parsing.

**Files:**
- Modify: `internal/agent/runner.go`
- Add test cases to: `internal/agent/runner_test.go`

**Step 1: Write failing tests**

Add to `internal/agent/runner_test.go`:

```go
func TestRunnerParsesMarkdownFencedJSON(t *testing.T) {
	fencedJSON := "```json\n{\"requirements\": [\"req1\"], \"complexity\": \"standard\"}\n```"
	providers := map[string]llm.Provider{
		"mock": llm.NewMockProvider("m", fencedJSON),
	}
	agents := map[Role]Agent{
		RoleStrategist: NewMockAgent(RoleStrategist),
	}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan", dispatch.Decision{
		Provider: "mock",
		Model:    "m",
	})
	require.NoError(t, err)
	require.NotNil(t, out.Parsed)
	assert.Equal(t, "standard", out.Parsed["complexity"])
}

func TestRunnerParsesGenericFencedJSON(t *testing.T) {
	fencedJSON := "```\n{\"key\": \"value\"}\n```"
	providers := map[string]llm.Provider{
		"mock": llm.NewMockProvider("m", fencedJSON),
	}
	agents := map[Role]Agent{
		RoleStrategist: NewMockAgent(RoleStrategist),
	}
	runner := NewRunner(agents, providers)
	tk := task.New("test", "user")

	out, err := runner.Execute(ctx(t), tk, RoleStrategist, "plan", dispatch.Decision{
		Provider: "mock",
		Model:    "m",
	})
	require.NoError(t, err)
	require.NotNil(t, out.Parsed)
	assert.Equal(t, "value", out.Parsed["key"])
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestRunnerParsesMarkdownFenced -v`
Expected: FAIL — `Parsed` is nil because fenced JSON doesn't parse directly

**Step 3: Add extractJSON helper**

In `internal/agent/runner.go`, add:

```go
import (
	"strings"
)

// extractJSON attempts to extract JSON from content that may be wrapped
// in markdown code fences.
func extractJSON(content string) string {
	trimmed := strings.TrimSpace(content)

	// Try stripping ```json ... ``` or ``` ... ```
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.SplitN(trimmed, "\n", 2)
		if len(lines) == 2 {
			rest := lines[1]
			if idx := strings.LastIndex(rest, "```"); idx >= 0 {
				return strings.TrimSpace(rest[:idx])
			}
		}
	}

	return trimmed
}
```

Then update the JSON parsing in `Execute`:

```go
var parsed map[string]any
cleaned := extractJSON(resp.Content)
if err := json.Unmarshal([]byte(cleaned), &parsed); err == nil {
    output.Parsed = parsed
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/agent/runner.go internal/agent/runner_test.go
git commit -m "feat(agent): extract JSON from markdown-fenced LLM responses"
```

---

### Task 15: Full Suite Green + Build Verification

Final verification that everything compiles and all tests pass.

**Step 1: Run the full test suite**

Run: `go test ./... -v -count=1`
Expected: ALL PASS

**Step 2: Build the binary**

Run: `go build -o bin/perry ./cmd/perry/`
Expected: Compiles successfully

**Step 3: Verify CLI usage message**

Run: `./bin/perry`
Expected: Prints usage message and exits with code 1:
```
Usage: perry <task description>
```

**Step 4: Run go vet**

Run: `go vet ./...`
Expected: No issues

**Step 5: Final commit (if any fixups needed)**

```bash
git add -A
git commit -m "chore: Phase 2 final cleanup and verification"
```

---

## Summary

| Batch | Tasks | What You Get |
|-------|-------|-------------|
| **1: Foundation** | 1–4 | CompletionRequest.Model, three real LLM providers with full test suites |
| **2: Runner + Agents** | 5–8 | Provider-map-aware Runner, JSON parsing, four real agent prompts |
| **3: Config + Wiring** | 9–12 | YAML config loader, provider builder, real CLI, fixed test suite |
| **4: Integration + Polish** | 13–15 | Integration test scaffolding, markdown fence handling, full green suite |

**Total: 15 tasks, ~15 commits, end-to-end real LLM pipeline.**

After this plan is complete:
```bash
./bin/perry "Build a Python script that fetches weather data and saves it as JSON"
```
will produce real LLM output at each stage (strategist → researcher → coder → auditor), printed to stdout, with structured logging of state transitions.
