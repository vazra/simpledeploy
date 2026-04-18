---
title: Retention and scheduling
description: Cron schedules and retention counts for backup configs. Old backups are pruned automatically after each successful run.
---

## Schedule

Backups use a 5-field cron expression in the `schedule_cron` config field, e.g. `0 2 * * *` for daily at 02:00. The UI Backup wizard provides a visual builder with daily / weekly / monthly / custom modes.

## Retention

`retention_count` keeps the last N successful backup files; older files are deleted after each successful run. Failed runs do not count against retention.

## Backup labels

Compose label shortcuts:

```yaml
labels:
  simpledeploy.backup.schedule: "0 2 * * *"
  simpledeploy.backup.retention: "7"
```

See also: [Backups overview](/guides/backups/overview/).
