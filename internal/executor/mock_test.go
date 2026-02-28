// internal/executor/mock_test.go
package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockExecutorSuccess(t *testing.T) {
	exec := NewMockExecutor(Result{
		Success:  true,
		Output:   "result.json",
		ExitCode: 0,
		Logs:     "executed successfully",
	})

	result, err := exec.Run(context.Background(), RunRequest{Code: "print('hi')"})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.ExitCode)
}

func TestMockExecutorFailure(t *testing.T) {
	exec := NewMockExecutor(Result{
		Success:  false,
		ExitCode: 1,
		Logs:     "NameError: name 'foo' is not defined",
	})

	result, err := exec.Run(context.Background(), RunRequest{Code: "foo()"})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.ExitCode)
}
