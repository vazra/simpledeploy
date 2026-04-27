package reconciler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
	"gopkg.in/yaml.v3"

	"github.com/vazra/simpledeploy/internal/configsync"
)

// TestWatcher_AppliesSidecarEditOnChange verifies that an external write to
// {apps_dir}/{slug}/simpledeploy.yml triggers ApplyAppSidecar via the watcher.
func TestWatcher_AppliesSidecarEditOnChange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	r, _, st, appsDir := newTestEnv(t)

	// Pre-create app and webhook so name resolution works.
	writeComposeFile(t, appsDir, "edited")
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	wh := &store.Webhook{Name: "ops", Type: "slack", URL: "http://x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Watch(ctx) }()
	time.Sleep(150 * time.Millisecond)

	// Hand-write sidecar adding an alert rule.
	sc := &configsync.AppSidecar{
		Version: configsync.Version,
		App:     configsync.AppMeta{Slug: "edited", DisplayName: "Edited"},
		AlertRules: []configsync.AlertRuleEntry{{
			Metric: "cpu", Operator: ">", Threshold: 0.9, DurationSec: 60,
			Webhook: "ops", Enabled: true,
		}},
	}
	data, err := yaml.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	sidecarPath := filepath.Join(appsDir, "edited", "simpledeploy.yml")
	if err := os.WriteFile(sidecarPath, data, 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	// Wait debounce + grace.
	time.Sleep(2 * time.Second)
	cancel()
	<-done

	app, err := st.GetAppBySlug("edited")
	if err != nil {
		t.Fatalf("GetAppBySlug: %v", err)
	}
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("ListAlertRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 alert rule applied from sidecar, got %d", len(rules))
	}
	if rules[0].Metric != "cpu" || rules[0].WebhookID != wh.ID {
		t.Errorf("unexpected rule: %+v", rules[0])
	}
}

// TestWatcher_GlobalConfigEdit verifies that an external write to
// {data_dir}/config.yml triggers ApplyGlobalSidecar via the watcher.
func TestWatcher_GlobalConfigEdit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping watcher test in short mode")
	}

	r, _, st, _ := newTestEnv(t)
	dataDir := r.syncer.DataDir()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Watch(ctx) }()
	time.Sleep(150 * time.Millisecond)

	gs := &configsync.GlobalSidecar{
		Version: configsync.Version,
		Webhooks: []configsync.WebhookEntry{
			{Name: "globalhook", Type: "slack"},
		},
	}
	data, err := yaml.Marshal(gs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.yml"), data, 0644); err != nil {
		t.Fatalf("write global: %v", err)
	}

	time.Sleep(2 * time.Second)
	cancel()
	<-done

	whs, err := st.ListWebhooks()
	if err != nil {
		t.Fatalf("ListWebhooks: %v", err)
	}
	found := false
	for _, w := range whs {
		if w.Name == "globalhook" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected globalhook webhook to be applied from global sidecar; got %+v", whs)
	}
}
