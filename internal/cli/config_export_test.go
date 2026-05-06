package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfigExportCommandExists(t *testing.T) {
	t.Parallel()
	cmd := NewRootCmd()

	var configCmd *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Use == "config" {
			configCmd = c
			break
		}
	}
	if configCmd == nil {
		t.Fatal("config command not found")
	}

	var exportCmd *cobra.Command
	for _, c := range configCmd.Commands() {
		if strings.HasPrefix(c.Use, "export") {
			exportCmd = c
			break
		}
	}
	if exportCmd == nil {
		t.Fatal("export subcommand not found")
	}
}

func TestConfigExport_SingleCategory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `agents: {
	"claude": {
		bin: "claude"
		description: "Anthropic Claude"
	}
}
`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export", "agents"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if output != content {
		t.Errorf("expected exact file content, got %q", output)
	}
	// Single category should not include a header
	if strings.Contains(output, "// agents.cue") {
		t.Error("single category export should not include file header")
	}
}

func TestConfigExport_AllCategories(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"agents.cue": `agents: { "claude": { bin: "claude" } }
`,
		"roles.cue": `roles: { "go-expert": { description: "Go expert" } }
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(globalDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "// agents.cue") {
		t.Error("expected agents.cue header in multi-file export")
	}
	if !strings.Contains(output, "// roles.cue") {
		t.Error("expected roles.cue header in multi-file export")
	}
	if !strings.Contains(output, `"claude"`) {
		t.Error("expected agents content in output")
	}
	if !strings.Contains(output, `"go-expert"`) {
		t.Error("expected roles content in output")
	}
}

func TestConfigExport_LocalFlag(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	localDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `tasks: { "review": { description: "Code review" } }
`
	if err := os.WriteFile(filepath.Join(localDir, "tasks.cue"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export", "tasks", "--local"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if output != content {
		t.Errorf("expected local file content, got %q", output)
	}
}

func TestConfigExport_UnknownCategory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export", "unknown"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("expected 'unknown category' error, got %v", err)
	}
}

func TestConfigExport_MissingConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	// Do not create the global config directory

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no configuration directory found") {
		t.Errorf("expected 'no configuration directory found' error, got %v", err)
	}
}

func TestConfigExport_MissingLocalConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	// Do not create the .start/ directory

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export", "--local"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no configuration directory found") {
		t.Errorf("expected 'no configuration directory found' error, got %v", err)
	}
	if !strings.Contains(err.Error(), ".start") {
		t.Errorf("expected error to reference .start path, got %v", err)
	}
}

func TestConfigExport_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	// Create global dir but no agents.cue
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export", "agents"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "configuration file not found") {
		t.Errorf("expected 'configuration file not found' error, got %v", err)
	}
}

func TestConfigExport_SingularAndPluralCategories(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `settings: { timeout: 30 }
`
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	for _, category := range []string{"setting", "settings"} {
		t.Run(category, func(t *testing.T) {
			cmd := NewRootCmd()
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"config", "export", category})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error for category %q: %v", category, err)
			}

			if stdout.String() != content {
				t.Errorf("category %q: expected file content, got %q", category, stdout.String())
			}
		})
	}
}

func TestConfigExport_SkipsNonCueFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	chdir(t, tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte("agents: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "notes.txt"), []byte("some notes"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "export"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if strings.Contains(output, "notes.txt") {
		t.Error("non-CUE files should be skipped in export")
	}
	if !strings.Contains(output, "// agents.cue") {
		t.Error("expected agents.cue in output")
	}
}
