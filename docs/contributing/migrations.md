---
title: Migrations and data changes
description: Adding SQLite migrations safely, naming, and forward-compatibility rules.
---

Migrations live in `internal/store/migrations/` and are embedded into the binary via `go:embed`. The store runs them in numeric order on startup.

## Adding a migration

1. Pick the next sequence number (look at the existing files; current high water mark is 17).
2. Create `NNN_short_topic.sql`. Example: `017_add_user_avatar_url.sql`.
3. Write idempotent SQL. Always include `IF NOT EXISTS` on tables and indexes when feasible.
4. Run the suite: `make test`. The store tests apply all migrations on a fresh DB.
5. If you add a column, add a default so existing rows are valid.
6. Update `CLAUDE.md` and `docs/architecture/store.md` with the new entry.

## Forward-compatibility rules

Migrations are forward-only. We need a freshly built old binary to be able to roll back without crashing on a database touched by a newer binary.

- **Allowed**: add tables, add nullable columns, add columns with defaults, add indexes.
- **Discouraged**: drop columns, drop tables. If absolutely required, do it in two releases (stop reading first, then drop in a later release).
- **Never**: rename columns or tables. Add the new name, dual-write, drop the old later.
- **Avoid**: data backfills inside migrations that scan the whole table. If you need one, gate it on size or run as a background job from Go code.

## Schema introspection

The store package wraps SQL in typed methods (`Upsert*`, `List*`, etc.) so the rest of the codebase never depends on raw SQL. When you change a table, update all callers.

## Testing

- `go test ./internal/store/...` exercises every migration on a fresh database and runs basic CRUD against the new schema.
- Add integration tests that exercise the new behavior end-to-end.

## Production rollouts

Migrations run automatically on first launch after upgrade. They are short and online (no downtime), assuming you follow the forward-compat rules. Always backup the database first (see [Upgrade and rollback](/operations/upgrade-rollback/)).
