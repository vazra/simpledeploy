package audit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRecorderRoundTrip(t *testing.T) {
	s := openTestStore(t)
	rec := NewRecorder(s)
	ctx := context.Background()

	// Use the composeView shape that the compose renderer expects.
	type svc struct {
		Image string `json:"image"`
	}
	type composeView struct {
		Services map[string]svc `json:"services"`
	}
	before, _ := json.Marshal(composeView{Services: map[string]svc{"web": {Image: "nginx:1.24"}}})
	after, _ := json.Marshal(composeView{Services: map[string]svc{"web": {Image: "nginx:1.25"}}})

	id, err := rec.Record(ctx, RecordReq{
		Category: "compose",
		Action:   "changed",
		Before:   before,
		After:    after,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	got, err := s.GetActivity(ctx, id)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}

	// Renderer should produce a diff mentioning the image change.
	if !strings.Contains(got.Summary, "nginx:1.24") || !strings.Contains(got.Summary, "nginx:1.25") {
		t.Errorf("summary %q missing expected image diff", got.Summary)
	}

	// "compose" is in syncEligibleCategories so sync_status should be "pending".
	if got.SyncStatus == nil || *got.SyncStatus != "pending" {
		t.Errorf("sync_status = %v, want pending", got.SyncStatus)
	}

	// actor_source defaults to "system" when no Ctx is set.
	if got.ActorSource != "system" {
		t.Errorf("actor_source = %q, want system", got.ActorSource)
	}
}

func TestRecorderNilSafe(t *testing.T) {
	var rec *Recorder
	id, err := rec.Record(context.Background(), RecordReq{Category: "auth", Action: "login_succeeded"})
	if err != nil {
		t.Fatalf("nil Recorder.Record returned error: %v", err)
	}
	if id != 0 {
		t.Fatalf("nil Recorder.Record returned non-zero id: %d", id)
	}
}
