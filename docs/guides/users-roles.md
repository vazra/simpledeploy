---
title: Users and roles
description: Create users with role-based access (super_admin, admin, viewer) and scope non-super_admins to specific apps.
---

import { Tabs, TabItem, Aside, Steps } from '@astrojs/starlight/components';

SimpleDeploy ships with three roles. Pick the lowest privilege that gets the job done.

| Role | Dashboard | Granted apps | All apps | User mgmt | System |
|------|-----------|--------------|----------|-----------|--------|
| `viewer` | read | read | - | - | - |
| `admin` | read | read/write | - | - | - |
| `super_admin` | read | read/write | read/write | full | full |

`super_admin` is the only role that can create users and see system pages. Bootstrap your first super_admin during setup.

## Create a user

<Tabs>
<TabItem label="UI">
<Steps>

1. Settings, Users, **Add user**.
2. Pick username, password, role (`admin` or `viewer`).
3. Save.
4. Open the new user, click **Grant access**, pick apps from the list.

</Steps>
</TabItem>
<TabItem label="CLI (server-side)">
```bash
# Interactive prompt for password (recommended)
sudo simpledeploy users create --username alice --role admin

# Or via env var (for CI/scripts)
SD_PASSWORD=hunter2 sudo simpledeploy users create --username alice --role admin
```
The CLI creates the user but does not grant per-app access. Use the UI or API to assign apps.
</TabItem>
<TabItem label="API">
```bash
# Create the user (super_admin token required)
curl -X POST https://manage.example.com/api/users \
  -H "Authorization: Bearer $SD_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"hunter2","role":"admin"}'

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
