---
title: Git sync
description: How to keep your SimpleDeploy app configs in a git repository, synced automatically on every change.
---

Git sync is optional and disabled by default.

When enabled, SimpleDeploy treats your `apps_dir` as a git working tree and commits every config change to a remote repository. Each deploy, env-var edit, or sidecar update triggers a commit and push within seconds. You can also pull remote changes back in, making it possible to manage deployments through git rather than (or alongside) the UI.

Git sync complements [config sidecars](./config-sidecars), which write app config to local YAML files. Think of sidecars as the local source of truth and git sync as the transport layer that makes that truth portable. Sidecars are what get committed; git sync is what moves them to a remote.

Git sync is **not** a replacement for database backup. It does not capture metrics, deploy history, audit logs, or any file outside `apps_dir`. Secrets stay local: `config.yml` (which holds password hashes and encrypted registry credentials) and the SQLite database are never committed.

## What gets committed

- `{apps_dir}/{slug}/docker-compose.yml`
- `{apps_dir}/{slug}/.env`
- `{apps_dir}/{slug}/simpledeploy.yml` (per-app sidecar: alert rules, backup configs, access)
- `{apps_dir}/_global.yml` (redacted global config: users, registries, webhooks without secrets)
- `.gitignore` (auto-generated allowlist that restricts commits to the above files)

Never committed:

- `{data_dir}/config.yml` (password hashes, encrypted credentials)
- `{data_dir}/simpledeploy.db` (SQLite database)
- Metrics, logs, and anything outside `apps_dir`

## Configure from the UI

Super admins can enable and edit git sync configuration directly on the **Git Sync** page without a server restart. Changes saved through the UI are written to the database and take effect immediately. DB values override the `config.yml` YAML block, and the API response includes a `source` field (`"db"` or `"yaml"`) so you can tell which value is active. To reset a field back to the YAML default, remove it from the DB via the UI.

## Config block

Add a `git_sync:` block to your server `config.yml`:

```yaml
git_sync:
  enabled: true
  remote: git@github.com:owner/infra.git
  branch: main
  author_name: SimpleDeploy
  author_email: bot@example.com
  ssh_key_path: /etc/simpledeploy/gitsync_id_ed25519
  poll_interval: 60s
  webhook_secret: "your-webhook-secret"
  poll_enabled: true
  auto_push_enabled: true
  auto_apply_enabled: true
  webhook_enabled: true
```

| Field | Required | Description |
|---|---|---|
| `enabled` | yes | Set to `true` to start the sync worker. |
| `remote` | yes | Git remote URL: `https://`, `http://`, `ssh://`, `git://`, `file://`, an absolute local path, or `user@host:path`. Transport helpers (`ext::`, `fd::`) and values starting with `-` are rejected. |
| `branch` | yes | Branch to push to and pull from. |
| `author_name` | no | Commit author name. Defaults to `SimpleDeploy`. |
| `author_email` | no | Commit author email. |
| `ssh_key_path` | one of | Path to an SSH private key. Mutually exclusive with `https_token`. |
| `https_username` | one of | HTTPS username. Used together with `https_token`. |
| `https_token` | one of | HTTPS token or password. Mutually exclusive with `ssh_key_path`. |
| `poll_interval` | no | How often to pull from remote. Default `60s`. |
| `webhook_secret` | no | HMAC secret for verifying GitHub-compatible webhook pushes. |
| `poll_enabled` | no | Whether to run the poll loop. Default `true`. |
| `auto_push_enabled` | no | Whether to push local changes to the remote. When `false` (pull-only), changes are still committed locally. Default `true`. |
| `auto_apply_enabled` | no | Whether to auto-apply fetched remote changes. Default `true`. |
| `webhook_enabled` | no | Whether the webhook endpoint is active. Default `true`. |

## Toggles

Four behaviour toggles let you adjust the sync mode without disabling git sync entirely. All default to `true`. They can be set in `config.yml` or changed at runtime from the UI (DB overrides YAML).

### `poll_enabled`

Controls whether the background poll loop runs on `poll_interval`. Set to `false` if you rely exclusively on webhooks and want no background polling. The sync worker still starts; it just never fires the timer. Useful when you want tight control over when fetches happen.

### `auto_push_enabled`

Controls whether local config changes (deploys, env edits, sidecar updates) are pushed to the remote. Set to `false` for a pull-only setup where the remote is the source of truth and local changes are never pushed back. This is useful when you manage config entirely through the git repo and want to prevent the server from writing back.

In pull-only mode, dashboard changes are still committed to the local repository in `apps_dir`, with the same files and bot commit message as auto-push, but the commits are not pushed. Before applying remote commits, SimpleDeploy also commits any other uncommitted changes to synced files, including new app folders, so git can apply the pull. The remote commits are applied and the local commits are replayed on top of them; when both sides changed the same file, [conflict behavior](#conflict-behavior) decides which version is kept. Apart from the initial commit to an empty remote on first run, nothing is pushed in this mode. If you turn auto-push back on later, the next push includes these local commits.

A pull can still be blocked by changes git sync does not commit itself, such as edits to other files tracked in the repository, or symlinks at synced paths. The Git Sync page then shows a `rebase refused` error with git's output. Commit or remove those changes, then sync again. With auto-apply on, each poll also retries the pull.

### `auto_apply_enabled`

Controls whether fetched remote commits are automatically rebased onto the local working tree. Set to `false` to enter fetch-only mode: the server fetches on each poll or webhook trigger and tracks how many commits the remote is ahead, but it does not apply those commits. A banner appears in the UI showing the number of commits behind, and you can apply them manually with the **Apply** button (or via `POST /api/git/apply-pending`). Useful when you want to review remote changes before they affect running deployments.

### `webhook_enabled`

Controls whether `POST /api/git/webhook` accepts and processes incoming webhook payloads. Set to `false` to temporarily block webhook-triggered syncs, for example during maintenance windows. When disabled, the endpoint returns `404` with an empty body. The HMAC secret is not exposed in the response.

## Pending apply

When `auto_apply_enabled` is `false`, each fetch (poll or webhook) updates an internal counter of how many commits the remote is ahead of the local HEAD. The Git Sync page shows a banner:

> **3 commits behind remote.** Review the changes in your git repository, then click Apply to update running deployments.

Clicking **Apply now** (or calling `POST /api/git/apply-pending`) runs the full fetch, rebase with server-wins conflict resolution, sidecar import, and reconcile cycle. After a successful apply, the counter resets to zero and the banner disappears.

If the remote is already up-to-date when apply is triggered, the operation completes immediately with no changes.

## Authentication

**SSH:** Set `ssh_key_path` to an Ed25519 or RSA private key on the server. Add the corresponding public key as a deploy key on the remote (GitHub: Settings > Deploy keys, with write access).

**HTTPS:** Set `https_username` and `https_token`. For GitHub, create a fine-grained personal access token with `Contents: Read and write` permission on the target repository.

## First run and adopting existing state

If the remote repository is empty, SimpleDeploy initializes a repo in `apps_dir`, commits current state, and pushes. This is the recommended starting point. The Git Sync config form detects this case: testing the connection shows an info banner, and the save button changes to **Save & push initial commit**, making it explicit that saving will push your current SimpleDeploy configs as the initial commit.

If the remote already has commits, SimpleDeploy refuses to push and surfaces an error in `git status` and on the Git Sync page in admin nav. You then have two options:

- **Adopt local state:** from an admin shell, `git push --force` from `apps_dir` to overwrite the remote with current local state.
- **Adopt remote state:** manually clone the remote, move the files into `apps_dir`, and restart the server so sidecars are imported.

Start with an empty remote whenever possible to avoid this decision.

<Aside type="caution">
Force-pushing overwrites remote history. Only do it when you are certain local state is the source of truth.
</Aside>

## Webhook setup (optional but recommended)

A webhook lets the remote trigger an immediate pull instead of waiting for the next poll.

**GitHub:**
1. Repository Settings > Webhooks > Add webhook.
2. Payload URL: `https://<your-server>/api/git/webhook`
3. Content type: `application/json`
4. Secret: the value of `webhook_secret` in your config.
5. Event: "Just the push event."

SimpleDeploy verifies the `X-Hub-Signature-256` header using your secret. Gitea and GitLab use the same header format and are also supported.

## Poll and webhook coexistence

The poll worker runs on `poll_interval` (default 60s) regardless of webhook configuration. When a webhook arrives, an immediate sync runs; the poll continues as a safety net. There is no harm in running both.

## What a pull can and cannot change

Anyone who can push to the remote can change what your apps run, so pulled
content is treated as less trusted than the dashboard:

- **Compose files** are checked with the same [security validation](/reference/compose-labels/#compose-security-validation) as a dashboard deploy before anything starts.
- **Per-app sidecars** (`simpledeploy.yml`) apply alert rules and backup configs, but never change who can access an app. If a pulled sidecar edits its `access` list, SimpleDeploy reverts that list in git with a bot commit and shows a conflict entry. While git sync is on, hand edits to the `access` list on disk are ignored too. Grant or revoke access from the dashboard.
- **`_global.yml` is push-only.** Users, roles, registries, webhooks and DB backup settings never change because of a pull; remote edits to this file are ignored and overwritten the next time global settings change.
- **Symlinks** are never followed. The repo is checked out with `core.symlinks=false`, so a symlink committed to the repo lands as a plain file, and a symlinked compose file or sidecar is never deployed or read. If the repo tracks a symlink, or an app the pull touched has a symlinked folder or managed file, the Git Sync page shows an error naming the paths and the sync skips importing that app's `simpledeploy.yml` settings. The rest of the pull still applies: other apps' settings are imported and changed apps are reconciled. The blocked app's pulled files stay checked out, so its regular (non-symlink) files are still picked up by the normal file watcher; its access grants stay as set in the dashboard. A symlinked `_global.yml` or `.gitignore` at the repo root skips the settings import and reconcile for the whole pull. Local symlinks git does not track, in apps the pull did not touch, are left alone.
- **Branch names** must be plain (letters, digits, `.`, `_`, `/`, `-`; no leading `-` or `..`).

## Conflict behavior

Local state wins on conflict. If a remote change conflicts with a local change, SimpleDeploy logs the conflict to `alert_history` and surfaces it on the Git Sync page. The remote change is not applied.

Conflicts usually mean two operators edited the same file at the same time. To apply the remote change, re-enter it through the UI after reviewing what was lost.

With `auto_push_enabled` off, local commits are never pushed, so they are replayed on every pull. A local change therefore keeps taking precedence over later remote edits to the same part of that file.

Two kinds of local bot commits never override remote edits: one that only restored access grants, and one that only holds synced files SimpleDeploy rewrote from its database after applying a pull (for example a hand-formatted `simpledeploy.yml` saved back in SimpleDeploy's own format). Both stay unpushed when `auto_push_enabled` is off. On conflict the remote file is taken (and its access list restored again), so remote changes such as alert thresholds still apply.

## CLI

```bash
simpledeploy git status      # print worker status and last sync time
simpledeploy git sync-now    # one-shot pull-and-apply against current config
```

`sync-now` is useful after a credentials change or for a manual bootstrap without restarting the server.

## Disabling git sync

Set `enabled: false` (or remove the block). The sync worker stops. The `.git` directory and all local history remain in `apps_dir` untouched. Re-enabling picks up where it left off.

## See also

- [Config sidecars and sidecar-based recovery](./config-sidecars) - sidecar schema and local DR recovery without git.
