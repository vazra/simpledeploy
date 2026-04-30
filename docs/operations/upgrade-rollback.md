---
title: Upgrade and rollback
description: Safely upgrade SimpleDeploy. Rollback procedures if a release breaks something in your environment.
---

SimpleDeploy is a single binary with embedded UI. Upgrades replace the binary and run any pending DB migrations on startup.

<Aside type="caution">
Always back up the SQLite database before upgrading. Migrations are forward-only; rolling back a schema change requires restoring the pre-upgrade DB.
</Aside>

## Pre-upgrade

1. Read the [CHANGELOG](https://github.com/vazra/simpledeploy/blob/main/CHANGELOG.md) for the target version. Look for `BREAKING CHANGE` markers.
2. Test the upgrade in staging first if you have one.
3. Pick a maintenance window. The service is down for ~5 seconds during binary swap.

## Step 1: Back up the DB

```bash
# Triggered backup via API
curl -X POST -H "Authorization: Bearer $SD_API_KEY" \
  https://manage.example.com/api/system/db-backup/run

# Or copy the file directly (WAL-safe with VACUUM INTO)
sqlite3 /var/lib/simpledeploy/simpledeploy.db \
  "VACUUM INTO '/tmp/pre-upgrade-$(date +%F).db'"
```

Verify the file exists and is non-empty before continuing.

## Step 2: Stop the service

```bash
sudo systemctl stop simpledeploy
```

Apps keep running (they are managed by Docker, not by SimpleDeploy). Caddy stops, so HTTP traffic to apps pauses until the service is back up.

## Step 3: Replace the binary

### apt (Debian/Ubuntu)

```bash
sudo apt update
sudo apt install --only-upgrade simpledeploy
```

### Homebrew (macOS, Linux)

```bash
brew update
brew upgrade simpledeploy
```

### Direct binary

```bash
curl -L https://github.com/vazra/simpledeploy/releases/download/v1.3.0/simpledeploy-linux-amd64 \
  -o /usr/local/bin/simpledeploy.new
chmod +x /usr/local/bin/simpledeploy.new
mv /usr/local/bin/simpledeploy /usr/local/bin/simpledeploy.old
mv /usr/local/bin/simpledeploy.new /usr/local/bin/simpledeploy
```

Keeping `.old` around makes rollback instant.

### Docker

```bash
cd /etc/simpledeploy
sudo docker compose pull
sudo docker compose up -d
```

The container restarts with the new image. To pin a specific version, edit `image:` in `docker-compose.yml` (e.g. `ghcr.io/vazra/simpledeploy:1.3.0`) before `up -d`.

### From source

```bash
cd /opt/simpledeploy
git fetch --tags
git checkout v1.3.0
make build
sudo install -m 755 ./bin/simpledeploy /usr/local/bin/simpledeploy
```

## Step 4: Start the service

```bash
sudo systemctl start simpledeploy
sudo systemctl status simpledeploy
```

Migrations run automatically on startup. Watch the logs:

```bash
journalctl -u simpledeploy -f
```

Look for `migration N applied` lines. Errors here mean the DB is in an inconsistent state. Restore from backup before troubleshooting.

## Step 5: Verify

```bash
simpledeploy version           # confirms new version
curl https://manage.example.com/api/health
```

Log into the UI. Check that the dashboard loads, an app detail page renders metrics, and a deploy/redeploy succeeds.

## Rollback

<Aside type="danger">
If the new version applied a migration that the old version does not understand, the old binary will refuse to start or will misbehave. You must restore the pre-upgrade DB backup as well.
</Aside>

### Step 1: Stop the service

```bash
sudo systemctl stop simpledeploy
```

### Step 2: Restore the previous binary

```bash
# If you kept .old
mv /usr/local/bin/simpledeploy /usr/local/bin/simpledeploy.bad
mv /usr/local/bin/simpledeploy.old /usr/local/bin/simpledeploy

# Or via package manager
sudo apt install simpledeploy=1.2.0
brew install simpledeploy@1.2.0

# Or via Docker (edit image: tag in /etc/simpledeploy/docker-compose.yml)
cd /etc/simpledeploy && sudo docker compose up -d
```

### Step 3: Restore the DB if schema changed

Check if migrations ran during the failed upgrade:

```bash
sqlite3 /var/lib/simpledeploy/simpledeploy.db \
  "SELECT MAX(version) FROM schema_migrations;"
```

If the number is higher than what the old binary expects, restore:

```bash
sudo systemctl stop simpledeploy
sudo cp /tmp/pre-upgrade-2026-04-15.db /var/lib/simpledeploy/simpledeploy.db
sudo chown simpledeploy:simpledeploy /var/lib/simpledeploy/simpledeploy.db
sudo chmod 0600 /var/lib/simpledeploy/simpledeploy.db
sudo systemctl start simpledeploy
```

### Step 4: Report the bug

Open a GitHub issue with the version, the error logs (`journalctl -u simpledeploy -p err --since "1 hour ago"`), and steps to reproduce.

## Config sidecars and upgrade

After upgrading to a version with config sidecar support (the first release of the feature), the first `serve` start writes a complete sidecar set from the existing DB and records `{data_dir}/.configsync_backfill_v1`. No action is needed. If you downgrade to a version without sidecar support, the marker file and sidecar files are harmless and will be ignored.

## Major version upgrades

For `vN -> v(N+1)` jumps, read the migration guide in the release notes. These can include API breaking changes (e.g., the `master_secret` requirement and API key hash change introduced in v1.0).
