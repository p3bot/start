package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// setupGetTestConfig writes a CUE config covering each get code path.
func setupGetTestConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Isolate from the user's real config and registry cache.
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// CUE module cache writes read-only files; chmod before TempDir cleanup.
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return os.Chmod(path, 0o755)
			}
			return os.Chmod(path, 0o644)
		})
	})

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	// File-source role (real on-disk file, expanded via DefaultFileReader).
	roleFile := filepath.Join(dir, "role.md")
	if err := os.WriteFile(roleFile, []byte("Role file contents.\n"), 0o644); err != nil {
		t.Fatalf("writing role file: %v", err)
	}

	// File+prompt UTD: file must win over prompt for get.
	mixedFile := filepath.Join(dir, "mixed.md")
	if err := os.WriteFile(mixedFile, []byte("MIXED FILE CONTENT"), 0o644); err != nil {
		t.Fatalf("writing mixed file: %v", err)
	}

	// Tilde-path UTD: referenced as "~/tilde-role.md" in CUE; lives at $HOME
	// (== dir, set above). Exercises ExpandFilePath via DefaultFileReader.
	tildeFile := filepath.Join(dir, "tilde-role.md")
	if err := os.WriteFile(tildeFile, []byte("Tilde file contents."), 0o644); err != nil {
		t.Fatalf("writing tilde file: %v", err)
	}

	// Origin-bearing role: a fake origin string is fine — printGetVerbose only
	// reads the value from CUE; @module/ resolution is not triggered for an
	// absolute file path.
	tracedFile := filepath.Join(dir, "traced.md")
	if err := os.WriteFile(tracedFile, []byte("traced contents"), 0o644); err != nil {
		t.Fatalf("writing traced file: %v", err)
	}

	// Relative-path role: file written into dir (the test's cwd after chdir
	// below), referenced as "./relative-role.md" in CUE. Exercises
	// ExpandFilePath's filepath.Abs branch via DefaultFileReader.
	relativeFile := filepath.Join(dir, "relative-role.md")
	if err := os.WriteFile(relativeFile, []byte("relative contents"), 0o644); err != nil {
		t.Fatalf("writing relative file: %v", err)
	}

	// File whose content references {{.command_output}}, paired with a non-empty
	// command in CUE. Used to assert getUTD's trim block suppresses
	// TemplateProcessor.Process's lazy command execution
	// (template.go: needsCommandOutput && fields.Command != "").
	fcCmdRefFile := filepath.Join(dir, "fc-cmd-ref.md")
	if err := os.WriteFile(fcCmdRefFile, []byte("before {{.command_output}} after"), 0o644); err != nil {
		t.Fatalf("writing fc-cmd-ref file: %v", err)
	}

	cueConfig := `
agents: {
	claude: {
		bin:           "claude"
		command:       "{{.bin}} --model {{.model}} '{{.prompt}}'"
		description:   "Claude"
		default_model: "sonnet"
		models: {
			sonnet: "claude-sonnet-4"
			haiku:  "claude-haiku-4"
		}
	}
	bare: {
		bin:         "bare"
		description: "Agent with no command"
	}
}

roles: {
	"role-file": {
		description: "File-source role"
		file:        "` + roleFile + `"
	}
	"role-prompt": {
		description: "Prompt-source role"
		prompt:      "Hello {{.user}}"
	}
	"role-mixed": {
		description: "File and prompt; file should win"
		file:        "` + mixedFile + `"
		prompt:      "PROMPT WINS"
	}
	"role-empty": {
		description: "No source fields"
	}
	"role-tilde": {
		description: "Tilde-path file source"
		file:        "~/tilde-role.md"
	}
	"role-relative": {
		description: "Relative-path file source"
		file:        "./relative-role.md"
	}
	"pc-priority": {
		description: "Prompt and command; prompt should win"
		prompt:      "PROMPT VALUE"
		command:     "echo COMMAND VALUE"
	}
	"role-module-no-origin": {
		description: "@module/ file path without origin (error guard)"
		file:        "@module/anywhere.md"
	}
	"role-traced": {
		description: "File source with origin (verbose metadata)"
		file:        "` + tracedFile + `"
		origin:      "github.com/example/start-library/roles/traced@v1.2.3"
	}
	"fc-cmd-ref": {
		description: "File source whose content references {{.command_output}}; command must not run"
		file:        "` + fcCmdRefFile + `"
		command:     "echo SHOULD-NOT-APPEAR"
	}
	"pc-cmd-ref": {
		description: "Prompt referencing {{.command_output}} with command; command must not run"
		prompt:      "before {{.command_output}} after"
		command:     "echo SHOULD-NOT-APPEAR"
	}
}

contexts: {
	"ctx-cmd": {
		description: "Command-source context with custom shell and timeout"
		command:     "printf 'cmd-output'"
		shell:       "bash -c"
		timeout:     5
	}
}

tasks: {
	"task-prompt": {
		description: "Prompt-source task"
		prompt:      "Task body"
	}
	"task-cmd": {
		description: "Command-source task"
		command:     "printf 'task-cmd-output'"
	}
}
`

	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, dir)
	return dir
}

// runGetCmd runs `start get` with the given args and a non-TTY stdin.
// Returns stdout, stderr, and any error from cmd.Execute().
func runGetCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(append([]string{"get"}, args...))
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// TestGetUTDPromptSource verifies get renders a prompt-source role and the
// rendered template variables (e.g. {{.user}}) are substituted.
func TestGetUTDPromptSource(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "role-prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if !strings.HasPrefix(stdout, "Hello ") {
		t.Errorf("expected stdout to start with rendered prompt, got: %q", stdout)
	}
	if strings.Contains(stdout, "{{.user}}") {
		t.Errorf("template placeholder should be substituted, got: %q", stdout)
	}
}

// TestGetUTDFileSource verifies a file-source role outputs the file contents.
func TestGetUTDFileSource(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "role-file")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if stdout != "Role file contents.\n" {
		t.Errorf("stdout = %q, want %q", stdout, "Role file contents.\n")
	}
}

// TestGetUTDFileWinsOverPrompt verifies the file > prompt > command priority:
// when both file and prompt are defined, get outputs the file.
func TestGetUTDFileWinsOverPrompt(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "role-mixed")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "MIXED FILE CONTENT") {
		t.Errorf("expected file content in stdout, got: %q", stdout)
	}
	if strings.Contains(stdout, "PROMPT WINS") {
		t.Errorf("prompt should not appear when file is set, got: %q", stdout)
	}
}

// TestGetUTDCommandSource verifies a command-source UTD module executes the
// command and that custom shell/timeout flow through to the runner. The trim
// block in getUTD must preserve Shell and Timeout — they are execution
// config, not source fields.
func TestGetUTDCommandSourceWithShellTimeout(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "ctx-cmd")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if stdout != "cmd-output\n" {
		t.Errorf("stdout = %q, want %q", stdout, "cmd-output\n")
	}
}

// TestGetAgent verifies an agent's command template is partially rendered:
// {{.bin}} and {{.model}} are substituted; runtime placeholders remain.
func TestGetAgent(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "claude")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if !strings.Contains(stdout, "claude --model claude-sonnet-4") {
		t.Errorf("expected resolved bin and model, got: %q", stdout)
	}
	if !strings.Contains(stdout, "{{.prompt}}") {
		t.Errorf("runtime {{.prompt}} placeholder should remain, got: %q", stdout)
	}
}

// TestGetAgentNoCommand verifies an agent with no command field returns a
// configuration error naming the agent and leaves stdout empty.
func TestGetAgentNoCommand(t *testing.T) {
	setupGetTestConfig(t)

	stdout, _, err := runGetCmd(t, "bare")
	if err == nil {
		t.Fatal("expected error for agent with no command field")
	}
	if !strings.Contains(err.Error(), "bare") {
		t.Errorf("error should name the agent, got: %v", err)
	}
	if !strings.Contains(err.Error(), "command") {
		t.Errorf("error should mention command field, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on error, got: %q", stdout)
	}
}

// TestGetUTDEmptyFields verifies a UTD module with no file, prompt, or command
// returns a configuration error naming the module and listing the expected
// fields. Stdout stays empty.
func TestGetUTDEmptyFields(t *testing.T) {
	setupGetTestConfig(t)

	stdout, _, err := runGetCmd(t, "role-empty")
	if err == nil {
		t.Fatal("expected error for UTD module with no source fields")
	}
	if !strings.Contains(err.Error(), "role-empty") {
		t.Errorf("error should name the module, got: %v", err)
	}
	for _, want := range []string{"file", "prompt", "command"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list %q as expected field, got: %v", want, err)
		}
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on error, got: %q", stdout)
	}
}

// TestGetNoArgNonTTY verifies that running `get` with no argument in a
// non-interactive environment returns an error rather than blocking on a
// prompt.
func TestGetNoArgNonTTY(t *testing.T) {
	setupGetTestConfig(t)

	_, _, err := runGetCmd(t)
	if err == nil {
		t.Fatal("expected error for no argument in non-TTY mode")
	}
	if !strings.Contains(err.Error(), "non-interactive") {
		t.Errorf("error should mention non-interactive mode, got: %v", err)
	}
}

// TestGetAmbiguousNonTTY verifies that an ambiguous name in non-TTY mode
// returns an error listing the candidate matches.
func TestGetAmbiguousNonTTY(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return os.Chmod(path, 0o755)
			}
			return os.Chmod(path, 0o644)
		})
	})

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
roles: {
	helper: {
		prompt: "role helper"
	}
}
tasks: {
	helper: {
		prompt: "task helper"
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)

	_, _, err := runGetCmd(t, "helper")
	if err == nil {
		t.Fatal("expected ambiguity error in non-TTY mode")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention ambiguity, got: %v", err)
	}
	for _, want := range []string{"roles:helper", "tasks:helper"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list candidate %q, got: %v", want, err)
		}
	}
}

// TestGetVerboseCommandSource verifies --verbose against a command-source
// UTD module emits a "Command: ..." line on stderr alongside Type/Name.
// Without this metadata, a user piping `start get --verbose ctx-cmd | ...`
// has no visibility into the shell-out that produced stdout.
func TestGetVerboseCommandSource(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--verbose", "get", "ctx-cmd"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	stderrStr := stderr.String()
	for _, want := range []string{"Type: Context", "Name: ctx-cmd", "Command: printf 'cmd-output'"} {
		if !strings.Contains(stderrStr, want) {
			t.Errorf("stderr missing %q\ngot: %s", want, stderrStr)
		}
	}
	if stdout.String() != "cmd-output\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "cmd-output\n")
	}
}

// TestGetVerboseToStderr verifies --verbose writes metadata to stderr without
// polluting stdout.
func TestGetVerboseToStderr(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--verbose", "get", "role-prompt"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	stderrStr := stderr.String()
	for _, want := range []string{"Type: Role", "Name: role-prompt"} {
		if !strings.Contains(stderrStr, want) {
			t.Errorf("stderr missing %q\ngot: %s", want, stderrStr)
		}
	}

	stdoutStr := stdout.String()
	for _, banned := range []string{"Type:", "Name:"} {
		if strings.Contains(stdoutStr, banned) {
			t.Errorf("stdout should not contain %q metadata, got: %q", banned, stdoutStr)
		}
	}
	if !strings.HasPrefix(stdoutStr, "Hello ") {
		t.Errorf("stdout should still contain rendered content, got: %q", stdoutStr)
	}
}

// TestGetQuietSuppressesStderr verifies that --quiet leaves stdout holding
// only the module content with stderr empty. Three independent stderr-write
// paths converge in runGet and get* helpers — autoInstall progress
// (resolve.go), notifyScopeWidenedIfLocal (describe.go), and printGetVerbose
// (get.go) — and a regression in any single Quiet/Verbose gate would leak
// metadata into a `start get --quiet | bar` pipeline. The autoInstall arm
// of that contract is unit-tested in resolve.go's tests; the widen-notice
// arm in TestNotifyScopeWidenedIfLocal. This test covers the verbose-path
// gate and the integration shape (no flag combination produces stderr on the
// happy path).
func TestGetQuietSuppressesStderr(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--quiet", "get", "role-prompt"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	if stderr.Len() != 0 {
		t.Errorf("--quiet must leave stderr empty on happy path, got: %q", stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "Hello ") {
		t.Errorf("stdout should carry rendered content, got: %q", stdout.String())
	}
}

// TestGetResolveQueryRoutesToStderr asserts the wiring contract from the
// implementation plan: `get` must invoke promptSearchQuery with stderr (not
// stdout) and emit the short-query fallback to stderr. This keeps `start get
// | bar` pipe-clean when stdin is a TTY but stdout is piped.
func TestGetResolveQueryRoutesToStderr(t *testing.T) {
	setupGetTestConfig(t)

	t.Run("no-arg non-TTY surfaces error without writing stderr", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		_, err := getResolveQuery(nil, stderr, strings.NewReader(""))
		if err == nil {
			t.Fatal("expected error for no-arg non-TTY")
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr should be untouched on non-TTY error path, got: %q", stderr.String())
		}
	})

	t.Run("short-query non-TTY surfaces error without writing stderr", func(t *testing.T) {
		stderr := new(bytes.Buffer)
		_, err := getResolveQuery([]string{"ab"}, stderr, strings.NewReader(""))
		if err == nil {
			t.Fatal("expected error for short query in non-TTY")
		}
		if !strings.Contains(err.Error(), "3 characters") {
			t.Errorf("error should mention minimum length, got: %v", err)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr should be untouched on non-TTY error path, got: %q", stderr.String())
		}
	})
}

// TestGetCommandHelp verifies `start get help` prints the command help and
// that the get command registers --global plus inherits --local from root.
// Help-string assertions are limited to text that only lives in help (the
// stdout-routing contract, the start-show pointer, the auto-install widening
// note); flag presence is asserted via direct flag lookup so cosmetic
// help-formatter changes (heading order, line wrapping, colour) cannot
// false-fail it. Source priority is pinned by TestGetUTDFileWinsOverPrompt
// and TestGetUTDPromptWinsOverCommand, so help wording is not the place to
// re-assert it. No config isolation is needed: `help` short-circuits in
// checkHelpArg before any config is loaded.
func TestGetCommandHelp(t *testing.T) {
	stdout, _, err := runGetCmd(t, "help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"get", "stdout", "start describe", "Auto-installed"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output missing %q\ngot: %s", want, stdout)
		}
	}

	root := NewRootCmd()
	var getCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "get" {
			getCmd = c
			break
		}
	}
	if getCmd == nil {
		t.Fatal("get command not registered on root")
	}
	if getCmd.Flag("global") == nil {
		t.Error("get command missing --global flag")
	}
	if getCmd.Flag("local") == nil {
		t.Error("get command missing inherited --local flag (expected via root persistent flags)")
	}
}

// TestGetAppearsInRootHelp verifies the get command is registered on the
// root with GroupID "commands" so it lands in the Commands section of help
// output. Asserting the structural property avoids fragility against Cobra
// help-formatter changes (heading order, colour codes, line wrapping).
func TestGetAppearsInRootHelp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	cmd := NewRootCmd()
	for _, c := range cmd.Commands() {
		if c.Name() == "get" {
			if c.GroupID != "commands" {
				t.Errorf("get.GroupID = %q, want %q", c.GroupID, "commands")
			}
			return
		}
	}
	t.Fatal("get command not registered on root")
}

// TestGetUnknownName verifies that a name with no installed or registry
// matches surfaces a clear error and leaves stdout empty. Acceptance criterion
// "Unknown module names produce a clear error".
func TestGetUnknownName(t *testing.T) {
	setupGetTestConfig(t)

	stdout, _, err := runGetCmd(t, "definitely-not-a-real-module-zzz")
	if err == nil {
		t.Fatal("expected error for unknown module name")
	}
	if !strings.Contains(err.Error(), "definitely-not-a-real-module-zzz") {
		t.Errorf("error should name the missing module, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on error, got: %q", stdout)
	}
}

// TestGetUTDTildePath verifies that a UTD module whose `file` field uses a
// `~/`-prefixed path resolves through DefaultFileReader's tilde expansion and
// outputs the file's contents. Acceptance criterion: "UTD file resolution
// succeeds for @module/, ~/, and relative paths".
func TestGetUTDTildePath(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "role-tilde")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "Tilde file contents.\n" {
		t.Errorf("stdout = %q, want %q", stdout, "Tilde file contents.\n")
	}
}

// TestGetVerboseFileAndOrigin verifies that --verbose against a UTD module
// with both `file` and `origin` emits Type, Name, Origin, and Path metadata
// lines to stderr, while stdout still receives the raw file contents.
func TestGetVerboseFileAndOrigin(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--verbose", "get", "role-traced"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	stderrStr := stderr.String()
	wants := []string{
		"Type: Role",
		"Name: role-traced",
		"Origin: github.com/example/start-library/roles/traced@v1.2.3",
		"Path: ", // Path: <absolute> — only assert the prefix; absolute path varies by tempdir
	}
	for _, want := range wants {
		if !strings.Contains(stderrStr, want) {
			t.Errorf("stderr missing %q\ngot: %s", want, stderrStr)
		}
	}
	// Path line should reference the actual file basename so we know it's the
	// resolved path, not the literal config string.
	if !strings.Contains(stderrStr, "traced.md") {
		t.Errorf("stderr Path line missing resolved file name 'traced.md'\ngot: %s", stderrStr)
	}

	if stdout.String() != "traced contents\n" {
		t.Errorf("stdout = %q, want %q", stdout.String(), "traced contents\n")
	}
}

// TestGetShortQueryNonTTYEndToEnd is the cobra-level counterpart to the
// getResolveQuery unit test: a short query in a non-TTY environment must
// return the descriptive error and never write to stdout. Together with
// TestGetResolveQueryRoutesToStderr this proves the runGet → getResolveQuery
// wiring keeps stdout pipe-clean on the failure path. The TTY-mode re-prompt
// on stderr is not covered here because the project has no pseudo-TTY helpers
// (see project plan, Implementation Guidance).
func TestGetShortQueryNonTTYEndToEnd(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "ab")
	if err == nil {
		t.Fatal("expected error for short query in non-TTY")
	}
	if !strings.Contains(err.Error(), "3 characters") {
		t.Errorf("error should mention minimum length, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on short-query failure, got: %q", stdout)
	}
	// Sanity check that the non-TTY path did not also dump the TTY-mode
	// "Query must be at least 3 characters" line to stderr — that fallback is
	// only meaningful when the user can be re-prompted. Failing to gate it
	// would clutter scripted callers' error output.
	if strings.Contains(stderr, "Query must be at least 3 characters") {
		t.Errorf("non-TTY path should not emit TTY re-prompt notice, stderr: %q", stderr)
	}
}

// TestGetUTDFileSourceSuppressesCommand pins the safety property of
// getUTD's source-priority trim block for the file branch. With both `file`
// and `command` set, and the file's content referencing {{.command_output}},
// the module's command must not execute. TemplateProcessor.Process's lazy
// {{.command_output}} expansion (template.go: needsCommandOutput &&
// fields.Command != "") would otherwise shell out — getUTD's trim block
// (get.go: file != "" → fields.Command = "") is what prevents it. If
// Process's source-selection or lazy-eval semantics ever change so the trim
// block stops protecting against this, this test fails. The companion
// TestGetUTDFileWinsOverPrompt covers the externally observable behaviour
// (file content wins) but not the no-shell-out invariant.
func TestGetUTDFileSourceSuppressesCommand(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "fc-cmd-ref")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if strings.Contains(stdout, "SHOULD-NOT-APPEAR") {
		t.Errorf("command output leaked into file-source render — getUTD trim block did not suppress command execution\nstdout: %q\nstderr: %s", stdout, stderr)
	}
	for _, marker := range []string{"before", "after"} {
		if !strings.Contains(stdout, marker) {
			t.Errorf("file content marker %q missing from stdout: %q", marker, stdout)
		}
	}
}

// TestGetUTDPromptSourceSuppressesCommand is the prompt-branch counterpart
// to TestGetUTDFileSourceSuppressesCommand. With `prompt` set and the prompt
// referencing {{.command_output}} alongside a non-empty `command`, the
// command must not execute. Guards the prompt-branch arm of getUTD's trim
// block (get.go: prompt != "" → fields.Command = "") against the same
// TemplateProcessor.Process lazy-eval regression.
func TestGetUTDPromptSourceSuppressesCommand(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "pc-cmd-ref")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	if strings.Contains(stdout, "SHOULD-NOT-APPEAR") {
		t.Errorf("command output leaked into prompt-source render — getUTD trim block did not suppress command execution\nstdout: %q\nstderr: %s", stdout, stderr)
	}
	for _, marker := range []string{"before", "after"} {
		if !strings.Contains(stdout, marker) {
			t.Errorf("prompt content marker %q missing from stdout: %q", marker, stdout)
		}
	}
}

// TestGetUTDPromptWinsOverCommand covers the second branch of getUTD's
// source-priority trim. With file empty and both prompt and command set, the
// prompt must win (file > prompt > command). Without this test the
// `else if fields.Prompt != ""` branch is uncovered.
func TestGetUTDPromptWinsOverCommand(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "pc-priority")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "PROMPT VALUE") {
		t.Errorf("stdout should contain prompt content, got: %q", stdout)
	}
	if strings.Contains(stdout, "COMMAND VALUE") {
		t.Errorf("command output must not appear when prompt is set, got: %q", stdout)
	}
}

// TestGetUTDRelativePath verifies a UTD module whose `file` field is a
// relative path (e.g. "./role.md") resolves through ExpandFilePath's
// filepath.Abs branch and outputs the file's contents. Acceptance criterion:
// "UTD file resolution succeeds for @module/, ~/, and relative paths".
func TestGetUTDRelativePath(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "role-relative")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "relative contents\n" {
		t.Errorf("stdout = %q, want %q", stdout, "relative contents\n")
	}
}

// TestGetUTDModuleNoOrigin verifies the error guard in getUTD: a module
// with an @module/ file path but no origin field returns a descriptive error
// naming the module, and stdout stays empty.
func TestGetUTDModuleNoOrigin(t *testing.T) {
	setupGetTestConfig(t)

	stdout, _, err := runGetCmd(t, "role-module-no-origin")
	if err == nil {
		t.Fatal("expected error for @module/ path without origin")
	}
	if !strings.Contains(err.Error(), "role-module-no-origin") {
		t.Errorf("error should name the module, got: %v", err)
	}
	if !strings.Contains(err.Error(), "@module/") {
		t.Errorf("error should mention @module/, got: %v", err)
	}
	if !strings.Contains(err.Error(), "origin") {
		t.Errorf("error should mention origin field, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on error, got: %q", stdout)
	}
}

// TestGetTooManyArgs verifies that more than one positional argument is
// rejected (Requirement 5.1: "accepts zero or one positional argument"). The
// Args validator runs before RunE, so this is a cobra-level rejection and
// stdout never gets touched.
func TestGetTooManyArgs(t *testing.T) {
	setupGetTestConfig(t)

	stdout, _, err := runGetCmd(t, "role-prompt", "extra-arg")
	if err == nil {
		t.Fatal("expected error for two positional arguments")
	}
	if stdout != "" {
		t.Errorf("stdout should be empty when args validation fails, got: %q", stdout)
	}
}

// TestGetUTDModulePath verifies @module/ resolution end-to-end: the file is
// looked up in $CUE_CACHE_DIR/mod/extract/<dir(modulePath)>/<base(modulePath)+version>/
// (see ResolveModulePath in composer.go). The test fabricates that directory
// layout and reads through getUTD's @module/ branch and DefaultFileReader.
// Acceptance criterion: "UTD file resolution succeeds for @module/, ~/, and
// relative paths".
func TestGetUTDModulePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	cacheDir := filepath.Join(dir, "cue-cache")
	t.Setenv("CUE_CACHE_DIR", cacheDir)

	// CUE module cache writes read-only files; chmod before TempDir cleanup.
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return os.Chmod(path, 0o755)
			}
			return os.Chmod(path, 0o644)
		})
	})

	// Origin layout: <dir(modulePath)>/<base(modulePath)+version>
	// → github.com/example/test-mod / sub@v1.0.0
	origin := "github.com/example/test-mod/sub@v1.0.0"
	versionedDir := filepath.Join(cacheDir, "mod", "extract",
		"github.com", "example", "test-mod", "sub@v1.0.0")
	if err := os.MkdirAll(versionedDir, 0o755); err != nil {
		t.Fatalf("creating versioned cache dir: %v", err)
	}
	moduleFile := filepath.Join(versionedDir, "module-content.md")
	if err := os.WriteFile(moduleFile, []byte("module file content"), 0o644); err != nil {
		t.Fatalf("writing module file: %v", err)
	}

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
roles: {
	"role-module": {
		description: "Module-prefixed file"
		file:        "@module/module-content.md"
		origin:      "` + origin + `"
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)

	stdout, stderr, err := runGetCmd(t, "role-module")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "module file content\n" {
		t.Errorf("stdout = %q, want %q", stdout, "module file content\n")
	}
}

// TestGetMergedScopeFindsGlobalAndLocal pins the scope constant in
// runGet's loadConfig call from both directions. The two existing scopes
// (Global at $HOME/.config/start/, Local at ./.start/) each contain a unique
// role; both must be reachable through `start get`. A regression that flipped
// runGet to ScopeLocal would break the global sub-test; flipping to
// ScopeGlobal would break the local sub-test. Pattern follows
// TestDescribeGlobalFlag in describe_test.go: HOME is set but XDG_CONFIG_HOME is not,
// so globalConfigDir resolves to $HOME/.config/start/.
func TestGetMergedScopeFindsGlobalAndLocal(t *testing.T) {
	dir := t.TempDir()

	globalStartDir := filepath.Join(dir, ".config", "start")
	if err := os.MkdirAll(globalStartDir, 0o755); err != nil {
		t.Fatalf("creating global config dir: %v", err)
	}
	globalCueConfig := `
roles: {
	"global-only-role": {
		description: "Lives only in global config"
		prompt:      "from global"
	}
}
`
	if err := os.WriteFile(filepath.Join(globalStartDir, "settings.cue"), []byte(globalCueConfig), 0o644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	localStartDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(localStartDir, 0o755); err != nil {
		t.Fatalf("creating local config dir: %v", err)
	}
	localCueConfig := `
roles: {
	"local-only-role": {
		description: "Lives only in local config"
		prompt:      "from local"
	}
}
`
	if err := os.WriteFile(filepath.Join(localStartDir, "settings.cue"), []byte(localCueConfig), 0o644); err != nil {
		t.Fatalf("writing local config: %v", err)
	}

	// globalConfigDir prefers XDG_CONFIG_HOME over $HOME/.config. Force the
	// $HOME/.config branch so this test is deterministic regardless of the
	// caller's environment.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", dir)
	chdir(t, dir)

	t.Run("global-only role visible via merged scope", func(t *testing.T) {
		stdout, stderr, err := runGetCmd(t, "global-only-role")
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
		}
		if stdout != "from global\n" {
			t.Errorf("stdout = %q, want %q", stdout, "from global\n")
		}
	})

	t.Run("local-only role visible via merged scope", func(t *testing.T) {
		stdout, stderr, err := runGetCmd(t, "local-only-role")
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
		}
		if stdout != "from local\n" {
			t.Errorf("stdout = %q, want %q", stdout, "from local\n")
		}
	})
}

// TestGetAgentModelOverrideExact verifies that --model with a key in the
// agent's models map produces the resolved id, not the agent's default_model.
// Regression guard for the rendering contract: `start --model haiku get claude`
// and `start --model haiku claude` must agree on the substituted model id.
func TestGetAgentModelOverrideExact(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--model", "haiku", "get", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	stdoutStr := stdout.String()
	if !strings.Contains(stdoutStr, "claude-haiku-4") {
		t.Errorf("expected --model haiku to resolve to %q, got: %q", "claude-haiku-4", stdoutStr)
	}
	if strings.Contains(stdoutStr, "claude-sonnet-4") {
		t.Errorf("default_model should not appear when --model is set, got: %q", stdoutStr)
	}
}

// TestGetAgentModelOverrideSubstring verifies the multi-term substring path
// of resolveModelName: --model "hai" should match "haiku" since it is the
// only key containing that substring. Pins parity with `start`'s --model
// resolution rather than just exact-match.
func TestGetAgentModelOverrideSubstring(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--model", "hai", "get", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	stdoutStr := stdout.String()
	if !strings.Contains(stdoutStr, "claude-haiku-4") {
		t.Errorf("expected --model 'hai' to resolve to haiku id %q, got: %q", "claude-haiku-4", stdoutStr)
	}
}

// TestGetAgentModelOverridePassthrough verifies that a --model value not
// present in the agent's models map is substituted verbatim. This lets users
// pass arbitrary model identifiers without having to register them in CUE.
func TestGetAgentModelOverridePassthrough(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--model", "claude-opus-4-7", "get", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	stdoutStr := stdout.String()
	if !strings.Contains(stdoutStr, "claude-opus-4-7") {
		t.Errorf("expected literal --model value to pass through, got: %q", stdoutStr)
	}
}

// TestGetVerboseTildePathExpanded verifies --verbose reports the resolved
// absolute path for a tilde-prefixed file source, not the literal "~/..."
// string from the CUE config. The file is read from the expanded location
// regardless; this pins the metadata reported to the user.
func TestGetVerboseTildePathExpanded(t *testing.T) {
	dir := setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--verbose", "get", "role-tilde"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	expectedPath := filepath.Join(dir, "tilde-role.md")
	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "Path: "+expectedPath) {
		t.Errorf("stderr should report expanded path %q, got: %s", expectedPath, stderrStr)
	}
	if strings.Contains(stderrStr, "Path: ~/tilde-role.md") {
		t.Errorf("stderr should not contain the unexpanded literal path, got: %s", stderrStr)
	}
}

// setupGetDualScopeConfig writes a global config at $HOME/.config/start/ and
// a local config at ./.start/. Both define a "shared-role" with distinct
// content so --local vs --global resolution can be told apart; each scope also
// defines a scope-only role to exercise the not-found path of the other scope.
// Mirrors TestGetMergedScopeFindsGlobalAndLocal's environment setup.
func setupGetDualScopeConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Not-found lookups fall through to the registry, which writes read-only
	// files into HOME/.cache/cue. Re-chmod before TempDir cleanup so the
	// teardown unlink calls can succeed.
	t.Cleanup(func() {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return os.Chmod(path, 0o755)
			}
			return os.Chmod(path, 0o644)
		})
	})

	globalStartDir := filepath.Join(dir, ".config", "start")
	if err := os.MkdirAll(globalStartDir, 0o755); err != nil {
		t.Fatalf("creating global config dir: %v", err)
	}
	globalCueConfig := `
roles: {
	"global-only-role": {
		description: "Lives only in global config"
		prompt:      "from global"
	}
	"shared-role": {
		description: "Defined in both scopes"
		prompt:      "shared from global"
	}
}
`
	if err := os.WriteFile(filepath.Join(globalStartDir, "settings.cue"), []byte(globalCueConfig), 0o644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	localStartDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(localStartDir, 0o755); err != nil {
		t.Fatalf("creating local config dir: %v", err)
	}
	localCueConfig := `
roles: {
	"local-only-role": {
		description: "Lives only in local config"
		prompt:      "from local"
	}
	"shared-role": {
		description: "Defined in both scopes"
		prompt:      "shared from local"
	}
}
`
	if err := os.WriteFile(filepath.Join(localStartDir, "settings.cue"), []byte(localCueConfig), 0o644); err != nil {
		t.Fatalf("writing local config: %v", err)
	}

	// globalConfigDir prefers XDG_CONFIG_HOME over $HOME/.config. Force the
	// $HOME/.config branch so this test is deterministic regardless of the
	// caller's environment.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", dir)
	chdir(t, dir)
	return dir
}

// TestGetLocalScope verifies --local restricts resolution to the local
// config by asserting a global-only role is not visible under --local.
//
// This test has only one assertion (compared to TestGetGlobalScope's two)
// because merged-scope CUE resolution makes local override global on field
// conflict — so a "shared role under --local returns local content" check
// would pass even if --local were silently ignored and merged scope used
// instead. The only discriminating assertion for --local wiring is the
// not-found path on a global-only module: under --local-respected the module
// is invisible, under --local-ignored merged scope finds it. See
// TestGetGlobalScope for the symmetric test, which has two assertions
// because merged scope returns the local value for shared roles, making the
// "global wins under --global" assertion discriminating.
func TestGetLocalScope(t *testing.T) {
	setupGetDualScopeConfig(t)

	stdout, _, err := runGetCmd(t, "--local", "global-only-role")
	if err == nil {
		t.Fatal("expected not-found error for global-only role under --local")
	}
	if !strings.Contains(err.Error(), "global-only-role") {
		t.Errorf("error should name the missing module, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on not-found error, got: %q", stdout)
	}
}

// TestGetGlobalScope verifies --global restricts resolution to the global
// config: the shared role resolves to the global definition, and a local-only
// name fails with a not-found error and empty stdout.
func TestGetGlobalScope(t *testing.T) {
	setupGetDualScopeConfig(t)

	t.Run("shared role resolves to global definition", func(t *testing.T) {
		stdout, stderr, err := runGetCmd(t, "--global", "shared-role")
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
		}
		if stdout != "shared from global\n" {
			t.Errorf("stdout = %q, want %q", stdout, "shared from global\n")
		}
	})

	t.Run("local-only role not found under --global", func(t *testing.T) {
		stdout, _, err := runGetCmd(t, "--global", "local-only-role")
		if err == nil {
			t.Fatal("expected not-found error for local-only role under --global")
		}
		if !strings.Contains(err.Error(), "local-only-role") {
			t.Errorf("error should name the missing module, got: %v", err)
		}
		if stdout != "" {
			t.Errorf("stdout should be empty on not-found error, got: %q", stdout)
		}
	})
}

// TestGetLocalAndGlobalMutuallyExclusive verifies that passing both --local
// and --global returns the same mutual-exclusion error as `start describe` and
// writes nothing to stdout. No fixture is required: describeScopeFromCmd errors
// before runGet reaches loadConfig, so the cwd and HOME contents are
// irrelevant on this path.
func TestGetLocalAndGlobalMutuallyExclusive(t *testing.T) {
	stdout, _, err := runGetCmd(t, "--local", "--global", "any-name")
	if err == nil {
		t.Fatal("expected mutual-exclusion error when both --local and --global are set")
	}
	if !strings.Contains(err.Error(), "--local and --global are mutually exclusive") {
		t.Errorf("error should match show's mutual-exclusion text, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on mutual-exclusion error, got: %q", stdout)
	}
}

// TestEnsureTrailingNewline pins the helper that normalises stdout line-
// alignment. Empty content stays empty (so an empty module does not produce a
// stray blank line), an already-newline-terminated string is returned
// unchanged (no double newline), and a string without a trailing newline
// gets exactly one appended.
func TestEnsureTrailingNewline(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty stays empty", "", ""},
		{"already newline-terminated", "abc\n", "abc\n"},
		{"missing newline gets one", "abc", "abc\n"},
		{"multi-line with newline preserved", "a\nb\nc\n", "a\nb\nc\n"},
		{"multi-line without newline gets one", "a\nb\nc", "a\nb\nc\n"},
		{"trailing whitespace untouched", "abc  ", "abc  \n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureTrailingNewline(tc.in)
			if got != tc.want {
				t.Errorf("ensureTrailingNewline(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestGetAgentVerboseMetadata covers the --verbose branch of getAgent
// (get.go: 175-177). The agent has no file path or command source field,
// so verbose stderr must contain Type and Name only — no Path or Command
// line — and stdout is unaffected by the verbose flag.
func TestGetAgentVerboseMetadata(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--verbose", "get", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}

	stderrStr := stderr.String()
	for _, want := range []string{"Type: Agent", "Name: claude"} {
		if !strings.Contains(stderrStr, want) {
			t.Errorf("stderr missing %q\ngot: %s", want, stderrStr)
		}
	}
	for _, banned := range []string{"Path:", "Command:"} {
		if strings.Contains(stderrStr, banned) {
			t.Errorf("stderr should not contain %q for an agent (no UTD source fields), got: %s", banned, stderrStr)
		}
	}

	stdoutStr := stdout.String()
	for _, banned := range []string{"Type:", "Name:"} {
		if strings.Contains(stdoutStr, banned) {
			t.Errorf("stdout should not contain %q metadata, got: %q", banned, stdoutStr)
		}
	}
	if !strings.Contains(stdoutStr, "claude --model claude-sonnet-4") {
		t.Errorf("stdout should still contain the rendered agent command, got: %q", stdoutStr)
	}
}

// TestGetAgentRuntimePlaceholdersIntact pins get's contract that runtime
// placeholders are passed through verbatim: only {{.bin}} and {{.model}} are
// resolved at get time. {{.prompt}}, {{.role}}, {{.role_file}}, and
// {{.datetime}} are filled by the agent execution path (start/task), not
// get, and must remain in the rendered command for downstream piping.
func TestGetAgentRuntimePlaceholdersIntact(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
agents: {
	"runtime-rich": {
		bin:           "rt"
		command:       "{{.bin}} -m {{.model}} -r {{.role}} -rf {{.role_file}} -d {{.datetime}} -- {{.prompt}}"
		description:   "Agent exercising every runtime placeholder"
		default_model: "fast"
		models: {
			fast: "rt-fast-1"
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)

	stdout, stderr, err := runGetCmd(t, "runtime-rich")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}

	for _, want := range []string{"rt -m rt-fast-1", "{{.role}}", "{{.role_file}}", "{{.datetime}}", "{{.prompt}}"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q\ngot: %q", want, stdout)
		}
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout should end with a single newline, got: %q", stdout)
	}
}

// TestGetStdoutContentOnly verifies the default-mode output contract: with
// no --verbose, stdout receives only the rendered content — no Type/Name/
// Path metadata — and stderr is empty on the happy path. The contract is
// asserted independently from --quiet so a regression that drops the
// verbose gate cannot pass merely because --quiet still suppresses it.
func TestGetStdoutContentOnly(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "role-file")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "Role file contents.\n" {
		t.Errorf("stdout = %q, want exactly %q (no metadata, no extra newlines)", stdout, "Role file contents.\n")
	}
	if stderr != "" {
		t.Errorf("stderr should be empty on default-mode happy path, got: %q", stderr)
	}
}

// TestGetAllSourceFieldsFileWins covers the explicit three-field case: an
// module declaring file, prompt, AND command must emit only the file
// content. This pins the trim block (file != "" → both prompt and command
// cleared) against a regression that handled only the two-field case.
func TestGetAllSourceFieldsFileWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	roleFile := filepath.Join(dir, "all-three.md")
	if err := os.WriteFile(roleFile, []byte("FILE-WINS"), 0o644); err != nil {
		t.Fatalf("writing role file: %v", err)
	}

	cueConfig := `
roles: {
	"all-three": {
		description: "File, prompt, and command — file must win"
		file:        "` + roleFile + `"
		prompt:      "PROMPT-LOSES"
		command:     "echo COMMAND-LOSES"
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)

	stdout, stderr, err := runGetCmd(t, "all-three")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(stdout, "FILE-WINS") {
		t.Errorf("stdout should contain file content, got: %q", stdout)
	}
	for _, banned := range []string{"PROMPT-LOSES", "COMMAND-LOSES"} {
		if strings.Contains(stdout, banned) {
			t.Errorf("stdout must not contain %q when file is set, got: %q", banned, stdout)
		}
	}
}

// TestGetUTDFileMissingOnDisk covers the getUTD process error path
// (get.go: 265-267). A file source that points at a non-existent file
// must surface a descriptive error and leave stdout empty.
func TestGetUTDFileMissingOnDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	missing := filepath.Join(dir, "does-not-exist.md")
	cueConfig := `
roles: {
	"role-missing-file": {
		description: "File path that does not exist on disk"
		file:        "` + missing + `"
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)

	stdout, _, err := runGetCmd(t, "role-missing-file")
	if err == nil {
		t.Fatal("expected error when file source path does not exist")
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on file-read error, got: %q", stdout)
	}
}

// TestGetUTDTaskPromptSource verifies a prompt-source task resolves and
// renders. The fixture defines task-prompt, but no other test exercises a
// task module through get — this guards regressions where get silently
// stops handling a category.
func TestGetUTDTaskPromptSource(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "task-prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "Task body\n" {
		t.Errorf("stdout = %q, want %q", stdout, "Task body\n")
	}
}

// TestGetUTDTaskCommandSource verifies a command-source task executes its
// command and the output flows through ensureTrailingNewline. Companion to
// TestGetUTDTaskPromptSource — together they confirm tasks are not a
// dead category in the cross-resolver.
func TestGetUTDTaskCommandSource(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "task-cmd")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "task-cmd-output\n" {
		t.Errorf("stdout = %q, want %q", stdout, "task-cmd-output\n")
	}
}

// TestGetUTDFileMultilineNoExtraNewline verifies that a multi-line file
// already carrying a trailing newline is emitted verbatim — no extra blank
// line appended by ensureTrailingNewline. A double-newline regression in
// `start get foo | wc -l` would silently shift line counts for scripted
// callers.
func TestGetUTDFileMultilineNoExtraNewline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	multi := filepath.Join(dir, "multi.md")
	content := "line one\nline two\nline three\n"
	if err := os.WriteFile(multi, []byte(content), 0o644); err != nil {
		t.Fatalf("writing multi file: %v", err)
	}

	cueConfig := `
roles: {
	"role-multi": {
		description: "Multi-line file with trailing newline"
		file:        "` + multi + `"
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)

	stdout, stderr, err := runGetCmd(t, "role-multi")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != content {
		t.Errorf("stdout = %q, want %q (no extra newline)", stdout, content)
	}
}

// TestGetUTDPromptAddsTrailingNewline verifies that a prompt with no
// trailing newline gets exactly one appended on the way to stdout. Pins the
// "abc" → "abc\n" branch of ensureTrailingNewline through the full get
// pipeline.
func TestGetUTDPromptAddsTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
roles: {
	"no-newline": {
		description: "Prompt without trailing newline"
		prompt:      "no-newline-here"
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)

	stdout, stderr, err := runGetCmd(t, "no-newline")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "no-newline-here\n" {
		t.Errorf("stdout = %q, want %q (exactly one trailing newline)", stdout, "no-newline-here\n")
	}
}

// TestGetCrossCategoryFindsContext verifies a context module is reachable
// via the cross-category resolver. The other categories (agents, roles,
// tasks) all have at least one direct get test; without this case a
// regression dropping contexts from describeCategories would only surface in
// `describe` tests.
func TestGetCrossCategoryFindsContext(t *testing.T) {
	setupGetTestConfig(t)

	stdout, stderr, err := runGetCmd(t, "ctx-cmd")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
	}
	if stdout != "cmd-output\n" {
		t.Errorf("stdout = %q, want %q", stdout, "cmd-output\n")
	}
}

// TestGetAgentExplicitlyEmptyModelFlag verifies that an empty --model
// value (e.g. `start --model "" get claude`) does not trigger
// resolveModelName and the agent's default_model still wins. Regression
// guard for a "treat empty string as override" bug — getAgent gates on
// flags.Model != "" before invoking the resolver.
func TestGetAgentExplicitlyEmptyModelFlag(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--model", "", "get", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "claude-sonnet-4") {
		t.Errorf("empty --model should fall back to default_model, got: %q", stdout.String())
	}
}

// TestGetDebugFlagDoesNotPolluteStdout verifies that --debug — which
// emits diagnostic lines via debugf — never writes to stdout. The flag is
// useful for diagnosing path-expansion failures (get.go: ExpandFilePath
// debug branch) but stdout must remain pipe-clean regardless of verbosity.
func TestGetDebugFlagDoesNotPolluteStdout(t *testing.T) {
	setupGetTestConfig(t)

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--debug", "get", "role-file"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr.String())
	}
	if stdout.String() != "Role file contents.\n" {
		t.Errorf("stdout should contain only the file content under --debug, got: %q", stdout.String())
	}
}
