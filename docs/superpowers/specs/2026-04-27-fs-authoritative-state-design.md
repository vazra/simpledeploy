# Filesystem-authoritative declarative state

Date: 2026-04-27
Status: approved (brainstorm complete, ready for plan)

## Problem

SimpleDeploy's declarative state today is split across:
- `docker-compose.yml` per app (filesystem, authoritative).
- DB rows for alert rules, backup configs, access lists, users, registries, webhooks, db backup config (DB-authoritative).
- `internal/configsync` writes per-app `simpledeploy.yml` and global `config.yml` sidecars **as a mirror** of the DB.

This blocks:
- GitOps workflows: pulling a git commit doesn't update server state, because the DB is the source of truth.
- Editor-driven workflows: hand-editing a sidecar gets overwritten on the next DB-driven flush.
- DR: rebuilding from sidecars works only via a special "import if missing" code path; routine drift between DB and FS is not auto-reconciled.

## Goals

- Per-app and global declarative state on the filesystem is the source of truth.
- DB serves as a fast read cache, populated from the filesystem on startup and on file change.
- Mutations via UI / API write the FS first, then update the DB cache, in a single logical transaction.
- Out-of-band edits (gitsync pull, manual `vim`, `git checkout`) propagate to the DB via the existing watcher mechanism.
- Secrets are stored on disk with appropriate at-rest protection and filesystem permissions.
- Hard cutover migration: first boot of upgraded binary seeds FS files from DB, flips a flag, and from then on FS wins.

## Non-goals

- Removing the DB. The DB stays for runtime state (metrics, request_stats, sessions, audit_log, deploy_events, backup_runs, alert_history, compose_hash, gitsync state, rate-limit counters, archive markers). Only **declarative configuration** moves to FS-authoritative.
- Per-resource phased rollout. Single coherent cutover, per the brainstorm decision.
- Multi-machine consensus. SimpleDeploy is single-node; this design assumes one server reading/writing one filesystem.

## Scope: which state moves to FS-authoritative

| Resource          | Current location  | New authority           | Where on disk                          |
| ----------------- | ----------------- | ----------------------- | -------------------------------------- |
| compose           | FS                | FS (unchanged)          | `<apps_dir>/<slug>/docker-compose.yml` |
| app meta (display name) | DB          | FS                      | `<apps_dir>/<slug>/simpledeploy.yml`   |
| alert_rules       | DB                | FS                      | `<apps_dir>/<slug>/simpledeploy.yml`   |
| backup_configs (non-secret fields) | DB | FS                  | `<apps_dir>/<slug>/simpledeploy.yml`   |
| backup_configs (`target_config_enc`) | DB | FS                | `<apps_dir>/<slug>/simpledeploy.secrets.yml` |
| user_app_access   | DB                | FS                      | `<apps_dir>/<slug>/simpledeploy.yml`   |
| users (non-secret) | DB               | FS                      | `{data_dir}/config.yml`                |
| users (password_hash) | DB            | FS                      | `{data_dir}/secrets.yml`               |
| api_keys (key_hash) | DB              | FS                      | `{data_dir}/secrets.yml`               |
| registries (id, name, url) | DB       | FS                      | `{data_dir}/config.yml`                |
| registries (encrypted creds) | DB     | FS                      | `{data_dir}/secrets.yml`               |
| webhooks (name, type) | DB            | FS                      | `{data_dir}/config.yml`                |
| webhooks (url, headers, template) | DB | FS                    | `{data_dir}/secrets.yml`               |
| db_backup_config (schedule, target type) | DB | FS                | `{data_dir}/config.yml`                |
| db_backup_config (encrypted target_config) | DB | FS              | `{data_dir}/secrets.yml`               |
| compose_hash      | DB                | DB (derived/cache)      | n/a                                    |
| metrics, request_stats, audit_log, deploy_events, backup_runs, alert_history, compose_versions, gitsync state, rate-limit | DB | DB (runtime state) | n/a |
| `master_secret`, bootstrap config | `/etc/simpledeploy/config.yaml` | unchanged | not in FS-authoritative state |

## File layout

### Per-app

```
<apps_dir>/<slug>/
  docker-compose.yml          # mode 0644, git-tracked
  simpledeploy.yml            # mode 0644, git-tracked, declarative non-secret app state
  simpledeploy.secrets.yml    # mode 0600, gitignored, encrypted blobs only
```

### Global

```
{data_dir}/
  config.yml                  # mode 0644, git-tracked, declarative non-secret global state
  secrets.yml                 # mode 0600, gitignored, encrypted/hashed material
  archive/<slug>.yml          # see archive spec
  simpledeploy.db             # DB cache (regenerable from FS)
```

### Schemas

`simpledeploy.yml` extends today's `internal/configsync/types.go AppSidecar` with no breaking changes. `simpledeploy.secrets.yml` is a new type:

```go
type AppSecrets struct {
    Version       int                     `yaml:"version"`
    Slug          string                  `yaml:"slug"`
    BackupConfigs []BackupSecretsEntry    `yaml:"backup_configs,omitempty"`
}
type BackupSecretsEntry struct {
    Strategy        string `yaml:"strategy"`
    Target          string `yaml:"target"`
    TargetConfigEnc string `yaml:"target_config_enc"`  // ciphertext, master_secret-encrypted
}
```

The `BackupConfigs` correlation key between `simpledeploy.yml` and `simpledeploy.secrets.yml` is `(strategy, target)` plus a stable hash of the schedule cron — sufficient for one-target-per-strategy, which matches today's UI constraint. (See open questions below.)

`config.yml` extends today's `GlobalSidecar` schema, but with secret fields removed. `secrets.yml` collects them:

```go
type GlobalSecrets struct {
    Version    int                       `yaml:"version"`
    Users      []UserSecretsEntry        `yaml:"users,omitempty"`
    APIKeys    []APIKeySecretsEntry      `yaml:"api_keys,omitempty"`
    Registries []RegistrySecretsEntry    `yaml:"registries,omitempty"`
    Webhooks   []WebhookSecretsEntry     `yaml:"webhooks,omitempty"`
    DBBackup   *DBBackupSecretsEntry     `yaml:"db_backup,omitempty"`
}
type UserSecretsEntry struct {
    Username     string `yaml:"username"`
    PasswordHash string `yaml:"password_hash"`
}
type APIKeySecretsEntry struct {
    KeyHash   string     `yaml:"key_hash"`
    Username  string     `yaml:"username"`
    Name      string     `yaml:"name"`
    ExpiresAt *time.Time `yaml:"expires_at,omitempty"`
}
type RegistrySecretsEntry struct {
    ID          string `yaml:"id"`
    UsernameEnc string `yaml:"username_enc"`
    PasswordEnc string `yaml:"password_enc"`
}
type WebhookSecretsEntry struct {
    Name             string `yaml:"name"`
    URL              string `yaml:"url"`
    HeadersJSON      string `yaml:"headers_json,omitempty"`
    TemplateOverride string `yaml:"template_override,omitempty"`
}
type DBBackupSecretsEntry struct {
    TargetConfigEnc string `yaml:"target_config_enc"`
}
```

### Secret material classification

| Field                    | At-rest treatment                          |
| ------------------------ | ------------------------------------------ |
| `password_hash`          | bcrypt hash, stored in `secrets.yml` (0600). Not encrypted (already irreversible). |
| `api_key.key_hash`       | sha256 hash, stored in `secrets.yml` (0600). Not encrypted. |
| `registry.username_enc`, `registry.password_enc` | `auth.Encrypt` ciphertext, stored in `secrets.yml`. |
| `webhook.url`, `webhook.headers_json`, `webhook.template_override` | plaintext in `secrets.yml` (0600). Same trust boundary as today's DB. |
| `backup_config.target_config_enc`, `db_backup.target_config_enc` | `auth.Encrypt` ciphertext, stored in respective `*secrets.yml`. |

`master_secret` is the encryption key for the `*_enc` fields. It lives in `/etc/simpledeploy/config.yaml` (or `SIMPLEDEPLOY_MASTER_SECRET` env), unchanged.

## Authority & runtime model

### Reads

All reads serve from the DB. No change.

### Writes (via API / UI)

Pseudo-flow for any mutation that affects FS-authoritative state:

```
1. Build new desired state in memory.
2. Write affected FS files atomically (write to .tmp, fsync, rename). Set perms (0644 / 0600) explicitly.
3. Begin DB transaction.
4. Apply DB upserts/deletes to match the new desired state.
5. Commit DB transaction.
6. Audit-log the mutation.
```

If step 2 fails: abort with 5xx, no DB change. If step 5 fails: log inconsistency (FS ahead of DB), trigger a watcher-style reload to bring DB back in line with FS.

This is "FS-first, DB cache" semantics: FS always reflects the latest committed intent, DB may briefly trail on crash and self-heals.

### Watcher-driven reload

Existing `internal/reconciler/watcher.go` watches `apps_dir` for compose changes. Extend to also watch:

- `<apps_dir>/<slug>/simpledeploy.yml`
- `<apps_dir>/<slug>/simpledeploy.secrets.yml`
- `{data_dir}/config.yml`
- `{data_dir}/secrets.yml`

On debounced change events:
- Per-app file changed → re-parse, diff against DB rows for that slug, apply DB updates inside a tx. Fire reconcile if hash-relevant fields changed (already handled for compose).
- Global file changed → re-parse, diff against DB global rows, apply DB updates inside a tx.

FS always wins on reload. If a runtime DB-only field (e.g., a counter) gets clobbered, that's fine because FS-authoritative scope excludes runtime state.

### Startup

On every server boot:
1. Load FS files (per-app + global).
2. For each FS file, reconcile DB rows (insert/update/delete to match FS exactly within FS-authoritative scope).
3. Existing reconciler proceeds with deploys/teardowns based on compose state.

Step 2 is idempotent — skipped if the parsed FS state matches the DB cache.

## Migration & cutover

Hard cutover, single migration:

### Migration #22

```sql
-- 022_fs_authoritative_seeded.sql
ALTER TABLE system_meta ADD COLUMN fs_authoritative_seeded_at TIMESTAMP NULL;
-- (system_meta table is created here if it does not yet exist)
```

(If a system_meta-equivalent table exists, reuse it. Otherwise create a 1-row settings table.)

### First boot after upgrade

Inside server bootstrap, before reconciler starts and before the API serves requests:

```
if system_meta.fs_authoritative_seeded_at IS NULL:
    for each app row in DB:
        write <apps_dir>/<slug>/simpledeploy.yml from DB (if missing, or if existing file is older than DB row's updated_at)
        write <apps_dir>/<slug>/simpledeploy.secrets.yml from DB (same rule)
    write {data_dir}/config.yml and {data_dir}/secrets.yml from DB (same rule)
    set system_meta.fs_authoritative_seeded_at = now()
    log: "FS-authoritative seed complete; FS is now the source of truth"
```

After this point, every boot runs the FS→DB reconcile in step 2 of startup.

### Rollback

If a release needs to be reverted, the prior binary still reads its DB. The new FS files remain on disk and are ignored by the older binary (today's configsync only mirrors). The user loses any FS-only edits made under the new binary, but no data corruption occurs.

## .gitignore template

On first boot, if `<apps_dir>/.gitignore` is missing or doesn't already cover `*.secrets.yml`, append:

```
# simpledeploy: never commit secrets
*.secrets.yml
secrets.yml
```

Same for `{data_dir}/.gitignore`. Idempotent: if already present, skip. Documented in the operations doc.

## Documentation

New file: `docs/operations/state-on-disk.md`. Sections:

1. **File layout** — diagram of per-app and global directories with what each file contains.
2. **What's authoritative** — table of resources × authority (FS vs DB), with the same rows as the scope table above.
3. **Secrets boundary** — what's encrypted, what's hashed, what's plaintext-with-perms; what `master_secret` protects; threat model (filesystem read access ≈ DB read access).
4. **Editing files by hand** — workflow: stop server / edit / start; or edit live and let watcher pick it up; perms expectations.
5. **GitOps with gitsync** — how to commit/push/pull declarative files; never commit `*.secrets.yml`; pulling rebases DB cache.
6. **Recovery from corrupted DB** — delete `simpledeploy.db`, restart, server rebuilds DB from FS. Caveat: runtime state (metrics, audit_log, deploy history) is lost.
7. **Troubleshooting** — what happens if FS and DB disagree, where to find logs, how to force a reload.

Linked from the Starlight sidebar in `docs-site/astro.config.mjs` under "Operations."

## Code changes (summary)

- `internal/configsync/`: extend `Syncer` to be the FS-authority engine. New methods `LoadAppFromFS(slug)`, `LoadGlobalFromFS()`, `ReconcileDBFromFS()`. Existing `WriteAppSidecar` / `WriteGlobalSidecar` repurposed as the FS-write step in the FS-first write flow.
- `internal/store/`: add transactional helpers that accept a parsed FS state and apply diffs (`ApplyAppSidecar(slug, sidecar, secrets)`, `ApplyGlobalSidecar(global, secrets)`).
- `internal/api/`: every mutating handler that touches FS-authoritative state switches to the FS-first pattern. Handlers grouped by resource: alerts, backups, access, users, registries, webhooks, db_backup.
- `internal/reconciler/watcher.go`: extend file glob list, route events to configsync reload functions.
- `cmd/simpledeploy/main.go`: bootstrap order — open DB, run migrations, run FS→DB seed (if migration #22 is fresh), then run startup FS→DB reconcile, then start reconciler/API.

## Tests

- configsync: `TestFSAuthoritativeSeedFirstBoot`, `TestFSAuthoritativeReloadOnChange`, `TestFSWriteFailureAbortsDBWrite`, `TestSecretsFilePermissions` (verifies 0600).
- Migration: fresh install + simulated existing-DB upgrade.
- API: per-resource tests verify FS file is updated before DB on every mutation.
- E2E: edit `simpledeploy.yml` by hand in the running server's apps dir, wait for watcher debounce, assert UI reflects new alert rule. Edit `secrets.yml` to add a webhook URL, assert reflected.
- Vitest: no UI changes for this design (UI continues reading from DB-backed APIs).

## Forward-compat

- Archive spec: tombstones live in `{data_dir}/archive/`. Startup FS→DB reconcile detects tombstones and ensures DB rows have `archived_at` set accordingly.
- Future: external secret managers (Vault, OS keychain) plug in by replacing the `secrets.yml` reader/writer with a different backend behind the same interface. Out of scope for v1.

## Risks

- **Performance.** Watcher fires on every save; debounce must be tight enough to feel responsive but loose enough to avoid thrash. Reuse existing reconciler debounce (~1s).
- **Atomicity.** Multi-file mutations (e.g., adding a backup config that touches both `simpledeploy.yml` and `simpledeploy.secrets.yml`) are not atomic across files. Mitigation: write the secrets file first, then the declarative file; readers tolerate a missing secrets entry by failing closed (treat as encrypted-blob-not-yet-available, surface a clear error in UI). Document in operations doc.
- **Editor vs. server race.** User edits a file while server is mid-write. Mitigation: atomic rename means readers always see a complete file; if the user's editor uses atomic save (most do), the watcher picks up exactly one event per save.
- **Secrets in process memory.** Same as today; out of scope.

## Open questions

- **Backup config correlation key.** Spec proposes `(strategy, target, schedule_hash)` to link `simpledeploy.yml` entries to `simpledeploy.secrets.yml` entries. If the UI ever allows multiple backup configs with the same strategy+target+cron, this breaks. Today's data does not, but the implementation plan should add a UNIQUE constraint or generate a stable per-config UUID stored in both files. Decide during plan.
- **Watcher coverage on macOS Docker Desktop dev.** The dev-docker compose file bind-mounts `${HOME}/.simpledeploy-local-dev`. fsnotify on bind mounts behaves correctly for writes from within the container; cross-host edits may be missed. Acceptable for prod (Linux native), document the dev caveat.
- **gitsync interaction.** Today gitsync pushes the redacted global sidecar. With FS-authoritative `config.yml` already non-secret, the redacted sidecar may become redundant. Plan should reconcile.
