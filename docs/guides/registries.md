---
title: Private registries
description: Add credentials for ghcr.io, Docker Hub, AWS ECR, Azure ACR, or any registry. Encrypted at rest with master_secret.
---

import { Tabs, TabItem, Aside } from '@astrojs/starlight/components';

Add credentials so SimpleDeploy can pull private images. Credentials are encrypted with AES-256-GCM derived from `master_secret` before being stored.

## GitHub Container Registry (ghcr.io)

Create a Personal Access Token with the `read:packages` scope, then:

<Tabs>
<TabItem label="CLI">
```bash
simpledeploy registry add \
  --name ghcr \
  --url ghcr.io \
  --username my-github-user \
  --password $(cat ~/.ghcr-token)
```
</TabItem>
<TabItem label="UI">
Settings, Registries, **Add registry**. Name `ghcr`, URL `ghcr.io`, username, paste the token.
</TabItem>
<TabItem label="API">
```bash
curl -X POST https://manage.example.com/api/registries \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"ghcr","url":"ghcr.io","username":"my-github-user","password":"ghp_..."}'
```
</TabItem>
</Tabs>

## Docker Hub

Create a Hub Access Token (Account Settings, Security). Username is your Hub username.

```bash
simpledeploy registry add \
  --name dockerhub \
  --url docker.io \
  --username my-hub-user \
  --password dckr_pat_xxxxx
```

## ECR / ACR / GCR / self-hosted

Same shape, swap the URL.

```bash
# AWS ECR (use an IAM user with ecr:GetAuthorizationToken or rotate via cron)
simpledeploy registry add \
  --name my-ecr \
  --url 123456.dkr.ecr.us-east-1.amazonaws.com \
  --username AWS \
  --password "$(aws ecr get-login-password --region us-east-1)"

# Azure Container Registry
simpledeploy registry add --name acr --url myreg.azurecr.io --username myreg --password $ACR_PASSWORD

# Self-hosted Harbor / Gitea
simpledeploy registry add --name harbor --url registry.example.com --username robot$ci --password $HARBOR_TOKEN
```

## Tell apps which registries to use

Set globally in `config.yaml`:

```yaml
registries:
  - ghcr
  - dockerhub
```

Or per-app via compose label (overrides the global list):

```yaml
services:
  web:
    image: ghcr.io/myorg/myapp:latest
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
      simpledeploy.registries: "ghcr"
```

Use the special value `none` to disable all registries (including global) for an app that pulls only public images.

<Aside>
Passwords are write-only via the API and never returned. To rotate, `PUT /api/registries/{id}` with the new password, or just re-add and remove the old.
</Aside>

See also: [Compose labels](/reference/compose-labels/), [CLI reference](/reference/cli/).
