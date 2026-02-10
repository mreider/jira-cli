package cmd

import (
	"fmt"
	"os"

	"github.com/dt-pm-tools/jira-cli/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	appConfig config.Config
	version   = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:     "jira",
	Short:   "JIRA <-> Markdown bidirectional sync tool",
	Long:    `A CLI tool for pulling JIRA tickets to markdown and pushing markdown changes back to JIRA. Keeps JIRA access under explicit human control for AI data access policy compliance.`,
	Version: version,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.jira-cli.yaml)")
}

// loadConfig loads and validates configuration. Commands that need JIRA access call this.
func loadConfig() error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w\nRun 'jira config' to set up credentials", err)
	}
	appConfig = cfg
	return nil
}
