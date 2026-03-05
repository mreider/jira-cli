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
	confSearchSpace      string
	confSearchLabels     []string
	confSearchType       string
	confSearchUpdated    string
	confSearchCreated    string
	confSearchContrib    string
	confSearchMaxResults int
)

var confluenceSearchCmd = &cobra.Command{
	Use:   "search [keywords]",
	Short: "Search Confluence pages using CQL or smart filters",
	Long: `Search for Confluence pages using keywords and smart filters.

Examples:
  # Text search
  jira confluence search "deployment guide"

  # Filter by space and labels
  jira confluence search --space ENG --label architecture
  jira confluence search --space ENG --label architecture "deployment"

  # Recent pages
  jira confluence search --space ENG --updated recent
  jira confluence search --updated "last week"

  # Pull results to markdown files
  jira confluence search --space ENG --updated recent --output-dir ./pages

Smart date values for --updated/--created:
  today, yesterday, recent (7 days), last week, this week,
  last month, this month, this quarter, this year, or ISO dates (2025-01-15)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}

		cql, err := buildCQL(args)
		if err != nil {
			return err
		}

		client := jira.NewClient(appConfig)

		result, err := client.SearchConfluence(cql, confSearchMaxResults, 0)
		if err != nil {
			return fmt.Errorf("search failed: %w", err)
		}

		if len(result.Results) == 0 {
			fmt.Fprintf(os.Stderr, "No pages found.\nCQL: %s\n", cql)
			return nil
		}

		fmt.Fprintf(os.Stderr, "Found %d pages (showing %d)  CQL: %s\n\n", result.TotalSize, len(result.Results), cql)

		if confluenceOutputDir != "" {
			return pullConfluenceSearchResults(client, result.Results)
		}

		printConfluenceTable(result.Results, client)
		return nil
	},
}

func buildCQL(args []string) (string, error) {
	var conditions []string

	// Content type
	contentType := "page"
	if confSearchType != "" {
		contentType = confSearchType
	}
	conditions = append(conditions, fmt.Sprintf("type = %q", contentType))

	if confSearchSpace != "" {
		conditions = append(conditions, fmt.Sprintf("space = %q", confSearchSpace))
	}

	for _, label := range confSearchLabels {
		conditions = append(conditions, fmt.Sprintf("label = %q", label))
	}

	if confSearchUpdated != "" {
		conditions = append(conditions, dateparse.ToCQLDateClause("lastModified", confSearchUpdated))
	}
	if confSearchCreated != "" {
		conditions = append(conditions, dateparse.ToCQLDateClause("created", confSearchCreated))
	}
	if confSearchContrib != "" {
		conditions = append(conditions, fmt.Sprintf("contributor = %q", confSearchContrib))
	}

	// Positional args as text search
	if len(args) > 0 {
		text := strings.Join(args, " ")
		conditions = append(conditions, fmt.Sprintf("text ~ %q", text))
	}

	if len(conditions) <= 1 {
		// Only have the type condition — need at least one real filter
		return "", fmt.Errorf("no search criteria provided; use keywords, --space, --label, --updated, or other flags")
	}

	return strings.Join(conditions, " AND "), nil
}

func printConfluenceTable(entries []jira.ConfluenceSearchEntry, client *jira.Client) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSPACE\tLAST MODIFIED\tTITLE")
	fmt.Fprintln(w, "--\t-----\t-------------\t-----")

	for _, entry := range entries {
		space := ""
		if entry.Content.Space.Key != "" {
			space = entry.Content.Space.Key
		}

		lastMod := ""
		if entry.LastModified != "" && len(entry.LastModified) >= 10 {
			lastMod = entry.LastModified[:10]
		}

		title := entry.Content.Title
		if title == "" {
			title = entry.Title
		}
		if len(title) > 60 {
			title = title[:57] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			entry.Content.ID,
			space,
			lastMod,
			title,
		)
	}
	w.Flush()
}

func pullConfluenceSearchResults(client *jira.Client, entries []jira.ConfluenceSearchEntry) error {
	if err := os.MkdirAll(confluenceOutputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	pulled := 0
	for _, entry := range entries {
		pageID := entry.Content.ID
		if pageID == "" {
			continue
		}

		page, err := client.GetConfluencePage(pageID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch page %s: %v\n", pageID, err)
			continue
		}

		var space *jira.ConfluenceSpace
		if page.SpaceID != "" {
			if s, err := client.GetConfluenceSpace(page.SpaceID); err == nil {
				space = s
			}
		}

		md, err := markdown.MarshalConfluencePage(page, space, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not convert page %s: %v\n", pageID, err)
			continue
		}

		filename := sanitizeFilename(page.Title) + ".md"
		outPath := filepath.Join(confluenceOutputDir, filename)
		if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		fmt.Fprintf(os.Stderr, "  %s -> %s\n", page.Title, outPath)
		pulled++
	}

	fmt.Fprintf(os.Stderr, "\nPulled %d pages to %s\n", pulled, confluenceOutputDir)
	return nil
}

func init() {
	confluenceSearchCmd.Flags().StringVarP(&confSearchSpace, "space", "s", "", "filter by space key")
	confluenceSearchCmd.Flags().StringSliceVarP(&confSearchLabels, "label", "l", nil, "filter by label (repeatable)")
	confluenceSearchCmd.Flags().StringVar(&confSearchType, "type", "", "content type: page (default) or blogpost")
	confluenceSearchCmd.Flags().StringVar(&confSearchUpdated, "updated", "", "filter by last modified date (recent, today, last week, etc.)")
	confluenceSearchCmd.Flags().StringVar(&confSearchCreated, "created", "", "filter by created date")
	confluenceSearchCmd.Flags().StringVar(&confSearchContrib, "contributor", "", "filter by contributor")
	confluenceSearchCmd.Flags().IntVar(&confSearchMaxResults, "max-results", 25, "maximum results to return")
	confluenceSearchCmd.Flags().StringVar(&confluenceOutputDir, "output-dir", "", "pull all results to markdown files in this directory")
	confluenceCmd.AddCommand(confluenceSearchCmd)
}
