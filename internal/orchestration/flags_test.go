package orchestration

import (
	"errors"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/p3bot/start/internal/fault"
)

func loadFlagAgent(t *testing.T, src, name string) Agent {
	t.Helper()
	v := cuecontext.New().CompileString(src)
	if err := v.Err(); err != nil {
		t.Fatalf("compile: %v", err)
	}
	agentVal := v.LookupPath(cue.ParsePath("agents")).LookupPath(cue.MakePath(cue.Str(name)))
	if !agentVal.Exists() {
		t.Fatalf("agent %q missing", name)
	}
	return AgentFromValue(agentVal, name)
}

func TestCommandFragments_LaunchQuoting(t *testing.T) {
	t.Parallel()
	agent := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.permission}}{{.print}}"
	flags: {
		permission: {
			edit: ["--permission-mode", "acceptEdits"]
			none: []
		}
		print: {
			off: ["--brief"]
			on: ["--print", "{{.prompt}}"]
		}
	}
}
`, "edit")

	exec := NewExecutor(t.TempDir())
	launch := LaunchFlags{PermissionSet: true, Permission: "edit", Print: true}
	cmd, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "hello",
		Launch: launch,
	})
	if err != nil {
		t.Fatalf("BuildCommand: %v", err)
	}
	want := "'echo' --permission-mode acceptEdits --print 'hello'"
	if cmd != want {
		t.Errorf("command = %q, want %q", cmd, want)
	}

	omittedPerm, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "hello",
		Launch: LaunchFlags{Print: true},
	})
	if err != nil {
		t.Fatalf("omit permission: %v", err)
	}
	if strings.Contains(omittedPerm, "--permission-mode") || !strings.Contains(omittedPerm, "--print 'hello'") {
		t.Errorf("omitted permission = %q", omittedPerm)
	}

	omittedPrint, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "hello",
		Launch: LaunchFlags{PermissionSet: true, Permission: "edit"},
	})
	if err != nil {
		t.Fatalf("omit print: %v", err)
	}
	if strings.Contains(omittedPrint, "--print") || !strings.Contains(omittedPrint, "--brief") {
		t.Errorf("omitted print = %q", omittedPrint)
	}

	empty, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "hello",
		Launch: LaunchFlags{PermissionSet: true, Permission: "none", Print: true},
	})
	if err != nil {
		t.Fatalf("empty word list: %v", err)
	}
	if strings.Contains(empty, "--permission-mode") {
		t.Errorf("empty permission list inserted words: %q", empty)
	}
}

func TestCommandFragments_ResumeAndReject(t *testing.T) {
	t.Parallel()
	agent := loadFlagAgent(t, `
agents: talk: {
	bin: "echo"
	command: "{{.bin}}{{.resume}} {{.prompt}}"
	flags: resume: {
		latest: ["--continue"]
		id: ["--resume", "{{.resume}}"]
	}
}
`, "talk")
	exec := NewExecutor(t.TempDir())

	latest, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "hello",
		Launch: LaunchFlags{Resume: ResumeLatest},
	})
	if err != nil {
		t.Fatalf("latest: %v", err)
	}
	if latest != "'echo' --continue 'hello'" {
		t.Errorf("latest = %q", latest)
	}

	withID, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "hello",
		Launch: LaunchFlags{Resume: ResumeID, ResumeID: "sess-1"},
	})
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	if withID != "'echo' --resume 'sess-1' 'hello'" {
		t.Errorf("id = %q", withID)
	}

	quoted, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "it's",
		Launch: LaunchFlags{Resume: ResumeID, ResumeID: "o'brien"},
	})
	if err != nil {
		t.Fatalf("quote: %v", err)
	}
	if !strings.Contains(quoted, `'o'"'"'brien'`) || !strings.Contains(quoted, `'it'"'"'s'`) {
		t.Errorf("quoting = %q", quoted)
	}

	emptyID, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "hello",
		Launch: LaunchFlags{Resume: ResumeID, ResumeID: ""},
	})
	if err != nil {
		t.Fatalf("empty id: %v", err)
	}
	if emptyID != "'echo' --resume '' 'hello'" {
		t.Errorf("empty id = %q", emptyID)
	}

	plain := loadFlagAgent(t, `
agents: plain: {
	bin: "echo"
	command: "{{.bin}} {{.prompt}}"
}
`, "plain")
	ok, err := exec.BuildCommand(ExecuteConfig{Agent: plain, Prompt: "hello"})
	if err != nil {
		t.Fatalf("omitted flags: %v", err)
	}
	if ok != "'echo' 'hello'" {
		t.Errorf("plain = %q", ok)
	}

	_, err = exec.BuildCommand(ExecuteConfig{
		Agent:  plain,
		Prompt: "hello",
		Launch: LaunchFlags{PermissionSet: true, Permission: "edit"},
	})
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "--permission") {
		t.Fatalf("missing table: %v", err)
	}
	// Usage wins over a missing binary: the process is not started, and the
	// error is not the PATH failure.
	plain.Bin = "start-missing-bin-xyz"
	_, err = exec.BuildCommand(ExecuteConfig{
		Agent:  plain,
		Launch: LaunchFlags{OutputSet: true, Output: "json"},
	})
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "--output") {
		t.Fatalf("usage before lookpath: %v", err)
	}
}

func TestCommandFragments_UnlistedValue(t *testing.T) {
	t.Parallel()
	agent := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.permission}}{{.effort}}"
	flags: {
		permission: {
			default: ["--permission-mode", "default"]
			edit: ["--permission-mode", "acceptEdits"]
			bypass: ["--permission-mode", "bypassPermissions"]
			plan: ["--permission-mode", "plan"]
		}
		effort: {
			low: ["--effort", "low"]
			high: ["--effort", "high"]
		}
	}
}
`, "edit")
	_, err := agent.CommandFragments(LaunchFlags{PermissionSet: true, Permission: "auto"}, "", FragmentLaunch)
	if !errors.Is(err, fault.ErrUsage) {
		t.Fatalf("err = %v", err)
	}
	msg := err.Error()
	accepted := msg[strings.Index(msg, "Accepted values:"):]
	for _, key := range []string{"default", "edit", "bypass", "plan"} {
		if !strings.Contains(accepted, key) {
			t.Errorf("accepted list missing %s: %s", key, accepted)
		}
	}
	if strings.Contains(accepted, "auto") {
		t.Errorf("accepted list includes auto: %s", accepted)
	}

	_, err = agent.CommandFragments(LaunchFlags{EffortSet: true, Effort: "deep"}, "", FragmentLaunch)
	if err == nil || !strings.Contains(err.Error(), "low") || !strings.Contains(err.Error(), "high") {
		t.Fatalf("effort: %v", err)
	}
	effortAccepted := err.Error()
	if i := strings.Index(effortAccepted, "Accepted values:"); i >= 0 {
		effortAccepted = effortAccepted[i:]
	}
	if strings.Contains(effortAccepted, "deep") {
		t.Errorf("effort accepted list includes deep: %s", effortAccepted)
	}
}

func TestFillAgentCommandForDisplay_FlagSlots(t *testing.T) {
	t.Parallel()
	agent := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.permission}}{{.print}}{{.resume}}"
	flags: {
		permission: edit: ["--permission-mode", "acceptEdits"]
		print: {
			off: ["--brief"]
			on: ["--print", "{{.prompt}}"]
		}
		resume: {
			latest: ["--continue"]
			id: ["--resume", "{{.resume}}"]
		}
	}
}
`, "edit")

	got, err := FillAgentCommandForDisplay(agent.Command, "echo", "", agent, LaunchFlags{
		PermissionSet: true,
		Permission:    "edit",
		Print:         true,
		Resume:        ResumeID,
		ResumeID:      "sess-1",
	})
	if err != nil {
		t.Fatalf("display: %v", err)
	}
	want := "echo --permission-mode acceptEdits --print {{.prompt}} --resume sess-1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	off, err := FillAgentCommandForDisplay(agent.Command, "echo", "", agent, LaunchFlags{})
	if err != nil {
		t.Fatalf("off: %v", err)
	}
	if off != "echo --brief" {
		t.Errorf("omitted = %q", off)
	}

	emptyShown, err := FillAgentCommandForDisplay(agent.Command, "echo", "", agent, LaunchFlags{
		Resume:   ResumeID,
		ResumeID: "",
	})
	if err != nil {
		t.Fatalf("empty id display: %v", err)
	}
	if emptyShown != "echo --brief --resume ''" {
		t.Errorf("empty id display = %q", emptyShown)
	}
}

func TestCommandFragments_EmptyPrintPrompt(t *testing.T) {
	t.Parallel()
	agent := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.print}}"
	flags: print: {
		off: ["--brief"]
		on: ["--print", "{{.prompt}}"]
	}
}
`, "edit")
	exec := NewExecutor(t.TempDir())
	got, err := exec.BuildCommand(ExecuteConfig{
		Agent:  agent,
		Prompt: "",
		Launch: LaunchFlags{Print: true},
	})
	if err != nil {
		t.Fatalf("empty prompt: %v", err)
	}
	if got != "'echo' --print ''" {
		t.Errorf("empty prompt = %q", got)
	}
}

func TestCommandFragments_DecodeErrors(t *testing.T) {
	t.Parallel()
	exec := NewExecutor(t.TempDir())

	badPrint := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.print}}"
	flags: print: {
		off: "--brief"
		on: ["--print", "{{.prompt}}"]
	}
}
`, "edit")
	_, err := exec.BuildCommand(ExecuteConfig{Agent: badPrint, Prompt: "hello"})
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "print") {
		t.Fatalf("bad print off: %v", err)
	}
	if _, err := FillAgentCommandForDisplay(badPrint.Command, "echo", "", badPrint, LaunchFlags{}); err == nil {
		t.Fatal("display of a broken print table returned a command")
	}

	mixed := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.print}}{{.effort}}"
	flags: {
		print: {
			off: ["--brief"]
			on: ["--on"]
		}
		effort: low: "deep"
	}
}
`, "edit")
	cmd, err := exec.BuildCommand(ExecuteConfig{Agent: mixed, Prompt: "hello"})
	if err != nil {
		t.Fatalf("unused broken effort must still launch: %v", err)
	}
	if !strings.Contains(cmd, "--brief") {
		t.Errorf("print off missing: %q", cmd)
	}
	_, err = mixed.CommandFragments(LaunchFlags{EffortSet: true, Effort: "low"}, "", FragmentLaunch)
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "effort") {
		t.Fatalf("effort decode: %v", err)
	}

	notStruct := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.print}}"
	flags: "nope"
}
`, "edit")
	_, err = exec.BuildCommand(ExecuteConfig{Agent: notStruct, Prompt: "hello"})
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "struct") {
		t.Fatalf("flags not a struct: %v", err)
	}
}

func TestCommandFragments_BadPlaceholder(t *testing.T) {
	t.Parallel()
	agent := loadFlagAgent(t, `
agents: edit: {
	bin: "echo"
	command: "{{.bin}}{{.permission}}"
	flags: permission: edit: ["--permission-mode", "{{.model}}"]
}
`, "edit")
	_, err := agent.CommandFragments(LaunchFlags{PermissionSet: true, Permission: "edit"}, "", FragmentLaunch)
	if !errors.Is(err, fault.ErrUsage) || !strings.Contains(err.Error(), "{{.model}}") {
		t.Fatalf("err = %v", err)
	}
}
