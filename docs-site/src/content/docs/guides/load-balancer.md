---
title: Behind a load balancer
description: Run SimpleDeploy behind Cloudflare, nginx, an ALB, or another TLS-terminating proxy. Disables built-in TLS and uses real client IPs.
---

If running behind Cloudflare, nginx, or another reverse proxy:

1. Set `tls.mode: "off"` in config
2. Set `listen_addr: ":80"` (or whatever port the LB forwards to)
3. The LB handles TLS termination

## Trusted proxies

To preserve real client IPs for rate limiting and account lockout, configure `trusted_proxies`:

```yaml
trusted_proxies:
  - "127.0.0.1"
  - "10.0.0.1"
```

When the direct connection comes from a trusted proxy, the client IP is extracted from `X-Forwarded-For` (rightmost untrusted entry). Without this config, `RemoteAddr` is used directly.

See also: [TLS and HTTPS](/guides/tls/), [Security hardening](/operations/security-hardening/).
