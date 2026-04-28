-- Rework roles: rename 'admin' to 'manage' and tighten the CHECK constraint to
-- {super_admin, manage, viewer}. Existing 'admin' users keep their per-app
-- access grants; super_admin must explicitly grant new manage users.

CREATE TABLE users_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer' CHECK(role IN ('super_admin', 'manage', 'viewer')),
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    display_name TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT ''
);

INSERT INTO users_new (id, username, password_hash, role, created_at, display_name, email)
SELECT id, username, password_hash,
       CASE WHEN role = 'admin' THEN 'manage' ELSE role END,
       created_at, display_name, email
FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
