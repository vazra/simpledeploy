package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/store"
)

// cliTestEnv points the package-level cfgFile at a temp config and seeds the
// state a running server leaves behind: an admin user in the DB, mirrored to
// config.yml + secrets.yml.
type cliTestEnv struct {
	dataDir, appsDir string
	adminID          int64
}

func newCLITestEnv(t *testing.T) *cliTestEnv {
	t.Helper()
	root := t.TempDir()
	env := &cliTestEnv{
		dataDir: filepath.Join(root, "data"),
		appsDir: filepath.Join(root, "apps"),
	}
	for _, d := range []string{env.dataDir, env.appsDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfgPath := filepath.Join(root, "config.yaml")
	body := "data_dir: " + env.dataDir + "\napps_dir: " + env.appsDir + "\nmaster_secret: test-secret-0123456789\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = prev })

	db := env.openDB(t)
	admin, err := db.CreateUser("admin", "admin-hash", "super_admin", "", "")
	if err != nil {
		t.Fatal(err)
	}
	env.adminID = admin.ID
	if err := configsync.New(db, env.appsDir, env.dataDir).WriteGlobal(); err != nil {
		t.Fatal(err)
	}
	db.Close()
	return env
}

func (e *cliTestEnv) openDB(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(e.dataDir, "simpledeploy.db"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// reconcile simulates the server's boot-time FS -> DB reconcile.
func (e *cliTestEnv) reconcile(t *testing.T) {
	t.Helper()
	db := e.openDB(t)
	defer db.Close()
	if err := configsync.New(db, e.appsDir, e.dataDir).ReconcileDBFromFS(context.Background()); err != nil {
		t.Fatalf("ReconcileDBFromFS: %v", err)
	}
}

func (e *cliTestEnv) loadGlobal(t *testing.T) *configsync.LoadedGlobal {
	t.Helper()
	db := e.openDB(t)
	defer db.Close()
	g, err := configsync.New(db, e.appsDir, e.dataDir).LoadGlobalFromFS()
	if err != nil {
		t.Fatal(err)
	}
	if g.Sidecar == nil || g.Secrets == nil {
		t.Fatalf("config.yml/secrets.yml missing: %+v", g)
	}
	return g
}

// runCmd sets flags (callers pass every flag the command reads, since cobra
// commands are package globals) and invokes the RunE function.
func runCmd(t *testing.T, cmd *cobra.Command, run func(*cobra.Command, []string) error, flags map[string]string) error {
	t.Helper()
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set flag %s: %v", k, err)
		}
	}
	return run(cmd, nil)
}

func hasKey(g *configsync.LoadedGlobal, user, name string) (inConfig, inSecrets bool) {
	for _, k := range g.Sidecar.APIKeys {
		if k.Username == user && k.Name == name {
			inConfig = true
		}
	}
	for _, k := range g.Secrets.APIKeys {
		if k.Username == user && k.Name == name && k.KeyHash != "" {
			inSecrets = true
		}
	}
	return
}

func hasUser(g *configsync.LoadedGlobal, user string) (inConfig, inSecrets bool) {
	for _, u := range g.Sidecar.Users {
		if u.Username == user {
			inConfig = true
		}
	}
	for _, u := range g.Secrets.Users {
		if u.Username == user && u.PasswordHash != "" {
			inSecrets = true
		}
	}
	return
}

func TestCLIAPIKeyCreateRevokePersistsToSidecars(t *testing.T) {
	env := newCLITestEnv(t)

	if err := runCmd(t, apikeyCreateCmd, runAPIKeyCreate, map[string]string{
		"name": "ci", "user-id": strconv.FormatInt(env.adminID, 10),
	}); err != nil {
		t.Fatalf("apikey create: %v", err)
	}

	c, s := hasKey(env.loadGlobal(t), "admin", "ci")
	if !c || !s {
		t.Fatalf("key not in sidecars after create: config=%v secrets=%v", c, s)
	}

	// Server restart must not prune the CLI-created key.
	env.reconcile(t)
	db := env.openDB(t)
	keys, err := db.ListAPIKeysByUser(env.adminID)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].Name != "ci" {
		t.Fatalf("key lost after ReconcileDBFromFS: %+v", keys)
	}

	if err := runCmd(t, apikeyRevokeCmd, runAPIKeyRevoke, map[string]string{
		"id": strconv.FormatInt(keys[0].ID, 10),
	}); err != nil {
		t.Fatalf("apikey revoke: %v", err)
	}
	c, s = hasKey(env.loadGlobal(t), "admin", "ci")
	if c || s {
		t.Fatalf("key still in sidecars after revoke: config=%v secrets=%v", c, s)
	}

	// Revoked key must stay revoked across restart.
	env.reconcile(t)
	db = env.openDB(t)
	keys, _ = db.ListAPIKeysByUser(env.adminID)
	db.Close()
	if len(keys) != 0 {
		t.Fatalf("revoked key reappeared: %+v", keys)
	}
}

func TestCLIUsersCreateDeletePersistsToSidecars(t *testing.T) {
	env := newCLITestEnv(t)

	if err := runCmd(t, usersCreateCmd, runUsersCreate, map[string]string{
		"username": "bob", "password": "hunter2-long-pass", "role": "viewer",
	}); err != nil {
		t.Fatalf("users create: %v", err)
	}
	c, s := hasUser(env.loadGlobal(t), "bob")
	if !c || !s {
		t.Fatalf("user not in sidecars after create: config=%v secrets=%v", c, s)
	}

	env.reconcile(t)
	db := env.openDB(t)
	bob, err := db.GetUserByUsername("bob")
	db.Close()
	if err != nil || bob == nil {
		t.Fatalf("user lost after ReconcileDBFromFS: %v", err)
	}

	if err := runCmd(t, usersDeleteCmd, runUsersDelete, map[string]string{
		"id": strconv.FormatInt(bob.ID, 10),
	}); err != nil {
		t.Fatalf("users delete: %v", err)
	}
	c, s = hasUser(env.loadGlobal(t), "bob")
	if c || s {
		t.Fatalf("user still in sidecars after delete: config=%v secrets=%v", c, s)
	}
	if c, _ := hasUser(env.loadGlobal(t), "admin"); !c {
		t.Fatal("admin missing from sidecars")
	}

	env.reconcile(t)
	db = env.openDB(t)
	_, err = db.GetUserByUsername("bob")
	db.Close()
	if err == nil {
		t.Fatal("deleted user reappeared after ReconcileDBFromFS")
	}
}

func TestCLIRegistryAddRemovePersistsToSidecars(t *testing.T) {
	env := newCLITestEnv(t)

	if err := runCmd(t, registryAddCmd, runRegistryAdd, map[string]string{
		"name": "ghcr", "url": "ghcr.io", "username": "u", "password": "p",
	}); err != nil {
		t.Fatalf("registry add: %v", err)
	}
	g := env.loadGlobal(t)
	if len(g.Sidecar.Registries) != 1 || g.Sidecar.Registries[0].Name != "ghcr" {
		t.Fatalf("registry not in config.yml: %+v", g.Sidecar.Registries)
	}

	env.reconcile(t)
	db := env.openDB(t)
	regs, _ := db.ListRegistries()
	db.Close()
	if len(regs) != 1 {
		t.Fatalf("registry lost after ReconcileDBFromFS: %+v", regs)
	}

	if err := registryRemoveCmd.RunE(registryRemoveCmd, []string{"ghcr"}); err != nil {
		t.Fatalf("registry remove: %v", err)
	}
	if g := env.loadGlobal(t); len(g.Sidecar.Registries) != 0 {
		t.Fatalf("registry still in config.yml: %+v", g.Sidecar.Registries)
	}

	env.reconcile(t)
	db = env.openDB(t)
	regs, _ = db.ListRegistries()
	db.Close()
	if len(regs) != 0 {
		t.Fatalf("removed registry reappeared after ReconcileDBFromFS: %+v", regs)
	}
}

func TestCLIWritesSidecarModesAndGitignore(t *testing.T) {
	env := newCLITestEnv(t)
	// Start from no files so modes come from the CLI write itself.
	for _, f := range []string{"config.yml", "secrets.yml"} {
		_ = os.Remove(filepath.Join(env.dataDir, f))
	}

	if err := runCmd(t, usersCreateCmd, runUsersCreate, map[string]string{
		"username": "dave", "password": "hunter2-long-pass", "role": "viewer",
	}); err != nil {
		t.Fatalf("users create: %v", err)
	}
	for name, want := range map[string]os.FileMode{"config.yml": 0o644, "secrets.yml": 0o600} {
		info, err := os.Stat(filepath.Join(env.dataDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", name, got, want)
		}
	}
	gi, err := os.ReadFile(filepath.Join(env.dataDir, ".gitignore"))
	if err != nil || !strings.Contains(string(gi), "secrets.yml") {
		t.Errorf("data_dir .gitignore missing secrets.yml: %q, %v", gi, err)
	}
}

func TestCLIRedactedWriteFailureIsNonFatal(t *testing.T) {
	env := newCLITestEnv(t)
	// Block _global.yml with a non-empty directory so its rename fails.
	if err := os.MkdirAll(filepath.Join(env.appsDir, "_global.yml", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runCmd(t, usersCreateCmd, runUsersCreate, map[string]string{
		"username": "erin", "password": "hunter2-long-pass", "role": "viewer",
	}); err != nil {
		t.Fatalf("users create should succeed when only redacted write fails: %v", err)
	}
	if c, s := hasUser(env.loadGlobal(t), "erin"); !c || !s {
		t.Fatalf("user not persisted: config=%v secrets=%v", c, s)
	}
}

func TestCLIAPIKeyCreateRejectsDuplicateName(t *testing.T) {
	env := newCLITestEnv(t)
	flags := map[string]string{"name": "ci", "user-id": strconv.FormatInt(env.adminID, 10)}
	if err := runCmd(t, apikeyCreateCmd, runAPIKeyCreate, flags); err != nil {
		t.Fatalf("first create: %v", err)
	}
	err := runCmd(t, apikeyCreateCmd, runAPIKeyCreate, flags)
	if err == nil || !strings.Contains(err.Error(), `already has an API key named "ci"`) {
		t.Fatalf("duplicate create err = %v", err)
	}
	db := env.openDB(t)
	keys, _ := db.ListAPIKeysByUser(env.adminID)
	db.Close()
	if len(keys) != 1 {
		t.Fatalf("got %d keys, want 1", len(keys))
	}
}

func TestCLIAPIKeyCreatePersistFailureSaysKeyIsValid(t *testing.T) {
	env := newCLITestEnv(t)
	blockSecrets(t, env)
	err := runCmd(t, apikeyCreateCmd, runAPIKeyCreate, map[string]string{
		"name": "ci", "user-id": strconv.FormatInt(env.adminID, 10),
	})
	if err == nil {
		t.Fatal("expected error when sidecar write fails")
	}
	for _, want := range []string{"IS valid now", "simpledeploy config export"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
}

// blockSecrets makes secrets.yml unwritable by replacing it with a non-empty
// directory, so WriteGlobal fails.
func blockSecrets(t *testing.T, env *cliTestEnv) {
	t.Helper()
	secrets := filepath.Join(env.dataDir, "secrets.yml")
	if err := os.Remove(secrets); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(secrets, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestCLIPersistFailureReturnsError(t *testing.T) {
	env := newCLITestEnv(t)

	blockSecrets(t, env)

	err := runCmd(t, usersCreateCmd, runUsersCreate, map[string]string{
		"username": "carol", "password": "hunter2-long-pass", "role": "viewer",
	})
	if err == nil {
		t.Fatal("expected error when sidecar write fails")
	}
	if !strings.Contains(err.Error(), "reverted on next server restart") || !strings.Contains(err.Error(), "global sidecars") {
		t.Fatalf("error does not explain consequence: %v", err)
	}
}
