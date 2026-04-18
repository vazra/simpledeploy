---
title: First deploy
description: End-to-end walk-through for installing SimpleDeploy on a VPS, configuring it, creating an admin, and deploying your first compose app.
---

## Prerequisites

- Linux VPS (Ubuntu 22.04+ recommended)
- Docker Engine with Docker Compose plugin installed and running
- Domain pointing to the server (for automatic TLS)
- Ports 80 and 443 open (for Let's Encrypt and HTTPS)

> SimpleDeploy requires Docker and Docker Compose. If either is missing, the server will exit with an error and a link to the install guide: https://docs.docker.com/engine/install/

## Installation

Pick your platform:

- [macOS (Homebrew)](/install/macos/)
- [Ubuntu / Debian (APT)](/install/ubuntu/)
- [Generic Linux (binary)](/install/linux/)
- [From source](/install/from-source/)

Verify:

```bash
simpledeploy version
```

## Server Setup

### 1. Generate Config

```bash
sudo simpledeploy init --config /etc/simpledeploy/config.yaml
```

### 2. Edit Config

```bash
sudo vim /etc/simpledeploy/config.yaml
```

Key settings to configure:

```yaml
domain: manage.yourdomain.com    # management UI domain
tls:
  mode: auto
  email: you@example.com         # for Let's Encrypt
master_secret: "generate-a-random-string-here"
```

Generate a master secret:
```bash
openssl rand -hex 32
```

### 3. Create Directories

```bash
sudo mkdir -p /var/lib/simpledeploy
sudo mkdir -p /etc/simpledeploy/apps
```

### 4. Run with systemd

If installed via `.deb` package (APT), the systemd service is already installed. Just enable and start:

```bash
sudo systemctl enable --now simpledeploy
```

If installed manually, create `/etc/systemd/system/simpledeploy.service`:

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

[Install]
WantedBy=multi-user.target
```

Then enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable simpledeploy
sudo systemctl start simpledeploy
```

### 5. Create Admin Account

On first run, simpledeploy prints a setup message. Create the admin account:

```bash
# Option A: via CLI (on the server)
simpledeploy users create --username admin --password yourpassword --role super_admin

# Option B: via API
curl -X POST http://localhost:8443/api/setup \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"yourpassword"}'

# Option C: via web UI
# Open https://manage.yourdomain.com and click "Create admin account"
```

### 6. Create API Key

For CLI and automation access:

```bash
simpledeploy apikey create --name "deploy-key" --user-id 1
# Save the printed key (sd_...)
```

## Client Setup (Local Machine)

### Configure Remote Context

```bash
simpledeploy context add production \
  --url https://manage.yourdomain.com \
  --api-key sd_your_api_key_here

simpledeploy context use production
```

### Deploy Apps

```bash
# Single app
simpledeploy apply -f ./myapp/docker-compose.yml --name myapp

# All apps in a directory
simpledeploy apply -d ./apps/

# Check status
simpledeploy list
```

## App Configuration

Create a compose file with SimpleDeploy labels:

```yaml
# apps/myapp/docker-compose.yml
services:
  web:
    image: myapp:latest
    ports:
      - "3000:3000"
    environment:
      DATABASE_URL: postgres://db:5432/myapp
    labels:
      simpledeploy.domain: "myapp.yourdomain.com"
      simpledeploy.port: "3000"
    depends_on:
      - db
    restart: unless-stopped

  db:
    image: postgres:16
    environment:
      POSTGRES_DB: myapp
      POSTGRES_PASSWORD: secret
    volumes:
      - pgdata:/var/lib/postgresql/data
    labels:
      simpledeploy.backup.strategy: "postgres"
      simpledeploy.backup.schedule: "0 2 * * *"
      simpledeploy.backup.target: "local"
      simpledeploy.backup.retention: "7"
    restart: unless-stopped

volumes:
  pgdata:
```

Deploy:
```bash
simpledeploy apply -f apps/myapp/docker-compose.yml --name myapp
```

The app will be:
- Deployed via Docker
- Accessible at `https://myapp.yourdomain.com` (automatic TLS)
- Backed up daily at 2 AM (7 backups retained)
- Monitored with default alerts (CPU > 80%, memory > 90%)

## Backup configuration

See [Backups overview](/guides/backups/overview/) for local and S3 setup.

## Alert configuration

See [Alert rules](/guides/alerts/rules/) and [Webhooks](/guides/alerts/webhooks/).

## Behind a load balancer

See [Behind a load balancer](/guides/load-balancer/).

## Monitoring

### Logs

```bash
# Server logs
journalctl -u simpledeploy -f

# App logs
simpledeploy logs myapp --follow
```

### Metrics

Access via the dashboard at `https://manage.yourdomain.com` or the API:

```bash
# System metrics
curl https://manage.yourdomain.com/api/metrics/system \
  -H "Authorization: Bearer sd_..."

# App metrics
curl https://manage.yourdomain.com/api/apps/myapp/metrics \
  -H "Authorization: Bearer sd_..."
```

## Resource Usage

SimpleDeploy targets ~60MB RAM for the management layer (excluding app containers).

| Component | Estimated RAM |
|-----------|---------------|
| Go runtime + Caddy | ~30-35MB |
| SQLite | ~5-8MB |
| Metrics buffers | ~2-3MB |
| Other | ~5MB |
| **Total** | **~45-55MB** |
