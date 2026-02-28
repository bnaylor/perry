// internal/notary/mock_test.go
package notary

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockNotaryApproves(t *testing.T) {
	n := NewMockNotary(true)
	result, err := n.Review(context.Background(), ReviewRequest{
		ArtifactPath: "/out/result.json",
		ExpectedType: "json",
	})
	require.NoError(t, err)
	assert.True(t, result.Approved)
}

func TestMockNotaryRejects(t *testing.T) {
	n := NewMockNotary(false)
	result, err := n.Review(context.Background(), ReviewRequest{
		ArtifactPath: "/out/backdoor.sh",
		ExpectedType: "json",
	})
	require.NoError(t, err)
	assert.False(t, result.Approved)
}
