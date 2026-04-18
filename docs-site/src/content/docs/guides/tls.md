---
title: TLS and HTTPS
description: Pick the right TLS mode for SimpleDeploy. Auto Let's Encrypt, custom certs, off (behind LB), or local CA, configured globally and per-endpoint.
---

import { Tabs, TabItem, Aside, Steps } from '@astrojs/starlight/components';

SimpleDeploy embeds Caddy. TLS is configured globally for the management UI and per-endpoint for each app.

## Pick a mode

| Mode | When to use |
|------|-------------|
| `auto` | Public domain with DNS pointing at the server. Caddy gets a free cert from Let's Encrypt. |
| `off` | Behind Cloudflare, an ALB, nginx, or any proxy that terminates TLS upstream. |
| `custom` | Cert issued by a corporate CA, an external Let's Encrypt manager, or for mTLS. |
| `local` | Home labs and dev only. Caddy acts as a local CA, devices must trust its root. |

## Prerequisites for `auto`

<Steps>

1. A public DNS record (`A` or `AAAA`) pointing `myapp.example.com` at the server.
2. TCP ports `80` and `443` open from the internet. Port 80 is needed for the ACME HTTP-01 challenge.
3. A reachable email in `tls.email` so Let's Encrypt can warn you about expiry problems.

</Steps>

<Aside type="caution">
Wildcard certs (`*.example.com`) need DNS-01 and are not supported out of the box. Issue per-subdomain certs or use a custom cert.
</Aside>

## Global config

```yaml
# /etc/simpledeploy/config.yaml
domain: manage.example.com
tls:
  mode: auto
  email: admin@example.com
```

Restart after switching modes: `sudo systemctl restart simpledeploy`.

## Per-endpoint TLS

The endpoint label `simpledeploy.endpoints.N.tls` overrides per app. The shorthand `simpledeploy.tls` works for single-endpoint apps.

<Tabs>
<TabItem label="Auto (default)">
```yaml
services:
  web:
    image: myapp:latest
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
      simpledeploy.endpoints.0.tls: "auto"
```
</TabItem>
<TabItem label="Off (LB upstream)">
```yaml
services:
  web:
    image: myapp:latest
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
      simpledeploy.endpoints.0.tls: "off"
```
</TabItem>
<TabItem label="Custom cert">
```yaml
services:
  web:
    image: myapp:latest
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
      simpledeploy.endpoints.0.tls: "custom"
```
Then upload the PEM cert + key. See [Custom certificates](/guides/custom-certs/).
</TabItem>
</Tabs>

See also: [Behind a load balancer](/guides/load-balancer/), [Custom certificates](/guides/custom-certs/), [Configuration reference](/reference/configuration/).
