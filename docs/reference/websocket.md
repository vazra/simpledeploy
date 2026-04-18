---
title: WebSocket endpoints
description: Live streams for deploy progress, container logs, and server process logs. Authentication, message format, close codes, and reconnect guidance.
---

SimpleDeploy exposes three WebSocket endpoints for live data. They share the
same auth model as the REST API and the same `/api` prefix.

## Authentication

WebSockets are upgraded HTTP requests, so they use the same auth headers and
cookies as REST calls:

- **Browser clients**: the `session` cookie set by `POST /api/auth/login` is
  sent automatically; nothing extra needed.
- **Programmatic clients**: send `Authorization: Bearer sd_<token>` on the
  HTTP upgrade request, or include the cookie manually.

Same-origin requests are accepted. Cross-origin upgrades are rejected at the
`Origin` check.

## Connection lifecycle

- The server sets a 5 minute read deadline. Any client message (including pings)
  resets it. Send a WebSocket ping every 30 to 60 seconds to stay connected.
- The server closes the socket cleanly when the underlying stream ends (deploy
  finishes, container exits, server shuts down).
- Treat unexpected disconnects as transient. Reconnect with exponential backoff
  (start at 1s, cap at 30s). Re-fetch any state you may have missed via the
  REST API.

## Common close codes

| Code | Meaning |
| ---- | ------- |
| 1000 | Normal close. Stream finished or client closed. |
| 1001 | Server going away (restart, shutdown). |
| 1006 | Abnormal close. Network error; reconnect. |
| 1008 | Policy violation. Origin check failed. |
| 1011 | Server error. Check process logs. |

---

## GET /api/apps/{slug}/deploy-logs

Streams `docker compose` output for the most recent or in-flight deploy of an
app. Connect immediately after `POST /api/apps/deploy` (or any redeploy
trigger) to watch progress in real time.

### Query parameters

None.

### Message shape

Each frame is a JSON object:

```json
{
  "timestamp": "2026-04-17T10:14:32.118Z",
  "action": "pull",
  "output": "Pulling web (nginx:1.27)...",
  "done": false
}
```

| Field | Type | Notes |
| ----- | ---- | ----- |
| `timestamp` | string | RFC 3339 server timestamp. |
| `action` | string | `pull`, `up`, `down`, `restart`, `scale`, or `none`. |
| `output` | string | Raw line from `docker compose`. |
| `done` | boolean | `true` on the final frame; the server then closes the socket. |

If no deploy is currently running and none starts within 3 seconds, the server
sends a single `{"done": true, "action": "none"}` frame and closes.

### Example

```js
const ws = new WebSocket(`wss://${host}/api/apps/my-app/deploy-logs`);
ws.onmessage = (e) => {
  const msg = JSON.parse(e.data);
  if (msg.done) {
    console.log("deploy finished");
    return;
  }
  console.log(`[${msg.action}] ${msg.output}`);
};
```

---

## GET /api/apps/{slug}/logs

Streams stdout/stderr from one container of an app, sourced from the Docker
log driver.

### Query parameters

| Name | Default | Notes |
| ---- | ------- | ----- |
| `service` | `web` | Compose service name to tail. |
| `tail` | `100` | Number of historical lines to send before live tail. |
| `follow` | `true` | Set to `false` to fetch only the historical tail and close. |
| `since` | empty | RFC 3339 timestamp or duration like `10m`. |

### Message shape

```json
{
  "ts": "2026-04-17T10:15:01.444Z",
  "stream": "stdout",
  "line": "request 200 GET /healthz"
}
```

| Field | Type | Notes |
| ----- | ---- | ----- |
| `ts` | string | Timestamp parsed from the Docker log line, when present. |
| `stream` | string | `stdout` or `stderr`. |
| `line` | string | Single log line, trailing newline removed. |

If the container cannot be located, the server sends one frame
`{"error": "container not found"}` and closes.

### Example

```js
const url = new URL(`wss://${host}/api/apps/my-app/logs`);
url.searchParams.set("service", "api");
url.searchParams.set("tail", "200");
const ws = new WebSocket(url);
ws.onmessage = (e) => {
  const { stream, line, ts } = JSON.parse(e.data);
  console.log(`${ts ?? ""} [${stream}] ${line}`);
};
```

---

## GET /api/system/process-logs/stream

Streams the SimpleDeploy server's own stdout and stderr from the in-memory
ring buffer used by the dashboard's process log viewer.

### Query parameters

None. The buffer's recent contents are flushed on connect, then live lines
follow.

### Message shape

Each frame is a single log line as a JSON string:

```json
"2026-04-17T10:14:32.118Z [api] handled GET /api/apps in 4.2ms"
```

Lines are emitted exactly as written to the ring buffer (timestamp prefix,
component tag, message).

### Example

```js
const ws = new WebSocket(`wss://${host}/api/system/process-logs/stream`);
ws.onmessage = (e) => {
  const line = JSON.parse(e.data);
  console.log(line);
};
```

## Reconnect guidance

A small reusable helper for any of the streams:

```js
function connect(url, onMessage) {
  let backoff = 1000;
  const open = () => {
    const ws = new WebSocket(url);
    ws.onmessage = (e) => onMessage(JSON.parse(e.data));
    ws.onopen = () => { backoff = 1000; };
    ws.onclose = (ev) => {
      if (ev.code === 1000) return; // clean close, do not retry
      setTimeout(open, backoff);
      backoff = Math.min(backoff * 2, 30_000);
    };
  };
  open();
}
```

For request authentication in the browser, the `session` cookie is sent
automatically. For Node clients (e.g. the `ws` package), pass the
`Authorization` header in the upgrade options:

```js
import WebSocket from "ws";

const ws = new WebSocket(`wss://${host}/api/apps/my-app/logs`, {
  headers: { Authorization: `Bearer ${process.env.SIMPLEDEPLOY_TOKEN}` },
});
```
