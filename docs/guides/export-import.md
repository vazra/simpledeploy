---
title: Export and Import App Config
description: Export an app's config as a portable bundle and re-import it on the same or another SimpleDeploy instance.
---

Every SimpleDeploy app can be exported as a single ZIP bundle that captures the compose file, alerts, backup configs, and access settings. You can re-import the bundle on the same instance (to clone or restore) or on a different one (to migrate).

## When to use it

- Migrating an app to a new SimpleDeploy server.
- Cloning an app to a staging slug for testing.
- Sharing your stack with a teammate or the community.
- Keeping an offline backup of an app's config (separate from data backups).

This is for app **configuration**. Application data (database volumes, uploaded files) is handled by [Backups](backups/).

## What's included

The exported ZIP contains:

| File | Purpose |
| --- | --- |
| `docker-compose.yml` | Recipe-compatible compose. Deployable standalone via "Deploy from compose". |
| `simpledeploy.yml` | Alert rules, backup configs, access list. Non-secret settings only. |
| `env.example` | Every env key referenced by the app, with values blanked. |
| `manifest.json` | `schema_version`, `exported_at`, `source_simpledeploy_version`, `app`, redaction notes. |

## What's redacted

For safety, the bundle never contains:

- `.env` values — only the keys, in `env.example`.
- `simpledeploy.secrets.yml` — encrypted with the source instance's `master_secret`, which is not portable.
- Registry credentials and S3 backup credentials — those live in instance-level config.

You re-enter those on the target instance after import.

## Export an app

1. Open the app, go to the **Settings** tab.
2. Click **Export config**.
3. Browser downloads `<slug>-export.zip`.

CLI/API: `GET /api/apps/{slug}/export` returns the ZIP.

## Import an app

1. Open the **Deploy** wizard.
2. Pick the **Import from file** tile.
3. Choose the bundle ZIP.
4. Pick a mode and slug:

   - **New app**: creates a fresh app at the slug you enter. The slug must not already exist. `.env` is created with the keys from `env.example` and empty values — fill them in before deploying.
   - **Overwrite existing**: replaces the compose, alerts, backup configs, and access for an existing app at the given slug. The on-disk `.env` and `simpledeploy.secrets.yml` are **preserved**, so secrets you already set stay intact.

5. Click **Import**.
   - **New app**: imports immediately. The app appears in the dashboard, stopped.
   - **Overwrite existing**: a confirmation panel shows what will change (compose changed/unchanged, sidecar changed/unchanged, alert rule and backup config counts before -> after). Click **Confirm overwrite** to apply, or **Back** to adjust. Nothing on disk changes until you confirm.
6. Review env values, then deploy.

CLI/API: `POST /api/apps/import` (multipart: `file=@bundle.zip`, `mode=new|overwrite`, `slug=myapp`). For a dry-run diff before applying, `POST /api/apps/import/preview` with the same fields returns the current vs incoming compose/sidecar plus a `changes` summary.

## Recipe compatibility

The `docker-compose.yml` inside the bundle is a self-contained, recipe-compatible compose file. You can:

- Extract it and paste into **Deploy from compose** on any SimpleDeploy instance.
- Submit it to the [community recipes catalog](community-recipes/).
- Use it as a starting point outside SimpleDeploy with plain `docker compose up`.

The non-compose pieces (alerts, backups, access) only apply when imported via the wizard.

## FAQ

**Do I need to re-enter env values after import?**
For "new app" imports, yes — `.env` ships with empty values. For "overwrite" imports, no — your existing `.env` on disk is kept.

**What about S3 or registry credentials?**
Those are instance-level, not per-app, so they aren't in the bundle. Configure them once on the target instance under Settings.

**Can I import a bundle exported from an older SimpleDeploy version?**
The `manifest.json` records the source version and a `schema_version`. Forward compatibility is best-effort; very old bundles may need manual edits.

**Does export include app data (DB rows, uploads)?**
No. Use [Backups](backups/) for data. Export covers config only.

**Is it safe to share a bundle publicly?**
Yes — secrets and env values are stripped. Double-check `docker-compose.yml` for any inline values you may have hardcoded.
