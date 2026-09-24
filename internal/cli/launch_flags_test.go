package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/p3bot/start/internal/fault"
)

const launchFlagConfig = `
agents: {
	plain: {
		bin: "echo"
		command: "{{.bin}} {{.prompt}}"
	}
	edit: {
		bin: "echo"
		command: "{{.bin}}{{.permission}}{{.print}}"
		flags: {
			permission: {
				default: ["--permission-mode", "default"]
				edit: ["--permission-mode", "acceptEdits"]
				bypass: ["--permission-mode", "bypassPermissions"]
				plan: ["--permission-mode", "plan"]
			}
			print: {
				off: ["--brief"]
				on: ["--print", "{{.prompt}}"]
			}
		}
	}
	talk: {
		bin: "echo"
		command: "{{.bin}}{{.resume}} {{.prompt}}"
		flags: resume: {
			latest: ["--continue"]
			id: ["--resume", "{{.resume}}"]
		}
	}
	badprint: {
		bin: "echo"
		command: "{{.bin}}{{.print}}"
		flags: print: {
			off: "--brief"
			on: ["--print", "{{.prompt}}"]
		}
	}
	levels: {
		bin: "echo"
		command: "{{.bin}}{{.effort}}{{.output}}{{.print}}"
		flags: {
			effort: {
				low: ["--effort", "low"]
				high: ["--effort", "high"]
			}
			output: {
				json: ["--output-format", "json"]
				text: ["--output-format", "text"]
			}
			print: {
				off: ["--off"]
				on: ["--on", "{{.prompt}}"]
			}
		}
	}
}

tasks: review: {
	prompt: "TASKBODY review"
}

settings: default_agent: "talk"
`

func writeLaunchFlagConfig(t *testing.T) (dir string, before []byte) {
	t.Helper()
	dir = isolateConfigEnv(t)
	chdir(t, dir)
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(startDir, "settings.cue")
	if err := os.WriteFile(path, []byte(launchFlagConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return dir, before
}

func configUnchanged(t *testing.T, dir string, before []byte) {
	t.Helper()
	after, err := os.ReadFile(filepath.Join(dir, ".start", "settings.cue"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("installed config changed:\n%s", after)
	}
	entries, err := os.ReadDir(filepath.Join(dir, ".start"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.cue" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf(".start entries = %v, want only settings.cue", names)
	}
}

func runLaunchArgs(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func acceptedLine(t *testing.T, msg string) string {
	t.Helper()
	i := strings.Index(msg, "Accepted values:")
	if i < 0 {
		t.Fatalf("no accepted list in %q", msg)
	}
	return msg[i:]
}

func TestLaunchFlags_PlainOmitsAndRejects(t *testing.T) {
	dir, before := writeLaunchFlagConfig(t)

	stdout, _, err := runLaunchArgs(t, "--agent", "plain", "prompt", "--dry-run", "hello")
	if err != nil {
		t.Fatalf("omit flags: %v", err)
	}
	cmd := dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "'hello'") {
		t.Errorf("prompt missing from %q", cmd)
	}

	rejects := []struct {
		name string
		args []string
	}{
		{"permission", []string{"--permission", "edit"}},
		{"print", []string{"--print"}},
		{"resume", []string{"--resume"}},
		{"effort", []string{"--effort", "low"}},
		{"output", []string{"--output", "json"}},
	}
	for _, tc := range rejects {
		args := append([]string{"--agent", "plain", "prompt"}, tc.args...)
		args = append(args, "hello")
		_, _, err := runLaunchArgs(t, args...)
		if !errors.Is(err, fault.ErrUsage) || ExitCodeFromError(err) != ExitUsage {
			t.Fatalf("%s: err = %v", tc.name, err)
		}
		if !strings.Contains(err.Error(), "--"+tc.name) {
			t.Fatalf("%s error = %v", tc.name, err)
		}
	}
	configUnchanged(t, dir, before)
}

func TestLaunchFlags_EditPrint(t *testing.T) {
	dir, before := writeLaunchFlagConfig(t)

	stdout, _, err := runLaunchArgs(t, "--agent", "edit", "--permission", "edit", "--print", "prompt", "--dry-run", "hello")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	cmd := dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "--permission-mode acceptEdits --print 'hello'") {
		t.Errorf("command = %q", cmd)
	}

	stdout, _, err = runLaunchArgs(t, "--agent", "edit", "--print", "prompt", "--dry-run", "hello")
	if err != nil {
		t.Fatalf("omit permission: %v", err)
	}
	cmd = dryRunCommand(t, stdout)
	if strings.Contains(cmd, "--permission-mode") || !strings.Contains(cmd, "--print 'hello'") {
		t.Errorf("omit permission = %q", cmd)
	}

	stdout, _, err = runLaunchArgs(t, "--agent", "edit", "--permission", "edit", "prompt", "--dry-run", "hello")
	if err != nil {
		t.Fatalf("omit print: %v", err)
	}
	cmd = dryRunCommand(t, stdout)
	if strings.Contains(cmd, "--print") || !strings.Contains(cmd, "--brief") || !strings.Contains(cmd, "acceptEdits") {
		t.Errorf("omit print = %q", cmd)
	}

	_, _, err = runLaunchArgs(t, "--agent", "edit", "--permission", "auto", "prompt", "hello")
	if !errors.Is(err, fault.ErrUsage) || ExitCodeFromError(err) != ExitUsage {
		t.Fatalf("auto: %v", err)
	}
	accepted := acceptedLine(t, err.Error())
	for _, key := range []string{"default", "edit", "bypass", "plan"} {
		if !strings.Contains(accepted, key) {
			t.Errorf("missing %s in %s", key, accepted)
		}
	}
	if strings.Contains(accepted, "auto") {
		t.Errorf("lists auto: %s", accepted)
	}
	configUnchanged(t, dir, before)
}

func TestLaunchFlags_ResumeDoesNotConsumeNext(t *testing.T) {
	dir, before := writeLaunchFlagConfig(t)

	stdout, _, err := runLaunchArgs(t, "prompt", "--resume", "hello", "--dry-run")
	if err != nil {
		t.Fatalf("bare resume prompt: %v", err)
	}
	cmd := dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "--continue") || !strings.Contains(cmd, "'hello'") || strings.Contains(cmd, "--resume") {
		t.Errorf("bare prompt command = %q", cmd)
	}

	stdout, _, err = runLaunchArgs(t, "prompt", "--resume=sess-1", "hello", "--dry-run")
	if err != nil {
		t.Fatalf("id prompt: %v", err)
	}
	cmd = dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "--resume 'sess-1'") || !strings.Contains(cmd, "'hello'") {
		t.Errorf("id prompt command = %q", cmd)
	}

	stdout, _, err = runLaunchArgs(t, "prompt", "--resume=true", "hello", "--dry-run")
	if err != nil {
		t.Fatalf("id true: %v", err)
	}
	cmd = dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "--resume 'true'") || strings.Contains(cmd, "--continue") || !strings.Contains(cmd, "'hello'") {
		t.Errorf("id true command = %q", cmd)
	}

	stdout, _, err = runLaunchArgs(t, "prompt", "--resume=", "hello", "--dry-run")
	if err != nil {
		t.Fatalf("empty id: %v", err)
	}
	cmd = dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "--resume ''") || !strings.Contains(cmd, "'hello'") || strings.Contains(cmd, "--continue") {
		t.Errorf("empty id command = %q", cmd)
	}

	stdout, _, err = runLaunchArgs(t, "task", "--resume", "review", "--dry-run")
	if err != nil {
		t.Fatalf("bare resume task: %v", err)
	}
	cmd = dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "--continue") || !strings.Contains(cmd, "TASKBODY") || strings.Contains(cmd, "--resume") {
		t.Errorf("bare task command = %q", cmd)
	}

	_, _, err = runLaunchArgs(t, "--agent", "plain", "prompt", "--resume", "hello")
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("plain bare resume: %v", err)
	}
	_, _, err = runLaunchArgs(t, "--agent", "plain", "prompt", "--resume=sess-1", "hello")
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "--resume") {
		t.Fatalf("plain id resume: %v", err)
	}
	configUnchanged(t, dir, before)
}

func TestLaunchFlags_OutputDoesNotRequirePrint(t *testing.T) {
	dir, before := writeLaunchFlagConfig(t)

	_, _, err := runLaunchArgs(t, "--agent", "plain", "--output", "json", "prompt", "hello")
	if !errors.Is(err, fault.ErrUsage) || ExitCodeFromError(err) != ExitUsage || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("missing output: %v", err)
	}

	stdout, _, err := runLaunchArgs(t, "--agent", "levels", "--output", "json", "prompt", "--dry-run", "hello")
	if err != nil {
		t.Fatalf("output without print: %v", err)
	}
	cmd := dryRunCommand(t, stdout)
	if !strings.Contains(cmd, "--output-format json") || !strings.Contains(cmd, "--off") {
		t.Errorf("output command = %q", cmd)
	}

	_, _, err = runLaunchArgs(t, "--agent", "levels", "--effort", "deep", "prompt", "hello")
	if !errors.Is(err, fault.ErrUsage) {
		t.Fatalf("effort: %v", err)
	}
	accepted := acceptedLine(t, err.Error())
	if !strings.Contains(accepted, "low") || !strings.Contains(accepted, "high") || strings.Contains(accepted, "deep") {
		t.Errorf("effort list = %s", accepted)
	}
	configUnchanged(t, dir, before)
}

func TestLaunchFlags_GetAndDescribe(t *testing.T) {
	dir, before := writeLaunchFlagConfig(t)

	stdout, _, err := runLaunchArgs(t, "--permission", "edit", "--print", "get", "edit")
	if err != nil {
		t.Fatalf("get: %v\n%s", err, stdout)
	}
	if stdout != "echo --permission-mode acceptEdits --print {{.prompt}}\n" {
		t.Errorf("get = %q", stdout)
	}
	if strings.Contains(stdout, "hello") || strings.Contains(stdout, "'") {
		t.Errorf("get quoted or substituted prompt: %q", stdout)
	}

	stdout, _, err = runLaunchArgs(t, "get", "edit")
	if err != nil {
		t.Fatalf("get omitted: %v", err)
	}
	if stdout != "echo --brief\n" {
		t.Errorf("get omitted = %q", stdout)
	}

	stdout, _, err = runLaunchArgs(t, "--resume=sess-1", "get", "talk")
	if err != nil {
		t.Fatalf("get resume: %v", err)
	}
	if stdout != "echo --resume sess-1 {{.prompt}}\n" {
		t.Errorf("get resume = %q", stdout)
	}

	stdout, _, err = runLaunchArgs(t, "describe", "edit")
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	for _, want := range []string{"default", "edit", "bypass", "plan", "--brief", "--print", "{{.prompt}}"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("describe missing %q\n%s", want, stdout)
		}
	}

	stdout, _, err = runLaunchArgs(t, "describe", "plain")
	if err != nil {
		t.Fatalf("describe plain: %v", err)
	}
	if strings.Contains(stdout, "Flags:") {
		t.Errorf("plain describe listed flags:\n%s", stdout)
	}

	_, _, err = runLaunchArgs(t, "--permission", "auto", "get", "edit")
	if !errors.Is(err, fault.ErrUsage) || ExitCodeFromError(err) != ExitUsage {
		t.Fatalf("get reject: %v", err)
	}
	_, _, err = runLaunchArgs(t, "--permission", "auto", "describe", "edit")
	if !errors.Is(err, fault.ErrUsage) {
		t.Fatalf("describe reject: %v", err)
	}
	configUnchanged(t, dir, before)
}

func TestLaunchFlags_BrokenPrintPrintsNoCommand(t *testing.T) {
	dir, before := writeLaunchFlagConfig(t)

	stdout, _, err := runLaunchArgs(t, "get", "badprint")
	if !errors.Is(err, fault.ErrUsage) || ExitCodeFromError(err) != ExitUsage {
		t.Fatalf("get: %v", err)
	}
	if strings.Contains(stdout, "echo") || strings.Contains(stdout, "--brief") {
		t.Errorf("get printed a command: %q", stdout)
	}

	stdout, _, err = runLaunchArgs(t, "describe", "badprint")
	if !errors.Is(err, fault.ErrUsage) || ExitCodeFromError(err) != ExitUsage {
		t.Fatalf("describe: %v", err)
	}
	if strings.Contains(stdout, "Command:") || strings.Contains(stdout, "--brief") || strings.Contains(stdout, "echo") {
		t.Errorf("describe printed a command: %q", stdout)
	}

	_, _, err = runLaunchArgs(t, "--agent", "badprint", "prompt", "hello")
	if !errors.Is(err, fault.ErrUsage) {
		t.Fatalf("launch: %v", err)
	}
	configUnchanged(t, dir, before)
}

func TestLaunchFlags_ListIgnoresUnknownValue(t *testing.T) {
	dir, before := writeLaunchFlagConfig(t)
	if _, _, err := runLaunchArgs(t, "list", "--permission", "not-a-mode"); err != nil {
		t.Fatalf("list should ignore launch flags: %v", err)
	}
	configUnchanged(t, dir, before)
}

func TestRootHelp_ResumeIsBareSwitch(t *testing.T) {
	cmd := NewRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertResumeHelp(t, stdout.String())

	cmd = NewRootCmd()
	stdout.Reset()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"prompt", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertResumeHelp(t, stdout.String())
}

func assertResumeHelp(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, resumeBareSentinel) {
		t.Fatalf("help contains the bare-flag stand-in:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "--resume") {
			continue
		}
		if strings.Contains(line, "[=") || strings.Contains(line, "--resume id") {
			t.Errorf("resume help = %q", line)
		}
		synopsis, _, _ := strings.Cut(strings.TrimSpace(line), "  ")
		if synopsis != "--resume" {
			t.Errorf("resume synopsis = %q, want --resume", synopsis)
		}
		return
	}
	t.Fatalf("help missing --resume:\n%s", out)
}
