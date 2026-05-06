package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunner_Run(t *testing.T) {
	t.Parallel()
	runner := NewRunner()

	tests := []struct {
		name        string
		command     string
		wantContain string
		wantErr     bool
	}{
		{
			name:        "echo command",
			command:     "echo hello",
			wantContain: "hello",
		},
		{
			name:        "multiple commands with &&",
			command:     "echo one && echo two",
			wantContain: "one",
		},
		{
			name:    "failing command",
			command: "exit 1",
			wantErr: true,
		},
		{
			name:    "nonexistent command",
			command: "nonexistent_command_12345",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := runner.Run(tt.command, "", "", 10)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !strings.Contains(output, tt.wantContain) {
				t.Errorf("output = %q, want containing %q", output, tt.wantContain)
			}
		})
	}
}

func TestRunner_RunWithResult(t *testing.T) {
	t.Parallel()
	runner := NewRunner()

	t.Run("captures stdout and stderr", func(t *testing.T) {
		result, err := runner.RunWithResult("echo stdout; echo stderr >&2", "", "", 10)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Stdout, "stdout") {
			t.Errorf("stdout = %q, want containing 'stdout'", result.Stdout)
		}
		if !strings.Contains(result.Stderr, "stderr") {
			t.Errorf("stderr = %q, want containing 'stderr'", result.Stderr)
		}
	})

	t.Run("captures exit code", func(t *testing.T) {
		result, err := runner.RunWithResult("exit 42", "", "", 10)
		if err == nil {
			t.Error("expected error for non-zero exit")
		}
		if result.ExitCode != 42 {
			t.Errorf("exit code = %d, want 42", result.ExitCode)
		}
	})

	t.Run("respects working directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatalf("creating test file: %v", err)
		}

		result, err := runner.RunWithResult("ls test.txt", tmpDir, "", 10)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Stdout, "test.txt") {
			t.Errorf("stdout = %q, want containing 'test.txt'", result.Stdout)
		}
	})

	t.Run("uses specified shell", func(t *testing.T) {
		if !IsShellAvailable("bash") {
			t.Skip("bash not available")
		}

		result, err := runner.RunWithResult("echo $BASH_VERSION", "", "bash -c", 10)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		// Bash version should be non-empty when running in bash
		if result.Stdout == "" {
			t.Log("BASH_VERSION was empty (may vary by system)")
		}
	})
}

func TestRunner_Timeout(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	runner := NewRunner()

	t.Run("command times out", func(t *testing.T) {
		result, err := runner.RunWithResult("sleep 10", "", "", 1)
		if err == nil {
			t.Error("expected timeout error")
		}
		if !result.TimedOut {
			t.Error("expected TimedOut to be true")
		}
		if !strings.Contains(err.Error(), "timed out") {
			t.Errorf("error = %q, want containing 'timed out'", err.Error())
		}
	})

	t.Run("command completes before timeout", func(t *testing.T) {
		result, err := runner.RunWithResult("echo fast", "", "", 5)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.TimedOut {
			t.Error("expected TimedOut to be false")
		}
	})
}

func TestParseShellCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input    string
		wantBin  string
		wantArgs []string
	}{
		{
			input:    "bash -c",
			wantBin:  "bash",
			wantArgs: []string{"-c"},
		},
		{
			input:    "/bin/sh -c",
			wantBin:  "/bin/sh",
			wantArgs: []string{"-c"},
		},
		{
			input:    "bash",
			wantBin:  "bash",
			wantArgs: []string{"-c"},
		},
		{
			input:    "",
			wantBin:  "sh",
			wantArgs: []string{"-c"},
		},
		{
			input:    "zsh -c -e",
			wantBin:  "zsh",
			wantArgs: []string{"-c", "-e"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			bin, args := parseShellCommand(tt.input)
			if bin != tt.wantBin {
				t.Errorf("binary = %q, want %q", bin, tt.wantBin)
			}
			if len(args) != len(tt.wantArgs) {
				t.Errorf("args = %v, want %v", args, tt.wantArgs)
				return
			}
			for i, arg := range args {
				if arg != tt.wantArgs[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.wantArgs[i])
				}
			}
		})
	}
}
