# IP Access Allowlist

Per-app IP allowlisting via a new Caddy handler module. When active, only requests from allowed IPs/CIDRs reach the app. Non-matching requests get 404.

## Compose Label

```yaml
simpledeploy.access.allow: "10.0.0.0/8,192.168.1.0/24,203.0.113.5"
```

- Comma-separated IPs and CIDR ranges
- Absent or empty = all traffic allowed (no restriction)
- Single source of truth: the compose file label
- UI/API edits write to the compose file (same pattern as domain, port, etc.)

## Label Parsing

**`internal/compose/parser.go`**
- New field `AccessAllow string` on `AppConfig`
- Extracted from `simpledeploy.access.allow`

**`internal/proxy/route.go`**
- New field `AllowedIPs []string` on `Route`
- `ResolveRoute` splits comma-separated string, trims whitespace
- Validates each entry with `net.ParseIP` / `net.ParseCIDR`
- Invalid entries logged as warnings and skipped (don't break the whole allowlist)
- API endpoint rejects invalid entries before writing (strict validation at write time)

## Caddy Module: `simpledeploy_ipaccess`

New file: `internal/proxy/ipaccess.go`

### Registration
- Module ID: `http.handlers.simpledeploy_ipaccess`
- Registered via `init()` (same pattern as ratelimit/metrics modules)

### Global Registry
- `IPAccessRules` - maps domain to parsed allowlist
- Each allowlist pre-parsed into `[]net.IPNet` + `[]net.IP` at registration time
- Updated when `SetRoutes` is called (same as rate limiters)

### ServeHTTP Logic
1. Look up rules by `r.Host`
2. No rules for domain = pass through (no restriction)
3. Extract client IP from `r.RemoteAddr` via `net.SplitHostPort`
4. Check against allowlist: exact IP match, then CIDR contains
5. Allowed = call `next.ServeHTTP`
6. Not allowed = return 404 with generic "not found" body

### Pipeline Order
```
simpledeploy_ipaccess -> simpledeploy_ratelimit -> simpledeploy_metrics -> reverse_proxy
```

Blocked IPs never hit rate limiting or metrics.

## API

**`PUT /api/apps/{slug}/access`**
- Body: `{"allow": "10.0.0.0/8,192.168.1.5"}` (or `""` to disable)
- Validates all entries are valid IPs/CIDRs before writing
- Updates `simpledeploy.access.allow` label in compose file
- Triggers reconcile to reload proxy routes
- Returns updated app config

**Read path:** Existing `GET /api/apps/{slug}` returns labels, UI reads allowlist from there.

## UI

IP Access section in app detail/settings:
- Text input showing current comma-separated allowlist
- Save button calls `PUT /api/apps/{slug}/access`
- Validation feedback for malformed IPs/CIDRs
- Empty state shows "All traffic allowed"
- When populated, shows parsed entry list

## Testing

### `proxy/ipaccess_test.go`
- No rules for domain = pass through
- Exact IP match = allowed
- CIDR match = allowed
- Non-matching IP = 404
- Empty allowlist = pass through
- Invalid RemoteAddr handling

### `compose/parser_test.go`
- `simpledeploy.access.allow` label extraction

### `proxy/route_test.go`
- `ResolveRoute` parses and validates IP entries
- Rejects invalid IPs/CIDRs

### `api/` tests
- PUT endpoint: valid input, invalid IPs, empty string, reconcile triggered
