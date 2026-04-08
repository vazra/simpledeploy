package docker

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
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

	if _, err := m.NetworkCreate(ctx, "testnet", network.CreateOptions{}); err != nil {
		t.Fatalf("NetworkCreate: %v", err)
	}
	if !m.HasCall("NetworkCreate:testnet") {
		t.Error("expected NetworkCreate:testnet call recorded")
	}

	resp, err := m.ContainerCreate(ctx, &container.Config{Image: "alpine"}, nil, nil, "mycontainer")
	if err != nil {
		t.Fatalf("ContainerCreate: %v", err)
	}
	if !m.HasCall("ContainerCreate:mycontainer") {
		t.Error("expected ContainerCreate:mycontainer call recorded")
	}

	if err := m.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		t.Fatalf("ContainerStart: %v", err)
	}
	if !m.HasCall("ContainerStart:" + resp.ID) {
		t.Error("expected ContainerStart call recorded")
	}

	containers, err := m.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		t.Fatalf("ContainerList: %v", err)
	}
	if len(containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(containers))
	}

	if err := m.ContainerStop(ctx, resp.ID, container.StopOptions{}); err != nil {
		t.Fatalf("ContainerStop: %v", err)
	}
	if err := m.ContainerRemove(ctx, resp.ID, container.RemoveOptions{}); err != nil {
		t.Fatalf("ContainerRemove: %v", err)
	}

	containers, _ = m.ContainerList(ctx, container.ListOptions{})
	if len(containers) != 0 {
		t.Errorf("expected 0 containers after remove, got %d", len(containers))
	}

	if err := m.NetworkRemove(ctx, "testnet"); err != nil {
		t.Fatalf("NetworkRemove: %v", err)
	}
	if !m.HasCall("NetworkRemove:testnet") {
		t.Error("expected NetworkRemove:testnet call recorded")
	}
}
