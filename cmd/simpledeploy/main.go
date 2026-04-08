package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vazra/simpledeploy/internal/api"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/docker"
	"github.com/vazra/simpledeploy/internal/metrics"
	"github.com/vazra/simpledeploy/internal/proxy"
	"github.com/vazra/simpledeploy/internal/reconciler"
	"github.com/vazra/simpledeploy/internal/store"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "simpledeploy",
	Short: "Lightweight deployment manager for Docker Compose apps",
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the simpledeploy server",
	RunE:  runServe,
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

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/simpledeploy/config.yaml", "config file path")

	applyCmd.Flags().StringP("file", "f", "", "compose file path")
	applyCmd.Flags().StringP("dir", "d", "", "directory of app subdirectories")
	applyCmd.Flags().String("name", "", "app name (required with -f)")

	removeCmd.Flags().String("name", "", "app name to remove")
	removeCmd.MarkFlagRequired("name")

	usersCreateCmd.Flags().String("username", "", "username")
	usersCreateCmd.Flags().String("password", "", "password")
	usersCreateCmd.Flags().String("role", "viewer", "role: super_admin, admin, viewer")
	usersCreateCmd.MarkFlagRequired("username")
	usersCreateCmd.MarkFlagRequired("password")

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

	rootCmd.AddCommand(serveCmd, initCmd, applyCmd, removeCmd, listCmd, usersCmd, apikeyCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	if err := os.MkdirAll(cfg.AppsDir, 0755); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	jwtSecret := cfg.MasterSecret
	if jwtSecret == "" {
		jwtSecret = "simpledeploy-default-secret"
	}
	jwtMgr := auth.NewJWTManager(jwtSecret, 24*time.Hour)
	rl := auth.NewRateLimiter(10, time.Minute)

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	if err := dc.Ping(cmd.Context()); err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}

	proxyCfg := proxy.CaddyConfig{
		ListenAddr: cfg.ListenAddr,
		TLSMode:    cfg.TLS.Mode,
		TLSEmail:   cfg.TLS.Email,
	}
	caddyProxy := proxy.NewCaddyProxy(proxyCfg)
	defer caddyProxy.Stop()

	dep := deployer.New(dc)
	rec := reconciler.New(db, dep, caddyProxy, cfg.AppsDir)

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
	writer := metrics.NewWriter(db, metricsCh, 100)
	tiers := parseTierConfigs(cfg.Metrics.Tiers)
	rollup := metrics.NewRollupManager(db, tiers)

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

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

	go collector.Run(ctx, 10*time.Second)
	go writer.Run(ctx, 10*time.Second)
	go rollup.Run(ctx)

	count, _ := db.UserCount()
	if count == 0 {
		fmt.Printf("No users found. Create one at: POST http://localhost:%d/api/setup\n", cfg.ManagementPort)
	}

	srv := api.NewServer(cfg.ManagementPort, db, jwtMgr, rl)
	fmt.Printf("simpledeploy listening on :%d\n", cfg.ManagementPort)
	return srv.ListenAndServe()
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

	if err := os.WriteFile(cfgFile, data, 0644); err != nil {
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

	if err := os.MkdirAll(cfg.AppsDir, 0755); err != nil {
		return fmt.Errorf("create apps dir: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "simpledeploy.db")
	db, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer db.Close()

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	dep := deployer.New(dc)
	rec := reconciler.New(db, dep, nil, cfg.AppsDir)
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

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	dep := deployer.New(dc)
	rec := reconciler.New(db, dep, nil, cfg.AppsDir)

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
	password, _ := cmd.Flags().GetString("password")
	role, _ := cmd.Flags().GetString("role")

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	user, err := db.CreateUser(username, hash, role)
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

	plaintext, hash, err := auth.GenerateAPIKey()
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
	if err := db.DeleteAPIKey(id); err != nil {
		return err
	}

	fmt.Printf("revoked API key %d\n", id)
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
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	dest := filepath.Join(destDir, "docker-compose.yml")
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", fmt.Errorf("write %s: %w", dest, err)
	}

	return dest, nil
}
