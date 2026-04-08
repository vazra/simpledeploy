package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vazra/simpledeploy/internal/api"
	"github.com/vazra/simpledeploy/internal/config"
	"github.com/vazra/simpledeploy/internal/docker"
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

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/simpledeploy/config.yaml", "config file path")
	rootCmd.AddCommand(serveCmd, initCmd)
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

	srv := api.NewServer(cfg.ManagementPort)
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
