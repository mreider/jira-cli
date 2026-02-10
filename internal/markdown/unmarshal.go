package markdown

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/dt-pm-tools/jira-cli/internal/jira"
	"gopkg.in/yaml.v3"
)

// frontmatter is the YAML frontmatter fields for JIRA issues.
type frontmatter struct {
	Key            string   `yaml:"key"`
	Title          string   `yaml:"title"`
	Status         string   `yaml:"status"`
	StatusCategory string   `yaml:"statusCategory"`
	Type           string   `yaml:"type"`
	Priority       string   `yaml:"priority"`
	Labels         []string `yaml:"labels"`
	Assignee       string   `yaml:"assignee"`
	Reporter       string   `yaml:"reporter"`
	URL            string   `yaml:"url"`
	Synced         string   `yaml:"synced"`
}

// confluenceFrontmatter is the YAML frontmatter fields for Confluence pages.
type confluenceFrontmatter struct {
	Source    string `yaml:"source"`
	PageID   string `yaml:"pageId"`
	Title    string `yaml:"title"`
	Status   string `yaml:"status"`
	SpaceKey string `yaml:"spaceKey"`
	SpaceName string `yaml:"spaceName"`
	Version  int    `yaml:"version"`
	URL      string `yaml:"url"`
	Synced   string `yaml:"synced"`
}

// Unmarshal parses a markdown file with YAML frontmatter into a Ticket.
func Unmarshal(content string) (*Ticket, error) {
	fm, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var meta frontmatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	if meta.Key == "" {
		return nil, fmt.Errorf("frontmatter missing required 'key' field")
	}

	// Strip the title heading (# KEY: Title) if present
	body = stripTitleHeading(body, meta.Key)

	// Split body from comments section
	desc, comments := splitComments(body)

	ticket := &Ticket{
		Key:      meta.Key,
		Title:    meta.Title,
		Status:   meta.Status,
		Type:     meta.Type,
		Priority: meta.Priority,
		Labels:   meta.Labels,
		Assignee: meta.Assignee,
		Reporter: meta.Reporter,
		URL:      meta.URL,
		Synced:   meta.Synced,
		Body:     strings.TrimSpace(desc),
		Comments: comments,
	}

	return ticket, nil
}

// ToUpdatePayload converts a Ticket into a JIRA API update payload.
func ToUpdatePayload(ticket *Ticket) (*jira.UpdatePayload, error) {
	adf, err := markdownToADF(ticket.Body)
	if err != nil {
		return nil, fmt.Errorf("converting description to ADF: %w", err)
	}

	payload := &jira.UpdatePayload{
		Fields: jira.UpdateFields{
			Summary:     ticket.Title,
			Labels:      ticket.Labels,
			Description: adf,
		},
	}

	return payload, nil
}

// splitFrontmatter separates YAML frontmatter from the body.
func splitFrontmatter(content string) (string, string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return "", "", fmt.Errorf("no YAML frontmatter found (must start with ---)")
	}

	// Find the closing ---
	rest := content[3:]
	rest = strings.TrimLeft(rest, "\n\r")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", "", fmt.Errorf("no closing --- for frontmatter")
	}

	fm := rest[:idx]
	body := rest[idx+4:] // skip past \n---
	body = strings.TrimLeft(body, "\n\r")

	return fm, body, nil
}

// stripTitleHeading removes the "# KEY: Title" heading from the body.
func stripTitleHeading(body string, key string) string {
	lines := strings.SplitN(body, "\n", 2)
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if strings.HasPrefix(first, "# "+key) || strings.HasPrefix(first, "# ") {
			if len(lines) > 1 {
				return strings.TrimLeft(lines[1], "\n\r")
			}
			return ""
		}
	}
	return body
}

// splitComments separates the description from the ## Comments section.
func splitComments(body string) (string, []TicketComment) {
	// Strip the "## Description" heading if present (always, not just when comments exist)
	body = stripDescriptionHeading(body)

	// Look for "## Comments" (case-insensitive)
	re := regexp.MustCompile(`(?m)^## Comments\s*$`)
	loc := re.FindStringIndex(body)
	if loc == nil {
		return body, nil
	}

	desc := body[:loc[0]]
	commentSection := body[loc[1]:]

	comments := parseComments(commentSection)

	return desc, comments
}

// stripDescriptionHeading removes "## Description" from the beginning of the description.
func stripDescriptionHeading(desc string) string {
	trimmed := strings.TrimSpace(desc)
	if strings.HasPrefix(trimmed, "## Description") {
		rest := strings.TrimPrefix(trimmed, "## Description")
		rest = strings.TrimLeft(rest, "\n\r")
		return rest
	}
	return desc
}

// parseComments parses the comments section into TicketComment structs.
func parseComments(section string) []TicketComment {
	// Comments are under ### headings
	re := regexp.MustCompile(`(?m)^### (.+) - (\S+)\s*$`)
	matches := re.FindAllStringSubmatchIndex(section, -1)

	if len(matches) == 0 {
		return nil
	}

	var comments []TicketComment
	for i, match := range matches {
		author := section[match[2]:match[3]]
		date := section[match[4]:match[5]]

		var body string
		start := match[1]
		if i+1 < len(matches) {
			body = section[start:matches[i+1][0]]
		} else {
			body = section[start:]
		}

		comments = append(comments, TicketComment{
			Author: author,
			Date:   date,
			Body:   strings.TrimSpace(body),
		})
	}

	return comments
}

// markdownToADF converts markdown text to an ADF document node.
func markdownToADF(md string) (*jira.ADFNode, error) {
	doc := &jira.ADFNode{
		Type:    "doc",
		Attrs:   map[string]any{"version": 1},
		Content: []jira.ADFNode{},
	}

	lines := strings.Split(md, "\n")
	i := 0

	for i < len(lines) {
		line := lines[i]

		// Empty line - skip
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		// Preserved ADF marker — restore original node from base64 data
		if strings.HasPrefix(strings.TrimSpace(line), preserveStart) {
			node, endI := parsePreservedMarker(lines, i)
			if node != nil {
				doc.Content = append(doc.Content, *node)
				i = endI
				continue
			}
			// If parsing failed, fall through to treat as regular content
		}

		// Horizontal rule
		if strings.TrimSpace(line) == "---" || strings.TrimSpace(line) == "***" || strings.TrimSpace(line) == "___" {
			doc.Content = append(doc.Content, jira.ADFNode{Type: "rule"})
			i++
			continue
		}

		// Heading
		if headingLevel, text := parseHeading(line); headingLevel > 0 {
			doc.Content = append(doc.Content, jira.ADFNode{
				Type:    "heading",
				Attrs:   map[string]any{"level": headingLevel},
				Content: parseInline(text),
			})
			i++
			continue
		}

		// Code block
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			lang := strings.TrimPrefix(strings.TrimSpace(line), "```")
			lang = strings.TrimSpace(lang)
			var codeLines []string
			i++
			for i < len(lines) {
				if strings.TrimSpace(lines[i]) == "```" {
					i++
					break
				}
				codeLines = append(codeLines, lines[i])
				i++
			}
			codeText := strings.Join(codeLines, "\n")
			node := jira.ADFNode{
				Type:    "codeBlock",
				Content: []jira.ADFNode{{Type: "text", Text: codeText}},
			}
			if lang != "" {
				node.Attrs = map[string]any{"language": lang}
			}
			doc.Content = append(doc.Content, node)
			continue
		}

		// Blockquote
		if strings.HasPrefix(line, "> ") || line == ">" {
			var quoteLines []string
			for i < len(lines) && (strings.HasPrefix(lines[i], "> ") || strings.TrimSpace(lines[i]) == ">") {
				stripped := strings.TrimPrefix(lines[i], "> ")
				stripped = strings.TrimPrefix(stripped, ">")
				quoteLines = append(quoteLines, stripped)
				i++
			}
			quoteText := strings.Join(quoteLines, "\n")
			innerDoc, _ := markdownToADF(quoteText)
			doc.Content = append(doc.Content, jira.ADFNode{
				Type:    "blockquote",
				Content: innerDoc.Content,
			})
			continue
		}

		// Unordered list
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			items, newI := parseList(lines, i, false)
			doc.Content = append(doc.Content, jira.ADFNode{
				Type:    "bulletList",
				Content: items,
			})
			i = newI
			continue
		}

		// Ordered list
		if matched, _ := regexp.MatchString(`^\d+\.\s`, line); matched {
			items, newI := parseList(lines, i, true)
			doc.Content = append(doc.Content, jira.ADFNode{
				Type:    "orderedList",
				Content: items,
			})
			i = newI
			continue
		}

		// Markdown table (line starts with |)
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			tableNode, newI := parseMarkdownTable(lines, i)
			if tableNode != nil {
				doc.Content = append(doc.Content, *tableNode)
				i = newI
				continue
			}
			// Fall through to paragraph if not a valid table
		}

		// Regular paragraph - collect until empty line or block element
		var paraLines []string
		for i < len(lines) {
			l := lines[i]
			trimmed := strings.TrimSpace(l)
			if trimmed == "" {
				break
			}
			if strings.HasPrefix(l, "#") || strings.HasPrefix(l, "```") ||
				strings.HasPrefix(l, "> ") || strings.HasPrefix(l, "- ") ||
				strings.HasPrefix(l, "* ") || trimmed == "---" || trimmed == "***" {
				break
			}
			if strings.HasPrefix(trimmed, preserveStart) {
				break
			}
			if strings.HasPrefix(trimmed, "|") {
				break
			}
			if matched, _ := regexp.MatchString(`^\d+\.\s`, l); matched {
				break
			}
			paraLines = append(paraLines, l)
			i++
		}
		if len(paraLines) > 0 {
			text := strings.Join(paraLines, " ")
			doc.Content = append(doc.Content, jira.ADFNode{
				Type:    "paragraph",
				Content: parseInline(text),
			})
		}
	}

	return doc, nil
}

// parseHeading returns the heading level and text, or 0 if not a heading.
func parseHeading(line string) (int, string) {
	if !strings.HasPrefix(line, "#") {
		return 0, ""
	}
	level := 0
	for _, c := range line {
		if c == '#' {
			level++
		} else {
			break
		}
	}
	if level > 6 {
		return 0, ""
	}
	text := strings.TrimSpace(line[level:])
	return level, text
}

// parseList parses a bullet or ordered list from lines starting at index i.
// Handles nested lists via indentation.
func parseList(lines []string, i int, ordered bool) ([]jira.ADFNode, int) {
	var items []jira.ADFNode
	listRe := regexp.MustCompile(`^[-*]\s`)
	ordRe := regexp.MustCompile(`^\d+\.\s`)
	// Indented sub-items (3+ spaces or tab, then list marker)
	indentedBulletRe := regexp.MustCompile(`^(\s{2,}|\t)[-*]\s`)
	indentedOrdRe := regexp.MustCompile(`^(\s{2,}|\t)\d+\.\s`)

	for i < len(lines) {
		line := lines[i]

		isItem := false
		var text string
		if ordered {
			if loc := ordRe.FindStringIndex(line); loc != nil {
				isItem = true
				text = line[loc[1]:]
			}
		} else {
			if loc := listRe.FindStringIndex(line); loc != nil {
				isItem = true
				text = line[loc[1]:]
			}
		}

		if !isItem {
			break
		}

		item := jira.ADFNode{
			Type: "listItem",
			Content: []jira.ADFNode{
				{
					Type:    "paragraph",
					Content: parseInline(text),
				},
			},
		}
		i++

		// Check for indented sub-list items
		if i < len(lines) {
			nextLine := lines[i]
			if indentedBulletRe.MatchString(nextLine) {
				// Collect indented bullet sub-items, strip leading whitespace
				var subLines []string
				for i < len(lines) && indentedBulletRe.MatchString(lines[i]) {
					subLines = append(subLines, strings.TrimLeft(lines[i], " \t"))
					i++
				}
				subItems, _ := parseList(subLines, 0, false)
				item.Content = append(item.Content, jira.ADFNode{
					Type:    "bulletList",
					Content: subItems,
				})
			} else if indentedOrdRe.MatchString(nextLine) {
				// Collect indented ordered sub-items
				var subLines []string
				for i < len(lines) && indentedOrdRe.MatchString(lines[i]) {
					subLines = append(subLines, strings.TrimLeft(lines[i], " \t"))
					i++
				}
				subItems, _ := parseList(subLines, 0, true)
				item.Content = append(item.Content, jira.ADFNode{
					Type:    "orderedList",
					Content: subItems,
				})
			}
		}

		items = append(items, item)
	}

	return items, i
}

// parseMarkdownTable parses a markdown table starting at line i.
// Returns the ADF table node and the line index after the table,
// or nil if the lines don't form a valid table.
func parseMarkdownTable(lines []string, i int) (*jira.ADFNode, int) {
	// Collect all consecutive lines starting with |
	var tableLines []string
	start := i
	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
		tableLines = append(tableLines, strings.TrimSpace(lines[i]))
		i++
	}

	if len(tableLines) < 2 {
		return nil, start + 1
	}

	// Check if second line is a separator (| --- | --- |)
	isSeparator := true
	sepCells := splitTableRow(tableLines[1])
	for _, cell := range sepCells {
		trimmed := strings.TrimSpace(cell)
		cleaned := strings.Trim(trimmed, ":-")
		if cleaned != "" {
			isSeparator = false
			break
		}
	}

	var rows []jira.ADFNode
	headerCells := splitTableRow(tableLines[0])

	if isSeparator {
		// First row is header
		row := buildTableRow(headerCells, true)
		rows = append(rows, row)
		// Data rows start at line 2
		for _, line := range tableLines[2:] {
			cells := splitTableRow(line)
			rows = append(rows, buildTableRow(cells, false))
		}
	} else {
		// No header — all rows are regular
		for _, line := range tableLines {
			cells := splitTableRow(line)
			rows = append(rows, buildTableRow(cells, false))
		}
	}

	return &jira.ADFNode{
		Type:    "table",
		Content: rows,
		Attrs:   map[string]any{"isNumberColumnEnabled": false, "layout": "default"},
	}, i
}

// splitTableRow splits a markdown table row into cell strings.
func splitTableRow(line string) []string {
	// Trim leading/trailing |
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	var cells []string
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(p))
	}
	return cells
}

// buildTableRow creates an ADF tableRow node from cell texts.
func buildTableRow(cells []string, isHeader bool) jira.ADFNode {
	cellType := "tableCell"
	if isHeader {
		cellType = "tableHeader"
	}
	var cellNodes []jira.ADFNode
	for _, text := range cells {
		cellNodes = append(cellNodes, jira.ADFNode{
			Type: cellType,
			Content: []jira.ADFNode{
				{
					Type:    "paragraph",
					Content: parseInline(text),
				},
			},
		})
	}
	return jira.ADFNode{
		Type:    "tableRow",
		Content: cellNodes,
	}
}

// parsePreservedMarker detects a PRESERVED marker block and decodes the
// base64-encoded ADF node. Returns the decoded node and the line index
// after the closing marker, or nil if the block can't be parsed.
func parsePreservedMarker(lines []string, startIdx int) (*jira.ADFNode, int) {
	// Line 0: <!-- PRESERVED: ... -->
	// Line 1: <!-- data:BASE64 -->
	// Line 2: <!-- /PRESERVED -->
	if startIdx+2 >= len(lines) {
		return nil, startIdx + 1
	}

	dataLine := strings.TrimSpace(lines[startIdx+1])
	closeLine := strings.TrimSpace(lines[startIdx+2])

	if !strings.HasPrefix(dataLine, preserveData) || closeLine != preserveEnd {
		return nil, startIdx + 1
	}

	// Extract base64 payload between "<!-- data:" and " -->"
	encoded := strings.TrimPrefix(dataLine, preserveData)
	encoded = strings.TrimSuffix(encoded, " -->")
	encoded = strings.TrimSpace(encoded)

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, startIdx + 1
	}

	var node jira.ADFNode
	if err := json.Unmarshal(decoded, &node); err != nil {
		return nil, startIdx + 1
	}

	return &node, startIdx + 3
}

// UnmarshalConfluencePage parses a markdown file with Confluence frontmatter.
func UnmarshalConfluencePage(content string) (*ConfluenceDoc, error) {
	fm, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	var meta confluenceFrontmatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return nil, fmt.Errorf("parsing frontmatter: %w", err)
	}

	if meta.PageID == "" {
		return nil, fmt.Errorf("frontmatter missing required 'pageId' field")
	}

	if meta.Source != "confluence" {
		return nil, fmt.Errorf("not a Confluence document (source: %q, expected 'confluence')", meta.Source)
	}

	// Strip the title heading (# Title) if present
	body = stripConfluenceTitleHeading(body, meta.Title)

	doc := &ConfluenceDoc{
		PageID:    meta.PageID,
		Title:     meta.Title,
		Status:    meta.Status,
		SpaceKey:  meta.SpaceKey,
		SpaceName: meta.SpaceName,
		Version:   meta.Version,
		URL:       meta.URL,
		Synced:    meta.Synced,
		Body:      strings.TrimSpace(body),
	}

	return doc, nil
}

// stripConfluenceTitleHeading removes the "# Title" heading from the body.
func stripConfluenceTitleHeading(body string, title string) string {
	lines := strings.SplitN(body, "\n", 2)
	if len(lines) > 0 {
		first := strings.TrimSpace(lines[0])
		if first == "# "+title || strings.HasPrefix(first, "# ") {
			if len(lines) > 1 {
				return strings.TrimLeft(lines[1], "\n\r")
			}
			return ""
		}
	}
	return body
}

// BodyToADF converts a markdown body string to an ADF document node.
// This is the public entry point for push commands.
func BodyToADF(markdownBody string) (*jira.ADFNode, error) {
	return markdownToADF(markdownBody)
}

// parseInline converts inline markdown (bold, italic, code, links, strike) to ADF nodes.
func parseInline(text string) []jira.ADFNode {
	if text == "" {
		return []jira.ADFNode{{Type: "text", Text: ""}}
	}

	var nodes []jira.ADFNode

	// Process inline formatting using a simple state machine
	// Order of patterns matters: check longer patterns first
	patterns := []struct {
		re      *regexp.Regexp
		markFn  func(match []string) ([]jira.ADFNode, bool)
	}{
		// Links: [text](url)
		{
			re: regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`),
			markFn: func(match []string) ([]jira.ADFNode, bool) {
				return []jira.ADFNode{{
					Type: "text",
					Text: match[1],
					Marks: []jira.ADFMark{{
						Type:  "link",
						Attrs: map[string]any{"href": match[2]},
					}},
				}}, true
			},
		},
		// Bold: **text**
		{
			re: regexp.MustCompile(`\*\*([^*]+)\*\*`),
			markFn: func(match []string) ([]jira.ADFNode, bool) {
				return []jira.ADFNode{{
					Type:  "text",
					Text:  match[1],
					Marks: []jira.ADFMark{{Type: "strong"}},
				}}, true
			},
		},
		// Strikethrough: ~~text~~
		{
			re: regexp.MustCompile(`~~([^~]+)~~`),
			markFn: func(match []string) ([]jira.ADFNode, bool) {
				return []jira.ADFNode{{
					Type:  "text",
					Text:  match[1],
					Marks: []jira.ADFMark{{Type: "strike"}},
				}}, true
			},
		},
		// Inline code: `text`
		{
			re: regexp.MustCompile("`([^`]+)`"),
			markFn: func(match []string) ([]jira.ADFNode, bool) {
				return []jira.ADFNode{{
					Type:  "text",
					Text:  match[1],
					Marks: []jira.ADFMark{{Type: "code"}},
				}}, true
			},
		},
		// Italic: *text*
		{
			re: regexp.MustCompile(`\*([^*]+)\*`),
			markFn: func(match []string) ([]jira.ADFNode, bool) {
				return []jira.ADFNode{{
					Type:  "text",
					Text:  match[1],
					Marks: []jira.ADFMark{{Type: "em"}},
				}}, true
			},
		},
	}

	remaining := text
	for remaining != "" {
		earliestIdx := len(remaining)
		var earliestMatch []string
		var earliestPattern int = -1
		var earliestLoc []int

		for pi, p := range patterns {
			loc := p.re.FindStringSubmatchIndex(remaining)
			if loc != nil && loc[0] < earliestIdx {
				earliestIdx = loc[0]
				match := make([]string, 0)
				for j := 0; j < len(loc); j += 2 {
					if loc[j] >= 0 {
						match = append(match, remaining[loc[j]:loc[j+1]])
					} else {
						match = append(match, "")
					}
				}
				earliestMatch = match
				earliestPattern = pi
				earliestLoc = loc
			}
		}

		if earliestPattern < 0 {
			// No more patterns found
			if remaining != "" {
				nodes = append(nodes, jira.ADFNode{Type: "text", Text: remaining})
			}
			break
		}

		// Add text before the match
		if earliestIdx > 0 {
			nodes = append(nodes, jira.ADFNode{Type: "text", Text: remaining[:earliestIdx]})
		}

		// Add the matched nodes
		matchedNodes, _ := patterns[earliestPattern].markFn(earliestMatch)
		nodes = append(nodes, matchedNodes...)

		remaining = remaining[earliestLoc[1]:]
	}

	if len(nodes) == 0 {
		return []jira.ADFNode{{Type: "text", Text: text}}
	}

	return nodes
}
