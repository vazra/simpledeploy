---
title: Upgrading
description: Upgrade SimpleDeploy via apt, brew, or by replacing the binary. Always back up the SQLite DB first. Schema migrations run automatically.
---

import { Tabs, TabItem, Steps, Aside } from '@astrojs/starlight/components';

<Aside type="caution" title="Back up first">
Always back up the SQLite database (and your `apps/` directory) before upgrading. Migrations run forward only. See [Disaster recovery](/operations/disaster-recovery/) for the procedure.
</Aside>

## Quick backup

```bash
# DB snapshot (WAL-safe via VACUUM INTO)
sudo simpledeploy db backup --output /var/lib/simpledeploy/backups/pre-upgrade.db

# Compose files
sudo tar -czf ~/apps-pre-upgrade.tgz /etc/simpledeploy/apps
```

## Upgrade by install method

<Tabs>
<TabItem label="Ubuntu / Debian">
```bash
sudo apt update
sudo apt upgrade simpledeploy
```

systemd restarts the service automatically when the package replaces the binary.
</TabItem>

<TabItem label="macOS (CLI)">
```bash
brew update
brew upgrade simpledeploy
```
</TabItem>

<TabItem label="Generic Linux (binary)">
<Steps>

1. Download the new tarball:

   ```bash
   curl -L https://github.com/vazra/simpledeploy/releases/latest/download/simpledeploy_linux_amd64.tar.gz | tar xz
   ```

2. Stop, replace, start:

   ```bash
   sudo systemctl stop simpledeploy
   sudo mv simpledeploy /usr/local/bin/
   sudo systemctl start simpledeploy
   ```

</Steps>
</TabItem>

<TabItem label="From source">
```bash
cd simpledeploy
git pull
make build
sudo systemctl stop simpledeploy
sudo cp bin/simpledeploy /usr/local/bin/
sudo systemctl start simpledeploy
```
</TabItem>
</Tabs>

## What happens on first start

1. Binary boots, opens the SQLite file.
2. Embedded migrations run in order. Any new schema is applied transactionally.
3. Caddy reloads with the existing route config.
4. The reconciler re-scans `apps_dir` and reconciles desired state.

App containers stay running across the upgrade. The dashboard is briefly unavailable (a few seconds).

## Breaking changes

Read the [CHANGELOG](https://github.com/vazra/simpledeploy/blob/main/CHANGELOG.md) before any minor version bump. Compose label renames or config field changes will be called out there with migration steps.

## Rollback

If a new version misbehaves:

<Steps>

1. Stop the service:

   ```bash
   sudo systemctl stop simpledeploy
   ```

2. Restore the previous binary:

   ```bash
   # apt
   sudo apt install simpledeploy=<previous-version>

   # binary
   sudo cp /path/to/old/simpledeploy /usr/local/bin/
   ```

3. Restore the DB snapshot:

   ```bash
   sudo cp /var/lib/simpledeploy/backups/pre-upgrade.db /var/lib/simpledeploy/simpledeploy.db
   ```

4. Start:

   ```bash
   sudo systemctl start simpledeploy
   ```

</Steps>

Restoring the DB undoes any new-schema rows the new version wrote. App containers themselves are unaffected by the rollback.
