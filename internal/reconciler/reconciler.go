package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/events"
	"github.com/vazra/simpledeploy/internal/fsutil"
	"github.com/vazra/simpledeploy/internal/mirror"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/store"
)

// sharedNetworkName is the docker bridge network that endpoint services are
// attached to so the host-native Caddy can reach them by container IP.
const sharedNetworkName = "simpledeploy-public"

// AppDeployer is the interface the reconciler uses to deploy and remove apps.
type AppDeployer interface {
	Deploy(ctx context.Context, app *compose.AppConfig, auths ...deployer.RegistryAuth) deployer.DeployResult
	RollbackDeploy(ctx context.Context, app *compose.AppConfig, version int, composeVersionID *int64, auths ...deployer.RegistryAuth) deployer.DeployResult
	Teardown(ctx context.Context, projectName string) error
	Restart(ctx context.Context, app *compose.AppConfig) deployer.DeployResult
	Stop(ctx context.Context, projectName string) error
	Start(ctx context.Context, projectName string) error
	Pull(ctx context.Context, app *compose.AppConfig, auths []deployer.RegistryAuth) deployer.DeployResult
	Scale(ctx context.Context, app *compose.AppConfig, scales map[string]int) error
	Status(ctx context.Context, projectName string) ([]deployer.ServiceStatus, error)
	Cancel(ctx context.Context, app *compose.AppConfig) error
}

// Reconciler syncs the apps directory with the running containers and store.
type Reconciler struct {
	store        *store.Store
	deployer     AppDeployer
	proxy        proxy.Proxy // can be nil
	appsDir      string
	config       *config.Config
	masterSecret string
	resolver     proxy.UpstreamResolver // nil-safe
	syncer       *configsync.Syncer     // nil means configsync disabled
	audit        *audit.Recorder        // nil-safe
	bus          *events.Bus            // nil-safe
}

// scanResult is one pass over the apps directory.
type scanResult struct {
	desired map[string]*compose.AppConfig
	// refused holds apps rejected on compose security rules or for a
	// symlinked compose file; file-reference refusals hold a route-only
	// parse (compose.ParseForRoutes). An app deployed before the rules
	// tightened keeps its routes while its compose file is unchanged (see
	// withRouteOnlyApps); it cannot be redeployed until the file is fixed.
	refused map[string]*compose.AppConfig
}

// SetEventBus wires the realtime events bus for status flips. Nil-safe.
func (r *Reconciler) SetEventBus(b *events.Bus) { r.bus = b }

// New creates a Reconciler. syncer may be nil (disables configsync recovery).
// It also registers data_dir and apps_dir with the compose validator so no
// app can bind-mount SimpleDeploy's own data or another app's folder.
func New(st *store.Store, d AppDeployer, p proxy.Proxy, appsDir string, cfg *config.Config, syncer *configsync.Syncer) *Reconciler {
	secret := ""
	dataDir := ""
	if cfg != nil {
		secret = cfg.MasterSecret
		dataDir = cfg.DataDir
	}
	compose.SetProtectedPaths(dataDir, appsDir)
	var allowed []string
	if cfg != nil {
		allowed = cfg.AllowedBindPaths
	}
	compose.SetAllowedHostPaths(allowed)
	return &Reconciler{store: st, deployer: d, proxy: p, appsDir: appsDir, config: cfg, masterSecret: secret, syncer: syncer}
}

// SetAuditRecorder wires an audit recorder for archive events. Nil-safe.
func (r *Reconciler) SetAuditRecorder(rec *audit.Recorder) {
	r.audit = rec
}

// SetDockerClient wires a Docker client for container-IP upstream resolution.
// Nil is safe; routes fall back to DNS names in that case.
func (r *Reconciler) SetDockerClient(c docker.Client) {
	if c == nil {
		r.resolver = nil
		return
	}
	r.resolver = &proxy.DockerResolver{Client: c}
}

// SubscribeDeployLog returns a channel of deploy output lines for the given app slug.
func (r *Reconciler) SubscribeDeployLog(slug string) (<-chan deployer.OutputLine, func(), bool) {
	if d, ok := r.deployer.(*deployer.Deployer); ok {
		return d.Tracker.Subscribe(slug)
	}
	return nil, nil, false
}

// Reconcile diffs the apps directory against the store and deploys/removes as needed.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	scan, err := r.scanAppsDir()
	if err != nil {
		return fmt.Errorf("scan apps dir: %w", err)
	}
	desired := scan.desired

	current, err := r.store.ListApps()
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}

	currentMap := make(map[string]store.App, len(current))
	for _, a := range current {
		currentMap[a.Slug] = a
	}

	// deploy new or changed apps (max 3 concurrent)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3)
	for slug, cfg := range desired {
		existing, exists := currentMap[slug]
		needsDeploy := !exists
		if exists {
			hash, hashErr := hashFile(cfg.ComposePath)
			if hashErr != nil {
				log.Printf("[reconciler] hash %s: %v", cfg.ComposePath, hashErr)
			}
			needsDeploy = hash != "" && hash != existing.ComposeHash
			// Normalize stored ComposePath if apps_dir changed under us
			// (or the row was imported from a different layout). Without
			// this, API handlers read a stale path and 500.
			if existing.ComposePath != cfg.ComposePath {
				existing.ComposePath = cfg.ComposePath
				if err := r.store.UpsertApp(&existing, nil); err != nil {
					log.Printf("[reconciler] normalize compose path %s: %v", slug, err)
				} else {
					currentMap[slug] = existing
				}
			}
		}
		if !needsDeploy {
			continue
		}
		// Skip slugs that are already being deployed via an API-driven path
		// (handleDeploy spawns its own goroutine). Without this guard the
		// watcher-triggered reconcile races against the API goroutine,
		// double-dispatches docker compose for the same project, and
		// orphans the WS subscribers watching the first deploy.
		if r.IsDeploying(slug) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(slug string, cfg *compose.AppConfig, exists bool) {
			defer wg.Done()
			defer func() { <-sem }()
			action := "deploy"
			if exists {
				action = "redeploy"
			}
			if err := r.deployApp(ctx, slug, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "reconciler: %s %s: %v\n", action, slug, err)
			}
		}(slug, cfg, exists)
	}
	wg.Wait()

	// archive apps no longer on disk
	for _, a := range current {
		if a.ArchivedAt.Valid {
			continue // already archived; do nothing
		}
		if _, exists := desired[a.Slug]; !exists {
			if _, refused := scan.refused[a.Slug]; refused {
				// Still on disk; the scan already logged why it is refused.
				continue
			}
			// Re-stat the compose path before archiving to absorb transient
			// FS hiccups (slow NFS, watcher firing mid-rename, etc.) and
			// files that failed to parse. If the file exists now, skip archive.
			composePath := filepath.Join(r.appsDir, a.Slug, "docker-compose.yml")
			if _, statErr := os.Stat(composePath); statErr == nil {
				log.Printf("[reconciler] skipping archive of %s: docker-compose.yml exists but was not loaded", a.Slug)
				continue
			}
			if err := r.archiveApp(ctx, a.Slug); err != nil {
				fmt.Fprintf(os.Stderr, "reconciler: archive %s: %v\n", a.Slug, err)
			}
		}
	}

	if r.proxy != nil {
		r.updateProxyRoutes(desired, scan.refused)
	}

	return nil
}

// RefreshRoutes re-parses the compose files on disk and reloads the proxy
// routes without redeploying containers. Used after endpoint/access label
// changes where only routing needs to change. Also updates the stored
// compose hash so a subsequent Reconcile doesn't falsely redeploy.
func (r *Reconciler) RefreshRoutes(ctx context.Context) error {
	scan, err := r.scanAppsDir()
	if err != nil {
		return fmt.Errorf("scan apps dir: %w", err)
	}
	for slug, cfg := range scan.desired {
		hash, hashErr := hashFile(cfg.ComposePath)
		if hashErr != nil || hash == "" {
			continue
		}
		existing, err := r.store.GetAppBySlug(slug)
		if err != nil || existing.ComposeHash == hash {
			continue
		}
		existing.ComposeHash = hash
		existing.ComposePath = cfg.ComposePath
		if err := r.store.UpsertApp(existing, nil); err != nil {
			log.Printf("[reconciler] RefreshRoutes: update hash %s: %v", slug, err)
		}
	}
	if r.proxy != nil {
		r.updateProxyRoutes(scan.desired, scan.refused)
	}
	return nil
}

// updateProxyRoutes routes desired plus the refused apps withRouteOnlyApps
// keeps. desired and refused must come from the same scan.
func (r *Reconciler) updateProxyRoutes(desired, refused map[string]*compose.AppConfig) {
	apps := r.withRouteOnlyApps(desired, refused)
	ids := r.appIDs()
	// Iterate apps in slug order so route order (and the generated Caddy
	// config) is deterministic across reloads.
	slugs := make([]string, 0, len(apps))
	for slug := range apps {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	var routes []proxy.Route
	for _, slug := range slugs {
		app := apps[slug]
		appRoutes, err := proxy.ResolveRoutes(app, r.resolver)
		if err != nil {
			continue
		}
		// The proxy gives a shared domain to the lowest AppID. Apps not in
		// the store yet sort last so existing apps keep their domains.
		id, ok := ids[slug]
		if !ok {
			id = math.MaxInt64
		}
		for i := range appRoutes {
			appRoutes[i].AppID = id
		}
		routes = append(routes, appRoutes...)
	}
	if err := r.proxy.SetRoutes(routes); err != nil {
		fmt.Fprintf(os.Stderr, "reconciler: update proxy routes: %v\n", err)
	}
}

// appIDs maps app slugs to store IDs. Empty when the store is unavailable.
func (r *Reconciler) appIDs() map[string]int64 {
	ids := map[string]int64{}
	if r.store == nil {
		return ids
	}
	apps, err := r.store.ListApps()
	if err != nil {
		log.Printf("[reconciler] list apps for route ownership: %v", err)
		return ids
	}
	for _, a := range apps {
		ids[a.Slug] = a.ID
	}
	return ids
}

// ensureSharedNetwork rewrites the compose file at path atomically if the
// shared-network declaration is missing. Idempotent and nil-safe on errors
// (logs and continues). Returns true iff the file was rewritten.
func ensureSharedNetwork(path string) bool {
	content, err := fsutil.ReadRegularFile(path)
	if err != nil {
		return false
	}
	out, changed, err := compose.InjectSharedNetwork(content, sharedNetworkName)
	if err != nil {
		log.Printf("[reconciler] inject shared network %s: %v", path, err)
		return false
	}
	if !changed {
		return false
	}
	if err := fsutil.WriteFileAtomic(path, out, 0o600); err != nil {
		log.Printf("[reconciler] write %s: %v", path, err)
		return false
	}
	log.Printf("[reconciler] injected shared network into %s", path)
	return true
}

// DeployOne deploys a single app from a compose file path.
func (r *Reconciler) DeployOne(ctx context.Context, composePath, appName string) error {
	cfg, err := compose.ParseFile(composePath, appName)
	if err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}
	return r.deployApp(ctx, appName, cfg)
}

// RemoveOne removes a single app by slug.
func (r *Reconciler) RemoveOne(ctx context.Context, appName string) error {
	return r.removeApp(ctx, appName)
}

func (r *Reconciler) RestartOne(ctx context.Context, slug string) error {
	cfg, err := r.loadAppConfig(slug)
	if err != nil {
		return err
	}
	result := r.deployer.Restart(ctx, cfg)
	if result.Skipped {
		return nil
	}
	action := "restart"
	status := "running"
	if result.Err != nil {
		action = "restart_failed"
		status = "error"
	} else if result.Status == "unstable" {
		action = "restart_unstable"
		status = "unstable"
	}
	r.store.CreateDeployEvent(slug, action, nil, result.Output)
	r.store.UpdateAppStatus(slug, status)
	if result.Err != nil {
		return fmt.Errorf("restart failed, check deploy events for details")
	}
	return nil
}

func (r *Reconciler) StopOne(ctx context.Context, slug string) error {
	if err := r.deployer.Stop(ctx, slug); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return r.store.UpdateAppStatus(slug, "stopped")
}

func (r *Reconciler) StartOne(ctx context.Context, slug string) error {
	if err := r.deployer.Start(ctx, slug); err != nil {
		return fmt.Errorf("start: %w", err)
	}
	return r.store.UpdateAppStatus(slug, "running")
}

func (r *Reconciler) resolveRegistries(app *compose.AppConfig) ([]deployer.RegistryAuth, error) {
	if r.masterSecret == "" {
		return nil, nil
	}

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

func (r *Reconciler) PullOne(ctx context.Context, slug string) error {
	cfg, err := r.loadAppConfig(slug)
	if err != nil {
		return err
	}
	auths, err := r.resolveRegistries(cfg)
	if err != nil {
		return fmt.Errorf("resolve registries: %w", err)
	}
	result := r.deployer.Pull(ctx, cfg, auths)
	if result.Skipped {
		return nil
	}
	action := "pull"
	status := "running"
	if result.Err != nil {
		action = "pull_failed"
		status = "error"
	}
	r.store.CreateDeployEvent(slug, action, nil, result.Output)
	r.store.UpdateAppStatus(slug, status)
	if result.Err != nil {
		return fmt.Errorf("pull failed, check deploy events for details")
	}
	return nil
}

func (r *Reconciler) ScaleOne(ctx context.Context, slug string, scales map[string]int) error {
	cfg, err := r.loadAppConfig(slug)
	if err != nil {
		return err
	}
	if err := r.deployer.Scale(ctx, cfg, scales); err != nil {
		return fmt.Errorf("scale: %w", err)
	}
	return nil
}

func (r *Reconciler) AppServices(ctx context.Context, slug string) ([]deployer.ServiceStatus, error) {
	return r.deployer.Status(ctx, slug)
}

// AppConfig returns the parsed compose config for the given app slug.
func (r *Reconciler) AppConfig(slug string) (*compose.AppConfig, error) {
	return r.loadAppConfig(slug)
}

// RefreshStatuses polls every running/unstable app and flips between those
// two states based on actual container health. Conservative on purpose:
// only detects post-deploy crash loops, never overwrites stopped/error/
// degraded statuses (which are owned by explicit user actions and the
// reconciler's own Reconcile pass) so it cannot race with Stop/Start.
func (r *Reconciler) RefreshStatuses(ctx context.Context) {
	apps, err := r.store.ListApps()
	if err != nil {
		log.Printf("[reconciler] refresh: list apps: %v", err)
		return
	}
	for _, app := range apps {
		if app.Status != "running" && app.Status != "unstable" {
			continue
		}
		if r.IsDeploying(app.Slug) {
			continue
		}
		services, err := r.deployer.Status(ctx, app.Slug)
		if err != nil || len(services) == 0 {
			continue
		}
		bad := false
		for _, s := range services {
			if s.State == "restarting" || s.Health == "unhealthy" {
				bad = true
				break
			}
		}
		newStatus := app.Status
		if bad && app.Status == "running" {
			newStatus = "unstable"
		} else if !bad && app.Status == "unstable" {
			newStatus = "running"
		}
		if newStatus == app.Status {
			continue
		}
		if err := r.store.UpdateAppStatus(app.Slug, newStatus); err != nil {
			log.Printf("[reconciler] refresh %s: update status %q: %v", app.Slug, newStatus, err)
			continue
		}
		log.Printf("[reconciler] %s: status %s -> %s", app.Slug, app.Status, newStatus)
		if r.bus != nil {
			r.bus.Publish(ctx, events.Event{Type: "app.status", Topic: events.AppTopic(app.Slug)})
			r.bus.Publish(ctx, events.Event{Type: "app.status", Topic: events.TopicGlobalApps})
		}
	}
}

// classifyStatus is exported for unit tests to verify the classification
// rules used inside RefreshStatuses.
func classifyStatus(services []deployer.ServiceStatus) string {
	if len(services) == 0 {
		return ""
	}
	hasRunning := false
	hasUnstable := false
	hasStopped := false
	for _, s := range services {
		switch s.State {
		case "running":
			if s.Health == "unhealthy" {
				hasUnstable = true
			} else {
				hasRunning = true
			}
		case "restarting":
			hasUnstable = true
		case "exited", "dead", "removing":
			hasStopped = true
		}
	}
	if hasUnstable {
		return "unstable"
	}
	if hasRunning && hasStopped {
		return "degraded"
	}
	if hasRunning {
		return "running"
	}
	return "stopped"
}

func (r *Reconciler) CancelOne(ctx context.Context, slug string) error {
	cfg, err := r.loadAppConfig(slug)
	if err != nil {
		return err
	}
	return r.deployer.Cancel(ctx, cfg)
}

func (r *Reconciler) IsDeploying(slug string) bool {
	if d, ok := r.deployer.(*deployer.Deployer); ok {
		return d.Tracker.IsDeploying(slug)
	}
	return false
}

// ErrVersionNotFound is returned by RollbackOne when the version does not
// exist or belongs to a different app.
var ErrVersionNotFound = errors.New("version not found for this app")

func (r *Reconciler) RollbackOne(ctx context.Context, slug string, versionID int64) error {
	app, err := r.store.GetAppBySlug(slug)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	ver, err := r.store.GetComposeVersion(versionID)
	if err != nil || ver.AppID != app.ID {
		// A version ID from another app must never be written into this
		// app's folder.
		return ErrVersionNotFound
	}

	composePath := filepath.Join(r.appsDir, slug, "docker-compose.yml")
	composeData := []byte(ver.Content)
	if prefix := os.Getenv("SIMPLEDEPLOY_IMAGE_MIRROR_PREFIX"); prefix != "" {
		composeData = mirror.RewriteCompose(composeData, prefix)
	}
	if os.Getenv("SIMPLEDEPLOY_DISABLE_PORT_LOOPBACK") != "true" {
		composeData = mirror.RewritePortsLoopback(composeData)
	}

	// Check the old version against today's rules before it replaces the
	// current file, so a refused rollback leaves the app as it was.
	dotEnv, err := compose.ReadDotEnv(filepath.Dir(composePath))
	if err != nil {
		return err
	}
	pre, err := compose.ParseContent(composeData, composePath, slug, dotEnv)
	if err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}
	if v := compose.ValidateComposeSecurity(pre); len(v) > 0 {
		return &compose.ViolationError{Violations: v}
	}

	if err := fsutil.WriteFileAtomic(composePath, composeData, 0o600); err != nil {
		return fmt.Errorf("write compose: %w", err)
	}

	cfg, err := compose.ParseFile(composePath, slug)
	if err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}

	auths, authErr := r.resolveRegistries(cfg)
	if authErr != nil {
		log.Printf("[reconciler] resolve registries for %s rollback: %v", slug, authErr)
	}
	cvID := versionID
	result := r.deployer.RollbackDeploy(ctx, cfg, ver.Version, &cvID, auths...)
	if result.Skipped {
		return nil
	}

	action := "rollback"
	status := "running"
	if result.Err != nil {
		action = "rollback_failed"
		status = "error"
	} else if result.Status == "unstable" {
		action = "rollback_unstable"
		status = "unstable"
	}

	labels := make(map[string]string)
	for _, svc := range cfg.Services {
		for k, v := range svc.Labels {
			if strings.HasPrefix(k, "simpledeploy.") {
				if _, exists := labels[k]; !exists {
					labels[k] = v
				}
			}
		}
	}
	// A refused file never ran: keep the stored hash so it cannot qualify
	// for route-only retention (see withRouteOnlyApps).
	hash := app.ComposeHash
	if !refusedByDeployer(result.Err) {
		var hashErr error
		hash, hashErr = hashFile(cfg.ComposePath)
		if hashErr != nil {
			log.Printf("[reconciler] hash %s rollback: %v", cfg.ComposePath, hashErr)
		}
	}
	app = &store.App{
		Name:        slug,
		Slug:        slug,
		ComposePath: cfg.ComposePath,
		Status:      status,
		Domain:      cfg.PrimaryDomain(),
		ComposeHash: hash,
	}
	if err := r.store.UpsertApp(app, labels); err != nil {
		return fmt.Errorf("upsert app: %w", err)
	}
	r.store.CreateDeployEvent(slug, action, nil, fmt.Sprintf("rollback to version %d: %s", ver.Version, result.Output))
	if result.Err != nil {
		return fmt.Errorf("redeploy: %w", result.Err)
	}
	return nil
}

func (r *Reconciler) ListVersions(ctx context.Context, slug string) ([]store.ComposeVersion, error) {
	app, err := r.store.GetAppBySlug(slug)
	if err != nil {
		return nil, err
	}
	return r.store.ListComposeVersions(app.ID)
}

func (r *Reconciler) ListDeployEvents(ctx context.Context, slug string) ([]store.DeployEvent, error) {
	return r.store.ListDeployEvents(slug)
}

func (r *Reconciler) loadAppConfig(slug string) (*compose.AppConfig, error) {
	composePath := filepath.Join(r.appsDir, slug, "docker-compose.yml")
	cfg, err := compose.ParseFile(composePath, slug)
	if err != nil {
		return nil, fmt.Errorf("parse compose for %s: %w", slug, err)
	}
	return cfg, nil
}

// scanAppsDir reads subdirectories and parses each docker-compose.yml.
// Hidden directories (starting with ".") are skipped.
func (r *Reconciler) scanAppsDir() (scanResult, error) {
	entries, err := os.ReadDir(r.appsDir)
	if err != nil {
		return scanResult{}, fmt.Errorf("read dir: %w", err)
	}

	result := make(map[string]*compose.AppConfig)
	refused := make(map[string]*compose.AppConfig)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		composePath := filepath.Join(r.appsDir, name, "docker-compose.yml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			continue
		}

		// Auto-migrate: ensure the shared-network declaration is present.
		// Idempotent; only writes when bytes change.
		ensureSharedNetwork(composePath)

		cfg, err := compose.ParseFile(composePath, name)
		// ErrDotEnv first: a symlinked .env also matches ErrNotRegular.
		if errors.Is(err, compose.ErrDotEnv) {
			// docker compose cannot use this .env either, and the deployer
			// re-parses strictly before every `up`, so nothing new can start
			// from it. Keep routing the running app from the compose file
			// alone instead of dropping it.
			log.Printf("[reconciler] WARNING: %s: %v (app kept; fix .env before the next deploy)", name, err)
			cfg, err = parseNoDotEnv(composePath, name, false)
		} else if errors.Is(err, fsutil.ErrNotRegular) {
			// A symlinked compose file is never deployed, but an app that was
			// running from one before keeps its routes while unchanged.
			if linked, lerr := parseNoDotEnv(composePath, name, true); lerr == nil {
				log.Printf("[reconciler] SECURITY: skipping %s: docker-compose.yml is a symlink; replace it with a regular file", name)
				refused[name] = linked
				continue
			}
		}
		var ve *compose.ViolationError
		if errors.As(err, &ve) {
			// include, label_file or extends outside the app folder: never
			// deployed, but a running app keeps its routes while the file
			// is unchanged (see withRouteOnlyApps).
			log.Printf("[reconciler] SECURITY: skipping %s: %v", name, ve.Violations)
			if routes, rerr := compose.ParseForRoutes(composePath, name); rerr == nil {
				refused[name] = routes
				continue
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "reconciler: parse %s: %v\n", name, err)
			continue
		}
		// Apply the same security validation as the API deploy path so a
		// compose file dropped via gitsync, SSH, or restore cannot bypass
		// the privileged-container / dangerous-bind-mount guards just by
		// being on disk. Skip-and-warn rather than aborting the whole scan.
		if violations := compose.ValidateComposeSecurity(cfg); len(violations) > 0 {
			fmt.Fprintf(os.Stderr, "reconciler: refuse %s: compose security: %v\n", name, violations)
			log.Printf("[reconciler] SECURITY: skipping %s: %v", name, violations)
			refused[name] = cfg
			continue
		}
		// Endpoint label problems (collisions, conflicting tls) are rejected
		// on deploy but only warned about here, so an app written by an older
		// version keeps serving after an upgrade. ResolveRoutes keeps the
		// first endpoint per matcher and the first tls mode per domain.
		if v := compose.ValidateEndpoints(cfg.Endpoints); len(v) > 0 {
			log.Printf("[reconciler] WARNING: %s: endpoint labels: %v (app kept; first matching endpoint and first tls per domain win)", name, v)
		}
		result[name] = cfg
	}
	return scanResult{desired: result, refused: refused}, nil
}

// withRouteOnlyApps returns apps plus refused apps that are still running
// from an unchanged, previously deployed compose file (same hash as the
// last successful deploy). Upgrading to stricter validation then keeps
// existing apps reachable instead of silently dropping their routes; a
// changed file never qualifies, so new content is never routed unchecked.
func (r *Reconciler) withRouteOnlyApps(apps, refused map[string]*compose.AppConfig) map[string]*compose.AppConfig {
	if len(refused) == 0 || r.store == nil {
		return apps
	}
	merged := make(map[string]*compose.AppConfig, len(apps)+len(refused))
	for slug, cfg := range apps {
		merged[slug] = cfg
	}
	for slug, cfg := range refused {
		if _, ok := merged[slug]; ok {
			continue
		}
		existing, err := r.store.GetAppBySlug(slug)
		if err != nil || existing == nil || existing.ArchivedAt.Valid || existing.ComposeHash == "" {
			continue
		}
		hash, err := hashFile(cfg.ComposePath)
		if errors.Is(err, fsutil.ErrNotRegular) {
			// Older versions hashed symlinked compose files through the link.
			hash, err = hashLinkedFile(cfg.ComposePath)
		}
		if err != nil || hash != existing.ComposeHash {
			continue
		}
		log.Printf("[reconciler] WARNING: %s: keeping routes for the running app; fix its compose file before the next deploy", slug)
		merged[slug] = cfg
	}
	return merged
}

// parseNoDotEnv parses the compose file ignoring the app's .env.
// followLink reads a symlinked compose file through the link; that is only
// used to keep routes for an app already running from one, such files are
// never deployed.
func parseNoDotEnv(composePath, name string, followLink bool) (*compose.AppConfig, error) {
	read := fsutil.ReadRegularFile
	if followLink {
		read = os.ReadFile
	}
	content, err := read(composePath)
	if err != nil {
		return nil, err
	}
	return compose.ParseContent(content, composePath, name, nil)
}

// refusedByDeployer reports whether err means the deployer refused the
// compose file (security rules or an unusable .env) before running it.
func refusedByDeployer(err error) bool {
	var ve *compose.ViolationError
	return errors.As(err, &ve) || errors.Is(err, compose.ErrDotEnv)
}

// deployApp calls deployer.Deploy then upserts the app in the store with labels.
func (r *Reconciler) deployApp(ctx context.Context, slug string, cfg *compose.AppConfig) error {
	auths, authErr := r.resolveRegistries(cfg)
	if authErr != nil {
		// Non-fatal: log and continue without auth (compose may still succeed
		// for public images, and the error message surfaces to the deploy event).
		log.Printf("[reconciler] resolve registries for %s: %v", slug, authErr)
	}
	result := r.deployer.Deploy(ctx, cfg, auths...)
	if result.Skipped {
		// Another in-flight deploy for this slug is handling store updates
		// and WS notifications; nothing more to do here.
		return nil
	}

	labels := make(map[string]string)
	for _, svc := range cfg.Services {
		for k, v := range svc.Labels {
			if strings.HasPrefix(k, "simpledeploy.") {
				if _, exists := labels[k]; !exists {
					labels[k] = v
				}
			}
		}
	}

	// A refused file never ran: keep the stored hash and skip the compose
	// version so it cannot qualify for route-only retention (see
	// withRouteOnlyApps).
	refused := refusedByDeployer(result.Err)
	var hash string
	if refused {
		if existing, err := r.store.GetAppBySlug(slug); err == nil {
			hash = existing.ComposeHash
		}
	} else {
		var hashErr error
		hash, hashErr = hashFile(cfg.ComposePath)
		if hashErr != nil {
			log.Printf("[reconciler] hash %s: %v", cfg.ComposePath, hashErr)
		}
	}
	status := "running"
	action := "deploy"
	if result.Err != nil {
		status = "error"
		action = "deploy_failed"
	} else if result.Status == "unstable" {
		status = "unstable"
		action = "deploy_unstable"
	}

	app := &store.App{
		Name:        slug,
		Slug:        slug,
		ComposePath: cfg.ComposePath,
		Status:      status,
		Domain:      cfg.PrimaryDomain(),
		ComposeHash: hash,
	}
	if err := r.store.UpsertApp(app, labels); err != nil {
		return fmt.Errorf("upsert app: %w", err)
	}

	// Rehydrate app config from sidecar if DB had no state (DR recovery path).
	if r.syncer != nil {
		if imported, err := r.syncer.ImportAppSidecarIfMissing(slug); err != nil {
			log.Printf("[configsync] ImportAppSidecarIfMissing %s: %v", slug, err)
		} else if imported {
			log.Printf("[configsync] imported app sidecar for %s", slug)
		}
	}

	if !refused {
		content, _ := fsutil.ReadRegularFile(cfg.ComposePath)
		if len(content) > 0 {
			r.store.CreateComposeVersion(app.ID, string(content), hash)
		}
	}
	r.store.CreateDeployEvent(slug, action, nil, result.Output)

	// Re-resolve proxy routes now that containers are up and attached to
	// the public network. The pre-deploy watcher-triggered Reconcile ran
	// updateProxyRoutes before containers had IPs on the shared network,
	// so its routes fell back to Docker DNS names, unreachable from the
	// host-native Caddy. Scan-and-reroute only; do NOT touch compose
	// hashes here (that would mask the "compose changed" signal for the
	// next Reconcile and prevent legitimate redeploys).
	if result.Err == nil && r.proxy != nil {
		go func() {
			scan, err := r.scanAppsDir()
			if err != nil {
				log.Printf("[reconciler] post-deploy route refresh scan for %s: %v", slug, err)
				return
			}
			r.updateProxyRoutes(scan.desired, scan.refused)
		}()
	}

	if result.Err != nil {
		return fmt.Errorf("deploy: %w", result.Err)
	}
	return nil
}

// hashLinkedFile hashes path through a symlink, matching how older versions
// recorded ComposeHash. Only used for the route-only check.
func hashLinkedFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

func hashFile(path string) (string, error) {
	data, err := fsutil.ReadRegularFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// archiveApp runs Teardown, writes the tombstone, marks the row archived, and
// records an audit entry. Replaces removeApp on the directory-missing branch.
func (r *Reconciler) archiveApp(ctx context.Context, slug string) error {
	// Capture app ID once up front; MarkAppArchived may interleave with other
	// writers, so a single read here avoids a re-query race for the audit row.
	var appID *int64
	if app, err := r.store.GetAppBySlug(slug); err == nil {
		id := app.ID
		appID = &id
	}
	if err := r.deployer.Teardown(ctx, slug); err != nil {
		log.Printf("[reconciler] archive teardown %s: %v", slug, err)
		// continue: still want to mark archived
	}
	now := time.Now().UTC()
	if r.syncer != nil {
		if err := r.syncer.WriteTombstone(slug, now); err != nil {
			log.Printf("[reconciler] archive tombstone %s: %v", slug, err)
		}
	}
	if err := r.store.MarkAppArchived(slug, now); err != nil {
		return fmt.Errorf("mark archived: %w", err)
	}
	if r.audit != nil {
		after, _ := json.Marshal(map[string]any{"name": slug})
		_, _ = r.audit.Record(ctx, audit.RecordReq{
			AppID:    appID,
			AppSlug:  slug,
			Category: "lifecycle",
			Action:   "archived",
			After:    after,
		})
	}
	return nil
}

// removeApp calls deployer.Teardown then purges the app + history from the
// store. Used only by the API DELETE handler now (the dir-watch path archives).
func (r *Reconciler) removeApp(ctx context.Context, slug string) error {
	if err := r.deployer.Teardown(ctx, slug); err != nil {
		return fmt.Errorf("teardown: %w", err)
	}
	if err := r.store.PurgeApp(slug); err != nil {
		return fmt.Errorf("purge app: %w", err)
	}
	if r.syncer != nil {
		if err := r.syncer.DeleteAppSidecar(slug); err != nil {
			log.Printf("[configsync] DeleteAppSidecar %s: %v", slug, err)
		}
	}
	return nil
}
