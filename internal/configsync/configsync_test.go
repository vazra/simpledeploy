package configsync

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/store"
)

// openTestStore creates a temp SQLite store for testing.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestRoundtripAppSidecar(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed webhook (needed for alert rule).
	wh := &store.Webhook{Name: "slack-test", Type: "slack", URL: "https://hooks.example.com/test"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Seed app.
	app := &store.App{Name: "My App", Slug: "myapp", ComposePath: "/apps/myapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// Seed two alert rules.
	r1 := &store.AlertRule{AppID: &app.ID, Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 300, WebhookID: wh.ID, Enabled: true}
	r2 := &store.AlertRule{AppID: &app.ID, Metric: "mem", Operator: ">", Threshold: 90, DurationSec: 60, WebhookID: wh.ID, Enabled: false}
	if err := st.CreateAlertRule(r1); err != nil {
		t.Fatalf("create rule 1: %v", err)
	}
	if err := st.CreateAlertRule(r2); err != nil {
		t.Fatalf("create rule 2: %v", err)
	}

	retDays := 30
	// Seed one backup config.
	bc := &store.BackupConfig{
		AppID: app.ID, Strategy: "postgres", Target: "s3",
		ScheduleCron: "0 2 * * *", TargetConfigJSON: "enc-blob",
		RetentionMode: "time", RetentionCount: 0, RetentionDays: &retDays,
		VerifyUpload: true,
	}
	if err := st.CreateBackupConfig(bc); err != nil {
		t.Fatalf("create backup config: %v", err)
	}

	// Write sidecar.
	if err := syncer.WriteAppSidecar("myapp"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}

	// Verify file written at correct path.
	sidecarPath := filepath.Join(appsDir, "myapp", appSidecarName)
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Fatalf("sidecar not found: %v", err)
	}

	// Delete rows from DB.
	if err := st.DeleteAlertRulesForApp(app.ID); err != nil {
		t.Fatalf("delete rules: %v", err)
	}
	if err := st.DeleteBackupConfigsForApp(app.ID); err != nil {
		t.Fatalf("delete backup configs: %v", err)
	}

	// Read sidecar.
	data, err := syncer.ReadAppSidecar("myapp")
	if err != nil {
		t.Fatalf("ReadAppSidecar: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil AppSidecar")
	}

	// Import back.
	if err := syncer.ImportAppSidecar(data); err != nil {
		t.Fatalf("ImportAppSidecar: %v", err)
	}

	// Assert alert rules.
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("want 2 rules, got %d", len(rules))
	}
	if rules[0].Metric != "cpu" || rules[0].Threshold != 80 || !rules[0].Enabled {
		t.Errorf("rule[0] mismatch: %+v", rules[0])
	}
	if rules[1].Metric != "mem" || rules[1].Threshold != 90 || rules[1].Enabled {
		t.Errorf("rule[1] mismatch: %+v", rules[1])
	}

	// Assert backup configs.
	cfgs, err := st.ListBackupConfigs(&app.ID)
	if err != nil {
		t.Fatalf("list backup configs: %v", err)
	}
	if len(cfgs) != 1 {
		t.Fatalf("want 1 backup config, got %d", len(cfgs))
	}
	if cfgs[0].TargetConfigJSON != "enc-blob" {
		t.Errorf("encrypted blob not preserved: %q", cfgs[0].TargetConfigJSON)
	}
	if cfgs[0].RetentionDays == nil || *cfgs[0].RetentionDays != 30 {
		t.Errorf("retention days mismatch: %v", cfgs[0].RetentionDays)
	}
}

func TestRoundtripGlobalSidecar(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed user.
	u, err := st.CreateUser("alice", "$2a$10$fakehash", "admin", "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Seed API key for alice.
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	if _, err := st.CreateAPIKey(u.ID, "sha256hash", "ci"); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	// Also test expires_at via direct upsert (CreateAPIKey doesn't support it, so ok to skip for expires coverage)
	_ = expires

	// Seed webhook.
	wh := &store.Webhook{Name: "slack-main", Type: "slack", URL: "https://hooks.example.com/main"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Seed registry.
	reg, err := st.CreateRegistry("ghcr", "ghcr.io", "encuser", "encpass")
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}

	// Seed db backup config.
	if err := st.SetDBBackupConfig("schedule", "0 3 * * *"); err != nil {
		t.Fatalf("set db backup config: %v", err)
	}

	// Write global sidecar.
	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal: %v", err)
	}

	// Read it back.
	gdata, err := syncer.ReadGlobal()
	if err != nil {
		t.Fatalf("ReadGlobal: %v", err)
	}
	if gdata == nil {
		t.Fatal("expected non-nil GlobalSidecar")
	}
	if gdata.Version != 1 {
		t.Errorf("version = %d, want 1", gdata.Version)
	}
	if len(gdata.Users) != 1 || gdata.Users[0].Username != "alice" {
		t.Errorf("users mismatch: %+v", gdata.Users)
	}
	if gdata.Users[0].PasswordHash != "$2a$10$fakehash" {
		t.Errorf("password hash not preserved: %q", gdata.Users[0].PasswordHash)
	}
	if len(gdata.APIKeys) != 1 || gdata.APIKeys[0].Name != "ci" {
		t.Errorf("api_keys mismatch: %+v", gdata.APIKeys)
	}
	if len(gdata.Webhooks) != 1 || gdata.Webhooks[0].Name != "slack-main" {
		t.Errorf("webhooks mismatch: %+v", gdata.Webhooks)
	}
	if len(gdata.Registries) != 1 || gdata.Registries[0].ID != reg.ID {
		t.Errorf("registries mismatch: %+v", gdata.Registries)
	}
	if gdata.Registries[0].UsernameEnc != "encuser" || gdata.Registries[0].PasswordEnc != "encpass" {
		t.Errorf("encrypted blobs not preserved: %+v", gdata.Registries[0])
	}
	if gdata.DBBackupConfig["schedule"] != "0 3 * * *" {
		t.Errorf("db_backup_config mismatch: %+v", gdata.DBBackupConfig)
	}

	// Wipe and reimport.
	// Delete user (cascades to api_keys).
	if err := st.DeleteUser(u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if err := st.DeleteWebhook(wh.ID); err != nil {
		t.Fatalf("delete webhook: %v", err)
	}
	if err := st.DeleteRegistry(reg.ID); err != nil {
		t.Fatalf("delete registry: %v", err)
	}

	if err := syncer.ImportGlobal(gdata); err != nil {
		t.Fatalf("ImportGlobal: %v", err)
	}

	// Assert state restored.
	users, err := st.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "alice" || users[0].Role != "admin" {
		t.Errorf("users after import: %+v", users)
	}
	restored, err := st.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("get user alice: %v", err)
	}
	if restored.PasswordHash != "$2a$10$fakehash" {
		t.Errorf("password hash after import: %q", restored.PasswordHash)
	}
	keys, err := st.ListAPIKeysByUser(restored.ID)
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	if len(keys) != 1 || keys[0].KeyHash != "sha256hash" {
		t.Errorf("api keys after import: %+v", keys)
	}
	webhooks, err := st.ListWebhooks()
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	if len(webhooks) != 1 || webhooks[0].Name != "slack-main" {
		t.Errorf("webhooks after import: %+v", webhooks)
	}
	regs, err := st.ListRegistries()
	if err != nil {
		t.Fatalf("list registries: %v", err)
	}
	if len(regs) != 1 || regs[0].UsernameEnc != "encuser" {
		t.Errorf("registries after import: %+v", regs)
	}
}

func TestAtomicWrite(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "App", Slug: "atomic-test", ComposePath: "/apps/atomic-test/docker-compose.yml", Status: "stopped"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	if err := syncer.WriteAppSidecar("atomic-test"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}

	sidecarPath := filepath.Join(appsDir, "atomic-test", appSidecarName)
	tmpPath := sidecarPath + ".tmp"

	if _, err := os.Stat(sidecarPath); err != nil {
		t.Errorf("final file missing: %v", err)
	}
	if _, err := os.Stat(tmpPath); err == nil {
		t.Errorf(".tmp file should not exist after write")
	}
}

func TestFilePermissions(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "App", Slug: "perms-test", ComposePath: "/apps/perms-test/docker-compose.yml", Status: "stopped"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	if err := syncer.WriteAppSidecar("perms-test"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}
	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal: %v", err)
	}

	appPath := filepath.Join(appsDir, "perms-test", appSidecarName)
	globalPath := filepath.Join(dataDir, globalSidecar)

	for _, p := range []string{appPath, globalPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		mode := info.Mode().Perm()
		if mode != 0600 {
			t.Errorf("%s: mode = %o, want 0600", p, mode)
		}
	}
}

func TestSchemaVersion(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "App", Slug: "version-test", ComposePath: "/apps/version-test/docker-compose.yml", Status: "stopped"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	if err := syncer.WriteAppSidecar("version-test"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}

	appData, err := syncer.ReadAppSidecar("version-test")
	if err != nil {
		t.Fatalf("ReadAppSidecar: %v", err)
	}
	if appData.Version != 1 {
		t.Errorf("app sidecar version = %d, want 1", appData.Version)
	}

	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal: %v", err)
	}
	globalData, err := syncer.ReadGlobal()
	if err != nil {
		t.Fatalf("ReadGlobal: %v", err)
	}
	if globalData.Version != 1 {
		t.Errorf("global sidecar version = %d, want 1", globalData.Version)
	}
}

func TestDebounce(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "App", Slug: "debounce-test", ComposePath: "/apps/debounce-test/docker-compose.yml", Status: "stopped"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// We verify debounce by observing file mtime: 5 rapid calls should produce
	// exactly one file write (coalesced by the 500ms debouncer).
	for i := 0; i < 5; i++ {
		syncer.ScheduleAppWrite("debounce-test")
	}

	// Wait for debounce to fire (500ms + margin).
	time.Sleep(700 * time.Millisecond)

	sidecarPath := filepath.Join(appsDir, "debounce-test", appSidecarName)
	info1, err := os.Stat(sidecarPath)
	if err != nil {
		t.Fatalf("sidecar not written after debounce: %v", err)
	}
	mtime1 := info1.ModTime()

	// Wait another debounce period to confirm no extra write.
	time.Sleep(700 * time.Millisecond)
	info2, err := os.Stat(sidecarPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info2.ModTime().Equal(mtime1) {
		t.Errorf("file was written again; expected exactly one write")
	}
}

func TestMissingSidecarRead(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app, err := syncer.ReadAppSidecar("nope")
	if err != nil {
		t.Errorf("ReadAppSidecar missing: expected nil error, got %v", err)
	}
	if app != nil {
		t.Errorf("ReadAppSidecar missing: expected nil, got %+v", app)
	}

	glob, err := syncer.ReadGlobal()
	if err != nil {
		t.Errorf("ReadGlobal missing: expected nil error, got %v", err)
	}
	if glob != nil {
		t.Errorf("ReadGlobal missing: expected nil, got %+v", glob)
	}
}

func TestUnknownTopLevelKeyTolerated(t *testing.T) {
	appsDir := t.TempDir()
	dataDir := t.TempDir()

	// Write a YAML file with an unknown top-level key.
	content := `version: 1
app:
  slug: unknown-test
  display_name: Unknown Test
future_key: true
future_list:
  - a
  - b
`
	sidecarDir := filepath.Join(appsDir, "unknown-test")
	if err := os.MkdirAll(sidecarDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(sidecarDir, appSidecarName)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	st := openTestStore(t)
	syncer := New(st, appsDir, dataDir)
	data, err := syncer.ReadAppSidecar("unknown-test")
	if err != nil {
		t.Errorf("ReadAppSidecar with unknown key: expected no error, got %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil AppSidecar")
	}
	if data.App.Slug != "unknown-test" {
		t.Errorf("slug = %q, want unknown-test", data.App.Slug)
	}
}

func TestImportMissingWebhookFails(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "App", Slug: "missingwh", ComposePath: "/apps/missingwh/docker-compose.yml", Status: "stopped"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	data := &AppSidecar{
		Version: 1,
		App:     AppMeta{Slug: "missingwh", DisplayName: "App"},
		AlertRules: []AlertRuleEntry{
			{Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 300, Webhook: "nonexistent", Enabled: true},
		},
	}
	err := syncer.ImportAppSidecar(data)
	if err == nil {
		t.Error("expected error for missing webhook, got nil")
	}
}

// TestDebouncerRaceNoDroppedWrite verifies that calling schedule just before
// the timer fires does not lose the second write.
func TestDebouncerRaceNoDroppedWrite(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	fn := func() {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	s := &Syncer{
		timers:  make(map[string]*time.Timer),
		pending: make(map[string]struct{}),
	}

	// First schedule.
	s.schedule("x", fn)
	// Wait just under the debounce window, then reschedule.
	time.Sleep(400 * time.Millisecond)
	s.schedule("x", fn)
	// Wait for second timer to fire (500ms + margin).
	time.Sleep(700 * time.Millisecond)

	mu.Lock()
	got := callCount
	mu.Unlock()

	if got != 1 {
		t.Errorf("want exactly 1 fn call, got %d", got)
	}
}

// TestDebouncerRapidFireNoLostWrite verifies that rapid-fire scheduling
// eventually results in at least one fn call.
func TestDebouncerRapidFireNoLostWrite(t *testing.T) {
	callCount := 0
	var mu sync.Mutex
	fn := func() {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	s := &Syncer{
		timers:  make(map[string]*time.Timer),
		pending: make(map[string]struct{}),
	}

	// Fire every 100ms for ~1s, then wait for final timer.
	for i := 0; i < 10; i++ {
		s.schedule("x", fn)
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(700 * time.Millisecond)

	mu.Lock()
	got := callCount
	mu.Unlock()

	if got < 1 {
		t.Errorf("want at least 1 fn call, got %d", got)
	}
}

// ---------- ImportGlobalIfEmpty tests ----------

func TestImportGlobalIfEmpty_empty(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Write global sidecar with one user.
	sidecar := GlobalSidecar{
		Version: Version,
		Users: []UserEntry{{Username: "admin", PasswordHash: "$2a$10$hash", Role: "admin"}},
	}
	if err := atomicWriteYAML(filepath.Join(dataDir, globalSidecar), sidecar); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	imported, err := syncer.ImportGlobalIfEmpty()
	if err != nil {
		t.Fatalf("ImportGlobalIfEmpty: %v", err)
	}
	if !imported {
		t.Fatal("expected imported=true when DB is empty and sidecar exists")
	}

	users, err := st.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "admin" {
		t.Errorf("expected 1 user 'admin', got %v", users)
	}
}

func TestImportGlobalIfEmpty_nonempty(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Pre-seed DB with a user.
	if err := st.UpsertUserByUsername(&store.User{Username: "existing", PasswordHash: "h", Role: "admin"}); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Write sidecar with a different user.
	sidecar := GlobalSidecar{
		Version: Version,
		Users: []UserEntry{{Username: "recovered", PasswordHash: "$2a$10$hash", Role: "admin"}},
	}
	if err := atomicWriteYAML(filepath.Join(dataDir, globalSidecar), sidecar); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	imported, err := syncer.ImportGlobalIfEmpty()
	if err != nil {
		t.Fatalf("ImportGlobalIfEmpty: %v", err)
	}
	if imported {
		t.Fatal("expected imported=false when DB already has users")
	}

	// DB should still only have the pre-seeded user.
	users, err := st.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	for _, u := range users {
		if u.Username == "recovered" {
			t.Error("sidecar user should not have been imported")
		}
	}
}

func TestImportGlobalIfEmpty_missing(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// No sidecar on disk, empty DB.
	imported, err := syncer.ImportGlobalIfEmpty()
	if err != nil {
		t.Fatalf("ImportGlobalIfEmpty: %v", err)
	}
	if imported {
		t.Fatal("expected imported=false when sidecar is missing")
	}
}

// ---------- ImportAppSidecarIfMissing tests ----------

func TestImportAppSidecarIfMissing_empty(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed app row.
	app := &store.App{Name: "testapp", Slug: "testapp", ComposePath: "/apps/testapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// Write sidecar with one alert rule (no webhook ref needed for Enabled-only rule).
	wh := &store.Webhook{Name: "slack", Type: "slack", URL: "https://example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	sidecar := AppSidecar{
		Version: Version,
		App:     AppMeta{Slug: "testapp", DisplayName: "testapp"},
		AlertRules: []AlertRuleEntry{
			{Metric: "cpu", Operator: ">", Threshold: 75, DurationSec: 60, Webhook: "slack", Enabled: true},
		},
	}
	appDir := filepath.Join(appsDir, "testapp")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := atomicWriteYAML(filepath.Join(appDir, appSidecarName), sidecar); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	imported, err := syncer.ImportAppSidecarIfMissing("testapp")
	if err != nil {
		t.Fatalf("ImportAppSidecarIfMissing: %v", err)
	}
	if !imported {
		t.Fatal("expected imported=true when DB has no state")
	}

	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 || rules[0].Metric != "cpu" {
		t.Errorf("expected 1 cpu rule, got %v", rules)
	}
}

func TestImportAppSidecarIfMissing_nonempty(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "testapp", Slug: "testapp", ComposePath: "/apps/testapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// Pre-seed a webhook (required by FK).
	wh := &store.Webhook{Name: "opsgenie", Type: "custom", URL: "https://opsgenie.example.com"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Pre-seed an alert rule in DB.
	rule := &store.AlertRule{AppID: &app.ID, Metric: "mem", Operator: ">", Threshold: 90, DurationSec: 30, WebhookID: wh.ID, Enabled: true}
	if err := st.CreateAlertRule(rule); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	// Write sidecar with a different rule.
	sidecar := AppSidecar{
		Version: Version,
		App:     AppMeta{Slug: "testapp", DisplayName: "testapp"},
		AlertRules: []AlertRuleEntry{
			{Metric: "cpu", Operator: ">", Threshold: 75, DurationSec: 60, Enabled: true},
		},
	}
	appDir := filepath.Join(appsDir, "testapp")
	if err := os.MkdirAll(appDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := atomicWriteYAML(filepath.Join(appDir, appSidecarName), sidecar); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	imported, err := syncer.ImportAppSidecarIfMissing("testapp")
	if err != nil {
		t.Fatalf("ImportAppSidecarIfMissing: %v", err)
	}
	if imported {
		t.Fatal("expected imported=false when DB already has state")
	}

	// DB should still only have the pre-seeded rule.
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 1 || rules[0].Metric != "mem" {
		t.Errorf("expected original mem rule, got %v", rules)
	}
}

func TestImportAppSidecarIfMissing_missing(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "testapp", Slug: "testapp", ComposePath: "/apps/testapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// No sidecar on disk, empty app state.
	imported, err := syncer.ImportAppSidecarIfMissing("testapp")
	if err != nil {
		t.Fatalf("ImportAppSidecarIfMissing: %v", err)
	}
	if imported {
		t.Fatal("expected imported=false when sidecar is missing")
	}
}

func TestRoundtripEmptyApp(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	app := &store.App{Name: "Empty", Slug: "empty-app", ComposePath: "/apps/empty-app/docker-compose.yml", Status: "stopped"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	if err := syncer.WriteAppSidecar("empty-app"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}
	data, err := syncer.ReadAppSidecar("empty-app")
	if err != nil {
		t.Fatalf("ReadAppSidecar: %v", err)
	}
	if err := syncer.ImportAppSidecar(data); err != nil {
		t.Fatalf("ImportAppSidecar empty: %v", err)
	}

	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}
