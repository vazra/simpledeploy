package configsync

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

// TestFSAuth_AlertRuleEndToEnd verifies the eventual-consistency contract:
// a DB mutation routed through the store mutation hook eventually lands in
// the on-disk app sidecar via the debounced ScheduleAppWrite path.
func TestFSAuth_AlertRuleEndToEnd(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)
	t.Cleanup(func() { _ = syncer.Close() })

	// Wire the same mutation hook that cmd/simpledeploy/main.go installs.
	st.SetMutationHook(func(scope store.MutationScope, slug string) {
		switch scope {
		case store.ScopeApp:
			if slug != "" {
				syncer.ScheduleAppWrite(slug)
			}
		case store.ScopeGlobal:
			syncer.ScheduleGlobalWrite()
		}
	})

	// Webhook (global) + app, then create an alert rule on the app.
	wh := &store.Webhook{Name: "ops-slack", Type: "slack", URL: "https://hooks.example.com/x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	app := &store.App{Name: "FS App", Slug: "fsapp", ComposePath: "/apps/fsapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	rule := &store.AlertRule{
		AppID: &app.ID, Metric: "cpu", Operator: ">", Threshold: 75,
		DurationSec: 120, WebhookID: wh.ID, Enabled: true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create alert rule: %v", err)
	}

	// Poll for the debounced write (debounceDelay=500ms). Allow up to 5s.
	sidecarPath := filepath.Join(appsDir, "fsapp", appSidecarName)
	deadline := time.Now().Add(5 * time.Second)
	var got *AppSidecar
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sidecarPath); err == nil {
			data, err := syncer.ReadAppSidecar("fsapp")
			if err == nil && data != nil && len(data.AlertRules) > 0 {
				got = data
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got == nil {
		t.Fatalf("sidecar at %s never received alert rule within 5s", sidecarPath)
	}

	if len(got.AlertRules) != 1 {
		t.Fatalf("want 1 alert rule, got %d", len(got.AlertRules))
	}
	r := got.AlertRules[0]
	if r.Metric != "cpu" || r.Operator != ">" || r.Threshold != 75 || r.DurationSec != 120 || r.Webhook != "ops-slack" || !r.Enabled {
		t.Fatalf("unexpected alert rule on disk: %+v", r)
	}
	if got.App.Slug != "fsapp" {
		t.Fatalf("unexpected sidecar app slug: %q", got.App.Slug)
	}
}
