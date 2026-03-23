package jira

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/mreider/a-cli/internal/config"
)

func testClient(baseURL string) *Client {
	return NewClient(config.Config{
		URL:   baseURL,
		Email: "test@example.com",
		Token: "test-token",
	})
}

// --- Unit tests (httptest mock server) ---

func TestSearchIssues_Endpoint(t *testing.T) {
	var gotPath, gotMethod string
	var gotPayload SearchPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &gotPayload)

		json.NewEncoder(w).Encode(SearchResult{
			Issues: []Issue{{Key: "TEST-1", Fields: Fields{Summary: "test issue"}}},
			Total:  1,
		})
	}))
	defer srv.Close()

	client := testClient(srv.URL)
	result, err := client.SearchIssues("project = TEST", 25, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/rest/api/3/search/jql" {
		t.Errorf("expected path /rest/api/3/search/jql, got %s", gotPath)
	}
	if gotMethod != "POST" {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotPayload.JQL != "project = TEST" {
		t.Errorf("expected JQL 'project = TEST', got %q", gotPayload.JQL)
	}
	if gotPayload.MaxResults != 25 {
		t.Errorf("expected maxResults 25, got %d", gotPayload.MaxResults)
	}
	if len(result.Issues) != 1 || result.Issues[0].Key != "TEST-1" {
		t.Errorf("unexpected result: %+v", result)
	}
}

func TestGetIssue_Endpoint(t *testing.T) {
	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewEncoder(w).Encode(Issue{Key: "PROJ-123", Fields: Fields{Summary: "Test"}})
	}))
	defer srv.Close()

	client := testClient(srv.URL)
	issue, err := client.GetIssue("PROJ-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/rest/api/3/issue/PROJ-123" {
		t.Errorf("expected path /rest/api/3/issue/PROJ-123, got %s", gotPath)
	}
	if gotMethod != "GET" {
		t.Errorf("expected GET, got %s", gotMethod)
	}
	if issue.Key != "PROJ-123" {
		t.Errorf("expected key PROJ-123, got %s", issue.Key)
	}
}

func TestGetConfluencePage_Endpoint(t *testing.T) {
	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		json.NewEncoder(w).Encode(ConfluencePage{ID: "12345", Title: "Test Page"})
	}))
	defer srv.Close()

	client := testClient(srv.URL)
	page, err := client.GetConfluencePage("12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/wiki/api/v2/pages/12345" {
		t.Errorf("expected path /wiki/api/v2/pages/12345, got %s", gotPath)
	}
	if gotMethod != "GET" {
		t.Errorf("expected GET, got %s", gotMethod)
	}
	if page.Title != "Test Page" {
		t.Errorf("expected title 'Test Page', got %s", page.Title)
	}
}

func TestSearchConfluence_Endpoint(t *testing.T) {
	var gotPath, gotMethod, gotCQL string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotCQL = r.URL.Query().Get("cql")
		json.NewEncoder(w).Encode(ConfluenceSearchResult{Size: 1})
	}))
	defer srv.Close()

	client := testClient(srv.URL)
	_, err := client.SearchConfluence("type = page", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/wiki/rest/api/content/search" {
		t.Errorf("expected path /wiki/rest/api/content/search, got %s", gotPath)
	}
	if gotMethod != "GET" {
		t.Errorf("expected GET, got %s", gotMethod)
	}
	if gotCQL != "type = page" {
		t.Errorf("expected CQL 'type = page', got %q", gotCQL)
	}
}

func TestSetHeaders(t *testing.T) {
	var gotAuth, gotContentType, gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		json.NewEncoder(w).Encode(Issue{Key: "X-1"})
	}))
	defer srv.Close()

	client := testClient(srv.URL)
	client.GetIssue("X-1")

	if gotAuth == "" {
		t.Error("expected Authorization header to be set")
	}
	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", gotContentType)
	}
	if gotAccept != "application/json" {
		t.Errorf("expected Accept application/json, got %s", gotAccept)
	}
}

func TestAPIError_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(JiraErrors{
			ErrorMessages: []string{"Issue does not exist"},
		})
	}))
	defer srv.Close()

	client := testClient(srv.URL)
	_, err := client.GetIssue("NOPE-999")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// --- Integration tests (require env vars, run in CI with secrets) ---

func integrationClient(t *testing.T) *Client {
	t.Helper()
	url := os.Getenv("JIRA_URL")
	email := os.Getenv("JIRA_EMAIL")
	token := os.Getenv("JIRA_TOKEN")
	if url == "" || email == "" || token == "" {
		t.Skip("skipping integration test: JIRA_URL, JIRA_EMAIL, JIRA_TOKEN not set")
	}
	return NewClient(config.Config{URL: url, Email: email, Token: token})
}

func TestIntegration_GetIssue(t *testing.T) {
	client := integrationClient(t)
	issueKey := os.Getenv("TEST_JIRA_ISSUE_KEY")
	if issueKey == "" {
		t.Skip("skipping: TEST_JIRA_ISSUE_KEY not set")
	}

	issue, err := client.GetIssue(issueKey)
	if err != nil {
		t.Fatalf("GetIssue(%s) failed: %v", issueKey, err)
	}
	if issue.Key != issueKey {
		t.Errorf("expected key %s, got %s", issueKey, issue.Key)
	}
	if issue.Fields.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestIntegration_SearchIssues(t *testing.T) {
	client := integrationClient(t)
	issueKey := os.Getenv("TEST_JIRA_ISSUE_KEY")
	if issueKey == "" {
		t.Skip("skipping: TEST_JIRA_ISSUE_KEY not set")
	}

	result, err := client.SearchIssues("key = "+issueKey, 5, 0)
	if err != nil {
		t.Fatalf("SearchIssues failed: %v", err)
	}
	if result.Total < 1 {
		t.Errorf("expected at least 1 result, got %d", result.Total)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Key == issueKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected to find %s in search results", issueKey)
	}
}

func TestIntegration_GetConfluencePage(t *testing.T) {
	client := integrationClient(t)
	pageID := os.Getenv("TEST_CONFLUENCE_PAGE_ID")
	if pageID == "" {
		t.Skip("skipping: TEST_CONFLUENCE_PAGE_ID not set")
	}

	page, err := client.GetConfluencePage(pageID)
	if err != nil {
		t.Fatalf("GetConfluencePage(%s) failed: %v", pageID, err)
	}
	if page.ID != pageID {
		t.Errorf("expected page ID %s, got %s", pageID, page.ID)
	}
	if page.Title == "" {
		t.Error("expected non-empty title")
	}
}
