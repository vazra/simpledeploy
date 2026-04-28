package configsync

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
	"gopkg.in/yaml.v3"
)

func TestFirstBootSeed_WritesFilesAndMarker(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed: webhook, user, two apps.
	wh := &store.Webhook{Name: "slack", Type: "slack", URL: "https://hooks.example.com/x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	if _, err := st.CreateUser("admin", "hash$abc", "manage", "Admin", "a@b.c"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	a1 := &store.App{Name: "App One", Slug: "app1", ComposePath: "/x/a1.yml", Status: "running"}
	if err := st.UpsertApp(a1, nil); err != nil {
		t.Fatalf("upsert a1: %v", err)
	}
	a2 := &store.App{Name: "App Two", Slug: "app2", ComposePath: "/x/a2.yml", Status: "running"}
	if err := st.UpsertApp(a2, nil); err != nil {
		t.Fatalf("upsert a2: %v", err)
	}

	if err := RunFirstBootSeedIfNeeded(context.Background(), st, syncer, nil); err != nil {
		t.Fatalf("first boot: %v", err)
	}

	for _, slug := range []string{"app1", "app2"} {
		p := filepath.Join(appsDir, slug, appSidecarName)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected sidecar %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dataDir, globalSidecar)); err != nil {
		t.Fatalf("expected config.yml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "secrets.yml")); err != nil {
		t.Fatalf("expected secrets.yml: %v", err)
	}

	v, ok, err := st.GetMeta(fsSeededKey)
	if err != nil || !ok || v == "" {
		t.Fatalf("marker missing: v=%q ok=%v err=%v", v, ok, err)
	}

	// Idempotent: second run leaves marker unchanged.
	markerBefore := v
	configPath := filepath.Join(dataDir, globalSidecar)
	stBefore, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config before: %v", err)
	}
	mtimeBefore := stBefore.ModTime()

	if err := RunFirstBootSeedIfNeeded(context.Background(), st, syncer, nil); err != nil {
		t.Fatalf("second run: %v", err)
	}
	v2, _, _ := st.GetMeta(fsSeededKey)
	if v2 != markerBefore {
		t.Fatalf("marker changed: before=%q after=%q", markerBefore, v2)
	}
	stAfter, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat config after: %v", err)
	}
	if !stAfter.ModTime().Equal(mtimeBefore) {
		t.Fatalf("config.yml rewritten on second run; mtime changed")
	}
}

func TestReconcileDBFromFS_AppliesAlertRules(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Pre-seed webhook so name resolution works.
	wh := &store.Webhook{Name: "slack", Type: "slack", URL: "https://hooks.example.com/x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	app := &store.App{Name: "App", Slug: "myapp", ComposePath: "/x/a.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// Hand-write per-app sidecar with one alert rule.
	sidecar := AppSidecar{
		Version: Version,
		App:     AppMeta{Slug: "myapp", DisplayName: "App"},
		AlertRules: []AlertRuleEntry{
			{Metric: "cpu", Operator: ">", Threshold: 75, DurationSec: 120, Webhook: "slack", Enabled: true},
		},
	}
	dir := filepath.Join(appsDir, "myapp")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := yaml.Marshal(sidecar)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, appSidecarName), data, 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	if err := syncer.ReconcileDBFromFS(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	if rules[0].Metric != "cpu" || rules[0].Threshold != 75 || rules[0].WebhookID != wh.ID {
		t.Fatalf("rule wrong: %+v", rules[0])
	}
}

func TestReconcileDBFromFS_RehydratesArchivedFromTombstone(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	archivedAt := time.Now().UTC().Truncate(time.Second)
	tomb := &Tombstone{
		Version:    Version,
		ArchivedAt: archivedAt,
		App:        AppMeta{Slug: "old-slug", DisplayName: "Old"},
	}
	if err := syncer.writeTombstoneFile("old-slug", tomb); err != nil {
		t.Fatalf("write tombstone: %v", err)
	}

	if err := syncer.ReconcileDBFromFS(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	got, err := st.GetAppBySlug("old-slug")
	if err != nil {
		t.Fatalf("get app: %v", err)
	}
	if got.Name != "Old" {
		t.Fatalf("name: got %q want Old", got.Name)
	}
	if !got.ArchivedAt.Valid {
		t.Fatalf("expected ArchivedAt valid")
	}
}

func TestEnsureGitignore_AppendsMissingOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")

	if err := ensureGitignore(dir, []string{"*.secrets.yml"}); err != nil {
		t.Fatalf("first call: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	first := string(data)
	if !strings.Contains(first, "*.secrets.yml") || !strings.Contains(first, "# simpledeploy: never commit secrets") {
		t.Fatalf("missing block/line: %q", first)
	}

	if err := ensureGitignore(dir, []string{"*.secrets.yml"}); err != nil {
		t.Fatalf("second call: %v", err)
	}
	data2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read2: %v", err)
	}
	if string(data2) != first {
		t.Fatalf("expected idempotent; got %q vs %q", data2, first)
	}

	dir2 := t.TempDir()
	path2 := filepath.Join(dir2, ".gitignore")
	if err := os.WriteFile(path2, []byte("node_modules\n"), 0644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	if err := ensureGitignore(dir2, []string{"*.secrets.yml"}); err != nil {
		t.Fatalf("third call: %v", err)
	}
	data3, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("read3: %v", err)
	}
	got := string(data3)
	if !strings.Contains(got, "node_modules") {
		t.Fatalf("lost node_modules: %q", got)
	}
	if !strings.Contains(got, "*.secrets.yml") {
		t.Fatalf("missing appended line: %q", got)
	}
}
