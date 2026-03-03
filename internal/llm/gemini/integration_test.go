//go:build integration

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
		Model:    "gemini-3-flash",
		Messages: []llm.Message{{Role: "user", Content: "Reply with exactly: INTEGRATION_OK"}},
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Content, "INTEGRATION_OK")
	assert.Greater(t, resp.Usage.InputTokens, 0)
	assert.Greater(t, resp.Usage.OutputTokens, 0)
}
