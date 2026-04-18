---
title: Terraform
description: Manage SimpleDeploy from Terraform. Community module stub.
---

There is no official Terraform provider yet. Below is what one could look like. Contributions welcome at [github.com/vazra/simpledeploy](https://github.com/vazra/simpledeploy).

## Two layers

A useful integration has two parts:

1. **Provisioning** the host (use any cloud provider's resources to create a VPS, render `/etc/simpledeploy/config.yaml`, install the package via `cloud-init` or `remote-exec`).
2. **Managing apps** (declare apps, registries, users, alert rules, webhooks as Terraform resources backed by a SimpleDeploy provider that talks to the REST API).

The first layer is straightforward Terraform. The second needs a custom provider.

## Sketch: provisioning a SimpleDeploy host

```hcl
resource "hcloud_server" "deploy" {
  name        = "deploy-1"
  image       = "ubuntu-24.04"
  server_type = "cx21"
  user_data   = templatefile("${path.module}/cloud-init.yaml", {
    master_secret = var.sd_master_secret
    tls_email     = var.tls_email
  })
}

resource "hcloud_firewall" "web" {
  name = "web"
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "80"
    source_ips = ["0.0.0.0/0", "::/0"]
  }
  rule {
    direction  = "in"
    protocol   = "tcp"
    port       = "443"
    source_ips = ["0.0.0.0/0", "::/0"]
  }
}
```

`cloud-init.yaml` should install the SimpleDeploy `.deb`, render `config.yaml`, and start the service.

## Sketch: a SimpleDeploy provider

A real provider would expose:

```hcl
provider "simpledeploy" {
  url     = "https://manage.example.com"
  api_key = var.sd_api_key
}

resource "simpledeploy_app" "myapp" {
  name           = "myapp"
  compose_yaml   = file("compose/myapp.yml")
}

resource "simpledeploy_registry" "ghcr" {
  name     = "ghcr"
  url      = "ghcr.io"
  username = var.ghcr_user
  password = var.ghcr_token
}

resource "simpledeploy_alert_rule" "high_cpu" {
  metric    = "cpu"
  op        = ">"
  threshold = 85
  duration  = "5m"
}
```

The provider would map these to the REST API documented at [the API reference](/reference/api/).

## Want to publish a provider?

The API is stable enough to wrap. Open an issue describing your scope and we will help review.
