package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/dt-pm-tools/jira-cli/internal/dateparse"
	"github.com/dt-pm-tools/jira-cli/internal/jira"
	"github.com/dt-pm-tools/jira-cli/internal/markdown"
	"github.com/spf13/cobra"
)

var (
	searchJQL        string
	searchProject    string
	searchStatus     string
	searchAssignee   string
	searchReporter   string
	searchType       string
	searchLabels     []string
	searchUpdated    string
	searchCreated    string
	searchText       string
	searchMaxResults int
	searchOrderBy    string
	searchOutputDir  string
)

var searchCmd = &cobra.Command{
	Use:   "search [text query]",
	Short: "Search JIRA issues using JQL or smart filters",
	Long: `Search for JIRA issues using JQL directly or build queries with smart filters.

Examples:
  # Raw JQL
  jira search --jql "project = PRODUCT AND status = 'In Progress'"

  # Smart filters (builds JQL automatically)
  jira search --project PRODUCT --status "In Progress"
  jira search --project PRODUCT --updated recent
  jira search --project PRODUCT --type Bug --assignee me
  jira search -p PRODUCT -l backend -l q1 --updated "last week"

  # Text search
  jira search -q "login error"
  jira search --project PRODUCT -q "deployment"

  # Pull all results to markdown files
  jira search --project PRODUCT --updated recent --output-dir ./tickets

Smart date values for --updated/--created:
  today, yesterday, recent (7 days), last week, this week,
  last month, this month, this quarter, this year, or ISO dates (2025-01-15)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}

		jql, err := buildJQL(args)
		if err != nil {
			return err
		}

		client := jira.NewClient(appConfig)

		result, err := client.SearchIssues(jql, searchMaxResults, 0)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if len(result.Issues) == 0 {
			fmt.Fprintf(os.Stderr, "No issues found.\nJQL: %s\n", jql)
			return nil
		}

		fmt.Fprintf(os.Stderr, "Found %d issues (showing %d)  JQL: %s\n\n", result.Total, len(result.Issues), jql)

		if searchOutputDir != "" {
			return pullSearchResults(client, result.Issues)
		}

		printIssueTable(result.Issues)
		return nil
	},
}

func buildJQL(args []string) (string, error) {
	if searchJQL != "" {
		if hasAnyFilter() {
			fmt.Fprintf(os.Stderr, "Warning: --jql provided, ignoring other filter flags\n")
		}
		return searchJQL, nil
	}

	var conditions []string

	if searchProject != "" {
		conditions = append(conditions, fmt.Sprintf("project = %q", searchProject))
	}
	if searchStatus != "" {
		conditions = append(conditions, fmt.Sprintf("status = %q", searchStatus))
	}
	if searchAssignee != "" {
		if strings.EqualFold(searchAssignee, "me") {
			conditions = append(conditions, "assignee = currentUser()")
		} else {
			conditions = append(conditions, fmt.Sprintf("assignee = %q", searchAssignee))
		}
	}
	if searchReporter != "" {
		if strings.EqualFold(searchReporter, "me") {
			conditions = append(conditions, "reporter = currentUser()")
		} else {
			conditions = append(conditions, fmt.Sprintf("reporter = %q", searchReporter))
		}
	}
	if searchType != "" {
		conditions = append(conditions, fmt.Sprintf("issuetype = %q", searchType))
	}
	for _, label := range searchLabels {
		conditions = append(conditions, fmt.Sprintf("labels = %q", label))
	}
	if searchUpdated != "" {
		conditions = append(conditions, dateparse.ToJQLDateClause("updated", searchUpdated))
	}
	if searchCreated != "" {
		conditions = append(conditions, dateparse.ToJQLDateClause("created", searchCreated))
	}
	if searchText != "" {
		conditions = append(conditions, fmt.Sprintf("text ~ %q", searchText))
	}

	// Positional args as text search
	if len(args) > 0 && searchText == "" {
		text := strings.Join(args, " ")
		conditions = append(conditions, fmt.Sprintf("text ~ %q", text))
	}

	if len(conditions) == 0 {
		return "", fmt.Errorf("no search criteria provided; use --jql, --project, -q, or other flags")
	}

	jql := strings.Join(conditions, " AND ")
	if searchOrderBy != "" {
		jql += " ORDER BY " + searchOrderBy
	}

	return jql, nil
}

func hasAnyFilter() bool {
	return searchProject != "" || searchStatus != "" || searchAssignee != "" ||
		searchReporter != "" || searchType != "" || len(searchLabels) > 0 ||
		searchUpdated != "" || searchCreated != "" || searchText != ""
}

func printIssueTable(issues []jira.Issue) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tTYPE\tSTATUS\tASSIGNEE\tUPDATED\tSUMMARY")
	fmt.Fprintln(w, "---\t----\t------\t--------\t-------\t-------")

	for _, issue := range issues {
		assignee := ""
		if issue.Fields.Assignee != nil {
			assignee = issue.Fields.Assignee.DisplayName
		}

		updated := ""
		if issue.Fields.Updated != "" {
			// Truncate to date only
			if len(issue.Fields.Updated) >= 10 {
				updated = issue.Fields.Updated[:10]
			}
		}

		summary := issue.Fields.Summary
		if len(summary) > 60 {
			summary = summary[:57] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			issue.Key,
			issue.Fields.IssueType.Name,
			issue.Fields.Status.Name,
			assignee,
			updated,
			summary,
		)
	}
	w.Flush()
}

func pullSearchResults(client *jira.Client, issues []jira.Issue) error {
	if err := os.MkdirAll(searchOutputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	for _, issue := range issues {
		// Re-fetch with full fields (description, comments)
		full, err := client.GetIssue(issue.Key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch %s: %v\n", issue.Key, err)
			continue
		}

		// Check for existing custom properties
		var customProps map[string]interface{}
		existingPath := filepath.Join(searchOutputDir, issue.Key+".md")
		if existing, err := os.ReadFile(existingPath); err == nil {
			customProps, _ = markdown.ExtractCustomProperties(string(existing))
		}

		md, err := markdown.Marshal(full, appConfig.URL, customProps)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not convert %s: %v\n", issue.Key, err)
			continue
		}

		filename := filepath.Join(searchOutputDir, issue.Key+".md")
		if err := os.WriteFile(filename, []byte(md), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filename, err)
		}
		fmt.Fprintf(os.Stderr, "  %s -> %s\n", issue.Key, filename)
	}

	fmt.Fprintf(os.Stderr, "\nPulled %d issues to %s\n", len(issues), searchOutputDir)
	return nil
}

func init() {
	searchCmd.Flags().StringVar(&searchJQL, "jql", "", "raw JQL query (overrides other filters)")
	searchCmd.Flags().StringVarP(&searchProject, "project", "p", "", "filter by project key")
	searchCmd.Flags().StringVarP(&searchStatus, "status", "s", "", "filter by status")
	searchCmd.Flags().StringVarP(&searchAssignee, "assignee", "a", "", "filter by assignee (email or 'me')")
	searchCmd.Flags().StringVar(&searchReporter, "reporter", "", "filter by reporter (email or 'me')")
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "filter by issue type (Bug, Story, Task, Epic)")
	searchCmd.Flags().StringSliceVarP(&searchLabels, "label", "l", nil, "filter by label (repeatable)")
	searchCmd.Flags().StringVar(&searchUpdated, "updated", "", "filter by updated date (recent, today, last week, etc.)")
	searchCmd.Flags().StringVar(&searchCreated, "created", "", "filter by created date")
	searchCmd.Flags().StringVarP(&searchText, "text", "q", "", "full text search")
	searchCmd.Flags().IntVar(&searchMaxResults, "max-results", 25, "maximum results to return")
	searchCmd.Flags().StringVar(&searchOrderBy, "order-by", "updated DESC", "JQL ORDER BY clause")
	searchCmd.Flags().StringVar(&searchOutputDir, "output-dir", "", "pull all results to markdown files in this directory")
	rootCmd.AddCommand(searchCmd)
}
