package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestExecute_Help(t *testing.T) {
	t.Parallel()
	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(--help) error = %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("Expected help output, got empty string")
	}
	if !strings.Contains(output, "start") {
		t.Error("Expected help output to contain 'start'")
	}
}

func TestExecute_Version(t *testing.T) {
	t.Parallel()
	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("Execute(--version) error = %v", err)
	}

	output := buf.String()

	// Should contain version line
	if !strings.Contains(output, "start version") {
		t.Errorf("Expected 'start version' in output, got: %s", output)
	}

	// Should contain repository URL
	if !strings.Contains(output, "https://github.com/grantcarthew/start") {
		t.Errorf("Expected repository URL in output, got: %s", output)
	}

	// Should contain issues URL
	if !strings.Contains(output, "/issues/new") {
		t.Errorf("Expected issues URL in output, got: %s", output)
	}
}

// TestHelpArgLeafCommands verifies that "help" as a positional argument works
// on leaf commands (those with no subcommands) the same as --help.
// This covers all 17 commands updated with noArgsOrHelp in place of cobra.NoArgs.
func TestHelpArgLeafCommands(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
		want string // expected substring in help output
	}{
		// assets subcommands
		{
			name: "assets browse help",
			args: []string{"assets", "browse", "help"},
			want: "browser",
		},
		{
			name: "assets index help",
			args: []string{"assets", "index", "help"},
			want: "asset catalog",
		},
		{
			name: "assets list help",
			args: []string{"assets", "list", "help"},
			want: "installed",
		},
		{
			name: "assets validate help",
			args: []string{"assets", "validate", "help"},
			want: "git tags",
		},
		// completion subcommands
		{
			name: "completion bash help",
			args: []string{"completion", "bash", "help"},
			want: "bash",
		},
		{
			name: "completion zsh help",
			args: []string{"completion", "zsh", "help"},
			want: "zsh",
		},
		{
			name: "completion fish help",
			args: []string{"completion", "fish", "help"},
			want: "fish",
		},
		// config verb commands
		{
			name: "config add help",
			args: []string{"config", "add", "help"},
			want: "interactively",
		},
		{
			name: "config edit help",
			args: []string{"config", "edit", "help"},
			want: "Search by name",
		},
		{
			name: "config remove help",
			args: []string{"config", "remove", "help"},
			want: "Remove an agent",
		},
		{
			name: "config list help",
			args: []string{"config", "list", "help"},
			want: "category",
		},
		{
			name: "config info help",
			args: []string{"config", "info", "help"},
			want: "raw stored",
		},
		// config order commands
		{
			name: "config order help",
			args: []string{"config", "order", "help"},
			want: "Reorder",
		},
		{
			name: "config export help",
			args: []string{"config", "export", "help"},
			want: "Output raw CUE",
		},
		{
			name: "config open help",
			args: []string{"config", "open", "help"},
			want: "EDITOR",
		},
		{
			name: "config search help",
			args: []string{"config", "search", "help"},
			want: "installed assets",
		},
		{
			name: "config settings help",
			args: []string{"config", "settings", "help"},
			want: "settings",
		},
		// top-level commands
		{
			name: "search help",
			args: []string{"search", "help"},
			want: "registry",
		},
		{
			name: "task help",
			args: []string{"task", "help"},
			want: "List configured tasks",
		},
		{
			name: "doctor help",
			args: []string{"doctor", "help"},
			want: "Check start installation",
		},
		// assets subcommands
		{
			name: "assets add help",
			args: []string{"assets", "add", "help"},
			want: "Install",
		},
		{
			name: "assets search help",
			args: []string{"assets", "search", "help"},
			want: "registry index",
		},
		{
			name: "assets info help",
			args: []string{"assets", "info", "help"},
			want: "asset",
		},
		{
			name: "assets update help",
			args: []string{"assets", "update", "help"},
			want: "Update installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err != nil {
				t.Fatalf("%v: unexpected error: %v", tt.args, err)
			}
			if !strings.Contains(buf.String(), tt.want) {
				t.Errorf("%v: expected %q in output, got:\n%s", tt.args, tt.want, buf.String())
			}
		})
	}
}

// TestNoArgsOrHelpRejectsInvalidArgs verifies that noArgsOrHelp still rejects
// positional arguments other than a lone "help", preserving cobra.NoArgs behaviour.
func TestNoArgsOrHelpRejectsInvalidArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "unknown positional arg",
			args: []string{"assets", "browse", "unexpected"},
		},
		{
			name: "help plus extra arg",
			args: []string{"assets", "browse", "help", "extra"},
		},
		{
			name: "multiple unknown args",
			args: []string{"assets", "browse", "foo", "bar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Errorf("%v: expected error for invalid args, got nil", tt.args)
			}
		})
	}
}

func TestDebugImpliesVerbose(t *testing.T) {
	t.Parallel()
	var capturedFlags *Flags
	cmd := NewRootCmd()
	// Add a probe subcommand so PersistentPreRunE runs and we can inspect flags.
	probe := &cobra.Command{
		Use:   "probe",
		Short: "test probe",
		RunE: func(cmd *cobra.Command, args []string) error {
			capturedFlags = getFlags(cmd)
			return nil
		},
	}
	cmd.AddCommand(probe)
	cmd.SetArgs([]string{"--debug", "probe"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if capturedFlags == nil {
		t.Fatal("probe RunE was not called; PersistentPreRunE did not run")
	}
	if !capturedFlags.Verbose {
		t.Error("--debug should set Verbose=true, but Verbose was false")
	}
}
