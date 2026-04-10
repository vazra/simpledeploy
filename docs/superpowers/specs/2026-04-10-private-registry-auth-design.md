# Private Container Registry Auth

Support pulling images from private registries (Docker Hub, GHCR, ECR, ACR, self-hosted) with username/password credentials managed within SimpleDeploy.

## Data Model

### Migration `010_registries.sql`

```sql
CREATE TABLE registries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    username_enc TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Fields `username_enc` and `password_enc` store AES-GCM encrypted values using `master_secret`.

### Global Config

```yaml
# simpledeploy.yml
registries:
  - ghcr-org
  - my-ecr
```

List of registry names (referencing `registries.name` in DB) applied to all apps by default.

### Per-App Override

Compose label: `simpledeploy.registries=ghcr-org,my-ecr`

Special value `simpledeploy.registries=none` opts out of global defaults.

## Encryption

New `internal/auth/crypto.go`:

- `Encrypt(plaintext, key string) (string, error)` - AES-256-GCM, returns base64-encoded nonce+ciphertext
- `Decrypt(ciphertext, key string) (string, error)` - reverses the above

Key derived from `master_secret` via SHA-256 hash (to get a fixed 32-byte key).

## Store Layer

New methods in `internal/store/`:

- `CreateRegistry(name, url, usernameEnc, passwordEnc string) (*Registry, error)`
- `ListRegistries() ([]Registry, error)`
- `GetRegistry(id string) (*Registry, error)`
- `GetRegistryByName(name string) (*Registry, error)`
- `UpdateRegistry(id, name, url, usernameEnc, passwordEnc string) error`
- `DeleteRegistry(id string) error`

Registry struct:

```go
type Registry struct {
    ID          string
    Name        string
    URL         string
    UsernameEnc string
    PasswordEnc string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

## Pull Flow

In `internal/deployer/deployer.go`, updated `Pull` method:

1. Caller (reconciler) resolves applicable registries: merge global config list with per-app label overrides
2. Caller decrypts credentials, passes `[]deployer.RegistryAuth{URL, Username, Password}` to Pull
3. Deployer builds a temp Docker `config.json`:
   ```json
   {
     "auths": {
       "ghcr.io": {"auth": "base64(user:pass)"},
       "123456.dkr.ecr...": {"auth": "base64(user:pass)"}
     }
   }
   ```
4. Writes to `os.MkdirTemp`, defers cleanup
5. Runs `docker --config <tmpdir> compose -f <path> -p <project> pull`
6. Runs `docker compose up -d` as normal (images already local, no auth needed)

```go
type RegistryAuth struct {
    URL      string
    Username string
    Password string
}
```

## Reconciler Changes

`internal/reconciler/reconciler.go`:

- Add `config *config.Config` and `store Store` fields (store already exists)
- New method `resolveRegistries(app *compose.AppConfig) ([]deployer.RegistryAuth, error)`:
  1. Get global registry names from `config.Registries`
  2. Check app label `simpledeploy.registries` for overrides
  3. If label is "none", return empty
  4. If label set, use those names instead of globals
  5. Look up each name via `store.GetRegistryByName`, decrypt credentials
  6. Return auth list
- Pass auth list to `deployer.Pull(ctx, app, auths)`

## Compose Parser Changes

Add `Registries` field to `AppConfig`:

```go
Registries string // from simpledeploy.registries label
```

Parse it in `parseLabels`.

## API Endpoints

In `internal/api/`:

| Method | Path | Handler | Auth |
|--------|------|---------|------|
| GET | `/api/registries` | List all (passwords redacted) | JWT |
| POST | `/api/registries` | Create | JWT |
| PUT | `/api/registries/{id}` | Update | JWT |
| DELETE | `/api/registries/{id}` | Delete | JWT |

Request body for create/update:

```json
{"name": "ghcr-org", "url": "ghcr.io", "username": "x", "password": "y"}
```

Response (list/get) redacts password:

```json
{"id": "...", "name": "ghcr-org", "url": "ghcr.io", "username": "x", "created_at": "...", "updated_at": "..."}
```

## CLI Commands

In `cmd/simpledeploy/main.go`:

- `simpledeploy registry add --name <name> --url <url> --username <user> --password <pass>`
- `simpledeploy registry list`
- `simpledeploy registry remove <name>`

CLI commands call the local API or store directly depending on mode.

## UI

Settings page: simple table listing registries with add/remove actions. Password field masked on display, shown as input on add form.

## Config Changes

Add to `config.Config`:

```go
Registries []string `yaml:"registries"` // global default registry names
```

## Testing

- `internal/auth/crypto_test.go` - encrypt/decrypt roundtrip, wrong key fails
- `internal/store/` - registry CRUD tests with temp DB
- `internal/deployer/` - mock runner verifies `--config` flag passed, temp dir created/cleaned
- `internal/reconciler/` - registry resolution logic (global, per-app, "none")
- `internal/api/` - registry endpoint tests with httptest

## File Summary

| File | Change |
|------|--------|
| `internal/store/migrations/010_registries.sql` | New migration |
| `internal/store/registry.go` | New: CRUD methods |
| `internal/auth/crypto.go` | New: Encrypt/Decrypt |
| `internal/auth/crypto_test.go` | New: tests |
| `internal/compose/parser.go` | Add Registries field + label parsing |
| `internal/deployer/deployer.go` | Update Pull to accept auths, build temp config.json |
| `internal/deployer/deployer_test.go` | Test auth flow |
| `internal/reconciler/reconciler.go` | Add resolveRegistries, pass to Pull |
| `internal/config/config.go` | Add Registries field |
| `internal/api/registries.go` | New: CRUD handlers |
| `internal/api/routes.go` | Register new routes |
| `cmd/simpledeploy/main.go` | Add registry subcommands |
| `ui/src/routes/settings/` | Registry management UI |
