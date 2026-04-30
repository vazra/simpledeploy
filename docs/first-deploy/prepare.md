---
title: Prepare the server
description: What you need before installing SimpleDeploy. VPS sizing, Docker, firewall ports, and DNS records.
---

Five minutes of prep saves an hour of debugging later.

## Pick a VPS

A small Linux box. Ubuntu 22.04+ or Debian 12+ is the most-tested path; any systemd distro works.

| Workload | vCPU | RAM | Disk |
|---|---|---|---|
| 1-3 small apps, hobby | 1 | 1 GB (min) | 20 GB |
| 5-10 apps, small team | 2 | 2 GB (recommended) | 40 GB |
| 10-20 apps, busy | 2-4 | 4 GB | 80 GB+ |

SimpleDeploy itself uses ~60 MB RAM. Everything else is your apps and Docker.

## Install Docker

SimpleDeploy needs Docker Engine + the Compose plugin. Skip this if Docker is already running.

```bash
# Official one-liner (verify on docs.docker.com first)
curl -fsSL https://get.docker.com | sudo sh
sudo systemctl enable --now docker
docker version
docker compose version
```

<Aside type="tip">
On Ubuntu 22.04+, `apt install docker.io docker-compose-v2` also works.
</Aside>

## Open the firewall

You need three ports inbound: 22 (SSH), 80 (HTTP, Let's Encrypt), 443 (HTTPS).

```bash
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

If your provider has a separate cloud firewall (AWS Security Group, Hetzner, DO), open the same ports there.

## Set up DNS

Point an A record for the **management domain** at the server, e.g. `manage.example.com`.

For each app you plan to deploy, add an A record for its domain too. Wildcard records (`*.example.com`) work and save you a step.

<Steps>

1. In your DNS provider, create:
   - `manage.example.com` &rarr; `203.0.113.10` (your VPS IP)
   - `whoami.example.com` &rarr; `203.0.113.10`

2. Verify propagation:

   ```bash
   dig +short manage.example.com
   ```

   You should see your VPS IP.

</Steps>

Ready? On to [generating the config](/first-deploy/config/).
