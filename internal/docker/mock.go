package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dockerclient "github.com/moby/moby/client"
)

// MockClient is a thread-safe in-memory Docker client for testing.
type MockClient struct {
	mu         sync.Mutex
	Calls      []string
	containers map[string]container.Summary

	// Optional overrides for test setup.
	NetworkInspectFn   func(ctx context.Context, id string) (network.Inspect, error)
	NetworkCreateFn    func(ctx context.Context, name string, opts dockerclient.NetworkCreateOptions) (network.CreateResponse, error)
	ContainerInspectFn func(ctx context.Context, id string) (container.InspectResponse, error)
}

func NewMockClient() *MockClient {
	return &MockClient{
		containers: make(map[string]container.Summary),
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

// AddContainer pre-populates the mock with a container for testing.
func (m *MockClient) AddContainer(id string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.containers[id] = container.Summary{ID: id, Labels: labels}
}

func (m *MockClient) Ping(_ context.Context) error {
	m.record("Ping")
	return nil
}

func (m *MockClient) Close() error {
	m.record("Close")
	return nil
}

func (m *MockClient) ContainerList(_ context.Context, _ dockerclient.ContainerListOptions) ([]container.Summary, error) {
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

func (m *MockClient) ContainerStats(_ context.Context, _ string) (io.ReadCloser, error) {
	m.record("ContainerStats")
	return io.NopCloser(strings.NewReader(mockStatsJSON)), nil
}

func (m *MockClient) Raw() *dockerclient.Client {
	return nil
}

func (m *MockClient) NetworkInspect(ctx context.Context, id string) (network.Inspect, error) {
	m.record(fmt.Sprintf("NetworkInspect:%s", id))
	if m.NetworkInspectFn != nil {
		return m.NetworkInspectFn(ctx, id)
	}
	return network.Inspect{}, nil
}

func (m *MockClient) NetworkCreate(ctx context.Context, name string, opts dockerclient.NetworkCreateOptions) (network.CreateResponse, error) {
	m.record(fmt.Sprintf("NetworkCreate:%s", name))
	if m.NetworkCreateFn != nil {
		return m.NetworkCreateFn(ctx, name, opts)
	}
	return network.CreateResponse{ID: name}, nil
}

func (m *MockClient) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	m.record(fmt.Sprintf("ContainerInspect:%s", id))
	if m.ContainerInspectFn != nil {
		return m.ContainerInspectFn(ctx, id)
	}
	return container.InspectResponse{}, nil
}

func (m *MockClient) ContainerLogs(_ context.Context, id string, _ dockerclient.ContainerLogsOptions) (io.ReadCloser, error) {
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
