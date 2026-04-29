package main

import (
	"context"
	"encoding/binary"
	"errors"
	"net/http"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bufio"
	"bytes"

	"github.com/docker/docker/api/types/container"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"github.com/vazra/simpledeploy/internal/alerts"
	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/logbuf"
	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/api"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/backup"
	"github.com/vazra/simpledeploy/internal/client"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/events"
	"github.com/vazra/simpledeploy/internal/gitsync"
	"github.com/vazra/simpledeploy/internal/metrics"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/recipes"
	"github.com/vazra/simpledeploy/internal/reconciler"
	"github.com/vazra/simpledeploy/internal/store"
	"sync/atomic"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var cfgFile string

// readPassword reads a password from: flag value, SD_PASSWORD env var, or stdin prompt.
func readPassword(cmd *cobra.Command) (string, error) {
	pw, _ := cmd.Flags().GetString("password")
	if pw != "" {
		return pw, nil
	}
	if env := os.Getenv("SD_PASSWORD"); env != "" {
		return env, nil
	}
	fmt.Fprint(os.Stderr, "Password: ")
	b, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}

var rootCmd = &cobra.Command{
	Use:   "simpledeploy",
	Short: "Lightweight deployment manager for Docker Compose apps",
}

var serveCmd = &cobra.Command{
	Use:          "serve",
	Short:        "Start the simpledeploy server",
	SilenceUsage: true,
	RunE:         runServe,
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default config file",
	RunE:  runInit,
}

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Deploy an app from a compose file",
	RunE:  runApply,
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a deployed app",
	RunE:  runRemove,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List deployed apps",
	RunE:  runList,
}

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "Manage users",
}

var usersCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a user",
	RunE:  runUsersCreate,
}

var usersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	RunE:  runUsersList,
}

var usersDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a user",
	RunE:  runUsersDelete,
}

var apikeyCmd = &cobra.Command{
	Use:   "apikey",
	Short: "Manage API keys",
}

var apikeyCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an API key",
	RunE:  runAPIKeyCreate,
}

var apikeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List API keys",
	RunE:  runAPIKeyList,
}

var apikeyRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke an API key",
	RunE:  runAPIKeyRevoke,
}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage backups",
}

var backupRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Trigger backup",
	RunE:  runBackupNow,
}

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List backup runs",
	RunE:  runBackupList,
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore from backup",
	RunE:  runRestore,
}

var logsCmd = &cobra.Command{
	Use:   "logs [app]",
	Short: "Stream app logs",
	Args:  cobra.ExactArgs(1),
	RunE:  runLogs,
}

var contextCmd = &cobra.Command{Use: "context", Short: "Manage remote contexts"}
var contextAddCmd = &cobra.Command{Use: "add <name>", Short: "Add context", Args: cobra.ExactArgs(1), RunE: runContextAdd}
var contextUseCmd = &cobra.Command{Use: "use <name>", Short: "Switch context", Args: cobra.ExactArgs(1), RunE: runContextUse}
var contextListCmd = &cobra.Command{Use: "list", Short: "List contexts", RunE: runContextList}

var pullCmd = &cobra.Command{Use: "pull", Short: "Pull remote app config to local files", RunE: runPull}
var diffCmd = &cobra.Command{Use: "diff", Short: "Diff local vs remote config", RunE: runDiff}
var syncCmd = &cobra.Command{Use: "sync", Short: "Sync local dir to remote", RunE: runSync}

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

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("simpledeploy %s (commit: %s, built: %s)\n", version, commit, date)
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage config sidecars (export/import for DR recovery)",
}

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Git-backed config sync operations",
}

var gitStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Print git sync status",
	SilenceUsage: true,
	RunE:         runGitStatus,
}

var gitSyncNowCmd = &cobra.Command{
	Use:          "sync-now",
	Short:        "Trigger an immediate pull from the remote",
	SilenceUsage: true,
	RunE:         runGitSyncNow,
}

var configExportCmd = &cobra.Command{
	Use:          "export",
	Short:        "Write all config sidecars from current DB state",
	SilenceUsage: true,
	RunE:         runConfigExport,
}

var configImportCmd = &cobra.Command{
	Use:          "import",
	Short:        "Rebuild DB config from sidecars on disk (DR recovery)",
	SilenceUsage: true,
	RunE:         runConfigImport,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/simpledeploy/config.yaml", "config file path")

	applyCmd.Flags().StringP("file", "f", "", "compose file path")
	applyCmd.Flags().StringP("dir", "d", "", "directory of app subdirectories")
	applyCmd.Flags().String("name", "", "app name (required with -f)")

	removeCmd.Flags().String("name", "", "app name to remove")
	removeCmd.MarkFlagRequired("name")

	usersCreateCmd.Flags().String("username", "", "username")
	usersCreateCmd.Flags().String("password", "", "password (reads from stdin or SD_PASSWORD env if omitted)")
	usersCreateCmd.Flags().String("role", "viewer", "role: super_admin, manage, viewer")
	usersCreateCmd.MarkFlagRequired("username")

	usersDeleteCmd.Flags().Int64("id", 0, "user ID")
	usersDeleteCmd.MarkFlagRequired("id")

	usersCmd.AddCommand(usersCreateCmd, usersListCmd, usersDeleteCmd)

	apikeyCreateCmd.Flags().String("name", "", "key name")
	apikeyCreateCmd.Flags().Int64("user-id", 0, "user ID")
	apikeyCreateCmd.MarkFlagRequired("name")
	apikeyCreateCmd.MarkFlagRequired("user-id")

	apikeyRevokeCmd.Flags().Int64("id", 0, "key ID")
	apikeyRevokeCmd.MarkFlagRequired("id")

	apikeyListCmd.Flags().Int64("user-id", 0, "user ID")
	apikeyListCmd.MarkFlagRequired("user-id")

	apikeyCmd.AddCommand(apikeyCreateCmd, apikeyListCmd, apikeyRevokeCmd)

	backupRunCmd.Flags().String("app", "", "app slug")
	backupRunCmd.MarkFlagRequired("app")

	backupListCmd.Flags().String("app", "", "app slug")
	backupListCmd.MarkFlagRequired("app")

	restoreCmd.Flags().String("app", "", "app slug")
	restoreCmd.MarkFlagRequired("app")
	restoreCmd.Flags().Int64("id", 0, "backup run ID")
	restoreCmd.MarkFlagRequired("id")

	backupCmd.AddCommand(backupRunCmd, backupListCmd)

	logsCmd.Flags().BoolP("follow", "f", true, "follow log output")
	logsCmd.Flags().String("tail", "100", "number of lines")
	logsCmd.Flags().String("service", "", "service name")

	contextAddCmd.Flags().String("url", "", "server URL")
	contextAddCmd.Flags().String("api-key", "", "API key")
	contextAddCmd.MarkFlagRequired("url")
	contextAddCmd.MarkFlagRequired("api-key")

	pullCmd.Flags().String("app", "", "app to pull")
	pullCmd.Flags().Bool("all", false, "pull all apps")
	pullCmd.Flags().StringP("output", "o", ".", "output directory")

	diffCmd.Flags().String("app", "", "app to diff")
	diffCmd.Flags().StringP("dir", "d", "", "directory to diff")

	syncCmd.Flags().StringP("dir", "d", "", "directory to sync")
	syncCmd.MarkFlagRequired("dir")

	contextCmd.AddCommand(contextAddCmd, contextUseCmd, contextListCmd)
	rootCmd.AddCommand(contextCmd, pullCmd, diffCmd, syncCmd)

	registryAddCmd.Flags().String("name", "", "registry name")
	registryAddCmd.Flags().String("url", "", "registry URL (e.g. ghcr.io)")
	registryAddCmd.Flags().String("username", "", "username")
	registryAddCmd.Flags().String("password", "", "password (reads from stdin or SD_PASSWORD env if omitted)")
	registryAddCmd.MarkFlagRequired("name")
	registryAddCmd.MarkFlagRequired("url")
	registryAddCmd.MarkFlagRequired("username")

	registryCmd.AddCommand(registryAddCmd, registryListCmd, registryRemoveCmd)

	configImportCmd.Flags().Bool("force", false, "allow import even if DB has some state")
	configImportCmd.Flags().Bool("wipe", false, "truncate config tables before import")
	configCmd.AddCommand(configExportCmd, configImportCmd)

	gitCmd.AddCommand(gitStatusCmd, gitSyncNowCmd)

	rootCmd.AddCommand(serveCmd, initCmd, applyCmd, removeCmd, listCmd, usersCmd, apikeyCmd, backupCmd, restoreCmd, logsCmd, versionCmd, registryCmd, configCmd, gitCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// envDuration returns the duration parsed from env var `name`, or `def` if
// unset or invalid. Used to let e2e/tests dial down production polling
// intervals without changing production defaults.
func envDuration(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
		log.Printf("[config] invalid %s=%q, using default %s", name, v, def)
	}
	return def
}

func scanAndTee(r *os.File, orig *os.File, buf *logbuf.Buffer) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Fprintln(orig, line)
		_, _ = buf.Write([]byte(line))
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// 0700: apps dir contains per-app compose + .env files with secrets.
	if err := os.MkdirAll(cfg.AppsDir, 0o700); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	logBuf := logbuf.New(cfg.LogBufferSize)

	// Redirect stdout/stderr through pipes so ALL output (including Caddy/zap)
	// gets captured into the log buffer while still printing to the terminal.
	origStdout := os.Stdout
	origStderr := os.Stderr
	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()
	os.Stdout = stdoutW
	os.Stderr = stderrW
	log.SetOutput(stderrW)
	go scanAndTee(stdoutR, origStdout, logBuf)
	go scanAndTee(stderrR, origStderr, logBuf)

	log.Printf("simpledeploy starting (data_dir=%s)", cfg.DataDir)

	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	jwtSecret := cfg.MasterSecret
	if jwtSecret == "" {
		// Auto-generate a random secret and persist it
		generated, err := auth.GenerateRandomSecret(32)
		if err != nil {
			return fmt.Errorf("generate secret: %w", err)
		}
		jwtSecret = generated
		fmt.Fprintf(os.Stderr, "WARNING: master_secret not configured. Generated random secret for this session. Set master_secret in config for persistent sessions.\n")
	}
	jwtMgr := auth.NewJWTManager(jwtSecret, 24*time.Hour)
	rlRequests := cfg.RateLimit.Requests
	if rlRequests <= 0 {
		rlRequests = 200
	}
	rlWindow := time.Minute
	if d, err := time.ParseDuration(cfg.RateLimit.Window); err == nil {
		rlWindow = d
	}
	rl := auth.NewRateLimiter(rlRequests, rlWindow)
	lockout := auth.NewLoginLockout(10)

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	if err := dc.Ping(cmd.Context()); err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}

	// Best-effort: ensure the shared bridge network used for endpoint upstream
	// resolution exists. Non-fatal on failure so users who prefer host-port-only
	// deployments are not blocked.
	if err := docker.EnsureNetwork(cmd.Context(), dc, "simpledeploy-public"); err != nil {
		log.Printf("[serve] ensure shared network: %v (continuing)", err)
	}

	proxyCfg := proxy.CaddyConfig{
		ListenAddr: cfg.ListenAddr,
		TLSMode:    cfg.TLS.Mode,
		TLSEmail:   cfg.TLS.Email,
		DataDir:    cfg.DataDir,
	}
	caddyProxy := proxy.NewCaddyProxy(proxyCfg)
	defer func() { _ = caddyProxy.Stop() }()

	dep, err := deployer.New(&deployer.ExecRunner{})
	if err != nil {
		return fmt.Errorf("simpledeploy requires Docker and Docker Compose.\nInstall Docker Engine: https://docs.docker.com/engine/install/\n\n%w", err)
	}

	syncer := configsync.New(db, cfg.AppsDir, cfg.DataDir)
	defer syncer.Close()

	// Rehydrate global config if DB is empty (first boot after DR).
	if imported, err := syncer.ImportGlobalIfEmpty(); err != nil {
		log.Printf("[configsync] global import failed: %v", err)
	} else if imported {
		log.Printf("[configsync] imported global sidecar into empty DB")
	}

	// Prune sidecar files for apps that no longer exist in the DB (e.g. from
	// prior delete cycles before this feature existed).
	if pruned, err := syncer.PruneOrphanSidecars(); err != nil {
		log.Printf("[configsync] prune orphan sidecars: %v", err)
	} else if len(pruned) > 0 {
		log.Printf("[configsync] pruned orphan sidecars for: %v", pruned)
	}

	// Install mutation hook so future mutations update sidecars.
	// FS-authoritative state writes are eventually consistent: this hook fires
	// after a successful DB commit and schedules a debounced FS write (500ms).
	// If the FS write fails, the reconciler watcher (internal/reconciler) re-
	// applies on the next file edit. Contract: DB success => FS reflects soon.
	// See docs/operations/state-on-disk.md.
	db.SetMutationHook(func(scope store.MutationScope, slug string) {
		switch scope {
		case store.ScopeApp:
			if slug != "" {
				syncer.ScheduleAppWrite(slug)
			}
		case store.ScopeGlobal:
			syncer.ScheduleGlobalWrite()
		}
	})

	// First-boot sidecar backfill: write sidecars for all existing DB state
	// so that upgrades from pre-configsync installs get sidecars on first boot.
	backfillMarker := filepath.Join(cfg.DataDir, ".configsync_backfill_v1")
	if _, err := os.Stat(backfillMarker); os.IsNotExist(err) {
		allApps, listErr := db.ListApps()
		if listErr != nil {
			log.Printf("[configsync] backfill: list apps failed: %v (skipping)", listErr)
		} else {
			allOK := true
			if backfillErr := syncer.WriteGlobal(); backfillErr != nil {
				log.Printf("[configsync] backfill: write global failed: %v", backfillErr)
				allOK = false
			}
			for _, a := range allApps {
				if werr := syncer.WriteAppSidecar(a.Slug); werr != nil {
					log.Printf("[configsync] backfill: write sidecar for %s: %v", a.Slug, werr)
					allOK = false
				}
			}
			if allOK {
				log.Printf("[configsync] first-boot sidecar backfill: wrote %d apps + global", len(allApps))
				if werr := os.WriteFile(backfillMarker, nil, 0600); werr != nil {
					log.Printf("[configsync] backfill: write marker failed: %v", werr)
				}
			}
		}
	}

	// GitSync wiring (optional; non-fatal on error).
	// Build syncer using DB-wins resolver. Start deferred to after ctx creation.
	var gitSyncer *gitsync.Syncer
	var recRef atomic.Pointer[reconciler.Reconciler]
	gitReconcilerFn := gitsync.ReconcilerFunc(func(gctx context.Context, paths []string) error {
		r := recRef.Load()
		if r == nil {
			return nil
		}
		return r.Reconcile(gctx)
	})
	{
		yamlGS := cfg.GitSync
		gsCfg, resolveErr := gitsync.ResolveConfig(db, &yamlGS, cfg.AppsDir, cfg.MasterSecret)
		if resolveErr != nil {
			log.Printf("[gitsync] resolve config failed (continuing without git sync): %v", resolveErr)
		} else if gsCfg.Enabled {
			gs, gsErr := gitsync.New(*gsCfg, db, syncer, gitReconcilerFn)
			if gsErr != nil {
				log.Printf("[gitsync] init failed (continuing without git sync): %v", gsErr)
			} else {
				gitSyncer = gs
				syncer.SetSidecarWriteHook(func(path, reason string) {
					if path == "" {
						gs.EnqueueCommit(nil, reason)
						return
					}
					gs.EnqueueCommit([]string{path}, reason)
				})
			}
		}
	}

	// Realtime events bus (in-process publish/subscribe).
	eventBus := events.New()

	reconcilerAudit := audit.NewRecorder(db)
	reconcilerAudit.SetBus(eventBus)

	rec := reconciler.New(db, dep, caddyProxy, cfg.AppsDir, cfg, syncer)
	rec.SetDockerClient(dc)
	rec.SetAuditRecorder(reconcilerAudit)
	rec.SetEventBus(eventBus)

	// Late-bind reconciler pointer for gitsync callback.
	recRef.Store(rec)

	// metrics pipeline
	metricsCh := make(chan metrics.MetricPoint, 500)

	appLookup := func(slug string) (int64, error) {
		app, err := db.GetAppBySlug(slug)
		if err != nil {
			return 0, err
		}
		return app.ID, nil
	}
	collector := metrics.NewCollector(dc, appLookup, metricsCh)
	collector.SetStatusSyncer(&statusSyncAdapter{store: db})
	writer := metrics.NewWriter(db, metricsCh, 100)
	tiers := parseTierConfigs(cfg.Metrics.Tiers)
	rollup := metrics.NewRollupManager(db, tiers)

	// request stats pipeline
	reqStatsCh := make(chan proxy.RequestStatEvent, 1000)
	proxy.RequestStatsCh = reqStatsCh

	domainLookup := func(domain string) (int64, error) {
		host, _, _ := strings.Cut(domain, ":")
		apps, err := db.ListApps()
		if err != nil {
			return 0, err
		}
		for _, a := range apps {
			if a.Domain == host {
				return a.ID, nil
			}
		}
		return 0, fmt.Errorf("unknown domain: %s", domain)
	}
	reqWriter := metrics.NewRequestMetricsWriter(db, reqStatsCh, domainLookup, 200)
	reqRollup := metrics.NewReqMetricsRollupManager(db, tiers)

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// FS-authoritative seed (only on first boot post-upgrade).
	if err := configsync.RunFirstBootSeedIfNeeded(ctx, db, syncer, cfg); err != nil {
		return fmt.Errorf("fs-auth seed: %w", err)
	}
	// Reconcile DB cache from FS files (idempotent on every boot).
	if err := syncer.ReconcileDBFromFS(ctx); err != nil {
		return fmt.Errorf("fs-auth reload: %w", err)
	}

	// Start gitsync now that ctx exists.
	if gitSyncer != nil {
		if startErr := gitSyncer.Start(ctx); startErr != nil {
			log.Printf("[gitsync] start failed (continuing without git sync): %v", startErr)
			gitSyncer = nil
		} else {
			defer func() { _ = gitSyncer.Stop() }()
		}
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()

	go func() {
		if err := rec.Watch(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "reconciler watch: %v\n", err)
		}
	}()

	// Periodically reassess every running app's container health so that
	// crash-loops or unhealthy containers that develop after the post-deploy
	// stabilization window flip the app status to "unstable" without waiting
	// for the next user action.
	go func() {
		ticker := time.NewTicker(envDuration("SIMPLEDEPLOY_STATUS_REFRESH_INTERVAL", 30*time.Second))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rec.RefreshStatuses(ctx)
			}
		}
	}()

	metricsFlush := envDuration("SIMPLEDEPLOY_METRICS_FLUSH_INTERVAL", 10*time.Second)
	reqFlush := envDuration("SIMPLEDEPLOY_REQUEST_METRICS_FLUSH_INTERVAL", 5*time.Second)
	go collector.Run(ctx, metricsFlush)
	go writer.Run(ctx, metricsFlush)
	go rollup.Run(ctx)
	go reqWriter.Run(ctx, reqFlush)
	go reqRollup.Run(ctx)
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := db.IncrementalVacuum(); err != nil {
					log.Printf("incremental vacuum: %v", err)
				}
			}
		}
	}()

	auditPruner := audit.NewPruner(db, 6*time.Hour)
	go auditPruner.Loop(ctx)

	var dispatcher *alerts.WebhookDispatcher
	if os.Getenv("SIMPLEDEPLOY_ALLOW_PRIVATE_WEBHOOKS") == "1" {
		dispatcher = alerts.NewWebhookDispatcherAllowPrivate()
	} else {
		dispatcher = alerts.NewWebhookDispatcher()
	}
	evaluator := alerts.NewEvaluator(db, db, db, dispatcher)
	go evaluator.Run(ctx, envDuration("SIMPLEDEPLOY_ALERT_EVAL_INTERVAL", 30*time.Second))

	count, _ := db.UserCount()
	if count == 0 {
		fmt.Printf("No users found. Create one at: POST http://localhost:%d/api/setup\n", cfg.ManagementPort)
	}

	backupSched := backup.NewScheduler(db, nil)
	backupSched.RegisterStrategy("postgres", backup.NewPostgresStrategy())
	backupSched.RegisterStrategy("mysql", backup.NewMySQLStrategy())
	backupSched.RegisterStrategy("mongo", backup.NewMongoStrategy())
	backupSched.RegisterStrategy("redis", backup.NewRedisStrategy())
	backupSched.RegisterStrategy("sqlite", backup.NewSQLiteStrategy())
	backupSched.RegisterStrategy("volume", backup.NewVolumeStrategy())
	backupSched.RegisterTargetFactory("local", func(configJSON string) (backup.Target, error) {
		return backup.NewLocalTarget(filepath.Join(cfg.DataDir, "backups")), nil
	})
	backupSched.RegisterTargetFactory("s3", func(configJSON string) (backup.Target, error) {
		// Decrypt S3 config if encrypted
		decrypted := configJSON
		if cfg.MasterSecret != "" {
			if plain, err := auth.Decrypt(configJSON, cfg.MasterSecret); err == nil {
				decrypted = plain
			}
		}
		var s3cfg backup.S3Config
		json.Unmarshal([]byte(decrypted), &s3cfg)
		return backup.NewS3Target(s3cfg)
	})
	backupSched.SetAlertFunc(func(appName, strategy, message, eventType string) {
		evaluator.DispatchBackupAlert(alerts.BackupAlertEvent{
			AppName:   appName,
			Strategy:  strategy,
			Message:   message,
			EventType: eventType,
			FiredAt:   time.Now(),
		})
	})
	if err := backupSched.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "backup scheduler: %v\n", err)
	}
	defer backupSched.Stop()

	// Missed backup checker
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				backupSched.CheckMissed()
			}
		}
	}()

	srv := api.NewServer(cfg.ManagementPort, db, jwtMgr, rl)
	srv.SetBackupScheduler(backupSched)
	srv.SetDocker(dc)
	srv.SetAppsDir(cfg.AppsDir)
	srv.SetReconciler(rec)
	srv.SetLockout(lockout)
	srv.SetTrustedProxies(cfg.TrustedProxies)
	srv.SetMasterSecret(cfg.MasterSecret)
	srv.SetBuildInfo(version, commit, date)
	srv.SetDBPath(dbPath)
	srv.SetConfig(cfg, cfgFile)
	srv.SetTLSMode(cfg.TLS.Mode)
	srv.SetDataDir(cfg.DataDir)
	srv.SetWebhookDispatcher(dispatcher)
	auditRec := audit.NewRecorder(db)
	auditRec.SetBus(eventBus)
	srv.SetAudit(auditRec)
	srv.SetBus(eventBus)
	dep.SetAuditEmitter(&api.DeployerAuditAdapter{Rec: auditRec})
	srv.SetLogBuffer(logBuf)
	recipesClient := recipes.NewClient(cfg.RecipesIndexURL, 0)
	srv.SetRecipesCache(recipes.NewCache(recipesClient, 10*time.Minute))
	srv.InitDBBackupSchedule()
	if gitSyncer != nil {
		srv.SetGitSync(gitSyncer)
	}
	srv.SetConfigSync(syncer)
	srv.SetReconcilerRef(gitReconcilerFn)

	distFS, _ := fs.Sub(uiDistFS, "ui_dist")
	srv.SetUIFS(distFS)

	fmt.Printf("simpledeploy listening on :%d\n", cfg.ManagementPort)
	err = srv.ListenAndServe(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runInit(cmd *cobra.Command, args []string) error {
	cfg := config.DefaultConfig()
	data, err := cfg.Marshal()
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(cfgFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	// 0600: config.yaml contains master_secret.
	if err := os.WriteFile(cfgFile, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("config written to %s\n", cfgFile)
	return nil
}

func runApply(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.AppsDir, 0o700); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	dep, err := deployer.New(&deployer.ExecRunner{})
	if err != nil {
		return fmt.Errorf("simpledeploy requires Docker and Docker Compose.\nInstall Docker Engine: https://docs.docker.com/engine/install/\n\n%w", err)
	}
	rec := reconciler.New(db, dep, nil, cfg.AppsDir, cfg, nil)
	ctx := cmd.Context()

	file, _ := cmd.Flags().GetString("file")
	dir, _ := cmd.Flags().GetString("dir")
	name, _ := cmd.Flags().GetString("name")

	if file != "" {
		if name == "" {
			return fmt.Errorf("--name is required when using --file")
		}
		dest, err := copyCompose(file, cfg.AppsDir, name)
		if err != nil {
			return fmt.Errorf("copy compose: %w", err)
		}
		if err := rec.DeployOne(ctx, dest, name); err != nil {
			return fmt.Errorf("deploy %s: %w", name, err)
		}
		fmt.Printf("deployed %s\n", name)
		return nil
	}

	if dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read dir: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			appName := e.Name()
			src := filepath.Join(dir, appName, "docker-compose.yml")
			if _, err := os.Stat(src); os.IsNotExist(err) {
				continue
			}
			dest, err := copyCompose(src, cfg.AppsDir, appName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "apply %s: copy compose: %v\n", appName, err)
				continue
			}
			if err := rec.DeployOne(ctx, dest, appName); err != nil {
				fmt.Fprintf(os.Stderr, "apply %s: %v\n", appName, err)
				continue
			}
			fmt.Printf("deployed %s\n", appName)
		}
		return nil
	}

	return fmt.Errorf("must specify --file or --dir")
}

func runRemove(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	dep, err := deployer.New(&deployer.ExecRunner{})
	if err != nil {
		return fmt.Errorf("simpledeploy requires Docker and Docker Compose.\nInstall Docker Engine: https://docs.docker.com/engine/install/\n\n%w", err)
	}
	rec := reconciler.New(db, dep, nil, cfg.AppsDir, cfg, nil)

	if err := rec.RemoveOne(cmd.Context(), name); err != nil {
		return fmt.Errorf("remove %s: %w", name, err)
	}

	appDir := filepath.Join(cfg.AppsDir, name)
	if err := os.RemoveAll(appDir); err != nil {
		return fmt.Errorf("remove app dir: %w", err)
	}

	fmt.Printf("removed %s\n", name)
	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	apps, err := db.ListApps()
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}

	if len(apps) == 0 {
		fmt.Println("no apps deployed")
		return nil
	}

	fmt.Printf("%-20s %-10s %-30s\n", "NAME", "STATUS", "DOMAIN")
	for _, a := range apps {
		fmt.Printf("%-20s %-10s %-30s\n", a.Name, a.Status, a.Domain)
	}
	return nil
}

func runUsersCreate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	username, _ := cmd.Flags().GetString("username")
	password, err := readPassword(cmd)
	if err != nil {
		return err
	}
	role, _ := cmd.Flags().GetString("role")

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	user, err := db.CreateUser(username, hash, role, "", "")
	if err != nil {
		return err
	}

	fmt.Printf("created user %q (id=%d, role=%s)\n", user.Username, user.ID, user.Role)
	return nil
}

func runUsersList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	users, err := db.ListUsers()
	if err != nil {
		return err
	}

	fmt.Printf("%-5s %-20s %-15s\n", "ID", "USERNAME", "ROLE")
	for _, u := range users {
		fmt.Printf("%-5d %-20s %-15s\n", u.ID, u.Username, u.Role)
	}
	return nil
}

func runUsersDelete(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	id, _ := cmd.Flags().GetInt64("id")
	if err := db.DeleteUser(id); err != nil {
		return err
	}

	fmt.Printf("deleted user %d\n", id)
	return nil
}

func runAPIKeyCreate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	name, _ := cmd.Flags().GetString("name")
	userID, _ := cmd.Flags().GetInt64("user-id")

	plaintext, hash, err := auth.GenerateAPIKey(cfg.MasterSecret)
	if err != nil {
		return err
	}

	_, err = db.CreateAPIKey(userID, hash, name)
	if err != nil {
		return err
	}

	fmt.Printf("API key created: %s\n", plaintext)
	fmt.Println("Save this key - it won't be shown again.")
	return nil
}

func runAPIKeyList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	userID, _ := cmd.Flags().GetInt64("user-id")
	keys, err := db.ListAPIKeysByUser(userID)
	if err != nil {
		return err
	}

	fmt.Printf("%-5s %-20s\n", "ID", "NAME")
	for _, k := range keys {
		fmt.Printf("%-5d %-20s\n", k.ID, k.Name)
	}
	return nil
}

func runAPIKeyRevoke(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	id, _ := cmd.Flags().GetInt64("id")
	if err := db.DeleteAPIKey(id, 0); err != nil { // CLI is admin context
		return err
	}

	fmt.Printf("revoked API key %d\n", id)
	return nil
}

func newBackupScheduler(cfg *config.Config, db *store.Store) *backup.Scheduler {
	sched := backup.NewScheduler(db, nil)
	sched.RegisterStrategy("postgres", backup.NewPostgresStrategy())
	sched.RegisterStrategy("mysql", backup.NewMySQLStrategy())
	sched.RegisterStrategy("mongo", backup.NewMongoStrategy())
	sched.RegisterStrategy("redis", backup.NewRedisStrategy())
	sched.RegisterStrategy("sqlite", backup.NewSQLiteStrategy())
	sched.RegisterStrategy("volume", backup.NewVolumeStrategy())
	sched.RegisterTargetFactory("local", func(configJSON string) (backup.Target, error) {
		return backup.NewLocalTarget(filepath.Join(cfg.DataDir, "backups")), nil
	})
	sched.RegisterTargetFactory("s3", func(configJSON string) (backup.Target, error) {
		decrypted := configJSON
		if cfg.MasterSecret != "" {
			if plain, err := auth.Decrypt(configJSON, cfg.MasterSecret); err == nil {
				decrypted = plain
			}
		}
		var s3cfg backup.S3Config
		json.Unmarshal([]byte(decrypted), &s3cfg)
		return backup.NewS3Target(s3cfg)
	})
	return sched
}

func runBackupNow(cmd *cobra.Command, args []string) error {
	appSlug, _ := cmd.Flags().GetString("app")
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	app, err := db.GetAppBySlug(appSlug)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	appID := app.ID
	cfgs, err := db.ListBackupConfigs(&appID)
	if err != nil {
		return fmt.Errorf("list backup configs: %w", err)
	}
	if len(cfgs) == 0 {
		return fmt.Errorf("no backup config for app %s", appSlug)
	}

	sched := newBackupScheduler(cfg, db)
	if err := sched.RunBackup(cmd.Context(), cfgs[0].ID); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	fmt.Printf("backup completed for app %s\n", appSlug)
	return nil
}

func runBackupList(cmd *cobra.Command, args []string) error {
	appSlug, _ := cmd.Flags().GetString("app")
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	app, err := db.GetAppBySlug(appSlug)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	appID := app.ID
	backupCfgs, err := db.ListBackupConfigs(&appID)
	if err != nil {
		return fmt.Errorf("list backup configs: %w", err)
	}

	fmt.Printf("%-5s %-10s %-12s %-30s\n", "ID", "STATUS", "SIZE", "STARTED")
	for _, bcfg := range backupCfgs {
		runs, err := db.ListBackupRuns(bcfg.ID)
		if err != nil {
			return fmt.Errorf("list runs: %w", err)
		}
		for _, r := range runs {
			size := ""
			if r.SizeBytes != nil {
				size = fmt.Sprintf("%d", *r.SizeBytes)
			}
			fmt.Printf("%-5d %-10s %-12s %-30s\n", r.ID, r.Status, size, r.StartedAt.Format("2006-01-02 15:04:05"))
		}
	}
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	runID, _ := cmd.Flags().GetInt64("id")
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	sched := newBackupScheduler(cfg, db)
	if err := sched.RunRestore(cmd.Context(), runID); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	fmt.Printf("restore completed for run %d\n", runID)
	return nil
}

func runLogs(cmd *cobra.Command, args []string) error {
	appName := args[0]
	follow, _ := cmd.Flags().GetBool("follow")
	tail, _ := cmd.Flags().GetString("tail")
	service, _ := cmd.Flags().GetString("service")
	if service == "" {
		service = "web"
	}

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	containerName := fmt.Sprintf("simpledeploy-%s-%s", appName, service)

	reader, err := dc.ContainerLogs(cmd.Context(), containerName, container.LogsOptions{
		ShowStdout: true, ShowStderr: true,
		Follow: follow, Tail: tail, Timestamps: true,
	})
	if err != nil {
		return fmt.Errorf("container logs: %w", err)
	}
	defer reader.Close()

	hdr := make([]byte, 8)
	for {
		if _, err := io.ReadFull(reader, hdr); err != nil {
			break
		}
		size := binary.BigEndian.Uint32(hdr[4:8])
		line := make([]byte, size)
		if _, err := io.ReadFull(reader, line); err != nil {
			break
		}

		streamType := "stdout"
		if hdr[0] == 2 {
			streamType = "stderr"
		}
		fmt.Printf("[%s] %s", streamType, string(line))
	}
	return nil
}

func getRemoteClient() (*client.Client, error) {
	cfg, err := client.LoadClientConfig()
	if err != nil {
		return nil, err
	}
	ctx, err := cfg.GetCurrentContext()
	if err != nil {
		return nil, fmt.Errorf("no remote context configured, run: simpledeploy context add")
	}
	return client.New(ctx.URL, ctx.APIKey), nil
}

func runContextAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	url, _ := cmd.Flags().GetString("url")
	apiKey, _ := cmd.Flags().GetString("api-key")
	cfg, _ := client.LoadClientConfig()
	cfg.AddContext(name, url, apiKey)
	return client.SaveClientConfig(cfg)
}

func runContextUse(cmd *cobra.Command, args []string) error {
	cfg, _ := client.LoadClientConfig()
	if err := cfg.UseContext(args[0]); err != nil {
		return err
	}
	return client.SaveClientConfig(cfg)
}

func runContextList(cmd *cobra.Command, args []string) error {
	cfg, _ := client.LoadClientConfig()
	for name, ctx := range cfg.Contexts {
		marker := " "
		if name == cfg.CurrentContext {
			marker = "*"
		}
		fmt.Printf("%s %-20s %s\n", marker, name, ctx.URL)
	}
	return nil
}

func runPull(cmd *cobra.Command, args []string) error {
	rc, err := getRemoteClient()
	if err != nil {
		return err
	}
	outputDir, _ := cmd.Flags().GetString("output")
	appName, _ := cmd.Flags().GetString("app")
	all, _ := cmd.Flags().GetBool("all")

	pullOne := func(slug string) error {
		data, err := rc.GetAppCompose(slug)
		if err != nil {
			return fmt.Errorf("get compose for %s: %w", slug, err)
		}
		dir := filepath.Join(outputDir, slug)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		dest := filepath.Join(dir, "docker-compose.yml")
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return err
		}
		fmt.Printf("pulled %s -> %s\n", slug, dest)
		return nil
	}

	if appName != "" {
		return pullOne(appName)
	}
	if all {
		apps, err := rc.ListApps()
		if err != nil {
			return fmt.Errorf("list apps: %w", err)
		}
		for _, a := range apps {
			if err := pullOne(a.Slug); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
		}
		return nil
	}
	return fmt.Errorf("must specify --app or --all")
}

func runDiff(cmd *cobra.Command, args []string) error {
	rc, err := getRemoteClient()
	if err != nil {
		return err
	}
	appName, _ := cmd.Flags().GetString("app")
	dir, _ := cmd.Flags().GetString("dir")

	diffOne := func(slug, localPath string) error {
		local, err := os.ReadFile(localPath)
		if err != nil {
			return fmt.Errorf("read local %s: %w", localPath, err)
		}
		remote, err := rc.GetAppCompose(slug)
		if err != nil {
			return fmt.Errorf("get remote compose for %s: %w", slug, err)
		}
		if bytes.Equal(local, remote) {
			fmt.Printf("%s: matches\n", slug)
			return nil
		}
		fmt.Printf("%s: differs\n", slug)
		localLines := splitLines(local)
		remoteLines := splitLines(remote)
		maxLines := len(localLines)
		if len(remoteLines) > maxLines {
			maxLines = len(remoteLines)
		}
		for i := 0; i < maxLines; i++ {
			var l, r string
			if i < len(localLines) {
				l = localLines[i]
			}
			if i < len(remoteLines) {
				r = remoteLines[i]
			}
			if l != r {
				fmt.Printf("  local  %d: %s\n", i+1, l)
				fmt.Printf("  remote %d: %s\n", i+1, r)
			}
		}
		return nil
	}

	if appName != "" {
		localPath := filepath.Join(".", appName, "docker-compose.yml")
		if dir != "" {
			localPath = filepath.Join(dir, appName, "docker-compose.yml")
		}
		return diffOne(appName, localPath)
	}
	if dir != "" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("read dir: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			localPath := filepath.Join(dir, e.Name(), "docker-compose.yml")
			if _, err := os.Stat(localPath); os.IsNotExist(err) {
				continue
			}
			if err := diffOne(e.Name(), localPath); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
		}
		return nil
	}
	return fmt.Errorf("must specify --app or -d")
}

func splitLines(data []byte) []string {
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func runSync(cmd *cobra.Command, args []string) error {
	rc, err := getRemoteClient()
	if err != nil {
		return err
	}
	dir, _ := cmd.Flags().GetString("dir")

	// collect local apps
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir: %w", err)
	}
	localApps := make(map[string]string)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		composePath := filepath.Join(dir, e.Name(), "docker-compose.yml")
		if _, err := os.Stat(composePath); os.IsNotExist(err) {
			continue
		}
		localApps[e.Name()] = composePath
	}

	// list remote apps
	remoteApps, err := rc.ListApps()
	if err != nil {
		return fmt.Errorf("list remote apps: %w", err)
	}
	remoteSet := make(map[string]struct{})
	for _, a := range remoteApps {
		remoteSet[a.Slug] = struct{}{}
	}

	// deploy local apps
	for name, composePath := range localApps {
		data, err := os.ReadFile(composePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sync %s: read compose: %v\n", name, err)
			continue
		}
		if err := rc.DeployApp(name, data); err != nil {
			fmt.Fprintf(os.Stderr, "sync %s: deploy: %v\n", name, err)
			continue
		}
		fmt.Printf("synced %s\n", name)
	}

	// remove remote apps not in local dir
	for _, a := range remoteApps {
		if _, ok := localApps[a.Slug]; !ok {
			fmt.Printf("removing remote app %s (not in local dir)\n", a.Slug)
			if err := rc.RemoveApp(a.Slug); err != nil {
				fmt.Fprintf(os.Stderr, "remove %s: %v\n", a.Slug, err)
				continue
			}
			fmt.Printf("removed %s\n", a.Slug)
		}
	}

	fmt.Printf("sync complete: %d local, %d remote\n", len(localApps), len(remoteSet))
	return nil
}

func parseTierConfigs(cfgTiers []config.MetricsTier) []metrics.TierConfig {
	var tiers []metrics.TierConfig
	for _, t := range cfgTiers {
		retention, err := time.ParseDuration(t.Retention)
		if err != nil {
			retention = parseDuration(t.Retention)
		}
		tiers = append(tiers, metrics.TierConfig{
			Name:      t.Name,
			Retention: retention,
		})
	}
	return tiers
}

func parseDuration(s string) time.Duration {
	if strings.HasSuffix(s, "d") {
		days, _ := strconv.Atoi(strings.TrimSuffix(s, "d"))
		return time.Duration(days) * 24 * time.Hour
	}
	d, _ := time.ParseDuration(s)
	return d
}

// copyCompose reads the compose file at src and writes it to
// {appsDir}/{name}/docker-compose.yml, returning the destination path.
func copyCompose(src, appsDir, name string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", src, err)
	}

	destDir := filepath.Join(appsDir, name)
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	dest := filepath.Join(destDir, "docker-compose.yml")
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", dest, err)
	}

	return dest, nil
}

type statusSyncAdapter struct {
	store *store.Store
}

func (a *statusSyncAdapter) ListApps() ([]metrics.StatusApp, error) {
	apps, err := a.store.ListApps()
	if err != nil {
		return nil, err
	}
	result := make([]metrics.StatusApp, len(apps))
	for i, app := range apps {
		result[i] = metrics.StatusApp{Slug: app.Slug, Status: app.Status}
	}
	return result, nil
}

func (a *statusSyncAdapter) UpdateAppStatus(slug, status string) error {
	return a.store.UpdateAppStatus(slug, status)
}

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
	password, err := readPassword(cmd)
	if err != nil {
		return err
	}

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

func runConfigExport(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	syncer := configsync.New(db, cfg.AppsDir, cfg.DataDir)
	defer syncer.Close()

	if err := syncer.WriteGlobal(); err != nil {
		return fmt.Errorf("write global sidecar: %w", err)
	}

	apps, err := db.ListApps()
	if err != nil {
		return fmt.Errorf("list apps: %w", err)
	}
	for _, a := range apps {
		if err := syncer.WriteAppSidecar(a.Slug); err != nil {
			return fmt.Errorf("write sidecar for %s: %w", a.Slug, err)
		}
	}

	log.Printf("exported global + %d apps to sidecars", len(apps))
	return nil
}

func runConfigImport(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	wipe, _ := cmd.Flags().GetBool("wipe")

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, "simpledeploy.db"))
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	count, err := db.UserCount()
	if err != nil {
		return fmt.Errorf("check users: %w", err)
	}

	if count > 0 && !force {
		return fmt.Errorf("DB is not empty (%d users); pass --force to proceed", count)
	}
	if count > 0 && force && !wipe {
		return fmt.Errorf("DB is not empty; pass --wipe to clear config tables first")
	}

	if wipe {
		if err := db.WipeConfigForRestore(); err != nil {
			return fmt.Errorf("wipe config tables: %w", err)
		}
		log.Printf("config tables wiped")
	}

	syncer := configsync.New(db, cfg.AppsDir, cfg.DataDir)
	defer syncer.Close()

	// Import global sidecar.
	gdata, err := syncer.ReadGlobal()
	if err != nil {
		return fmt.Errorf("read global sidecar: %w", err)
	}
	if gdata != nil {
		if err := syncer.ImportGlobal(gdata); err != nil {
			return fmt.Errorf("import global: %w", err)
		}
		log.Printf("imported global sidecar")
	} else {
		log.Printf("no global sidecar found, skipping")
	}

	// Import per-app sidecars.
	entries, err := os.ReadDir(cfg.AppsDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read apps dir: %w", err)
	}
	appCount := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		slug := e.Name()
		data, err := syncer.ReadAppSidecar(slug)
		if err != nil {
			log.Printf("read sidecar for %s: %v (skipping)", slug, err)
			continue
		}
		if data == nil {
			continue
		}
		if err := syncer.ImportAppSidecar(data); err != nil {
			log.Printf("import sidecar for %s: %v (skipping)", slug, err)
			continue
		}
		appCount++
	}

	log.Printf("config import complete: %d app sidecars imported", appCount)
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

func buildGitSyncer(cfg *config.Config) (*gitsync.Syncer, error) {
	if !cfg.GitSync.Enabled {
		return nil, fmt.Errorf("gitsync is not enabled in config")
	}
	gsCfg := gitsync.Config{
		Enabled:       true,
		Remote:        cfg.GitSync.Remote,
		Branch:        cfg.GitSync.Branch,
		AppsDir:       cfg.AppsDir,
		AuthorName:    cfg.GitSync.AuthorName,
		AuthorEmail:   cfg.GitSync.AuthorEmail,
		SSHKeyPath:    cfg.GitSync.SSHKeyPath,
		HTTPSUsername: cfg.GitSync.HTTPSUsername,
		HTTPSToken:    cfg.GitSync.HTTPSToken,
		PollInterval:  cfg.GitSync.PollInterval,
		WebhookSecret: cfg.GitSync.WebhookSecret,
	}
	return gitsync.New(gsCfg, nil, nil, nil)
}

func runGitStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	gs, err := buildGitSyncer(cfg)
	if err != nil {
		return err
	}
	st := gs.Status()
	fmt.Printf("enabled:       %v\n", st.Enabled)
	fmt.Printf("remote:        %s\n", st.Remote)
	fmt.Printf("branch:        %s\n", st.Branch)
	fmt.Printf("head:          %s\n", st.HeadSHA)
	fmt.Printf("last_sync:     %s\n", st.LastSyncAt.Format(time.RFC3339))
	fmt.Printf("last_error:    %s\n", st.LastSyncError)
	fmt.Printf("pending:       %d\n", st.PendingCommits)
	fmt.Printf("dropped:       %d\n", st.DroppedRequests)
	return nil
}

func runGitSyncNow(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	gs, err := buildGitSyncer(cfg)
	if err != nil {
		return err
	}
	if err := gs.Start(cmd.Context()); err != nil {
		return fmt.Errorf("start gitsync: %w", err)
	}
	defer func() { _ = gs.Stop() }()
	if err := gs.SyncNow(cmd.Context()); err != nil {
		return fmt.Errorf("sync-now: %w", err)
	}
	fmt.Println("sync complete")
	return nil
}
