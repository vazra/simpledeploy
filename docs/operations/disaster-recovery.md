---
title: Disaster recovery
description: Restore a SimpleDeploy server from scratch using the system DB backup and per-app backups.
---

When things go badly wrong, the speed of recovery depends entirely on how you set things up beforehand. This page covers three failure modes, ordered by severity.

SimpleDeploy also mirrors every user-editable setting (users, alert rules, backup configs, registries, webhooks) to YAML sidecar files on disk. A wiped database can be recovered from those files without a prior backup. See [Config sidecars and sidecar-based recovery](/operations/config-sidecars/) for the full procedure.

## RTO and RPO targets

- **RPO** (recovery point objective) = how much data you can afford to lose. This is driven by *backup frequency*. Hourly backups = ~1h RPO.
- **RTO** (recovery time objective) = how long you can afford to be down. This is driven by *restore time*: download backup + provision host + restore = your floor.

| Setup | Typical RPO | Typical RTO |
|-------|-------------|-------------|
| Daily DB backup, no app backup | 24h | hours to days |
| Hourly DB + daily app volume backup | 1-24h | 1-2h |
| 15-min DB + hourly volume backup, hot standby | 15-60m | minutes |

Most small ops can hit 1h RPO / 1h RTO with hourly backups to S3 and a documented restore runbook.

## Scenario 1: SimpleDeploy crashes

The process died but the host is fine. Apps may keep running (Docker is independent) but the dashboard, proxy, and metrics collection are down.

### Diagnose

```bash
sudo systemctl status simpledeploy
journalctl -u simpledeploy -n 200 --no-pager
df -h /var/lib/simpledeploy            # disk full?
free -m                                 # OOM?
```

Common causes:
- Disk full (DB cannot write WAL). Free space, restart.
- Out of memory (kernel OOM killed it). Check `dmesg | grep -i kill`. Add swap or upgrade RAM.
- Corrupt DB after power loss. SQLite is WAL-mode and survives most crashes; if it does not, restore from the latest DB backup.
- Bad config after edit. `simpledeploy validate --config /etc/simpledeploy/config.yaml`.

### Restart

```bash
sudo systemctl restart simpledeploy
journalctl -u simpledeploy -f
```

If it dies again immediately, check the last error in the journal. Do not loop on restart; fix the root cause.

## Scenario 2: Whole VPS lost

Hardware failure, accidental termination, region outage. You need to rebuild on a new host.

<Aside type="caution">
This scenario is only survivable if your DB backup AND app data backups live OFF the lost host. S3 with a different region, or SFTP to a separate provider.
</Aside>

### Step 1: Provision the replacement

Same OS, same arch as before. Restore your usual hardening (firewall, SSH keys, user accounts).

### Step 2: Install SimpleDeploy

Same version as the lost host:

```bash
# apt
sudo apt install simpledeploy=1.2.0

# Or download binary
curl -L https://github.com/vazra/simpledeploy/releases/download/v1.2.0/simpledeploy-linux-amd64 \
  -o /usr/local/bin/simpledeploy && chmod +x /usr/local/bin/simpledeploy
```

Do not start the service yet.

### Step 3: Restore DNS

Update A/AAAA records to point at the new host. Do this early so DNS has time to propagate.

### Step 4: Restore the system DB

Download the latest backup from your off-host target (S3, SFTP). Place it at the configured `data_dir`:

```bash
sudo mkdir -p /var/lib/simpledeploy
sudo cp simpledeploy-2026-04-15.db /var/lib/simpledeploy/simpledeploy.db
sudo chown -R simpledeploy:simpledeploy /var/lib/simpledeploy
sudo chmod 0600 /var/lib/simpledeploy/simpledeploy.db
```

### Step 5: Restore config

Recreate `/etc/simpledeploy/config.yaml` with the same `master_secret` as before. **This is non-negotiable.** Without it, encrypted registry credentials and JWT signing keys cannot be recovered. Keep `master_secret` in a password manager separate from the host.

### Step 6: Start the service

```bash
sudo systemctl start simpledeploy
```

The reconciler reads the `apps_dir` and starts pulling images. Apps with no persistent data come up immediately. Stateful apps need their volumes restored before they will be useful.

### Step 7: Restore app data volumes

For each stateful app, restore the latest volume/database backup according to the strategy used:

- **Postgres backup**: see [Restoring app backups](/guides/backups/restore/) for `pg_restore` flow.
- **Volume backup**: extract the tarball into the app's named volume directory.

```bash
# Volume restore example
docker run --rm -v myapp_data:/restore -v $(pwd):/backup alpine \
  tar xzf /backup/myapp-2026-04-15.tar.gz -C /restore
```

<Aside type="caution">
SimpleDeploy's own volume and SQLite restore endpoints (`POST /api/apps/{slug}/backups/upload-restore`) reject tar archives that contain absolute paths, `..` segments, symlinks, hardlinks, or device entries. They also cap gzip decompression at 8 GiB and run no more than 4 concurrent restores at a time. Archives produced by the matching `Backup` step pass these checks; bring-your-own tarballs may need to be repacked without symlinks.
</Aside>

### Step 8: Verify

- Dashboard loads
- Each app reachable on its public domain
- Sample data present in each app
- New backup runs cleanly (close the loop)

## Scenario 3: Bad deploy bricked an app

A deploy went out, the app does not start or behaves badly. Other apps and SimpleDeploy itself are fine.

### Quick rollback via UI

App detail page -> **Versions** tab -> select the previous deploy -> **Rollback**. SimpleDeploy redeploys the prior `compose.yaml` and waits for containers to be healthy.

### Quick rollback via CLI

```bash
simpledeploy versions list myapp
simpledeploy rollback myapp --version <hash>
```

### Manual override

If the rollback itself fails (rare), edit the compose file directly:

```bash
sudo vim /etc/simpledeploy/apps/myapp/compose.yaml
# Restore the last-known-good content from your git repo or backup
sudo systemctl reload simpledeploy
```

The reconciler watches `apps_dir` and reapplies on change.

### Restore app data

If the bad deploy *also* corrupted data (a migration gone wrong), follow the app data restore steps from Scenario 2 for just that app.

## Drill schedule

Run a full disaster recovery drill once per quarter:

1. Spin up a throwaway VPS.
2. Restore the latest DB and one app from backups.
3. Time the whole process. If it took longer than your RTO, fix the gap.
4. Document deviations and update this runbook.

A backup you have never restored is theoretical.
