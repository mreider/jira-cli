package jira

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dt-pm-tools/jira-cli/internal/config"
)

// Client is a JIRA REST API v3 client.
type Client struct {
	baseURL    string
	authHeader string
	httpClient *http.Client
}

// NewClient creates a new JIRA client from the given config.
func NewClient(cfg config.Config) *Client {
	creds := base64.StdEncoding.EncodeToString([]byte(cfg.Email + ":" + cfg.Token))
	baseURL := strings.TrimRight(cfg.URL, "/")
	return &Client{
		baseURL:    baseURL,
		authHeader: "Basic " + creds,
		httpClient: &http.Client{},
	}
}

// GetIssue fetches a single issue by key.
func (c *Client) GetIssue(key string) (*Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=summary,status,issuetype,priority,labels,assignee,reporter,description,comment", c.baseURL, key)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JIRA API returned %d: %s", resp.StatusCode, string(body))
	}

	var issue Issue
	if err := json.NewDecoder(resp.Body).Decode(&issue); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &issue, nil
}

// UpdateIssue updates an issue's fields.
func (c *Client) UpdateIssue(key string, payload UpdatePayload) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", c.baseURL, key)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("JIRA API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetTransitions returns available transitions for an issue.
func (c *Client) GetTransitions(key string) ([]TransitionInfo, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", c.baseURL, key)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JIRA API returned %d: %s", resp.StatusCode, string(body))
	}

	var result TransitionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.Transitions, nil
}

// DoTransition performs a status transition on an issue.
func (c *Client) DoTransition(key string, transitionID string) error {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", c.baseURL, key)

	payload := TransitionPayload{
		Transition: Transition{ID: transitionID},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("JIRA API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetConfluencePage fetches a Confluence page by ID with ADF body.
func (c *Client) GetConfluencePage(pageID string) (*ConfluencePage, error) {
	url := fmt.Sprintf("%s/wiki/api/v2/pages/%s?body-format=atlas_doc_format", c.baseURL, pageID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Confluence API returned %d: %s", resp.StatusCode, string(body))
	}

	var page ConfluencePage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &page, nil
}

// GetConfluenceSpace fetches a Confluence space by ID.
func (c *Client) GetConfluenceSpace(spaceID string) (*ConfluenceSpace, error) {
	url := fmt.Sprintf("%s/wiki/api/v2/spaces/%s", c.baseURL, spaceID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Confluence API returned %d: %s", resp.StatusCode, string(body))
	}

	var space ConfluenceSpace
	if err := json.NewDecoder(resp.Body).Decode(&space); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &space, nil
}

// ConfluenceUpdatePayload is the body for PUT /wiki/api/v2/pages/{id}.
type ConfluenceUpdatePayload struct {
	ID      string                      `json:"id"`
	Status  string                      `json:"status"`
	Title   string                      `json:"title"`
	Body    ConfluenceUpdateBody        `json:"body"`
	Version ConfluenceUpdateVersion     `json:"version"`
}

// ConfluenceUpdateBody wraps the ADF value for Confluence page updates.
type ConfluenceUpdateBody struct {
	Representation string `json:"representation"`
	Value          string `json:"value"` // JSON-encoded ADF string
}

// ConfluenceUpdateVersion specifies the new version number.
type ConfluenceUpdateVersion struct {
	Number  int    `json:"number"`
	Message string `json:"message,omitempty"`
}

// UpdateConfluencePage updates a Confluence page body (ADF format).
func (c *Client) UpdateConfluencePage(pageID string, payload ConfluenceUpdatePayload) error {
	url := fmt.Sprintf("%s/wiki/api/v2/pages/%s", c.baseURL, pageID)

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling payload: %w", err)
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Confluence API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
}
