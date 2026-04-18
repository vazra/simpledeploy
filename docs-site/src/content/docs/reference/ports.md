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

The proxy listen address comes from `listen_addr` in `config.yaml` (default `:443`). Caddy automatically opens `:80` when `tls.mode` is `auto` so it can solve ACME challenges and redirect plaintext traffic.

The management port comes from `management_port` (default `8443`). It serves the Svelte UI, REST API (`/api/*`), and WebSocket streams. Bind it to a private interface or front it with the same Caddy proxy under a `manage.example.com` domain if you do not want it directly reachable.

App containers bind whatever ports the compose file declares. SimpleDeploy never auto-publishes ports; if you do not write `ports:` in your compose, the container is reachable only on the Docker bridge network and via the Caddy reverse proxy.

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
