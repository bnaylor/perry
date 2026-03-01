# Docker Executor Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement a Docker-based container runtime behind the existing `executor.Executor` interface, enabling Perry to run agent-generated code in ephemeral, network-isolated sandboxes.

**Architecture:** A `DockerExecutor` struct wraps the Docker Go SDK via an extracted `DockerClient` interface. The executor creates ephemeral containers with strict security constraints (no network, read-only rootfs, resource limits), injects code via tar archive, captures logs and output artifacts, then always removes the container. Fail-closed on all error paths.

**Tech Stack:** Go, Docker Go SDK (`github.com/docker/docker`), `github.com/docker/docker/client`

---

### Task 1: Add Docker SDK dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add the Docker SDK**

Run:
```bash
cd /Users/bnaylor/src/perry && go get github.com/docker/docker@latest
```

**Step 2: Tidy modules**

Run:
```bash
cd /Users/bnaylor/src/perry && go mod tidy
```

**Step 3: Verify existing tests still pass**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./...
```
Expected: All existing tests pass.

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add Docker Go SDK for container runtime"
```

---

### Task 2: Create DockerClient interface and SandboxLimits config

**Files:**
- Create: `internal/executor/docker_client.go`

This task extracts the subset of the Docker SDK we need into a testable interface, and defines the `SandboxLimits` config struct.

**Step 1: Write the failing test**

Create `internal/executor/docker_client_test.go`:

```go
//go:build !integration

package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultSandboxLimits(t *testing.T) {
	limits := DefaultSandboxLimits()
	assert.Equal(t, int64(256*1024*1024), limits.MemoryBytes)
	assert.Equal(t, int64(256), limits.PidsLimit)
	assert.Equal(t, 1.0, limits.CPUs)
	assert.Equal(t, int64(64*1024*1024), limits.TmpfsSizeBytes)
	assert.Equal(t, int64(64*1024*1024), limits.OutSizeBytes)
	assert.Equal(t, int64(32*1024*1024), limits.HomeSizeBytes)
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./internal/executor/ -run TestDefaultSandboxLimits -v
```
Expected: FAIL — `DefaultSandboxLimits` not defined.

**Step 3: Write the implementation**

Create `internal/executor/docker_client.go`:

```go
package executor

import (
	"context"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// DockerClient is the subset of the Docker SDK we use. Extracted for testability.
type DockerClient interface {
	ImagePull(ctx context.Context, refStr string, options image.PullOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerWait(ctx context.Context, containerID string, condition container.WaitCondition) (<-chan container.WaitResponse, <-chan error)
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	CopyToContainer(ctx context.Context, containerID, dstPath string, content io.Reader, options types.CopyToContainerOptions) error
	CopyFromContainer(ctx context.Context, containerID, srcPath string) (io.ReadCloser, container.PathStat, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
}

// SandboxLimits defines resource constraints for container execution.
type SandboxLimits struct {
	MemoryBytes    int64   // Max memory (bytes)
	PidsLimit      int64   // Max PIDs
	CPUs           float64 // CPU quota (1.0 = one core)
	TmpfsSizeBytes int64   // /tmp tmpfs size
	OutSizeBytes   int64   // /out tmpfs size
	HomeSizeBytes  int64   // /home/sandbox tmpfs size
}

// DefaultSandboxLimits returns secure defaults.
func DefaultSandboxLimits() SandboxLimits {
	return SandboxLimits{
		MemoryBytes:    256 * 1024 * 1024, // 256MB
		PidsLimit:      256,
		CPUs:           1.0,
		TmpfsSizeBytes: 64 * 1024 * 1024,  // 64MB
		OutSizeBytes:   64 * 1024 * 1024,  // 64MB
		HomeSizeBytes:  32 * 1024 * 1024,  // 32MB
	}
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./internal/executor/ -run TestDefaultSandboxLimits -v
```
Expected: PASS

**Step 5: Commit**

```bash
git add internal/executor/docker_client.go internal/executor/docker_client_test.go
git commit -m "feat(executor): add DockerClient interface and SandboxLimits config"
```

---

### Task 3: Create mock DockerClient for unit testing

**Files:**
- Create: `internal/executor/docker_mock_test.go`

This mock implements the `DockerClient` interface with configurable return values and call tracking. It lives in `_test.go` so it's only available in tests.

**Step 1: Write the mock**

Create `internal/executor/docker_mock_test.go`:

```go
//go:build !integration

package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// mockDockerClient implements DockerClient for unit tests.
type mockDockerClient struct {
	pullErr      error
	createResp   container.CreateResponse
	createErr    error
	startErr     error
	waitResp     container.WaitResponse
	waitErr      error
	logsData     string
	logsErr      error
	copyToErr    error
	copyFromData []byte // tar archive bytes
	copyFromErr  error
	removeErr    error

	// Call tracking
	removed      bool
	containerID  string
}

func (m *mockDockerClient) ImagePull(_ context.Context, _ string, _ image.PullOptions) (io.ReadCloser, error) {
	if m.pullErr != nil {
		return nil, m.pullErr
	}
	return io.NopCloser(bytes.NewReader(nil)), nil
}

func (m *mockDockerClient) ContainerCreate(_ context.Context, _ *container.Config, _ *container.HostConfig, _ *network.NetworkingConfig, _ *ocispec.Platform, _ string) (container.CreateResponse, error) {
	return m.createResp, m.createErr
}

func (m *mockDockerClient) ContainerStart(_ context.Context, id string, _ container.StartOptions) error {
	m.containerID = id
	return m.startErr
}

func (m *mockDockerClient) ContainerWait(_ context.Context, _ string, _ container.WaitCondition) (<-chan container.WaitResponse, <-chan error) {
	waitCh := make(chan container.WaitResponse, 1)
	errCh := make(chan error, 1)
	if m.waitErr != nil {
		errCh <- m.waitErr
	} else {
		waitCh <- m.waitResp
	}
	return waitCh, errCh
}

func (m *mockDockerClient) ContainerLogs(_ context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	return io.NopCloser(bytes.NewReader([]byte(m.logsData))), nil
}

func (m *mockDockerClient) CopyToContainer(_ context.Context, _ string, _ string, _ io.Reader, _ types.CopyToContainerOptions) error {
	return m.copyToErr
}

func (m *mockDockerClient) CopyFromContainer(_ context.Context, _ string, _ string) (io.ReadCloser, container.PathStat, error) {
	if m.copyFromErr != nil {
		return nil, container.PathStat{}, m.copyFromErr
	}
	return io.NopCloser(bytes.NewReader(m.copyFromData)), container.PathStat{}, nil
}

func (m *mockDockerClient) ContainerRemove(_ context.Context, _ string, _ container.RemoveOptions) error {
	m.removed = true
	return m.removeErr
}

// newHappyMock returns a mock configured for a successful execution.
func newHappyMock(containerID, logs string) *mockDockerClient {
	return &mockDockerClient{
		createResp: container.CreateResponse{ID: containerID},
		waitResp:   container.WaitResponse{StatusCode: 0},
		logsData:   logs,
	}
}

// newFailMock returns a mock that fails at a specific step.
func newFailMock(step string) *mockDockerClient {
	m := newHappyMock("test-id", "")
	switch step {
	case "pull":
		m.pullErr = fmt.Errorf("pull failed")
	case "create":
		m.createErr = fmt.Errorf("create failed")
	case "start":
		m.startErr = fmt.Errorf("start failed")
	case "wait":
		m.waitErr = fmt.Errorf("wait failed")
	case "logs":
		m.logsErr = fmt.Errorf("logs failed")
	case "copyTo":
		m.copyToErr = fmt.Errorf("copy to failed")
	case "copyFrom":
		m.copyFromErr = fmt.Errorf("copy from failed")
	}
	return m
}
```

**Step 2: Verify it compiles**

Run:
```bash
cd /Users/bnaylor/src/perry && go vet ./internal/executor/
```
Expected: No errors (mock is unused but that's OK in test files).

**Step 3: Commit**

```bash
git add internal/executor/docker_mock_test.go
git commit -m "test(executor): add mock DockerClient for unit testing"
```

---

### Task 4: Implement DockerExecutor.Run() — core lifecycle

**Files:**
- Create: `internal/executor/docker.go`
- Create: `internal/executor/docker_test.go`

This is the main implementation. TDD: write tests first, then implement.

**Step 1: Write the failing tests**

Create `internal/executor/docker_test.go`:

```go
//go:build !integration

package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerExecutor_UnsupportedLanguage(t *testing.T) {
	mock := newHappyMock("test-id", "")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{
		Code:     "console.log('hi')",
		Language: "ruby",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported language")
}

func TestDockerExecutor_HappyPath(t *testing.T) {
	mock := newHappyMock("abc123", "hello world\n")
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
	assert.True(t, mock.removed, "container should always be removed")
}

func TestDockerExecutor_NonZeroExit(t *testing.T) {
	mock := newHappyMock("abc123", "error output\n")
	mock.waitResp.StatusCode = 1
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	result, err := exec.Run(context.Background(), RunRequest{
		Code:     "import sys; sys.exit(1)",
		Language: "python",
	})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.ExitCode)
	assert.True(t, mock.removed)
}

func TestDockerExecutor_FailClosed_PullError(t *testing.T) {
	mock := newFailMock("pull")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{Code: "x", Language: "python"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pull")
}

func TestDockerExecutor_FailClosed_CreateError(t *testing.T) {
	mock := newFailMock("create")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{Code: "x", Language: "python"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "create")
}

func TestDockerExecutor_FailClosed_StartError(t *testing.T) {
	mock := newFailMock("start")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{Code: "x", Language: "python"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "start")
	assert.True(t, mock.removed, "container should be removed even on start failure")
}

func TestDockerExecutor_FailClosed_CopyToError(t *testing.T) {
	mock := newFailMock("copyTo")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{Code: "x", Language: "python"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "copy")
	assert.True(t, mock.removed)
}

func TestDockerExecutor_FailClosed_WaitError(t *testing.T) {
	mock := newFailMock("wait")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(context.Background(), RunRequest{Code: "x", Language: "python"})
	assert.Error(t, err)
	assert.True(t, mock.removed)
}

func TestDockerExecutor_CopyFromError_StillReturnsLogs(t *testing.T) {
	mock := newHappyMock("abc123", "some output\n")
	mock.copyFromErr = fmt.Errorf("no such directory")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	result, err := exec.Run(context.Background(), RunRequest{
		Code:     "print('hi')",
		Language: "python",
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Logs, "some output")
	assert.Empty(t, result.Output, "output should be empty when /out copy fails")
	assert.True(t, mock.removed)
}

func TestDockerExecutor_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mock := newHappyMock("abc123", "")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	_, err := exec.Run(ctx, RunRequest{Code: "x", Language: "python"})
	assert.Error(t, err)
}

func TestDockerExecutor_WithDependencies(t *testing.T) {
	mock := newHappyMock("abc123", "ok\n")
	exec := NewDockerExecutor(mock, "python:3.12-slim", DefaultSandboxLimits())

	result, err := exec.Run(context.Background(), RunRequest{
		Code:         "import requests",
		Language:     "python",
		Dependencies: []string{"requests"},
		TimeoutSec:   30,
	})
	require.NoError(t, err)
	assert.True(t, result.Success)
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./internal/executor/ -run TestDockerExecutor -v
```
Expected: FAIL — `NewDockerExecutor` not defined.

**Step 3: Write the implementation**

Create `internal/executor/docker.go`:

```go
package executor

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
)

// entrypoints maps supported languages to their container entrypoint commands.
var entrypoints = map[string][]string{
	"python": {"/bin/sh", "-c",
		"cd /workspace && if [ -f requirements.txt ]; then pip install -q --user -r requirements.txt; fi && python main.py"},
}

// fileNames maps languages to their main source file name.
var fileNames = map[string]string{
	"python": "main.py",
}

// DockerExecutor runs code in ephemeral Docker containers.
type DockerExecutor struct {
	client    DockerClient
	baseImage string
	limits    SandboxLimits
}

// NewDockerExecutor creates an executor backed by Docker.
func NewDockerExecutor(client DockerClient, baseImage string, limits SandboxLimits) *DockerExecutor {
	return &DockerExecutor{
		client:    client,
		baseImage: baseImage,
		limits:    limits,
	}
}

// Run implements Executor.Run. It creates an ephemeral container, injects the code,
// runs it, captures output, and always removes the container.
func (d *DockerExecutor) Run(ctx context.Context, req RunRequest) (Result, error) {
	// Validate language
	entrypoint, ok := entrypoints[req.Language]
	if !ok {
		return Result{}, fmt.Errorf("unsupported language: %q", req.Language)
	}

	// Apply timeout
	timeout := time.Duration(req.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Pull image
	pullReader, err := d.client.ImagePull(ctx, d.baseImage, image.PullOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("image pull failed: %w", err)
	}
	io.Copy(io.Discard, pullReader)
	pullReader.Close()

	// Build tar archive with code
	tarBuf, err := d.buildTar(req)
	if err != nil {
		return Result{}, fmt.Errorf("failed to build code archive: %w", err)
	}

	// Create container
	cpuQuota := int64(d.limits.CPUs * 100000)
	containerCfg := &container.Config{
		Image:      d.baseImage,
		Cmd:        strslice.StrSlice(entrypoint),
		WorkingDir: "/workspace",
		User:       "1000",
		Env:        d.buildEnv(req.EnvVars),
	}
	hostCfg := &container.HostConfig{
		NetworkMode: "none",
		ReadonlyRootfs: true,
		Resources: container.Resources{
			Memory:    d.limits.MemoryBytes,
			PidsLimit: &d.limits.PidsLimit,
			CPUQuota:  cpuQuota,
			CPUPeriod: 100000,
		},
		CapDrop: strslice.StrSlice{"ALL"},
		Mounts: []mount.Mount{
			{Type: mount.TypeTmpfs, Target: "/tmp", TmpfsOptions: &mount.TmpfsOptions{SizeBytes: d.limits.TmpfsSizeBytes}},
			{Type: mount.TypeTmpfs, Target: "/out", TmpfsOptions: &mount.TmpfsOptions{SizeBytes: d.limits.OutSizeBytes}},
			{Type: mount.TypeTmpfs, Target: "/home/sandbox", TmpfsOptions: &mount.TmpfsOptions{SizeBytes: d.limits.HomeSizeBytes}},
			{Type: mount.TypeTmpfs, Target: "/workspace", TmpfsOptions: &mount.TmpfsOptions{SizeBytes: d.limits.TmpfsSizeBytes}},
		},
	}

	resp, err := d.client.ContainerCreate(ctx, containerCfg, hostCfg, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		return Result{}, fmt.Errorf("container create failed: %w", err)
	}
	containerID := resp.ID

	// Always remove container
	defer func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rmCancel()
		if rmErr := d.client.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true}); rmErr != nil {
			slog.Warn("failed to remove container", "id", containerID, "error", rmErr)
		}
	}()

	// Copy code into container
	if err := d.client.CopyToContainer(ctx, containerID, "/", tarBuf, types.CopyToContainerOptions{}); err != nil {
		return Result{}, fmt.Errorf("copy code to container failed: %w", err)
	}

	// Start container
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return Result{}, fmt.Errorf("container start failed: %w", err)
	}

	// Wait for completion
	waitCh, errCh := d.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case waitResp := <-waitCh:
		statusCode = waitResp.StatusCode
	case err := <-errCh:
		return Result{}, fmt.Errorf("container wait failed: %w", err)
	case <-ctx.Done():
		return Result{}, fmt.Errorf("container execution timed out: %w", ctx.Err())
	}

	// Capture logs
	logs := d.captureLogs(ctx, containerID)

	// Copy /out contents
	outputDir := d.copyOutput(ctx, containerID)

	return Result{
		Success:  statusCode == 0,
		ExitCode: int(statusCode),
		Logs:     logs,
		Output:   outputDir,
	}, nil
}

// buildTar creates an in-memory tar archive containing the source code
// and optional requirements.txt.
func (d *DockerExecutor) buildTar(req RunRequest) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	// Add source file
	fileName := fileNames[req.Language]
	codeBytes := []byte(req.Code)
	if err := tw.WriteHeader(&tar.Header{
		Name: "workspace/" + fileName,
		Size: int64(len(codeBytes)),
		Mode: 0644,
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(codeBytes); err != nil {
		return nil, err
	}

	// Add requirements.txt if dependencies exist
	if len(req.Dependencies) > 0 {
		deps := []byte(strings.Join(req.Dependencies, "\n") + "\n")
		if err := tw.WriteHeader(&tar.Header{
			Name: "workspace/requirements.txt",
			Size: int64(len(deps)),
			Mode: 0644,
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(deps); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

// buildEnv converts a map to Docker-style KEY=VALUE env vars.
func (d *DockerExecutor) buildEnv(envVars map[string]string) []string {
	env := []string{"HOME=/home/sandbox"}
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}
	return env
}

// captureLogs reads container stdout/stderr. Best-effort; returns empty string on error.
func (d *DockerExecutor) captureLogs(ctx context.Context, containerID string) string {
	reader, err := d.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		slog.Warn("failed to read container logs", "error", err)
		return ""
	}
	defer reader.Close()
	logBytes, _ := io.ReadAll(reader)
	return string(logBytes)
}

// copyOutput extracts /out directory contents to a host temp dir.
// Returns empty string if /out doesn't exist or is empty. Best-effort.
func (d *DockerExecutor) copyOutput(ctx context.Context, containerID string) string {
	reader, _, err := d.client.CopyFromContainer(ctx, containerID, "/out")
	if err != nil {
		slog.Debug("no /out directory to copy", "error", err)
		return ""
	}
	defer reader.Close()

	tmpDir, err := os.MkdirTemp("", "perry-output-*")
	if err != nil {
		slog.Warn("failed to create temp dir for output", "error", err)
		return ""
	}

	tr := tar.NewReader(reader)
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		// Strip the leading "out/" prefix from the tar path
		name := strings.TrimPrefix(header.Name, "out/")
		if name == "" {
			continue
		}
		outPath := filepath.Join(tmpDir, name)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			continue
		}
		f, err := os.Create(outPath)
		if err != nil {
			continue
		}
		io.Copy(f, tr)
		f.Close()
	}

	return tmpDir
}
```

**Step 4: Add missing import to test file**

The test file needs `fmt` for `TestDockerExecutor_CopyFromError_StillReturnsLogs`. Add it to the import block in `docker_test.go`:

```go
import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)
```

**Step 5: Run tests to verify they pass**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./internal/executor/ -run TestDockerExecutor -v
```
Expected: All PASS.

**Step 6: Commit**

```bash
git add internal/executor/docker.go internal/executor/docker_test.go
git commit -m "feat(executor): implement DockerExecutor with fail-closed semantics"
```

---

### Task 5: Wire DockerExecutor into the CLI

**Files:**
- Modify: `cmd/perry/main.go`

Replace `MockExecutor` with `DockerExecutor` in the CLI entry point. Also fix the orchestrator's `StateExecuting` handler to pass actual coder output (not `"placeholder"`).

**Step 1: Update main.go**

In `cmd/perry/main.go`, replace the executor line:

Old:
```go
Executor:    executor.NewMockExecutor(executor.Result{Success: true, Output: "result.json", ExitCode: 0}),
```

New:
```go
Executor:    executor.NewDockerExecutor(dockerClient, "python:3.12-slim", executor.DefaultSandboxLimits()),
```

And add Docker client construction before the orchestrator block:

```go
	// Connect to Docker
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		slog.Warn("Docker not available, using mock executor", "error", err)
	}
```

Add imports:
```go
	dkclient "github.com/docker/docker/client"
```

Use a fallback pattern — if Docker isn't available, fall back to mock:

```go
	// Build executor — Docker if available, mock fallback
	var exec executor.Executor
	dockerClient, err := dkclient.NewClientWithOpts(dkclient.FromEnv, dkclient.WithAPIVersionNegotiation())
	if err != nil {
		slog.Warn("Docker not available, using mock executor", "error", err)
		exec = executor.NewMockExecutor(executor.Result{Success: true, Output: "mock-result", ExitCode: 0})
	} else {
		exec = executor.NewDockerExecutor(dockerClient, "python:3.12-slim", executor.DefaultSandboxLimits())
	}
```

Then in the orchestrator config:
```go
Executor: exec,
```

**Step 2: Verify it compiles**

Run:
```bash
cd /Users/bnaylor/src/perry && go build ./cmd/perry/
```
Expected: Compiles without error.

**Step 3: Verify existing tests pass**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./...
```
Expected: All pass (orchestrator tests still use mock).

**Step 4: Commit**

```bash
git add cmd/perry/main.go
git commit -m "feat(cli): wire DockerExecutor with mock fallback"
```

---

### Task 6: Fix orchestrator StateExecuting to use real coder output

**Files:**
- Modify: `internal/orchestrator/orchestrator.go`
- Modify: `internal/orchestrator/orchestrator_test.go` (if it exists — verify and update)

The `StateExecuting` handler currently passes `"placeholder"` to `executor.Run`. Fix it to pass the actual coder output.

**Step 1: Check for existing orchestrator tests**

Run:
```bash
ls internal/orchestrator/*_test.go
```

**Step 2: Update the StateExecuting handler**

In `internal/orchestrator/orchestrator.go`, replace the `case task.StateExecuting:` block:

Old:
```go
	case task.StateExecuting:
		result, err := o.executor.Run(ctx, executor.RunRequest{Code: "placeholder"})
```

New:
```go
	case task.StateExecuting:
		// Retrieve coder output for execution
		code := ""
		language := "python"
		var deps []string
		if coderOutput, ok := o.outputs[outputKey(tk.ID, agent.RoleCoder)]; ok {
			if codeVal, ok := coderOutput.Parsed["code"]; ok {
				if s, ok := codeVal.(string); ok {
					code = s
				}
			}
			if code == "" {
				code = coderOutput.Content
			}
			if langVal, ok := coderOutput.Parsed["language"]; ok {
				if s, ok := langVal.(string); ok {
					language = s
				}
			}
			if depsVal, ok := coderOutput.Parsed["dependencies"]; ok {
				if depsSlice, ok := depsVal.([]any); ok {
					for _, d := range depsSlice {
						if s, ok := d.(string); ok {
							deps = append(deps, s)
						}
					}
				}
			}
		}
		result, err := o.executor.Run(ctx, executor.RunRequest{
			Code:         code,
			Language:     language,
			Dependencies: deps,
			TimeoutSec:   60,
		})
```

**Step 3: Verify tests pass**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./...
```
Expected: All pass.

**Step 4: Commit**

```bash
git add internal/orchestrator/orchestrator.go
git commit -m "fix(orchestrator): pass real coder output to executor"
```

---

### Task 7: Write integration tests

**Files:**
- Create: `internal/executor/docker_integration_test.go`

These tests require a running Docker daemon and use the `//go:build integration` tag.

**Step 1: Write integration tests**

Create `internal/executor/docker_integration_test.go`:

```go
//go:build integration

package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	result, err := exec.Run(context.Background(), RunRequest{
		Code:       "import time\ntime.sleep(999)",
		Language:   "python",
		TimeoutSec: 3,
	})
	// Timeout should produce an error
	assert.Error(t, err)
	_ = result
}

func TestIntegration_NetworkIsolation(t *testing.T) {
	exec := newRealExecutor(t)
	result, err := exec.Run(context.Background(), RunRequest{
		Code: `
import urllib.request
try:
    urllib.request.urlopen('https://example.com', timeout=5)
    print('NETWORK_AVAILABLE')
except Exception as e:
    print(f'NETWORK_BLOCKED: {e}')
`,
		Language:   "python",
		TimeoutSec: 15,
	})
	require.NoError(t, err)
	assert.True(t, result.Success) // script exits 0 either way
	assert.Contains(t, result.Logs, "NETWORK_BLOCKED")
	assert.NotContains(t, result.Logs, "NETWORK_AVAILABLE")
}
```

**Step 2: Run integration tests (requires Docker)**

Run:
```bash
cd /Users/bnaylor/src/perry && go test -tags integration ./internal/executor/ -run TestIntegration -v -timeout 120s
```
Expected: All pass (may take 30-60s on first run for image pull).

**Step 3: Verify regular tests still pass (no integration tag)**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./...
```
Expected: All pass (integration tests skipped).

**Step 4: Commit**

```bash
git add internal/executor/docker_integration_test.go
git commit -m "test(executor): add Docker integration tests for sandbox lifecycle"
```

---

### Task 8: Final verification

**Step 1: Run all unit tests**

Run:
```bash
cd /Users/bnaylor/src/perry && go test ./... -v
```
Expected: All pass.

**Step 2: Run integration tests**

Run:
```bash
cd /Users/bnaylor/src/perry && go test -tags integration ./internal/executor/ -v -timeout 120s
```
Expected: All pass.

**Step 3: Build the binary**

Run:
```bash
cd /Users/bnaylor/src/perry && go build -o bin/perry/perry ./cmd/perry/
```
Expected: Compiles cleanly.

**Step 4: Verify Docker is detected**

Run:
```bash
cd /Users/bnaylor/src/perry && docker info >/dev/null 2>&1 && echo "Docker available" || echo "Docker not available"
```

**Step 5: Run `go vet` and check for issues**

Run:
```bash
cd /Users/bnaylor/src/perry && go vet ./...
```
Expected: No issues.
