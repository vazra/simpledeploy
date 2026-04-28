---
title: Realtime events
description: In-process pub/sub bus and notify-only WebSocket that keeps the UI in sync.
---

SimpleDeploy keeps the dashboard fresh without polling by broadcasting tiny notify-only events over a single per-tab WebSocket. REST stays the source of truth: the server says "something on topic X changed, refetch it" and the UI calls the same `loadX()` it already used on first mount.

## Architecture

```
[handler/worker] -> audit.Recorder.Record() --+
                                              +-> events.Bus.Publish() -> per-conn filter -> WS frame
[handler/worker] -> events.Bus.Publish() -----+

[WS conn] -> realtime.svelte.js -> registry[topic].forEach(refetch)
[mount]   -> registry.register(topic, refetchFn)
[unmount] -> registry.unregister(...)
```

## Components

- `internal/events/bus.go` is the in-process bus. Subscribers get a buffered channel and an unsubscribe function. On overflow the oldest event is dropped and a stale flag is set so the WS handler can emit a synthetic `resync` frame.
- `internal/events/topics.go` defines the topic constants and the audit-category-to-topic mapping.
- `internal/audit/recorder.go` wraps every mutation, then publishes the matching topics best-effort (publish errors never block or fail the originating change).
- `internal/api/events_ws.go` upgrades `GET /api/events`, applies a per-connection topic filter, gates `sub` frames on the caller's role and `user_app_access`, and pings every 30s.
- `ui/src/lib/stores/realtime.svelte.js` opens one WebSocket per tab, queues subscribes until open, and re-subscribes plus refetches all registered fns after every reconnect.

## Topics

| Topic              | Sent on                                                           |
|--------------------|-------------------------------------------------------------------|
| `app:<slug>`       | compose, env, endpoint, lifecycle, deploy, backup, status flips   |
| `global:apps`      | new/removed apps, status flips                                    |
| `global:settings`  | system settings, gitsync config, audit retention                  |
| `global:users`     | users, role changes, access grants/revokes, API keys              |
| `global:registries`| registry CRUD                                                     |
| `global:alerts`    | webhooks, alert rules                                             |
| `global:backups`   | any backup config or run                                          |
| `global:docker`    | docker prune actions                                              |
| `global:audit`     | every audit-recorded mutation                                     |

## Frame format

Client to server (JSON):

```json
{"op":"sub","topic":"app:foo"}
{"op":"unsub","topic":"app:foo"}
{"op":"ping"}
```

Server to client:

```json
{"type":"app.changed","topic":"app:foo","ts":"..."}
{"type":"resync","topic":"*"}
{"op":"err","topic":"app:foo","reason":"forbidden"}
{"op":"pong"}
```

No payload data ever ships in events. The UI runs its existing REST refetch when a frame arrives.

## Authorization

Topic ACL is computed when the WS opens. `super_admin` sees every global topic; everyone else sees `global:apps`, `global:backups`, `global:alerts`, `global:audit` plus `app:<slug>` for any slug they have `user_app_access` on. A `sub` for a forbidden topic returns `{op:"err", reason:"forbidden"}`. When an `access` or `user` audit event affects the connected user, the server closes the socket so the client reconnects with fresh authz.

## Non-goals

- No event replay, no sequence numbers, no payload bodies.
- No multi-replica fan-out (single binary).
- Existing log streaming WebSockets and `connection.svelte.js` health pinger are unchanged.
