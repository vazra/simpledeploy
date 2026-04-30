---
title: Generate config
description: Run simpledeploy init, edit the YAML, and set the three required fields (domain, master_secret, tls.email).
---

SimpleDeploy reads its config from `/etc/simpledeploy/config.yaml` by default. Generate a starter file, then edit three things.

## Generate

```bash
sudo simpledeploy init --config /etc/simpledeploy/config.yaml
```

That writes a fully commented YAML file with sensible defaults.

## Edit

```bash
sudo vim /etc/simpledeploy/config.yaml
```

You must set three fields before starting the server:

```yaml
# Management UI domain (TLS cert is provisioned for this name)
domain: manage.example.com

# TLS via Let's Encrypt
tls:
  mode: auto
  email: you@example.com    # ACME account email

# Encryption + signing key. NEVER commit this anywhere.
master_secret: "PASTE_LONG_RANDOM_STRING_HERE"
```

Generate the master secret:

```bash
openssl rand -hex 32
```

<Aside type="caution">
The `master_secret` encrypts stored registry passwords and signs JWTs. Losing it means you cannot decrypt those secrets. Back it up somewhere safe (password manager).
</Aside>

## Optional fields you might tweak

```yaml
# Where SQLite + local backups live
data_dir: /var/lib/simpledeploy

# Watched directory: each subdirectory = one app
apps_dir: /etc/simpledeploy/apps

# Reverse proxy listen address
listen_addr: ":443"

# Management API + dashboard port (used internally by Caddy)
management_port: 8443
```

For metrics retention, rate-limit defaults, and registry config, see the full [Configuration reference](/reference/configuration/).

## Create the directories

If they do not already exist:

```bash
sudo mkdir -p /var/lib/simpledeploy
sudo mkdir -p /etc/simpledeploy/apps
```

## Validate

```bash
sudo simpledeploy serve --config /etc/simpledeploy/config.yaml --check
```

(or just start the service: errors print to `journalctl -u simpledeploy`).

Next: [create the admin user](/first-deploy/admin/).
