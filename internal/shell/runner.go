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
	// Shell is the shell command (e.g., "bash -c"); empty means auto-detect.
	Shell string
	// Timeout is the default timeout in seconds; 0 means DefaultTimeout.
	Timeout int
}

// NewRunner creates a new shell runner with auto-detected shell.
func NewRunner() *Runner {
	return &Runner{}
}

// RunResult contains the result of a shell command execution.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// Run executes a command and returns its stdout.
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

	shellBin, shellArgs := parseShellCommand(shellCmd)

	timeoutSecs := timeout
	if timeoutSecs <= 0 {
		timeoutSecs = r.Timeout
	}
	if timeoutSecs <= 0 {
		timeoutSecs = DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	args := append(shellArgs, command)
	cmd := exec.CommandContext(ctx, shellBin, args...)

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Own process group so timeout can kill child processes too (Unix-only).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		// Negative PID signals the whole process group, reaping children.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return result, fmt.Errorf("command timed out after %d seconds", timeoutSecs)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, fmt.Errorf("command failed with exit code %d: %s", result.ExitCode, strings.TrimSpace(result.Stderr))
		}
		return result, fmt.Errorf("executing command: %w", err)
	}

	return result, nil
}

// parseShellCommand splits "bash -c" into ("bash", ["-c"]); a bare binary defaults to -c.
func parseShellCommand(shell string) (string, []string) {
	parts := strings.Fields(shell)
	if len(parts) == 0 {
		return "sh", []string{"-c"}
	}

	binary := parts[0]
	args := parts[1:]

	if len(args) == 0 {
		args = []string{"-c"}
	}

	return binary, args
}
