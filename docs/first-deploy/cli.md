---
title: Deploy via the CLI
description: Use simpledeploy apply from your laptop or CI to push compose files to a remote server. Contexts work like kubectl.
---

The CLI runs in two modes.

| Mode | When | How it works |
|---|---|---|
| **Local** | On the server itself | Reads `--config /etc/simpledeploy/config.yaml`, writes directly to `apps_dir`. |
| **Remote** | From your laptop or CI | Uses an API key + URL stored in `~/.simpledeploy/config.yaml` to call the server's REST API. |

Most people want the remote mode.

## Set up a context

A "context" is a named (URL, API key) pair. Like `kubectl config`.

<Steps>

1. On the server, create an API key (or use one from the dashboard):

   ```bash
   sudo simpledeploy apikey create --name "laptop" --user-id 1
   # prints sd_xxxx once
   ```

2. On your laptop:

   ```bash
   simpledeploy context add prod \
     --url https://manage.example.com \
     --api-key sd_xxxx
   simpledeploy context use prod
   ```

3. Verify:

   ```bash
   simpledeploy list
   ```

</Steps>

## Deploy

```bash
# Single app
simpledeploy apply -f docker-compose.yml --name myapp

# A whole directory of apps (each subdirectory is one app)
simpledeploy apply -d ./apps/
```

The CLI uploads the file, the server writes it to `apps_dir`, the reconciler deploys.

## Other useful commands

```bash
# List apps and status
simpledeploy list

# Stream logs
simpledeploy logs myapp --follow
simpledeploy logs myapp --tail 200 --service db

# Pull latest image, redeploy
simpledeploy pull myapp

# Compare local compose to deployed version
simpledeploy diff -f docker-compose.yml --name myapp

# Push everything in a directory in one shot
simpledeploy sync -d ./apps/
```

<Aside type="tip">
For CI: store the API key as a secret, run `simpledeploy context add ci --url ... --api-key $SD_KEY` in the job, then `simpledeploy apply -f compose.yml --name myapp`.
</Aside>

Full command reference: [CLI](/reference/cli/).
