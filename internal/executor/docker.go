package executor

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
)

// entrypoints maps language to the command used to execute code.
var entrypoints = map[string][]string{
	"python": {"python3", "/workspace/main.py"},
}

// fileNames maps language to the filename used for the code file.
var fileNames = map[string]string{
	"python": "main.py",
}

// DockerExecutor runs code inside Docker containers with strict security constraints.
type DockerExecutor struct {
	client    DockerClient
	baseImage string
	limits    SandboxLimits
}

// NewDockerExecutor creates a DockerExecutor with the given client, base image, and limits.
func NewDockerExecutor(client DockerClient, baseImage string, limits SandboxLimits) *DockerExecutor {
	return &DockerExecutor{
		client:    client,
		baseImage: baseImage,
		limits:    limits,
	}
}

// Run implements the Executor interface. It executes code in an isolated Docker container
// with fail-closed semantics: any infrastructure error aborts the run.
func (d *DockerExecutor) Run(ctx context.Context, req RunRequest) (Result, error) {
	// 1. Validate language (fail-closed).
	if _, ok := entrypoints[req.Language]; !ok {
		return Result{}, fmt.Errorf("unsupported language: %q", req.Language)
	}

	// 2. Apply timeout via context.
	timeout := 30 * time.Second
	if req.TimeoutSec > 0 {
		timeout = time.Duration(req.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 3. Pull image.
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("context cancelled before pull: %w", err)
	}
	pullReader, err := d.client.ImagePull(ctx, d.baseImage, image.PullOptions{})
	if err != nil {
		return Result{}, fmt.Errorf("image pull failed: %w", err)
	}
	// Drain and close the pull output.
	_, _ = io.Copy(io.Discard, pullReader)
	pullReader.Close()

	// 4. Build tar archive with code + optional requirements.txt.
	tarBuf, err := d.buildTar(req)
	if err != nil {
		return Result{}, fmt.Errorf("build tar failed: %w", err)
	}

	// 5. Build container command.
	cmd := entrypoints[req.Language]
	if len(req.Dependencies) > 0 {
		// Install deps before running code.
		depInstall := fmt.Sprintf("pip install --user --no-cache-dir -r /workspace/requirements.txt && %s",
			strings.Join(cmd, " "))
		cmd = []string{"sh", "-c", depInstall}
	}

	// 6. Create container with security constraints.
	pidsLimit := d.limits.PidsLimit
	cpuPeriod := int64(100000)
	cpuQuota := int64(d.limits.CPUs * float64(cpuPeriod))

	config := &container.Config{
		Image: d.baseImage,
		Cmd:   strslice.StrSlice(cmd),
		Env:   d.buildEnv(req.EnvVars),
		User:  "1000",
	}

	hostConfig := &container.HostConfig{
		NetworkMode:  "none",
		ReadonlyRootfs: true,
		CapDrop:      strslice.StrSlice{"ALL"},
		Resources: container.Resources{
			Memory:    d.limits.MemoryBytes,
			PidsLimit: &pidsLimit,
			CPUQuota:  cpuQuota,
			CPUPeriod: cpuPeriod,
		},
		Tmpfs: map[string]string{
			"/tmp":           fmt.Sprintf("size=%d,noexec", d.limits.TmpfsSizeBytes),
			"/out":           fmt.Sprintf("size=%d,noexec", d.limits.OutSizeBytes),
			"/home/sandbox":  fmt.Sprintf("size=%d,noexec", d.limits.HomeSizeBytes),
			"/workspace":     fmt.Sprintf("size=%d,noexec", d.limits.TmpfsSizeBytes),
		},
	}

	createResp, err := d.client.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		return Result{}, fmt.Errorf("container create failed: %w", err)
	}
	containerID := createResp.ID

	// Always remove container via defer with a fresh context (not the request context).
	defer func() {
		rmCtx, rmCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer rmCancel()
		if rmErr := d.client.ContainerRemove(rmCtx, containerID, container.RemoveOptions{Force: true}); rmErr != nil {
			slog.Warn("failed to remove container", "id", containerID, "error", rmErr)
		}
	}()

	// 7. Copy tar to container.
	if err := d.client.CopyToContainer(ctx, containerID, "/", tarBuf, types.CopyToContainerOptions{}); err != nil {
		return Result{}, fmt.Errorf("copy to container failed: %w", err)
	}

	// 8. Start container.
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return Result{}, fmt.Errorf("container start failed: %w", err)
	}

	// 9. Wait for completion.
	waitCh, errCh := d.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case waitResult := <-waitCh:
		statusCode = waitResult.StatusCode
	case waitErr := <-errCh:
		return Result{}, fmt.Errorf("container wait failed: %w", waitErr)
	case <-ctx.Done():
		return Result{}, fmt.Errorf("context cancelled during wait: %w", ctx.Err())
	}

	// 10. Capture logs (best-effort).
	logs := d.captureLogs(ctx, containerID)

	// 11. Copy /out contents to temp dir (best-effort).
	output := d.copyOutput(ctx, containerID)

	// 12. Return result.
	return Result{
		Success:  statusCode == 0,
		ExitCode: int(statusCode),
		Logs:     logs,
		Output:   output,
	}, nil
}

// buildTar creates a tar archive containing the code file and optional requirements.txt.
func (d *DockerExecutor) buildTar(req RunRequest) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	// Add code file.
	fileName := fileNames[req.Language]
	codeBytes := []byte(req.Code)
	if err := tw.WriteHeader(&tar.Header{
		Name: "workspace/" + fileName,
		Mode: 0644,
		Size: int64(len(codeBytes)),
	}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(codeBytes); err != nil {
		return nil, err
	}

	// Add requirements.txt if there are dependencies.
	if len(req.Dependencies) > 0 {
		reqContent := []byte(strings.Join(req.Dependencies, "\n") + "\n")
		if err := tw.WriteHeader(&tar.Header{
			Name: "workspace/requirements.txt",
			Mode: 0644,
			Size: int64(len(reqContent)),
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(reqContent); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

// buildEnv converts env vars to KEY=VALUE format, always including HOME=/home/sandbox.
func (d *DockerExecutor) buildEnv(envVars map[string]string) []string {
	env := []string{"HOME=/home/sandbox"}
	for k, v := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

// captureLogs attempts to retrieve container logs. Returns empty string on failure.
func (d *DockerExecutor) captureLogs(ctx context.Context, containerID string) string {
	reader, err := d.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		slog.Debug("failed to capture logs", "error", err)
		return ""
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		slog.Debug("failed to read logs", "error", err)
		return ""
	}
	return string(data)
}

// copyOutput attempts to copy /out from the container. Returns path to temp dir or empty string.
func (d *DockerExecutor) copyOutput(ctx context.Context, containerID string) string {
	reader, _, err := d.client.CopyFromContainer(ctx, containerID, "/out")
	if err != nil {
		slog.Debug("failed to copy output", "error", err)
		return ""
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		slog.Debug("failed to read output", "error", err)
		return ""
	}
	if len(data) == 0 {
		return ""
	}

	// For now, return a marker that output was captured. In a future phase,
	// this will write to a temp directory and return the path.
	return string(data)
}
