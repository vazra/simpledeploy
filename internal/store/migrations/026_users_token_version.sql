-- 026_users_token_version: per-user counter incremented on logout, password
-- change, and role change. JWTs embed the value at issue; middleware rejects
-- tokens whose claim does not match. Lets us invalidate sessions
-- server-side without a denylist table.

ALTER TABLE users ADD COLUMN token_version INTEGER NOT NULL DEFAULT 1;
