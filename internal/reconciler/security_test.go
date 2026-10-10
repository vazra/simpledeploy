package reconciler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/store"
)

const safeCompose = "services:\n  web:\n    image: nginx:1\n"

// seedApp writes a compose file for slug, registers the app, and stores
// content as a compose version. Returns the app ID and version ID.
func seedApp(t *testing.T, st *store.Store, appsDir, slug, content string) (int64, int64) {
	t.Helper()
	dir := filepath.Join(appsDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(safeCompose), 0o600); err != nil {
		t.Fatal(err)
	}
	app := &store.App{Name: slug, Slug: slug, ComposePath: path, Status: "running"}
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetAppBySlug(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateComposeVersion(got.ID, content, "h-"+slug); err != nil {
		t.Fatal(err)
	}
	versions, err := st.ListComposeVersions(got.ID)
	if err != nil || len(versions) == 0 {
		t.Fatalf("list versions: %v", err)
	}
	return got.ID, versions[0].ID
}

func readCompose(t *testing.T, appsDir, slug string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(appsDir, slug, "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRollbackRejectsOtherAppsVersion(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	seedApp(t, st, appsDir, "mine", safeCompose)
	_, otherVer := seedApp(t, st, appsDir, "theirs", "services:\n  secret:\n    image: theirs:1\n")

	err := r.RollbackOne(context.Background(), "mine", otherVer)
	if !errors.Is(err, ErrVersionNotFound) {
		t.Fatalf("err = %v, want ErrVersionNotFound", err)
	}
	if got := readCompose(t, appsDir, "mine"); got != safeCompose {
		t.Errorf("compose was overwritten: %q", got)
	}
	if mock.hasCall("RollbackDeploy:mine") {
		t.Error("RollbackDeploy ran for another app's version")
	}
}

func TestRollbackRefusesUnsafeVersionWithoutTouchingFile(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	_, ver := seedApp(t, st, appsDir, "app", "services:\n  web:\n    image: nginx\n    privileged: true\n")

	err := r.RollbackOne(context.Background(), "app", ver)
	var ve *compose.ViolationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want *compose.ViolationError", err)
	}
	if got := readCompose(t, appsDir, "app"); got != safeCompose {
		t.Errorf("compose was overwritten: %q", got)
	}
	if mock.hasCall("RollbackDeploy:app") {
		t.Error("RollbackDeploy ran for an unsafe version")
	}
}

func TestRollbackOwnVersion(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	old := "services:\n  web:\n    image: nginx:0.9\n"
	_, ver := seedApp(t, st, appsDir, "app", old)

	if err := r.RollbackOne(context.Background(), "app", ver); err != nil {
		t.Fatalf("RollbackOne: %v", err)
	}
	if got := readCompose(t, appsDir, "app"); got != old {
		t.Errorf("compose = %q, want %q", got, old)
	}
	if !mock.hasCall("RollbackDeploy:app") {
		t.Error("expected RollbackDeploy:app")
	}
}

func TestEnsureSharedNetworkDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.yml")
	content := "services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: a.example.com\n"
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "docker-compose.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if ensureSharedNetwork(link) {
		t.Fatal("ensureSharedNetwork rewrote a symlinked compose file")
	}
	got, _ := os.ReadFile(target)
	if string(got) != content {
		t.Errorf("symlink target modified: %q", got)
	}
}

func TestScanSkipsAppBindingDataDir(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	appsDir := t.TempDir()
	dataDir := t.TempDir()
	r := New(st, &mockDeployer{}, proxy.NewMockProxy(), appsDir, &config.Config{DataDir: dataDir}, nil)
	t.Cleanup(func() { compose.SetProtectedPaths("", "") })

	dir := filepath.Join(appsDir, "thief")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "services:\n  web:\n    image: nginx\n    volumes:\n      - " + dataDir + ":/data\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	scan, err := r.scanAppsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scan.desired["thief"]; ok {
		t.Fatal("app binding data_dir was not skipped")
	}
}

func TestScanSkipsAppUnsafeViaDotEnv(t *testing.T) {
	r, _, _, appsDir := newTestEnv(t)
	t.Cleanup(func() { compose.SetProtectedPaths("", "") })
	dir := filepath.Join(appsDir, "envpriv")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n    privileged: ${PRIV:-false}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PRIV=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scan, err := r.scanAppsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scan.desired["envpriv"]; ok {
		t.Fatal("app made privileged through .env was not skipped")
	}
}

func TestScanKeepsAppWithBrokenDotEnv(t *testing.T) {
	r, _, _, appsDir := newTestEnv(t)
	t.Cleanup(func() { compose.SetProtectedPaths("", "") })
	writeComposeFile(t, appsDir, "badenv")
	if err := os.WriteFile(filepath.Join(appsDir, "badenv", ".env"), []byte("PASS=\"unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scan, err := r.scanAppsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scan.desired["badenv"]; !ok {
		t.Fatal("running app with a broken .env was dropped from routing")
	}
}

// An app deployed before the validator tightened keeps its routes while its
// compose file is unchanged, and loses them once the file changes.
func TestRefusedAppKeepsRoutesWhileUnchanged(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	legacy := "services:\n  web:\n    image: nginx\n    privileged: true\n    labels:\n      simpledeploy.endpoints.0.domain: legacy.example.com\n      simpledeploy.endpoints.0.port: \"80\"\n"
	dir := filepath.Join(appsDir, "legacy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	// Apps deployed by earlier versions already carry the shared network.
	ensureSharedNetwork(path)
	hash, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertApp(&store.App{Name: "legacy", Slug: "legacy", ComposePath: path, Status: "running", ComposeHash: hash}, nil); err != nil {
		t.Fatal(err)
	}

	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mock.hasCall("Deploy:legacy") {
		t.Fatal("refused app must not be redeployed")
	}
	routes := r.proxy.(*proxy.MockProxy).Routes()
	if len(routes) == 0 || routes[0].Domain != "legacy.example.com" {
		t.Fatalf("routes = %+v, want legacy.example.com kept", routes)
	}

	// Changing the file drops the route-only exemption.
	edited, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(edited, []byte("# edited\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if routes := r.proxy.(*proxy.MockProxy).Routes(); len(routes) != 0 {
		t.Fatalf("routes = %+v, want none after the file changed", routes)
	}
}

// A file refused over a file reference (label_file outside the app folder)
// keeps the running app's routes while unchanged and is never deployed.
func TestFileRefRefusedAppKeepsRoutesWhileUnchanged(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	outside := filepath.Join(t.TempDir(), "labels")
	if err := os.WriteFile(outside, []byte("team=core\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy := "services:\n  web:\n    image: nginx\n    label_file: " + outside + "\n    labels:\n      simpledeploy.endpoints.0.domain: refs.example.com\n      simpledeploy.endpoints.0.port: \"80\"\n"
	dir := filepath.Join(appsDir, "refs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	ensureSharedNetwork(path)
	hash, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertApp(&store.App{Name: "refs", Slug: "refs", ComposePath: path, Status: "running", ComposeHash: hash}, nil); err != nil {
		t.Fatal(err)
	}

	scan, err := r.scanAppsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scan.refused["refs"]; !ok {
		t.Fatal("app with an unsafe label_file not in the refused set")
	}
	if _, ok := scan.desired["refs"]; ok {
		t.Fatal("app with an unsafe label_file is deployable")
	}

	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mock.hasCall("Deploy:refs") || mock.hasCall("Teardown:refs") {
		t.Fatalf("refused app deployed or archived: %v", mock.calls)
	}
	routes := r.proxy.(*proxy.MockProxy).Routes()
	if len(routes) == 0 || routes[0].Domain != "refs.example.com" {
		t.Fatalf("routes = %+v, want refs.example.com kept", routes)
	}

	edited, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(edited, []byte("# edited\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if routes := r.proxy.(*proxy.MockProxy).Routes(); len(routes) != 0 {
		t.Fatalf("routes = %+v, want none after the file changed", routes)
	}
}

// An app running from a symlinked compose file (allowed by older versions)
// keeps its routes while unchanged but is never redeployed.
func TestSymlinkedComposeKeepsRoutes(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	content := "services:\n  web:\n    image: nginx\n    labels:\n      simpledeploy.endpoints.0.domain: linked.example.com\n      simpledeploy.endpoints.0.port: \"80\"\n    networks: [default, simpledeploy-public]\nnetworks:\n  simpledeploy-public:\n    external: true\n"
	target := filepath.Join(t.TempDir(), "compose.yml")
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(appsDir, "linked")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	hash, err := hashLinkedFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertApp(&store.App{Name: "linked", Slug: "linked", ComposePath: path, Status: "running", ComposeHash: hash}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mock.hasCall("Deploy:linked") {
		t.Fatal("symlinked compose must not be deployed")
	}
	routes := r.proxy.(*proxy.MockProxy).Routes()
	if len(routes) == 0 || routes[0].Domain != "linked.example.com" {
		t.Fatalf("routes = %+v, want linked.example.com kept", routes)
	}
}

// checkingDeployer refuses files like the real deployer's pre-up check.
type checkingDeployer struct{ *mockDeployer }

func (d checkingDeployer) Deploy(ctx context.Context, app *compose.AppConfig, auths ...deployer.RegistryAuth) deployer.DeployResult {
	cfg, err := compose.ParseFile(app.ComposePath, app.Name)
	if err == nil {
		if v := compose.ValidateComposeSecurity(cfg); len(v) > 0 {
			err = &compose.ViolationError{Violations: v}
		}
	}
	if err != nil {
		return deployer.DeployResult{Err: fmt.Errorf("compose file could not be read: %w", err), Status: "failed"}
	}
	return d.mockDeployer.Deploy(ctx, app, auths...)
}

// A file the deployer refused must not look deployed, or the route-only
// exemption for unchanged files would route it.
func TestDeployOneRefusedFileIsNotRouted(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	r.deployer = checkingDeployer{mock}
	bad := "services:\n  web:\n    image: nginx\n    privileged: true\n    labels:\n      simpledeploy.endpoints.0.domain: bad.example.com\n      simpledeploy.endpoints.0.port: \"80\"\n"
	dir := filepath.Join(appsDir, "bad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "docker-compose.yml")
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	ensureSharedNetwork(path) // so the scan leaves the file as is

	var ve *compose.ViolationError
	if err := r.DeployOne(context.Background(), path, "bad"); !errors.As(err, &ve) {
		t.Fatalf("DeployOne err = %v, want *compose.ViolationError", err)
	}
	app, err := st.GetAppBySlug("bad")
	if err != nil {
		t.Fatal(err)
	}
	if app.ComposeHash != "" {
		t.Errorf("refused file recorded hash %q", app.ComposeHash)
	}
	if vs, _ := st.ListComposeVersions(app.ID); len(vs) != 0 {
		t.Errorf("refused file stored as %d compose version(s)", len(vs))
	}

	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if routes := r.proxy.(*proxy.MockProxy).Routes(); len(routes) != 0 {
		t.Fatalf("routes = %+v, want none for a refused file", routes)
	}
}

// A refused redeploy keeps the hash of the file that last ran.
func TestDeployOneRefusedKeepsStoredHash(t *testing.T) {
	r, mock, st, appsDir := newTestEnv(t)
	r.deployer = checkingDeployer{mock}
	seedApp(t, st, appsDir, "app", safeCompose)
	app, _ := st.GetAppBySlug("app")
	app.ComposeHash = "h-old"
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(appsDir, "app", "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services:\n  web:\n    image: nginx\n    privileged: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.DeployOne(context.Background(), path, "app"); err == nil {
		t.Fatal("refused deploy returned no error")
	}
	got, _ := st.GetAppBySlug("app")
	if got.ComposeHash != "h-old" {
		t.Errorf("hash = %q, want h-old kept", got.ComposeHash)
	}
	if vs, _ := st.ListComposeVersions(got.ID); len(vs) != 1 {
		t.Errorf("compose versions = %d, want only the seeded one", len(vs))
	}
}

// dotEnvRefuser refuses rollbacks the way the deployer does for a bad .env.
type dotEnvRefuser struct{ *mockDeployer }

func (d dotEnvRefuser) RollbackDeploy(context.Context, *compose.AppConfig, int, *int64, ...deployer.RegistryAuth) deployer.DeployResult {
	return deployer.DeployResult{Err: fmt.Errorf("compose file could not be read: %w", compose.ErrDotEnv), Status: "failed"}
}

func TestRollbackRefusedByDeployerKeepsStoredHash(t *testing.T) {
	t.Setenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK", "true")
	r, mock, st, appsDir := newTestEnv(t)
	r.deployer = dotEnvRefuser{mock}
	_, ver := seedApp(t, st, appsDir, "app", "services:\n  web:\n    image: nginx:0.9\n")
	app, _ := st.GetAppBySlug("app")
	app.ComposeHash = "h-old"
	if err := st.UpsertApp(app, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.RollbackOne(context.Background(), "app", ver); !errors.Is(err, compose.ErrDotEnv) {
		t.Fatalf("err = %v, want ErrDotEnv", err)
	}
	if got, _ := st.GetAppBySlug("app"); got.ComposeHash != "h-old" {
		t.Errorf("hash = %q, want h-old kept", got.ComposeHash)
	}
}

// A symlinked .env takes the .env path (route from compose alone), not the
// symlinked-compose path.
func TestScanSymlinkedDotEnvKeepsAppRouted(t *testing.T) {
	r, _, _, appsDir := newTestEnv(t)
	writeComposeFile(t, appsDir, "envlink")
	target := filepath.Join(t.TempDir(), "outside.env")
	if err := os.WriteFile(target, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(appsDir, "envlink", ".env")); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	scan, err := r.scanAppsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scan.desired["envlink"]; !ok {
		t.Fatal("app with a symlinked .env was dropped from routing")
	}
	if _, ok := scan.refused["envlink"]; ok {
		t.Error("app with a symlinked .env treated as refused")
	}
	if strings.Contains(buf.String(), "docker-compose.yml is a symlink") {
		t.Errorf("symlinked .env reported as symlinked compose file:\n%s", buf.String())
	}
}

// Refused apps are left alone without the "not loaded" archive message on
// every reconcile.
func TestReconcileRefusedAppNotArchivedQuietly(t *testing.T) {
	r, mock, st, appsDir := newTestEnv(t)
	seedApp(t, st, appsDir, "app", safeCompose)
	path := filepath.Join(appsDir, "app", "docker-compose.yml")
	if err := os.WriteFile(path, []byte("services:\n  web:\n    image: nginx\n    privileged: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mock.hasCall("Teardown:app") {
		t.Error("refused app was archived")
	}
	if strings.Contains(buf.String(), "skipping archive of app") {
		t.Errorf("archive skip logged for a refused app:\n%s", buf.String())
	}
}

func TestUpdateProxyRoutesSetsAppIDs(t *testing.T) {
	r, _, st, appsDir := newTestEnv(t)
	writeComposeFile(t, appsDir, "known")
	writeComposeFile(t, appsDir, "fresh")
	if err := st.UpsertApp(&store.App{Name: "known", Slug: "known", ComposePath: filepath.Join(appsDir, "known", "docker-compose.yml"), Status: "running"}, nil); err != nil {
		t.Fatal(err)
	}
	known, _ := st.GetAppBySlug("known")
	scan, err := r.scanAppsDir()
	if err != nil {
		t.Fatal(err)
	}
	r.updateProxyRoutes(scan.desired, scan.refused)
	ids := map[string]int64{}
	for _, rt := range r.proxy.(*proxy.MockProxy).Routes() {
		ids[rt.AppSlug] = rt.AppID
	}
	if ids["known"] != known.ID {
		t.Errorf("known AppID = %d, want %d", ids["known"], known.ID)
	}
	if ids["fresh"] != math.MaxInt64 {
		t.Errorf("fresh AppID = %d, want MaxInt64 (not in store yet)", ids["fresh"])
	}
}

// New without a config clears allowed host paths left by an earlier call.
func TestNewWithoutConfigClearsAllowedHostPaths(t *testing.T) {
	compose.SetAllowedHostPaths([]string{"/home/media"})
	t.Cleanup(func() { compose.SetAllowedHostPaths(nil) })
	r, _, _, appsDir := newTestEnv(t)
	dir := filepath.Join(appsDir, "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services:\n  web:\n    image: nginx\n    volumes:\n      - /home/media:/media\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scan, err := r.scanAppsDir()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scan.desired["media"]; ok {
		t.Fatal("stale allowed host path leaked into a reconciler built without config")
	}
}
