---
title: Deploy via the dashboard
description: Use the web UI to add a service, paste your compose file, deploy, and watch live logs stream.
---

import { Steps, Aside } from '@astrojs/starlight/components';

The fastest way to deploy if you already have the dashboard open.

<Steps>

1. **Log in** at `https://manage.example.com/`.

2. From the dashboard, click **Add Service** in the top right.

   ![Login screen](/screenshots/login-dark.png)

3. The deploy wizard opens. You can either:

   - **Paste** the YAML directly into the editor, or
   - **Upload** a `docker-compose.yml` from disk.

4. Give the app a **name** (becomes its slug in URLs and CLI). Lowercase letters, digits, dashes.

5. Click **Deploy**. SimpleDeploy:
   - Writes the file to `/etc/simpledeploy/apps/<name>/docker-compose.yml`.
   - Pulls images.
   - Brings the stack up via `docker compose`.
   - Wires routes into Caddy.

6. The **logs panel** streams stdout and stderr live as the deploy runs. Watch for image pulls and container starts.

   ![App detail view](/screenshots/appdetail-dark.png)

7. When the status badge turns **running**, visit your app's domain in a new tab.

</Steps>

## What the dashboard shows you

Once an app is deployed, its detail page has tabs for:

- **Overview**: services, status, resource usage at a glance.
- **Logs**: live stream, follow toggle, per-service filter.
- **Metrics**: CPU, memory, request rate, latency charts.
- **Config**: edit the compose file in-browser, redeploy.
- **Endpoints**: domains, TLS status, advanced proxy settings.
- **Versions**: every deploy attempt with timestamp and result.
- **Backups**: schedule and history.

<Aside type="tip">
Click any chart or screenshot for a zoomed view (image-zoom is enabled site-wide).
</Aside>

## When the UI is the wrong tool

Reach for the [CLI](/first-deploy/cli/) when:

- Deploying from CI (use an API key).
- Managing many apps at once (`apply -d ./apps/`).
- You want the compose file kept in git, not edited in a browser.
