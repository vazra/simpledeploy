---
title: Archived apps
description: How SimpleDeploy archives apps when their config directory disappears, what's preserved, and how to purge or restore them.
---

When an app's config directory disappears from `apps_dir`, SimpleDeploy does not silently forget it. The running containers are torn down, but the database row, deploy history, audit log, alert rules, backup configs, and access grants are all retained. The app moves to an **archived** state and shows up under "Archived apps" in the sidebar.

This is intentional: filesystem removal is reversible-ish (you can put the directory back or restore from git), and you almost never want to lose months of metrics, audit trail, or backup history just because someone ran `rm -rf` or git pulled a delete.

## What "archived" means

- **Containers torn down.** The compose stack is stopped and removed.
- **App directory gone.** `{apps_dir}/{slug}/` is no longer on disk.
- **DB row retained.** The `apps` row stays, with `archived_at` set.
- **History retained.** Deploy events, audit log, backup runs, alert history, and metrics rows are all kept.
- **Sidecar snapshot written.** A tombstone file is written to `{data_dir}/archive/{slug}.yml` capturing the per-app sidecar at the moment of archival.

## How to archive an app

There is no "Archive" button in the UI. Archival happens automatically when the app's directory is removed from `apps_dir`. Two common ways:

### Remove the directory directly

```bash
rm -rf /var/lib/simpledeploy/apps/myapp
```

The reconciler picks up the missing directory on its next scan, stops the stack, sets `archived_at`, and writes a tombstone. The app disappears from the main dashboard and appears under "Archived apps".

### Via git sync

If [git sync](./git-sync) is enabled and you delete the app's directory in your repo and push, the next pull will remove the directory locally. The reconciler then archives it the same way.

## Where archived apps appear

The dashboard sidebar gains an **Archived apps** entry whenever there is at least one archived app. Click it to see the list. Each entry links to a read-only detail page showing what's preserved (last config snapshot, deploy history, backup history, audit log).

Archived apps are excluded from:

- The main dashboard grid.
- App-count totals on the home page.
- Reconciler deploy/scale operations.

## What is preserved

Everything in the database tied to the app:

- `apps` row (with `archived_at` timestamp)
- `deploy_events` and `compose_versions` (full deploy history)
- `audit_log` rows for the app
- `backup_runs` and `backup_configs`
- `alert_rules` and `alert_history`
- `user_app_access` grants
- `metrics` and `request_stats` (subject to normal retention)

Containers, images, and the live filesystem state are **not** preserved. If you need those back, redeploy from a tombstone or a git-tracked compose file.

## How to purge

Purging removes the DB row and all associated history rows, then deletes the tombstone. This is irreversible.

### From the UI

Open the archived app's detail page and click **Clean up**. Confirm the dialog.

### From the API

```bash
curl -X POST -H "Authorization: Bearer $TOKEN" \
  https://your-server/api/apps/{slug}/purge
```

See the [REST API reference](/reference/api/) for details.

## Difference from UI Delete

The **Delete** action on a live app is a full purge in one step: it tears down containers, removes the directory, and wipes all DB rows. There is no archive intermediate state.

The archive flow only triggers when the directory disappears **without** going through the Delete action: a manual `rm -rf`, a git sync pulling a delete, or any other out-of-band removal. The reasoning is that intentional UI deletes are deliberate; out-of-band removals are often accidents or git-driven config moves where preserving history is the safer default.

| Path | Containers | App dir | DB row | History | Tombstone |
|------|------------|---------|--------|---------|-----------|
| UI Delete | removed | removed | removed | removed | none |
| Filesystem removal | removed | already gone | retained (`archived_at` set) | retained | written |
| Purge from archive | already gone | already gone | removed | removed | removed |

## Tombstone files

Each archived app has a tombstone at `{data_dir}/archive/{slug}.yml`. It is a snapshot of the per-app sidecar contents at the moment of archival, plus an `archived_at` timestamp.

```yaml
version: 1
archived_at: 2026-04-27T14:32:11Z
app:
  slug: myapp
  display_name: My App
  compose: |
    services:
      web:
        image: nginx:latest
alert_rules:
  - name: high-cpu
    metric: cpu_percent
    threshold: 90
backup_configs:
  - name: daily
    strategy: volume
    schedule: "0 2 * * *"
access:
  - user: alice
    role: deployer
```

Tombstones are written atomically (write to `.tmp`, then rename). They are read by the archived-app detail page to render the last-known config. Purging an app deletes its tombstone.

Tombstones live under `data_dir`, not `apps_dir`, so they are **not** committed by git sync. They are part of local server state and are included in DB backups indirectly via the data directory but not in the SQLite snapshot itself.
