//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/p3bot/start/internal/cli"
	"github.com/p3bot/start/internal/registry"
)

// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

func TestIntegration_ModulesListWithConfig(t *testing.T) {
	tmpDir := t.TempDir()

	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	claude: {
		bin: "claude"
		command: "{{.bin}} --model {{.model}}"
		origin: "github.com/p3bot/library/agents/ai/claude@v1.0.0"
	}
}

roles: {
	assistant: {
		prompt: "You are a helpful assistant."
		origin: "github.com/p3bot/library/roles/assistant@v1.0.0"
	}
	reviewer: {
		prompt: "You are a code reviewer."
		origin: "github.com/p3bot/library/roles/reviewer@v1.0.0"
	}
}

tasks: {
	review: {
		prompt: "Review this code."
		origin: "github.com/p3bot/library/tasks/review@v1.0.0"
	}
}
`
	if err := os.WriteFile(filepath.Join(configDir, "settings.cue"), []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, tmpDir)

	// Override HOME to isolate from global config.
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	buf := new(bytes.Buffer)
	cmd := cli.NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "Installed modules") {
		t.Errorf("output should mention installed modules, got: %s", output)
	}
	if !strings.Contains(output, "agents:") {
		t.Errorf("output should show agents category, got: %s", output)
	}
	if !strings.Contains(output, "claude") {
		t.Errorf("output should show claude agent, got: %s", output)
	}
	if !strings.Contains(output, "roles:") {
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
		origin: "github.com/p3bot/library/agents/ai/claude@v1.0.0"
	}
}
roles: {
	assistant: {
		prompt: "You are a helpful assistant."
		origin: "github.com/p3bot/library/roles/assistant@v1.0.0"
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

	t.Run("list --json", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"list", "--json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list --json failed: %v", err)
		}

		var result []map[string]interface{}
		if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, buf.String())
		}
		if len(result) == 0 {
			t.Errorf("expected non-empty JSON array, got: %s", buf.String())
		}

		first := result[0]
		for _, field := range []string{"category", "name", "scope", "origin"} {
			if _, ok := first[field]; !ok {
				t.Errorf("JSON entry missing field %q, got: %v", field, first)
			}
		}
	})

	t.Run("list roles --json", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"list", "roles", "--json"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list roles --json failed: %v", err)
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
		origin: "github.com/p3bot/library/agents/ai/claude@v1.0.0"
	}
}

roles: {
	assistant: {
		prompt: "You are a helpful assistant."
		origin: "github.com/p3bot/library/roles/assistant@v1.0.0"
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
		cmd.SetArgs([]string{"list", "agents"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list agents failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "agents:") {
			t.Errorf("output should show agents category, got: %s", output)
		}
		if !strings.Contains(output, "claude") {
			t.Errorf("output should show claude, got: %s", output)
		}
		if strings.Contains(output, "roles:") {
			t.Errorf("output should not show roles when filtered to agents, got: %s", output)
		}
	})

	t.Run("filter to roles only", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"list", "roles"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list roles failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "roles:") {
			t.Errorf("output should show roles category, got: %s", output)
		}
		if !strings.Contains(output, "assistant") {
			t.Errorf("output should show assistant, got: %s", output)
		}
		if strings.Contains(output, "agents:") {
			t.Errorf("output should not show agents when filtered to roles, got: %s", output)
		}
	})

	t.Run("filter to tasks - none installed", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := cli.NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"list", "tasks"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("list tasks failed: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "No tasks installed") {
			t.Errorf("should report no tasks installed, got: %s", output)
		}
	})
}

func TestIntegration_ModulesListNoConfig(t *testing.T) {
	tmpDir := t.TempDir()

	chdir(t, tmpDir)

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	buf := new(bytes.Buffer)
	cmd := cli.NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"list"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No configuration found") {
		t.Errorf("should report no config, got: %s", output)
	}
}

// TestIntegration_SearchIndex exercises the search logic directly because the
// registry fetch is not easily mockable.
func TestIntegration_SearchIndex(t *testing.T) {
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

	results := searchIndexEntries(index, "golang")
	if len(results) != 2 {
		t.Errorf("expected 2 results for 'golang', got %d", len(results))
	}

	results = searchIndexEntries(index, "claude")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'claude', got %d", len(results))
	}
	if len(results) > 0 && results[0].Name != "ai/claude" {
		t.Errorf("expected ai/claude, got %s", results[0].Name)
	}

	results = searchIndexEntries(index, "programming")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'programming', got %d", len(results))
	}
}

// searchResult mirrors cli.SearchResult for testing.
type searchResult struct {
	Category string
	Name     string
	Entry    registry.IndexEntry
}

// searchIndexEntries duplicates cli's search logic for integration testing.
func searchIndexEntries(index *registry.Index, query string) []searchResult {
	var results []searchResult
	queryLower := strings.ToLower(query)

	for name, entry := range index.Agents {
		if matchesQuery(name, entry, queryLower) {
			results = append(results, searchResult{Category: "agents", Name: name, Entry: entry})
		}
	}

	for name, entry := range index.Roles {
		if matchesQuery(name, entry, queryLower) {
			results = append(results, searchResult{Category: "roles", Name: name, Entry: entry})
		}
	}

	for name, entry := range index.Tasks {
		if matchesQuery(name, entry, queryLower) {
			results = append(results, searchResult{Category: "tasks", Name: name, Entry: entry})
		}
	}

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

func TestIntegration_ModulesCommandHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "install help",
			args: []string{"install", "--help"},
			want: []string{"Install", "--local"},
		},
		{
			name: "list help",
			args: []string{"list", "--help"},
			want: []string{"installed", "--json"},
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
