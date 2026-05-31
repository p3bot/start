package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/registry"
)

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

		if !strings.Contains(output, "(3 modules)") {
			t.Errorf("header should show full total even when filtered, got: %s", output)
		}
	})
}

// TestLibraryCommandExists checks the library command is registered with the
// lib alias and its flags, and that the old idx alias is gone.
func TestLibraryCommandExists(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()

	for _, c := range cmd.Commands() {
		if c.Name() != "library" {
			continue
		}

		hasLib := false
		for _, a := range c.Aliases {
			if a == "lib" {
				hasLib = true
			}
			if a == "idx" {
				t.Error("library should not carry the old 'idx' alias")
			}
		}
		if !hasLib {
			t.Errorf("expected alias 'lib', got %v", c.Aliases)
		}

		if c.Flags().Lookup("json") == nil {
			t.Error("--json flag not found")
		}
		if c.Flags().Lookup("export") == nil {
			t.Error("--export flag not found")
		}
		return
	}
	t.Fatal("library command not found")
}

// TestLibraryCategoryValidation checks invalid category args are rejected
// before network I/O, and that --export rejects a category arg.
func TestLibraryCategoryValidation(t *testing.T) {
	t.Parallel()

	t.Run("invalid category", func(t *testing.T) {
		cmd := NewRootCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"library", "invalid"})
		err := cmd.Execute()

		if err == nil || !strings.Contains(err.Error(), `unknown category "invalid"`) {
			t.Errorf("expected unknown category error, got %v", err)
		}
	})

	t.Run("category with --export", func(t *testing.T) {
		cmd := NewRootCmd()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs([]string{"library", "agents", "--export"})
		err := cmd.Execute()

		if err == nil || !strings.Contains(err.Error(), "cannot be used with --export") {
			t.Errorf("expected --export conflict error, got %v", err)
		}
	})
}

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
