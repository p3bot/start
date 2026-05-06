package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigInteractive_RequiresTerminal(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		wantMsg string
	}{
		{[]string{"config", "add"}, "interactive add requires a terminal"},
		{[]string{"config", "edit"}, "interactive edit requires a terminal"},
		{[]string{"config", "remove"}, "interactive remove requires a terminal"},
		{[]string{"config", "info"}, "interactive info requires a terminal"},
		// Explicit category arg still requires terminal for interactive prompts
		{[]string{"config", "add", "agent"}, "interactive add requires a terminal"},
		{[]string{"config", "add", "role"}, "interactive add requires a terminal"},
		{[]string{"config", "add", "context"}, "interactive add requires a terminal"},
		{[]string{"config", "add", "task"}, "interactive add requires a terminal"},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetIn(strings.NewReader(""))
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("expected error %q, got %q", tc.wantMsg, err.Error())
			}
		})
	}
}

func TestLoadNamesForCategory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Seed one item in each category using stdin-driven interactive input.
	if err := configAgentAdd(slowStdin("my-agent\nagent\n"+`agent "{{.prompt}}"`+"\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("setup agent add failed: %v", err)
	}
	if err := configRoleAdd(slowStdin("my-role\n\n3\nYou are a role.\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("setup role add failed: %v", err)
	}
	if err := configContextAdd(slowStdin("my-context\n\n3\nContext info.\n\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("setup context add failed: %v", err)
	}
	if err := configTaskAdd(slowStdin("my-task\n\n\nDo a task.\n\n\n\n"), &bytes.Buffer{}, false); err != nil {
		t.Fatalf("setup task add failed: %v", err)
	}

	for _, tc := range []struct {
		category string
		want     string
	}{
		{"agents", "my-agent"},
		{"roles", "my-role"},
		{"contexts", "my-context"},
		{"tasks", "my-task"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			names, err := loadNamesForCategory(tc.category, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			found := false
			for _, n := range names {
				if n == tc.want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected %q in names %v", tc.want, names)
			}
		})
	}

	t.Run("unknown category returns error", func(t *testing.T) {
		_, err := loadNamesForCategory("unknown", false)
		if err == nil {
			t.Fatal("expected error for unknown category")
		}
		if !strings.Contains(err.Error(), "unknown category") {
			t.Errorf("expected 'unknown category' error, got %v", err)
		}
	})
}
