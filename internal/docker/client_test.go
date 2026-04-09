package docker

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// compile-time interface conformance checks
var _ Client = (*DockerClient)(nil)
var _ Client = (*MockClient)(nil)

func TestNewClientDoesNotPanic(t *testing.T) {
	// NewClient may fail if Docker is not running, but it should not panic.
	// We test the constructor and Close() path.
	c, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestPingWithDocker(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer c.Close()

	if err := c.Ping(context.Background()); err != nil {
		t.Skipf("Docker daemon not responding: %v", err)
	}
}

func TestMockClientRecordsCalls(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()

	if err := m.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !m.HasCall("Ping") {
		t.Error("expected Ping call recorded")
	}

	m.AddContainer("mycontainer-id", map[string]string{
		"com.docker.compose.project": "simpledeploy-myapp",
		"com.docker.compose.service": "web",
	})

	containers, err := m.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	if len(containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(containers))
	}
	if containers[0].ID != "mycontainer-id" {
		t.Errorf("unexpected container ID: %s", containers[0].ID)
	}
}
