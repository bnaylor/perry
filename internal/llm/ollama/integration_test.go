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
		model = "llama3.2:1b"
	}
	p := New(url)
	resp, err := p.Complete(context.Background(), llm.CompletionRequest{
		Model:    model,
		Messages: []llm.Message{{Role: "user", Content: "Reply with exactly: INTEGRATION_OK"}},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Content)
	t.Logf("Ollama response: %s", resp.Content)
}
