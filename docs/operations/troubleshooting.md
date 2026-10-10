---
title: Troubleshooting
description: Common issues with SimpleDeploy and how to diagnose them: TLS, deploys, backups, proxy, auth.
---

Each entry follows: **symptom -> diagnostic command -> fix**.

## Deploy hangs at "pulling image"

**Symptom:** Deploy progress bar sticks. Logs show `Pulling foo/bar:latest...` for >5 minutes.

**Diagnose:**

```bash
# Check what compose is doing
sudo docker compose -f /etc/simpledeploy/apps/<slug>/compose.yaml pull

# Check registry auth
simpledeploy registry list
```

**Fix:**
- Bad image name or tag: correct the `image:` line in the compose file.
- Private registry: add credentials via **Settings -> Registries** (or `simpledeploy registry add`).
- Network issue: confirm DNS works (`getent hosts registry-1.docker.io`) and outbound 443 is open.
- Rate limited by Docker Hub: log in to a paid account or use a registry mirror.

## Deploy reported "Unstable"

**Symptom:** The deploy wizard shows a yellow "Unstable" status. The per-service summary lists one or more services as `restarting`, `unhealthy`, or with `restarted Nx`.

**What it means:** `docker compose up` finished without an error, but during the 30-second post-deploy stabilization window one of the containers either kept restarting, reported `unhealthy` from its healthcheck, or exited. This is almost always an application-level error rather than a SimpleDeploy bug.

**Diagnose:**

```bash
# Watch the offending container's logs (replace slug + service)
sudo docker logs simpledeploy-<slug>-<service>-1 --tail 200
```

In the UI: open the app, **Logs** tab, pick the unhealthy service, scroll to the latest entries. The actual error from the container is usually in the last few lines before the restart.

**Common causes:**
- **Permission errors writing to a volume** (`EACCES`, `Permission denied`). The image runs as a non-root user but the named volume was created root-owned. Add `user: "1000:1000"` (or whatever UID the image documents) to the service, or use the templated version of the app from the wizard which handles this.
- **Required env var missing.** Container exits with "X is required". Add the missing env var via the **Config** tab.
- **Port already bound** in `port-only` mode if the same host port is requested by another app.
- **Healthcheck endpoint not yet implemented** in your code. Either add the endpoint or relax the healthcheck (longer `start_period`).

**Fix and redeploy:** edit the compose via the **Config** tab and click **Redeploy**. The next deploy will re-run the stabilization check.

## Deploy fails with "compose file rejected"

**Symptom:** Deploy, rollback, pull or scale refused with a list of violations (`compose file contains disallowed directives`, or `Stopped before starting: the compose file breaks these security rules:` in the deploy log). An app added through git sync or SSH never starts.

**Diagnose:** The dashboard and API show each violation. For files picked up from disk, check the server log:

```bash
journalctl -u simpledeploy | grep "SECURITY: skipping"
```

The check runs with `${VARS}` filled in from the app's `.env`, so a value there can cause a violation too.

**Fix:** Remove or change the offending setting and deploy again. The full list is in [Compose security validation](/reference/compose-labels/#compose-security-validation). If an app needs a host folder under a protected path (for example a media library under `/home`), add that folder to `allowed_bind_paths` in `config.yaml` and restart SimpleDeploy. Other rules have no override; if you genuinely need one, file an issue explaining the use case.

## App still serves traffic but redeploys are refused

**Symptom:** After an upgrade, an app keeps working, but redeploy, pull and scale fail with violations, and saving endpoints, IP access or `.env` returns `409` with "this app's compose file no longer passes security checks". The log shows `keeping routes for the running app; fix its compose file before the next deploy`.

**What it means:** The app was deployed before the current compose rules. It keeps its routes only while its compose file is unchanged.

**Fix:** Edit the compose file so it passes the checks (see the entry above) and redeploy. Any change to the file ends the route retention: a file that still fails stays offline until it is fixed.

## TLS certificate fails to issue

**Symptom:** Browser shows cert error or `tls: no certificates configured`. Logs show ACME errors.

**Diagnose:**

```bash
journalctl -u simpledeploy -n 200 | grep -i acme
# Verify DNS
dig +short manage.example.com
# Verify port 80 reachable from outside
curl -I http://manage.example.com/.well-known/acme-challenge/test
```

**Fix:**
- DNS not pointing at this host: fix A/AAAA record, wait for TTL.
- Port 80 blocked: open inbound `80/tcp` in firewall and any cloud security group.
- Let's Encrypt rate limit (5 certs/week per domain): wait 1 week or use staging endpoint while testing.
- CAA record blocking: `dig CAA example.com` should include `letsencrypt.org` or be empty.
- `tls.email` missing: required for ACME. Set in config and restart.

## "permission denied" on data_dir

**Symptom:** Service fails to start with permission errors writing to `/var/lib/simpledeploy`.

**Diagnose:**

```bash
ls -la /var/lib/simpledeploy
ps aux | grep simpledeploy   # what user is the process running as?
```

**Fix:**

```bash
sudo chown -R simpledeploy:simpledeploy /var/lib/simpledeploy
sudo chmod 0700 /var/lib/simpledeploy
sudo chmod 0600 /var/lib/simpledeploy/simpledeploy.db
```

## WebSocket logs not streaming

**Symptom:** Log viewer in the UI shows "Connecting..." forever or disconnects immediately.

**Diagnose:** Open browser dev tools -> Network -> WS. Look for the failed `/api/apps/<slug>/logs` connection and read the close code.

**Fix:**
- Behind Cloudflare with WS disabled: enable WebSockets in Cloudflare dashboard for the management hostname.
- Behind nginx/another proxy: ensure `proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection upgrade;` are set.
- Origin mismatch: the management UI must be served from the same hostname and port as the API. Cross-origin WS is rejected by design. A proxy that drops the port from `Host` still works when the browser sends `Sec-Fetch-Site: same-origin`.
- Close code `1008` ("authorization changed"): sockets re-check the session every 60 seconds and close after logout, a password or role change, API key revocation, or lost app access. Sign in again, or ask an admin to restore access.
- Idle timeout: connections close after 5 minutes idle. The UI auto-reconnects.

## High memory usage

**Symptom:** Server RAM growing over time. `top` shows large `simpledeploy` or Docker resident size.

**Diagnose:**

```bash
free -m
docker system df
docker images | wc -l
du -sh /var/lib/simpledeploy
```

**Fix:**

```bash
# Clean up unused Docker stuff
docker image prune -af
docker volume prune -f
docker system prune -af --volumes   # nuclear option

# Lower metrics retention if DB is huge (see capacity-sizing.md)
```

If SimpleDeploy itself is leaking, capture a profile:

```bash
curl http://localhost:8443/debug/pprof/heap > heap.out
```

and open a GitHub issue.

## App not reachable from the internet

**Symptom:** App's domain returns 404, connection refused, or times out.

**Diagnose:**

```bash
# Is the container actually running?
docker ps | grep <app-slug>

# Is it listening on the expected port inside the container?
docker exec -it <container> ss -tln

# Does Caddy know about this domain?
curl -s http://localhost:2019/config/ | jq '.apps.http.servers'
```

**Fix:**
- Container not running: redeploy. Check container logs for crash loop.
- Missing endpoint label: add `simpledeploy.endpoint=example.com` to the service in `compose.yaml`.
- Domain claimed by two apps: the log shows `domain is already served by app "..."`. See [409 domain already used by another app](#409-domain-already-used-by-another-app).
- Wrong port: confirm the service `expose:` or `ports:` matches what the app listens on.
- DNS not resolving: `dig +short example.com` should match server IP.

## 429 rate limit hitting the dashboard

**Symptom:** Dashboard or API returns `429 Too Many Requests`. Common during scripted use.

**Diagnose:** Check what is hammering the server. Audit log for repeated requests from one IP.

**Fix:**
- Login flood (10/min): wait 60s. Check for misconfigured auto-login scripts.
- Per-app rate limit: tune `simpledeploy.ratelimit.*` labels on the affected app.
- Behind a proxy: set `trusted_proxies` in config so rate limiting uses real client IPs.
- Restore returns `too many restores in progress, try again later`: at most 4 restores run at once (uploads and backup history together). Wait for one to finish. See [Concurrent restores](/guides/backups/restore/#concurrent-restores).

## 409 domain already used by another app

**Symptom:** Saving endpoints or `.env` values, or restoring or rolling back to a compose version, returns `409` with `domain ... is already used by app "..."` (or `by another app`), `is reserved for the SimpleDeploy dashboard`, or `is a wildcard domain; only a super admin can add one`.

**What it means:** Each domain is served by one app, even on different paths. The dashboard `domain` and wildcard domains are reserved for super admins.

**Fix:** Remove the domain from the other app first, or pick another domain. If two compose files on disk (for example from git sync) claim the same domain, the app created first keeps it and the other app's routes on it are dropped with a log warning.

## Backup failed

**Symptom:** Backup run shows `failed` in **Backups** UI.

**Diagnose:**

```bash
# Check the backup run log
curl -H "Authorization: Bearer $SD_API_KEY" \
  https://manage.example.com/api/apps/<slug>/backups/runs/<id>
```

**Fix:**
- Bad S3 credentials: re-enter in **Settings -> Backup target**.
- S3 bucket not reachable: check region, endpoint URL, network.
- Strategy script crashed: check the run logs for the exact error from `pg_dump`/`tar`.
- Disk full on local target: free space or move target to S3.
- Restore fails with `backup archive exceeds the restore size limit`: raise `SIMPLEDEPLOY_RESTORE_MAX_GB`. See [Restore size limits](/guides/backups/restore/#size-limits).

## S3 endpoint refused as a private address

**Symptom:** Saving an S3 backup config, **Test S3**, or a backup run fails with `S3 endpoint points to a private or reserved network address`.

**What it means:** Custom S3 endpoints on loopback, private, link-local or CGNAT/Tailscale addresses (for example MinIO on the same server or LAN) are refused by default. Public S3 services need nothing extra.

**Fix:** Start the server with `SIMPLEDEPLOY_ALLOW_PRIVATE_S3=1` (for example `Environment=SIMPLEDEPLOY_ALLOW_PRIVATE_S3=1` in the systemd unit) and restart. Set it in your shell too for `simpledeploy backup run` and `simpledeploy restore`. See [Environment variables](/reference/env-vars/).

## Git sync: "rebase refused"

**Symptom:** The Git Sync page shows `rebase refused: apps_dir has uncommitted or untracked changes that the pull would overwrite`.

**What it means:** Local files in `apps_dir` differ from git (common with `auto_push_enabled: false`, where dashboard edits stay uncommitted), and git will not apply remote commits over them.

**Diagnose:**

```bash
sudo git -C /etc/simpledeploy/apps status
```

**Fix:** Commit the changes (or turn on [`auto_push_enabled`](/operations/git-sync/#auto_push_enabled)), or discard them, for example with `git stash -u` to keep a copy. Then sync again.

## Startup warning about master_secret

**Symptom:** The log shows `WARNING: master_secret is still the example placeholder ...` or `master_secret is shorter than 32 characters`.

**What it means:** The config still has an example value from the docs (or a weak one). With a placeholder, sessions are signed with a random key from `data_dir/session-signing.key`, but stored credentials are still encrypted with the public value.

**Fix:** Generate a new secret with `openssl rand -hex 32`, put it in `config.yaml`, and restart. Then re-enter registry and S3 credentials and re-create API keys; everyone signs in again. See [Generating a master secret](/operations/security-hardening/#generating-a-master-secret).

## Forgot admin password

<Aside type="caution">
There is no password reset email. Recovery requires shell access to the server.
</Aside>

**Diagnose:** No need; if you cannot log in, you cannot log in.

**Fix:**

```bash
# Create a new super_admin (will prompt for password)
sudo -u simpledeploy simpledeploy users create \
  --username recovery \
  --role super_admin

# Log in as 'recovery', delete the old account from the UI
# Then delete the recovery account or rotate its password
```

## Service won't start after upgrade

See [Upgrade and rollback - Rollback](/operations/upgrade-rollback/#rollback). Usually a migration ran that the previous binary does not understand. Restore the pre-upgrade DB backup and downgrade.

## Still stuck

1. Search [GitHub issues](https://github.com/vazra/simpledeploy/issues).
2. Open a new issue with: version (`simpledeploy version`), OS, last 200 lines of `journalctl -u simpledeploy`, and steps to reproduce.
