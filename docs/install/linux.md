---
title: Install on generic Linux
description: Install SimpleDeploy on any Linux distribution from the prebuilt binary tarball, with a manual systemd unit and dedicated user.
---

import { Steps, Tabs, TabItem, Aside } from '@astrojs/starlight/components';

For distros without an APT package (Fedora, Arch, Alpine, RHEL-likes, etc.). Drop the binary in `/usr/local/bin/`, set up a systemd unit, you are done.

Prefer a containerized install? See [Install via Docker](/install/docker/); it works on the same distros without needing a systemd unit.

## Prerequisites

- A modern Linux with systemd (or your own init manager).
- Docker Engine with the Compose plugin.
- Ports 80, 443, and 22 open.
- DNS pointing at the server.

## Download

<Tabs>
<TabItem label="amd64">
```bash
curl -L https://github.com/vazra/simpledeploy/releases/latest/download/simpledeploy_linux_amd64.tar.gz | tar xz
sudo mv simpledeploy /usr/local/bin/
sudo chmod +x /usr/local/bin/simpledeploy
```
</TabItem>
<TabItem label="arm64">
```bash
curl -L https://github.com/vazra/simpledeploy/releases/latest/download/simpledeploy_linux_arm64.tar.gz | tar xz
sudo mv simpledeploy /usr/local/bin/
sudo chmod +x /usr/local/bin/simpledeploy
```
</TabItem>
</Tabs>

Verify:

```bash
simpledeploy version
```

## Create a service user (optional, recommended)

Running as a dedicated user limits blast radius. The user must be in the `docker` group to drive the Docker socket.

```bash
sudo useradd --system --home /var/lib/simpledeploy --shell /usr/sbin/nologin simpledeploy
sudo usermod -aG docker simpledeploy
sudo mkdir -p /var/lib/simpledeploy /etc/simpledeploy/apps
sudo chown -R simpledeploy:simpledeploy /var/lib/simpledeploy /etc/simpledeploy
```

<Aside type="note">
For a single-user box you can skip this and run as `root`. The `.deb` package on Ubuntu currently runs as root too.
</Aside>

## systemd unit

Create `/etc/systemd/system/simpledeploy.service`:

```ini
[Unit]
Description=SimpleDeploy
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/simpledeploy serve --config /etc/simpledeploy/config.yaml
Restart=always
RestartSec=5
# Uncomment the next two lines if you created a service user
# User=simpledeploy
# Group=simpledeploy

[Install]
WantedBy=multi-user.target
```

## Configure and start

<Steps>

1. Generate the config (see [Configure SimpleDeploy](/first-deploy/config/)):

   ```bash
   sudo simpledeploy init --config /etc/simpledeploy/config.yaml
   sudo vim /etc/simpledeploy/config.yaml
   ```

2. Enable and start:

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable --now simpledeploy
   sudo journalctl -u simpledeploy -f
   ```

</Steps>

## Upgrading

Download the new tarball, replace the binary, restart:

```bash
sudo systemctl stop simpledeploy
sudo mv simpledeploy /usr/local/bin/
sudo systemctl start simpledeploy
```

Next: [First deploy](/first-deploy/prepare/).
