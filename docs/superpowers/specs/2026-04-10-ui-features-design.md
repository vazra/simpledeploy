# UI Features: Log Viewer, Env Editor, Domain Assignment, Deploy Cancel

## Feature 1: Per-Service Log Viewer

**Scope:** UI only. Backend already supports per-service log streaming.

**Existing backend:**
- `GET /api/apps/{slug}/services` returns service list
- WebSocket `GET /api/apps/{slug}/logs?service=X&follow=true&tail=100` streams logs

**UI changes (AppDetail.svelte):**
- Fetch services list on mount
- Dropdown/tab selector for service name
- On service change, close existing WebSocket, open new one with `?service=selected`
- Default to first service alphabetically

---

## Feature 2: Env Var Editor via `.env` File

**Approach:** Manage a `.env` file alongside the compose file. Docker Compose auto-loads it. Users reference vars with `${VAR}` in compose.

**API endpoints:**
- `GET /api/apps/{slug}/env` - Read `.env` from app's compose directory, return as key-value pairs JSON
- `PUT /api/apps/{slug}/env` - Write key-value pairs to `.env` file (full overwrite)

**Backend logic:**
- App's compose directory path already known from reconciler/config
- Parse: `KEY=VALUE` lines, skip comments/blanks
- Write: serialize map to `KEY=VALUE` format, no comment preservation
- No auto-redeploy on save

**UI (AppDetail.svelte):**
- "Environment" tab/section
- Table of key-value rows with add/remove buttons
- Save button calls `PUT /api/apps/{slug}/env`
- Show/hide toggle for masking secret values

---

## Feature 3: Domain Assignment via Compose Label Rewriting

**Approach:** Directly rewrite the `simpledeploy.domain` label in the compose YAML file.

**API endpoint:**
- `PUT /api/apps/{slug}/domain` - Accepts `{ "domain": "example.com" }`, rewrites compose label

**Backend logic:**
- Use `gopkg.in/yaml.v3` node-level manipulation to preserve formatting/comments
- Find service with `simpledeploy.domain` label (or first service if none)
- Update/add the label value
- Write modified YAML back to disk
- Trigger reconcile after write (re-parse + update proxy routes), no full redeploy needed

**UI (AppDetail.svelte):**
- Domain text input in app settings/header area
- Save button calls `PUT /api/apps/{slug}/domain`
- Shows current domain from existing app data

---

## Feature 4: Deploy Cancellation

**Approach:** Track in-flight deploys via cancellable contexts. Hard cancel + reconcile with `docker compose up -d`.

**Deployer changes (internal/deployer/deployer.go):**
- Add `inFlight sync.Map` storing `slug -> context.CancelFunc`
- Wrap deploy/pull/restart: create cancellable context, store cancel func, remove on completion
- `Cancel(slug string) error` method: look up and call cancel func
- After cancellation, run `docker compose up -d` to reconcile container state

**API:**
- `POST /api/apps/{slug}/cancel` - Calls `deployer.Cancel(slug)`. 404 if no in-flight deploy, 200 on success.
- Extend `GET /api/apps/{slug}` response with `deploying bool` field

**UI (AppDetail.svelte):**
- Show "Cancel" button when `deploying == true`
- On click, call `POST /api/apps/{slug}/cancel`
- On success, refresh app state

---

## Cross-cutting Notes

- All new API endpoints require auth (existing middleware)
- No new DB migrations needed (env vars in files, domain in compose, deploy state in memory)
- Feature 3 (compose rewriting) uses `gopkg.in/yaml.v3` for safe round-tripping, not `compose-go` serialization
