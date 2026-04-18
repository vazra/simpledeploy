---
title: GitHub Actions
description: Build a Docker image, push to GHCR, and deploy to a remote SimpleDeploy server from GitHub Actions.
---

The CLI talks to the management API over HTTPS using an API key. Store the key as a GitHub Actions secret and call `simpledeploy apply` from a workflow.

## Prerequisites

- A SimpleDeploy server reachable from GitHub runners (or a self-hosted runner inside your network).
- An API key with deploy permission. Generate it under `Settings -> API keys` in the dashboard.
- A `docker-compose.yml` checked into the repo.

## Secrets to set

| Secret | Value |
|--------|-------|
| `SIMPLEDEPLOY_URL` | `https://deploy.example.com` |
| `SIMPLEDEPLOY_TOKEN` | the API key |
| `GHCR_USERNAME` | your GitHub username |
| `GHCR_TOKEN` | a PAT with `write:packages` scope (or use `GITHUB_TOKEN`) |

## Workflow

```yaml
# .github/workflows/deploy.yml
name: Build and deploy

on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push image
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: |
            ghcr.io/${{ github.repository }}:${{ github.sha }}
            ghcr.io/${{ github.repository }}:latest

      - name: Install SimpleDeploy CLI
        run: |
          curl -fsSL https://get.simpledeploy.io | sh
          simpledeploy version

      - name: Configure remote context
        env:
          SD_URL: ${{ secrets.SIMPLEDEPLOY_URL }}
          SD_TOKEN: ${{ secrets.SIMPLEDEPLOY_TOKEN }}
        run: |
          simpledeploy context add prod \
            --url "$SD_URL" \
            --token "$SD_TOKEN"
          simpledeploy context use prod

      - name: Apply compose file
        run: |
          # Substitute the freshly pushed image tag.
          sed -i "s|IMAGE_TAG|${{ github.sha }}|g" docker-compose.yml
          simpledeploy apply -f docker-compose.yml --name myapp --wait
```

## Compose with a tagged image

```yaml
# docker-compose.yml
services:
  web:
    image: ghcr.io/your-org/your-repo:IMAGE_TAG
    labels:
      simpledeploy.domain: app.example.com
    ports:
      - "3000"
```

`sed` rewrites `IMAGE_TAG` to the commit SHA before `apply`. The server pulls the new image, redeploys, and reports back. `--wait` blocks until the deploy is healthy or fails.

## Rollback

`simpledeploy versions <app>` lists previous deploys. Roll back with:

```yaml
- name: Rollback to previous version
  run: simpledeploy rollback myapp --to v42
```

Wire this to a `workflow_dispatch` trigger so you can roll back manually from the Actions tab.

## Tips

- Pin the CLI version in CI: `curl -fsSL https://get.simpledeploy.io | sh -s -- v1.2.0`.
- Use environments (Production, Staging) and store separate `SIMPLEDEPLOY_URL` / `SIMPLEDEPLOY_TOKEN` secrets per environment.
- For matrix deploys to many servers, loop over an array of context names rather than duplicating steps.
