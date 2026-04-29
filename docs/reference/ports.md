---
title: Ports and firewall
description: Default network ports SimpleDeploy listens on and recommended firewall rules.
---

SimpleDeploy listens on three TCP ports by default. App container ports are bound by Docker; SimpleDeploy itself does not publish them.

## Default ports

| Port | Process | Purpose | Public? |
|------|---------|---------|---------|
| `80` | Caddy | HTTP. ACME HTTP-01 challenges and redirect to 443. | Yes |
| `443` | Caddy | HTTPS reverse proxy for all apps with `simpledeploy.domain`. | Yes |
| `8443` | management API | Dashboard, REST API, WebSockets. | Optional |

The proxy listen address comes from `listen_addr` in `config.yaml` (default `:443`). Caddy automatically opens `:80` when `tls.mode` is `auto` or `local` (via `http_listen_addr`, defaulted to `:80`) so it can solve ACME challenges and 308-redirect plaintext traffic to HTTPS.

The management dashboard listens on `management_port` (default `8443`) bound to `management_addr` (default `127.0.0.1`). With the default bind, it is reachable only from the host itself; route external traffic to it through Caddy under a `manage.<domain>` route, or set `management_addr: ""` to expose every interface (legacy behavior, plain HTTP).

App containers bind whatever ports the compose file declares — but any `ports: "8080:80"` mapping is rewritten at deploy time to `127.0.0.1:8080:80` so the published port is reachable only from the host. Caddy still proxies external traffic to the same upstream, but raw connections from outside the host cannot bypass the per-app `simpledeploy.access.allow` IP allowlist or `simpledeploy.ratelimit.*` controls. Operators who explicitly want the legacy 0.0.0.0 binding can set `SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK=true`, or write the bind explicitly (`"0.0.0.0:8080:80"`) which is preserved verbatim.

If you do not write `ports:` at all, the container is reachable only on the Docker bridge network and via the Caddy reverse proxy.

## Firewall examples

### ufw (Ubuntu/Debian)

```bash
sudo ufw allow 80/tcp   comment 'HTTP / ACME'
sudo ufw allow 443/tcp  comment 'HTTPS apps'
sudo ufw allow 8443/tcp comment 'SimpleDeploy management'
sudo ufw enable
```

To keep the management port internal, replace the third rule with a source restriction:

```bash
sudo ufw allow from 10.0.0.0/8 to any port 8443 proto tcp
```

### AWS security group

| Type | Protocol | Port | Source |
|------|----------|------|--------|
| HTTP | TCP | 80 | `0.0.0.0/0` |
| HTTPS | TCP | 443 | `0.0.0.0/0` |
| Custom TCP | TCP | 8443 | `your-office-cidr/32` |

### GCP firewall

```bash
gcloud compute firewall-rules create simpledeploy-public \
  --allow tcp:80,tcp:443 --source-ranges 0.0.0.0/0

gcloud compute firewall-rules create simpledeploy-mgmt \
  --allow tcp:8443 --source-ranges YOUR.OFFICE.IP/32
```

## Notes

- ACME requires inbound `:80` from the public internet for HTTP-01 challenges. Block it only if you switch to DNS-01 or `tls.mode: custom`.
- Outbound `:443` to Let's Encrypt and your container registry must be allowed.
- The Docker daemon socket is local-only by default. Do not expose it to the network.
- App-to-app traffic goes over the Docker bridge network and never touches the host firewall.
