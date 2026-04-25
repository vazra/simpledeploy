---
title: Activity & Audit Log
description: Per-app and global activity feed tracking every config change, deploy outcome, auth event, and system action in SimpleDeploy.
---

import { Aside } from '@astrojs/starlight/components';

The activity log is a persistent, queryable record of everything that changes in SimpleDeploy: who did what, when, and for which app.

## Where to find it

| Surface | Location |
|---------|----------|
| Per-app feed | App detail page, **Activity** tab |
| Global feed | System → **Audit Log** |
| Recent activity | Dashboard home card |

## What gets captured

| Category | Events |
|----------|--------|
| `compose` | Services added/removed, image/env/ports/replicas/labels changed |
| `endpoint` | Endpoints added/removed, TLS settings, advanced settings |
| `backup` | Backup config created/changed/removed |
| `alert` | Alert rules created/changed/removed |
| `webhook` | Webhooks created/changed/removed |
| `registry` | Registry credentials added/removed |
| `access` | User app-access grants and revocations |
| `deploy` | Deploy succeeded, deploy failed (with error), rollback |
| `lifecycle` | App created, stopped, started, restarted, scaled, removed |
| `auth` | Login success/failure, password change (global only) |
| `system` | User CRUD, API key CRUD, public-host change, git sync config, retention settings (global only) |

Each entry shows: actor, source (UI / API / CLI / git sync / system), timestamp, and a human-readable summary. For config changes, before and after values are stored as structured JSON and visible via the expand chevron on each row.

## Sync status badges

Entries for config-change categories (`compose`, `endpoint`, `backup`, `alert`, `webhook`, `registry`, `access`) carry a sync badge when [git sync](/operations/git-sync/) is enabled:

| Badge | Meaning |
|-------|---------|
| Synced | Change committed and pushed to the git remote |
| Pending | Waiting for the next sync cycle |
| Sync failed | Commit or push failed; see git sync error details |

Runtime-only events (`deploy`, `auth`, `system`, `lifecycle` start/stop/scale) are never committed to git and show no badge.

## Retention

Default: **365 days**. Set to `0` to keep entries forever.

Super-admins can adjust retention on the System → Audit Log page or via the API:

```bash
curl -X PUT https://manage.example.com/api/system/audit-config \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"retention_days": 180}'
```

A background pruner runs nightly and removes entries older than the configured threshold.

## Super-admin capabilities

Super-admins can:

- Adjust retention days (UI or API).
- Purge all entries via the **Purge activity log** button on System → Audit Log (or `DELETE /api/activity`).

## Authorization

| Role | Visible entries |
|------|----------------|
| Super-admin / Admin | All entries across all apps and system events |
| Non-admin | Entries for apps they have access to, plus their own `auth` events |

## Privacy

Registry credentials are **never** stored in audit JSON. Before/after snapshots for `registry` entries record only the registry name and host, not the password or token.

## API endpoints

```
GET  /api/apps/{slug}/activity   # per-app feed, cursor-paginated
GET  /api/activity               # global feed
GET  /api/activity/recent        # dashboard mini-feed (8 entries)
GET  /api/activity/{id}          # single entry with full before/after JSON
GET  /api/system/audit-config    # current retention setting (super-admin)
PUT  /api/system/audit-config    # update retention (super-admin)
DELETE /api/activity             # purge all entries (super-admin)
```

Query params for list endpoints: `categories=compose,deploy`, `app=<slug>`, `limit=50`, `before=<id>` (cursor).

<!-- TODO: screenshot of Activity tab -->

## See also

- [Git sync](/operations/git-sync/) - how config changes flow to a git remote.
- [Security hardening](/operations/security-hardening/) - login rate limits, account lockout, and other controls that generate auth events.
