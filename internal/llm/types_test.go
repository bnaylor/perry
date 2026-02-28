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
