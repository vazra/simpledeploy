package metrics

import (
	"context"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/docker"
)

func TestCollectSystem(t *testing.T) {
	c := NewCollector(docker.NewMockClient(), nil, make(chan MetricPoint, 1))

	pt, err := c.CollectSystem()
	if err != nil {
		t.Fatalf("CollectSystem: %v", err)
	}
	if pt.Tier != TierRaw {
		t.Errorf("tier = %q, want %q", pt.Tier, TierRaw)
	}
	if pt.AppID != nil {
		t.Error("system point should have nil AppID")
	}
	if pt.ContainerID != "" {
		t.Error("system point should have empty ContainerID")
	}
	if pt.MemLimit <= 0 {
		t.Errorf("MemLimit = %d, want > 0", pt.MemLimit)
	}
	if pt.MemBytes <= 0 {
		t.Errorf("MemBytes = %d, want > 0", pt.MemBytes)
	}
	if pt.Ts == 0 {
		t.Error("Ts should not be zero")
	}
}

func TestCollectContainersEmpty(t *testing.T) {
	mock := docker.NewMockClient()
	c := NewCollector(mock, nil, make(chan MetricPoint, 1))

	pts, err := c.CollectContainers(context.Background())
	if err != nil {
		t.Fatalf("CollectContainers: %v", err)
	}
	if len(pts) != 0 {
		t.Errorf("expected 0 points from empty docker, got %d", len(pts))
	}
}

func TestCollectorRun(t *testing.T) {
	mock := docker.NewMockClient()
	ch := make(chan MetricPoint, 10)
	c := NewCollector(mock, nil, ch)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go c.Run(ctx, 50*time.Millisecond)

	// wait for at least one point (system metric)
	select {
	case pt := <-ch:
		if pt.Tier != TierRaw {
			t.Errorf("tier = %q, want %q", pt.Tier, TierRaw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for metric point")
	}

	cancel()
}
