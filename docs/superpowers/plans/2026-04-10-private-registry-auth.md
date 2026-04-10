# Private Registry Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Support pulling images from private container registries with encrypted credential storage.

**Architecture:** Credentials stored encrypted in SQLite using AES-256-GCM keyed from `master_secret`. At pull time, a temp Docker `config.json` is generated with decrypted auths and passed via `docker --config <tmpdir>`. Global defaults in config, per-app overrides via compose labels.

**Tech Stack:** Go stdlib `crypto/aes`, `crypto/cipher`, `crypto/sha256`, `encoding/base64`

---

### Task 1: Crypto helpers (Encrypt/Decrypt)

**Files:**
- Create: `internal/auth/crypto.go`
- Create: `internal/auth/crypto_test.go`

- [ ] **Step 1: Write failing tests**

```go
// internal/auth/crypto_test.go
package auth

import "testing"

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key := "my-secret-key-for-testing"
	plaintext := "hunter2"

	encrypted, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if encrypted == plaintext {
		t.Fatal("encrypted should differ from plaintext")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	encrypted, err := Encrypt("secret", "key1")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	_, err = Decrypt(encrypted, "key2")
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestEncryptEmptyString(t *testing.T) {
	encrypted, err := Encrypt("", "key")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	decrypted, err := Decrypt(encrypted, "key")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if decrypted != "" {
		t.Errorf("got %q, want empty", decrypted)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth/ -run TestEncrypt -v`
Expected: compilation error, `Encrypt` undefined

- [ ] **Step 3: Implement Encrypt/Decrypt**

```go
// internal/auth/crypto.go
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// Encrypt encrypts plaintext with AES-256-GCM using a SHA-256 hash of key.
// Returns base64-encoded nonce+ciphertext.
func Encrypt(plaintext, key string) (string, error) {
	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decrypts a base64-encoded nonce+ciphertext with AES-256-GCM.
func Decrypt(encoded, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(deriveKey(key))
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

func deriveKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run TestEncrypt -v`
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/auth/crypto.go internal/auth/crypto_test.go
git commit -m "feat(auth): add AES-256-GCM encrypt/decrypt helpers"
```

---

### Task 2: Database migration and store CRUD

**Files:**
- Create: `internal/store/migrations/010_registries.sql`
- Create: `internal/store/registry.go`
- Create: `internal/store/registry_test.go`

- [ ] **Step 1: Write the migration**

```sql
-- internal/store/migrations/010_registries.sql
CREATE TABLE IF NOT EXISTS registries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    url TEXT NOT NULL,
    username_enc TEXT NOT NULL,
    password_enc TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 2: Write failing store tests**

```go
// internal/store/registry_test.go
package store

import (
	"os"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndListRegistries(t *testing.T) {
	db := openTestDB(t)

	reg, err := db.CreateRegistry("ghcr", "ghcr.io", "enc-user", "enc-pass")
	if err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}
	if reg.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if reg.Name != "ghcr" {
		t.Errorf("Name = %q, want ghcr", reg.Name)
	}

	regs, err := db.ListRegistries()
	if err != nil {
		t.Fatalf("ListRegistries: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("got %d registries, want 1", len(regs))
	}
	if regs[0].URL != "ghcr.io" {
		t.Errorf("URL = %q, want ghcr.io", regs[0].URL)
	}
}

func TestGetRegistryByName(t *testing.T) {
	db := openTestDB(t)
	db.CreateRegistry("ecr", "123.dkr.ecr.us-east-1.amazonaws.com", "u", "p")

	reg, err := db.GetRegistryByName("ecr")
	if err != nil {
		t.Fatalf("GetRegistryByName: %v", err)
	}
	if reg.URL != "123.dkr.ecr.us-east-1.amazonaws.com" {
		t.Errorf("URL = %q", reg.URL)
	}
}

func TestUpdateRegistry(t *testing.T) {
	db := openTestDB(t)
	reg, _ := db.CreateRegistry("test", "old.io", "u", "p")

	err := db.UpdateRegistry(reg.ID, "test2", "new.io", "u2", "p2")
	if err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}

	updated, _ := db.GetRegistry(reg.ID)
	if updated.Name != "test2" || updated.URL != "new.io" {
		t.Errorf("got name=%q url=%q", updated.Name, updated.URL)
	}
}

func TestDeleteRegistry(t *testing.T) {
	db := openTestDB(t)
	reg, _ := db.CreateRegistry("del", "del.io", "u", "p")

	if err := db.DeleteRegistry(reg.ID); err != nil {
		t.Fatalf("DeleteRegistry: %v", err)
	}
	regs, _ := db.ListRegistries()
	if len(regs) != 0 {
		t.Errorf("got %d registries after delete", len(regs))
	}
}

func TestCreateRegistryDuplicateName(t *testing.T) {
	db := openTestDB(t)
	db.CreateRegistry("dup", "a.io", "u", "p")
	_, err := db.CreateRegistry("dup", "b.io", "u", "p")
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/store/ -run TestCreateAndList -v`
Expected: compilation error, `CreateRegistry` undefined

- [ ] **Step 4: Implement store CRUD**

```go
// internal/store/registry.go
package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type Registry struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	UsernameEnc string    `json:"-"`
	PasswordEnc string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) CreateRegistry(name, url, usernameEnc, passwordEnc string) (*Registry, error) {
	id := newID()
	_, err := s.db.Exec(`
		INSERT INTO registries (id, name, url, username_enc, password_enc)
		VALUES (?, ?, ?, ?, ?)`,
		id, name, url, usernameEnc, passwordEnc,
	)
	if err != nil {
		return nil, fmt.Errorf("insert registry: %w", err)
	}
	return s.GetRegistry(id)
}

func (s *Store) ListRegistries() ([]Registry, error) {
	rows, err := s.db.Query(`SELECT id, name, url, username_enc, password_enc, created_at, updated_at FROM registries ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("query registries: %w", err)
	}
	defer rows.Close()

	var regs []Registry
	for rows.Next() {
		var r Registry
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.UsernameEnc, &r.PasswordEnc, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan registry: %w", err)
		}
		regs = append(regs, r)
	}
	return regs, rows.Err()
}

func (s *Store) GetRegistry(id string) (*Registry, error) {
	var r Registry
	err := s.db.QueryRow(`SELECT id, name, url, username_enc, password_enc, created_at, updated_at FROM registries WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.URL, &r.UsernameEnc, &r.PasswordEnc, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get registry: %w", err)
	}
	return &r, nil
}

func (s *Store) GetRegistryByName(name string) (*Registry, error) {
	var r Registry
	err := s.db.QueryRow(`SELECT id, name, url, username_enc, password_enc, created_at, updated_at FROM registries WHERE name = ?`, name).
		Scan(&r.ID, &r.Name, &r.URL, &r.UsernameEnc, &r.PasswordEnc, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get registry by name: %w", err)
	}
	return &r, nil
}

func (s *Store) UpdateRegistry(id, name, url, usernameEnc, passwordEnc string) error {
	_, err := s.db.Exec(`
		UPDATE registries SET name = ?, url = ?, username_enc = ?, password_enc = ?, updated_at = datetime('now')
		WHERE id = ?`,
		name, url, usernameEnc, passwordEnc, id,
	)
	if err != nil {
		return fmt.Errorf("update registry: %w", err)
	}
	return nil
}

func (s *Store) DeleteRegistry(id string) error {
	_, err := s.db.Exec(`DELETE FROM registries WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete registry: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/store/ -run TestRegistry -v`
Expected: all 5 tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/store/migrations/010_registries.sql internal/store/registry.go internal/store/registry_test.go
git commit -m "feat(store): add registries table and CRUD methods"
```

---

### Task 3: Config + compose label parsing

**Files:**
- Modify: `internal/config/config.go:9-19` (add Registries field)
- Modify: `internal/compose/parser.go:12-29` (add Registries to AppConfig)
- Modify: `internal/compose/parser.go:59-71` (add Registries to LabelConfig)
- Modify: `internal/compose/parser.go:140-158` (extract Registries label)
- Modify: `internal/compose/parser.go:115-130` (set Registries in ParseFile)

- [ ] **Step 1: Add Registries to Config**

In `internal/config/config.go`, add after `RateLimit` field:

```go
Registries []string `yaml:"registries"`
```

- [ ] **Step 2: Add Registries to AppConfig and LabelConfig**

In `internal/compose/parser.go`, add `Registries string` field to `AppConfig` (after `PathPatterns`):

```go
Registries      string
```

Add `Registries string` to `LabelConfig` (after `PathPatterns`):

```go
Registries string
```

- [ ] **Step 3: Extract label and assign**

In `ExtractLabels`, add:

```go
Registries: labels["simpledeploy.registries"],
```

In `ParseFile`, in the cfg assignment block, add:

```go
Registries: lc.Registries,
```

- [ ] **Step 4: Run existing tests**

Run: `go test ./internal/compose/ -v && go test ./internal/config/ -v`
Expected: all PASS (no breaking changes)

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/compose/parser.go
git commit -m "feat(config): add registries field to config and compose labels"
```

---

### Task 4: Deployer auth-aware pull

**Files:**
- Modify: `internal/deployer/deployer.go:99-118` (update Pull signature and logic)
- Modify: `internal/deployer/deployer_test.go` (add auth pull tests)
- Modify: `internal/reconciler/reconciler.go:19-27` (update AppDeployer interface)

- [ ] **Step 1: Add RegistryAuth type and update Pull signature**

In `internal/deployer/deployer.go`, add after the `ServiceStatus` type:

```go
type RegistryAuth struct {
	URL      string
	Username string
	Password string
}
```

Update `Pull` to:

```go
func (d *Deployer) Pull(ctx context.Context, app *compose.AppConfig, auths []RegistryAuth) error {
	project := "simpledeploy-" + app.Name

	pullArgs := []string{"compose", "-f", app.ComposePath, "-p", project, "pull"}

	if len(auths) > 0 {
		tmpDir, err := writeDockerConfig(auths)
		if err != nil {
			return fmt.Errorf("write docker config: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		pullArgs = []string{"--config", tmpDir, "compose", "-f", app.ComposePath, "-p", project, "pull"}
	}

	_, stderr, err := d.runner.Run(ctx, "docker", pullArgs...)
	if err != nil {
		return fmt.Errorf("compose pull: %s: %w", stderr, err)
	}
	upArgs := []string{
		"compose",
		"-f", app.ComposePath,
		"-p", project,
		"up", "-d",
		"--remove-orphans",
	}
	_, stderr, err = d.runner.Run(ctx, "docker", upArgs...)
	if err != nil {
		return fmt.Errorf("compose up after pull: %s: %w", stderr, err)
	}
	return nil
}
```

Add the helper (add `"encoding/base64"`, `"os"`, `"path/filepath"` to imports):

```go
func writeDockerConfig(auths []RegistryAuth) (string, error) {
	type authEntry struct {
		Auth string `json:"auth"`
	}
	configData := struct {
		Auths map[string]authEntry `json:"auths"`
	}{
		Auths: make(map[string]authEntry, len(auths)),
	}
	for _, a := range auths {
		encoded := base64.StdEncoding.EncodeToString([]byte(a.Username + ":" + a.Password))
		configData.Auths[a.URL] = authEntry{Auth: encoded}
	}
	data, err := json.Marshal(configData)
	if err != nil {
		return "", err
	}
	tmpDir, err := os.MkdirTemp("", "simpledeploy-docker-*")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.json"), data, 0600); err != nil {
		os.RemoveAll(tmpDir)
		return "", err
	}
	return tmpDir, nil
}
```

- [ ] **Step 2: Update AppDeployer interface**

In `internal/reconciler/reconciler.go`, update the `Pull` method in the `AppDeployer` interface:

```go
Pull(ctx context.Context, app *compose.AppConfig, auths []deployer.RegistryAuth) error
```

- [ ] **Step 3: Update PullOne caller**

In `internal/reconciler/reconciler.go`, update `PullOne` to pass `nil` for now (auth resolution added in Task 5):

```go
func (r *Reconciler) PullOne(ctx context.Context, slug string) error {
	cfg, err := r.loadAppConfig(slug)
	if err != nil {
		return err
	}
	if err := r.deployer.Pull(ctx, cfg, nil); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return r.store.UpdateAppStatus(slug, "running")
}
```

- [ ] **Step 4: Write deployer tests**

Add to `internal/deployer/deployer_test.go`:

```go
func TestPullWithAuth(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/tmp/docker-compose.yml"}

	auths := []RegistryAuth{
		{URL: "ghcr.io", Username: "user", Password: "pass"},
	}
	err := d.Pull(context.Background(), app, auths)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// verify --config flag was passed
	found := false
	for _, c := range mock.Calls {
		for _, arg := range c.Args {
			if arg == "--config" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected --config flag in docker pull call")
	}
}

func TestPullWithoutAuth(t *testing.T) {
	mock := &MockRunner{}
	d := &Deployer{runner: mock}
	app := &compose.AppConfig{Name: "myapp", ComposePath: "/tmp/docker-compose.yml"}

	err := d.Pull(context.Background(), app, nil)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	// verify no --config flag
	for _, c := range mock.Calls {
		for _, arg := range c.Args {
			if arg == "--config" {
				t.Error("unexpected --config flag when no auths")
			}
		}
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/deployer/ -run TestPull -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/deployer/deployer.go internal/deployer/deployer_test.go internal/reconciler/reconciler.go
git commit -m "feat(deployer): support registry auth via temp docker config"
```

---

### Task 5: Reconciler registry resolution

**Files:**
- Modify: `internal/reconciler/reconciler.go` (add config, resolveRegistries, update PullOne)

- [ ] **Step 1: Add config and masterSecret to Reconciler**

Update the `Reconciler` struct and `New` function:

```go
type Reconciler struct {
	store        *store.Store
	deployer     AppDeployer
	proxy        proxy.Proxy
	appsDir      string
	config       *config.Config
	masterSecret string
}

func New(st *store.Store, d AppDeployer, p proxy.Proxy, appsDir string, cfg *config.Config) *Reconciler {
	secret := ""
	if cfg != nil {
		secret = cfg.MasterSecret
	}
	return &Reconciler{store: st, deployer: d, proxy: p, appsDir: appsDir, config: cfg, masterSecret: secret}
}
```

Add import for `"github.com/vazra/simpledeploy/internal/auth"` and `"github.com/vazra/simpledeploy/internal/config"`.

- [ ] **Step 2: Add resolveRegistries method**

```go
func (r *Reconciler) resolveRegistries(app *compose.AppConfig) ([]deployer.RegistryAuth, error) {
	if r.masterSecret == "" {
		return nil, nil
	}

	// determine which registry names to use
	var names []string
	switch app.Registries {
	case "none":
		return nil, nil
	case "":
		if r.config != nil {
			names = r.config.Registries
		}
	default:
		for _, n := range strings.Split(app.Registries, ",") {
			n = strings.TrimSpace(n)
			if n != "" {
				names = append(names, n)
			}
		}
	}

	if len(names) == 0 {
		return nil, nil
	}

	var auths []deployer.RegistryAuth
	for _, name := range names {
		reg, err := r.store.GetRegistryByName(name)
		if err != nil {
			return nil, fmt.Errorf("lookup registry %q: %w", name, err)
		}
		username, err := auth.Decrypt(reg.UsernameEnc, r.masterSecret)
		if err != nil {
			return nil, fmt.Errorf("decrypt username for %q: %w", name, err)
		}
		password, err := auth.Decrypt(reg.PasswordEnc, r.masterSecret)
		if err != nil {
			return nil, fmt.Errorf("decrypt password for %q: %w", name, err)
		}
		auths = append(auths, deployer.RegistryAuth{URL: reg.URL, Username: username, Password: password})
	}
	return auths, nil
}
```

- [ ] **Step 3: Update PullOne to use resolveRegistries**

```go
func (r *Reconciler) PullOne(ctx context.Context, slug string) error {
	cfg, err := r.loadAppConfig(slug)
	if err != nil {
		return err
	}
	auths, err := r.resolveRegistries(cfg)
	if err != nil {
		return fmt.Errorf("resolve registries: %w", err)
	}
	if err := r.deployer.Pull(ctx, cfg, auths); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return r.store.UpdateAppStatus(slug, "running")
}
```

- [ ] **Step 4: Update all callers of reconciler.New**

In `cmd/simpledeploy/main.go:301`, update:

```go
rec := reconciler.New(db, dep, caddyProxy, cfg.AppsDir, cfg)
```

Search for any other callers (tests, etc.) and update them to pass `nil` for the config param where appropriate.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/reconciler/ -v && go test ./... 2>&1 | head -50`
Expected: all PASS (compilation check across whole project)

- [ ] **Step 6: Commit**

```bash
git add internal/reconciler/reconciler.go cmd/simpledeploy/main.go
git commit -m "feat(reconciler): resolve registry auth for pulls"
```

---

### Task 6: API endpoints for registry CRUD

**Files:**
- Create: `internal/api/registries.go`
- Modify: `internal/api/server.go:16-27` (add masterSecret field)
- Modify: `internal/api/server.go:67-147` (register routes)

- [ ] **Step 1: Add masterSecret to Server**

In `internal/api/server.go`, add `masterSecret string` field to `Server` struct.

Add setter:

```go
func (s *Server) SetMasterSecret(secret string) { s.masterSecret = secret }
```

- [ ] **Step 2: Implement registry handlers**

```go
// internal/api/registries.go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/vazra/simpledeploy/internal/auth"
)

type registryRequest struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type registryResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Username  string `json:"username"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Server) handleListRegistries(w http.ResponseWriter, r *http.Request) {
	regs, err := s.store.ListRegistries()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := make([]registryResponse, len(regs))
	for i, reg := range regs {
		username := ""
		if s.masterSecret != "" {
			username, _ = auth.Decrypt(reg.UsernameEnc, s.masterSecret)
		}
		resp[i] = registryResponse{
			ID:        reg.ID,
			Name:      reg.Name,
			URL:       reg.URL,
			Username:  username,
			CreatedAt: reg.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: reg.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateRegistry(w http.ResponseWriter, r *http.Request) {
	var req registryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Name == "" || req.URL == "" || req.Username == "" || req.Password == "" {
		http.Error(w, "name, url, username, password required", http.StatusBadRequest)
		return
	}
	if s.masterSecret == "" {
		http.Error(w, "master_secret not configured", http.StatusInternalServerError)
		return
	}
	usernameEnc, err := auth.Encrypt(req.Username, s.masterSecret)
	if err != nil {
		http.Error(w, "encrypt username: "+err.Error(), http.StatusInternalServerError)
		return
	}
	passwordEnc, err := auth.Encrypt(req.Password, s.masterSecret)
	if err != nil {
		http.Error(w, "encrypt password: "+err.Error(), http.StatusInternalServerError)
		return
	}
	reg, err := s.store.CreateRegistry(req.Name, req.URL, usernameEnc, passwordEnc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(registryResponse{
		ID:        reg.ID,
		Name:      reg.Name,
		URL:       reg.URL,
		Username:  req.Username,
		CreatedAt: reg.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: reg.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (s *Server) handleUpdateRegistry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req registryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if s.masterSecret == "" {
		http.Error(w, "master_secret not configured", http.StatusInternalServerError)
		return
	}
	usernameEnc, err := auth.Encrypt(req.Username, s.masterSecret)
	if err != nil {
		http.Error(w, "encrypt: "+err.Error(), http.StatusInternalServerError)
		return
	}
	passwordEnc, err := auth.Encrypt(req.Password, s.masterSecret)
	if err != nil {
		http.Error(w, "encrypt: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.store.UpdateRegistry(id, req.Name, req.URL, usernameEnc, passwordEnc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteRegistry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.store.DeleteRegistry(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 3: Register routes**

In `internal/api/server.go`, in the `routes()` function, add after the backup routes block:

```go
// Registry management
s.mux.Handle("GET /api/registries", s.authMiddleware(http.HandlerFunc(s.handleListRegistries)))
s.mux.Handle("POST /api/registries", s.authMiddleware(http.HandlerFunc(s.handleCreateRegistry)))
s.mux.Handle("PUT /api/registries/{id}", s.authMiddleware(http.HandlerFunc(s.handleUpdateRegistry)))
s.mux.Handle("DELETE /api/registries/{id}", s.authMiddleware(http.HandlerFunc(s.handleDeleteRegistry)))
```

- [ ] **Step 4: Set masterSecret in cmd/simpledeploy/main.go**

Find where `srv` (the API server) is created in `runServe`, and add after any existing `Set*` calls:

```go
srv.SetMasterSecret(cfg.MasterSecret)
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/api/ -v && go build ./...`
Expected: PASS, compiles

- [ ] **Step 6: Commit**

```bash
git add internal/api/registries.go internal/api/server.go cmd/simpledeploy/main.go
git commit -m "feat(api): add registry CRUD endpoints"
```

---

### Task 7: CLI registry commands

**Files:**
- Modify: `cmd/simpledeploy/main.go` (add registry commands)

- [ ] **Step 1: Add command vars**

Add after the existing command vars (near line 163):

```go
var registryCmd = &cobra.Command{Use: "registry", Short: "Manage container registries"}
var registryAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a registry",
	RunE:  runRegistryAdd,
}
var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registries",
	RunE:  runRegistryList,
}
var registryRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a registry",
	Args:  cobra.ExactArgs(1),
	RunE:  runRegistryRemove,
}
```

- [ ] **Step 2: Register flags and subcommands in init()**

Add in the `init()` function:

```go
registryAddCmd.Flags().String("name", "", "registry name")
registryAddCmd.Flags().String("url", "", "registry URL (e.g. ghcr.io)")
registryAddCmd.Flags().String("username", "", "username")
registryAddCmd.Flags().String("password", "", "password")
registryAddCmd.MarkFlagRequired("name")
registryAddCmd.MarkFlagRequired("url")
registryAddCmd.MarkFlagRequired("username")
registryAddCmd.MarkFlagRequired("password")

registryCmd.AddCommand(registryAddCmd, registryListCmd, registryRemoveCmd)
```

Add `registryCmd` to `rootCmd.AddCommand(...)`.

- [ ] **Step 3: Implement RunE functions**

```go
func runRegistryAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.MasterSecret == "" {
		return fmt.Errorf("master_secret must be set in config")
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	name, _ := cmd.Flags().GetString("name")
	url, _ := cmd.Flags().GetString("url")
	username, _ := cmd.Flags().GetString("username")
	password, _ := cmd.Flags().GetString("password")

	usernameEnc, err := auth.Encrypt(username, cfg.MasterSecret)
	if err != nil {
		return fmt.Errorf("encrypt username: %w", err)
	}
	passwordEnc, err := auth.Encrypt(password, cfg.MasterSecret)
	if err != nil {
		return fmt.Errorf("encrypt password: %w", err)
	}

	reg, err := db.CreateRegistry(name, url, usernameEnc, passwordEnc)
	if err != nil {
		return err
	}
	fmt.Printf("added registry %q (%s) id=%s\n", reg.Name, reg.URL, reg.ID)
	return nil
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	regs, err := db.ListRegistries()
	if err != nil {
		return err
	}
	if len(regs) == 0 {
		fmt.Println("no registries configured")
		return nil
	}
	for _, r := range regs {
		username := "(encrypted)"
		if cfg.MasterSecret != "" {
			if u, err := auth.Decrypt(r.UsernameEnc, cfg.MasterSecret); err == nil {
				username = u
			}
		}
		fmt.Printf("%-20s %-40s user=%-15s id=%s\n", r.Name, r.URL, username, r.ID)
	}
	return nil
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	name := args[0]
	reg, err := db.GetRegistryByName(name)
	if err != nil {
		return fmt.Errorf("registry %q not found: %w", name, err)
	}
	if err := db.DeleteRegistry(reg.ID); err != nil {
		return err
	}
	fmt.Printf("removed registry %q\n", name)
	return nil
}
```

- [ ] **Step 4: Build and verify**

Run: `go build ./cmd/simpledeploy/ && ./simpledeploy registry --help`
Expected: shows add/list/remove subcommands

- [ ] **Step 5: Commit**

```bash
git add cmd/simpledeploy/main.go
git commit -m "feat(cli): add registry add/list/remove commands"
```

---

### Task 8: UI registry management

**Files:**
- Check existing UI structure for settings page pattern, then create registry management component

This task depends on the existing UI structure. The implementer should:

- [ ] **Step 1: Explore UI structure**

Run: `ls ui/src/routes/` and `ls ui/src/lib/` to find existing page/component patterns.

- [ ] **Step 2: Add API client functions**

Add to the existing API client (likely `ui/src/lib/api.ts` or similar):

```typescript
export async function listRegistries(): Promise<Registry[]> {
  const res = await fetch('/api/registries', { headers: authHeaders() });
  return res.json();
}

export async function createRegistry(data: { name: string; url: string; username: string; password: string }) {
  const res = await fetch('/api/registries', {
    method: 'POST',
    headers: { ...authHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function deleteRegistry(id: string) {
  const res = await fetch(`/api/registries/${id}`, {
    method: 'DELETE',
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error(await res.text());
}
```

Types:

```typescript
export interface Registry {
  id: string;
  name: string;
  url: string;
  username: string;
  created_at: string;
  updated_at: string;
}
```

- [ ] **Step 3: Create registry settings component**

Create a Svelte component (path depends on existing patterns found in step 1) with:
- Table listing registries (name, url, username, created_at)
- "Add Registry" form with name, url, username, password fields
- Delete button per row with confirmation
- Follow existing UI patterns for styling and layout

- [ ] **Step 4: Add to settings page**

Add the registry component to the existing settings/admin page, following the pattern used by other settings sections (webhooks, alert rules, etc.).

- [ ] **Step 5: Test manually**

Run: `cd ui && npm run dev` (or however the dev server starts)
Verify: can add, list, and remove registries through the UI

- [ ] **Step 6: Commit**

```bash
git add ui/
git commit -m "feat(ui): add registry management in settings"
```
