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
// Keys are lowercase to match documented placeholder names (e.g., {{.file}}).
type TemplateData map[string]string

// UTDFields represents the raw Unified Template Design (UTD) fields extracted from CUE configuration.
type UTDFields struct {
	File    string
	Command string
	Prompt  string
	Shell   string
	Timeout int // command timeout in seconds, 0 = default
}

// ShellRunner executes shell commands and returns output.
type ShellRunner interface {
	Run(command, workingDir, shell string, timeout int) (string, error)
}

// FileReader reads file contents.
type FileReader interface {
	Read(path string) (string, error)
}

// DefaultFileReader implements FileReader using os.ReadFile.
type DefaultFileReader struct{}

// Read reads file contents from the filesystem.
func (r *DefaultFileReader) Read(path string) (string, error) {
	return ReadFilePath(path)
}

// TemplateProcessor resolves UTD templates.
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
	Content string
	// TempFile is set only when the source was file-based and a temp file was created.
	TempFile        string
	FileRead        bool
	CommandExecuted bool
	Warnings        []string
}

// Process resolves a UTD template with lazy evaluation.
// It only reads files or executes commands if the template references them.
func (p *TemplateProcessor) Process(fields UTDFields, instructions string) (ProcessResult, error) {
	var result ProcessResult

	templateStr := fields.Prompt
	if templateStr == "" {
		if fields.File != "" {
			content, err := p.fileReader.Read(fields.File)
			if err != nil {
				return result, fmt.Errorf("reading file %s: %w", fields.File, err)
			}
			templateStr = content
			result.FileRead = true
		} else if fields.Command != "" {
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

	needsFileContents := strings.Contains(templateStr, "{{.file_contents}}") ||
		strings.Contains(templateStr, "{{ .file_contents }}")
	needsCommandOutput := strings.Contains(templateStr, "{{.command_output}}") ||
		strings.Contains(templateStr, "{{ .command_output }}")

	data := envTemplateData(p.workingDir)
	data["file"] = fields.File
	data["command"] = fields.Command
	data["datetime"] = time.Now().Format(time.RFC3339)
	data["instructions"] = instructions

	if needsFileContents && fields.File != "" && !result.FileRead {
		content, err := p.fileReader.Read(fields.File)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("could not read file %s: %v", fields.File, err))
		} else {
			data["file_contents"] = content
			result.FileRead = true
		}
	}

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

	// missingkey=zero lets file content carry template-like syntax (e.g. code examples) without erroring.
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

// envTemplateData builds the environment-based template variables. All values
// fall back to empty string on error so templates always render.
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

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// osName returns a human-readable OS/distro name, falling back to runtime.GOOS.
func osName() string {
	switch runtime.GOOS {
	case "linux":
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for line := range strings.SplitSeq(string(data), "\n") {
				if after, ok := strings.CutPrefix(line, "NAME="); ok {
					return strings.Trim(after, `"`)
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

// IsUTDValid reports whether at least one of file, command, or prompt is set.
func IsUTDValid(fields UTDFields) bool {
	return fields.File != "" || fields.Command != "" || fields.Prompt != ""
}
