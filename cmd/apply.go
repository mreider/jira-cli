package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/dt-pm-tools/jira-cli/internal/jira"
	"github.com/dt-pm-tools/jira-cli/internal/markdown"
	"github.com/spf13/cobra"
)

var (
	applyFile string
	dryRun    bool
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Push markdown changes back to JIRA",
	Long:  `Reads a markdown file with YAML frontmatter, compares it to the current JIRA state, and applies changes. Use --dry-run to preview without applying.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if applyFile == "" {
			return fmt.Errorf("--file (-f) is required")
		}

		if err := loadConfig(); err != nil {
			return err
		}

		// Read the markdown file
		content, err := os.ReadFile(applyFile)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		// Parse markdown into ticket
		ticket, err := markdown.Unmarshal(string(content))
		if err != nil {
			return fmt.Errorf("parsing markdown: %w", err)
		}

		client := jira.NewClient(appConfig)

		// Fetch current JIRA state
		current, err := client.GetIssue(ticket.Key)
		if err != nil {
			return fmt.Errorf("fetching current state of %s: %w", ticket.Key, err)
		}

		// Conflict check: compare updated timestamps
		if ticket.Updated != "" && current.Fields.Updated != "" && ticket.Updated != current.Fields.Updated {
			return fmt.Errorf("conflict: %s was modified in JIRA since your last pull.\n  Local:  %s\n  JIRA:   %s\nRe-pull the ticket before pushing.", ticket.Key, ticket.Updated, current.Fields.Updated)
		}

		// Build update payload
		payload, err := markdown.ToUpdatePayload(ticket)
		if err != nil {
			return fmt.Errorf("building update payload: %w", err)
		}

		// Show diff
		changes := computeChanges(current, ticket, payload)
		if len(changes) == 0 {
			fmt.Println("No changes detected.")
			return nil
		}

		fmt.Printf("Changes to %s:\n", ticket.Key)
		for _, change := range changes {
			fmt.Printf("  %s\n", change)
		}
		fmt.Println()

		if dryRun {
			fmt.Println("(dry run - no changes applied)")
			return nil
		}

		// Apply field updates (summary, labels, description)
		hasFieldChanges := false
		if payload.Fields.Summary != current.Fields.Summary {
			hasFieldChanges = true
		}
		if !labelsEqual(payload.Fields.Labels, current.Fields.Labels) {
			hasFieldChanges = true
		}
		if payload.Fields.Description != nil {
			hasFieldChanges = true
		}

		if hasFieldChanges {
			if err := client.UpdateIssue(ticket.Key, *payload); err != nil {
				return fmt.Errorf("updating issue: %w", err)
			}
			fmt.Printf("Updated fields for %s\n", ticket.Key)
		}

		// Handle status transition
		if ticket.Status != "" && !strings.EqualFold(ticket.Status, current.Fields.Status.Name) {
			if err := transitionIssue(client, ticket.Key, ticket.Status); err != nil {
				return fmt.Errorf("transitioning status: %w", err)
			}
			fmt.Printf("Transitioned %s to '%s'\n", ticket.Key, ticket.Status)
		}

		fmt.Println("Done.")
		return nil
	},
}

func computeChanges(current *jira.Issue, ticket *markdown.Ticket, payload *jira.UpdatePayload) []string {
	var changes []string

	// Summary
	if payload.Fields.Summary != current.Fields.Summary {
		changes = append(changes, fmt.Sprintf("title: %q -> %q", current.Fields.Summary, payload.Fields.Summary))
	}

	// Labels
	if !labelsEqual(payload.Fields.Labels, current.Fields.Labels) {
		changes = append(changes, fmt.Sprintf("labels: %v -> %v", current.Fields.Labels, payload.Fields.Labels))
	}

	// Description (just note that it changed; don't show full ADF diff)
	if payload.Fields.Description != nil {
		changes = append(changes, "description: (updated)")
	}

	// Status
	if ticket.Status != "" && !strings.EqualFold(ticket.Status, current.Fields.Status.Name) {
		changes = append(changes, fmt.Sprintf("status: %q -> %q", current.Fields.Status.Name, ticket.Status))
	}

	return changes
}

func labelsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aMap := make(map[string]bool)
	for _, v := range a {
		aMap[v] = true
	}
	for _, v := range b {
		if !aMap[v] {
			return false
		}
	}
	return true
}

func transitionIssue(client *jira.Client, key string, targetStatus string) error {
	transitions, err := client.GetTransitions(key)
	if err != nil {
		return fmt.Errorf("fetching transitions: %w", err)
	}

	for _, t := range transitions {
		if strings.EqualFold(t.To.Name, targetStatus) || strings.EqualFold(t.Name, targetStatus) {
			return client.DoTransition(key, t.ID)
		}
	}

	// List available transitions for user
	var available []string
	for _, t := range transitions {
		available = append(available, fmt.Sprintf("'%s' (-> %s)", t.Name, t.To.Name))
	}

	return fmt.Errorf("no transition found to status %q; available transitions: %s",
		targetStatus, strings.Join(available, ", "))
}

func init() {
	applyCmd.Flags().StringVarP(&applyFile, "file", "f", "", "markdown file to apply (required)")
	applyCmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview changes without applying")
	rootCmd.AddCommand(applyCmd)
}
