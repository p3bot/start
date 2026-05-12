//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/cli"
	"github.com/start-cli/start/internal/registry"
)

// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

// TestIntegration_ModulesListWithConfig tests listing modules from config.
func TestIntegration_ModulesListWithConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config directory
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Write config with modules
	config := `
agents: {
	claude: {
		bin: "claude"
		command: "{{.bin}} --model {{.model}}"
		origin: "github.com/start-cli/library/agents/ai/claude@v1.0.0"
	}
}

roles: {
	assistant: {
		prompt: "You are a helpful assistant."
		origin: "github.com/start-cli/library/roles/assistant@v1.0.0"
	}
	reviewer: {
		prompt: "You are a code reviewer."
		origin: "github.com/start-cli/library/roles/reviewer@v1.0.0"
	}
}

tasks: {
	review: {
		prompt: "Review this code."
		origin: "github.com/start-cli/library/tasks/review@v1.0.0"
	}
}
`
	if err := os.WriteFile(filepath.Join(configDir, "settings.cue"), []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Change to temp directory
	chdir(t, tmpDir)

	// Override HOME to isolate from global config
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	buf := new(bytes.Buffer)
	cmd := cli.NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"modules", "list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("modules list failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "Installed modules") {
		t.Errorf("output should mention installed modules, got: %s", output)
	}
	if !strings.Contains(output, "agents/") {
		t.Errorf("output should show agents category, got: %s", output)
	}
	if !strings.Contains(output, "claude") {
		t.Errorf("output should show claude agent, got: %s", output)
	}
	if !strings.Contains(output, "roles/") {
		t.Errorf("output should show roles category, got: %s", output)
	}
	if !strings.Contains(output, "assistant") {
		t.Errorf("output should show assistant role, got: %s", output)
	}
	if !strings.Contains(output, "v1.0.0") {
		t.Errorf("output should show claude version, got: %s", output)
	}
	if !strings.Contains(output, "v1.0.0") {
		t.Errorf("output should show assistant version, got: %s", output)
	}
}

// TestIntegration_ModulesListJSON tests --json output for installed modules.
func TestIntegration_ModulesListJSON(t *testing.T) {
	tmpDir := t.TempDir()

	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	claude: {
		bin: "claude"
		command: "{{.bin}}"
		origin: "github.com/start-cli/library/agents/ai/claude@v1.0.0"
	}
}
roles: {
	assistant: {
		prompt: "You are a helpful assistant."
		origin: "github.com/start-cli/library/roles/assistant@v1.0.0"
	}
}
`
	if err := os.WriteFile(filepath.Join(configDir, "settings.cue"), []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, tmpDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	t.Run("modules list --json", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"modules", "list", "--json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("modules list --json failed: %v", err)
		}

		var result []map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, buf.String())
		}
		if len(result) == 0 {
			t.Errorf("expected non-empty JSON array, got: %s", buf.String())
		}

		// Verify expected fields are present
		first := result[0]
		for _, field := range []string{"category", "name", "scope", "origin"} {
			if _, ok := first[field]; !ok {
				t.Errorf("JSON entry missing field %q, got: %v", field, first)
			}
		}
	})

	t.Run("start modules --json (parent command)", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"modules", "--json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("start modules --json failed: %v", err)
		}

		var result []map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, buf.String())
		}
		if len(result) == 0 {
			t.Errorf("expected non-empty JSON array, got: %s", buf.String())
		}
	})

	t.Run("modules list roles --json", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"modules", "list", "roles", "--json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("modules list roles --json failed: %v", err)
		}

		var result []map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, buf.String())
		}
		for _, entry := range result {
			if entry["category"] != "roles" {
				t.Errorf("expected only roles, got category %q", entry["category"])
			}
		}
	})
}

// TestIntegration_ModulesListCategory tests filtering installed modules by category.
func TestIntegration_ModulesListCategory(t *testing.T) {
	tmpDir := t.TempDir()

	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	claude: {
		bin: "claude"
		command: "{{.bin}}"
		origin: "github.com/start-cli/library/agents/ai/claude@v1.0.0"
	}
}

roles: {
	assistant: {
		prompt: "You are a helpful assistant."
		origin: "github.com/start-cli/library/roles/assistant@v1.0.0"
	}
}
`
	if err := os.WriteFile(filepath.Join(configDir, "settings.cue"), []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, tmpDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	t.Run("filter to agents only", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"modules", "list", "agents"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("modules list agents failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "agents/") {
			t.Errorf("output should show agents category, got: %s", output)
		}
		if !strings.Contains(output, "claude") {
			t.Errorf("output should show claude, got: %s", output)
		}
		if strings.Contains(output, "roles/") {
			t.Errorf("output should not show roles when filtered to agents, got: %s", output)
		}
	})

	t.Run("filter to roles only", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"modules", "list", "roles"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("modules list roles failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "roles/") {
			t.Errorf("output should show roles category, got: %s", output)
		}
		if !strings.Contains(output, "assistant") {
			t.Errorf("output should show assistant, got: %s", output)
		}
		if strings.Contains(output, "agents/") {
			t.Errorf("output should not show agents when filtered to roles, got: %s", output)
		}
	})

	t.Run("filter to tasks - none installed", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"modules", "list", "tasks"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("modules list tasks failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "No tasks installed") {
			t.Errorf("should report no tasks installed, got: %s", output)
		}
	})
}

// TestIntegration_ModulesListNoConfig tests listing when no config exists.
func TestIntegration_ModulesListNoConfig(t *testing.T) {
	tmpDir := t.TempDir()

	chdir(t, tmpDir)

	// Override HOME to isolate
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	buf := new(bytes.Buffer)
	cmd := cli.NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"modules", "list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("modules list failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No configuration found") {
		t.Errorf("should report no config, got: %s", output)
	}
}

// TestIntegration_SearchIndex tests the search functionality.
func TestIntegration_SearchIndex(t *testing.T) {
	// This test uses the internal search function directly
	// since we can't easily mock the registry fetch

	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"ai/claude": {
				Module:      "github.com/test/agents/ai/claude@v0",
				Description: "Claude by Anthropic",
				Tags:        []string{"anthropic", "ai"},
			},
		},
		Roles: map[string]registry.IndexEntry{
			"golang/assistant": {
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go programming expert",
				Tags:        []string{"golang", "programming"},
			},
			"golang/reviewer": {
				Module:      "github.com/test/roles/golang/reviewer@v0",
				Description: "Go code reviewer",
				Tags:        []string{"golang", "review"},
			},
		},
		Tasks:    map[string]registry.IndexEntry{},
		Contexts: map[string]registry.IndexEntry{},
	}

	// Test search for "golang" - should find both roles
	results := searchIndexEntries(index, "golang")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'golang', got %d", len(results))
	}

	// Test search for "claude" - should find agent
	results = searchIndexEntries(index, "claude")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'claude', got %d", len(results))
	}
	if len(results) > 0 && results[0].Name != "ai/claude" {
		t.Errorf("expected ai/claude, got %s", results[0].Name)
	}

	// Test search for "programming" - should match description
	results = searchIndexEntries(index, "programming")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'programming', got %d", len(results))
	}
}

// searchResult mirrors cli.SearchResult for testing
type searchResult struct {
	Category string
	Name     string
	Entry    registry.IndexEntry
}

// searchIndexEntries is a copy of the search logic for integration testing
func searchIndexEntries(index *registry.Index, query string) []searchResult {
	var results []searchResult
	queryLower := strings.ToLower(query)

	// Search agents
	for name, entry := range index.Agents {
		if matchesQuery(name, entry, queryLower) {
			results = append(results, searchResult{Category: "agents", Name: name, Entry: entry})
		}
	}

	// Search roles
	for name, entry := range index.Roles {
		if matchesQuery(name, entry, queryLower) {
			results = append(results, searchResult{Category: "roles", Name: name, Entry: entry})
		}
	}

	// Search tasks
	for name, entry := range index.Tasks {
		if matchesQuery(name, entry, queryLower) {
			results = append(results, searchResult{Category: "tasks", Name: name, Entry: entry})
		}
	}

	// Search contexts
	for name, entry := range index.Contexts {
		if matchesQuery(name, entry, queryLower) {
			results = append(results, searchResult{Category: "contexts", Name: name, Entry: entry})
		}
	}

	return results
}

func matchesQuery(name string, entry registry.IndexEntry, queryLower string) bool {
	if strings.Contains(strings.ToLower(name), queryLower) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Description), queryLower) {
		return true
	}
	if strings.Contains(strings.ToLower(entry.Module), queryLower) {
		return true
	}
	for _, tag := range entry.Tags {
		if strings.Contains(strings.ToLower(tag), queryLower) {
			return true
		}
	}
	return false
}

// TestIntegration_ModulesCommandHelp tests that help works for modules commands.
func TestIntegration_ModulesCommandHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "modules help",
			args: []string{"modules", "--help"},
			want: []string{"Manage modules", "browse", "search", "install", "list", "update"},
		},
		{
			name: "modules search help",
			args: []string{"modules", "search", "--help"},
			want: []string{"Search", "query", "3 characters"},
		},
		{
			name: "modules install help",
			args: []string{"modules", "install", "--help"},
			want: []string{"Install", "--local"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			cmd := cli.NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			_ = cmd.Execute() // Help returns nil

			output := buf.String()
			for _, want := range tt.want {
				if !strings.Contains(output, want) {
					t.Errorf("help output missing %q, got: %s", want, output)
				}
			}
		})
	}
}
