---
title: Install on Ubuntu / Debian
description: Install SimpleDeploy on Ubuntu or Debian via the official APT repository. Includes systemd setup, firewall config, and verification.
---

import { Steps, Aside } from '@astrojs/starlight/components';

The recommended path for production. The `.deb` package ships a systemd unit and pulls updates through normal `apt upgrade`.

Prefer containers or on a non-Debian distro? See [Install via Docker](/install/docker/).

## Prerequisites

- Ubuntu 22.04+ or Debian 12+, x86_64 or arm64.
- Docker Engine with the Compose plugin. Install via [docs.docker.com/engine/install](https://docs.docker.com/engine/install/) if missing.
- A DNS A record for the management domain (and any app domains) pointing at the server.

## Install

<Steps>

1. Add the repo and install:

   ```bash
   curl -fsSL https://vazra.github.io/apt-repo/gpg.key \
     | sudo gpg --dearmor -o /usr/share/keyrings/vazra.gpg

   echo "deb [signed-by=/usr/share/keyrings/vazra.gpg arch=$(dpkg --print-architecture)] https://vazra.github.io/apt-repo stable main" \
     | sudo tee /etc/apt/sources.list.d/vazra.list

   sudo apt update && sudo apt install simpledeploy
   ```

2. Verify:

   ```bash
   simpledeploy version
   ```

</Steps>

## Open the firewall

SimpleDeploy needs ports 80 and 443 reachable from the internet (Let's Encrypt validates over 80, traffic flows over 443). Keep 22 open for SSH.

```bash
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw enable
```

If you run a cloud firewall (AWS SG, Hetzner Cloud, DigitalOcean), open the same ports there too.

## Configure and start

The `.deb` installs a systemd unit at `/etc/systemd/system/simpledeploy.service`, but does not start the service until you generate a config.

<Steps>

1. Generate the config:

   ```bash
   sudo simpledeploy init --config /etc/simpledeploy/config.yaml
   sudo vim /etc/simpledeploy/config.yaml
   ```

   Set `domain`, `tls.email`, and `master_secret` (generate via `openssl rand -hex 32`). See [Configure SimpleDeploy](/first-deploy/config/) for the full walkthrough.

2. Enable and start:

   ```bash
   sudo systemctl enable --now simpledeploy
   sudo systemctl status simpledeploy
   ```

3. Tail the logs while it boots:

   ```bash
   sudo journalctl -u simpledeploy -f
   ```

</Steps>

## Verify

Hit the management UI:

```
https://manage.your-domain.com/
```

You should land on the setup wizard. If you get a TLS error, give Let's Encrypt 30-60 seconds and reload.

## Upgrading

```bash
sudo apt update && sudo apt upgrade simpledeploy
```

Schema migrations run automatically on the next start. See [Upgrading](/install/upgrading/) for rollback notes.

Next: [First deploy](/first-deploy/prepare/).
