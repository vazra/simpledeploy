package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/store"
)

// AppDeployer is the interface the reconciler uses to deploy and remove apps.
type AppDeployer interface {
	Deploy(ctx context.Context, app *compose.AppConfig, auths ...deployer.RegistryAuth) deployer.DeployResult
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
}

// New creates a Reconciler.
func New(st *store.Store, d AppDeployer, p proxy.Proxy, appsDir string, cfg *config.Config) *Reconciler {
	secret := ""
	if cfg != nil {
		secret = cfg.MasterSecret
	}
	return &Reconciler{store: st, deployer: d, proxy: p, appsDir: appsDir, config: cfg, masterSecret: secret}
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
	desired, err := r.scanAppsDir()
	if err != nil {
		return fmt.Errorf("scan apps dir: %w", err)
	}

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
			hash, _ := hashFile(cfg.ComposePath)
			needsDeploy = hash != "" && hash != existing.ComposeHash
		}
		if !needsDeploy {
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

	// remove apps no longer on disk
	for _, a := range current {
		if _, exists := desired[a.Slug]; !exists {
			if err := r.removeApp(ctx, a.Slug); err != nil {
				fmt.Fprintf(os.Stderr, "reconciler: remove %s: %v\n", a.Slug, err)
			}
		}
	}

	if r.proxy != nil {
		r.updateProxyRoutes(desired)
	}

	return nil
}

func (r *Reconciler) updateProxyRoutes(apps map[string]*compose.AppConfig) {
	var routes []proxy.Route
	for _, app := range apps {
		appRoutes, err := proxy.ResolveRoutes(app)
		if err != nil {
			continue
		}
		routes = append(routes, appRoutes...)
	}
	if err := r.proxy.SetRoutes(routes); err != nil {
		fmt.Fprintf(os.Stderr, "reconciler: update proxy routes: %v\n", err)
	}
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
	action := "restart"
	status := "running"
	if result.Err != nil {
		action = "restart_failed"
		status = "error"
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

func (r *Reconciler) RollbackOne(ctx context.Context, slug string, versionID int64) error {
	ver, err := r.store.GetComposeVersion(versionID)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}

	composePath := filepath.Join(r.appsDir, slug, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte(ver.Content), 0644); err != nil {
		return fmt.Errorf("write compose: %w", err)
	}

	cfg, err := compose.ParseFile(composePath, slug)
	if err != nil {
		return fmt.Errorf("parse compose: %w", err)
	}

	if err := r.deployApp(ctx, slug, cfg); err != nil {
		return fmt.Errorf("redeploy: %w", err)
	}

	r.store.CreateDeployEvent(slug, "rollback", nil, fmt.Sprintf("rollback to version %d", ver.Version))
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
func (r *Reconciler) scanAppsDir() (map[string]*compose.AppConfig, error) {
	entries, err := os.ReadDir(r.appsDir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	result := make(map[string]*compose.AppConfig)
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

		cfg, err := compose.ParseFile(composePath, name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reconciler: parse %s: %v\n", name, err)
			continue
		}
		result[name] = cfg
	}
	return result, nil
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

	hash, _ := hashFile(cfg.ComposePath)
	status := "running"
	action := "deploy"
	if result.Err != nil {
		status = "error"
		action = "deploy_failed"
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

	content, _ := os.ReadFile(cfg.ComposePath)
	if len(content) > 0 {
		r.store.CreateComposeVersion(app.ID, string(content), hash)
	}
	r.store.CreateDeployEvent(slug, action, nil, result.Output)

	if result.Err != nil {
		return fmt.Errorf("deploy: %w", result.Err)
	}
	return nil
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// removeApp calls deployer.Teardown then deletes the app from the store.
func (r *Reconciler) removeApp(ctx context.Context, slug string) error {
	if err := r.deployer.Teardown(ctx, slug); err != nil {
		return fmt.Errorf("teardown: %w", err)
	}
	if err := r.store.DeleteApp(slug); err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	return nil
}
