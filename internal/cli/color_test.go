package cli

import (
	"errors"
	"testing"

	"github.com/start-cli/start/internal/fault"
)

// TestResolveColorMode covers the --color precedence rules. Cases mutate
// process-global env via t.Setenv, so they cannot run in parallel.
func TestResolveColorMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		stdoutTTY bool
		env       map[string]string
		want      bool
		wantErr   bool
	}{
		{name: "auto on tty", mode: "auto", stdoutTTY: true, want: true},
		{name: "auto off non-tty", mode: "auto", stdoutTTY: false, want: false},
		{name: "always forces on without tty", mode: "always", stdoutTTY: false, want: true},
		{name: "never disables on tty", mode: "never", stdoutTTY: true, want: false},
		{name: "NO_COLOR beats always", mode: "always", stdoutTTY: true, env: map[string]string{"NO_COLOR": "1"}, want: false},
		{name: "TERM=dumb disables auto", mode: "auto", stdoutTTY: true, env: map[string]string{"TERM": "dumb"}, want: false},
		{name: "FORCE_COLOR forces auto on without tty", mode: "auto", stdoutTTY: false, env: map[string]string{"FORCE_COLOR": "1"}, want: true},
		{name: "CLICOLOR_FORCE forces auto on without tty", mode: "auto", stdoutTTY: false, env: map[string]string{"CLICOLOR_FORCE": "1"}, want: true},
		{name: "invalid value errors", mode: "rainbow", stdoutTTY: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear the env vars the resolver consults so the host environment does not leak in.
			for _, k := range []string{"NO_COLOR", "TERM", "FORCE_COLOR", "CLICOLOR_FORCE"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := resolveColorMode(tt.mode, tt.stdoutTTY)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveColorMode err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !errors.Is(err, fault.ErrUsage) {
					t.Errorf("invalid --color should be a usage fault; got %v", err)
				}
				return
			}
			if got != tt.want {
				t.Errorf("resolveColorMode(%q, tty=%v) = %v, want %v", tt.mode, tt.stdoutTTY, got, tt.want)
			}
		})
	}
}

// TestSettleMarkdownStyle covers the default-to-dark branches. The "light"
// result requires a raw-mode background probe against a TTY stdout; the test
// process's os.Stdout is not a terminal, so settle must skip the probe entirely
// and default to dark — both when not decorating and when decorating to a
// non-TTY. This pins that settle never probes (and never hangs) off-TTY.
func TestSettleMarkdownStyle(t *testing.T) {
	tests := []struct {
		name      string
		decorated bool
		want      string
	}{
		{name: "not decorating defaults to dark", decorated: false, want: markdownStyleDark},
		{name: "decorating to non-tty stdout defaults to dark", decorated: true, want: markdownStyleDark},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := settleMarkdownStyle(tt.decorated); got != tt.want {
				t.Errorf("settleMarkdownStyle(%v) = %q, want %q", tt.decorated, got, tt.want)
			}
		})
	}
}
