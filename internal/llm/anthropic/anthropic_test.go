package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bnaylor/perry/internal/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ llm.Provider = (*Provider)(nil)

func TestAnthropicProviderName(t *testing.T) {
	p := New("key")
	assert.Equal(t, "anthropic", p.Name())
}

func TestAnthropicComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and headers.
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))

		// Parse request body.
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]any
		require.NoError(t, json.Unmarshal(body, &reqBody))

		// System message should be extracted to top-level "system" field.
		assert.Equal(t, "You are helpful.", reqBody["system"])

		// Messages array should NOT contain system messages.
		msgs, ok := reqBody["messages"].([]any)
		require.True(t, ok)
		assert.Len(t, msgs, 1)
		msg0 := msgs[0].(map[string]any)
		assert.Equal(t, "user", msg0["role"])
		assert.Equal(t, "Hello", msg0["content"])

		// Model and max_tokens passed through.
		assert.Equal(t, "claude-sonnet-4-20250514", reqBody["model"])
		assert.Equal(t, float64(1024), reqBody["max_tokens"])

		// Write response.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hi there!"},
			},
			"model": "claude-sonnet-4-20250514",
			"usage": map[string]any{
				"input_tokens":  10,
				"output_tokens": 5,
			},
		})
	}))
	defer srv.Close()

	p := New("test-key")
	p.baseURL = srv.URL

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:     "claude-sonnet-4-20250514",
		MaxTokens: 1024,
		Messages: []llm.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Hi there!", resp.Content)
	assert.Equal(t, "claude-sonnet-4-20250514", resp.Model)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
}

func TestAnthropicNoSystemMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]any
		require.NoError(t, json.Unmarshal(body, &reqBody))

		// No system field should be present.
		_, hasSystem := reqBody["system"]
		assert.False(t, hasSystem, "system field should not be present when no system message")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hello!"},
			},
			"model": "claude-sonnet-4-20250514",
			"usage": map[string]any{
				"input_tokens":  5,
				"output_tokens": 3,
			},
		})
	}))
	defer srv.Close()

	p := New("test-key")
	p.baseURL = srv.URL

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []llm.Message{
			{Role: "user", Content: "Hi"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello!", resp.Content)
}

func TestAnthropicAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`))
	}))
	defer srv.Close()

	p := New("test-key")
	p.baseURL = srv.URL

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []llm.Message{
			{Role: "user", Content: "Hi"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}
