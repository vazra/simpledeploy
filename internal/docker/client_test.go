package docker

import (
	"context"
	"testing"
)

// compile-time interface conformance check
var _ Client = (*DockerClient)(nil)

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
