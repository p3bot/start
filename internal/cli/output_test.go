package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/orchestration"
)

// TestRoleSkipOutcome asserts the --role none opt-out producer yields
// SectionSkipped naming the flag, satisfying the value-level guarantee the CLI
// branches stamp onto RoleOutcome.
func TestRoleSkipOutcome(t *testing.T) {
	got := roleSkipOutcome()
	if got.State != orchestration.SectionSkipped {
		t.Errorf("State = %v, want SectionSkipped", got.State)
	}
	if got.Reason != "--role none" {
		t.Errorf("Reason = %q, want %q", got.Reason, "--role none")
	}
}

// TestPrintRoleTable asserts the render layer is a pure switch over
// RoleOutcome.State: SectionNone and SectionSkipped print their single-line
// states, SectionListed renders the table, and the empty-state arms never leak
// the table even when resolutions are present.
func TestPrintRoleTable(t *testing.T) {
	listed := []orchestration.RoleResolution{
		{Name: "assistant", Status: "loaded", File: "assistant.md"},
	}

	tests := []struct {
		name        string
		outcome     orchestration.SectionOutcome
		resolutions []orchestration.RoleResolution
		want        []string
		notWant     []string
	}{
		{
			name:    "none",
			outcome: orchestration.SectionOutcome{State: orchestration.SectionNone},
			want:    []string{"Role: none"},
			notWant: []string{"skipped", "Name", "Status"},
		},
		{
			name:    "skipped",
			outcome: orchestration.SectionOutcome{State: orchestration.SectionSkipped, Reason: "--role none"},
			want:    []string{"Role: skipped", "(via --role none)"},
			notWant: []string{"Role: none", "Name", "Status"},
		},
		{
			name:        "listed",
			outcome:     orchestration.SectionOutcome{State: orchestration.SectionListed},
			resolutions: listed,
			want:        []string{"Role:", "assistant", "Name", "Status"},
			notWant:     []string{"Role: none", "skipped"},
		},
		{
			// A SectionNone stamp must suppress stray resolutions, not render them.
			name:        "none ignores resolutions",
			outcome:     orchestration.SectionOutcome{State: orchestration.SectionNone},
			resolutions: listed,
			want:        []string{"Role: none"},
			notWant:     []string{"assistant", "Name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			printRoleTable(buf, tt.outcome, tt.resolutions)
			out := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("expected %q in output, got:\n%s", w, out)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(out, nw) {
					t.Errorf("did not expect %q in output, got:\n%s", nw, out)
				}
			}
		})
	}
}

// TestPrintContextTable asserts the render layer is a pure switch over
// ContextOutcome.State: SectionNone and SectionSkipped print their single-line
// states, SectionListed renders the table, and the empty-state arms never leak
// the table even when contexts are present.
func TestPrintContextTable(t *testing.T) {
	listed := []orchestration.Context{
		{Name: "env", Status: "loaded", Required: true},
	}
	requiredSel := orchestration.ContextSelection{IncludeRequired: true}

	tests := []struct {
		name      string
		outcome   orchestration.SectionOutcome
		contexts  []orchestration.Context
		selection orchestration.ContextSelection
		want      []string
		notWant   []string
	}{
		{
			name:    "none",
			outcome: orchestration.SectionOutcome{State: orchestration.SectionNone},
			want:    []string{"Context: none"},
			notWant: []string{"skipped", "Name", "Status"},
		},
		{
			name:    "skipped",
			outcome: orchestration.SectionOutcome{State: orchestration.SectionSkipped, Reason: "--context none"},
			want:    []string{"Context: skipped", "(via --context none)"},
			notWant: []string{"Context: none", "Name", "Status"},
		},
		{
			name:      "listed",
			outcome:   orchestration.SectionOutcome{State: orchestration.SectionListed},
			contexts:  listed,
			selection: requiredSel,
			want:      []string{"Context:", "env", "Name", "Status", "required"},
			notWant:   []string{"Context: none", "skipped"},
		},
		{
			// A SectionNone stamp must suppress stray contexts, not render them.
			name:     "none ignores contexts",
			outcome:  orchestration.SectionOutcome{State: orchestration.SectionNone},
			contexts: listed,
			want:     []string{"Context: none"},
			notWant:  []string{"env", "Name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			printContextTable(buf, tt.contexts, tt.outcome, tt.selection)
			out := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("expected %q in output, got:\n%s", w, out)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(out, nw) {
					t.Errorf("did not expect %q in output, got:\n%s", nw, out)
				}
			}
		})
	}
}
