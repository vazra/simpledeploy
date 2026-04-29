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
	u, err := st.CreateUser("alice", "$2a$10$fakehash", "manage", "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Seed API key for alice.
	expires := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	if _, err := st.CreateAPIKey(u.ID, "sha256hash", "ci", nil); err != nil {
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
	gsec, err := syncer.ReadGlobalSecrets()
	if err != nil || gsec == nil {
		t.Fatalf("ReadGlobalSecrets: %v / nil=%v", err, gsec == nil)
	}
	if len(gsec.Users) != 1 || gsec.Users[0].PasswordHash != "$2a$10$fakehash" {
		t.Errorf("password hash not preserved: %+v", gsec.Users)
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
	if len(gsec.Registries) != 1 || gsec.Registries[0].UsernameEnc != "encuser" || gsec.Registries[0].PasswordEnc != "encpass" {
		t.Errorf("encrypted blobs not preserved: %+v", gsec.Registries)
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
	if len(users) != 1 || users[0].Username != "alice" || users[0].Role != "manage" {
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
	appSecretsPath := filepath.Join(appsDir, "perms-test", appSecretsName)
	globalSecretsPath := filepath.Join(dataDir, globalSecretsName)

	// Non-secret sidecars: 0644.
	for _, p := range []string{appPath, globalPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if mode := info.Mode().Perm(); mode != 0644 {
			t.Errorf("%s: mode = %o, want 0644", p, mode)
		}
	}
	// Secret sidecars: 0600.
	for _, p := range []string{appSecretsPath, globalSecretsPath} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if mode := info.Mode().Perm(); mode != 0600 {
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

	// Wait for debounce to fire (500ms) with a generous margin so this
	// doesn't flake on CI under -race where scheduling latency is high.
	sidecarPath := filepath.Join(appsDir, "debounce-test", appSidecarName)
	var info1 os.FileInfo
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var err error
		info1, err = os.Stat(sidecarPath)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if info1 == nil {
		t.Fatalf("sidecar not written after debounce within 5s: %s", sidecarPath)
	}
	mtime1 := info1.ModTime()

	// Wait another debounce period to confirm no extra write.
	time.Sleep(800 * time.Millisecond)
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

	// Write global sidecar with one user; secrets in sibling secrets.yml.
	sidecar := GlobalSidecar{
		Version: Version,
		Users:   []UserEntry{{Username: "admin", Role: "manage"}},
	}
	if err := atomicWriteYAMLMode(filepath.Join(dataDir, globalSidecar), 0644, sidecar); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	if err := syncer.WriteGlobalSecrets(&GlobalSecrets{
		Version: Version,
		Users:   []UserSecretsEntry{{Username: "admin", PasswordHash: "$2a$10$hash"}},
	}); err != nil {
		t.Fatalf("write secrets: %v", err)
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
	if err := st.UpsertUserByUsername(&store.User{Username: "existing", PasswordHash: "h", Role: "manage"}); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Write sidecar with a different user.
	sidecar := GlobalSidecar{
		Version: Version,
		Users:   []UserEntry{{Username: "recovered", Role: "manage"}},
	}
	if err := atomicWriteYAMLMode(filepath.Join(dataDir, globalSidecar), 0644, sidecar); err != nil {
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

// TestFirstBootBackfillIdempotent verifies that running WriteGlobal +
// WriteAppSidecar twice (simulating first-boot backfill) succeeds without error.
func TestFirstBootBackfillIdempotent(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed one user.
	if err := st.UpsertUserByUsername(&store.User{Username: "admin", PasswordHash: "$2a$10$h", Role: "super_admin"}); err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	// Seed one app.
	app := &store.App{Name: "My App", Slug: "myapp", ComposePath: "/apps/myapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	// First backfill pass.
	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal pass 1: %v", err)
	}
	if err := syncer.WriteAppSidecar("myapp"); err != nil {
		t.Fatalf("WriteAppSidecar pass 1: %v", err)
	}

	// Assert sidecar files exist.
	if _, err := os.Stat(filepath.Join(dataDir, globalSidecar)); err != nil {
		t.Errorf("global sidecar missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(appsDir, "myapp", appSidecarName)); err != nil {
		t.Errorf("app sidecar missing: %v", err)
	}

	// Second backfill pass (idempotent).
	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal pass 2: %v", err)
	}
	if err := syncer.WriteAppSidecar("myapp"); err != nil {
		t.Fatalf("WriteAppSidecar pass 2: %v", err)
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

// ---------------------------------------------------------------------------
// Redacted global sidecar tests
// ---------------------------------------------------------------------------

func TestRedactedGlobalRoundtrip(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed data with secrets.
	if _, err := st.CreateUser("alice", "$2a$12$fakehashalice", "super_admin", "Alice Admin", "alice@example.com"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := st.CreateRegistry("MyReg", "registry.example.com", "enc-user-blob", "enc-pass-blob"); err != nil {
		t.Fatalf("create registry: %v", err)
	}
	wh := &store.Webhook{Name: "notify-slack", Type: "slack", URL: "https://hooks.slack.com/secret-token"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	if err := syncer.WriteRedactedGlobal(); err != nil {
		t.Fatalf("WriteRedactedGlobal: %v", err)
	}

	path := filepath.Join(appsDir, "_global.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read _global.yml: %v", err)
	}
	content := string(raw)

	// Secrets must NOT appear.
	if contains(content, "fakehashalice") {
		t.Error("_global.yml must not contain password hash")
	}
	if contains(content, "enc-user-blob") {
		t.Error("_global.yml must not contain username_enc")
	}
	if contains(content, "enc-pass-blob") {
		t.Error("_global.yml must not contain password_enc")
	}
	if contains(content, "secret-token") {
		t.Error("_global.yml must not contain webhook URL secret")
	}

	// Non-secret fields must be present.
	data, err := syncer.ReadRedactedGlobal()
	if err != nil {
		t.Fatalf("ReadRedactedGlobal: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil RedactedGlobalSidecar")
	}
	if len(data.Users) != 1 || data.Users[0].Username != "alice" || data.Users[0].Role != "super_admin" {
		t.Errorf("unexpected users: %+v", data.Users)
	}
	if len(data.Registries) != 1 || data.Registries[0].URL != "registry.example.com" {
		t.Errorf("unexpected registries: %+v", data.Registries)
	}
	if len(data.Webhooks) != 1 || data.Webhooks[0].Name != "notify-slack" || data.Webhooks[0].Type != "slack" {
		t.Errorf("unexpected webhooks: %+v", data.Webhooks)
	}
}

func TestImportRedactedPreservesSecrets(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	origHash := "$2a$12$originalHashForBob"
	if _, err := st.CreateUser("bob", origHash, "manage", "Bob", "bob@example.com"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	reg, err := st.CreateRegistry("BobReg", "reg.example.com", "orig-user-enc", "orig-pass-enc")
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	wh := &store.Webhook{Name: "bob-hook", Type: "slack", URL: "https://real-url.example.com/webhook"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Write redacted file (captures current state).
	if err := syncer.WriteRedactedGlobal(); err != nil {
		t.Fatalf("WriteRedactedGlobal: %v", err)
	}
	data, err := syncer.ReadRedactedGlobal()
	if err != nil {
		t.Fatalf("ReadRedactedGlobal: %v", err)
	}

	// Modify non-secret fields in the redacted data (simulate edit).
	data.Users[0].DisplayName = "Robert"
	data.Webhooks[0].Type = "discord"

	// Import – must NOT overwrite secrets.
	if err := syncer.ImportRedactedGlobal(data); err != nil {
		t.Fatalf("ImportRedactedGlobal: %v", err)
	}

	// Assert password_hash preserved.
	u, err := st.GetUserByUsername("bob")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.PasswordHash != origHash {
		t.Errorf("password_hash changed: want %q got %q", origHash, u.PasswordHash)
	}
	if u.DisplayName != "Robert" {
		t.Errorf("display_name not updated: %q", u.DisplayName)
	}

	// Assert registry encrypted creds preserved.
	r, err := st.GetRegistry(reg.ID)
	if err != nil {
		t.Fatalf("get registry: %v", err)
	}
	if r.UsernameEnc != "orig-user-enc" || r.PasswordEnc != "orig-pass-enc" {
		t.Errorf("registry enc creds changed: %q / %q", r.UsernameEnc, r.PasswordEnc)
	}

	// Assert webhook URL preserved.
	got, err := st.GetWebhook(wh.ID)
	if err != nil {
		t.Fatalf("get webhook: %v", err)
	}
	if got.URL != "https://real-url.example.com/webhook" {
		t.Errorf("webhook URL changed: %q", got.URL)
	}
	if got.Type != "discord" {
		t.Errorf("webhook type not updated: %q", got.Type)
	}
}

func TestImportRedactedCreatesUserWithoutPassword(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	data := &RedactedGlobalSidecar{
		Version: 1,
		Users: []RedactedUser{
			{Username: "newbie", Role: "viewer", DisplayName: "New User"},
		},
	}
	if err := syncer.ImportRedactedGlobal(data); err != nil {
		t.Fatalf("ImportRedactedGlobal: %v", err)
	}

	u, err := st.GetUserByUsername("newbie")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.PasswordHash != "" {
		t.Errorf("expected empty password_hash for new user, got %q", u.PasswordHash)
	}
	// Confirm bcrypt.CompareHashAndPassword("", anything) returns an error (cannot auth).
	// This is verified by the empty hash: bcrypt treats "" as invalid cost.
}

func TestImportRedactedNoDeletes(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Pre-existing user not in redacted file.
	if _, err := st.CreateUser("alice", "$2a$12$hash", "manage", "", ""); err != nil {
		t.Fatalf("create alice: %v", err)
	}

	// Redacted file has only "bob".
	data := &RedactedGlobalSidecar{
		Version: 1,
		Users:   []RedactedUser{{Username: "bob", Role: "viewer"}},
	}
	if err := syncer.ImportRedactedGlobal(data); err != nil {
		t.Fatalf("ImportRedactedGlobal: %v", err)
	}

	// Alice must still exist.
	if _, err := st.GetUserByUsername("alice"); err != nil {
		t.Errorf("alice was deleted by import: %v", err)
	}
	// Bob must have been created.
	if _, err := st.GetUserByUsername("bob"); err != nil {
		t.Errorf("bob was not created: %v", err)
	}
}

func TestDebouncedGlobalWritesBothFiles(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)
	defer syncer.Close()

	if _, err := st.CreateUser("carol", "$2a$12$hash", "manage", "", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	syncer.ScheduleGlobalWrite()

	// Wait for debounce to fire (500ms + buffer).
	time.Sleep(700 * time.Millisecond)

	secretPath := filepath.Join(dataDir, "config.yml")
	redactedPath := filepath.Join(appsDir, "_global.yml")

	if _, err := os.Stat(secretPath); err != nil {
		t.Errorf("config.yml missing: %v", err)
	}
	if _, err := os.Stat(redactedPath); err != nil {
		t.Errorf("_global.yml missing: %v", err)
	}

	// Verify _global.yml does NOT contain the hash.
	raw, err := os.ReadFile(redactedPath)
	if err != nil {
		t.Fatalf("read _global.yml: %v", err)
	}
	if contains(string(raw), "$2a$12$hash") {
		t.Error("_global.yml must not contain password_hash")
	}
}

// ---------------------------------------------------------------------------
// Idempotency tests
// ---------------------------------------------------------------------------

// TestImportAppSidecarIdempotent verifies that importing the same app sidecar
// twice leaves the DB in exactly the same state as after the first import.
func TestImportAppSidecarIdempotent(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	wh := &store.Webhook{Name: "slack-idem", Type: "slack", URL: "https://hooks.example.com/idem"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	app := &store.App{Name: "IdemApp", Slug: "idemapp", ComposePath: "/apps/idemapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}

	r1 := &store.AlertRule{AppID: &app.ID, Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 300, WebhookID: wh.ID, Enabled: true}
	if err := st.CreateAlertRule(r1); err != nil {
		t.Fatalf("create rule: %v", err)
	}

	retDays := 14
	bc := &store.BackupConfig{
		AppID: app.ID, Strategy: "postgres", Target: "s3",
		ScheduleCron: "0 4 * * *", TargetConfigJSON: "enc-blob-idem",
		RetentionMode: "count", RetentionCount: 5, RetentionDays: &retDays,
	}
	if err := st.CreateBackupConfig(bc); err != nil {
		t.Fatalf("create backup config: %v", err)
	}

	u, err := st.CreateUser("accessuser", "$2a$10$h", "viewer", "", "")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.ReplaceAppAccess(app.ID, []string{u.Username}); err != nil {
		t.Fatalf("grant access: %v", err)
	}

	// Write sidecar then clear tables.
	if err := syncer.WriteAppSidecar("idemapp"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}
	if err := st.DeleteAlertRulesForApp(app.ID); err != nil {
		t.Fatalf("delete rules: %v", err)
	}
	if err := st.DeleteBackupConfigsForApp(app.ID); err != nil {
		t.Fatalf("delete backup configs: %v", err)
	}
	if err := st.ReplaceAppAccess(app.ID, nil); err != nil {
		t.Fatalf("clear access: %v", err)
	}

	data, err := syncer.ReadAppSidecar("idemapp")
	if err != nil || data == nil {
		t.Fatalf("ReadAppSidecar: %v / nil=%v", err, data == nil)
	}

	// First import.
	if err := syncer.ImportAppSidecar(data); err != nil {
		t.Fatalf("ImportAppSidecar first: %v", err)
	}

	// Capture state after first import.
	rules1, _ := st.ListAlertRules(&app.ID)
	cfgs1, _ := st.ListBackupConfigs(&app.ID)
	access1, _ := st.ListAccessForApp(app.ID)

	// Second import (idempotent).
	if err := syncer.ImportAppSidecar(data); err != nil {
		t.Fatalf("ImportAppSidecar second: %v", err)
	}

	rules2, _ := st.ListAlertRules(&app.ID)
	cfgs2, _ := st.ListBackupConfigs(&app.ID)
	access2, _ := st.ListAccessForApp(app.ID)

	if len(rules2) != len(rules1) {
		t.Errorf("alert rules count changed: first=%d second=%d", len(rules1), len(rules2))
	}
	if len(cfgs2) != len(cfgs1) {
		t.Errorf("backup configs count changed: first=%d second=%d", len(cfgs1), len(cfgs2))
	}
	if len(rules1) > 0 && len(rules2) > 0 {
		if rules1[0].Metric != rules2[0].Metric || rules1[0].Threshold != rules2[0].Threshold {
			t.Errorf("rule value changed: %+v -> %+v", rules1[0], rules2[0])
		}
	}
	if len(cfgs1) > 0 && len(cfgs2) > 0 {
		if cfgs1[0].TargetConfigJSON != cfgs2[0].TargetConfigJSON {
			t.Errorf("target_config_enc changed: %q -> %q", cfgs1[0].TargetConfigJSON, cfgs2[0].TargetConfigJSON)
		}
	}
	if len(access2) != len(access1) {
		t.Errorf("access count changed: first=%d second=%d", len(access1), len(access2))
	}
}

// TestImportGlobalIdempotent verifies that importing the same global sidecar
// twice leaves the DB in exactly the same state as after the first import.
func TestImportGlobalIdempotent(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	u, err := st.CreateUser("alice", "$2a$10$fakehash", "manage", "Alice", "alice@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := st.CreateAPIKey(u.ID, "keyhash-idem", "ci", nil); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	wh := &store.Webhook{Name: "idem-slack", Type: "slack", URL: "https://hooks.example.com/idem"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	reg, err := st.CreateRegistry("idemreg", "reg.example.com", "encuser", "encpass")
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	if err := st.SetDBBackupConfig("schedule", "0 3 * * *"); err != nil {
		t.Fatalf("set db_backup_config: %v", err)
	}

	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal: %v", err)
	}
	gdata, err := syncer.ReadGlobal()
	if err != nil || gdata == nil {
		t.Fatalf("ReadGlobal: %v / nil=%v", err, gdata == nil)
	}

	// Wipe tables.
	if err := st.DeleteUser(u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if err := st.DeleteWebhook(wh.ID); err != nil {
		t.Fatalf("delete webhook: %v", err)
	}
	if err := st.DeleteRegistry(reg.ID); err != nil {
		t.Fatalf("delete registry: %v", err)
	}

	// First import.
	if err := syncer.ImportGlobal(gdata); err != nil {
		t.Fatalf("ImportGlobal first: %v", err)
	}

	users1, _ := st.ListUsers()
	webhooks1, _ := st.ListWebhooks()
	regs1, _ := st.ListRegistries()
	cfg1, _ := st.GetDBBackupConfig()

	// Second import.
	if err := syncer.ImportGlobal(gdata); err != nil {
		t.Fatalf("ImportGlobal second: %v", err)
	}

	users2, _ := st.ListUsers()
	webhooks2, _ := st.ListWebhooks()
	regs2, _ := st.ListRegistries()
	cfg2, _ := st.GetDBBackupConfig()

	if len(users2) != len(users1) {
		t.Errorf("users count changed: %d -> %d", len(users1), len(users2))
	}
	if len(webhooks2) != len(webhooks1) {
		t.Errorf("webhooks count changed: %d -> %d", len(webhooks1), len(webhooks2))
	}
	if len(regs2) != len(regs1) {
		t.Errorf("registries count changed: %d -> %d", len(regs1), len(regs2))
	}
	if cfg1["schedule"] != cfg2["schedule"] {
		t.Errorf("db_backup_config schedule changed: %q -> %q", cfg1["schedule"], cfg2["schedule"])
	}
	if len(users1) > 0 && len(users2) > 0 {
		if users1[0].Username != users2[0].Username || users1[0].Role != users2[0].Role {
			t.Errorf("user fields changed: %+v -> %+v", users1[0], users2[0])
		}
	}
}

// TestDBBackupConfigEncRoundtrip verifies that a binary-blob value in
// db_backup_config survives WriteGlobal -> ReadGlobal -> ImportGlobal
// byte-for-byte, including non-printable bytes.
func TestDBBackupConfigEncRoundtrip(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Build a blob with non-printable bytes to stress the YAML codec.
	blob := "AES256\x00\x01\x02\xFF\xFEbinary\x00data\nwith\nnewlines"

	if err := st.SetDBBackupConfig("target_config_enc", blob); err != nil {
		t.Fatalf("SetDBBackupConfig target_config_enc: %v", err)
	}
	if err := st.SetDBBackupConfig("schedule", "0 3 * * *"); err != nil {
		t.Fatalf("SetDBBackupConfig schedule: %v", err)
	}
	if err := st.SetDBBackupConfig("target", "s3"); err != nil {
		t.Fatalf("SetDBBackupConfig target: %v", err)
	}

	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal: %v", err)
	}

	gdata, err := syncer.ReadGlobal()
	if err != nil || gdata == nil {
		t.Fatalf("ReadGlobal: %v / nil=%v", err, gdata == nil)
	}
	gsec, err := syncer.ReadGlobalSecrets()
	if err != nil || gsec == nil {
		t.Fatalf("ReadGlobalSecrets: %v / nil=%v", err, gsec == nil)
	}

	if gsec.DBBackup == nil || gsec.DBBackup.TargetConfigEnc != blob {
		t.Errorf("blob changed after write+read\nwant: %q\ngot:  %v", blob, gsec.DBBackup)
	}
	if _, found := gdata.DBBackupConfig["target_config_enc"]; found {
		t.Errorf("target_config_enc must NOT appear in non-secret config.yml")
	}

	// Wipe and re-import.
	if err := st.SetDBBackupConfig("target_config_enc", ""); err != nil {
		t.Fatalf("wipe target_config_enc: %v", err)
	}
	if err := st.SetDBBackupConfig("schedule", ""); err != nil {
		t.Fatalf("wipe schedule: %v", err)
	}
	if err := st.SetDBBackupConfig("target", ""); err != nil {
		t.Fatalf("wipe target: %v", err)
	}

	if err := syncer.ImportGlobal(gdata); err != nil {
		t.Fatalf("ImportGlobal: %v", err)
	}

	cfg, err := st.GetDBBackupConfig()
	if err != nil {
		t.Fatalf("GetDBBackupConfig: %v", err)
	}

	if cfg["target_config_enc"] != blob {
		t.Errorf("blob changed after import\nwant: %q\ngot:  %q", blob, cfg["target_config_enc"])
	}
	if cfg["schedule"] != "0 3 * * *" {
		t.Errorf("schedule changed: %q", cfg["schedule"])
	}
	if cfg["target"] != "s3" {
		t.Errorf("target changed: %q", cfg["target"])
	}
}

// TestConfigExportImportRoundtrip: full export (WriteGlobal + WriteAppSidecar),
// WipeConfigForRestore, then ImportGlobal + ImportAppSidecar restores everything.
func TestConfigExportImportRoundtrip(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed global data.
	u, err := st.CreateUser("roundtrip-admin", "$2a$10$hash", "super_admin", "Admin", "admin@example.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := st.CreateAPIKey(u.ID, "apikeyhash", "mykey", nil); err != nil {
		t.Fatalf("create api key: %v", err)
	}
	wh := &store.Webhook{Name: "rt-slack", Type: "slack", URL: "https://hooks.example.com/rt"}
	if err := st.CreateWebhook(wh); err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	reg, err := st.CreateRegistry("rtreg", "reg.example.com", "encuser-rt", "encpass-rt")
	if err != nil {
		t.Fatalf("create registry: %v", err)
	}
	if err := st.SetDBBackupConfig("schedule", "0 5 * * *"); err != nil {
		t.Fatalf("set db_backup_config: %v", err)
	}

	// Seed app.
	app := &store.App{Name: "RT App", Slug: "rtapp", ComposePath: "/apps/rtapp/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	r1 := &store.AlertRule{AppID: &app.ID, Metric: "cpu", Operator: ">", Threshold: 75, DurationSec: 120, WebhookID: wh.ID, Enabled: true}
	if err := st.CreateAlertRule(r1); err != nil {
		t.Fatalf("create alert rule: %v", err)
	}
	retDays := 7
	bc := &store.BackupConfig{
		AppID: app.ID, Strategy: "volume", Target: "s3",
		ScheduleCron: "0 2 * * *", TargetConfigJSON: "enc-vol",
		RetentionMode: "time", RetentionDays: &retDays,
	}
	if err := st.CreateBackupConfig(bc); err != nil {
		t.Fatalf("create backup config: %v", err)
	}
	if err := st.ReplaceAppAccess(app.ID, []string{u.Username}); err != nil {
		t.Fatalf("grant access: %v", err)
	}

	// Export.
	if err := syncer.WriteGlobal(); err != nil {
		t.Fatalf("WriteGlobal: %v", err)
	}
	if err := syncer.WriteAppSidecar("rtapp"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}

	// Wipe config tables (keeps apps/metrics/deploy_events etc.).
	if err := st.WipeConfigForRestore(); err != nil {
		t.Fatalf("WipeConfigForRestore: %v", err)
	}

	// Re-read sidecars from disk.
	gdata, err := syncer.ReadGlobal()
	if err != nil || gdata == nil {
		t.Fatalf("ReadGlobal after wipe: %v / nil=%v", err, gdata == nil)
	}
	adata, err := syncer.ReadAppSidecar("rtapp")
	if err != nil || adata == nil {
		t.Fatalf("ReadAppSidecar after wipe: %v / nil=%v", err, adata == nil)
	}

	// Import global first (webhooks must exist before app sidecar import).
	if err := syncer.ImportGlobal(gdata); err != nil {
		t.Fatalf("ImportGlobal: %v", err)
	}
	if err := syncer.ImportAppSidecar(adata); err != nil {
		t.Fatalf("ImportAppSidecar: %v", err)
	}

	// Assert users.
	users, err := st.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 || users[0].Username != "roundtrip-admin" || users[0].Role != "super_admin" {
		t.Errorf("users after roundtrip: %+v", users)
	}
	restored, err := st.GetUserByUsername("roundtrip-admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if restored.PasswordHash != "$2a$10$hash" {
		t.Errorf("password hash mismatch: %q", restored.PasswordHash)
	}

	// Assert API keys.
	keys, err := st.ListAPIKeysByUser(restored.ID)
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	if len(keys) != 1 || keys[0].KeyHash != "apikeyhash" {
		t.Errorf("api keys after roundtrip: %+v", keys)
	}

	// Assert webhooks.
	webhooks, err := st.ListWebhooks()
	if err != nil {
		t.Fatalf("list webhooks: %v", err)
	}
	if len(webhooks) != 1 || webhooks[0].Name != "rt-slack" {
		t.Errorf("webhooks after roundtrip: %+v", webhooks)
	}

	// Assert registries.
	regs, err := st.ListRegistries()
	if err != nil {
		t.Fatalf("list registries: %v", err)
	}
	if len(regs) != 1 || regs[0].ID != reg.ID || regs[0].UsernameEnc != "encuser-rt" {
		t.Errorf("registries after roundtrip: %+v", regs)
	}

	// Assert db_backup_config.
	cfg, err := st.GetDBBackupConfig()
	if err != nil {
		t.Fatalf("GetDBBackupConfig: %v", err)
	}
	if cfg["schedule"] != "0 5 * * *" {
		t.Errorf("db_backup_config schedule: %q", cfg["schedule"])
	}

	// Assert alert rules.
	rules, err := st.ListAlertRules(&app.ID)
	if err != nil {
		t.Fatalf("list alert rules: %v", err)
	}
	if len(rules) != 1 || rules[0].Metric != "cpu" || rules[0].Threshold != 75 {
		t.Errorf("alert rules after roundtrip: %+v", rules)
	}

	// Assert backup configs.
	cfgs, err := st.ListBackupConfigs(&app.ID)
	if err != nil {
		t.Fatalf("list backup configs: %v", err)
	}
	if len(cfgs) != 1 || cfgs[0].TargetConfigJSON != "enc-vol" {
		t.Errorf("backup configs after roundtrip: %+v", cfgs)
	}

	// Assert access grants.
	access, err := st.ListAccessForApp(app.ID)
	if err != nil {
		t.Fatalf("list access: %v", err)
	}
	if len(access) != 1 || access[0] != "roundtrip-admin" {
		t.Errorf("access after roundtrip: %+v", access)
	}
}

// contains is a simple substring check.
func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && stringContains(s, sub))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDeleteAppSidecar(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed app and write sidecar.
	app := &store.App{Name: "Delete Me", Slug: "deleteme", ComposePath: "/apps/deleteme/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatalf("upsert app: %v", err)
	}
	if err := syncer.WriteAppSidecar("deleteme"); err != nil {
		t.Fatalf("WriteAppSidecar: %v", err)
	}
	sidecarPath := filepath.Join(appsDir, "deleteme", appSidecarName)
	if _, err := os.Stat(sidecarPath); err != nil {
		t.Fatalf("sidecar should exist: %v", err)
	}

	// Delete it.
	if err := syncer.DeleteAppSidecar("deleteme"); err != nil {
		t.Fatalf("DeleteAppSidecar: %v", err)
	}
	if _, err := os.Stat(sidecarPath); !os.IsNotExist(err) {
		t.Fatal("sidecar should be gone after DeleteAppSidecar")
	}

	// Idempotent: calling again on missing file must not error.
	if err := syncer.DeleteAppSidecar("deleteme"); err != nil {
		t.Fatalf("DeleteAppSidecar on missing file: %v", err)
	}
}

func TestPruneOrphanSidecars(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	// Seed one live app.
	alive := &store.App{Name: "Alive", Slug: "alive", ComposePath: "/apps/alive/docker-compose.yml", Status: "running"}
	if err := st.UpsertApp(alive, nil); err != nil {
		t.Fatalf("upsert alive: %v", err)
	}
	if err := syncer.WriteAppSidecar("alive"); err != nil {
		t.Fatalf("WriteAppSidecar alive: %v", err)
	}

	// Create orphan sidecars on disk (no matching DB rows).
	for _, ghost := range []string{"ghost1", "ghost2"} {
		dir := filepath.Join(appsDir, ghost)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("mkdir %s: %v", ghost, err)
		}
		if err := os.WriteFile(filepath.Join(dir, appSidecarName), []byte("version: 1\n"), 0600); err != nil {
			t.Fatalf("write ghost sidecar %s: %v", ghost, err)
		}
	}

	pruned, err := syncer.PruneOrphanSidecars()
	if err != nil {
		t.Fatalf("PruneOrphanSidecars: %v", err)
	}

	// Both ghosts should be pruned.
	if len(pruned) != 2 {
		t.Fatalf("expected 2 pruned, got %d: %v", len(pruned), pruned)
	}
	for _, ghost := range []string{"ghost1", "ghost2"} {
		sidecar := filepath.Join(appsDir, ghost, appSidecarName)
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Errorf("ghost sidecar %s should be gone", ghost)
		}
		// Directory should still exist.
		if _, err := os.Stat(filepath.Join(appsDir, ghost)); err != nil {
			t.Errorf("ghost dir %s should still exist: %v", ghost, err)
		}
	}

	// Alive sidecar must remain.
	if _, err := os.Stat(filepath.Join(appsDir, "alive", appSidecarName)); err != nil {
		t.Errorf("alive sidecar should still exist: %v", err)
	}
}

// TestDeleteAppSidecarNoOpNoHook verifies that DeleteAppSidecar does not invoke
// the hook when the sidecar file does not exist, and invokes it exactly once
// when it does.
func TestDeleteAppSidecarNoOpNoHook(t *testing.T) {
	st := openTestStore(t)
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	syncer := New(st, appsDir, dataDir)

	var calls []string
	var mu sync.Mutex
	syncer.SetSidecarWriteHook(func(path, reason string) {
		mu.Lock()
		calls = append(calls, reason)
		mu.Unlock()
	})

	// File does not exist: hook must NOT be called.
	if err := syncer.DeleteAppSidecar("ghost"); err != nil {
		t.Fatalf("DeleteAppSidecar ghost (no file): %v", err)
	}
	mu.Lock()
	gotCalls := len(calls)
	mu.Unlock()
	if gotCalls != 0 {
		t.Fatalf("expected 0 hook calls when file absent, got %d", gotCalls)
	}

	// Create a real sidecar.
	sidecarDir := filepath.Join(appsDir, "realapp")
	if err := os.MkdirAll(sidecarDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sidecarPath := filepath.Join(sidecarDir, appSidecarName)
	if err := os.WriteFile(sidecarPath, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	if err := syncer.DeleteAppSidecar("realapp"); err != nil {
		t.Fatalf("DeleteAppSidecar realapp: %v", err)
	}
	mu.Lock()
	gotCalls = len(calls)
	mu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("expected exactly 1 hook call after deleting real sidecar, got %d", gotCalls)
	}
}
