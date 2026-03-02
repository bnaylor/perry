//go:build integration

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
		Model:     "claude-haiku-4-5-20251001",
		Messages:  []llm.Message{{Role: "user", Content: "Reply with exactly: INTEGRATION_OK"}},
		MaxTokens: 32,
	})
	require.NoError(t, err)
	assert.Contains(t, resp.Content, "INTEGRATION_OK")
	assert.Greater(t, resp.Usage.InputTokens, 0)
	assert.Greater(t, resp.Usage.OutputTokens, 0)
}
