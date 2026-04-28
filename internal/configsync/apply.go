package configsync

// FS->DB apply path. Unlike Import* (which only seeds an empty DB), Apply*
// reconciles the DB to match FS-authoritative state on every reload.
//
// Lives in configsync (not store) because store importing configsync types
// would create an import cycle (configsync already imports store).

import (
	"fmt"
	"log"

	"github.com/vazra/simpledeploy/internal/store"
)

// ApplyAppSidecar reconciles per-app DB rows to match the FS-loaded sidecar
// for the given slug. The app row must already exist. Performs full-replace
// for alert_rules, backup_configs, and user_app_access. Updates apps.name
// from sidecar display_name. Alert rules referencing unknown webhooks are
// dropped with a log line (rather than failing the whole apply).
//
// loaded.Sidecar must be non-nil. loaded.Secrets may be nil.
func (s *Syncer) ApplyAppSidecar(slug string, loaded *LoadedApp) error {
	if loaded == nil || loaded.Sidecar == nil {
		return nil
	}
	data := loaded.Sidecar

	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		return fmt.Errorf("ApplyAppSidecar %s: get app: %w", slug, err)
	}

	// Update display name if changed.
	if data.App.DisplayName != "" && data.App.DisplayName != app.Name {
		if err := s.store.UpdateAppDisplayName(slug, data.App.DisplayName); err != nil {
			return fmt.Errorf("ApplyAppSidecar %s: update display name: %w", slug, err)
		}
	}

	// Resolve webhooks by name.
	webhooks, err := s.store.ListWebhooks()
	if err != nil {
		return fmt.Errorf("ApplyAppSidecar %s: list webhooks: %w", slug, err)
	}
	whByName := make(map[string]int64, len(webhooks))
	for _, w := range webhooks {
		whByName[w.Name] = w.ID
	}

	// Reconcile alert_rules by natural key (metric,operator,threshold,
	// duration_sec,webhook_id). Existing rows whose key still matches an
	// incoming rule keep their id (and their alert_history rows); rows not
	// in the incoming set are deleted; new keys are created. Rules pointing
	// at unknown webhooks are dropped with a log line.
	existingRules, err := s.store.ListAlertRules(&app.ID)
	if err != nil {
		return fmt.Errorf("ApplyAppSidecar %s: list alert rules: %w", slug, err)
	}
	type ruleKey struct {
		metric, op string
		threshold  float64
		dur        int
		whID       int64
	}
	keyOf := func(r store.AlertRule) ruleKey {
		return ruleKey{r.Metric, r.Operator, r.Threshold, r.DurationSec, r.WebhookID}
	}
	existingByKey := make(map[ruleKey]store.AlertRule, len(existingRules))
	for _, r := range existingRules {
		existingByKey[keyOf(r)] = r
	}
	keepIDs := make(map[int64]struct{}, len(data.AlertRules))
	for i, r := range data.AlertRules {
		var whID int64
		if r.Webhook != "" {
			id, ok := whByName[r.Webhook]
			if !ok {
				log.Printf("configsync ApplyAppSidecar %s: alert_rules[%d] references unknown webhook %q; skipping", slug, i, r.Webhook)
				continue
			}
			whID = id
		}
		k := ruleKey{r.Metric, r.Operator, r.Threshold, r.DurationSec, whID}
		if prev, ok := existingByKey[k]; ok {
			keepIDs[prev.ID] = struct{}{}
			if prev.Enabled != r.Enabled {
				updated := prev
				updated.Enabled = r.Enabled
				if err := s.store.UpdateAlertRule(&updated); err != nil {
					return fmt.Errorf("ApplyAppSidecar %s: update alert rule: %w", slug, err)
				}
			}
			continue
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
			return fmt.Errorf("ApplyAppSidecar %s: create alert rule: %w", slug, err)
		}
		keepIDs[rule.ID] = struct{}{}
	}
	for _, prev := range existingRules {
		if _, keep := keepIDs[prev.ID]; keep {
			continue
		}
		if err := s.store.DeleteAlertRule(prev.ID); err != nil {
			return fmt.Errorf("ApplyAppSidecar %s: delete alert rule: %w", slug, err)
		}
	}

	// Reconcile backup_configs by UUID: update existing rows in place,
	// insert new ones, delete only those whose UUID is no longer present.
	// Preserves backup_runs (FK ON DELETE CASCADE) across sidecar reapplies.
	encByID := map[string]string{}
	if loaded.Secrets != nil {
		for _, e := range loaded.Secrets.BackupConfigs {
			encByID[e.ID] = e.TargetConfigEnc
		}
	}
	existingCfgs, err := s.store.ListBackupConfigs(&app.ID)
	if err != nil {
		return fmt.Errorf("ApplyAppSidecar %s: list backup configs: %w", slug, err)
	}
	existingByUUID := make(map[string]store.BackupConfig, len(existingCfgs))
	for _, c := range existingCfgs {
		existingByUUID[c.UUID] = c
	}
	incomingUUIDs := make(map[string]struct{}, len(data.BackupConfigs))
	for _, b := range data.BackupConfigs {
		incomingUUIDs[b.ID] = struct{}{}
		cfg := &store.BackupConfig{
			UUID:             b.ID,
			AppID:            app.ID,
			Strategy:         b.Strategy,
			Target:           b.Target,
			ScheduleCron:     b.ScheduleCron,
			TargetConfigJSON: encByID[b.ID],
			RetentionMode:    b.RetentionMode,
			RetentionCount:   b.RetentionCount,
			RetentionDays:    b.RetentionDays,
			VerifyUpload:     b.VerifyUpload,
			PreHooks:         b.PreHooks,
			PostHooks:        b.PostHooks,
			Paths:            b.Paths,
		}
		if prev, ok := existingByUUID[b.ID]; ok {
			cfg.ID = prev.ID
			// DR-safe: when the secrets sidecar lacks an entry for this
			// UUID (e.g. a debounced write hasn't flushed yet), preserve
			// the existing encrypted target_config_json instead of
			// blanking it.
			if cfg.TargetConfigJSON == "" {
				cfg.TargetConfigJSON = prev.TargetConfigJSON
			}
			if err := s.store.UpdateBackupConfig(cfg); err != nil {
				return fmt.Errorf("ApplyAppSidecar %s: update backup config: %w", slug, err)
			}
		} else {
			if err := s.store.CreateBackupConfig(cfg); err != nil {
				return fmt.Errorf("ApplyAppSidecar %s: create backup config: %w", slug, err)
			}
		}
	}
	for uuid, prev := range existingByUUID {
		if _, keep := incomingUUIDs[uuid]; keep {
			continue
		}
		if err := s.store.DeleteBackupConfig(prev.ID); err != nil {
			return fmt.Errorf("ApplyAppSidecar %s: delete backup config: %w", slug, err)
		}
	}

	// Full-replace user_app_access. ReplaceAppAccess silently skips users
	// not present in the DB.
	usernames := make([]string, 0, len(data.Access))
	for _, a := range data.Access {
		usernames = append(usernames, a.Username)
	}
	if err := s.store.ReplaceAppAccess(app.ID, usernames); err != nil {
		return fmt.Errorf("ApplyAppSidecar %s: replace access: %w", slug, err)
	}

	return nil
}

// ApplyGlobalSidecar reconciles global DB rows (users, api_keys, registries,
// webhooks, db_backup_config) to match the FS-loaded global sidecar.
//
// CRITICAL DR safety: if loaded.Secrets == nil (secrets file missing) we
// run only the non-secret diff and leave secret columns (password_hash,
// key_hash, *_enc, webhook URL/headers/template, db backup target_config_enc)
// untouched. This prevents a missing secrets.yml from wiping the admin's
// password and locking everyone out.
//
// Full-replace semantics: rows present in the DB but absent from the sidecar
// are deleted (users, api_keys, registries, webhooks). db_backup_config is
// upserted (single config row, not a set).
func (s *Syncer) ApplyGlobalSidecar(loaded *LoadedGlobal) error {
	if loaded == nil || loaded.Sidecar == nil {
		return nil
	}
	data := loaded.Sidecar
	secrets := loaded.Secrets // may be nil; DR-safe path below

	// Index secrets (nil-safe).
	pwByUser := map[string]string{}
	keyHashByUserName := map[string]string{}
	regSecByID := map[string]RegistrySecretsEntry{}
	whSecByName := map[string]WebhookSecretsEntry{}
	var dbBackupEnc string
	hasSecrets := secrets != nil
	if hasSecrets {
		for _, u := range secrets.Users {
			pwByUser[u.Username] = u.PasswordHash
		}
		for _, k := range secrets.APIKeys {
			keyHashByUserName[k.Username+"|"+k.Name] = k.KeyHash
		}
		for _, r := range secrets.Registries {
			regSecByID[r.ID] = r
		}
		for _, w := range secrets.Webhooks {
			whSecByName[w.Name] = w
		}
		if secrets.DBBackup != nil {
			dbBackupEnc = secrets.DBBackup.TargetConfigEnc
		}
	}

	// --- Webhooks: replace. Need name-set to drop missing.
	wantWebhooks := make(map[string]struct{}, len(data.Webhooks))
	for _, w := range data.Webhooks {
		wantWebhooks[w.Name] = struct{}{}
		wh := &store.Webhook{Name: w.Name, Type: w.Type}
		if hasSecrets {
			sec := whSecByName[w.Name]
			wh.URL = sec.URL
			wh.HeadersJSON = sec.HeadersJSON
			wh.TemplateOverride = sec.TemplateOverride
			if err := s.store.UpsertWebhookByName(wh); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: upsert webhook %q: %w", w.Name, err)
			}
		} else {
			// No secrets: do not touch URL/headers/template (preserve DB).
			if err := s.store.UpsertWebhookFromRedacted(w.Name, w.Type); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: upsert webhook (redacted) %q: %w", w.Name, err)
			}
		}
	}
	existingWebhooks, err := s.store.ListWebhooks()
	if err != nil {
		return fmt.Errorf("ApplyGlobalSidecar: list webhooks: %w", err)
	}
	for _, w := range existingWebhooks {
		if _, keep := wantWebhooks[w.Name]; !keep {
			if err := s.store.DeleteWebhook(w.ID); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: delete webhook %q: %w", w.Name, err)
			}
		}
	}

	// --- Users: replace.
	wantUsers := make(map[string]struct{}, len(data.Users))
	for _, u := range data.Users {
		wantUsers[u.Username] = struct{}{}
		if hasSecrets {
			user := &store.User{
				Username:     u.Username,
				PasswordHash: pwByUser[u.Username],
				Role:         u.Role,
				DisplayName:  u.DisplayName,
				Email:        u.Email,
			}
			if err := s.store.UpsertUserByUsername(user); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: upsert user %q: %w", u.Username, err)
			}
		} else {
			// DR-safe: do not touch password_hash.
			if err := s.store.UpdateUserFromRedacted(u.Username, u.Role, u.DisplayName, u.Email); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: upsert user (redacted) %q: %w", u.Username, err)
			}
		}
	}
	existingUsers, err := s.store.ListUsers()
	if err != nil {
		return fmt.Errorf("ApplyGlobalSidecar: list users: %w", err)
	}
	for _, u := range existingUsers {
		if _, keep := wantUsers[u.Username]; !keep {
			if err := s.store.DeleteUser(u.ID); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: delete user %q: %w", u.Username, err)
			}
		}
	}

	// --- API keys: replace per (username, name). Without secrets file we
	// cannot upsert (key_hash is required); skip api_keys entirely to avoid
	// wiping working keys during DR.
	if hasSecrets {
		// Build wanted set (username|name) and upsert each.
		wantKeys := make(map[string]struct{}, len(data.APIKeys))
		for _, k := range data.APIKeys {
			wantKeys[k.Username+"|"+k.Name] = struct{}{}
			hash := keyHashByUserName[k.Username+"|"+k.Name]
			if err := s.store.UpsertAPIKey(k.Username, hash, k.Name, k.ExpiresAt); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: upsert api key %q/%q: %w", k.Username, k.Name, err)
			}
		}
		// Delete keys absent from sidecar.
		users, err := s.store.ListUsers()
		if err != nil {
			return fmt.Errorf("ApplyGlobalSidecar: list users for api keys: %w", err)
		}
		for _, u := range users {
			keys, err := s.store.ListAPIKeysByUser(u.ID)
			if err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: list api keys: %w", err)
			}
			for _, k := range keys {
				if _, keep := wantKeys[u.Username+"|"+k.Name]; keep {
					continue
				}
				if err := s.store.DeleteAPIKey(k.ID, 0); err != nil {
					return fmt.Errorf("ApplyGlobalSidecar: delete api key %d: %w", k.ID, err)
				}
			}
		}
	}

	// --- Registries: replace.
	wantRegs := make(map[string]struct{}, len(data.Registries))
	for _, r := range data.Registries {
		wantRegs[r.ID] = struct{}{}
		if hasSecrets {
			sec := regSecByID[r.ID]
			reg := &store.Registry{
				ID: r.ID, Name: r.Name, URL: r.URL,
				UsernameEnc: sec.UsernameEnc, PasswordEnc: sec.PasswordEnc,
			}
			if err := s.store.UpsertRegistryByID(reg); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: upsert registry %q: %w", r.ID, err)
			}
		} else {
			if err := s.store.UpsertRegistryFromRedacted(r.ID, r.Name, r.URL); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: upsert registry (redacted) %q: %w", r.ID, err)
			}
		}
	}
	existingRegs, err := s.store.ListRegistries()
	if err != nil {
		return fmt.Errorf("ApplyGlobalSidecar: list registries: %w", err)
	}
	for _, r := range existingRegs {
		if _, keep := wantRegs[r.ID]; !keep {
			if err := s.store.DeleteRegistry(r.ID); err != nil {
				return fmt.Errorf("ApplyGlobalSidecar: delete registry %q: %w", r.ID, err)
			}
		}
	}

	// --- DB backup config: upsert single logical row.
	for k, v := range data.DBBackupConfig {
		if err := s.store.SetDBBackupConfig(k, v); err != nil {
			return fmt.Errorf("ApplyGlobalSidecar: set db_backup_config %q: %w", k, err)
		}
	}
	if hasSecrets && dbBackupEnc != "" {
		if err := s.store.SetDBBackupConfig("target_config_enc", dbBackupEnc); err != nil {
			return fmt.Errorf("ApplyGlobalSidecar: set db_backup_config target_config_enc: %w", err)
		}
	}

	return nil
}
