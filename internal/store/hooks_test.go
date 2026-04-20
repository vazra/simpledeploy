package store

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func openHookTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// recorder collects hook calls.
type recorder struct {
	mu    sync.Mutex
	calls []hookCall
}

type hookCall struct {
	scope MutationScope
	slug  string
}

func (r *recorder) hook() MutationHook {
	return func(scope MutationScope, slug string) {
		r.mu.Lock()
		r.calls = append(r.calls, hookCall{scope, slug})
		r.mu.Unlock()
	}
}

func (r *recorder) last() (hookCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		return hookCall{}, false
	}
	return r.calls[len(r.calls)-1], true
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

// seedApp inserts a minimal app and returns its ID.
func seedApp(t *testing.T, s *Store, slug string) int64 {
	t.Helper()
	var id int64
	err := s.db.QueryRow(
		`INSERT INTO apps (name, slug, compose_path, status) VALUES (?, ?, ?, ?) RETURNING id`,
		slug, slug, "/tmp/"+slug+"/docker-compose.yml", "stopped",
	).Scan(&id)
	if err != nil {
		t.Fatalf("seedApp %q: %v", slug, err)
	}
	return id
}

// seedWebhook inserts a webhook and returns its ID.
func seedWebhook(t *testing.T, s *Store) int64 {
	t.Helper()
	w := &Webhook{Name: "test-wh", Type: "slack", URL: "http://example.com"}
	if err := s.CreateWebhook(w); err != nil {
		t.Fatalf("seedWebhook: %v", err)
	}
	return w.ID
}

func TestMutationHook_NilByDefault(t *testing.T) {
	s := openHookTestStore(t)
	// Should not panic when no hook set.
	_ = s.CreateWebhook(&Webhook{Name: "x", Type: "slack", URL: "http://x.com"})
}

func TestMutationHook_SetNilDisables(t *testing.T) {
	s := openHookTestStore(t)
	rec := &recorder{}
	s.SetMutationHook(rec.hook())
	s.SetMutationHook(nil)
	_ = s.CreateWebhook(&Webhook{Name: "x", Type: "slack", URL: "http://x.com"})
	if rec.count() != 0 {
		t.Fatalf("expected 0 calls after nil, got %d", rec.count())
	}
}

func TestMutationHook_PanicRecovery(t *testing.T) {
	s := openHookTestStore(t)
	s.SetMutationHook(func(MutationScope, string) { panic("boom") })
	// Should not crash.
	if err := s.CreateWebhook(&Webhook{Name: "x", Type: "slack", URL: "http://x.com"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMutationHook_ErrorNoFire(t *testing.T) {
	s := openHookTestStore(t)
	rec := &recorder{}
	s.SetMutationHook(rec.hook())

	// Delete nonexistent webhook — should not fire.
	_ = s.DeleteWebhook(99999)
	if rec.count() != 0 {
		t.Fatalf("expected 0 hook calls on error, got %d", rec.count())
	}

	// Delete nonexistent alert rule — should not fire.
	_ = s.DeleteAlertRule(99999)
	if rec.count() != 0 {
		t.Fatalf("expected 0 hook calls on rule error, got %d", rec.count())
	}
}

func TestMutationHook_GlobalEntities(t *testing.T) {
	s := openHookTestStore(t)
	rec := &recorder{}
	s.SetMutationHook(rec.hook())

	// CreateWebhook
	w := &Webhook{Name: "wh1", Type: "slack", URL: "http://a.com"}
	if err := s.CreateWebhook(w); err != nil {
		t.Fatalf("CreateWebhook: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")
	before := rec.count()

	// UpdateWebhook
	w.URL = "http://b.com"
	if err := s.UpdateWebhook(w); err != nil {
		t.Fatalf("UpdateWebhook: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// DeleteWebhook
	if err := s.DeleteWebhook(w.ID); err != nil {
		t.Fatalf("DeleteWebhook: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")
	_ = before

	// CreateUser
	u, err := s.CreateUser("alice", "hash", "admin", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// DeleteUser
	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// UpdateProfile, UpdateUserRole, UpdatePassword — need a live user.
	u2, _ := s.CreateUser("bob", "hash", "admin", "", "")
	rec.mu.Lock(); rec.calls = nil; rec.mu.Unlock()

	if err := s.UpdateProfile(u2.ID, "Bob", "b@x.com"); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	if err := s.UpdateUserRole(u2.ID, "super_admin"); err != nil {
		t.Fatalf("UpdateUserRole: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	if err := s.UpdatePassword(u2.ID, "newhash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// UpsertUserByUsername
	uu := &User{Username: "carol", PasswordHash: "h", Role: "admin"}
	if err := s.UpsertUserByUsername(uu); err != nil {
		t.Fatalf("UpsertUserByUsername: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// CreateAPIKey
	k, err := s.CreateAPIKey(u2.ID, "keyhash", "mykey")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// DeleteAPIKey
	if err := s.DeleteAPIKey(k.ID, u2.ID); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// UpsertAPIKey
	if err := s.UpsertAPIKey(u2.Username, "keyhash2", "key2", nil); err != nil {
		t.Fatalf("UpsertAPIKey: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// Registry CRUD
	reg, err := s.CreateRegistry("myreg", "https://r.io", "u_enc", "p_enc")
	if err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	if err := s.UpdateRegistry(reg.ID, "myreg2", "https://r2.io", "u2", "p2"); err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	if err := s.UpsertRegistryByID(&Registry{ID: reg.ID, Name: "myreg3", URL: "https://r3.io", UsernameEnc: "u", PasswordEnc: "p"}); err != nil {
		t.Fatalf("UpsertRegistryByID: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	if err := s.DeleteRegistry(reg.ID); err != nil {
		t.Fatalf("DeleteRegistry: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// SetDBBackupConfig
	if err := s.SetDBBackupConfig("schedule", "0 * * * *"); err != nil {
		t.Fatalf("SetDBBackupConfig: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")

	// UpsertWebhookByName
	wh := &Webhook{Name: "upsert-wh", Type: "slack", URL: "http://c.com"}
	if err := s.UpsertWebhookByName(wh); err != nil {
		t.Fatalf("UpsertWebhookByName insert: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")
	wh2 := &Webhook{Name: "upsert-wh", Type: "slack", URL: "http://d.com"}
	if err := s.UpsertWebhookByName(wh2); err != nil {
		t.Fatalf("UpsertWebhookByName update: %v", err)
	}
	assertLastHook(t, rec, ScopeGlobal, "")
}

func TestMutationHook_AppEntities(t *testing.T) {
	s := openHookTestStore(t)
	appID := seedApp(t, s, "myapp")
	whID := seedWebhook(t, s)

	rec := &recorder{}
	s.SetMutationHook(rec.hook())

	// CreateAlertRule with AppID
	r := &AlertRule{AppID: &appID, Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 60, WebhookID: whID, Enabled: true}
	if err := s.CreateAlertRule(r); err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}
	c, ok := rec.last()
	if !ok || c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("CreateAlertRule hook: got %+v ok=%v", c, ok)
	}

	// UpdateAlertRule
	r.Threshold = 90
	if err := s.UpdateAlertRule(r); err != nil {
		t.Fatalf("UpdateAlertRule: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("UpdateAlertRule hook: %+v", c)
	}

	// DeleteAlertRule
	if err := s.DeleteAlertRule(r.ID); err != nil {
		t.Fatalf("DeleteAlertRule: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("DeleteAlertRule hook: %+v", c)
	}

	// DeleteAlertRulesForApp
	r2 := &AlertRule{AppID: &appID, Metric: "mem", Operator: ">", Threshold: 80, DurationSec: 60, WebhookID: whID, Enabled: true}
	_ = s.CreateAlertRule(r2)
	before := rec.count()
	if err := s.DeleteAlertRulesForApp(appID); err != nil {
		t.Fatalf("DeleteAlertRulesForApp: %v", err)
	}
	if rec.count() <= before {
		t.Fatalf("DeleteAlertRulesForApp: hook not fired")
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("DeleteAlertRulesForApp hook: %+v", c)
	}

	// CreateBackupConfig
	cfg := &BackupConfig{AppID: appID, Strategy: "postgres", Target: "local", ScheduleCron: "0 * * * *", TargetConfigJSON: "{}", RetentionMode: "count", RetentionCount: 3}
	if err := s.CreateBackupConfig(cfg); err != nil {
		t.Fatalf("CreateBackupConfig: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("CreateBackupConfig hook: %+v", c)
	}

	// UpdateBackupConfig
	cfg.ScheduleCron = "0 1 * * *"
	if err := s.UpdateBackupConfig(cfg); err != nil {
		t.Fatalf("UpdateBackupConfig: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("UpdateBackupConfig hook: %+v", c)
	}

	// DeleteBackupConfig
	if err := s.DeleteBackupConfig(cfg.ID); err != nil {
		t.Fatalf("DeleteBackupConfig: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("DeleteBackupConfig hook: %+v", c)
	}

	// GrantAppAccess / RevokeAppAccess / ReplaceAppAccess
	u, _ := s.CreateUser("dave", "hash", "admin", "", "")
	rec.mu.Lock(); rec.calls = nil; rec.mu.Unlock()

	if err := s.GrantAppAccess(u.ID, appID); err != nil {
		t.Fatalf("GrantAppAccess: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("GrantAppAccess hook: %+v", c)
	}

	if err := s.RevokeAppAccess(u.ID, appID); err != nil {
		t.Fatalf("RevokeAppAccess: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("RevokeAppAccess hook: %+v", c)
	}

	if err := s.ReplaceAppAccess(appID, []string{u.Username}); err != nil {
		t.Fatalf("ReplaceAppAccess: %v", err)
	}
	c, _ = rec.last()
	if c.scope != ScopeApp || c.slug != "myapp" {
		t.Fatalf("ReplaceAppAccess hook: %+v", c)
	}
}

func TestMutationHook_GlobalAlertRule(t *testing.T) {
	s := openHookTestStore(t)
	whID := seedWebhook(t, s)
	rec := &recorder{}
	s.SetMutationHook(rec.hook())

	// CreateAlertRule without AppID (global)
	r := &AlertRule{AppID: nil, Metric: "cpu", Operator: ">", Threshold: 80, DurationSec: 60, WebhookID: whID, Enabled: true}
	if err := s.CreateAlertRule(r); err != nil {
		t.Fatalf("CreateAlertRule global: %v", err)
	}
	c, ok := rec.last()
	if !ok || c.scope != ScopeGlobal {
		t.Fatalf("global CreateAlertRule hook: got %+v ok=%v", c, ok)
	}
}

func TestMutationHook_ConcurrentRace(t *testing.T) {
	s := openHookTestStore(t)
	appID := seedApp(t, s, "race-app")
	whID := seedWebhook(t, s)
	_ = whID

	var count atomic.Int64
	s.SetMutationHook(func(MutationScope, string) { count.Add(1) })

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				s.SetMutationHook(func(MutationScope, string) { count.Add(1) })
			} else {
				cfg := &BackupConfig{AppID: appID, Strategy: "postgres", Target: "local", ScheduleCron: "0 * * * *", TargetConfigJSON: "{}", RetentionMode: "count", RetentionCount: 3}
				_ = s.CreateBackupConfig(cfg)
			}
		}(i)
	}
	wg.Wait()
	// Just verify no race — count may be anything.
	_ = count.Load()
}

func TestMutationHook_DeleteAlertRule_NotFound(t *testing.T) {
	s := openHookTestStore(t)
	rec := &recorder{}
	s.SetMutationHook(rec.hook())

	// seed webhook to avoid FK issues
	seedWebhook(t, s)
	before := rec.count()

	err := s.DeleteAlertRule(99999)
	if err == nil {
		t.Fatal("expected error for missing rule")
	}
	if rec.count() != before {
		t.Fatalf("hook fired on error: before=%d after=%d", before, rec.count())
	}
}

func TestMutationHook_DeleteBackupConfig_NotFound(t *testing.T) {
	s := openHookTestStore(t)
	rec := &recorder{}
	s.SetMutationHook(rec.hook())

	err := s.DeleteBackupConfig(99999)
	if err == nil {
		t.Fatal("expected error")
	}
	if rec.count() != 0 {
		t.Fatalf("hook fired on error")
	}
}

// assertLastHook is a helper to check the last hook call.
func assertLastHook(t *testing.T, rec *recorder, wantScope MutationScope, wantSlug string) {
	t.Helper()
	c, ok := rec.last()
	if !ok {
		t.Fatalf("expected hook call, got none")
	}
	if c.scope != wantScope || c.slug != wantSlug {
		t.Fatalf("hook call: got {scope=%d slug=%q}, want {scope=%d slug=%q}", c.scope, c.slug, wantScope, wantSlug)
	}
}

var _ = time.Now // ensure time import used (not actually needed but suppresses lint)
