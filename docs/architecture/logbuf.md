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

At server startup, `os.Pipe` is wired between the process's stdout/stderr file descriptors and a `Buffer` (`captureProcessOutput` in `cmd/simpledeploy`). A goroutine per pipe runs `logbuf.Tee`, which copies raw bytes to the original stream and one line at a time into the buffer. This captures everything the binary prints, including third-party library output, and makes it queryable through the API and viewable in the dashboard's "System logs" page.

`Tee` never stops draining the pipe, since a stalled reader would block every write to stdout/stderr. Write errors are ignored. Lines longer than `DefaultTeeLineCap` (256 KiB) are truncated for the buffer only (`LineSplitter`, with `...[truncated]` appended); the original stream gets them unchanged. On a read error other than EOF it stops line capture, keeps forwarding raw bytes, and retries with a backoff of 10ms doubling to 1s. The buffer itself sanitizes each entry: ANSI/OSC escapes and control characters (except tab) are stripped, and entries are capped at 8 KiB.

## Per-app deploy logs

The deployer creates a per-deploy `Buffer`, passes it to the docker compose subprocess, and registers it under the app slug. The API serves recent contents on connect, then streams new entries.

## Container log streams

`/api/apps/{slug}/logs` and `simpledeploy logs` stream container logs straight from Docker, not from a `Buffer`. The container is inspected first. TTY containers emit raw bytes: the CLI copies them as-is and the WebSocket splits them into lines (64 KiB cap per line). Non-TTY streams use Docker's 8-byte frame header and go through `logbuf.DockerStreamReader`, which never trusts the declared size: a frame over `MaxDockerFrameSize` (1 MiB) ends the stream with `ErrDockerFrameTooLarge` instead of being allocated, and malformed headers return `ErrDockerStreamCorrupt`. The WebSocket then sends a short error message to the client.

## WebSocket fan-out

`/api/apps/{slug}/deploy-logs` and `/api/system/process-logs/stream` upgrade to WebSocket and subscribe to the relevant buffer. Slow consumers do not block the producer; if a subscriber's channel fills, the buffer drops messages for that subscriber and emits a warning.

## Lifecycle

Per-deploy buffers are kept until the app is removed or the buffer is GC'd after subscribers disconnect. The system buffer is process-lifetime.
