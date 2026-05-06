// Package orchestration handles UTD template processing, prompt composition, and agent execution.
package orchestration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"
)

// TemplateData holds the data available for UTD template substitution.
// Uses lowercase keys to match documented placeholder names (e.g., {{.file}}, {{.instructions}}).
type TemplateData map[string]string

// UTDFields represents the raw Unified Template Design (UTD) fields extracted from CUE configuration.
type UTDFields struct {
	// File is the path to read content from.
	File string
	// Command is the shell command to execute.
	Command string
	// Prompt is the template string to render.
	Prompt string
	// Shell is the shell to use for command execution (optional).
	Shell string
	// Timeout is the command timeout in seconds (optional, 0 = default).
	Timeout int
}

// ShellRunner executes shell commands and returns output.
// This interface allows for dependency injection in tests.
type ShellRunner interface {
	// Run executes a command and returns stdout.
	// workingDir specifies the directory to run in.
	// shell specifies the shell to use (e.g., "bash -c").
	// timeout specifies the timeout in seconds (0 = default).
	Run(command, workingDir, shell string, timeout int) (string, error)
}

// FileReader reads file contents.
// This interface allows for dependency injection in tests.
type FileReader interface {
	// Read reads the contents of a file.
	Read(path string) (string, error)
}

// DefaultFileReader implements FileReader using os.ReadFile.
type DefaultFileReader struct{}

// Read reads file contents from the filesystem.
func (r *DefaultFileReader) Read(path string) (string, error) {
	return ReadFilePath(path)
}

// TemplateProcessor handles UTD template resolution.
type TemplateProcessor struct {
	fileReader  FileReader
	shellRunner ShellRunner
	workingDir  string
}

// NewTemplateProcessor creates a new template processor.
func NewTemplateProcessor(fr FileReader, sr ShellRunner, workingDir string) *TemplateProcessor {
	if fr == nil {
		fr = &DefaultFileReader{}
	}
	return &TemplateProcessor{
		fileReader:  fr,
		shellRunner: sr,
		workingDir:  workingDir,
	}
}

// ProcessResult contains the result of template processing.
type ProcessResult struct {
	// Content is the rendered template output.
	Content string
	// TempFile is the path to the temp file created for {{.file}} placeholder.
	// Only set when the source was file-based and temp file was created.
	TempFile string
	// FileRead indicates whether a file was read.
	FileRead bool
	// CommandExecuted indicates whether a command was executed.
	CommandExecuted bool
	// Warnings contains any non-fatal issues encountered.
	Warnings []string
}

// Process resolves a UTD template with lazy evaluation.
// It only reads files or executes commands if the template references them.
func (p *TemplateProcessor) Process(fields UTDFields, instructions string) (ProcessResult, error) {
	var result ProcessResult

	// Determine the template source
	templateStr := fields.Prompt
	if templateStr == "" {
		// If no prompt, check for file content
		if fields.File != "" {
			content, err := p.fileReader.Read(fields.File)
			if err != nil {
				return result, fmt.Errorf("reading file %s: %w", fields.File, err)
			}
			templateStr = content
			result.FileRead = true
		} else if fields.Command != "" {
			// Command output becomes the template
			if p.shellRunner == nil {
				return result, fmt.Errorf("shell runner required for command execution")
			}
			output, err := p.shellRunner.Run(fields.Command, p.workingDir, fields.Shell, fields.Timeout)
			if err != nil {
				return result, fmt.Errorf("executing command: %w", err)
			}
			templateStr = output
			result.CommandExecuted = true
		} else {
			return result, fmt.Errorf("UTD requires at least one of: file, command, or prompt")
		}
	}

	// Check if template uses placeholders that require file/command execution
	// Match documented lowercase/snake_case placeholders
	needsFileContents := strings.Contains(templateStr, "{{.file_contents}}") ||
		strings.Contains(templateStr, "{{ .file_contents }}")
	needsCommandOutput := strings.Contains(templateStr, "{{.command_output}}") ||
		strings.Contains(templateStr, "{{ .command_output }}")

	// Build template data with lowercase keys to match documented placeholders
	data := envTemplateData(p.workingDir)
	data["file"] = fields.File
	data["command"] = fields.Command
	data["datetime"] = time.Now().Format(time.RFC3339)
	data["instructions"] = instructions

	// Lazy evaluation: only read file if needed
	if needsFileContents && fields.File != "" && !result.FileRead {
		content, err := p.fileReader.Read(fields.File)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("could not read file %s: %v", fields.File, err))
		} else {
			data["file_contents"] = content
			result.FileRead = true
		}
	}

	// Lazy evaluation: only execute command if needed
	if needsCommandOutput && fields.Command != "" && !result.CommandExecuted {
		if p.shellRunner == nil {
			result.Warnings = append(result.Warnings, "shell runner not available for command execution")
		} else {
			output, err := p.shellRunner.Run(fields.Command, p.workingDir, fields.Shell, fields.Timeout)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("command failed: %v", err))
			} else {
				data["command_output"] = output
				result.CommandExecuted = true
			}
		}
	}

	// Parse and execute template
	// Use Option("missingkey=zero") to handle unknown placeholders gracefully.
	// This allows file-only contexts to contain template-like syntax (e.g., in code examples)
	// without causing errors.
	tmpl, err := template.New("utd").Option("missingkey=zero").Parse(templateStr)
	if err != nil {
		return result, fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return result, fmt.Errorf("executing template: %w", err)
	}

	result.Content = buf.String()
	return result, nil
}

// envTemplateData builds the environment-based template variables.
// All values fall back to empty string on error so templates always render.
func envTemplateData(workingDir string) TemplateData {
	data := TemplateData{}

	if workingDir != "" {
		data["cwd"] = workingDir
	} else if cwd, err := os.Getwd(); err == nil {
		data["cwd"] = cwd
	}
	if home, err := os.UserHomeDir(); err == nil {
		data["home"] = home
	}
	if u, err := user.Current(); err == nil {
		data["user"] = u.Username
	}
	if hostname, err := os.Hostname(); err == nil {
		data["hostname"] = hostname
	}
	data["os"] = runtime.GOOS
	if sh := os.Getenv("SHELL"); sh != "" {
		data["shell"] = filepath.Base(sh)
	}

	// Git variables: run in workingDir, fall back to empty string if not a repo.
	if branch, err := gitOutput(workingDir, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		data["git_branch"] = branch
	}
	if root, err := gitOutput(workingDir, "rev-parse", "--show-toplevel"); err == nil {
		data["git_root"] = root
	}
	if name, err := gitOutput(workingDir, "config", "user.name"); err == nil {
		data["git_user"] = name
	}
	if email, err := gitOutput(workingDir, "config", "user.email"); err == nil {
		data["git_email"] = email
	}

	data["os_name"] = osName()

	return data
}

// gitOutput runs a git command in dir and returns trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// osName returns a human-readable OS/distro name.
// On Linux it reads NAME from /etc/os-release; on macOS it runs sw_vers;
// on other platforms it falls back to runtime.GOOS.
func osName() string {
	switch runtime.GOOS {
	case "linux":
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "NAME=") {
					return strings.Trim(strings.TrimPrefix(line, "NAME="), `"`)
				}
			}
		}
	case "darwin":
		cmd := exec.Command("sw_vers", "-productName")
		if out, err := cmd.Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return runtime.GOOS
}

// IsUTDValid checks if UTD fields satisfy the minimum requirement.
// At least one of file, command, or prompt must be specified.
func IsUTDValid(fields UTDFields) bool {
	return fields.File != "" || fields.Command != "" || fields.Prompt != ""
}
