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

// DockerClient is the extracted subset of the Docker SDK that the executor needs.
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

// SandboxLimits defines resource constraints for sandbox containers.
type SandboxLimits struct {
	MemoryBytes    int64
	PidsLimit      int64
	CPUs           float64
	TmpfsSizeBytes int64
	OutSizeBytes   int64
	HomeSizeBytes  int64
}

// DefaultSandboxLimits returns conservative defaults for sandbox resource limits.
func DefaultSandboxLimits() SandboxLimits {
	return SandboxLimits{
		MemoryBytes:    256 * 1024 * 1024,
		PidsLimit:      256,
		CPUs:           1.0,
		TmpfsSizeBytes: 64 * 1024 * 1024,
		OutSizeBytes:   64 * 1024 * 1024,
		HomeSizeBytes:  32 * 1024 * 1024,
	}
}
