package shell

import (
	"fmt"
	"os/exec"
)

// DetectShell finds an available Unix shell in PATH, preferring bash over sh.
func DetectShell() (string, error) {
	if path, err := exec.LookPath("bash"); err == nil {
		return path + " -c", nil
	}

	if path, err := exec.LookPath("sh"); err == nil {
		return path + " -c", nil
	}

	return "", fmt.Errorf("no shell found in PATH (tried bash, sh)")
}

// IsShellAvailable reports whether the named shell is in PATH.
func IsShellAvailable(shell string) bool {
	_, err := exec.LookPath(shell)
	return err == nil
}
