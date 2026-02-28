package ollama

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

func TestOllamaProviderName(t *testing.T) {
	p := New("http://localhost:11434")
	assert.Equal(t, "ollama", p.Name())
}

func TestOllamaComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and path.
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/chat", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Parse request body.
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]any
		require.NoError(t, json.Unmarshal(body, &reqBody))

		// Model passed through.
		assert.Equal(t, "llama3", reqBody["model"])

		// Stream must be false.
		assert.Equal(t, false, reqBody["stream"])

		// Messages passed through as-is (system + user).
		msgs, ok := reqBody["messages"].([]any)
		require.True(t, ok)
		assert.Len(t, msgs, 2)

		msg0 := msgs[0].(map[string]any)
		assert.Equal(t, "system", msg0["role"])
		assert.Equal(t, "You are helpful.", msg0["content"])

		msg1 := msgs[1].(map[string]any)
		assert.Equal(t, "user", msg1["role"])
		assert.Equal(t, "Hello", msg1["content"])

		// Temperature in options.
		opts, ok := reqBody["options"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, 0.7, opts["temperature"])

		// Write response.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"model": "llama3",
			"message": map[string]any{
				"role":    "assistant",
				"content": "Hi there!",
			},
			"prompt_eval_count": 15,
			"eval_count":        8,
		})
	}))
	defer srv.Close()

	p := New(srv.URL)

	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:       "llama3",
		Temperature: 0.7,
		Messages: []llm.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Hi there!", resp.Content)
	assert.Equal(t, "llama3", resp.Model)
	assert.Equal(t, 15, resp.Usage.InputTokens)
	assert.Equal(t, 8, resp.Usage.OutputTokens)
}

func TestOllamaConnectionError(t *testing.T) {
	p := New("http://localhost:1")

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "llama3",
		Messages: []llm.Message{
			{Role: "user", Content: "Hello"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama")
}

func TestOllamaServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer srv.Close()

	p := New(srv.URL)

	_, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model: "llama3",
		Messages: []llm.Message{
			{Role: "user", Content: "Hello"},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
