//go:build integration

package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRealExecutor(t *testing.T) *DockerExecutor {
	t.Helper()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err, "Docker must be available for integration tests")
	return NewDockerExecutor(cli, "python:3.12-slim", DefaultSandboxLimits())
}

func TestIntegration_HelloWorld(t *testing.T) {
	exec := newRealExecutor(t)
	result, err := exec.Run(context.Background(), RunRequest{
		Code:       "print('hello from sandbox')",
		Language:   "python",
		TimeoutSec: 30,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 0, result.ExitCode)
	assert.Contains(t, result.Logs, "hello from sandbox")
}

func TestIntegration_WriteOutput(t *testing.T) {
	exec := newRealExecutor(t)
	result, err := exec.Run(context.Background(), RunRequest{
		Code:       "with open('/out/result.txt', 'w') as f:\n    f.write('done')\nprint('wrote output')",
		Language:   "python",
		TimeoutSec: 30,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Logs, "wrote output")
	assert.NotEmpty(t, result.Output, "output dir should be set")

	// Verify the output file was copied
	data, err := os.ReadFile(filepath.Join(result.Output, "result.txt"))
	require.NoError(t, err)
	assert.Equal(t, "done", string(data))

	// Clean up
	os.RemoveAll(result.Output)
}

func TestIntegration_NonZeroExit(t *testing.T) {
	exec := newRealExecutor(t)
	result, err := exec.Run(context.Background(), RunRequest{
		Code:       "import sys\nprint('about to fail')\nsys.exit(42)",
		Language:   "python",
		TimeoutSec: 30,
	})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, 42, result.ExitCode)
	assert.Contains(t, result.Logs, "about to fail")
}

func TestIntegration_Timeout(t *testing.T) {
	exec := newRealExecutor(t)
	_, err := exec.Run(context.Background(), RunRequest{
		Code:       "import time\ntime.sleep(999)",
		Language:   "python",
		TimeoutSec: 3,
	})
	assert.Error(t, err)
}

func TestIntegration_NetworkIsolation(t *testing.T) {
	exec := newRealExecutor(t)
	result, err := exec.Run(context.Background(), RunRequest{
		Code:       "import urllib.request\ntry:\n    urllib.request.urlopen('https://example.com', timeout=5)\n    print('NETWORK_AVAILABLE')\nexcept Exception as e:\n    print(f'NETWORK_BLOCKED: {e}')",
		Language:   "python",
		TimeoutSec: 15,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Logs, "NETWORK_BLOCKED")
	assert.NotContains(t, result.Logs, "NETWORK_AVAILABLE")
}
