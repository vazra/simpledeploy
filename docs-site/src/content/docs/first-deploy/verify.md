---
title: Verify the deployment
description: Confirm the app is running, HTTPS works, logs stream, metrics flow, and set up your first alert.
---

import { Steps, Aside } from '@astrojs/starlight/components';

Five checks to make sure the deploy actually worked.

<Steps>

1. **Hit the domain.**

   ```bash
   curl -I https://whoami.example.com/
   ```

   Expect `HTTP/2 200`. The TLS cert is from Let's Encrypt; your browser will trust it without warnings.

2. **Verify HTTPS, not HTTP.**

   ```bash
   curl -I http://whoami.example.com/
   ```

   You should get a `308` redirect to `https://`. Caddy auto-redirects.

3. **Tail the app logs.**

   ```bash
   simpledeploy logs whoami --follow
   ```

   Hit the domain a few times in a browser, watch the requests appear.

   Or in the dashboard: app page &rarr; **Logs** tab.

4. **Check metrics.**

   In the dashboard, open the app and click **Metrics**. CPU and memory should be plotted within ~30 seconds. Request rate appears once traffic hits the proxy.

   ![Metrics view](/screenshots/system-dark.png)

5. **Set up an alert.**

   Go to **Alerts &rarr; Rules &rarr; New rule**. Pick the app, set CPU > 80% for 5 min, attach a webhook (Slack works out of the box). See [Alert rules](/guides/alerts/rules/) for details.

</Steps>

## Common problems

**Cert error / "not secure" in the browser.**
Wait 30-60 seconds. Caddy is provisioning. If it persists, check that DNS resolves to the box and port 80 is open from the public internet.

**`simpledeploy logs` shows nothing.**
The container may have exited. Run `simpledeploy list` and check status. Then `docker logs <container>` on the server for the raw output.

**`502 Bad Gateway`.**
The container is up but not listening on the port you set in `simpledeploy.port`. Double-check the label matches the port the app actually binds.

<Aside type="note">
For deeper troubleshooting: [Operations &rarr; Troubleshooting](/operations/troubleshooting/).
</Aside>

## You are done

That is the full happy path. Next, dig in:

- [Add backups](/guides/backups/overview/) for stateful services.
- [Invite teammates](/guides/users/roles/) and scope per-app access.
- [Wire up a private registry](/guides/registries/) for non-public images.
- [Front SimpleDeploy with a load balancer](/guides/load-balancer/) if you outgrow one VPS.
