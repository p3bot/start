package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestValidateScopeFlags guards the scope-exclusion helper independently of any
// command's RunE wiring. Both-set is a usage fault (exit 2); all else permitted.
func TestValidateScopeFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		local    bool
		global   bool
		wantErr  bool
		wantExit int
	}{
		{"neither", false, false, false, ExitSuccess},
		{"local only", true, false, false, ExitSuccess},
		{"global only", false, true, false, ExitSuccess},
		{"both is usage error", true, true, true, ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateScopeFlags(&Flags{Local: tt.local, Global: tt.global})
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateScopeFlags(local=%v, global=%v) err = %v, wantErr %v", tt.local, tt.global, err, tt.wantErr)
			}
			if got := ExitCodeFromError(err); got != tt.wantExit {
				t.Errorf("exit code = %d, want %d", got, tt.wantExit)
			}
		})
	}
}

// TestScopeFlagsMutualExclusion guards the per-command wiring of
// validateScopeFlags: it replaced declarative MarkFlagsMutuallyExclusive with an
// explicit RunE call, so every --global command must invoke it. Each rejects
// --local --global with exit 2 and an empty stdout, matching the --json contract.
func TestScopeFlagsMutualExclusion(t *testing.T) {
	commands := map[string][]string{
		"describe":   {"describe", "echo", "--local", "--global"},
		"get":        {"get", "echo", "--local", "--global"},
		"config get": {"config", "get", "echo", "--local", "--global"},
	}

	for name, args := range commands {
		t.Run(name, func(t *testing.T) {
			chdir(t, setupStartTestConfig(t))

			cmd := NewRootCmd()
			stdout := &bytes.Buffer{}
			cmd.SetOut(stdout)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("%s: expected mutual-exclusion error, got nil", name)
			}
			// Pin to the mutual-exclusion error so the test cannot pass for an
			// unrelated reason that happens to share an exit code.
			if msg := err.Error(); !strings.Contains(msg, "local") || !strings.Contains(msg, "global") {
				t.Errorf("%s: error should name both flags, got: %v", name, err)
			}
			if got := ExitCodeFromError(err); got != ExitUsage {
				t.Errorf("%s: exit code = %d (err=%v), want %d", name, got, err, ExitUsage)
			}
			if stdout.String() != "" {
				t.Errorf("%s: expected empty stdout on usage error, got %q", name, stdout.String())
			}
		})
	}
}
