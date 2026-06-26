package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Install validates its 3-character floor against the name only, excluding any
// "category:" prefix, and rejects an unknown category. Both checks run before
// the registry index is fetched, so the command fails fast with a usage error
// in the non-interactive test environment.
func TestInstallCommandValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		query   string
		wantSub string
	}{
		{"short bare query", "go", "3 characters"},
		{"short scoped name", "roles:go", "3 characters"},
		{"unknown category prefix", "bogus:golang", "unknown category"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := NewRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetIn(&bytes.Buffer{}) // non-terminal stdin → fail fast, no prompt
			cmd.SetArgs([]string{"install", tc.query})

			err := cmd.Execute()
			if err == nil {
				t.Fatalf("expected error for query %q", tc.query)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q should contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}
