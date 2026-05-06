package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveConfigOpenPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	tests := []struct {
		name     string
		local    bool
		category string
		wantFile string
		wantErr  bool
	}{
		// singular
		{name: "agent", category: "agent", wantFile: "agents.cue"},
		{name: "role", category: "role", wantFile: "roles.cue"},
		{name: "context", category: "context", wantFile: "contexts.cue"},
		{name: "task", category: "task", wantFile: "tasks.cue"},
		{name: "setting", category: "setting", wantFile: "settings.cue"},
		// plural aliases
		{name: "agents", category: "agents", wantFile: "agents.cue"},
		{name: "roles", category: "roles", wantFile: "roles.cue"},
		{name: "contexts", category: "contexts", wantFile: "contexts.cue"},
		{name: "tasks", category: "tasks", wantFile: "tasks.cue"},
		{name: "settings", category: "settings", wantFile: "settings.cue"},
		// local flag
		{name: "agent local", category: "agent", local: true, wantFile: "agents.cue"},
		{name: "setting local", category: "setting", local: true, wantFile: "settings.cue"},
		// error case
		{name: "unknown", category: "unknown", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveConfigOpenPath(tc.local, tc.category)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "unknown category") {
					t.Errorf("expected 'unknown category' error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if filepath.Base(got) != tc.wantFile {
				t.Errorf("got filename %q, want %q", filepath.Base(got), tc.wantFile)
			}
			if tc.local {
				if !strings.Contains(got, ".start") {
					t.Errorf("expected local path to contain .start, got %q", got)
				}
			} else {
				if strings.Contains(got, ".start") {
					t.Errorf("expected global path (no .start), got %q", got)
				}
			}
		})
	}
}

func TestConfigOpen_NonInteractiveNoArg(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "open"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "category required") {
		t.Errorf("expected 'category required' error, got %v", err)
	}
}

func TestConfigOpen_WithCategory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("EDITOR", "true")
	chdir(t, tmpDir)

	for _, category := range []string{
		"agent", "agents",
		"role", "roles",
		"context", "contexts",
		"task", "tasks",
		"setting", "settings",
	} {
		t.Run(category, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetIn(strings.NewReader(""))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"config", "open", category})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error for category %q: %v", category, err)
			}
		})
	}
}

func TestConfigOpen_WithLocalFlag(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("EDITOR", "true")
	chdir(t, tmpDir)

	localDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "open", "--local", "agent"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigOpen_InteractivePrompt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	// Each test selects a number from the openCategories prompt and verifies
	// the resolved path filename matches the expected CUE file.
	tests := []struct {
		input    string // number to enter at the prompt
		wantFile string
	}{
		{"1\n", "agents.cue"},
		{"2\n", "roles.cue"},
		{"3\n", "contexts.cue"},
		{"4\n", "tasks.cue"},
		{"5\n", "settings.cue"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			w := &bytes.Buffer{}
			category, err := promptSelectCategory(w, slowStdin(tc.input), openCategories)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if category == "" {
				t.Fatal("expected category, got empty string")
			}
			got, err := resolveConfigOpenPath(false, category)
			if err != nil {
				t.Fatalf("unexpected error resolving path: %v", err)
			}
			if filepath.Base(got) != tc.wantFile {
				t.Errorf("got filename %q, want %q", filepath.Base(got), tc.wantFile)
			}
		})
	}
}

func TestConfigOpen_InteractivePromptCancelled(t *testing.T) {
	w := &bytes.Buffer{}
	category, err := promptSelectCategory(w, slowStdin("\n"), openCategories)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if category != "" {
		t.Errorf("expected empty category on cancellation, got %q", category)
	}
}

func TestConfigOpen_UnknownCategory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	cmd := NewRootCmd()
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "open", "unknown"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("expected 'unknown category' error, got %v", err)
	}
}
