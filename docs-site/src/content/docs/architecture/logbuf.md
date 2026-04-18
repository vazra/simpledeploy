---
title: Log ring buffer
description: In-process log capture with WebSocket fan-out for live dashboards.
---

The `internal/logbuf/` package provides an `io.Writer`-shaped ring buffer used to capture process output and stream it to UI subscribers without blocking the producer.

## Why a ring buffer

`docker compose pull/up` and the SimpleDeploy server itself produce bursty output. The dashboard wants both history (so a late-joining tab sees what happened) and a live feed. A bounded ring buffer gives both with constant memory.

## Shape

A `Buffer` holds up to `N` entries (default 500, configurable via `log_buffer_size`). Each entry is `{timestamp, line}`. Write appends; the oldest entry is dropped when full. Subscribers receive new entries via a Go channel.

The buffer satisfies `io.Writer`, so it can be passed wherever the standard library wants a writer (e.g., `cmd.Stdout`, `cmd.Stderr` in the deployer).

## Process log capture

At server startup, `os.Pipe` is wired between the process's stdout/stderr file descriptors and a `Buffer`. A goroutine reads the pipe and forwards lines into the buffer. This captures everything the binary prints, including third-party library output, and makes it queryable through the API and viewable in the dashboard's "System logs" page.

## Per-app deploy logs

The deployer creates a per-deploy `Buffer`, passes it to the docker compose subprocess, and registers it under the app slug. The API serves recent contents on connect, then streams new entries.

## WebSocket fan-out

`/api/apps/{slug}/deploy-logs` and `/api/apps/{slug}/logs` upgrade to WebSocket and subscribe to the relevant buffer. Slow consumers do not block the producer; if a subscriber's channel fills, the buffer drops messages for that subscriber and emits a warning.

## Lifecycle

Per-deploy buffers are kept until the app is removed or the buffer is GC'd after subscribers disconnect. The system buffer is process-lifetime.
