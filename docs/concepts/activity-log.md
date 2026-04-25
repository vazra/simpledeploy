---
title: Activity log
description: How SimpleDeploy captures and stores every config change, deploy outcome, and system action as a persistent, queryable feed.
---

The activity log is a persistent SQLite-backed record of every meaningful action in SimpleDeploy. It replaces the old in-memory ring buffer.

## One stream, three surfaces

Every event writes to the same table and appears in three places:

- **Per-app Activity tab** -- config and deploy history scoped to one app.
- **System → Audit Log** -- all events across all apps plus system-level actions (user CRUD, auth, git sync config changes).
- **Dashboard recent-activity card** -- last few events across apps you can access.

## Entries

Each entry stores: actor, source (UI / API / CLI / git sync / system), category, action, human-readable summary, and optional structured before/after JSON for config changes. The before/after payload is lazy-loaded only when you expand a row, keeping list views fast.

## Sync eligibility

Entries for config-change categories are marked with a sync status when [git sync](/operations/git-sync/) is enabled. The git sync worker stamps each relevant entry with `synced`, `pending`, or `failed` after the push completes. Runtime events (deploys, auth, start/stop) are not git-tracked and carry no badge.

## Retention

Configurable per deployment (default 365 days, `0` = forever). Pruned nightly. See [Activity & Audit Log](/operations/security-audit/) for how to adjust it.
