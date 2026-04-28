package configsync

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vazra/simpledeploy/internal/store"
	"gopkg.in/yaml.v3"
)

// TestUpgradeSmoke_SeedThenReload simulates an existing install upgrade:
// DB has rows, no FS sidecars yet. Seed writes files with right perms,
// is idempotent, and a hand edit reflected back via reconcile.
func TestUpgradeSmoke_SeedThenReload(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// 1 user.
	if _, err := st.CreateUser("admin", "hash$abc", "manage", "Admin", "a@b.c"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	// 1 webhook.
	wh := &store.Webhook{Name: "smoke-hook", Type: "slack", URL: "https://hooks.example.com/x"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	// 1 app.
	app := &store.App{Name: "Smoke App", Slug: "smoke-app", ComposePath: "/x/smoke-app.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	// 1 alert rule.
	rule := &store.AlertRule{
		AppID: &app.ID, Metric: "cpu", Operator: ">", Threshold: 80,
		DurationSec: 300, WebhookID: wh.ID, Enabled: true,
	}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create alert rule: %v", err)
	}
	// 1 backup config with non-empty target_config_enc.
	retDays := 14
	bc := &store.BackupConfig{
		AppID: app.ID, Strategy: "postgres", Target: "s3",
		ScheduleCron: "0 2 * * *", TargetConfigJSON: "ciphertext",
		RetentionMode: "time", RetentionCount: 0, RetentionDays: &retDays,
		VerifyUpload: true,
	}
	if err := st.CreateBackupConfig(bc); err != nil {
		t.Fatalf("create backup config: %v", err)
	}

	// system_meta should not yet have the marker.
	if _, ok, err := st.GetMeta(fsSeededKey); err != nil {
		t.Fatalf("get meta: %v", err)
	} else if ok {
		t.Fatalf("marker unexpectedly already set")
	}

	// First-boot seed.
	if err := RunFirstBootSeedIfNeeded(context.Background(), st, syncer, nil); err != nil {
		t.Fatalf("first boot: %v", err)
	}

	// Assert files + perms.
	appSidecarPath := filepath.Join(appsDir, "smoke-app", appSidecarName)
	appSecretsPath := filepath.Join(appsDir, "smoke-app", "simpledeploy.secrets.yml")
	globalPath := filepath.Join(dataDir, globalSidecar)
	globalSecretsPath := filepath.Join(dataDir, "secrets.yml")

	checkPerm := func(path string, want os.FileMode) os.FileInfo {
		t.Helper()
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Fatalf("%s perm: got %#o want %#o", path, got, want)
		}
		return fi
	}
	checkPerm(appSidecarPath, 0644)
	checkPerm(appSecretsPath, 0600)
	checkPerm(globalPath, 0644)
	checkPerm(globalSecretsPath, 0600)

	v, ok, err := st.GetMeta(fsSeededKey)
	if err != nil || !ok || v == "" {
		t.Fatalf("marker missing after seed: v=%q ok=%v err=%v", v, ok, err)
	}

	// Idempotency: capture mtime, re-seed, verify unchanged.
	stBefore, err := os.Stat(appSidecarPath)
	if err != nil {
		t.Fatalf("stat before re-seed: %v", err)
	}
	mtimeBefore := stBefore.ModTime()

	if err := RunFirstBootSeedIfNeeded(context.Background(), st, syncer, nil); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	stAfter, err := os.Stat(appSidecarPath)
	if err != nil {
		t.Fatalf("stat after re-seed: %v", err)
	}
	if !stAfter.ModTime().Equal(mtimeBefore) {
		t.Fatalf("re-seed rewrote app sidecar; mtime changed")
	}

	// Confirm the rule is in DB before edit.
	if rules, err := st.ListAlertRules(&app.ID); err != nil {
		t.Fatalf("list rules pre-edit: %v", err)
	} else if len(rules) != 1 {
		t.Fatalf("expected 1 rule pre-edit, got %d", len(rules))
	}

	// Hand-edit: parse, drop alert rules, re-marshal.
	data, err := os.ReadFile(appSidecarPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	var sc AppSidecar
	if err := yaml.Unmarshal(data, &sc); err != nil {
		t.Fatalf("unmarshal sidecar: %v", err)
	}
	sc.AlertRules = nil
	out, err := yaml.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal sidecar: %v", err)
	}
	if err := os.WriteFile(appSidecarPath, out, 0644); err != nil {
		t.Fatalf("write edited sidecar: %v", err)
	}

	// Reconcile.
	if err := syncer.ReconcileDBFromFS(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules post-edit: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules after FS edit, got %d", len(rules))
	}
}
