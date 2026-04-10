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
	dockerclient "github.com/docker/docker/client"
)

// MockClient is a thread-safe in-memory Docker client for testing.
type MockClient struct {
	mu         sync.Mutex
	Calls      []string
	containers map[string]container.Summary
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

func (m *MockClient) Raw() *dockerclient.Client {
	return nil
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
