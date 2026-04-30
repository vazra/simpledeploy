---
title: Install via Docker
description: Install SimpleDeploy as a Docker container. The universal path for non-Debian distros (Fedora, Arch, Alpine), NAS boxes (Synology, TrueNAS), or a quick trial on any Linux host.
---

The universal path. Use Docker when you are on Fedora, Arch, Alpine, a NAS (Synology, TrueNAS), or just want a quick trial. For Debian/Ubuntu production servers, [apt](/install/ubuntu/) is still the recommended install.

<Aside type="note">
Requires a Linux host with Docker Engine and the Compose plugin, plus DNS A records for the management domain and any app domains pointing at the server.
</Aside>

## Install

<Steps>

1. Create the host directories:

   ```bash
   sudo mkdir -p /etc/simpledeploy /var/lib/simpledeploy
   ```

2. Drop in a compose file at `/etc/simpledeploy/docker-compose.yml`:

   ```yaml
   services:
     simpledeploy:
       image: ghcr.io/vazra/simpledeploy:latest
       restart: unless-stopped
       network_mode: host
       volumes:
         - /var/run/docker.sock:/var/run/docker.sock
         - /etc/simpledeploy:/etc/simpledeploy
         - /var/lib/simpledeploy:/var/lib/simpledeploy
   ```

   Also available as [`deploy/docker-compose.example.yml`](https://github.com/vazra/simpledeploy/blob/main/deploy/docker-compose.example.yml) in the repo. `network_mode: host` lets Caddy bind host :80/:443 directly so TLS and reverse-proxy upstreams behave identically to the native install. The same-path bind mounts are required so `docker compose -f /etc/simpledeploy/apps/<app>/docker-compose.yml` resolves the same paths inside the container and on the host.

3. Generate the config:

   ```bash
   sudo docker run --rm \
     -v /etc/simpledeploy:/etc/simpledeploy \
     ghcr.io/vazra/simpledeploy:latest \
     init --config /etc/simpledeploy/config.yaml

   sudo vim /etc/simpledeploy/config.yaml
   ```

   Set `domain`, `tls.email`, and `master_secret` (generate via `openssl rand -hex 32`). See [Configure SimpleDeploy](/first-deploy/config/) for the full walkthrough.

4. Start:

   ```bash
   cd /etc/simpledeploy
   sudo docker compose up -d
   ```

5. Tail the logs:

   ```bash
   sudo docker compose logs -f simpledeploy
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

## Verify

Hit the management UI:

```
https://manage.your-domain.com/
```

You should land on the setup wizard. If you get a TLS error, give Let's Encrypt 30-60 seconds and reload.

## Upgrading

```bash
cd /etc/simpledeploy
sudo docker compose pull
sudo docker compose up -d
```

<Aside type="note">
Schema migrations run on start. The container restarts with the new image; app containers stay running across the upgrade. See [Upgrading](/install/upgrading/) for the full matrix and rollback notes.
</Aside>

## Rollback

If a release misbehaves, pin the image tag to the previous version and restart:

<Steps>

1. Edit `/etc/simpledeploy/docker-compose.yml` and change the tag:

   ```yaml
   image: ghcr.io/vazra/simpledeploy:<previous-version>
   ```

2. Apply:

   ```bash
   cd /etc/simpledeploy
   sudo docker compose up -d
   ```

3. If the upgrade ran schema migrations, restore the pre-upgrade DB snapshot. See [Disaster recovery](/operations/disaster-recovery/) for the procedure.

</Steps>

## Docker Desktop (experimental)

<Aside type="caution" title="Not for production">
Docker Desktop on macOS or Windows does not give containers direct host networking, so Caddy cannot bind host :80/:443. This path uses bridge mode with published ports and an upstream-rewrite env var. Use it only for local trial or development.
</Aside>

Use [`deploy/docker-compose.desktop.example.yml`](https://github.com/vazra/simpledeploy/blob/main/deploy/docker-compose.desktop.example.yml) instead of the production compose file:

```yaml
services:
  simpledeploy:
    image: ghcr.io/vazra/simpledeploy:latest
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
      - "8443:8443"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    environment:
      SIMPLEDEPLOY_UPSTREAM_HOST: host.docker.internal
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - /etc/simpledeploy:/etc/simpledeploy
      - /var/lib/simpledeploy:/var/lib/simpledeploy
```

`SIMPLEDEPLOY_UPSTREAM_HOST` opts into an upstream rewrite: the proxy replaces `localhost:<port>` in resolved upstreams with `host.docker.internal:<port>` so Caddy inside the container can reach published app ports on the host.

The Desktop example also joins `simpledeploy-public` (see [Shared network](#shared-network) below) so Caddy can reach endpoint services over Docker DNS inside the VM, even when an app does not publish a host port.

### Contributor shortcut: `make dev-docker`

If you are hacking on simpledeploy locally on a Mac and want endpoint-only apps to work end to end, use the containerized dev workflow:

```bash
make dev-docker         # builds a linux binary + local image, starts the container
make dev-docker-down    # stops and cleans up
```

This uses [`deploy/docker-compose.dev.yml`](https://github.com/vazra/simpledeploy/blob/main/deploy/docker-compose.dev.yml), bind-mounts your repo at the same path inside the container (so `docker compose -f <abs>` resolves on both sides), reuses `config.dev.yaml`, and binds host :80/:443/:8500. Stop any native `./bin/simpledeploy` on :443 before running it.

## Shared network

On first start simpledeploy auto-creates a bridge network called `simpledeploy-public`. Every deployed app's endpoint-bearing services (any service with `simpledeploy.endpoints.*` or `simpledeploy.domain` labels) is auto-attached to it.

That is why endpoint services do not need to publish host ports to be reachable via their domain. Caddy resolves the upstream by container IP on the shared network.

<Aside type="tip">
You can still publish host ports with `ports:` if you want local access on `<host>:<port>`. When a host port is present, simpledeploy prefers it over the shared-network path.
</Aside>

For advanced app-to-app communication, you can reference `simpledeploy-public` as an external network on additional services. For most cross-app traffic, prefer exposing an endpoint and calling it over its domain.

## Security note

Mounting `/var/run/docker.sock` into the container is root-equivalent on the host: anyone who can reach the container can control every container on the Docker daemon, including creating a privileged container that can escape to the host. Treat the SimpleDeploy container as sensitive and keep the management port firewalled to trusted networks.

Next: [First deploy](/first-deploy/prepare/).
