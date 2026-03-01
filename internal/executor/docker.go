package executor

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/pkg/stdcopy"
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
	var cmd []string
	if req.Language == "python" {
		ep := req.Entrypoint
		if ep == "" {
			ep = "main.py"
		}
		cmd = []string{"python3", "/workspace/" + ep}
	} else {
		cmd = entrypoints[req.Language]
	}
	if len(req.Dependencies) > 0 {
		// Install deps before running code.
		depInstall := fmt.Sprintf("pip install --user --no-cache-dir -r /workspace/requirements.txt && %s",
			strings.Join(cmd, " "))
		cmd = []string{"sh", "-c", depInstall}
	}

	// 6. Create host-side output directory. Bind-mounted into the container so
	// output files persist after the container exits (unlike tmpfs).
	outDir, err := os.MkdirTemp("", "perry-output-*")
	if err != nil {
		return Result{}, fmt.Errorf("create output dir failed: %w", err)
	}
	// Make writable by container user (uid 1000).
	if err := os.Chmod(outDir, 0777); err != nil {
		os.RemoveAll(outDir)
		return Result{}, fmt.Errorf("chmod output dir failed: %w", err)
	}

	// 7. Create container with security constraints.
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
		NetworkMode: "none",
		// NOTE: ReadonlyRootfs is intentionally omitted. CopyToContainer writes to the
		// rootfs layer before the container starts (before tmpfs mounts are applied),
		// which fails with read-only rootfs. Security is maintained via: no network,
		// all capabilities dropped, non-root user, resource limits, and tmpfs mounts.
		CapDrop: strslice.StrSlice{"ALL"},
		Resources: container.Resources{
			Memory:    d.limits.MemoryBytes,
			PidsLimit: &pidsLimit,
			CPUQuota:  cpuQuota,
			CPUPeriod: cpuPeriod,
		},
		Binds: []string{
			fmt.Sprintf("%s:/out", outDir),
		},
		Tmpfs: map[string]string{
			"/tmp":          fmt.Sprintf("size=%d,noexec", d.limits.TmpfsSizeBytes),
			"/home/sandbox": fmt.Sprintf("size=%d,noexec", d.limits.HomeSizeBytes),
			// NOTE: /workspace is NOT a tmpfs — files are copied there via CopyToContainer
			// before the container starts. A tmpfs mount would hide those files.
			// NOTE: /out uses a bind mount (not tmpfs) so output files persist after
			// the container exits.
		},
	}

	createResp, err := d.client.ContainerCreate(ctx, config, hostConfig, &network.NetworkingConfig{}, nil, "")
	if err != nil {
		os.RemoveAll(outDir)
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

	// 8. Copy tar to container.
	if err := d.client.CopyToContainer(ctx, containerID, "/", tarBuf, container.CopyToContainerOptions{}); err != nil {
		os.RemoveAll(outDir)
		return Result{}, fmt.Errorf("copy to container failed: %w", err)
	}

	// 9. Start container.
	if err := d.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		os.RemoveAll(outDir)
		return Result{}, fmt.Errorf("container start failed: %w", err)
	}

	// 10. Wait for completion.
	waitCh, errCh := d.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case waitResult := <-waitCh:
		statusCode = waitResult.StatusCode
	case waitErr := <-errCh:
		os.RemoveAll(outDir)
		return Result{}, fmt.Errorf("container wait failed: %w", waitErr)
	case <-ctx.Done():
		os.RemoveAll(outDir)
		return Result{}, fmt.Errorf("context cancelled during wait: %w", ctx.Err())
	}

	// 11. Capture logs (best-effort).
	logs := d.captureLogs(ctx, containerID)

	// 12. Check if output directory has any files.
	output := d.checkOutput(outDir)

	// 13. Return result.
	return Result{
		Success:  statusCode == 0,
		ExitCode: int(statusCode),
		Logs:     logs,
		Output:   output,
	}, nil
}

// buildTar creates a tar archive containing the code files and optional requirements.txt.
func (d *DockerExecutor) buildTar(req RunRequest) (*bytes.Buffer, error) {
	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	// Add single code file if provided (legacy API).
	if req.Code != "" {
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
	}

	// Add multi-file map.
	for name, content := range req.Files {
		if err := tw.WriteHeader(&tar.Header{
			Name: "workspace/" + name,
			Mode: 0644,
			Size: int64(len(content)),
		}); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return nil, err
		}
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
// Docker multiplexes stdout/stderr with 8-byte headers; stdcopy.StdCopy demuxes them.
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

	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, reader); err != nil {
		slog.Debug("failed to demux logs", "error", err)
		return ""
	}
	return buf.String()
}

// checkOutput returns the output directory path if it contains any files,
// or empty string (and cleans up) if no output was produced.
func (d *DockerExecutor) checkOutput(outDir string) string {
	entries, err := os.ReadDir(outDir)
	if err != nil || len(entries) == 0 {
		os.RemoveAll(outDir)
		return ""
	}
	return outDir
}
