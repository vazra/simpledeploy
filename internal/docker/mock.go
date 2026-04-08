package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
)

// MockClient is a thread-safe in-memory Docker client for testing.
type MockClient struct {
	mu         sync.Mutex
	Calls      []string
	containers map[string]container.Summary
	networks   map[string]bool

	PullErr   error
	CreateErr error
	StartErr  error
	StopErr   error
	RemoveErr error
}

func NewMockClient() *MockClient {
	return &MockClient{
		containers: make(map[string]container.Summary),
		networks:   make(map[string]bool),
	}
}

func (m *MockClient) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, call)
}

// HasCall returns true if any recorded call has the given prefix.
func (m *MockClient) HasCall(prefix string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.Calls {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (m *MockClient) Ping(_ context.Context) error {
	m.record("Ping")
	return nil
}

func (m *MockClient) Close() error {
	m.record("Close")
	return nil
}

func (m *MockClient) NetworkCreate(_ context.Context, name string, _ network.CreateOptions) (network.CreateResponse, error) {
	m.record(fmt.Sprintf("NetworkCreate:%s", name))
	m.mu.Lock()
	m.networks[name] = true
	m.mu.Unlock()
	return network.CreateResponse{ID: name}, nil
}

func (m *MockClient) NetworkRemove(_ context.Context, name string) error {
	m.record(fmt.Sprintf("NetworkRemove:%s", name))
	m.mu.Lock()
	delete(m.networks, name)
	m.mu.Unlock()
	return nil
}

func (m *MockClient) ImagePull(_ context.Context, ref string, _ image.PullOptions) (io.ReadCloser, error) {
	m.record(fmt.Sprintf("ImagePull:%s", ref))
	if m.PullErr != nil {
		return nil, m.PullErr
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *MockClient) ContainerCreate(_ context.Context, config *container.Config, _ *container.HostConfig, _ *network.NetworkingConfig, name string) (container.CreateResponse, error) {
	m.record(fmt.Sprintf("ContainerCreate:%s", name))
	if m.CreateErr != nil {
		return container.CreateResponse{}, m.CreateErr
	}
	id := name + "-id"
	m.mu.Lock()
	m.containers[id] = container.Summary{ID: id, Names: []string{"/" + name}, Image: config.Image}
	m.mu.Unlock()
	return container.CreateResponse{ID: id}, nil
}

func (m *MockClient) ContainerStart(_ context.Context, id string, _ container.StartOptions) error {
	m.record(fmt.Sprintf("ContainerStart:%s", id))
	return m.StartErr
}

func (m *MockClient) ContainerStop(_ context.Context, id string, _ container.StopOptions) error {
	m.record(fmt.Sprintf("ContainerStop:%s", id))
	return m.StopErr
}

func (m *MockClient) ContainerRemove(_ context.Context, id string, _ container.RemoveOptions) error {
	m.record(fmt.Sprintf("ContainerRemove:%s", id))
	if m.RemoveErr != nil {
		return m.RemoveErr
	}
	m.mu.Lock()
	delete(m.containers, id)
	m.mu.Unlock()
	return nil
}

func (m *MockClient) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	m.record("ContainerList")
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]container.Summary, 0, len(m.containers))
	for _, c := range m.containers {
		out = append(out, c)
	}
	return out, nil
}

const mockStatsJSON = `{
	"cpu_stats":{"cpu_usage":{"total_usage":200000000},"system_cpu_usage":2000000000,"online_cpus":2},
	"precpu_stats":{"cpu_usage":{"total_usage":100000000},"system_cpu_usage":1000000000},
	"memory_stats":{"usage":52428800,"limit":1073741824},
	"networks":{"eth0":{"rx_bytes":1024,"tx_bytes":2048}},
	"blkio_stats":{"io_service_bytes_recursive":[{"op":"Read","value":4096},{"op":"Write","value":8192}]}
}`

func (m *MockClient) ContainerStats(_ context.Context, _ string) (container.StatsResponseReader, error) {
	m.record("ContainerStats")
	return container.StatsResponseReader{
		Body:   io.NopCloser(strings.NewReader(mockStatsJSON)),
		OSType: "linux",
	}, nil
}

func (m *MockClient) ContainerLogs(_ context.Context, id string, _ container.LogsOptions) (io.ReadCloser, error) {
	m.record(fmt.Sprintf("ContainerLogs:%s", id))
	var buf bytes.Buffer
	line := []byte("2026-04-08T12:00:00Z test log line\n")
	header := make([]byte, 8)
	header[0] = 1 // stdout
	binary.BigEndian.PutUint32(header[4:], uint32(len(line)))
	buf.Write(header)
	buf.Write(line)
	return io.NopCloser(&buf), nil
}
