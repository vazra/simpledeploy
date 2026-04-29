-- 027_apikeys_last_used: add a last_used_at column so operators can spot
-- stale API keys in the dashboard. Lazy-updated by GetAPIKeyByHash.
ALTER TABLE api_keys ADD COLUMN last_used_at DATETIME;
