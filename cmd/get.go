package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dt-pm-tools/jira-cli/internal/jira"
	"github.com/dt-pm-tools/jira-cli/internal/markdown"
	"github.com/spf13/cobra"
)

var (
	outputDir    string
	outputFormat string
)

var getCmd = &cobra.Command{
	Use:   "get <issue-key>",
	Short: "Fetch a JIRA issue and output as markdown",
	Long:  `Fetches a JIRA issue by key and converts it to markdown with YAML frontmatter. Writes to stdout by default, or to a file with --output-dir.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}

		issueKey := strings.ToUpper(args[0])

		client := jira.NewClient(appConfig)
		issue, err := client.GetIssue(issueKey)
		if err != nil {
			return fmt.Errorf("fetching issue %s: %w", issueKey, err)
		}

		md, err := markdown.Marshal(issue, appConfig.URL)
		if err != nil {
			return fmt.Errorf("converting to markdown: %w", err)
		}

		if outputDir != "" {
			// Ensure output directory exists
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}

			filename := filepath.Join(outputDir, issueKey+".md")
			if err := os.WriteFile(filename, []byte(md), 0644); err != nil {
				return fmt.Errorf("writing file: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Written to %s\n", filename)
		} else {
			fmt.Print(md)
		}

		return nil
	},
}

func init() {
	getCmd.Flags().StringVar(&outputDir, "output-dir", "", "write output to <dir>/<KEY>.md instead of stdout")
	getCmd.Flags().StringVarP(&outputFormat, "output", "o", "md", "output format (currently only 'md' supported)")
	rootCmd.AddCommand(getCmd)
}
