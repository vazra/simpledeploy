package reconciler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/store"
)

// AppDeployer is the interface the reconciler uses to deploy and remove apps.
type AppDeployer interface {
	Deploy(ctx context.Context, app *compose.AppConfig) error
	Teardown(ctx context.Context, projectName string) error
	Restart(ctx context.Context, app *compose.AppConfig) error
	Stop(ctx context.Context, projectName string) error
	Start(ctx context.Context, projectName string) error
	Pull(ctx context.Context, app *compose.AppConfig) error
	Scale(ctx context.Context, app *compose.AppConfig, scales map[string]int) error
}

// Reconciler syncs the apps directory with the running containers and store.
type Reconciler struct {
	store    *store.Store
	deployer AppDeployer
	proxy    proxy.Proxy // can be nil
	appsDir  string
}

// New creates a Reconciler.
func New(st *store.Store, d AppDeployer, p proxy.Proxy, appsDir string) *Reconciler {
	return &Reconciler{store: st, deployer: d, proxy: p, appsDir: appsDir}
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

	currentMap := make(map[string]struct{}, len(current))
	for _, a := range current {
		currentMap[a.Slug] = struct{}{}
	}

	// deploy new apps
	for slug, cfg := range desired {
		if _, exists := currentMap[slug]; !exists {
			if err := r.deployApp(ctx, slug, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "reconciler: deploy %s: %v\n", slug, err)
			}
		}
	}

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
		route, err := proxy.ResolveRoute(app)
		if err != nil {
			continue // app without domain, skip
		}
		routes = append(routes, *route)
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
	if err := r.deployer.Restart(ctx, cfg); err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	return r.store.UpdateAppStatus(slug, "running")
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

func (r *Reconciler) PullOne(ctx context.Context, slug string) error {
	cfg, err := r.loadAppConfig(slug)
	if err != nil {
		return err
	}
	if err := r.deployer.Pull(ctx, cfg); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	return r.store.UpdateAppStatus(slug, "running")
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
	if err := r.deployer.Deploy(ctx, cfg); err != nil {
		return fmt.Errorf("deploy: %w", err)
	}

	// collect simpledeploy.* labels from all services
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

	app := &store.App{
		Name:        slug,
		Slug:        slug,
		ComposePath: cfg.ComposePath,
		Status:      "running",
		Domain:      cfg.Domain,
	}
	if err := r.store.UpsertApp(app, labels); err != nil {
		return fmt.Errorf("upsert app: %w", err)
	}
	return nil
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
