# jira-cli

A command-line tool for syncing JIRA and Confluence content as markdown files. Pull tickets and pages to markdown, edit locally, push changes back.

Designed for workflows where you want to work with JIRA/Confluence content in your editor or feed it to AI tools, while keeping round-trip fidelity for images, macros, and other rich content.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mreider/jira-cli/main/install.sh | sh
```

This downloads the latest release for your OS/architecture and installs it to `/usr/local/bin`.

Or download a binary manually from [Releases](https://github.com/mreider/jira-cli/releases).

### Build from source

Requires Go 1.21+.

```bash
make build
```

## Setup

```bash
jira config
```

Enter your Atlassian Cloud URL, email, and [API token](https://id.atlassian.com/manage-profile/security/api-tokens). Config is saved to `~/.jira-cli.yaml`.

## Commands

### Pull JIRA ticket

```bash
jira get PRODUCT-12345
jira get PRODUCT-12345 --output-dir ./tickets
```

Outputs markdown with YAML frontmatter (key, title, status, labels, etc.) and the ticket body converted from Atlassian Document Format (ADF) to markdown.

### Push body back to JIRA

```bash
jira push -f ticket.md
jira push -f ticket.md --dry-run
```

Pushes **only the document body** (description field) back to JIRA. Frontmatter metadata (title, status, labels) is read-only and not pushed. Use `--dry-run` to preview the ADF output.

### Push everything to JIRA

```bash
jira apply -f ticket.md
jira apply -f ticket.md --dry-run
```

Pushes body, title, labels, and status transitions. Compares against current JIRA state and shows a diff before applying.

### Pull Confluence page

```bash
jira confluence get 85962893
jira confluence get https://your-org.atlassian.net/wiki/spaces/SPACE/pages/85962893/Page+Title
jira confluence get 85962893 --output-dir ./pages
```

Accepts a page ID or full URL.

### Search JIRA issues

```bash
# Raw JQL
jira search --jql "project = PRODUCT AND status = 'In Progress'"

# Smart filters (builds JQL automatically)
jira search --project PRODUCT --status "In Progress"
jira search --project PRODUCT --updated recent
jira search --project PRODUCT --type Bug --assignee me
jira search -p PRODUCT -l backend -l q1 --updated "last week"

# Full text search
jira search -q "login error"
jira search --project PRODUCT -q "deployment"

# Pull all results to markdown files
jira search --project PRODUCT --updated recent --output-dir ./tickets
```

Flags: `--project`/`-p`, `--status`/`-s`, `--assignee`/`-a`, `--reporter`, `--type`/`-t`, `--label`/`-l`, `--updated`, `--created`, `--text`/`-q`, `--max-results`, `--order-by`, `--output-dir`, `--jql`.

### Search Confluence pages

```bash
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
```

Flags: `--space`/`-s`, `--label`/`-l`, `--type`, `--updated`, `--created`, `--contributor`, `--max-results`, `--output-dir`.

### Smart date filters

The `--updated` and `--created` flags accept natural language date expressions:

| Expression | Meaning |
|---|---|
| `today` | Since start of today |
| `yesterday` | Since yesterday |
| `recent` | Last 7 days |
| `last week` | Last 7 days |
| `this week` | Since start of this week |
| `last month` | Last 30 days |
| `this month` | Since start of this month |
| `this quarter` | Last 90 days |
| `this year` | Since start of this year |
| `2025-01-15` | Since specific ISO date |

### Push body back to Confluence

```bash
jira confluence push -f page.md
jira confluence push -f page.md --dry-run
```

Pushes **only the page body** back. Title and metadata are not changed. The page version is auto-incremented.

## Frontmatter

Pulled files include YAML frontmatter with metadata from the source system. This metadata is read-only context — it is **not pushed back** on `push` commands (only `apply` pushes metadata for JIRA).

JIRA example:
```yaml
---
# READ-ONLY metadata pulled from JIRA. Changes here are NOT pushed back.
# Only the document body (below the frontmatter) is synced on push.
key: PRODUCT-12345
title: My Feature
status: In Progress
statusCategory: In Progress
type: Story
labels: [backend, q1]
url: https://your-org.atlassian.net/browse/PRODUCT-12345
synced: 2026-02-10T14:00:00Z
---
```

Confluence example:
```yaml
---
# READ-ONLY metadata pulled from Confluence. Changes here are NOT pushed back.
# Only the document body (below the frontmatter) is synced on push.
source: confluence
pageId: 85962893
title: My Page
spaceKey: ENG
spaceName: Engineering
version: 5
url: https://your-org.atlassian.net/wiki/spaces/ENG/pages/85962893/My+Page
synced: 2026-02-10T14:00:00Z
---
```

## Round-trip fidelity

Content that can't be represented in markdown (images, macros, panels, expand sections, etc.) is preserved as opaque markers:

```
<!-- PRESERVED: Inline image — Do not edit this block; it is restored on push to JIRA. -->
<!-- data:eyJ0eXBlIjoibWVkaWFTaW5nbGUi... -->
<!-- /PRESERVED -->
```

These markers contain the original ADF node encoded as base64 JSON. On push, they are decoded and restored byte-for-byte. Don't edit the data lines if you want to keep the original content intact.

Supported content types that round-trip through markdown: headings, paragraphs, bold/italic/code/strikethrough, links, bullet and ordered lists (including nested), code blocks, blockquotes, tables, horizontal rules.

## Requirements

- Atlassian Cloud (not Server/Data Center)
- [API token](https://id.atlassian.com/manage-profile/security/api-tokens) with read/write access

## License

MIT
