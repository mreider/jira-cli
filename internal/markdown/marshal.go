package markdown

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mreider/a-cli/internal/jira"
)

// Marker constants for preserved ADF nodes and comments.
const (
	preserveStart = "<!-- PRESERVED:"
	preserveData  = "<!-- data:"
	preserveEnd   = "<!-- /PRESERVED -->"

	// ConfluenceCommentsMarker is written before the ## Comments section so that
	// UnmarshalConfluencePage can reliably strip it (avoids false positives if
	// the page body itself contains a "## Comments" heading).
	ConfluenceCommentsMarker = "<!-- a-cli:comments -->"
)

// yamlQuote returns a YAML-safe string value. If the value contains characters
// that are syntactically significant in YAML (colon, hash, brackets, etc.),
// it is wrapped in double quotes with internal quotes escaped.
func yamlQuote(s string) string {
	if strings.ContainsAny(s, `:#{}[]&*?|->!%@"'` + "\n") {
		escaped := strings.ReplaceAll(s, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

// Human-readable descriptions for preserved ADF node types.
var preservedDescriptions = map[string]string{
	"mediaSingle":          "Inline image",
	"mediaGroup":           "Image group",
	"media":                "Attachment",
	"panel":                "Info/warning panel",
	"expand":               "Expand/collapse section",
	"nestedExpand":         "Nested expand section",
	"extension":            "JIRA extension",
	"bodiedExtension":      "JIRA macro",
	"inlineExtension":      "Inline JIRA macro",
	"multiBodiedExtension": "Multi-body JIRA macro",
	"layoutSection":        "Layout columns",
	"layoutColumn":         "Layout column",
	"decisionList":         "Decision list",
	"decisionItem":         "Decision item",
	"taskList":             "Task checklist",
	"taskItem":             "Task checkbox",
	"status":               "Status lozenge",
	"date":                 "Date",
	"placeholder":          "Placeholder",
}

// Marshal converts a JIRA issue into a markdown string with YAML frontmatter.
// If customProps is non-nil, those properties are preserved after the JIRA-managed
// fields. This allows user-added frontmatter (e.g., local_update_pending, para)
// to survive re-pulls.
func Marshal(issue *jira.Issue, baseURL string, customProps map[string]interface{}) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	var b strings.Builder

	// YAML frontmatter (read-only metadata — not pushed back to JIRA)
	b.WriteString("---\n")
	b.WriteString("# READ-ONLY metadata pulled from JIRA. Changes here are NOT pushed back.\n")
	b.WriteString("# Only the document body (below the frontmatter) is synced on push.\n")
	b.WriteString(fmt.Sprintf("key: %s\n", yamlQuote(issue.Key)))
	b.WriteString(fmt.Sprintf("title: %s\n", yamlQuote(issue.Fields.Summary)))
	b.WriteString(fmt.Sprintf("status: %s\n", yamlQuote(issue.Fields.Status.Name)))
	if issue.Fields.Status.StatusCategory != nil {
		b.WriteString(fmt.Sprintf("statusCategory: %s\n", yamlQuote(issue.Fields.Status.StatusCategory.Name)))
	}
	b.WriteString(fmt.Sprintf("type: %s\n", yamlQuote(issue.Fields.IssueType.Name)))
	if issue.Fields.Priority.Name != "" {
		b.WriteString(fmt.Sprintf("priority: %s\n", yamlQuote(issue.Fields.Priority.Name)))
	}
	if len(issue.Fields.Labels) > 0 {
		b.WriteString(fmt.Sprintf("labels: [%s]\n", strings.Join(issue.Fields.Labels, ", ")))
	} else {
		b.WriteString("labels: []\n")
	}
	if issue.Fields.Assignee != nil {
		b.WriteString(fmt.Sprintf("assignee: %s\n", yamlQuote(issue.Fields.Assignee.EmailAddress)))
	}
	if issue.Fields.Reporter != nil {
		b.WriteString(fmt.Sprintf("reporter: %s\n", yamlQuote(issue.Fields.Reporter.EmailAddress)))
	}
	b.WriteString(fmt.Sprintf("url: %s/browse/%s\n", baseURL, issue.Key))
	if issue.Fields.Updated != "" {
		b.WriteString(fmt.Sprintf("updated: %s\n", issue.Fields.Updated))
	}
	b.WriteString(fmt.Sprintf("synced: %s\n", time.Now().UTC().Format(time.RFC3339)))
	// Preserve custom frontmatter properties from existing file
	if len(customProps) > 0 {
		extra := FormatCustomProperties(customProps)
		if extra != "" {
			b.WriteString(extra)
		}
	}
	b.WriteString("---\n\n")

	// Title
	b.WriteString(fmt.Sprintf("# %s: %s\n\n", issue.Key, issue.Fields.Summary))

	// Description
	b.WriteString("## Description\n\n")
	if issue.Fields.Description != nil {
		desc := renderADF(issue.Fields.Description)
		b.WriteString(desc)
		if !strings.HasSuffix(desc, "\n") {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("(No description)\n")
	}
	b.WriteString("\n")

	// Comments
	if issue.Fields.Comment != nil && len(issue.Fields.Comment.Comments) > 0 {
		b.WriteString("## Comments\n\n")
		for _, c := range issue.Fields.Comment.Comments {
			author := c.Author.EmailAddress
			if author == "" {
				author = c.Author.DisplayName
			}
			date := formatDate(c.Created)
			b.WriteString(fmt.Sprintf("### %s - %s\n\n", author, date))
			if c.Body != nil {
				body := renderADF(c.Body)
				b.WriteString(body)
				if !strings.HasSuffix(body, "\n") {
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		}
	}

	return b.String(), nil
}

// MarshalConfluencePage converts a Confluence page (with ADF body) into markdown
// with YAML frontmatter. Reuses the same ADF→markdown converter as JIRA issues.
// If customProps is non-nil, those properties are preserved after the Confluence-managed fields.
func MarshalConfluencePage(page *jira.ConfluencePage, space *jira.ConfluenceSpace, customProps map[string]interface{}, footerComments, inlineComments []jira.ConfluenceComment) (string, error) {
	var b strings.Builder

	// YAML frontmatter (read-only)
	b.WriteString("---\n")
	b.WriteString("# READ-ONLY metadata pulled from Confluence. Changes here are NOT pushed back.\n")
	b.WriteString("# Only the document body (below the frontmatter) is synced on push.\n")
	b.WriteString(fmt.Sprintf("source: confluence\n"))
	b.WriteString(fmt.Sprintf("pageId: %s\n", yamlQuote(page.ID)))
	b.WriteString(fmt.Sprintf("title: %s\n", yamlQuote(page.Title)))
	b.WriteString(fmt.Sprintf("status: %s\n", yamlQuote(page.Status)))
	if space != nil {
		b.WriteString(fmt.Sprintf("spaceKey: %s\n", yamlQuote(space.Key)))
		b.WriteString(fmt.Sprintf("spaceName: %s\n", yamlQuote(space.Name)))
	}
	b.WriteString(fmt.Sprintf("version: %d\n", page.Version.Number))
	if page.Links.Base != "" && page.Links.WebUI != "" {
		b.WriteString(fmt.Sprintf("url: %s%s\n", page.Links.Base, page.Links.WebUI))
	}
	b.WriteString(fmt.Sprintf("synced: %s\n", time.Now().UTC().Format(time.RFC3339)))
	// Preserve custom frontmatter properties from existing file
	if len(customProps) > 0 {
		extra := FormatCustomProperties(customProps)
		if extra != "" {
			b.WriteString(extra)
		}
	}
	b.WriteString("---\n\n")

	// Title
	b.WriteString(fmt.Sprintf("# %s\n\n", page.Title))

	// Body (ADF → markdown)
	if page.Body.AtlasDocFormat != nil && page.Body.AtlasDocFormat.Value != "" {
		var adfDoc jira.ADFNode
		if err := json.Unmarshal([]byte(page.Body.AtlasDocFormat.Value), &adfDoc); err != nil {
			return "", fmt.Errorf("parsing ADF body: %w", err)
		}
		body := renderADF(&adfDoc)
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("(No content)\n")
	}

	// Render comments section (informational only — stripped on push)
	if len(footerComments) > 0 || len(inlineComments) > 0 {
		b.WriteString("\n")
		b.WriteString(ConfluenceCommentsMarker)
		b.WriteString("\n## Comments\n\n")

		for _, c := range inlineComments {
			date := formatDate(c.Version.CreatedAt)
			status := c.ResolutionStatus
			if status == "" {
				status = "open"
			}
			sel := inlineSelection(c.Properties)

			if status == "open" {
				b.WriteString(fmt.Sprintf("### [UNRESOLVED] %s · inline\n", date))
			} else {
				b.WriteString(fmt.Sprintf("### [resolved] %s · inline\n", date))
			}
			if sel != "" {
				b.WriteString(fmt.Sprintf("on: %q\n", sel))
			}
			body := commentBodyText(c.Body)
			if body != "" {
				b.WriteString(body + "\n")
			}
			b.WriteString("\n")
		}

		for _, c := range footerComments {
			date := formatDate(c.Version.CreatedAt)
			b.WriteString(fmt.Sprintf("### %s · footer\n", date))
			body := commentBodyText(c.Body)
			if body != "" {
				b.WriteString(body + "\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String(), nil
}

// renderADF converts an ADF node tree to markdown.
func renderADF(node *jira.ADFNode) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	renderNode(&b, node, "")
	return b.String()
}

func renderNode(b *strings.Builder, node *jira.ADFNode, listPrefix string) {
	switch node.Type {
	case "doc":
		renderChildren(b, node, "")

	case "paragraph":
		renderInlineChildren(b, node)
		b.WriteString("\n\n")

	case "heading":
		level := 2 // default
		if l, ok := node.Attrs["level"]; ok {
			if lf, ok := l.(float64); ok {
				level = int(lf)
			}
		}
		b.WriteString(strings.Repeat("#", level))
		b.WriteString(" ")
		renderInlineChildren(b, node)
		b.WriteString("\n\n")

	case "bulletList":
		for _, child := range node.Content {
			renderNode(b, &child, "- ")
		}

	case "orderedList":
		for i, child := range node.Content {
			renderNode(b, &child, fmt.Sprintf("%d. ", i+1))
		}

	case "listItem":
		// A list item may contain paragraphs or nested lists.
		for i, child := range node.Content {
			if i == 0 && child.Type == "paragraph" {
				b.WriteString(listPrefix)
				renderInlineChildren(b, &child)
				b.WriteString("\n")
			} else if child.Type == "bulletList" || child.Type == "orderedList" {
				// Indent nested lists
				indented := indentPrefix(listPrefix)
				for j, nested := range child.Content {
					prefix := "- "
					if child.Type == "orderedList" {
						prefix = fmt.Sprintf("%d. ", j+1)
					}
					renderNode(b, &nested, indented+prefix)
				}
			} else {
				renderNode(b, &child, listPrefix)
			}
		}

	case "codeBlock":
		lang := ""
		if l, ok := node.Attrs["language"]; ok {
			if ls, ok := l.(string); ok {
				lang = ls
			}
		}
		b.WriteString("```")
		b.WriteString(lang)
		b.WriteString("\n")
		for _, child := range node.Content {
			b.WriteString(child.Text)
		}
		b.WriteString("\n```\n\n")

	case "blockquote":
		var inner strings.Builder
		renderChildren(&inner, node, "")
		lines := strings.Split(strings.TrimRight(inner.String(), "\n"), "\n")
		for _, line := range lines {
			b.WriteString("> ")
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")

	case "rule":
		b.WriteString("---\n\n")

	case "table":
		renderTable(b, node)

	case "text":
		text := applyMarks(node.Text, node.Marks)
		b.WriteString(text)

	case "hardBreak":
		b.WriteString("\n")

	case "mention":
		name := ""
		if t, ok := node.Attrs["text"]; ok {
			if ts, ok := t.(string); ok {
				name = ts
			}
		}
		b.WriteString("@")
		b.WriteString(name)

	case "inlineCard":
		url := ""
		if u, ok := node.Attrs["url"]; ok {
			if us, ok := u.(string); ok {
				url = us
			}
		}
		b.WriteString(fmt.Sprintf("[link](%s)", url))

	case "emoji":
		text := ""
		if t, ok := node.Attrs["text"]; ok {
			if ts, ok := t.(string); ok {
				text = ts
			}
		}
		if text == "" {
			if sc, ok := node.Attrs["shortName"]; ok {
				if scs, ok := sc.(string); ok {
					text = scs
				}
			}
		}
		b.WriteString(text)

	case "mediaGroup", "mediaSingle", "media", "panel", "expand",
		"nestedExpand", "extension", "bodiedExtension", "inlineExtension",
		"layoutSection", "layoutColumn", "decisionList", "decisionItem",
		"taskList", "taskItem", "status", "date", "placeholder",
		"multiBodiedExtension":
		writePreservedMarker(b, node)

	default:
		// Best effort: try to render children
		renderChildren(b, node, "")
	}
}

func renderChildren(b *strings.Builder, node *jira.ADFNode, listPrefix string) {
	for i := range node.Content {
		renderNode(b, &node.Content[i], listPrefix)
	}
}

func renderInlineChildren(b *strings.Builder, node *jira.ADFNode) {
	for i := range node.Content {
		renderNode(b, &node.Content[i], "")
	}
}

func renderTable(b *strings.Builder, node *jira.ADFNode) {
	if len(node.Content) == 0 {
		return
	}

	rows := make([][]string, 0, len(node.Content))
	isHeaderRow := false

	for _, row := range node.Content {
		if row.Type != "tableRow" {
			continue
		}
		cells := make([]string, 0, len(row.Content))
		for _, cell := range row.Content {
			var cellBuf strings.Builder
			for i := range cell.Content {
				renderInlineChildren(&cellBuf, &cell.Content[i])
			}
			text := strings.TrimSpace(cellBuf.String())
			if cell.Type == "tableHeader" {
				isHeaderRow = true
				// Strip outer bold marks — tableHeader is already semantically bold,
				// and keeping them causes accumulation on round-trips.
				for strings.HasPrefix(text, "**") && strings.HasSuffix(text, "**") && len(text) > 4 {
					text = text[2 : len(text)-2]
				}
			}
			cells = append(cells, text)
		}
		rows = append(rows, cells)
		if isHeaderRow && len(rows) == 1 {
			isHeaderRow = false
		}
	}

	if len(rows) == 0 {
		return
	}

	// Determine column count
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Print first row
	b.WriteString("| ")
	b.WriteString(strings.Join(padRow(rows[0], maxCols), " | "))
	b.WriteString(" |\n")

	// Separator
	sep := make([]string, maxCols)
	for i := range sep {
		sep[i] = "---"
	}
	b.WriteString("| ")
	b.WriteString(strings.Join(sep, " | "))
	b.WriteString(" |\n")

	// Remaining rows
	for _, row := range rows[1:] {
		b.WriteString("| ")
		b.WriteString(strings.Join(padRow(row, maxCols), " | "))
		b.WriteString(" |\n")
	}
	b.WriteString("\n")
}

func padRow(row []string, cols int) []string {
	for len(row) < cols {
		row = append(row, "")
	}
	return row
}

func applyMarks(text string, marks []jira.ADFMark) string {
	for _, mark := range marks {
		switch mark.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "strike":
			text = "~~" + text + "~~"
		case "link":
			href := ""
			if h, ok := mark.Attrs["href"]; ok {
				if hs, ok := h.(string); ok {
					href = hs
				}
			}
			text = fmt.Sprintf("[%s](%s)", text, href)
		case "underline":
			// Markdown doesn't support underline natively; use emphasis
			text = "_" + text + "_"
		case "subsup":
			// Best effort - no standard markdown for sub/sup
		}
	}
	return text
}

func indentPrefix(prefix string) string {
	// Return spaces equal to the length of the prefix for indentation
	return strings.Repeat(" ", len(prefix))
}

// writePreservedMarker emits an opaque marker that preserves the original ADF
// node as base64-encoded JSON. On push, the marker is decoded and the original
// ADF node is restored byte-for-byte. Do not edit the data line.
func writePreservedMarker(b *strings.Builder, node *jira.ADFNode) {
	desc := preservedDescriptions[node.Type]
	if desc == "" {
		desc = node.Type
	}

	jsonBytes, err := json.Marshal(node)
	if err != nil {
		// Fallback: emit a plain comment if we can't serialize
		b.WriteString(fmt.Sprintf("<!-- PRESERVED: %s — Could not serialize for round-trip -->\n", desc))
		return
	}
	encoded := base64.StdEncoding.EncodeToString(jsonBytes)

	b.WriteString(fmt.Sprintf("%s %s — Do not edit this block; it is restored on push to JIRA. -->\n", preserveStart, desc))
	b.WriteString(fmt.Sprintf("%s%s -->\n", preserveData, encoded))
	b.WriteString(preserveEnd)
	b.WriteString("\n")
}

// commentBodyText extracts plain text from a comment's storage-format body.
func commentBodyText(body jira.CommentBody) string {
	if body.Storage != nil && body.Storage.Value != "" {
		return stripHTMLTags(body.Storage.Value)
	}
	return ""
}

// inlineSelection extracts the original selected text from inline comment properties.
func inlineSelection(props map[string]interface{}) string {
	if props == nil {
		return ""
	}
	if v, ok := props["inline-original-selection"]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// stripHTMLTags removes HTML tags and decodes common entities.
func stripHTMLTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	text := re.ReplaceAllString(s, "")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	return strings.TrimSpace(text)
}

func formatDate(isoDate string) string {
	t, err := time.Parse("2006-01-02T15:04:05.000-0700", isoDate)
	if err != nil {
		// Try alternative formats
		t, err = time.Parse("2006-01-02T15:04:05.000Z0700", isoDate)
		if err != nil {
			t, err = time.Parse(time.RFC3339, isoDate)
			if err != nil {
				return isoDate // Return as-is if we can't parse
			}
		}
	}
	return t.Format("2006-01-02")
}
