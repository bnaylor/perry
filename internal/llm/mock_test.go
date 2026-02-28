// internal/llm/mock_test.go
package llm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProvider(t *testing.T) {
	mock := NewMockProvider("test-model", "This is the response.")
	resp, err := mock.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "This is the response.", resp.Content)
	assert.Equal(t, "test-model", resp.Model)
	assert.Greater(t, resp.Usage.OutputTokens, 0)
}

func TestMockProviderName(t *testing.T) {
	mock := NewMockProvider("test-model", "resp")
	assert.Equal(t, "mock", mock.Name())
}

func TestMockProviderRecordsRequests(t *testing.T) {
	mock := NewMockProvider("test-model", "resp")
	_, _ = mock.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "hello"}},
	})
	_, _ = mock.Complete(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "world"}},
	})
	assert.Len(t, mock.Requests, 2)
	assert.Equal(t, "hello", mock.Requests[0].Messages[0].Content)
}
