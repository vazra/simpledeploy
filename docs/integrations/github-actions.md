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
      - uses: actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4.4.0

      - name: Log in to GHCR
        uses: docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3.7.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push image
        uses: docker/build-push-action@263435318d21b8e681c14492fe198d362a7d2c83 # v6.18.0
        with:
          context: .
          push: true
          tags: |
            ghcr.io/${{ github.repository }}:${{ github.sha }}
            ghcr.io/${{ github.repository }}:latest

      - name: Install SimpleDeploy CLI
        env:
          SD_VERSION: "1.4.3" # x-release-please-version
        run: |
          base="https://github.com/vazra/simpledeploy/releases/download/v${SD_VERSION}"
          file="simpledeploy_${SD_VERSION}_linux_amd64.tar.gz"
          curl -fsSLO "${base}/${file}"
          curl -fsSLO "${base}/checksums.txt"
          # Fails the step unless the tarball matches the release's checksums.txt.
          grep " ${file}$" checksums.txt | sha256sum -c -
          tar xzf "$file" simpledeploy
          sudo install -m 0755 simpledeploy /usr/local/bin/simpledeploy
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

- The CLI is pinned to a release with `SD_VERSION`. The step downloads that [release](https://github.com/vazra/simpledeploy/releases)'s `checksums.txt` and checks the tarball against it, so a corrupted or truncated download fails the step (run scripts use `bash -e`). To upgrade, bump `SD_VERSION`; there is no checksum to copy by hand.
- The example pins every action to a full commit SHA, with the release tag in a trailing comment. Keep that pattern when you add or bump actions; Dependabot and Renovate update the SHA and the comment together.
- Use environments (Production, Staging) and store separate `SIMPLEDEPLOY_URL` / `SIMPLEDEPLOY_TOKEN` secrets per environment.
- For matrix deploys to many servers, loop over an array of context names rather than duplicating steps.
