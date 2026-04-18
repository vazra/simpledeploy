---
title: Troubleshooting
description: Common issues with SimpleDeploy and how to diagnose them: TLS, deploys, backups, proxy, auth.
---

import { Aside } from '@astrojs/starlight/components';

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

## Deploy fails with "compose file rejected"

**Symptom:** Deploy refused with a validation error.

**Diagnose:** Read the exact error message in the deploy logs. SimpleDeploy validates compose files and rejects dangerous directives.

**Fix:** Remove the offending directive. See [Security hardening - Deployment safety](/operations/security-hardening/#deployment-safety) for the full reject list (`privileged`, `network_mode: host`, `cap_add: SYS_ADMIN`, bind mounts of `/etc`/`/proc`/`docker.sock`, etc.).

If you genuinely need one of these (rare), there is no override. File an issue explaining the use case.

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
- Origin mismatch: the management UI must be served from the same hostname as the API. Cross-origin WS is rejected by design.
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
- Wrong port: confirm the service `expose:` or `ports:` matches what the app listens on.
- DNS not resolving: `dig +short example.com` should match server IP.

## 429 rate limit hitting the dashboard

**Symptom:** Dashboard or API returns `429 Too Many Requests`. Common during scripted use.

**Diagnose:** Check what is hammering the server. Audit log for repeated requests from one IP.

**Fix:**
- Login flood (10/min): wait 60s. Check for misconfigured auto-login scripts.
- Per-app rate limit: tune `simpledeploy.ratelimit.*` labels on the affected app.
- Behind a proxy: set `trusted_proxies` in config so rate limiting uses real client IPs.

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
