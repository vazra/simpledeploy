---
title: Production checklist
description: Pre-flight checks for going live with SimpleDeploy: TLS, master_secret, backups, alerts, firewall, audit log.
---

Run through every box before you put real traffic on a SimpleDeploy host. Skipping items will burn you later.

<Aside type="caution">
Do not skip backup *restore* testing. An untested backup is not a backup.
</Aside>

## DNS

- [ ] `A` (and `AAAA` if IPv6) record for the management domain points at the server
- [ ] `A`/`AAAA` records for every app domain point at the server (or a CNAME chain that ends there)
- [ ] TTL low (300s) during cutover so you can move quickly if something is wrong
- [ ] No CAA records that block Let's Encrypt (`letsencrypt.org`)

## Firewall

- [ ] Inbound `80/tcp` open (ACME HTTP-01 + redirect to HTTPS)
- [ ] Inbound `443/tcp` open (proxied apps + management UI on `:443` if you front it with Caddy)
- [ ] Inbound `8443/tcp` open only if management UI runs on its own port
- [ ] SSH restricted to known IPs or behind a VPN/bastion
- [ ] Docker daemon socket NOT exposed on the network
- [ ] All other inbound traffic dropped by default

## TLS

- [ ] `tls.mode: auto` set in `/etc/simpledeploy/config.yaml`
- [ ] `tls.email` set to a monitored mailbox (Let's Encrypt sends expiry warnings here)
- [ ] First successful certificate issued (visit any domain over HTTPS, check the cert)
- [ ] HSTS acceptable for your domain (header is sent automatically when TLS is active)
- [ ] If using a custom CA: cert + key uploaded with valid PEM and DNS-safe domain

## Authentication and secrets

- [ ] `master_secret` is at least 32 random characters (`openssl rand -hex 32`)
- [ ] `master_secret` stored in a password manager or secret store (NOT in git)
- [ ] Default setup admin password rotated immediately after first login
- [ ] At least two `super_admin` accounts (so you cannot lock yourself out)
- [ ] Login rate limit defaults left in place (10/min) unless you have a reason to raise them
- [ ] `trusted_proxies` set if SimpleDeploy is behind a load balancer or Cloudflare

## Backups

- [ ] System DB backup configured (`Settings -> Database backup`) with off-host target
- [ ] At least one app data backup configured for every stateful service (Postgres, volumes)
- [ ] Backup schedule matches your RPO target (hourly for tight RPO, daily for relaxed)
- [ ] **Restore tested end-to-end** on a staging box. Check the data actually loads.
- [ ] S3/SFTP credentials encrypted (they are, automatically; verify by inspecting `registries` table)

## Alerts

- [ ] At least one webhook configured (Slack, PagerDuty, Discord, generic HTTP)
- [ ] Test webhook fires (use the test button in the UI)
- [ ] Alert rule for high CPU per app (e.g., >80% for 5 minutes)
- [ ] Alert rule for high memory per app
- [ ] Alert rule for low disk space on host
- [ ] Alert rule for app down (no metrics received for N minutes)

## External monitoring

- [ ] External uptime check pinging `https://manage.example.com/api/health` every 1 minute
- [ ] External uptime check on the public side of every critical app
- [ ] Notification channel for those checks is NOT the same host (else outage hides itself)

## Logging

- [ ] Server stdout/stderr captured by `journald` (default if installed via apt/brew service unit)
- [ ] Log retention policy on journald (`SystemMaxUse=2G` or similar)
- [ ] Optional: ship JSON audit events to Loki, CloudWatch, or Datadog

## Updates

- [ ] Subscribe to GitHub releases: `https://github.com/vazra/simpledeploy/releases.atom`
- [ ] Decide upgrade cadence (monthly is reasonable for most teams)
- [ ] Staging environment exists for upgrade dry runs

## GitOps (optional)

- [ ] If using two-way GitOps: enable `git_sync:` and configure the webhook (see [Git sync](/operations/git-sync/))

## Access

- [ ] Long-lived API key minted for CI/CD with appropriate role (manage, not super_admin)
- [ ] Per-developer accounts created (no shared `admin` login)
- [ ] Viewer accounts for stakeholders who only need to look
- [ ] `user_app_access` configured for non-admins so they only see their apps

## Audit

- [ ] Activity log reachable: `GET /api/activity?limit=10`
- [ ] Retention set to match your compliance requirement (default 365 days; `0` = forever)
- [ ] Plan to export entries before retention window if you need archival evidence
- [ ] Quarterly review of accounts and API keys (delete stale ones)

## Capacity

- [ ] Disk has at least 3x the expected DB size free (see [Capacity sizing](/operations/capacity-sizing/))
- [ ] At least 1GB RAM headroom above app baseline
- [ ] Docker image cleanup scheduled (`docker image prune -af` weekly)

## Final smoke test

- [ ] Deploy a test app, hit it over HTTPS, see metrics, see logs
- [ ] Trigger a manual backup, download it, verify file is non-empty
- [ ] Reboot the host, confirm SimpleDeploy and all apps come back automatically
