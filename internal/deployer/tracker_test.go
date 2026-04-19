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

// Regression: concurrent TrackWithLog for the same slug must not orphan the
// first subscriber. The second caller gets (nil, false) so it skips its own
// compose run, and subscribers on the first log still receive Done.
func TestTracker_TrackWithLogRejectsDuplicate(t *testing.T) {
	tr := NewTracker()
	_, cancel := context.WithCancel(context.Background())

	dl1, fresh1 := tr.TrackWithLog("app", cancel)
	if !fresh1 || dl1 == nil {
		t.Fatal("first TrackWithLog should return a fresh log")
	}
	ch, unsub := dl1.Subscribe()
	defer unsub()

	dl2, fresh2 := tr.TrackWithLog("app", cancel)
	if fresh2 {
		t.Fatal("second TrackWithLog for same slug should report busy")
	}
	if dl2 != nil {
		t.Fatal("second TrackWithLog should return nil log")
	}

	tr.DoneWithLog("app", "deploy")

	select {
	case line, ok := <-ch:
		if !ok {
			t.Fatal("subscriber channel closed without Done event")
		}
		if !line.Done || line.Action != "deploy" {
			t.Fatalf("expected Done deploy, got %+v", line)
		}
	default:
		t.Fatal("expected Done to be delivered to original subscriber")
	}
}
