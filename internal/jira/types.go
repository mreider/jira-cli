package jira

// Issue represents a JIRA issue from the REST API v3.
type Issue struct {
	Key    string `json:"key"`
	Fields Fields `json:"fields"`
}

// Fields contains the issue fields we care about.
type Fields struct {
	Summary     string    `json:"summary"`
	Status      Status    `json:"status"`
	IssueType   IssueType `json:"issuetype"`
	Priority    Priority  `json:"priority,omitempty"`
	Labels      []string  `json:"labels,omitempty"`
	Assignee    *User     `json:"assignee,omitempty"`
	Reporter    *User     `json:"reporter,omitempty"`
	Description *ADFNode  `json:"description,omitempty"`
	Comment     *Comments `json:"comment,omitempty"`
	Updated     string    `json:"updated,omitempty"`
}

// Status represents a JIRA status.
type Status struct {
	Name           string          `json:"name"`
	StatusCategory *StatusCategory `json:"statusCategory,omitempty"`
}

// StatusCategory represents the high-level category of a JIRA status.
type StatusCategory struct {
	Key  string `json:"key"`  // "new", "indeterminate", "done"
	Name string `json:"name"` // "To Do", "In Progress", "Done"
}

// IssueType represents a JIRA issue type.
type IssueType struct {
	Name string `json:"name"`
}

// Priority represents a JIRA priority.
type Priority struct {
	Name string `json:"name"`
}

// User represents a JIRA user.
type User struct {
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

// Comments wraps the comments array from the JIRA API.
type Comments struct {
	Comments []Comment `json:"comments"`
}

// Comment represents a single JIRA comment.
type Comment struct {
	Author  User     `json:"author"`
	Body    *ADFNode `json:"body"`
	Created string   `json:"created"`
}

// ADFNode represents a node in the Atlassian Document Format.
type ADFNode struct {
	Type    string         `json:"type"`
	Content []ADFNode      `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Marks   []ADFMark      `json:"marks,omitempty"`
}

// ADFMark represents an inline formatting mark in ADF.
type ADFMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// ConfluencePage represents a Confluence page from the REST API v2.
type ConfluencePage struct {
	ID      string         `json:"id"`
	Title   string         `json:"title"`
	Status  string         `json:"status"`
	SpaceID string         `json:"spaceId"`
	Version PageVersion    `json:"version"`
	Body    PageBody       `json:"body"`
	Links   PageLinks      `json:"_links"`
}

// PageVersion contains version info for a Confluence page.
type PageVersion struct {
	Number    int    `json:"number"`
	CreatedAt string `json:"createdAt"`
	AuthorID  string `json:"authorId"`
}

// PageBody contains the page body in ADF format.
type PageBody struct {
	AtlasDocFormat *PageBodyFormat `json:"atlas_doc_format,omitempty"`
}

// PageBodyFormat wraps the ADF value string.
type PageBodyFormat struct {
	Value          string `json:"value"`
	Representation string `json:"representation"`
}

// PageLinks contains the _links object from the Confluence API.
type PageLinks struct {
	WebUI string `json:"webui"`
	Base  string `json:"base"`
}

// ConfluenceSpace represents a Confluence space (minimal fields).
type ConfluenceSpace struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ConfluenceCreatePayload is the body for POST /wiki/api/v2/pages.
type ConfluenceCreatePayload struct {
	SpaceID  string               `json:"spaceId"`
	Status   string               `json:"status"`
	Title    string               `json:"title"`
	ParentID string               `json:"parentId,omitempty"`
	Body     ConfluenceUpdateBody `json:"body"`
}

// ConfluenceSpacesResponse wraps the results array from GET /wiki/api/v2/spaces.
type ConfluenceSpacesResponse struct {
	Results []ConfluenceSpace `json:"results"`
}

// UpdatePayload is the body for PUT /rest/api/3/issue/{key}.
type UpdatePayload struct {
	Fields     UpdateFields `json:"fields"`
	Transition *Transition  `json:"transition,omitempty"`
}

// UpdateFields contains the fields that can be updated.
type UpdateFields struct {
	Summary     string   `json:"summary,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	Description *ADFNode `json:"description,omitempty"`
}

// Transition is used to change issue status.
type Transition struct {
	ID string `json:"id"`
}

// TransitionPayload is the body for POST /rest/api/3/issue/{key}/transitions.
type TransitionPayload struct {
	Transition Transition `json:"transition"`
}

// TransitionsResponse is the response from GET transitions.
type TransitionsResponse struct {
	Transitions []TransitionInfo `json:"transitions"`
}

// TransitionInfo describes an available transition.
type TransitionInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   Status `json:"to"`
}
