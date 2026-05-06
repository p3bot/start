package shell

import (
	"fmt"
	"os/exec"
)

// DetectShell finds an available Unix shell in PATH.
// Prefers bash, falls back to sh. Windows shells (cmd.exe, PowerShell)
// are not supported.
func DetectShell() (string, error) {
	// Try bash first
	if path, err := exec.LookPath("bash"); err == nil {
		return path + " -c", nil
	}

	// Fall back to sh
	if path, err := exec.LookPath("sh"); err == nil {
		return path + " -c", nil
	}

	return "", fmt.Errorf("no shell found in PATH (tried bash, sh)")
}

// IsShellAvailable checks if a specific shell is available.
func IsShellAvailable(shell string) bool {
	_, err := exec.LookPath(shell)
	return err == nil
}
