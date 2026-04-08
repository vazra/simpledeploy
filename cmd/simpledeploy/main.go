package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "simpledeploy",
	Short: "Lightweight deployment manager for Docker Compose apps",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "/etc/simpledeploy/config.yaml", "config file path")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
