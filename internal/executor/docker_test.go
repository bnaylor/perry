//go:build !integration

package executor

import (
	"context"
	"os"
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

func TestDockerExecutor_OutputSizeLimitExceeded(t *testing.T) {
	mock := newHappyMock("abc123", "done")
	limits := DefaultSandboxLimits()
	limits.OutSizeBytes = 1 // 1 byte limit — any real output will exceed this
	exec := NewDockerExecutor(mock, "python:3.12-slim", limits)

	// The mock client creates a container that "succeeds", but since the output
	// check happens on the real filesystem (checkOutput reads outDir), we need
	// to create a temp dir with a file that exceeds the limit.
	// Instead, test checkOutput directly.
	result, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	// With the mock, the outDir will be empty (no real container), so checkOutput
	// will clean up and return "", which is fine — the size check only fires when
	// files exist. We test the checkOutput method directly below.
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Empty(t, result.Output)
}

func TestDockerExecutor_CheckOutputEnforcesLimit(t *testing.T) {
	limits := DefaultSandboxLimits()
	limits.OutSizeBytes = 10 // 10 bytes
	exec := NewDockerExecutor(nil, "", limits)

	// Create a temp dir with a file exceeding the limit.
	outDir := t.TempDir()
	err := os.WriteFile(outDir+"/big.txt", make([]byte, 100), 0644)
	require.NoError(t, err)

	result, checkErr := exec.checkOutput(outDir)
	assert.Error(t, checkErr)
	assert.Empty(t, result)
	assert.Contains(t, checkErr.Error(), "exceeds limit")
}

func TestDockerExecutor_CheckOutputAllowsWithinLimit(t *testing.T) {
	limits := DefaultSandboxLimits()
	limits.OutSizeBytes = 1000
	exec := NewDockerExecutor(nil, "", limits)

	outDir := t.TempDir()
	err := os.WriteFile(outDir+"/small.txt", []byte("hello"), 0644)
	require.NoError(t, err)

	result, checkErr := exec.checkOutput(outDir)
	assert.NoError(t, checkErr)
	assert.Equal(t, outDir, result)
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
