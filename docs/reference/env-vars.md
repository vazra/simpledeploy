---
title: Environment variables
description: All environment variables read by the simpledeploy binary, with defaults, scope, and effect.
---

SimpleDeploy reads a small number of environment variables. The CLI prefers explicit flags and config file values; env vars are an alternative for non-interactive scripts and CI.

## SimpleDeploy variables

| Name | Default | Scope | Effect |
|------|---------|-------|--------|
| `SD_PASSWORD` | (none) | client | Password used by `users create`, `apikey create`, and `registry add` when the `--password` flag is omitted. Avoids interactive stdin prompt. |
| `SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS` | `0` | server | When set to `1`, the alert webhook dispatcher allows posting to private/loopback IP ranges (RFC 1918, 127.0.0.0/8). Off by default to prevent SSRF. |
| `SIMPLEDEPLOY_UPSTREAM_HOST` | `localhost` | server | Overrides the host used for `localhost:<port>` upstreams. Set to `host.docker.internal` when running SimpleDeploy inside a Docker container (non-host network) so Caddy can reach app host-published ports. The Docker install docs enable this automatically. |
| `SIMPLEDEPLOY_HEALTH_PORT` | `8443` | container (healthcheck only) | Port used by the official Docker image's `HEALTHCHECK` to probe `http://localhost:$SIMPLEDEPLOY_HEALTH_PORT/api/health`. Override when your `management_port` differs from the default (e.g. `make dev-docker` sets `8500`). Not read by the simpledeploy binary itself. |

There is no `SD_CONFIG`, `SD_DATA_DIR`, etc. Pass `--config /path/to/config.yaml` instead. All server settings live in the YAML config (see [Configuration](/reference/configuration/)).

## Variables read indirectly

The Docker SDK and Caddy honor their own standard env vars. SimpleDeploy inherits whatever you set in the `simpledeploy serve` shell or systemd unit.

### Docker SDK (read by `docker` calls)

| Name | Default | Effect |
|------|---------|--------|
| `DOCKER_HOST` | `unix:///var/run/docker.sock` | Daemon socket or TCP endpoint. |
| `DOCKER_TLS_VERIFY` | unset | Enable TLS for the daemon connection. |
| `DOCKER_CERT_PATH` | unset | Directory containing `ca.pem`, `cert.pem`, `key.pem`. |
| `DOCKER_API_VERSION` | negotiated | Pin a specific Docker API version. |

The compose CLI shelled out by the deployer also picks these up, plus `COMPOSE_PROJECT_NAME` (SimpleDeploy sets this per app, do not override).

### Caddy / ACME

Caddy is embedded and uses its standard environment for ACME providers when a custom DNS challenge is needed. None are required for the default HTTP-01 challenge. See the [Caddy docs](https://caddyserver.com/docs/) for provider-specific variables.

### Standard

| Name | Effect |
|------|--------|
| `HOME` | Resolves the client config path `~/.simpledeploy/config.yaml`. |
| `TERM` | Affects `simpledeploy logs` color output. |

`XDG_CONFIG_HOME` is not consulted; the client config path is hard-coded under `$HOME`.

## Examples

Non-interactive admin creation in a provisioning script:

```bash
SD_PASSWORD='strong-password' simpledeploy users create \
  --username admin --role super_admin
```

Allowing webhook posts to a Slack-compatible internal collector during local testing:

```bash
SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS=1 simpledeploy serve --config ./dev.yaml
```

Pointing the deployer at a remote Docker daemon:

```bash
DOCKER_HOST=tcp://docker.internal:2376 \
DOCKER_TLS_VERIFY=1 \
DOCKER_CERT_PATH=/etc/docker/certs \
simpledeploy serve --config /etc/simpledeploy/config.yaml
```

## See also

- [Configuration](/reference/configuration/) for `config.yaml` fields.
- [CLI](/reference/cli/) for flags that override env values.
- [Ports and firewall](/reference/ports/) for what the server listens on.
