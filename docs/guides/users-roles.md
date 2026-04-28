---
title: Users and roles
description: Create users with role-based access (super_admin, manage, viewer) and scope non-super_admins to specific apps.
---

import { Tabs, TabItem, Aside, Steps } from '@astrojs/starlight/components';

SimpleDeploy ships with three roles. Pick the lowest privilege that gets the job done.

| Role | Granted apps (read) | Granted apps (mutate) | Create / delete apps | All apps | User mgmt | System / registries / docker / git sync |
|------|---------------------|-----------------------|----------------------|----------|-----------|------------------------------------------|
| `viewer` | yes | no | no | no | no | no |
| `manage` | yes | yes (start/stop/restart, scale, pull, edit compose+env, redeploy, rollback, backup config + run + restore, per-app alerts/webhooks) | no | no | no | no |
| `super_admin` | yes | yes | yes | yes | yes | yes |

`super_admin` is the only role that can create or delete apps, manage users, or touch platform settings (system, registries, docker, git sync, DB backups). Bootstrap your first super_admin during setup.

`manage` users can do everything a super_admin can do **inside an app they've been granted**, except creation/deletion of the app itself. They cannot see the Users, Registries, Docker, System, or Git Sync pages.

`viewer` users can read everything for granted apps (overview, logs, metrics, events, versions, backup history) but cannot mutate anything.

<Aside type="caution">
Pre-existing `admin` users are migrated to `manage` automatically on upgrade. They keep any per-app access grants they already had. If you want them to manage **all** apps, promote them to `super_admin`.
</Aside>

## Create a user

<Tabs>
<TabItem label="UI">
<Steps>

1. Settings, Users, **Add user**.
2. Pick username, password, role (`manage` or `viewer`).
3. Save.
4. Open the new user, click **Grant access**, pick apps from the list.

</Steps>
</TabItem>
<TabItem label="CLI (server-side)">
```bash
# Interactive prompt for password (recommended)
sudo simpledeploy users create --username alice --role manage

# Or via env var (for CI/scripts)
SD_PASSWORD=hunter2 sudo simpledeploy users create --username alice --role manage
```
The CLI creates the user but does not grant per-app access. Use the UI or API to assign apps.
</TabItem>
<TabItem label="API">
```bash
# Create the user (super_admin token required)
curl -X POST https://manage.example.com/api/users \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"hunter2","role":"manage"}'

# Grant access to myapp
curl -X POST https://manage.example.com/api/users/2/access \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"app_slug":"myapp"}'
```
</TabItem>
</Tabs>

## Per-app access

Non-`super_admin` users see only apps they've been granted. Apps they can't see return `404` (not `403`) so they don't even know they exist.

```bash
# Revoke
curl -X DELETE https://manage.example.com/api/users/2/access/myapp \
  -H "Authorization: Bearer $SD_API_KEY"
```

## Inviting teammates

There is no email invite flow. Workflow:

<Steps>

1. Create the user with a temporary password.
2. Send them the URL plus username + temp password over a private channel.
3. They log in and change the password from the Profile page.

</Steps>

## Removing access

```bash
# Delete the user entirely
curl -X DELETE https://manage.example.com/api/users/2 \
  -H "Authorization: Bearer $SD_API_KEY"
```

This also revokes all their API keys.

<Aside>
Account lockout kicks in after 10 failed logins. See [Security hardening](/operations/security-hardening/) for the backoff schedule.
</Aside>

See also: [API keys](/guides/api-keys/), [Audit log](/guides/audit-log/).
