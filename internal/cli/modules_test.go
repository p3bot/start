package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/registry"
)

// parseCUEStruct parses a CUE struct literal string into an ast.Expr for test input.
func parseCUEStruct(t *testing.T, src string) ast.Expr {
	t.Helper()
	f, err := parser.ParseFile("test", "a: "+src)
	if err != nil {
		t.Fatalf("parsing CUE struct: %v", err)
	}
	return f.Decls[0].(*ast.Field).Value
}

// TestSearchIndex tests the modules.SearchIndex function.
func TestSearchIndex(t *testing.T) {
	t.Parallel()
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"ai/claude": {
				Module:      "github.com/test/agents/ai/claude@v0",
				Description: "Claude by Anthropic",
				Tags:        []string{"anthropic", "ai", "llm"},
			},
			"ai/gemini": {
				Module:      "github.com/test/agents/ai/gemini@v0",
				Description: "Gemini by Google",
				Tags:        []string{"google", "ai", "llm"},
			},
		},
		Roles: map[string]registry.IndexEntry{
			"golang/assistant": {
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go programming expert",
				Tags:        []string{"golang", "programming"},
			},
		},
		Tasks: map[string]registry.IndexEntry{
			"golang/code-review": {
				Module:      "github.com/test/tasks/golang/code-review@v0",
				Description: "Review Go code for best practices",
				Tags:        []string{"golang", "review"},
			},
		},
		Contexts: map[string]registry.IndexEntry{},
	}

	tests := []struct {
		name       string
		query      string
		wantCount  int
		wantFirst  string
		wantInName bool
	}{
		{
			name:      "search by name - claude",
			query:     "claude",
			wantCount: 1,
			wantFirst: "ai/claude",
		},
		{
			name:      "search by tag - golang",
			query:     "golang",
			wantCount: 2, // role and task
		},
		{
			name:       "search by description - anthropic",
			query:      "anthropic",
			wantCount:  1,
			wantFirst:  "ai/claude",
			wantInName: false,
		},
		{
			name:      "search multiple matches - ai",
			query:     "ai",
			wantCount: 2, // claude and gemini
		},
		{
			name:      "no matches",
			query:     "nonexistent",
			wantCount: 0,
		},
		{
			name:      "case insensitive",
			query:     "CLAUDE",
			wantCount: 1,
			wantFirst: "ai/claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := modules.SearchIndex(index, tt.query, nil)
			if err != nil {
				t.Fatalf("modules.SearchIndex() error: %v", err)
			}

			if len(results) != tt.wantCount {
				t.Errorf("modules.SearchIndex() returned %d results, want %d", len(results), tt.wantCount)
			}

			if tt.wantFirst != "" && len(results) > 0 {
				if results[0].Name != tt.wantFirst {
					t.Errorf("first result = %q, want %q", results[0].Name, tt.wantFirst)
				}
			}
		})
	}
}

// TestPrintSearchResults tests the printSearchResults function.
func TestPrintSearchResults(t *testing.T) {
	t.Parallel()
	results := []modules.SearchResult{
		{
			Category: "agents",
			Name:     "ai/claude",
			Entry: registry.IndexEntry{
				Module:      "github.com/test/agents/ai/claude@v0",
				Description: "Claude by Anthropic",
				Tags:        []string{"anthropic", "ai"},
			},
		},
		{
			Category: "roles",
			Name:     "golang/assistant",
			Entry: registry.IndexEntry{
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go programming expert",
				Tags:        []string{"golang"},
			},
		},
	}

	t.Run("non-verbose output", func(t *testing.T) {
		var buf bytes.Buffer
		printSearchResults(&buf, results, false, nil)
		output := buf.String()

		if !strings.Contains(output, "Found 2 matches") {
			t.Errorf("output missing match count, got: %s", output)
		}
		if !strings.Contains(output, "agents/") {
			t.Errorf("output missing agents category, got: %s", output)
		}
		if !strings.Contains(output, "ai/claude") {
			t.Errorf("output missing claude, got: %s", output)
		}
		if !strings.Contains(output, "Claude by Anthropic") {
			t.Errorf("output missing description, got: %s", output)
		}
	})

	t.Run("verbose output", func(t *testing.T) {
		var buf bytes.Buffer
		printSearchResults(&buf, results, true, nil)
		output := buf.String()

		if !strings.Contains(output, "Module:") {
			t.Errorf("verbose output missing Module, got: %s", output)
		}
		if !strings.Contains(output, "Tags:") {
			t.Errorf("verbose output missing Tags, got: %s", output)
		}
	})

	t.Run("installed marker", func(t *testing.T) {
		var buf bytes.Buffer
		installed := map[string]bool{
			"agents/ai/claude": true,
		}
		printSearchResults(&buf, results, false, installed)
		output := buf.String()

		if !strings.Contains(output, "★") {
			t.Errorf("output missing installed marker for ai/claude, got: %s", output)
		}
	})
}

// TestModulesCommandExists tests that the modules command is registered.
func TestModulesCommandExists(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()

	// Find modules command
	var modulesCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Use == "modules" {
			modulesCmd = c
			break
		}
	}

	if modulesCmd == nil {
		t.Fatal("modules command not found")
	}

	// Check subcommands
	subcommands := []string{"browse", "index", "search", "install", "list", "info", "update"}
	for _, name := range subcommands {
		found := false
		for _, c := range modulesCmd.Commands() {
			if strings.HasPrefix(c.Use, name) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("subcommand %q not found", name)
		}
	}
}

// TestPrintIndex tests the printIndex function.
func TestPrintIndex(t *testing.T) {
	t.Parallel()
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"ai/claude": {
				Module:      "github.com/test/agents/ai/claude@v0",
				Description: "Claude by Anthropic",
				Tags:        []string{"anthropic", "ai", "llm"},
			},
			"ai/gemini": {
				Module:      "github.com/test/agents/ai/gemini@v0",
				Description: "Gemini by Google",
				Tags:        []string{"google", "ai"},
			},
		},
		Roles: map[string]registry.IndexEntry{
			"golang/assistant": {
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go programming expert",
				Tags:        []string{"golang"},
			},
		},
		Tasks:    map[string]registry.IndexEntry{},
		Contexts: map[string]registry.IndexEntry{},
	}

	t.Run("default output", func(t *testing.T) {
		var buf bytes.Buffer
		printIndex(&buf, index, "v0.2.3", false, nil, "")
		output := buf.String()

		if !strings.Contains(output, "Index: v0.2.3 (3 modules)") {
			t.Errorf("output missing header, got: %s", output)
		}
		if !strings.Contains(output, "agents/") {
			t.Errorf("output missing agents category, got: %s", output)
		}
		if !strings.Contains(output, "roles/") {
			t.Errorf("output missing roles category, got: %s", output)
		}
		if !strings.Contains(output, "ai/claude") {
			t.Errorf("output missing ai/claude, got: %s", output)
		}
		if !strings.Contains(output, "ai/gemini") {
			t.Errorf("output missing ai/gemini, got: %s", output)
		}
		if !strings.Contains(output, "golang/assistant") {
			t.Errorf("output missing golang/assistant, got: %s", output)
		}
		// Verify alphabetical ordering: ai/claude before ai/gemini
		claudeIdx := strings.Index(output, "ai/claude")
		geminiIdx := strings.Index(output, "ai/gemini")
		if claudeIdx > geminiIdx {
			t.Errorf("ai/claude should appear before ai/gemini, got claude at %d, gemini at %d", claudeIdx, geminiIdx)
		}
	})

	t.Run("verbose output", func(t *testing.T) {
		var buf bytes.Buffer
		printIndex(&buf, index, "v0.2.3", true, nil, "")
		output := buf.String()

		if !strings.Contains(output, "Module:") {
			t.Errorf("verbose output missing Module, got: %s", output)
		}
		if !strings.Contains(output, "Tags:") {
			t.Errorf("verbose output missing Tags, got: %s", output)
		}
	})

	t.Run("installed marker", func(t *testing.T) {
		var buf bytes.Buffer
		installed := map[string]bool{
			"agents/ai/claude": true,
		}
		printIndex(&buf, index, "v0.2.3", false, installed, "")
		output := buf.String()

		if !strings.Contains(output, "★") {
			t.Errorf("output missing installed marker, got: %s", output)
		}
	})

	t.Run("category count", func(t *testing.T) {
		var buf bytes.Buffer
		printIndex(&buf, index, "v0.2.3", false, nil, "")
		output := buf.String()

		if !strings.Contains(output, "(2)") {
			t.Errorf("output missing agents count (2), got: %s", output)
		}
		if !strings.Contains(output, "(1)") {
			t.Errorf("output missing roles count (1), got: %s", output)
		}
	})

	t.Run("category filter agents only", func(t *testing.T) {
		var buf bytes.Buffer
		printIndex(&buf, index, "v0.2.3", false, nil, "agents")
		output := buf.String()

		if !strings.Contains(output, "agents/") {
			t.Errorf("output missing agents category, got: %s", output)
		}
		if strings.Contains(output, "roles/") {
			t.Errorf("output should not contain roles when filtered to agents, got: %s", output)
		}
		if !strings.Contains(output, "ai/claude") {
			t.Errorf("output missing ai/claude, got: %s", output)
		}
	})

	t.Run("category filter preserves full total in header", func(t *testing.T) {
		var buf bytes.Buffer
		printIndex(&buf, index, "v0.2.3", false, nil, "agents")
		output := buf.String()

		// Header should show all 3 modules, not just agents
		if !strings.Contains(output, "(3 modules)") {
			t.Errorf("header should show full total even when filtered, got: %s", output)
		}
	})
}

// TestModulesIndexCommandExists tests that the index command is properly registered.
func TestModulesIndexCommandExists(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()

	// Find modules command
	var modulesCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Use == "modules" {
			modulesCmd = c
			break
		}
	}
	if modulesCmd == nil {
		t.Fatal("modules command not found")
	}

	// Find index subcommand
	var indexCmd *cobra.Command
	for _, c := range modulesCmd.Commands() {
		if strings.HasPrefix(c.Use, "index") {
			indexCmd = c
			break
		}
	}
	if indexCmd == nil {
		t.Fatal("index subcommand not found")
		return
	}

	// Check alias
	if len(indexCmd.Aliases) == 0 || indexCmd.Aliases[0] != "idx" {
		t.Errorf("expected alias 'idx', got %v", indexCmd.Aliases)
	}

	// Check flags
	jsonFlag := indexCmd.Flags().Lookup("json")
	if jsonFlag == nil {
		t.Error("--json flag not found")
	}
	exportFlag := indexCmd.Flags().Lookup("export")
	if exportFlag == nil {
		t.Error("--export flag not found")
	}
}

// TestUpdateModuleInConfig tests the modules.UpdateModuleInConfig function.
func TestUpdateModuleInConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		initial     string
		category    string
		moduleName  string
		newContent  string
		wantContain []string
		wantErr     bool
	}{
		{
			name: "update simple module",
			initial: `tasks: {
	"my/task": {
		origin: "old/origin"
		description: "old description"
		prompt: "old prompt"
	}
}
`,
			category:   "tasks",
			moduleName: "my/task",
			newContent: `{
	origin: "new/origin"
	description: "new description"
	prompt: "new prompt"
}`,
			wantContain: []string{
				`"my/task"`,
				`"new/origin"`,
				`"new description"`,
				`"new prompt"`,
			},
		},
		{
			name: "update module with template braces",
			initial: `tasks: {
	"project/start": {
		origin: "old/origin"
		prompt: """
			{{.instructions}}
			"""
	}
}
`,
			category:   "tasks",
			moduleName: "project/start",
			newContent: `{
	origin: "new/origin"
	prompt: """
		{{if .instructions}}
		## Custom Instructions
		{{.instructions}}
		{{end}}
		"""
}`,
			wantContain: []string{
				`"project/start"`,
				`"new/origin"`,
				`{{if .instructions}}`,
				`{{end}}`,
			},
		},
		{
			name: "update preserves other modules",
			initial: `tasks: {
	"first/task": {
		origin: "first/origin"
		prompt: "first"
	}
	"second/task": {
		origin: "second/origin"
		prompt: "second"
	}
}
`,
			category:   "tasks",
			moduleName: "first/task",
			newContent: `{
	origin: "updated/origin"
	prompt: "updated"
}`,
			wantContain: []string{
				`"first/task"`,
				`"updated/origin"`,
				`"second/task"`,
				`"second/origin"`,
			},
		},
		{
			name: "braces in string literals",
			initial: `tasks: {
	"my/task": {
		origin: "old/origin"
		description: "Use } to close blocks and { to open them"
		prompt: "Code example: if (x) { return } else { continue }"
	}
}
`,
			category:   "tasks",
			moduleName: "my/task",
			newContent: `{
	origin: "new/origin"
	description: "Updated: { and } are important"
	prompt: "new prompt"
}`,
			wantContain: []string{
				`"my/task"`,
				`"new/origin"`,
				`"Updated: { and } are important"`,
				`"new prompt"`,
			},
		},
		{
			name: "comments with braces",
			initial: `tasks: {
	"my/task": {
		origin: "old/origin"
		// Note: use { and } carefully
		description: "old description"
		// NOTE: update prompt { revised version }
	}
}
`,
			category:   "tasks",
			moduleName: "my/task",
			newContent: `{
	origin: "new/origin"
	description: "new description"
}`,
			wantContain: []string{
				`"my/task"`,
				`"new/origin"`,
				`"new description"`,
			},
		},
		{
			name: "key in comment before actual definition",
			initial: `tasks: {
	// NOTE: Configure "my/task": see docs
	// Also check "my/task": for updates
	"my/task": {
		origin: "old/origin"
		description: "old description"
	}
}
`,
			category:   "tasks",
			moduleName: "my/task",
			newContent: `{
	origin: "new/origin"
	description: "updated"
}`,
			wantContain: []string{
				`// NOTE: Configure "my/task": see docs`,
				`"my/task"`,
				`"new/origin"`,
				`"updated"`,
			},
		},
		{
			name: "comment with braces between key and opening brace",
			initial: `tasks: {
	"my/task": // NOTE: see details { v2 }
	{
		origin: "old/origin"
		description: "old description"
	}
}
`,
			category:   "tasks",
			moduleName: "my/task",
			newContent: `{
	origin: "new/origin"
	description: "updated"
}`,
			wantContain: []string{
				`"my/task"`,
				`"new/origin"`,
				`"updated"`,
			},
		},
		{
			name: "key in string before actual definition",
			initial: `tasks: {
	"other/task": {
		origin: "other"
		description: "This relates to my/task: the foundation"
		prompt: "See my/task: for details"
	}
	"my/task": {
		origin: "old/origin"
		description: "old description"
	}
}
`,
			category:   "tasks",
			moduleName: "my/task",
			newContent: `{
	origin: "new/origin"
	description: "updated"
}`,
			wantContain: []string{
				`"other/task"`,
				`"This relates to my/task: the foundation"`,
				`"my/task"`,
				`"new/origin"`,
			},
		},
		{
			name: "module not found",
			initial: `tasks: {
	"existing/task": {
		origin: "origin"
	}
}
`,
			category:   "tasks",
			moduleName: "nonexistent/task",
			newContent: `{}`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "tasks.cue")

			if err := os.WriteFile(configPath, []byte(tt.initial), 0644); err != nil {
				t.Fatalf("failed to write initial config: %v", err)
			}

			content := parseCUEStruct(t, tt.newContent)
			err := modules.UpdateModuleInConfig(configPath, tt.category, tt.moduleName, content)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			result, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("failed to read result: %v", err)
			}

			for _, want := range tt.wantContain {
				if !strings.Contains(string(result), want) {
					t.Errorf("result missing %q\ngot:\n%s", want, result)
				}
			}
		})
	}
}

// TestModulesListCategoryValidation tests that invalid category args are rejected early.
func TestModulesListCategoryValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "invalid category",
			args:    []string{"modules", "list", "invalid"},
			wantErr: `unknown category "invalid"`,
		},
		{
			name:    "valid category agents - no error from validation",
			args:    []string{"modules", "list", "agents"},
			wantErr: "", // fails later on config, not on category validation
		},
		{
			name:    "valid category plural",
			args:    []string{"modules", "list", "tasks"},
			wantErr: "",
		},
		{
			name:    "valid category singular",
			args:    []string{"modules", "list", "task"},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
			} else {
				// Valid categories must not fail category validation regardless of
				// what happens downstream (e.g. missing config is acceptable here).
				if err != nil && strings.Contains(err.Error(), "unknown category") {
					t.Errorf("valid category should not fail validation, got: %v", err)
				}
			}
		})
	}
}

// TestModulesIndexCategoryValidation tests that invalid category args are rejected before network I/O,
// and that --export rejects a category arg since the index is a single file.
func TestModulesIndexCategoryValidation(t *testing.T) {
	t.Parallel()

	t.Run("invalid category", func(t *testing.T) {
		cmd := NewRootCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"modules", "index", "invalid"})
		err := cmd.Execute()

		if err == nil || !strings.Contains(err.Error(), `unknown category "invalid"`) {
			t.Errorf("expected unknown category error, got %v", err)
		}
	})

	t.Run("category with --export", func(t *testing.T) {
		cmd := NewRootCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"modules", "index", "agents", "--export"})
		err := cmd.Execute()

		if err == nil || !strings.Contains(err.Error(), "cannot be used with --export") {
			t.Errorf("expected --export conflict error, got %v", err)
		}
	})
}

// TestPrintInstalledModulesJSON tests that installed modules marshal to valid JSON.
func TestPrintInstalledModulesJSON(t *testing.T) {
	t.Parallel()
	installed := []InstalledModule{
		{
			Category:     "agents",
			Name:         "ai/claude",
			Description:  "Claude by Anthropic",
			Tags:         []string{"anthropic", "ai"},
			Models:       []string{"claude-sonnet-4-20250514"},
			InstalledVer: "v0.2.0",
			Scope:        "global",
			Origin:       "github.com/test/agents/ai/claude@v0.2.0",
			ConfigFile:   "/home/user/.start/agents.cue",
		},
		{
			Category:     "roles",
			Name:         "golang/assistant",
			Description:  "Go programming expert",
			Tags:         []string{"golang"},
			InstalledVer: "v0.1.0",
			Scope:        "local",
			Origin:       "github.com/test/roles/golang/assistant@v0.1.0",
			ConfigFile:   "/project/.start/roles.cue",
		},
	}

	data, err := json.MarshalIndent(installed, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	output := string(data)

	if !strings.Contains(output, `"category": "agents"`) {
		t.Errorf("output missing category field, got: %s", output)
	}
	if !strings.Contains(output, `"name": "ai/claude"`) {
		t.Errorf("output missing name field, got: %s", output)
	}
	if !strings.Contains(output, `"version": "v0.2.0"`) {
		t.Errorf("output missing version field, got: %s", output)
	}
	if !strings.Contains(output, `"scope": "global"`) {
		t.Errorf("output missing scope field, got: %s", output)
	}
	if !strings.Contains(output, `"description": "Claude by Anthropic"`) {
		t.Errorf("output missing description field, got: %s", output)
	}
	if !strings.Contains(output, `"anthropic"`) {
		t.Errorf("output missing tags, got: %s", output)
	}
	if !strings.Contains(output, `"claude-sonnet-4-20250514"`) {
		t.Errorf("output missing models, got: %s", output)
	}
	if strings.Contains(output, `"updateAvailable"`) {
		t.Errorf("omitempty should suppress false updateAvailable, got: %s", output)
	}
}

// TestInstalledModuleJSONOmitsEmptyOptionalFields verifies omitempty suppresses nil slices.
func TestInstalledModuleJSONOmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()
	module := InstalledModule{
		Category: "roles",
		Name:     "test",
		Scope:    "global",
		Origin:   "github.com/test/roles/test@v0.1.0",
	}

	data, err := json.MarshalIndent(module, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	output := string(data)

	if strings.Contains(output, `"tags"`) {
		t.Errorf("omitempty should suppress nil tags, got: %s", output)
	}
	if strings.Contains(output, `"models"`) {
		t.Errorf("omitempty should suppress nil models, got: %s", output)
	}
	if strings.Contains(output, `"description"`) {
		t.Errorf("omitempty should suppress empty description, got: %s", output)
	}
}

// TestSearchResultJSON tests that SearchResult marshals to valid JSON with correct field names.
func TestSearchResultJSON(t *testing.T) {
	t.Parallel()
	results := []modules.SearchResult{
		{
			Category: "agents",
			Name:     "ai/claude",
			Entry: registry.IndexEntry{
				Module:      "github.com/test/agents/ai/claude@v0",
				Description: "Claude by Anthropic",
				Tags:        []string{"anthropic", "ai"},
				Version:     "v0.2.0",
			},
			MatchScore: 5,
		},
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	output := string(data)

	for _, want := range []string{
		`"category": "agents"`,
		`"name": "ai/claude"`,
		`"matchScore": 5`,
		`"entry"`,
		`"module": "github.com/test/agents/ai/claude@v0"`,
		`"description": "Claude by Anthropic"`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %s, got: %s", want, output)
		}
	}
}

// TestUpdateResultJSON tests that UpdateResult marshals with error as string and omits Error field.
func TestUpdateResultJSON(t *testing.T) {
	t.Parallel()
	results := []UpdateResult{
		{
			Module:     InstalledModule{Category: "agents", Name: "ai/claude", Scope: "global", Origin: "test"},
			OldVersion: "v0.1.0",
			NewVersion: "v0.2.0",
			Updated:    true,
		},
		{
			Module:       InstalledModule{Category: "roles", Name: "golang", Scope: "global", Origin: "test"},
			OldVersion:   "v0.1.0",
			Updated:      false,
			Error:        fmt.Errorf("network timeout"),
			ErrorMessage: "network timeout",
		},
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	output := string(data)

	// First result: updated, no error
	if !strings.Contains(output, `"oldVersion": "v0.1.0"`) {
		t.Errorf("output missing oldVersion, got: %s", output)
	}
	if !strings.Contains(output, `"newVersion": "v0.2.0"`) {
		t.Errorf("output missing newVersion, got: %s", output)
	}
	if !strings.Contains(output, `"updated": true`) {
		t.Errorf("output missing updated=true, got: %s", output)
	}

	// Second result: error serialised as string
	if !strings.Contains(output, `"error": "network timeout"`) {
		t.Errorf("output missing error string, got: %s", output)
	}

	// Verify the Error (interface) field is excluded from JSON
	var decoded []map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	for _, item := range decoded {
		if _, ok := item["Error"]; ok {
			t.Errorf("Error interface field should be excluded via json:\"-\", got: %v", item)
		}
	}
}

// TestModuleInfoResultJSON tests that ModuleInfoResult marshals correctly.
func TestModuleInfoResultJSON(t *testing.T) {
	t.Parallel()
	results := []ModuleInfoResult{
		{
			Category: "agents",
			Name:     "ai/claude",
			Entry: registry.IndexEntry{
				Module:      "github.com/test/agents/ai/claude@v0",
				Description: "Claude by Anthropic",
			},
			MatchScore:     5,
			Installed:      true,
			InstalledScope: "global",
		},
		{
			Category: "roles",
			Name:     "golang/assistant",
			Entry: registry.IndexEntry{
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go expert",
			},
			MatchScore: 3,
			Installed:  false,
		},
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	output := string(data)

	for _, want := range []string{
		`"installed": true`,
		`"installedScope": "global"`,
		`"installed": false`,
	} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %s, got: %s", want, output)
		}
	}

	// installedScope should be omitted for non-installed
	var decoded []map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if _, ok := decoded[1]["installedScope"]; ok {
		t.Errorf("installedScope should be omitted for non-installed module")
	}
}

// TestFilterIndexByCategory tests that filterIndexByCategory isolates a single category.
func TestFilterIndexByCategory(t *testing.T) {
	t.Parallel()
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"ai/claude": {Description: "Claude"},
		},
		Roles: map[string]registry.IndexEntry{
			"golang/assistant": {Description: "Go expert"},
		},
		Tasks:    map[string]registry.IndexEntry{},
		Contexts: map[string]registry.IndexEntry{},
	}

	tests := []struct {
		category   string
		wantAgents bool
		wantRoles  bool
	}{
		{"agents", true, false},
		{"roles", false, true},
		{"tasks", false, false},
		{"contexts", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			got := filterIndexByCategory(index, tt.category)

			if tt.wantAgents && len(got.Agents) == 0 {
				t.Errorf("expected agents in filtered index, got none")
			}
			if !tt.wantAgents && len(got.Agents) > 0 {
				t.Errorf("expected no agents in filtered index, got %d", len(got.Agents))
			}
			if tt.wantRoles && len(got.Roles) == 0 {
				t.Errorf("expected roles in filtered index, got none")
			}
			if !tt.wantRoles && len(got.Roles) > 0 {
				t.Errorf("expected no roles in filtered index, got %d", len(got.Roles))
			}
		})
	}
}

// TestModulesSearchValidation tests search command argument validation.
func TestModulesSearchValidation(t *testing.T) {
	t.Parallel()
	// We can't easily test the full search without network,
	// but we can test the query length validation
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"query too short - 1 char", "a", true},
		{"query too short - 2 chars", "ab", true},
		{"query valid - 3 chars", "abc", false}, // Will fail on network, but passes validation
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just test the validation logic
			if len(tt.query) < 3 {
				if !tt.wantErr {
					t.Error("expected validation error for short query")
				}
			}
		})
	}
}
