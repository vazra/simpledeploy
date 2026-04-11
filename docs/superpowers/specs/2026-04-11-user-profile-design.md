# User Profile Design

## Summary

Add basic user profile: view/edit display name + email, change password, dedicated profile UI page.

## Database

Migration `012_user_profile.sql`:

```sql
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT '';
```

No unique constraint on email (display only, not used for auth).

## Store Layer

Update `User` struct:

```go
type User struct {
    ID           int64
    Username     string
    PasswordHash string
    Role         string
    DisplayName  string
    Email        string
    CreatedAt    time.Time
}
```

New methods:
- `UpdateProfile(id int64, displayName, email string) error`
- `UpdatePassword(id int64, newHash string) error`

All existing SELECT queries updated to include `display_name, email`.

## API

All endpoints require auth.

### GET /api/me

Returns current user profile:

```json
{
  "id": 1,
  "username": "admin",
  "display_name": "Admin User",
  "email": "admin@example.com",
  "role": "super_admin",
  "created_at": "2026-04-01T00:00:00Z"
}
```

### PUT /api/me

Update display name and email:

```json
{ "display_name": "New Name", "email": "new@example.com" }
```

### PUT /api/me/password

Change password (verifies current first):

```json
{ "current_password": "old", "new_password": "new" }
```

Returns 400 if current password is wrong. Returns 400 if new password is empty.

## UI

New `Profile.svelte` route with:
- Profile info section: display name + email inputs with save button
- Password section: current password, new password, confirm password inputs with change button
- Read-only info: username, role, created at
- Accessible from sidebar link or user menu in layout

Follows existing UI patterns (Layout, Button, SlidePanel components).
