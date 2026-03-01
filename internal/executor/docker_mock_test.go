//go:build !integration

package executor

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// dockerStdoutFrame wraps data in a Docker multiplexed stream stdout frame.
// Docker log frames: [stream_type(1 byte), 0, 0, 0, size(4 bytes big-endian), payload].
func dockerStdoutFrame(data string) []byte {
	payload := []byte(data)
	header := make([]byte, 8)
	header[0] = 1 // stdout
	binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
	return append(header, payload...)
}

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
	copyFromData []byte
	copyFromErr  error
	removeErr    error
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
	// Return Docker multiplexed stream format (same as real Docker daemon).
	return io.NopCloser(bytes.NewReader(dockerStdoutFrame(m.logsData))), nil
}

func (m *mockDockerClient) CopyToContainer(_ context.Context, _ string, _ string, _ io.Reader, _ container.CopyToContainerOptions) error {
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

func newHappyMock(containerID, logs string) *mockDockerClient {
	return &mockDockerClient{
		createResp: container.CreateResponse{ID: containerID},
		waitResp:   container.WaitResponse{StatusCode: 0},
		logsData:   logs,
	}
}

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
