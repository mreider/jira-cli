package markdown

// Ticket is the intermediate representation between JIRA and markdown.
type Ticket struct {
	Key      string
	Title    string
	Status   string
	Type     string
	Priority string
	Labels   []string
	Assignee string
	Reporter string
	URL      string
	Synced   string
	Body     string // markdown description
	Comments []TicketComment
}

// TicketComment represents a single comment in the intermediate format.
type TicketComment struct {
	Author string
	Date   string
	Body   string
}

// ConfluenceDoc is the intermediate representation between Confluence and markdown.
type ConfluenceDoc struct {
	PageID    string
	Title     string
	Status    string
	SpaceKey  string
	SpaceName string
	Version   int
	URL       string
	Synced    string
	Body      string // markdown body (without frontmatter and title heading)
}
