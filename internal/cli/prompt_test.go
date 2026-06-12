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

	if promptCmd.Use != "prompt [text|file ...]" {
		t.Errorf("Use = %q, want %q", promptCmd.Use, "prompt [text|file ...]")
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

// dryRunPromptBody extracts the composed prompt body from a --dry-run run by
// reading prompt.md from the dry-run directory named in the summary's "Files:"
// line. This asserts the exact body, including seam spacing, which the
// truncated/indented stdout preview cannot show reliably.
func dryRunPromptBody(t *testing.T, output string) string {
	t.Helper()
	// The Files: block is the summary's last labelled section; the prompt-body
	// preview is printed earlier, so search from the end to avoid matching a
	// marker that happens to appear inside the previewed body.
	const marker = "Files: "
	idx := strings.LastIndex(output, marker)
	if idx < 0 {
		t.Fatalf("dry-run output missing %q marker:\n%s", marker, output)
	}
	rest := output[idx+len(marker):]
	if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
		rest = rest[:nl]
	}
	dir := strings.TrimSpace(rest)
	data, err := os.ReadFile(filepath.Join(dir, "prompt.md"))
	if err != nil {
		t.Fatalf("reading prompt.md from %q: %v", dir, err)
	}
	return string(data)
}

func writePromptFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func runPromptDryRun(t *testing.T, args ...string) string {
	t.Helper()
	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(append([]string{"prompt"}, append(args, "--dry-run")...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("prompt command error: %v", err)
	}
	return dryRunPromptBody(t, stdout.String())
}

// TestRunPrompt_MultipleSegments covers multi-argument composition: files and
// literals resolved in order and seam-joined with exactly one blank line,
// regardless of each file's trailing newline. tmpDir is the working directory
// (setupPromptTestConfig chdirs into it) so ./ paths resolve there.
func TestRunPrompt_MultipleSegments(t *testing.T) {
	tmpDir := setupPromptTestConfig(t)

	// First file ends in a newline, second does not; the seam must still
	// collapse to one blank line.
	writePromptFile(t, filepath.Join(tmpDir, "intro.md"), "intro line\n")
	writePromptFile(t, filepath.Join(tmpDir, "body.md"), "body line")
	writePromptFile(t, filepath.Join(tmpDir, "tilde.md"), "tilde contents\n")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "two files and a literal",
			args: []string{"./intro.md", "./body.md", "wrap up please"},
			want: "intro line\n\nbody line\n\nwrap up please",
		},
		{
			name: "literal before file",
			args: []string{"lead in", "./body.md"},
			want: "lead in\n\nbody line",
		},
		{
			name: "tilde path resolves to file contents",
			args: []string{"~/tilde.md", "trailer"},
			want: "tilde contents\n\ntrailer",
		},
		{
			name: "empty literal between segments is dropped",
			args: []string{"./body.md", "", "wrap up please"},
			want: "body line\n\nwrap up please",
		},
		{
			name: "three literals",
			args: []string{"a", "b", "c"},
			want: "a\n\nb\n\nc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runPromptDryRun(t, tt.args...); got != tt.want {
				t.Errorf("composed body = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRunPrompt_SingleArgumentParity pins that a lone literal or a lone file
// path resolves byte-identically to the previous single-argument handling: no
// drop-empty, no seam normalisation. The composer trims trailing newlines of
// the whole body, matching today's behaviour for both arg shapes.
func TestRunPrompt_SingleArgumentParity(t *testing.T) {
	tmpDir := setupPromptTestConfig(t)
	writePromptFile(t, filepath.Join(tmpDir, "only.md"), "only file body\n")

	if got := runPromptDryRun(t, "just literal"); got != "just literal" {
		t.Errorf("single literal body = %q, want %q", got, "just literal")
	}
	if got := runPromptDryRun(t, "./only.md"); got != "only file body" {
		t.Errorf("single file body = %q, want %q", got, "only file body")
	}
}

// TestRunPrompt_EmptyArgsOverridePipedStdin pins Requirement 6's edge: when
// arguments are present but all resolve to empty, piped stdin is still ignored
// and the body is empty. The stdin/interactive fallback is gated solely on
// len(args) == 0, not on whether the composed body is empty.
func TestRunPrompt_EmptyArgsOverridePipedStdin(t *testing.T) {
	setupPromptTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("PIPED_TEXT_SHOULD_NOT_APPEAR"))
	cmd.SetArgs([]string{"prompt", "", "", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("prompt command error: %v", err)
	}

	if body := dryRunPromptBody(t, stdout.String()); body != "" {
		t.Errorf("composed body = %q, want empty", body)
	}
	if strings.Contains(stdout.String(), "PIPED_TEXT_SHOULD_NOT_APPEAR") {
		t.Errorf("piped stdin must be ignored when args are present, got:\n%s", stdout.String())
	}
}

// TestRunPrompt_UnreadableFileArg verifies the first unreadable file-path
// argument aborts with the reading-prompt-file error and launches nothing.
func TestRunPrompt_UnreadableFileArg(t *testing.T) {
	setupPromptTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"prompt", "ok text", "./nonexistent-file-xyz.md", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unreadable file-path argument")
	}
	if !strings.Contains(err.Error(), `reading prompt "./nonexistent-file-xyz.md"`) {
		t.Errorf("error = %q, want it to name the unreadable file", err.Error())
	}
	if strings.Contains(stdout.String(), "Dry Run") {
		t.Errorf("nothing should launch on a read failure, got:\n%s", stdout.String())
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
