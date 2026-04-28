package audit

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/vazra/simpledeploy/internal/events"
	"github.com/vazra/simpledeploy/internal/store"
)

type fakeBus struct {
	mu     sync.Mutex
	events []events.Event
}

func (f *fakeBus) Publish(_ context.Context, e events.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
}

func (f *fakeBus) topics() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.events))
	for i, e := range f.events {
		out[i] = e.Topic
	}
	sort.Strings(out)
	return out
}

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

	if !strings.Contains(got.Summary, "nginx:1.24") || !strings.Contains(got.Summary, "nginx:1.25") {
		t.Errorf("summary %q missing expected image diff", got.Summary)
	}

	if got.SyncStatus == nil || *got.SyncStatus != "pending" {
		t.Errorf("sync_status = %v, want pending", got.SyncStatus)
	}

	if got.ActorSource != "system" {
		t.Errorf("actor_source = %q, want system", got.ActorSource)
	}
}

func TestRecorderBusEmission(t *testing.T) {
	s := openTestStore(t)
	rec := NewRecorder(s)
	bus := &fakeBus{}
	rec.SetBus(bus)
	ctx := context.Background()

	cases := []struct {
		category, slug string
		want           []string
	}{
		{"compose", "foo", []string{"app:foo", "global:audit"}},
		{"backup", "foo", []string{"app:foo", "global:audit", "global:backups"}},
		{"webhook", "", []string{"global:alerts", "global:audit"}},
		{"settings", "", []string{"global:audit", "global:settings"}},
	}
	for _, c := range cases {
		bus.events = nil
		_, err := rec.Record(ctx, RecordReq{Category: c.category, AppSlug: c.slug, Action: "x"})
		if err != nil {
			t.Fatalf("Record %s: %v", c.category, err)
		}
		got := bus.topics()
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v, want %v", c.category, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("%s: got %v, want %v", c.category, got, c.want)
			}
		}
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
