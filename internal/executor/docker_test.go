//go:build !integration

package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerExecutor_UnsupportedLanguage(t *testing.T) {
	mock := newHappyMock("abc123", "hello")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{
		Code:     "puts 'hello'",
		Language: "ruby",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported language")
}

func TestDockerExecutor_HappyPath(t *testing.T) {
	mock := newHappyMock("abc123", "hello world")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	result, err := exec.Run(context.Background(), RunRequest{
		Code:       "print('hello world')",
		Language:   "python",
		TimeoutSec: 30,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Logs, "hello world")
	assert.True(t, mock.removed, "container should be removed")
}

func TestDockerExecutor_NonZeroExit(t *testing.T) {
	mock := newHappyMock("abc123", "error output")
	mock.waitResp.StatusCode = 1
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	result, err := exec.Run(context.Background(), RunRequest{
		Code:     "import sys; sys.exit(1)",
		Language: "python",
	})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.ExitCode)
	assert.True(t, mock.removed, "container should be removed")
}

func TestDockerExecutor_FailClosed_PullError(t *testing.T) {
	mock := newFailMock("pull")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pull")
}

func TestDockerExecutor_FailClosed_CreateError(t *testing.T) {
	mock := newFailMock("create")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create")
}

func TestDockerExecutor_FailClosed_StartError(t *testing.T) {
	mock := newFailMock("start")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start")
	assert.True(t, mock.removed, "container should be removed on start failure")
}

func TestDockerExecutor_FailClosed_CopyToError(t *testing.T) {
	mock := newFailMock("copyTo")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copy")
	assert.True(t, mock.removed, "container should be removed on copy failure")
}

func TestDockerExecutor_FailClosed_WaitError(t *testing.T) {
	mock := newFailMock("wait")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wait")
	assert.True(t, mock.removed, "container should be removed on wait failure")
}

func TestDockerExecutor_CopyFromError_StillReturnsLogs(t *testing.T) {
	mock := newFailMock("copyFrom")
	mock.logsData = "some output"
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	result, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('some output')",
		Language: "python",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Logs, "some output")
	assert.Empty(t, result.Output, "output should be empty when copyFrom fails")
	assert.True(t, mock.removed)
}

func TestDockerExecutor_ContextCancelled(t *testing.T) {
	mock := newHappyMock("abc123", "")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := exec.Run(ctx, RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	require.Error(t, err)
}

func TestDockerExecutor_WithDependencies(t *testing.T) {
	mock := newHappyMock("abc123", "requests installed")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	result, err := exec.Run(context.Background(), RunRequest{
		Code:         "import requests; print('requests installed')",
		Language:     "python",
		Dependencies: []string{"requests"},
		TimeoutSec:   60,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.ExitCode)
	assert.True(t, mock.removed)
}
