package gemini

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

func TestGeminiProviderName(t *testing.T) {
	p := New("key")
	assert.Equal(t, "google", p.Name())
}

func TestGeminiComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and path.
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1beta/models/test-model:generateContent", r.URL.Path)

		// Verify API key in query string.
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))

		// Parse request body.
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		// Verify system_instruction from system message.
		sysInstr, ok := req["system_instruction"].(map[string]any)
		require.True(t, ok, "system_instruction should be present")
		parts := sysInstr["parts"].([]any)
		part := parts[0].(map[string]any)
		assert.Equal(t, "You are helpful.", part["text"])

		// Verify contents from non-system messages.
		contents := req["contents"].([]any)
		assert.Len(t, contents, 1)
		c := contents[0].(map[string]any)
		assert.Equal(t, "user", c["role"])
		cParts := c["parts"].([]any)
		assert.Equal(t, "Hello", cParts[0].(map[string]any)["text"])

		// Return mock response.
		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "Hi there!"},
						},
					},
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     10,
				"candidatesTokenCount": 5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New("test-key")
	p.baseURL = srv.URL

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "test-model",
		Messages: []llm.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Hi there!", resp.Content)
	assert.Equal(t, "test-model", resp.Model)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
}

func TestGeminiRoleMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var req map[string]any
		require.NoError(t, json.Unmarshal(body, &req))

		// Verify "assistant" is mapped to "model".
		contents := req["contents"].([]any)
		require.Len(t, contents, 2)
		assert.Equal(t, "user", contents[0].(map[string]any)["role"])
		assert.Equal(t, "model", contents[1].(map[string]any)["role"])

		resp := map[string]any{
			"candidates": []map[string]any{
				{
					"content": map[string]any{
						"parts": []map[string]any{
							{"text": "response"},
						},
					},
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     1,
				"candidatesTokenCount": 1,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := New("key")
	p.baseURL = srv.URL

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "m",
		Messages: []llm.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
		},
	})
	require.NoError(t, err)
}

func TestGeminiAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": {"message": "forbidden"}}`))
	}))
	defer srv.Close()

	p := New("bad-key")
	p.baseURL = srv.URL

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "m",
		Messages: []llm.Message{
			{Role: "user", Content: "hi"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}
