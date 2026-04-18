---
title: System database backup
description: Back up SimpleDeploy's own SQLite database via VACUUM INTO. WAL-safe atomic snapshots, with optional compact mode.
---

Separate from app backups. Backs up SimpleDeploy's own state (apps, users, configs, metrics). Lives in `internal/store/db_backup.go` and `internal/api/system.go`.

- Uses SQLite `VACUUM INTO` for atomic, consistent copies (WAL-safe)
- Compact mode strips `metrics` and `request_stats` tables before download to reduce size
- Managed from the System page in the UI, not the Backups page

For full disaster-recovery procedures, see [Disaster recovery](/operations/disaster-recovery/).
