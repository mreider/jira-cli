package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dt-pm-tools/jira-cli/internal/jira"
	"github.com/dt-pm-tools/jira-cli/internal/markdown"
	"github.com/spf13/cobra"
)

var pushFile string
var pushDryRun bool

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push only the document body back to JIRA (description field)",
	Long: `Reads a markdown file with YAML frontmatter and pushes ONLY the body content
back to the JIRA issue's description field. Title, labels, status, and other
metadata in the frontmatter are NOT pushed — they are read-only context.

Use --dry-run to preview the ADF output without applying.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if pushFile == "" {
			return fmt.Errorf("--file (-f) is required")
		}

		if err := loadConfig(); err != nil {
			return err
		}

		// Read the markdown file
		content, err := os.ReadFile(pushFile)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		// Parse markdown into ticket (to get key and body)
		ticket, err := markdown.Unmarshal(string(content))
		if err != nil {
			return fmt.Errorf("parsing markdown: %w", err)
		}

		// Convert body to ADF
		adf, err := markdown.BodyToADF(ticket.Body)
		if err != nil {
			return fmt.Errorf("converting body to ADF: %w", err)
		}

		client := jira.NewClient(appConfig)

		// Conflict check: compare updated timestamps
		if ticket.Updated != "" {
			current, err := client.GetIssue(ticket.Key)
			if err != nil {
				return fmt.Errorf("checking for conflicts on %s: %w", ticket.Key, err)
			}
			if current.Fields.Updated != "" && ticket.Updated != current.Fields.Updated {
				return fmt.Errorf("conflict: %s was modified in JIRA since your last pull.\n  Local:  %s\n  JIRA:   %s\nRe-pull the ticket before pushing.", ticket.Key, ticket.Updated, current.Fields.Updated)
			}
		}

		if pushDryRun {
			fmt.Fprintf(os.Stderr, "Dry run: would push body to %s\n\n", ticket.Key)
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(adf); err != nil {
				return fmt.Errorf("encoding ADF: %w", err)
			}
			return nil
		}

		// Push only the description
		payload := jira.UpdatePayload{
			Fields: jira.UpdateFields{
				Description: adf,
			},
		}

		if err := client.UpdateIssue(ticket.Key, payload); err != nil {
			return fmt.Errorf("pushing body to %s: %w", ticket.Key, err)
		}

		fmt.Fprintf(os.Stderr, "Pushed body to %s\n", ticket.Key)
		return nil
	},
}

func init() {
	pushCmd.Flags().StringVarP(&pushFile, "file", "f", "", "markdown file to push (required)")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "preview ADF output without pushing")
	rootCmd.AddCommand(pushCmd)
}
