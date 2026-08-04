package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/p3bot/start/internal/registry"
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

// TestInstallCommandMatchesDescriptionAndTags asserts install still surfaces
// description-only and tag-only matches, not just name matches — the regex/tag
// matcher (MatchSearch) install shares with search over the unified candidate
// set. A query that hits one registry entry only by description and another only
// by tag yields two candidates; the non-interactive selection path lists both,
// proving each surfaced through the command, not just the modules-package unit.
func TestInstallCommandMatchesDescriptionAndTags(t *testing.T) {
	idx := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"alpha/widget": {
				Module:      "github.com/p3bot/library/agents/alpha/widget@v1",
				Description: "a searchable helper",
				Version:     stubVersion,
			},
		},
		Roles: map[string]registry.IndexEntry{
			"beta/helper": {
				Module:      "github.com/p3bot/library/roles/beta/helper@v1",
				Description: "an unrelated role",
				Version:     stubVersion,
				Tags:        []string{"searchable"},
			},
		},
		// LoadIndex initialises every category, so the round-trip guard in
		// setupStartTestConfigWithRegistry requires non-nil maps for the empty ones.
		Contexts: map[string]registry.IndexEntry{},
		Tasks:    map[string]registry.IndexEntry{},
	}
	_, stub := setupStartTestConfigWithRegistry(t, idx)

	// "searchable" matches neither name; it hits the agent by description and the
	// role by tag. With non-interactive stdin the two matches drop to the
	// "multiple modules found" listing, so both surfaced candidates appear there.
	_, _, err := captureStreams(t, stub, "install", "searchable")
	if err == nil {
		t.Fatal("expected a multiple-match error listing both surfaced candidates")
	}
	msg := err.Error()
	if !strings.Contains(msg, "agents:alpha/widget") {
		t.Errorf("description-only match not surfaced as an install candidate: %v", msg)
	}
	if !strings.Contains(msg, "roles:beta/helper") {
		t.Errorf("tag-only match not surfaced as an install candidate: %v", msg)
	}
}
