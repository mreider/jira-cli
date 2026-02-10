package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/dt-pm-tools/jira-cli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure JIRA connection settings",
	Long:  `Interactively set up JIRA URL, email, and API token. Settings are saved to ~/.jira-cli.yaml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		// Load existing config for defaults
		existing, _ := config.Load(cfgFile)

		// URL
		defaultURL := existing.URL
		if defaultURL != "" {
			fmt.Printf("JIRA URL [%s]: ", defaultURL)
		} else {
			fmt.Print("JIRA URL (e.g., https://your-org.atlassian.net): ")
		}
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)
		if url == "" {
			url = defaultURL
		}

		// Email
		defaultEmail := existing.Email
		if defaultEmail != "" {
			fmt.Printf("Email [%s]: ", defaultEmail)
		} else {
			fmt.Print("Email: ")
		}
		email, _ := reader.ReadString('\n')
		email = strings.TrimSpace(email)
		if email == "" {
			email = defaultEmail
		}

		// Token (masked input)
		fmt.Print("API Token (input hidden): ")
		tokenBytes, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println() // newline after hidden input
		if err != nil {
			return fmt.Errorf("reading token: %w", err)
		}
		token := strings.TrimSpace(string(tokenBytes))
		if token == "" {
			token = existing.Token
		}

		cfg := config.Config{
			URL:   url,
			Email: email,
			Token: token,
		}

		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("invalid config: %w", err)
		}

		path := cfgFile
		if path == "" {
			path = config.DefaultPath()
		}

		if err := config.Save(cfg, path); err != nil {
			return err
		}

		fmt.Printf("Configuration saved to %s\n", path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}
