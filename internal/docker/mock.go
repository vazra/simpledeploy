package docker

import (
	"context"
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
