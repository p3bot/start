package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/parser"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/registry"
)

func parseCUEStruct(t *testing.T, src string) ast.Expr {
	t.Helper()
	f, err := parser.ParseFile("test", "a: "+src)
	if err != nil {
		t.Fatalf("parsing CUE struct: %v", err)
	}
	return f.Decls[0].(*ast.Field).Value
}

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

// TestFlatModuleCommandsExist verifies module commands are registered at the
// top level after the modules parent was removed.
func TestFlatModuleCommandsExist(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()

	want := map[string]bool{"install": false, "list": false, "update": false, "library": false}
	for _, c := range cmd.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("top-level command %q not found", name)
		}
	}
}

// TestModulesParentRemoved verifies the old modules parent, its singular alias,
// and the removed browse command return Cobra's unknown-command error.
func TestModulesParentRemoved(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"modules"},
		{"module"},
		{"modules", "install"},
		{"modules", "browse"},
		{"browse"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			cmd := NewRootCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected unknown-command error for %v, got nil", args)
			}
			if !strings.Contains(err.Error(), "unknown command") {
				t.Errorf("expected unknown-command error for %v, got: %v", args, err)
			}
		})
	}
}
