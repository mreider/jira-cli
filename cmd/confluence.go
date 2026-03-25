package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mreider/a-cli/internal/jira"
	"github.com/mreider/a-cli/internal/markdown"
	"github.com/spf13/cobra"
)

var (
	confluenceOutputDir    string
	confluencePushFile     string
	confluencePushDryRun   bool
	confluenceCreateSpace  string
	confluenceCreateTitle  string
	confluenceCreateParent string
	confluenceCreateFile   string
	confluenceRecursive    bool
	confluenceMaxDepth     int
	confluencePushForce    bool
)

var confluenceCmd = &cobra.Command{
	Use:   "confluence",
	Short: "Confluence page operations",
	Long:  `Pull Confluence pages to markdown. Pages are fetched in ADF format and converted using the same markdown converter as JIRA issues, with preserved markers for images and macros.`,
}

var confluenceGetCmd = &cobra.Command{
	Use:   "get <page-id-or-url>",
	Short: "Fetch a Confluence page and output as markdown",
	Long: `Fetches a Confluence page by ID or URL and converts it to markdown with YAML frontmatter.

Accepts either a numeric page ID or a full Confluence URL:
  a-cli confluence get 85962893
  a-cli confluence get https://your-org.atlassian.net/wiki/spaces/SPACE/pages/85962893/Page+Title

Writes to stdout by default, or to a file with --output-dir.

Use --recursive with --output-dir to crawl all child pages and output them
in a hierarchical directory structure. Use --max-depth to limit crawl depth.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := loadConfig(); err != nil {
			return err
		}

		pageID := extractPageID(args[0])
		if pageID == "" {
			return fmt.Errorf("could not extract page ID from %q — expected a numeric ID or Confluence URL", args[0])
		}

		client := jira.NewClient(appConfig)

		page, err := client.GetConfluencePage(pageID)
		if err != nil {
			return fmt.Errorf("fetching page %s: %w", pageID, err)
		}

		// Fetch space info for context
		var space *jira.ConfluenceSpace
		if page.SpaceID != "" {
			s, err := client.GetConfluenceSpace(page.SpaceID)
			if err == nil {
				space = s
			}
			// Non-fatal if space lookup fails
		}

		// Recursive mode: crawl the full page tree
		if confluenceRecursive {
			if confluenceOutputDir == "" {
				return fmt.Errorf("--recursive requires --output-dir")
			}
			return crawlConfluenceTree(client, page, space, confluenceOutputDir, confluenceMaxDepth)
		}

		// Check for existing custom properties to preserve on re-pull
		var customProps map[string]interface{}
		if confluenceOutputDir != "" {
			existingFilename := sanitizeFilename(page.Title) + ".md"
			existingPath := filepath.Join(confluenceOutputDir, existingFilename)
			if existing, err := os.ReadFile(existingPath); err == nil {
				customProps, _ = markdown.ExtractConfluenceCustomProperties(string(existing))
			}
		}

		// Fetch comments
		var footerComments, inlineComments []jira.ConfluenceComment
		if fc, err := client.GetConfluenceFooterComments(pageID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch footer comments: %v\n", err)
		} else {
			footerComments = fc
		}
		if ic, err := client.GetConfluenceInlineComments(pageID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not fetch inline comments: %v\n", err)
		} else {
			inlineComments = ic
		}

		md, err := markdown.MarshalConfluencePage(page, space, customProps, footerComments, inlineComments)
		if err != nil {
			return fmt.Errorf("converting to markdown: %w", err)
		}

		if confluenceOutputDir != "" {
			if err := os.MkdirAll(confluenceOutputDir, 0755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}

			// Use sanitized title as filename
			filename := sanitizeFilename(page.Title) + ".md"
			outPath := filepath.Join(confluenceOutputDir, filename)
			if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
				return fmt.Errorf("writing file: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Written to %s\n", outPath)
		} else {
			fmt.Print(md)
		}

		return nil
	},
}

// extractPageID extracts a numeric page ID from either a raw ID or a Confluence URL.
// Supported URL formats:
//
//	https://org.atlassian.net/wiki/spaces/SPACE/pages/12345/Title
//	https://org.atlassian.net/wiki/spaces/SPACE/pages/12345
func extractPageID(input string) string {
	input = strings.TrimSpace(input)

	// If it's already a numeric ID, return it
	if matched, _ := regexp.MatchString(`^\d+$`, input); matched {
		return input
	}

	// Try to extract from URL: /pages/{id} or /pages/{id}/title
	re := regexp.MustCompile(`/pages/(\d+)`)
	if match := re.FindStringSubmatch(input); match != nil {
		return match[1]
	}

	return ""
}

// sanitizeFilename creates a safe filename from a page title.
func sanitizeFilename(title string) string {
	// Replace unsafe characters with hyphens
	re := regexp.MustCompile(`[^a-zA-Z0-9\-_. ]+`)
	safe := re.ReplaceAllString(title, "-")
	safe = strings.TrimSpace(safe)
	if safe == "" {
		safe = "confluence-page"
	}
	return safe
}

var confluencePushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push only the document body back to a Confluence page",
	Long: `Reads a markdown file with Confluence frontmatter and pushes ONLY the body
content back to the Confluence page. Title, status, space, and other metadata
in the frontmatter are NOT pushed — they are read-only context.

The page version is automatically incremented.

Use --dry-run to preview the ADF output without applying.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if confluencePushFile == "" {
			return fmt.Errorf("--file (-f) is required")
		}

		if err := loadConfig(); err != nil {
			return err
		}

		// Read the markdown file
		content, err := os.ReadFile(confluencePushFile)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		// Parse markdown into ConfluenceDoc
		doc, err := markdown.UnmarshalConfluencePage(string(content))
		if err != nil {
			return fmt.Errorf("parsing markdown: %w", err)
		}

		// Convert body to ADF
		adf, err := markdown.BodyToADF(doc.Body)
		if err != nil {
			return fmt.Errorf("converting body to ADF: %w", err)
		}

		// Serialize ADF to JSON string (Confluence API requires string, not object)
		adfJSON, err := json.Marshal(adf)
		if err != nil {
			return fmt.Errorf("serializing ADF: %w", err)
		}

		if confluencePushDryRun {
			fmt.Fprintf(os.Stderr, "Dry run: would push body to Confluence page %s (version %d → %d)\n\n",
				doc.PageID, doc.Version, doc.Version+1)
			// Pretty-print the ADF
			var pretty json.RawMessage = adfJSON
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(pretty); err != nil {
				return fmt.Errorf("encoding ADF: %w", err)
			}
			return nil
		}

		client := jira.NewClient(appConfig)

		// Check for unresolved inline comments before pushing
		if !confluencePushForce {
			inlineComments, err := client.GetConfluenceInlineComments(doc.PageID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not check for inline comments: %v\n", err)
			} else {
				var unresolved []jira.ConfluenceComment
				for _, c := range inlineComments {
					if c.ResolutionStatus == "" || c.ResolutionStatus == "open" {
						unresolved = append(unresolved, c)
					}
				}
				if len(unresolved) > 0 {
					fmt.Fprintf(os.Stderr, "Page %s has %d unresolved inline comment(s).\n", doc.PageID, len(unresolved))
					fmt.Fprintf(os.Stderr, "Resolve them in Confluence before pushing, or use --force to push anyway.\n\n")
					for _, c := range unresolved {
						body := confluenceCommentBodyText(c)
						sel := confluenceCommentSelection(c)
						if sel != "" {
							fmt.Fprintf(os.Stderr, "  - %s (on: %q)\n", body, sel)
						} else {
							fmt.Fprintf(os.Stderr, "  - %s\n", body)
						}
					}
					fmt.Fprintln(os.Stderr)
					return fmt.Errorf("push blocked: %d unresolved inline comment(s) — use --force to override", len(unresolved))
				}
			}
		}

		// Fetch current page to get latest version (in case it was updated since pull)
		currentPage, err := client.GetConfluencePage(doc.PageID)
		if err != nil {
			return fmt.Errorf("fetching current page %s: %w", doc.PageID, err)
		}

		newVersion := currentPage.Version.Number + 1

		payload := jira.ConfluenceUpdatePayload{
			ID:     doc.PageID,
			Status: "current",
			Title:  currentPage.Title, // keep current title
			Body: jira.ConfluenceUpdateBody{
				Representation: "atlas_doc_format",
				Value:          string(adfJSON),
			},
			Version: jira.ConfluenceUpdateVersion{
				Number:  newVersion,
				Message: "Updated via a-cli push",
			},
		}

		if err := client.UpdateConfluencePage(doc.PageID, payload); err != nil {
			return fmt.Errorf("pushing body to page %s: %w", doc.PageID, err)
		}

		fmt.Fprintf(os.Stderr, "Pushed body to Confluence page %s (version %d → %d)\n",
			doc.PageID, currentPage.Version.Number, newVersion)
		return nil
	},
}

var confluenceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Confluence page",
	Long: `Creates a new Confluence page in the specified space.

Requires --space (space key) and --title. Optionally provide --parent to create
a child page under an existing page (by page ID or URL), and --file to use a
markdown file as the initial body content.

Examples:
  a-cli confluence create --space ENG --title "Decision Log"
  a-cli confluence create --space ENG --title "Decision Log" --parent 85962893
  a-cli confluence create --space ENG --title "Decision Log" --parent https://org.atlassian.net/wiki/spaces/ENG/pages/85962893/Parent --file body.md
  a-cli confluence create --space ENG --title "Decision Log" --output-dir ./pages`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if confluenceCreateSpace == "" {
			return fmt.Errorf("--space is required")
		}
		if confluenceCreateTitle == "" {
			return fmt.Errorf("--title is required")
		}

		if err := loadConfig(); err != nil {
			return err
		}

		client := jira.NewClient(appConfig)

		// Resolve space key → space ID
		space, err := client.GetConfluenceSpaceByKey(confluenceCreateSpace)
		if err != nil {
			return fmt.Errorf("looking up space %q: %w", confluenceCreateSpace, err)
		}

		// Build ADF body
		var adfJSON []byte
		if confluenceCreateFile != "" {
			content, err := os.ReadFile(confluenceCreateFile)
			if err != nil {
				return fmt.Errorf("reading file: %w", err)
			}

			// Strip frontmatter if present (use just the body)
			body := stripFrontmatter(string(content))

			adf, err := markdown.BodyToADF(body)
			if err != nil {
				return fmt.Errorf("converting body to ADF: %w", err)
			}
			adfJSON, err = json.Marshal(adf)
			if err != nil {
				return fmt.Errorf("serializing ADF: %w", err)
			}
		} else {
			// Empty document
			adfJSON = []byte(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[]}]}`)
		}

		// Resolve parent page ID if provided
		parentID := ""
		if confluenceCreateParent != "" {
			parentID = extractPageID(confluenceCreateParent)
			if parentID == "" {
				return fmt.Errorf("could not extract page ID from --parent %q", confluenceCreateParent)
			}
		}

		payload := jira.ConfluenceCreatePayload{
			SpaceID:  space.ID,
			Status:   "current",
			Title:    confluenceCreateTitle,
			ParentID: parentID,
			Body: jira.ConfluenceUpdateBody{
				Representation: "atlas_doc_format",
				Value:          string(adfJSON),
			},
		}

		page, err := client.CreateConfluencePage(payload)
		if err != nil {
			return fmt.Errorf("creating page: %w", err)
		}

		pageURL := ""
		if page.Links.Base != "" && page.Links.WebUI != "" {
			pageURL = page.Links.Base + page.Links.WebUI
		}

		fmt.Fprintf(os.Stderr, "Created Confluence page %s: %s\n", page.ID, confluenceCreateTitle)
		if pageURL != "" {
			fmt.Fprintf(os.Stderr, "URL: %s\n", pageURL)
		}

		// If --output-dir specified, pull the created page back as markdown
		if confluenceOutputDir != "" {
			if err := os.MkdirAll(confluenceOutputDir, 0755); err != nil {
				return fmt.Errorf("creating output directory: %w", err)
			}

			md, err := markdown.MarshalConfluencePage(page, space, nil, nil, nil)
			if err != nil {
				return fmt.Errorf("converting created page to markdown: %w", err)
			}

			filename := sanitizeFilename(page.Title) + ".md"
			outPath := filepath.Join(confluenceOutputDir, filename)
			if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
				return fmt.Errorf("writing file: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Written to %s\n", outPath)
		}

		return nil
	},
}

// stripFrontmatter removes YAML frontmatter and the first H1 heading from markdown content,
// returning just the body text suitable for conversion to ADF.
func stripFrontmatter(content string) string {
	// Remove YAML frontmatter
	if strings.HasPrefix(content, "---\n") {
		if idx := strings.Index(content[4:], "\n---\n"); idx >= 0 {
			content = content[4+idx+5:]
		} else if idx := strings.Index(content[4:], "\n---"); idx >= 0 && 4+idx+4 == len(content) {
			content = ""
		}
	}

	// Remove leading H1 heading (the title line)
	content = strings.TrimLeft(content, "\n")
	if strings.HasPrefix(content, "# ") {
		if idx := strings.Index(content, "\n"); idx >= 0 {
			content = content[idx+1:]
		} else {
			content = ""
		}
	}

	return strings.TrimLeft(content, "\n")
}

// treeNode tracks a page and its position in the tree during recursive crawl.
type treeNode struct {
	pageID  string
	title   string
	dirPath string
	depth   int
}

func crawlConfluenceTree(client *jira.Client, rootPage *jira.ConfluencePage, space *jira.ConfluenceSpace, baseDir string, maxDepth int) error {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Discovering page tree from %q...\n", rootPage.Title)

	// Start with the root page
	rootDir := baseDir
	var toFetch []treeNode
	toFetch = append(toFetch, treeNode{
		pageID:  rootPage.ID,
		title:   rootPage.Title,
		dirPath: rootDir,
		depth:   0,
	})

	// BFS to discover all descendant pages
	queue := []treeNode{toFetch[0]}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if maxDepth > 0 && current.depth >= maxDepth {
			continue
		}

		children, err := client.GetConfluenceChildPages(current.pageID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not get children of %q: %v\n", current.title, err)
			continue
		}

		for _, child := range children {
			childDir := filepath.Join(current.dirPath, sanitizeFilename(child.Title))
			node := treeNode{
				pageID:  child.ID,
				title:   child.Title,
				dirPath: childDir,
				depth:   current.depth + 1,
			}
			toFetch = append(toFetch, node)
			queue = append(queue, node)
		}
	}

	total := len(toFetch)
	fmt.Fprintf(os.Stderr, "Found %d pages. Fetching content...\n\n", total)

	fetched := 0
	for i, node := range toFetch {
		fmt.Fprintf(os.Stderr, "  [%d/%d] %s\n", i+1, total, node.title)

		// For the root page, reuse the already-fetched data
		var page *jira.ConfluencePage
		if i == 0 {
			page = rootPage
		} else {
			var err error
			page, err = client.GetConfluencePage(node.pageID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not fetch page %s (%s): %v\n", node.pageID, node.title, err)
				continue
			}
		}

		if err := os.MkdirAll(node.dirPath, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", node.dirPath, err)
		}

		filename := sanitizeFilename(page.Title) + ".md"
		outPath := filepath.Join(node.dirPath, filename)

		// Check for existing file to preserve custom properties
		var customProps map[string]interface{}
		if existing, err := os.ReadFile(outPath); err == nil {
			customProps, _ = markdown.ExtractConfluenceCustomProperties(string(existing))
		}

		md, err := markdown.MarshalConfluencePage(page, space, customProps, nil, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not convert page %s: %v\n", node.pageID, err)
			continue
		}

		if err := os.WriteFile(outPath, []byte(md), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		fetched++
	}

	fmt.Fprintf(os.Stderr, "\nPulled %d/%d pages to %s\n", fetched, total, baseDir)
	return nil
}

// confluenceCommentBodyText extracts plain text from a comment's storage-format body.
func confluenceCommentBodyText(c jira.ConfluenceComment) string {
	if c.Body.Storage != nil && c.Body.Storage.Value != "" {
		re := regexp.MustCompile(`<[^>]*>`)
		return strings.TrimSpace(re.ReplaceAllString(c.Body.Storage.Value, ""))
	}
	return "(no body)"
}

// confluenceCommentSelection extracts the original selected text from inline comment properties.
func confluenceCommentSelection(c jira.ConfluenceComment) string {
	if c.Properties == nil {
		return ""
	}
	if v, ok := c.Properties["inline-original-selection"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func init() {
	confluenceGetCmd.Flags().StringVar(&confluenceOutputDir, "output-dir", "", "write output to <dir>/<Title>.md instead of stdout")
	confluenceGetCmd.Flags().BoolVar(&confluenceRecursive, "recursive", false, "recursively crawl child pages (requires --output-dir)")
	confluenceGetCmd.Flags().IntVar(&confluenceMaxDepth, "max-depth", 0, "maximum depth for recursive crawl (0 = unlimited)")
	confluencePushCmd.Flags().StringVarP(&confluencePushFile, "file", "f", "", "markdown file to push (required)")
	confluencePushCmd.Flags().BoolVar(&confluencePushDryRun, "dry-run", false, "preview ADF output without pushing")
	confluencePushCmd.Flags().BoolVar(&confluencePushForce, "force", false, "push even if there are unresolved inline comments")
	confluenceCreateCmd.Flags().StringVar(&confluenceCreateSpace, "space", "", "Confluence space key (required)")
	confluenceCreateCmd.Flags().StringVar(&confluenceCreateTitle, "title", "", "page title (required)")
	confluenceCreateCmd.Flags().StringVar(&confluenceCreateParent, "parent", "", "parent page ID or URL (creates child page)")
	confluenceCreateCmd.Flags().StringVarP(&confluenceCreateFile, "file", "f", "", "markdown file for initial body content")
	confluenceCreateCmd.Flags().StringVar(&confluenceOutputDir, "output-dir", "", "write created page to <dir>/<Title>.md")
	confluenceCmd.AddCommand(confluenceGetCmd)
	confluenceCmd.AddCommand(confluencePushCmd)
	confluenceCmd.AddCommand(confluenceCreateCmd)
	rootCmd.AddCommand(confluenceCmd)
}
