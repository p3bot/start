package shell

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectShell(t *testing.T) {
	t.Parallel()
	shell, err := DetectShell()
	if err != nil {
		t.Fatalf("DetectShell() error = %v", err)
	}

	if !strings.HasSuffix(shell, " -c") {
		t.Errorf("DetectShell() = %q, want ending with ' -c'", shell)
	}

	if !strings.Contains(shell, "bash") && !strings.Contains(shell, "sh") {
		t.Errorf("DetectShell() = %q, want containing 'bash' or 'sh'", shell)
	}
}

func TestDetectShell_FallbackToSh(t *testing.T) {
	// Create a temp dir containing only "sh" (no "bash") and override PATH.
	// Temp dir with only "sh" (no "bash") overrides PATH to force the sh fallback.
	dir := t.TempDir()
	shPath := filepath.Join(dir, "sh")
	if err := os.WriteFile(shPath, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)

	shell, err := DetectShell()
	if err != nil {
		t.Fatalf("DetectShell() error = %v (expected sh fallback)", err)
	}
	if !strings.Contains(shell, "sh") {
		t.Errorf("DetectShell() = %q, want containing 'sh'", shell)
	}
	if !strings.HasSuffix(shell, " -c") {
		t.Errorf("DetectShell() = %q, want ending with ' -c'", shell)
	}
}

func TestDetectShell_NoShell(t *testing.T) {
	// Empty PATH so neither bash nor sh is found.
	dir := t.TempDir()
	t.Setenv("PATH", dir)

	_, err := DetectShell()
	if err == nil {
		t.Error("DetectShell() should return error when no shell is in PATH")
	}
	if !strings.Contains(err.Error(), "no shell") {
		t.Errorf("error = %q, want containing 'no shell'", err.Error())
	}
}

func TestIsShellAvailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		shell string
		want  bool
	}{
		{"bash", true},
		{"sh", true},
		{"nonexistent_shell_12345", false},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			got := IsShellAvailable(tt.shell)
			if tt.shell == "bash" && !got {
				t.Skip("bash not available on this system")
			}
			if got != tt.want {
				t.Errorf("IsShellAvailable(%q) = %v, want %v", tt.shell, got, tt.want)
			}
		})
	}
}
