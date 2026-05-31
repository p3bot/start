package cli

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestIsNoneToken(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"none", true},
		{"nil", true},
		{"off", true},
		{"0", true},
		{"NONE", true},
		{"Off", true},
		{"  none  ", true},
		{"", false},
		{"  ", false},
		{"00", false},
		{"nones", false},
		{"go-expert", false},
		{"./none.md", false},
	}
	for _, tc := range cases {
		if got := isNoneToken(tc.in); got != tc.want {
			t.Errorf("isNoneToken(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestResolveContextSkip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		in       []string
		wantSkip bool
		wantRest []string
	}{
		{"empty", nil, false, nil},
		{"sole none", []string{"none"}, true, nil},
		{"sole alias", []string{"off"}, true, nil},
		{"mixed-case none", []string{"NONE"}, true, nil},
		{"real selectors only", []string{"project", "readme"}, false, []string{"project", "readme"}},
		{"none then real", []string{"none", "project"}, true, []string{"project"}},
		{"real then none", []string{"project", "off"}, true, []string{"project"}},
		{"none keeps multiple selectors", []string{"nil", "project", "readme"}, true, []string{"project", "readme"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			skip, rest := resolveContextSkip(tc.in)
			if skip != tc.wantSkip {
				t.Errorf("resolveContextSkip(%v) skip = %v, want %v", tc.in, skip, tc.wantSkip)
			}
			if !slices.Equal(rest, tc.wantRest) {
				t.Errorf("resolveContextSkip(%v) rest = %v, want %v", tc.in, rest, tc.wantRest)
			}
		})
	}
}

// TestRoleNoneTokensSkipRole verifies --role with each none-token spelling
// (canonical, aliases, mixed-case) skips role assignment for both start and task,
// matching the former --no-role behaviour.
func TestRoleNoneTokensSkipRole(t *testing.T) {
	tokens := []string{"none", "nil", "off", "0", "NONE"}
	commands := map[string][]string{
		"start": {"--dry-run", "--role"},
		"task":  {"task", "test-task", "--dry-run", "--role"},
	}
	for cmdName, base := range commands {
		for _, token := range tokens {
			t.Run(cmdName+"/"+token, func(t *testing.T) {
				chdir(t, setupStartTestConfig(t))

				cmd := NewRootCmd()
				stdout := new(bytes.Buffer)
				cmd.SetOut(stdout)
				cmd.SetErr(new(bytes.Buffer))
				cmd.SetIn(strings.NewReader(""))
				cmd.SetArgs(append(append([]string{}, base...), token))

				if err := cmd.Execute(); err != nil {
					t.Fatalf("execute: %v", err)
				}
				if strings.Contains(stdout.String(), "You are a helpful assistant") {
					t.Errorf("--role %s should suppress role content, got:\n%s", token, stdout.String())
				}
			})
		}
	}
}

// TestContextNoneDropsRequiredContext verifies --context none drops every
// context, including a context marked required: true that bare start injects.
func TestContextNoneDropsRequiredContext(t *testing.T) {
	for _, token := range []string{"none", "nil", "off", "0", "NONE"} {
		t.Run(token, func(t *testing.T) {
			chdir(t, setupStartTestConfig(t))

			cmd := NewRootCmd()
			stdout := new(bytes.Buffer)
			cmd.SetOut(stdout)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetIn(strings.NewReader(""))
			cmd.SetArgs([]string{"--dry-run", "--context", token})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			for line := range strings.SplitSeq(stdout.String(), "\n") {
				if strings.Contains(line, "env") && strings.Contains(line, "✓") {
					t.Errorf("--context %s should drop the required 'env' context, got line: %s", token, line)
				}
			}
		})
	}
}

// TestTaskContextNoneResets verifies the none sentinel reaches the task path
// too: --context none drops the required 'env' context a bare task loads, and
// --context none,project keeps 'project' while still dropping 'env'.
func TestTaskContextNoneResets(t *testing.T) {
	cases := []struct {
		name        string
		ctx         string
		wantProject bool
	}{
		{"sole none drops everything", "none", false},
		{"none with selector keeps it", "none,project", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chdir(t, setupStartTestConfig(t))

			cmd := NewRootCmd()
			stdout := new(bytes.Buffer)
			cmd.SetOut(stdout)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetIn(strings.NewReader(""))
			cmd.SetArgs([]string{"task", "test-task", "--dry-run", "--context", tc.ctx})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}

			envLoaded, projectLoaded := false, false
			for line := range strings.SplitSeq(stdout.String(), "\n") {
				if !strings.Contains(line, "✓") {
					continue
				}
				if strings.Contains(line, "env") {
					envLoaded = true
				}
				if strings.Contains(line, "project") {
					projectLoaded = true
				}
			}
			if envLoaded {
				t.Errorf("task --context %s should drop required 'env', got:\n%s", tc.ctx, stdout.String())
			}
			if projectLoaded != tc.wantProject {
				t.Errorf("task --context %s: project loaded = %v, want %v\n%s", tc.ctx, projectLoaded, tc.wantProject, stdout.String())
			}
		})
	}
}

// TestContextNoneSanityRequiredLoadsByDefault guards the negative test above:
// without --context none, bare start must inject the required 'env' context.
func TestContextNoneSanityRequiredLoadsByDefault(t *testing.T) {
	chdir(t, setupStartTestConfig(t))

	cmd := NewRootCmd()
	stdout := new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	loaded := false
	for line := range strings.SplitSeq(stdout.String(), "\n") {
		if strings.Contains(line, "env") && strings.Contains(line, "✓") {
			loaded = true
			break
		}
	}
	if !loaded {
		t.Fatalf("bare start should inject the required 'env' context, got:\n%s", stdout.String())
	}
}

// TestContextNoneWithSelectorResets verifies that combining none with a real
// selector suppresses the implicit required/default contexts yet still loads the
// named selector: --context none,project drops required 'env' but keeps
// 'project'. Both comma-joined and repeated-flag spellings behave identically.
func TestContextNoneWithSelectorResets(t *testing.T) {
	cases := map[string][]string{
		"comma-joined":   {"--dry-run", "--context", "none,project"},
		"repeated flags": {"--dry-run", "--context", "none", "--context", "project"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			chdir(t, setupStartTestConfig(t))

			cmd := NewRootCmd()
			stdout := new(bytes.Buffer)
			cmd.SetOut(stdout)
			cmd.SetErr(new(bytes.Buffer))
			cmd.SetIn(strings.NewReader(""))
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}

			envLoaded, projectLoaded := false, false
			for line := range strings.SplitSeq(stdout.String(), "\n") {
				if !strings.Contains(line, "✓") {
					continue
				}
				if strings.Contains(line, "env") {
					envLoaded = true
				}
				if strings.Contains(line, "project") {
					projectLoaded = true
				}
			}
			if envLoaded {
				t.Errorf("none should drop the required 'env' context, got:\n%s", stdout.String())
			}
			if !projectLoaded {
				t.Errorf("none,project should still load 'project', got:\n%s", stdout.String())
			}
		})
	}
}

// TestNoRoleFlagRemoved verifies the --no-role flag is gone: invoking it is an
// unknown-flag usage error (exit 2) via the FlagErrorFunc path.
func TestNoRoleFlagRemoved(t *testing.T) {
	chdir(t, setupStartTestConfig(t))

	cmd := NewRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs([]string{"--no-role", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown-flag error, got nil")
	}
	if got := ExitCodeFromError(err); got != ExitUsage {
		t.Errorf("exit code = %d (err=%v), want %d", got, err, ExitUsage)
	}
}
