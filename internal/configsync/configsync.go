// Package configsync mirrors user-editable DB rows to YAML sidecar files on disk.
// Per-app sidecar: {apps_dir}/{slug}/simpledeploy.yml
// Global sidecar:  {data_dir}/config.yml
//
// Writers are debounced (500ms) so rapid successive calls coalesce into one write.
// Importers perform idempotent upserts, restoring config after a DB wipe.
package configsync

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vazra/simpledeploy/internal/store"
)

const (
	appSidecarName        = "simpledeploy.yml"
	globalSidecar         = "config.yml"
	redactedGlobalSidecar = "_global.yml"
	debounceDelay         = 500 * time.Millisecond
	globalKey             = "\x00global" // non-slug sentinel for global debounce entry
)

// SidecarWriteHook is invoked after every successful sidecar write with the
// absolute path of the file written and a short reason string.
type SidecarWriteHook func(path string, reason string)

// Syncer writes and reads config sidecar files, debouncing frequent writes.
type Syncer struct {
	store   *store.Store
	appsDir string
	dataDir string

	mu      sync.Mutex
	timers  map[string]*time.Timer
	pending map[string]struct{} // keys with a live timer
	closed  bool

	hookMu sync.RWMutex
	hook   SidecarWriteHook
}

// SetSidecarWriteHook installs a callback invoked after each successful sidecar
// write. The hook receives the absolute file path. Panics from the hook are
// recovered so a buggy hook cannot corrupt writes.
func (s *Syncer) SetSidecarWriteHook(h SidecarWriteHook) {
	s.hookMu.Lock()
	s.hook = h
	s.hookMu.Unlock()
}

func (s *Syncer) callHook(path, reason string) {
	s.hookMu.RLock()
	h := s.hook
	s.hookMu.RUnlock()
	if h == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("configsync: SidecarWriteHook panic: %v", r)
		}
	}()
	h(path, reason)
}

// New creates a Syncer. It does not start any background work until Schedule* is called.
func New(st *store.Store, appsDir, dataDir string) *Syncer {
	return &Syncer{
		store:   st,
		appsDir: appsDir,
		dataDir: dataDir,
		timers:  make(map[string]*time.Timer),
		pending: make(map[string]struct{}),
	}
}

// Close flushes all pending debounced writes and stops all timers.
// Safe to call multiple times.
func (s *Syncer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	// collect pending keys before unlocking
	keys := make([]string, 0, len(s.pending))
	for k := range s.pending {
		keys = append(keys, k)
		if t, ok := s.timers[k]; ok {
			t.Stop()
		}
		delete(s.timers, k)
		delete(s.pending, k)
	}
	s.mu.Unlock()

	var errs []error
	for _, k := range keys {
		if k == globalKey {
			if err := s.WriteGlobal(); err != nil {
				errs = append(errs, err)
			}
			if err := s.WriteRedactedGlobal(); err != nil {
				errs = append(errs, err)
			}
		} else {
			if err := s.WriteAppSidecar(k); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("configsync close: %v", errs)
	}
	return nil
}

// ScheduleAppWrite schedules a debounced write of the app sidecar for slug.
func (s *Syncer) ScheduleAppWrite(slug string) {
	s.schedule(slug, func() { _ = s.WriteAppSidecar(slug) })
}

// ScheduleGlobalWrite schedules a debounced write of both global sidecars
// (config.yml and _global.yml) in a single 500ms window.
func (s *Syncer) ScheduleGlobalWrite() {
	s.schedule(globalKey, func() {
		if err := s.WriteGlobal(); err != nil {
			log.Printf("configsync: WriteGlobal: %v", err)
		}
		if err := s.WriteRedactedGlobal(); err != nil {
			log.Printf("configsync: WriteRedactedGlobal: %v", err)
		}
	})
}

func (s *Syncer) schedule(key string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	// Stop and discard any existing timer so its closure can't run after we
	// replace it. We always create a fresh timer whose closure captures a
	// pointer to itself; under the lock it checks "am I still the active
	// timer?" and skips if a newer call has already replaced it.
	if t, ok := s.timers[key]; ok {
		t.Stop()
	}
	s.pending[key] = struct{}{}
	var self *time.Timer
	self = time.AfterFunc(debounceDelay, func() {
		s.mu.Lock()
		if s.timers[key] != self {
			// superseded by a later schedule() call; skip.
			s.mu.Unlock()
			return
		}
		delete(s.timers, key)
		delete(s.pending, key)
		s.mu.Unlock()
		fn()
	})
	s.timers[key] = self
}

// DeleteAppSidecar removes {apps_dir}/{slug}/simpledeploy.yml if it exists.
// Safe to call even if the file or directory is gone. Ignores not-exist errors.
func (s *Syncer) DeleteAppSidecar(slug string) error {
	path := filepath.Join(s.appsDir, slug, appSidecarName)
	removed := true
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("DeleteAppSidecar %s: %w", slug, err)
		}
		removed = false
	}
	if removed {
		// Empty path signals "stage everything" so gitsync picks up the deletion.
		s.callHook("", fmt.Sprintf("app sidecar deleted: %s", slug))
	}
	return nil
}

// PruneOrphanSidecars removes per-app sidecar files for slugs not present in the DB.
// Safe to call on every startup. Returns the list of slugs whose sidecars were removed.
// Skips hidden dirs and names starting with '_' or '.'.
func (s *Syncer) PruneOrphanSidecars() ([]string, error) {
	entries, err := os.ReadDir(s.appsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("PruneOrphanSidecars: readdir %s: %w", s.appsDir, err)
	}

	var removed []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) == 0 || name[0] == '.' || name[0] == '_' {
			continue
		}
		sidecar := filepath.Join(s.appsDir, name, appSidecarName)
		if _, err := os.Stat(sidecar); os.IsNotExist(err) {
			continue
		}
		_, err := s.store.GetAppBySlug(name)
		if err == nil {
			// App exists in DB; keep sidecar.
			continue
		}
		// Not in DB; remove sidecar.
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return removed, fmt.Errorf("PruneOrphanSidecars: remove %s: %w", sidecar, err)
		}
		removed = append(removed, name)
	}
	if len(removed) > 0 {
		// Empty path signals "stage everything" so gitsync picks up deletions.
		s.callHook("", fmt.Sprintf("orphan sidecars pruned: %v", removed))
	}
	return removed, nil
}

// WriteAppSidecar reads the app and its config from the store and writes the sidecar atomically.
func (s *Syncer) WriteAppSidecar(slug string) error {
	sidecar, err := s.buildAppSidecar(slug)
	if err != nil {
		return fmt.Errorf("WriteAppSidecar %s: %w", slug, err)
	}
	path := filepath.Join(s.appsDir, slug, appSidecarName)
	if err := atomicWriteYAML(path, *sidecar); err != nil {
		return err
	}
	s.callHook(path, "app:"+slug)
	return nil
}

// buildAppSidecar reads app + per-app config from the store and returns the
// AppSidecar struct without writing it to disk. Used by WriteAppSidecar and
// by tombstone construction in archive.go.
func (s *Syncer) buildAppSidecar(slug string) (*AppSidecar, error) {
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		return nil, fmt.Errorf("get app: %w", err)
	}

	rules, err := s.store.ListAlertRules(&app.ID)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}

	webhooks, err := s.store.ListWebhooks()
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	webhookByID := make(map[int64]string, len(webhooks))
	for _, w := range webhooks {
		webhookByID[w.ID] = w.Name
	}

	backups, err := s.store.ListBackupConfigs(&app.ID)
	if err != nil {
		return nil, fmt.Errorf("list backup configs: %w", err)
	}

	accessUsernames, err := s.store.ListAccessForApp(app.ID)
	if err != nil {
		return nil, fmt.Errorf("list access: %w", err)
	}

	sidecar := &AppSidecar{
		Version: Version,
		App: AppMeta{
			Slug:        app.Slug,
			DisplayName: app.Name,
		},
	}

	for _, r := range rules {
		whName := webhookByID[r.WebhookID]
		sidecar.AlertRules = append(sidecar.AlertRules, AlertRuleEntry{
			Metric:      r.Metric,
			Operator:    r.Operator,
			Threshold:   r.Threshold,
			DurationSec: r.DurationSec,
			Webhook:     whName,
			Enabled:     r.Enabled,
		})
	}

	for _, b := range backups {
		sidecar.BackupConfigs = append(sidecar.BackupConfigs, BackupConfigEntry{
			Strategy:        b.Strategy,
			Target:          b.Target,
			ScheduleCron:    b.ScheduleCron,
			TargetConfigEnc: b.TargetConfigJSON,
			RetentionMode:   b.RetentionMode,
			RetentionCount:  b.RetentionCount,
			RetentionDays:   b.RetentionDays,
			VerifyUpload:    b.VerifyUpload,
			PreHooks:        b.PreHooks,
			PostHooks:       b.PostHooks,
			Paths:           b.Paths,
		})
	}

	for _, u := range accessUsernames {
		sidecar.Access = append(sidecar.Access, AccessEntry{Username: u})
	}

	return sidecar, nil
}

// WriteGlobal reads global config from the store and writes the global sidecar atomically.
func (s *Syncer) WriteGlobal() error {
	users, err := s.store.ListUsersWithHashes()
	if err != nil {
		return fmt.Errorf("WriteGlobal: list users: %w", err)
	}

	webhooks, err := s.store.ListWebhooks()
	if err != nil {
		return fmt.Errorf("WriteGlobal: list webhooks: %w", err)
	}

	registries, err := s.store.ListRegistries()
	if err != nil {
		return fmt.Errorf("WriteGlobal: list registries: %w", err)
	}

	dbBackupCfg, err := s.store.GetDBBackupConfig()
	if err != nil {
		return fmt.Errorf("WriteGlobal: get db backup config: %w", err)
	}

	sidecar := GlobalSidecar{
		Version: Version,
	}

	for _, u := range users {
		entry := UserEntry{
			Username:     u.Username,
			PasswordHash: u.PasswordHash,
			Role:         u.Role,
			DisplayName:  u.DisplayName,
			Email:        u.Email,
		}
		sidecar.Users = append(sidecar.Users, entry)

		keys, err := s.store.ListAPIKeysByUser(u.ID)
		if err != nil {
			return fmt.Errorf("WriteGlobal: list api keys for user %s: %w", u.Username, err)
		}
		for _, k := range keys {
			sidecar.APIKeys = append(sidecar.APIKeys, APIKeyEntry{
				Username:  u.Username,
				KeyHash:   k.KeyHash,
				Name:      k.Name,
				ExpiresAt: k.ExpiresAt,
			})
		}
	}

	for _, r := range registries {
		sidecar.Registries = append(sidecar.Registries, RegistryEntry{
			ID:          r.ID,
			Name:        r.Name,
			URL:         r.URL,
			UsernameEnc: r.UsernameEnc,
			PasswordEnc: r.PasswordEnc,
		})
	}

	for _, w := range webhooks {
		sidecar.Webhooks = append(sidecar.Webhooks, WebhookEntry{
			Name:             w.Name,
			Type:             w.Type,
			URL:              w.URL,
			TemplateOverride: w.TemplateOverride,
			HeadersJSON:      w.HeadersJSON,
		})
	}

	if len(dbBackupCfg) > 0 {
		sidecar.DBBackupConfig = dbBackupCfg
	}

	path := filepath.Join(s.dataDir, globalSidecar)
	if err := atomicWriteYAML(path, sidecar); err != nil {
		return err
	}
	s.callHook(path, "global")
	return nil
}

// ReadAppSidecar reads the app sidecar from disk. Returns (nil, nil) if the file does not exist.
// Unknown YAML keys are tolerated (logged as warnings).
func (s *Syncer) ReadAppSidecar(slug string) (*AppSidecar, error) {
	path := filepath.Join(s.appsDir, slug, appSidecarName)
	return readYAML[AppSidecar](path)
}

// ReadGlobal reads the global sidecar from disk. Returns (nil, nil) if the file does not exist.
func (s *Syncer) ReadGlobal() (*GlobalSidecar, error) {
	path := filepath.Join(s.dataDir, globalSidecar)
	return readYAML[GlobalSidecar](path)
}

// ImportAppSidecar performs idempotent upserts for all rows in data into the store.
// Existing alert_rules and backup_configs for the app are replaced wholesale.
// The caller must ensure the app exists in the DB before calling.
func (s *Syncer) ImportAppSidecar(data *AppSidecar) error {
	if data == nil {
		return nil
	}
	app, err := s.store.GetAppBySlug(data.App.Slug)
	if err != nil {
		return fmt.Errorf("ImportAppSidecar: get app %q: %w", data.App.Slug, err)
	}

	// Resolve webhook names -> IDs upfront so we fail atomically before mutating.
	webhooks, err := s.store.ListWebhooks()
	if err != nil {
		return fmt.Errorf("ImportAppSidecar: list webhooks: %w", err)
	}
	whByName := make(map[string]int64, len(webhooks))
	for _, w := range webhooks {
		whByName[w.Name] = w.ID
	}

	for i, rule := range data.AlertRules {
		if rule.Webhook != "" {
			if _, ok := whByName[rule.Webhook]; !ok {
				return fmt.Errorf("ImportAppSidecar: alert_rules[%d] references unknown webhook %q; import webhooks first", i, rule.Webhook)
			}
		}
	}

	// Full-replace alert rules for this app.
	if err := s.store.DeleteAlertRulesForApp(app.ID); err != nil {
		return fmt.Errorf("ImportAppSidecar: delete alert rules: %w", err)
	}
	for _, r := range data.AlertRules {
		var whID int64
		if r.Webhook != "" {
			whID = whByName[r.Webhook]
		}
		rule := &store.AlertRule{
			AppID:       &app.ID,
			Metric:      r.Metric,
			Operator:    r.Operator,
			Threshold:   r.Threshold,
			DurationSec: r.DurationSec,
			WebhookID:   whID,
			Enabled:     r.Enabled,
		}
		if err := s.store.CreateAlertRule(rule); err != nil {
			return fmt.Errorf("ImportAppSidecar: create alert rule: %w", err)
		}
	}

	// Full-replace backup configs for this app.
	if err := s.store.DeleteBackupConfigsForApp(app.ID); err != nil {
		return fmt.Errorf("ImportAppSidecar: delete backup configs: %w", err)
	}
	for _, b := range data.BackupConfigs {
		cfg := &store.BackupConfig{
			AppID:            app.ID,
			Strategy:         b.Strategy,
			Target:           b.Target,
			ScheduleCron:     b.ScheduleCron,
			TargetConfigJSON: b.TargetConfigEnc,
			RetentionMode:    b.RetentionMode,
			RetentionCount:   b.RetentionCount,
			RetentionDays:    b.RetentionDays,
			VerifyUpload:     b.VerifyUpload,
			PreHooks:         b.PreHooks,
			PostHooks:        b.PostHooks,
			Paths:            b.Paths,
		}
		if err := s.store.CreateBackupConfig(cfg); err != nil {
			return fmt.Errorf("ImportAppSidecar: create backup config: %w", err)
		}
	}

	// Full-replace access grants for this app.
	usernames := make([]string, 0, len(data.Access))
	for _, a := range data.Access {
		usernames = append(usernames, a.Username)
	}
	if err := s.store.ReplaceAppAccess(app.ID, usernames); err != nil {
		return fmt.Errorf("ImportAppSidecar: replace access: %w", err)
	}

	return nil
}

// ImportGlobal performs idempotent upserts for all rows in data into the store.
// Import order: webhooks -> users -> api_keys -> registries -> db_backup_config.
func (s *Syncer) ImportGlobal(data *GlobalSidecar) error {
	if data == nil {
		return nil
	}

	for _, w := range data.Webhooks {
		wh := &store.Webhook{
			Name:             w.Name,
			Type:             w.Type,
			URL:              w.URL,
			TemplateOverride: w.TemplateOverride,
			HeadersJSON:      w.HeadersJSON,
		}
		if err := s.store.UpsertWebhookByName(wh); err != nil {
			return fmt.Errorf("ImportGlobal: upsert webhook %q: %w", w.Name, err)
		}
	}

	for _, u := range data.Users {
		user := &store.User{
			Username:     u.Username,
			PasswordHash: u.PasswordHash,
			Role:         u.Role,
			DisplayName:  u.DisplayName,
			Email:        u.Email,
		}
		if err := s.store.UpsertUserByUsername(user); err != nil {
			return fmt.Errorf("ImportGlobal: upsert user %q: %w", u.Username, err)
		}
	}

	for _, k := range data.APIKeys {
		if err := s.store.UpsertAPIKey(k.Username, k.KeyHash, k.Name, k.ExpiresAt); err != nil {
			return fmt.Errorf("ImportGlobal: upsert api key %q/%q: %w", k.Username, k.Name, err)
		}
	}

	for _, r := range data.Registries {
		reg := &store.Registry{
			ID:          r.ID,
			Name:        r.Name,
			URL:         r.URL,
			UsernameEnc: r.UsernameEnc,
			PasswordEnc: r.PasswordEnc,
		}
		if err := s.store.UpsertRegistryByID(reg); err != nil {
			return fmt.Errorf("ImportGlobal: upsert registry %q: %w", r.ID, err)
		}
	}

	for k, v := range data.DBBackupConfig {
		if err := s.store.SetDBBackupConfig(k, v); err != nil {
			return fmt.Errorf("ImportGlobal: set db_backup_config %q: %w", k, err)
		}
	}

	return nil
}

// ImportGlobalIfEmpty imports the global sidecar only when the DB has zero users.
// Returns (imported bool, err error). If no sidecar exists -> (false, nil).
// If users table is non-empty -> (false, nil) without modifying DB.
func (s *Syncer) ImportGlobalIfEmpty() (bool, error) {
	users, err := s.store.ListUsers()
	if err != nil {
		return false, fmt.Errorf("ImportGlobalIfEmpty: list users: %w", err)
	}
	if len(users) > 0 {
		return false, nil
	}

	data, err := s.ReadGlobal()
	if err != nil {
		return false, fmt.Errorf("ImportGlobalIfEmpty: read global: %w", err)
	}
	if data == nil {
		return false, nil
	}

	if err := s.ImportGlobal(data); err != nil {
		return false, fmt.Errorf("ImportGlobalIfEmpty: import: %w", err)
	}
	return true, nil
}

// WriteRedactedGlobal reads global config from the store and writes the redacted
// {apps_dir}/_global.yml. Contains no secrets.
func (s *Syncer) WriteRedactedGlobal() error {
	users, err := s.store.ListUsersWithHashes()
	if err != nil {
		return fmt.Errorf("WriteRedactedGlobal: list users: %w", err)
	}

	webhooks, err := s.store.ListWebhooks()
	if err != nil {
		return fmt.Errorf("WriteRedactedGlobal: list webhooks: %w", err)
	}

	registries, err := s.store.ListRegistries()
	if err != nil {
		return fmt.Errorf("WriteRedactedGlobal: list registries: %w", err)
	}

	dbBackupCfg, err := s.store.GetDBBackupConfig()
	if err != nil {
		return fmt.Errorf("WriteRedactedGlobal: get db backup config: %w", err)
	}

	sidecar := RedactedGlobalSidecar{
		Version:          Version,
		DBBackupSchedule: dbBackupCfg["schedule"],
		DBBackupTarget:   dbBackupCfg["target"],
	}

	for _, u := range users {
		sidecar.Users = append(sidecar.Users, RedactedUser{
			Username:    u.Username,
			Role:        u.Role,
			DisplayName: u.DisplayName,
			Email:       u.Email,
		})
	}

	for _, r := range registries {
		sidecar.Registries = append(sidecar.Registries, RedactedRegistry{
			ID:   r.ID,
			Name: r.Name,
			URL:  r.URL,
		})
	}

	for _, w := range webhooks {
		sidecar.Webhooks = append(sidecar.Webhooks, RedactedWebhook{
			Name: w.Name,
			Type: w.Type,
		})
	}

	path := filepath.Join(s.appsDir, redactedGlobalSidecar)
	if err := atomicWriteYAML(path, sidecar); err != nil {
		return err
	}
	s.callHook(path, "redacted-global")
	return nil
}

// ReadRedactedGlobal reads the redacted global sidecar. Returns (nil, nil) if missing.
func (s *Syncer) ReadRedactedGlobal() (*RedactedGlobalSidecar, error) {
	path := filepath.Join(s.appsDir, redactedGlobalSidecar)
	return readYAML[RedactedGlobalSidecar](path)
}

// ImportRedactedGlobal applies non-secret deltas from the redacted file to the DB.
// Preserves existing password hashes, encrypted registry credentials, and webhook URLs.
// Does NOT delete users/registries/webhooks that are absent from the file.
// Does NOT touch api_keys.
func (s *Syncer) ImportRedactedGlobal(data *RedactedGlobalSidecar) error {
	if data == nil {
		return nil
	}

	for _, w := range data.Webhooks {
		if err := s.store.UpsertWebhookFromRedacted(w.Name, w.Type); err != nil {
			return fmt.Errorf("ImportRedactedGlobal: upsert webhook %q: %w", w.Name, err)
		}
	}

	for _, u := range data.Users {
		if err := s.store.UpdateUserFromRedacted(u.Username, u.Role, u.DisplayName, u.Email); err != nil {
			return fmt.Errorf("ImportRedactedGlobal: upsert user %q: %w", u.Username, err)
		}
	}

	for _, r := range data.Registries {
		if err := s.store.UpsertRegistryFromRedacted(r.ID, r.Name, r.URL); err != nil {
			return fmt.Errorf("ImportRedactedGlobal: upsert registry %q: %w", r.ID, err)
		}
	}

	if data.DBBackupSchedule != "" {
		if err := s.store.SetDBBackupConfig("schedule", data.DBBackupSchedule); err != nil {
			return fmt.Errorf("ImportRedactedGlobal: set db_backup_config schedule: %w", err)
		}
	}
	if data.DBBackupTarget != "" {
		if err := s.store.SetDBBackupConfig("target", data.DBBackupTarget); err != nil {
			return fmt.Errorf("ImportRedactedGlobal: set db_backup_config target: %w", err)
		}
	}

	return nil
}

// ImportAppSidecarIfMissing reads the per-app sidecar and imports it only when
// the app's DB-side state (alert rules + backup configs + access grants) is empty.
// Returns (imported bool, err error). Missing sidecar -> (false, nil).
// If any of the three is non-empty, treat as "DB has state" and skip import.
// The app row must already exist before calling.
func (s *Syncer) ImportAppSidecarIfMissing(slug string) (bool, error) {
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		return false, fmt.Errorf("ImportAppSidecarIfMissing %s: get app: %w", slug, err)
	}

	rules, err := s.store.ListAlertRules(&app.ID)
	if err != nil {
		return false, fmt.Errorf("ImportAppSidecarIfMissing %s: list alert rules: %w", slug, err)
	}
	backups, err := s.store.ListBackupConfigs(&app.ID)
	if err != nil {
		return false, fmt.Errorf("ImportAppSidecarIfMissing %s: list backup configs: %w", slug, err)
	}
	access, err := s.store.ListAccessForApp(app.ID)
	if err != nil {
		return false, fmt.Errorf("ImportAppSidecarIfMissing %s: list access: %w", slug, err)
	}

	if len(rules) > 0 || len(backups) > 0 || len(access) > 0 {
		// DB already has state; do not clobber.
		return false, nil
	}

	data, err := s.ReadAppSidecar(slug)
	if err != nil {
		return false, fmt.Errorf("ImportAppSidecarIfMissing %s: read sidecar: %w", slug, err)
	}
	if data == nil {
		return false, nil
	}

	if err := s.ImportAppSidecar(data); err != nil {
		return false, fmt.Errorf("ImportAppSidecarIfMissing %s: import: %w", slug, err)
	}
	return true, nil
}

// atomicWriteYAML marshals v to YAML and atomically writes it to path (0600 perm).
func atomicWriteYAML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open tmp %s: %w", tmp, err)
	}

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("yaml encode %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("yaml encode close %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("fsync %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// readYAML reads and decodes a YAML file into T. Returns (nil, nil) if the file does not exist.
// Unknown keys are tolerated (logged as warnings).
func readYAML[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var v T
	dec := yaml.NewDecoder(bytesReader(data))
	dec.KnownFields(false) // tolerate unknown keys
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("yaml decode %s: %w", path, err)
	}
	return &v, nil
}

// bytesReader wraps a byte slice for yaml.NewDecoder.
func bytesReader(data []byte) *bytes.Reader { return bytes.NewReader(data) }
