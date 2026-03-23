package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_AllPresent(t *testing.T) {
	cfg := Config{URL: "https://example.atlassian.net", Email: "a@b.com", Token: "tok"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingURL(t *testing.T) {
	cfg := Config{Email: "a@b.com", Token: "tok"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing URL")
	}
}

func TestValidate_MissingEmail(t *testing.T) {
	cfg := Config{URL: "https://example.atlassian.net", Token: "tok"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing email")
	}
}

func TestValidate_MissingToken(t *testing.T) {
	cfg := Config{URL: "https://example.atlassian.net", Email: "a@b.com"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing token")
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Clear env vars so they don't override file values
	t.Setenv("JIRA_URL", "")
	t.Setenv("JIRA_EMAIL", "")
	t.Setenv("JIRA_TOKEN", "")

	dir := t.TempDir()
	path := filepath.Join(dir, "test-config.yaml")

	original := Config{
		URL:   "https://example.atlassian.net",
		Email: "user@example.com",
		Token: "secret-token",
	}

	if err := Save(original, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected permissions 0600, got %o", perm)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.URL != original.URL {
		t.Errorf("URL: expected %s, got %s", original.URL, loaded.URL)
	}
	if loaded.Email != original.Email {
		t.Errorf("Email: expected %s, got %s", original.Email, loaded.Email)
	}
	if loaded.Token != original.Token {
		t.Errorf("Token: expected %s, got %s", original.Token, loaded.Token)
	}
}

func TestLoad_EnvVarOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-config.yaml")

	Save(Config{URL: "https://file.atlassian.net", Email: "file@example.com", Token: "file-token"}, path)

	t.Setenv("JIRA_URL", "https://env.atlassian.net")
	t.Setenv("JIRA_EMAIL", "env@example.com")
	t.Setenv("JIRA_TOKEN", "env-token")

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.URL != "https://env.atlassian.net" {
		t.Errorf("expected env URL override, got %s", loaded.URL)
	}
	if loaded.Email != "env@example.com" {
		t.Errorf("expected env Email override, got %s", loaded.Email)
	}
	if loaded.Token != "env-token" {
		t.Errorf("expected env Token override, got %s", loaded.Token)
	}
}

func TestLoad_NoFile_EnvOnly(t *testing.T) {
	t.Setenv("JIRA_URL", "https://envonly.atlassian.net")
	t.Setenv("JIRA_EMAIL", "envonly@example.com")
	t.Setenv("JIRA_TOKEN", "envonly-token")

	loaded, err := Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.URL != "https://envonly.atlassian.net" {
		t.Errorf("expected env URL, got %s", loaded.URL)
	}
}
