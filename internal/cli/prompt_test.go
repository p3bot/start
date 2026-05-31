package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptCommand_Metadata(t *testing.T) {
	cmd := NewRootCmd()

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

	if !strings.Contains(promptCmd.Long, "required") {
		t.Error("Long description should mention required contexts")
	}
}

func setupPromptTestConfig(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

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
	return tmpDir
}

// Tests below use os.Chdir (process-global): do not add t.Parallel() to any
// test that calls os.Chdir — it races on the working directory.

func TestRunPrompt_DryRun(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

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

	if !strings.Contains(output, "Dry Run") {
		t.Errorf("output should contain 'Dry Run', got: %s", output)
	}

	if !strings.Contains(output, "echo") {
		t.Errorf("output should contain agent 'echo', got: %s", output)
	}
}

func TestRunPrompt_WithText(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

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

	output := stdout.String()
	if !strings.Contains(output, "Dry Run") {
		t.Errorf("expected dry run output, got: %s", output)
	}
}

func TestRunPrompt_NoText(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

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
	cmd.SetArgs([]string{"prompt", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Dry Run") {
		t.Errorf("expected dry run output, got: %s", output)
	}
}

func TestRunPrompt_RequiredContextsOnly(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

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

	if !strings.Contains(output, "required_context") {
		t.Errorf("output should include required_context, got: %s", output)
	}

	if !strings.Contains(output, "default_context") {
		t.Errorf("output should show default_context (as skipped), got: %s", output)
	}

	for line := range strings.SplitSeq(output, "\n") {
		if strings.Contains(line, "default_context") {
			if !strings.Contains(line, "○") {
				t.Errorf("default_context should show skipped status (○), got line: %s", line)
			}
			if strings.Contains(line, "✓") {
				t.Errorf("default_context should NOT show loaded status (✓), got line: %s", line)
			}
		}
	}

	if strings.Contains(output, "This is default only") {
		t.Errorf("prompt should NOT contain default context content, got: %s", output)
	}
}

// TestRunPrompt_ArgWinsOverPipedStdin verifies a positional argument
// short-circuits piped stdin so piped data is not misread as the prompt body.
func TestRunPrompt_ArgWinsOverPipedStdin(t *testing.T) {
	setupPromptTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("PIPED_TEXT_SHOULD_NOT_APPEAR"))
	cmd.SetArgs([]string{"prompt", "arg text wins", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	output := stdout.String()

	if !strings.Contains(output, "arg text wins") {
		t.Errorf("expected positional arg text in output, got:\n%s", output)
	}
	if strings.Contains(output, "PIPED_TEXT_SHOULD_NOT_APPEAR") {
		t.Errorf("piped stdin should be ignored when a positional arg is given, got:\n%s", output)
	}
}

// TestRunPrompt_PipedStdin verifies `start prompt` with no positional arg
// consumes piped stdin as the prompt text.
func TestRunPrompt_PipedStdin(t *testing.T) {
	setupPromptTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("piped prompt body\n"))
	cmd.SetArgs([]string{"prompt", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	output := stdout.String()

	if !strings.Contains(output, "piped prompt body") {
		t.Errorf("expected piped text in output, got:\n%s", output)
	}
}
