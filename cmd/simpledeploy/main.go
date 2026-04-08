package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vazra/simpledeploy/internal/api"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/deployer"
	"github.com/vazra/simpledeploy/internal/docker"
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

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/simpledeploy/config.yaml", "config file path")

	applyCmd.Flags().StringP("file", "f", "", "compose file path")
	applyCmd.Flags().StringP("dir", "d", "", "directory of app subdirectories")
	applyCmd.Flags().String("name", "", "app name (required with -f)")

	removeCmd.Flags().String("name", "", "app name to remove")
	removeCmd.MarkFlagRequired("name")

	rootCmd.AddCommand(serveCmd, initCmd, applyCmd, removeCmd, listCmd)
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

	dc, err := docker.NewClient()
	if err != nil {
		return fmt.Errorf("connect to docker: %w", err)
	}
	defer dc.Close()

	if err := dc.Ping(cmd.Context()); err != nil {
		return fmt.Errorf("docker ping: %w", err)
	}

	dep := deployer.New(dc)
	rec := reconciler.New(db, dep, cfg.AppsDir)

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

	srv := api.NewServer(cfg.ManagementPort, db)
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
	rec := reconciler.New(db, dep, cfg.AppsDir)
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
	rec := reconciler.New(db, dep, cfg.AppsDir)

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
