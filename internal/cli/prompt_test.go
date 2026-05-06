package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptCommand_Exists(t *testing.T) {
	cmd := NewRootCmd()

	// Find the prompt command
	promptCmd, _, err := cmd.Find([]string{"prompt"})
	if err != nil {
		t.Fatalf("prompt command not found: %v", err)
	}

	if promptCmd.Use != "prompt [text]" {
		t.Errorf("Use = %q, want %q", promptCmd.Use, "prompt [text]")
	}

	if promptCmd.Short == "" {
		t.Error("Short description should not be empty")
	}

	// Check that Long description mentions required contexts behavior
	if !strings.Contains(promptCmd.Long, "required") {
		t.Error("Long description should mention required contexts")
	}
}

// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

func TestPromptCommand_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	// Isolate from global config
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create local config
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	echo: {
		bin: "echo"
		command: "{{.bin}} test"
		default_model: "default"
	}
}

roles: {
	assistant: {
		prompt: "You are a helpful assistant."
	}
}

contexts: {
	required_ctx: {
		required: true
		prompt: "Required context content"
	}
	default_ctx: {
		default: true
		prompt: "Default context content"
	}
}

settings: {
	default_agent: "echo"
}
`
	configFile := filepath.Join(configDir, "settings.cue")
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"prompt", "test prompt text", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	output := stdout.String()

	// Should show dry run header
	if !strings.Contains(output, "Dry Run") {
		t.Errorf("output should contain 'Dry Run', got: %s", output)
	}

	// Should show the agent
	if !strings.Contains(output, "echo") {
		t.Errorf("output should contain agent 'echo', got: %s", output)
	}
}

func TestPromptCommand_WithText(t *testing.T) {
	tmpDir := t.TempDir()

	// Isolate from global config
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create local config
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	echo: {
		bin: "echo"
		command: "{{.bin}} test"
		default_model: "default"
	}
}

settings: {
	default_agent: "echo"
}
`
	configFile := filepath.Join(configDir, "settings.cue")
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"prompt", "my custom prompt", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	// Command should execute successfully with custom text
	output := stdout.String()
	if !strings.Contains(output, "Dry Run") {
		t.Errorf("expected dry run output, got: %s", output)
	}
}

func TestPromptCommand_NoText(t *testing.T) {
	tmpDir := t.TempDir()

	// Isolate from global config
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create local config
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	echo: {
		bin: "echo"
		command: "{{.bin}} test"
		default_model: "default"
	}
}

settings: {
	default_agent: "echo"
}
`
	configFile := filepath.Join(configDir, "settings.cue")
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	// No text argument, just prompt command
	cmd.SetArgs([]string{"prompt", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	// Should work without text argument
	output := stdout.String()
	if !strings.Contains(output, "Dry Run") {
		t.Errorf("expected dry run output, got: %s", output)
	}
}

func TestPromptCommand_RequiredContextsOnly(t *testing.T) {
	// This test verifies that prompt command includes required contexts
	// but excludes default contexts.
	tmpDir := t.TempDir()

	// Isolate from global config
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create local config with both required and default contexts
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	echo: {
		bin: "echo"
		command: "{{.bin}} test"
		default_model: "default"
	}
}

contexts: {
	required_context: {
		required: true
		prompt: "This is required"
	}
	default_context: {
		default: true
		prompt: "This is default only"
	}
}

settings: {
	default_agent: "echo"
}
`
	configFile := filepath.Join(configDir, "settings.cue")
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"prompt", "test", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	output := stdout.String()

	// Should include required context with loaded status
	if !strings.Contains(output, "required_context") {
		t.Errorf("output should include required_context, got: %s", output)
	}

	// Default context should appear in context table with skipped status (○)
	// but should NOT appear in the prompt section
	if !strings.Contains(output, "default_context") {
		t.Errorf("output should show default_context (as skipped), got: %s", output)
	}

	// Verify it's shown as skipped (○), not loaded (✓)
	// Find the default_context line and check it has ○
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "default_context") {
			if !strings.Contains(line, "○") {
				t.Errorf("default_context should show skipped status (○), got line: %s", line)
			}
			if strings.Contains(line, "✓") {
				t.Errorf("default_context should NOT show loaded status (✓), got line: %s", line)
			}
		}
	}

	// Verify default context content is NOT in the prompt
	if strings.Contains(output, "This is default only") {
		t.Errorf("prompt should NOT contain default context content, got: %s", output)
	}
}
