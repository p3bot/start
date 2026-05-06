package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/assets"
	"github.com/start-cli/start/internal/registry"
)

func TestPrintSearchSections(t *testing.T) {
	t.Parallel()

	results := []assets.SearchResult{
		{Category: "roles", Name: "golang", Entry: registry.IndexEntry{Description: "Go programming expert"}, MatchScore: 5},
		{Category: "tasks", Name: "pre-commit-review", Entry: registry.IndexEntry{Description: "Review staged changes"}, MatchScore: 3},
	}

	t.Run("single section output", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "local", Path: "./.start", Results: results},
		}
		printSearchSections(&buf, sections, false, nil)
		out := buf.String()

		if !strings.Contains(out, "local") {
			t.Error("missing section label")
		}
		if !strings.Contains(out, "./.start") {
			t.Error("missing section path")
		}
		if !strings.Contains(out, "roles/") {
			t.Error("missing roles category")
		}
		if !strings.Contains(out, "tasks/") {
			t.Error("missing tasks category")
		}
		if !strings.Contains(out, "golang") {
			t.Error("missing golang result")
		}
		if !strings.Contains(out, "pre-commit-review") {
			t.Error("missing pre-commit-review result")
		}
	})

	t.Run("multiple sections with blank line separator", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "local", Path: "./.start", Results: results},
			{Label: "registry", Results: results, ShowInstalled: true},
		}
		printSearchSections(&buf, sections, false, nil)
		out := buf.String()

		if !strings.Contains(out, "local") {
			t.Error("missing local section label")
		}
		if !strings.Contains(out, "registry") {
			t.Error("missing registry section label")
		}

		// Sections should be separated by a blank line
		localIdx := strings.Index(out, "local")
		registryIdx := strings.Index(out, "registry")
		between := out[localIdx:registryIdx]
		if !strings.Contains(between, "\n\n") {
			t.Error("sections should be separated by blank line")
		}
	})

	t.Run("empty sections omitted", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "local", Path: "./.start", Results: nil},
			{Label: "registry", Results: results},
		}
		printSearchSections(&buf, sections, false, nil)
		out := buf.String()

		if strings.Contains(out, "./.start") {
			t.Error("empty local section should be omitted")
		}
		if !strings.Contains(out, "registry") {
			t.Error("missing registry section")
		}
	})

	t.Run("installed markers only in registry section", func(t *testing.T) {
		t.Parallel()
		installed := map[string]bool{
			"roles/golang": true,
		}
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "local", Path: "./.start", Results: results},
			{Label: "registry", Results: results, ShowInstalled: true},
		}
		printSearchSections(&buf, sections, false, installed)
		out := buf.String()

		// Split output into local and registry parts
		registryIdx := strings.Index(out, "registry")
		localPart := out[:registryIdx]
		registryPart := out[registryIdx:]

		if strings.Contains(localPart, "★") {
			t.Error("local section should not have installed markers")
		}
		if !strings.Contains(registryPart, "★") {
			t.Error("registry section should have installed marker for golang")
		}
	})

	t.Run("verbose shows tags and module paths", func(t *testing.T) {
		t.Parallel()
		verboseResults := []assets.SearchResult{
			{
				Category:   "roles",
				Name:       "golang",
				Entry:      registry.IndexEntry{Description: "Go expert", Module: "github.com/test/roles/golang@v0", Tags: []string{"go", "programming"}},
				MatchScore: 5,
			},
		}
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "registry", Results: verboseResults},
		}
		printSearchSections(&buf, sections, true, nil)
		out := buf.String()

		if !strings.Contains(out, "Module:") {
			t.Error("verbose mode should show module path")
		}
		if !strings.Contains(out, "github.com/test/roles/golang@v0") {
			t.Error("verbose mode should show actual module path")
		}
		if !strings.Contains(out, "Tags:") {
			t.Error("verbose mode should show tags")
		}
		if !strings.Contains(out, "go, programming") {
			t.Error("verbose mode should show actual tags")
		}
	})

	t.Run("non-verbose hides tags and module paths", func(t *testing.T) {
		t.Parallel()
		verboseResults := []assets.SearchResult{
			{
				Category:   "roles",
				Name:       "golang",
				Entry:      registry.IndexEntry{Description: "Go expert", Module: "github.com/test/roles/golang@v0", Tags: []string{"go"}},
				MatchScore: 5,
			},
		}
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "registry", Results: verboseResults},
		}
		printSearchSections(&buf, sections, false, nil)
		out := buf.String()

		if strings.Contains(out, "Module:") {
			t.Error("non-verbose should not show module path")
		}
		if strings.Contains(out, "Tags:") {
			t.Error("non-verbose should not show tags")
		}
	})

	t.Run("category order is agents roles contexts tasks", func(t *testing.T) {
		t.Parallel()
		allCatResults := []assets.SearchResult{
			{Category: "contexts", Name: "env", Entry: registry.IndexEntry{Description: "Environment"}, MatchScore: 3},
			{Category: "agents", Name: "claude", Entry: registry.IndexEntry{Description: "Claude AI"}, MatchScore: 3},
			{Category: "tasks", Name: "review", Entry: registry.IndexEntry{Description: "Code review"}, MatchScore: 3},
			{Category: "roles", Name: "golang", Entry: registry.IndexEntry{Description: "Go expert"}, MatchScore: 3},
		}
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "test", Results: allCatResults},
		}
		printSearchSections(&buf, sections, false, nil)
		out := buf.String()

		agentsIdx := strings.Index(out, "agents/")
		rolesIdx := strings.Index(out, "roles/")
		tasksIdx := strings.Index(out, "tasks/")
		contextsIdx := strings.Index(out, "contexts/")

		if agentsIdx > rolesIdx || rolesIdx > contextsIdx || contextsIdx > tasksIdx {
			t.Errorf("categories in wrong order: agents=%d roles=%d contexts=%d tasks=%d",
				agentsIdx, rolesIdx, contextsIdx, tasksIdx)
		}
	})

	t.Run("items indented under category", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		sections := []searchSection{
			{Label: "test", Results: []assets.SearchResult{
				{Category: "roles", Name: "golang", Entry: registry.IndexEntry{Description: "Go expert"}, MatchScore: 3},
			}},
		}
		printSearchSections(&buf, sections, false, nil)

		lines := strings.Split(buf.String(), "\n")
		for _, line := range lines {
			if strings.Contains(line, "golang") {
				if !strings.HasPrefix(line, "    ") {
					t.Errorf("item should be indented with 4 spaces, got: %q", line)
				}
			}
			if strings.Contains(line, "roles/") {
				if !strings.HasPrefix(line, "  ") {
					t.Errorf("category should be indented with 2 spaces, got: %q", line)
				}
			}
		}
	})
}

func TestSearchCommandJSON_ShortQueryError(t *testing.T) {
	// With --json, a short query returns error immediately instead of prompting
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"search", "go", "--json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for short query with --json")
	}
	if !strings.Contains(err.Error(), "3 characters") {
		t.Errorf("error should mention 3 characters, got: %v", err)
	}
	// Should not have produced any JSON output
	if stdout.Len() > 0 {
		t.Errorf("expected no output on error, got: %s", stdout.String())
	}
}

func TestSearchCommandJSON_WithConfigResults(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `roles: {
	"golang": {
		description: "Go programming expert"
		prompt: "You are a Go expert."
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "config.cue"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"search", "golang", "--json"})

	// May fail due to registry unavailability but should still output JSON
	_ = cmd.Execute()

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		t.Fatal("expected JSON output even if registry unavailable")
	}

	var sections []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &sections); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	// Should find the global config result at minimum
	if len(sections) == 0 {
		t.Error("expected at least one section (global config) in JSON output")
	}

	// Verify section structure
	for _, section := range sections {
		if _, ok := section["label"]; !ok {
			t.Error("section missing 'label' field")
		}
		if _, ok := section["results"]; !ok {
			t.Error("section missing 'results' field")
		}
	}
}

func TestSearchCommandJSON_NoResults(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"search", "nonexistentthing", "--json"})

	_ = cmd.Execute()

	output := strings.TrimSpace(stdout.String())
	if output != "[]" {
		t.Errorf("expected empty JSON array, got: %s", output)
	}
}

func TestSearchCommandValidation(t *testing.T) {
	t.Parallel()

	t.Run("query under 3 characters returns error", func(t *testing.T) {
		t.Parallel()
		cmd := NewRootCmd()
		cmd.SetArgs([]string{"search", "go"})
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected error for short query")
		}
		if !strings.Contains(err.Error(), "3 characters") {
			t.Errorf("error should mention 3 characters, got: %s", err.Error())
		}
	})

	t.Run("find alias is registered", func(t *testing.T) {
		t.Parallel()
		cmd := NewRootCmd()
		// Walk the command tree to find the search command
		for _, sub := range cmd.Commands() {
			if sub.Name() == "search" {
				found := false
				for _, alias := range sub.Aliases {
					if alias == "find" {
						found = true
					}
				}
				if !found {
					t.Error("search command should have 'find' alias")
				}
				return
			}
		}
		t.Error("search command not found")
	})
}

func TestShortenHome(t *testing.T) {
	t.Parallel()

	t.Run("shortens home directory", func(t *testing.T) {
		t.Parallel()
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot determine home directory")
		}
		result := shortenHome(home + "/.config/start")
		if result != "~/.config/start" {
			t.Errorf("shortenHome(%q) = %q, want %q", home+"/.config/start", result, "~/.config/start")
		}
	})

	t.Run("returns non-home path unchanged", func(t *testing.T) {
		t.Parallel()
		result := shortenHome("/tmp/some/path")
		if result != "/tmp/some/path" {
			t.Errorf("shortenHome(/tmp/some/path) = %q, want /tmp/some/path", result)
		}
	})
}
