// Package shell handles shell command execution with timeout support.
package shell

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// DefaultTimeout is the default command timeout in seconds.
const DefaultTimeout = 30

// Runner executes shell commands with timeout support.
type Runner struct {
	// Shell is the shell command to use (e.g., "bash -c", "sh -c").
	// If empty, auto-detection is used.
	Shell string
	// Timeout is the default timeout in seconds for commands.
	// If 0, the DefaultTimeout constant is used.
	Timeout int
}

// NewRunner creates a new shell runner with auto-detected shell.
func NewRunner() *Runner {
	return &Runner{}
}

// RunResult contains the result of a shell command execution.
type RunResult struct {
	// Stdout is the standard output of the command.
	Stdout string
	// Stderr is the standard error of the command.
	Stderr string
	// ExitCode is the exit code of the command.
	ExitCode int
	// TimedOut indicates whether the command timed out.
	TimedOut bool
}

// Run executes a command and returns the output.
// Implements the orchestration.ShellRunner interface.
func (r *Runner) Run(command, workingDir, shell string, timeout int) (string, error) {
	result, err := r.RunWithResult(command, workingDir, shell, timeout)
	if err != nil {
		return result.Stdout, err
	}
	return result.Stdout, nil
}

// RunWithResult executes a command and returns detailed result.
func (r *Runner) RunWithResult(command, workingDir, shell string, timeout int) (RunResult, error) {
	var result RunResult

	// Determine shell to use
	shellCmd := shell
	if shellCmd == "" {
		shellCmd = r.Shell
	}
	if shellCmd == "" {
		detected, err := DetectShell()
		if err != nil {
			return result, fmt.Errorf("detecting shell: %w", err)
		}
		shellCmd = detected
	}

	// Parse shell command into binary and args
	shellBin, shellArgs := parseShellCommand(shellCmd)

	// Determine timeout
	timeoutSecs := timeout
	if timeoutSecs <= 0 {
		timeoutSecs = r.Timeout
	}
	if timeoutSecs <= 0 {
		timeoutSecs = DefaultTimeout
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	// Build command: shell binary + shell args + user command
	args := append(shellArgs, command)
	cmd := exec.CommandContext(ctx, shellBin, args...)

	// Set working directory
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Unix-only: Create process group for clean termination on timeout.
	// Allows killing child processes spawned by the command.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	err := cmd.Run()

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	// Check for timeout
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		// Unix-only: Kill entire process group (negative PID) to clean up children.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return result, fmt.Errorf("command timed out after %d seconds", timeoutSecs)
	}

	// Extract exit code
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, fmt.Errorf("command failed with exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
		}
		return result, fmt.Errorf("executing command: %w", err)
	}

	return result, nil
}

// parseShellCommand splits a shell command string into binary and arguments.
// Examples:
//   - "bash -c" -> ("bash", ["-c"])
//   - "/bin/sh -c" -> ("/bin/sh", ["-c"])
//   - "bash" -> ("bash", ["-c"])
func parseShellCommand(shell string) (string, []string) {
	parts := strings.Fields(shell)
	if len(parts) == 0 {
		return "sh", []string{"-c"}
	}

	binary := parts[0]
	args := parts[1:]

	// If no args provided, default to -c for command execution
	if len(args) == 0 {
		args = []string{"-c"}
	}

	return binary, args
}
