---
title: Install on macOS
description: Install the SimpleDeploy CLI on macOS via Homebrew. Use it to manage remote Linux servers from your laptop.
---

import { Aside, Steps } from '@astrojs/starlight/components';

<Aside type="caution" title="macOS is for the CLI, not the server">
SimpleDeploy targets Linux for production. Run the server on a Linux VPS. The macOS install is for the CLI client so you can manage remote servers from your laptop, the same way `kubectl` works.
</Aside>

Running a Linux server? See [Install via Docker](/install/docker/) for a non-Debian path.

## Install

```bash
brew install vazra/tap/simpledeploy
```

Verify:

```bash
simpledeploy version
```

## Use it against a remote server

<Steps>

1. Install and start the server on Linux. See [Ubuntu](/install/ubuntu/) or [Generic Linux](/install/linux/).

2. Create an API key on the server (or in the dashboard):

   ```bash
   # on the server
   simpledeploy apikey create --name "laptop" --user-id 1
   ```

3. Add the context on your Mac:

   ```bash
   simpledeploy context add prod \
     --url https://manage.example.com \
     --api-key sd_paste_key_here
   simpledeploy context use prod
   ```

4. Deploy from your laptop:

   ```bash
   simpledeploy apply -f docker-compose.yml --name myapp
   simpledeploy list
   simpledeploy logs myapp --follow
   ```

</Steps>

## Upgrading

```bash
brew upgrade simpledeploy
```

Next: [First deploy](/first-deploy/prepare/).
