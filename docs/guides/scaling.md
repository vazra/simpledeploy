---
title: Scaling services
description: Run multiple replicas of a compose service on the same host. Useful for stateless web/worker tiers, with caveats for stateful services.
---

import { Tabs, TabItem, Aside } from '@astrojs/starlight/components';

Run more than one replica of a compose service. SimpleDeploy shells out to `docker compose up --scale` and the built-in proxy round-robins requests across the replicas.

## Use cases

- Stateless web tier handling more concurrent requests.
- Background worker pool processing a queue faster.
- Quick capacity bump during a traffic spike, scale down after.

## Compose setup

Skip the `container_name` field (Compose can't create N containers with the same name) and let the service expose a single internal port.

```yaml
services:
  web:
    image: myapp:latest
    labels:
      simpledeploy.endpoints.0.domain: "myapp.example.com"
      simpledeploy.endpoints.0.port: "3000"
    restart: unless-stopped
  worker:
    image: myapp-worker:latest
```

## Scale it

<Tabs>
<TabItem label="UI">
App page, Services tab, click the **+/-** next to a service. Apply.
</TabItem>
<TabItem label="API">
```bash
curl -X POST https://manage.example.com/api/apps/myapp/scale \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"scales": {"web": 3, "worker": 2}}'
```
</TabItem>
</Tabs>

The proxy rebuilds its upstream pool and starts load-balancing across all replicas immediately.

## Limits and gotchas

<Aside type="caution">
Single host. SimpleDeploy is not an orchestrator. Replicas all run on the same machine, so they share CPU, memory, and disk. Use a real LB plus multiple SimpleDeploy hosts if you need cross-machine HA.
</Aside>

- **No sticky sessions.** Round-robin only. Store sessions in Redis or a signed cookie, not in-process memory.
- **Don't scale stateful services.** Scaling Postgres/MySQL/Redis past 1 will corrupt data. Apps that hold a file lock or bind a host port will fail to start past replica 2.
- **Host port mappings break past 1 replica.** If your service has `ports: ["8080:8080"]`, only one replica can bind. Use the `expose:` field plus the `simpledeploy.endpoints.*` labels for proxy routing instead.
- **Healthchecks recommended.** Compose `healthcheck:` lets the proxy skip unhealthy replicas during rolling restarts.

See also: [REST API](/reference/api/), [Compose labels](/reference/compose-labels/).
