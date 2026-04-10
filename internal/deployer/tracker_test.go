package deployer

import (
	"context"
	"testing"
)

func TestTracker_TrackAndCancel(t *testing.T) {
	tr := NewTracker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tr.Track("myapp", cancel)
	if !tr.IsDeploying("myapp") {
		t.Fatal("expected myapp to be deploying")
	}
	if err := tr.Cancel("myapp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("expected context to be cancelled")
	}
	if tr.IsDeploying("myapp") {
		t.Fatal("expected myapp to not be deploying after cancel")
	}
}

func TestTracker_CancelNotFound(t *testing.T) {
	tr := NewTracker()
	if err := tr.Cancel("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent app")
	}
}

func TestTracker_Done(t *testing.T) {
	tr := NewTracker()
	_, cancel := context.WithCancel(context.Background())
	tr.Track("myapp", cancel)
	tr.Done("myapp")
	if tr.IsDeploying("myapp") {
		t.Fatal("expected myapp to not be deploying after done")
	}
}
