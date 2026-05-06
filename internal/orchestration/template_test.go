package orchestration

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mockShellRunner implements ShellRunner for testing.
type mockShellRunner struct {
	output string
	err    error
	calls  []shellCall
}

type shellCall struct {
	command    string
	workingDir string
	shell      string
	timeout    int
}

func (m *mockShellRunner) Run(command, workingDir, shell string, timeout int) (string, error) {
	m.calls = append(m.calls, shellCall{command, workingDir, shell, timeout})
	return m.output, m.err
}

func TestTemplateProcessor_Process(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		fields       UTDFields
		instructions string
		fileContent  string // content to write to temp file
		shellOutput  string
		wantContains string
		wantErr      bool
		errContains  string
	}{
		{
			name: "simple prompt without placeholders",
			fields: UTDFields{
				Prompt: "Hello, world!",
			},
			wantContains: "Hello, world!",
		},
		{
			name: "prompt with datetime placeholder",
			fields: UTDFields{
				Prompt: "Today is {{.datetime}}",
			},
			wantContains: "Today is 20", // Partial match for year prefix
		},
		{
			name: "prompt with instructions placeholder",
			fields: UTDFields{
				Prompt: "Instructions: {{.instructions}}",
			},
			instructions: "focus on error handling",
			wantContains: "Instructions: focus on error handling",
		},
		{
			name: "prompt with file contents placeholder",
			fields: UTDFields{
				Prompt: "File content:\n{{.file_contents}}",
			},
			fileContent:  "This is the file content.",
			wantContains: "File content:\nThis is the file content.",
		},
		{
			name: "prompt with command output placeholder",
			fields: UTDFields{
				Prompt:  "Output: {{.command_output}}",
				Command: "echo hello",
			},
			shellOutput:  "hello\n",
			wantContains: "Output: hello\n",
		},
		{
			name:   "file only - content becomes template",
			fields: UTDFields{
				// File will be set in test
			},
			fileContent:  "Content from file only",
			wantContains: "Content from file only",
		},
		{
			name: "command only - output becomes template",
			fields: UTDFields{
				Command: "echo test",
			},
			shellOutput:  "test output",
			wantContains: "test output",
		},
		{
			name: "prompt with cwd placeholder",
			fields: UTDFields{
				Prompt: "Working dir: {{.cwd}}",
			},
			wantContains: "Working dir: /",
		},
		{
			name: "prompt with os placeholder",
			fields: UTDFields{
				Prompt: "OS: {{.os}}",
			},
			wantContains: "OS: " + runtime.GOOS,
		},
		{
			name: "prompt with home placeholder",
			fields: UTDFields{
				Prompt: "Home: {{.home}}",
			},
			wantContains: "Home: /",
		},
		{
			name: "prompt with hostname placeholder",
			fields: UTDFields{
				Prompt: "Host: {{.hostname}}",
			},
			wantContains: "Host: ",
		},
		{
			name: "prompt with user placeholder",
			fields: UTDFields{
				Prompt: "User: {{.user}}",
			},
			wantContains: "User: ",
		},
		{
			name:        "no file, command, or prompt",
			fields:      UTDFields{},
			wantErr:     true,
			errContains: "UTD requires at least one of",
		},
		{
			name: "invalid template syntax",
			fields: UTDFields{
				Prompt: "Bad template {{.Unknown",
			},
			wantErr:     true,
			errContains: "parsing template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create file if content provided
			if tt.fileContent != "" && tt.fields.File == "" {
				filePath := filepath.Join(tmpDir, "test.md")
				if err := os.WriteFile(filePath, []byte(tt.fileContent), 0644); err != nil {
					t.Fatalf("writing test file: %v", err)
				}
				tt.fields.File = filePath
			}

			// Create mock shell runner
			var runner *mockShellRunner
			if tt.shellOutput != "" || tt.fields.Command != "" {
				runner = &mockShellRunner{output: tt.shellOutput}
			}

			processor := NewTemplateProcessor(nil, runner, tmpDir)
			result, err := processor.Process(tt.fields, tt.instructions)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errContains)
					return
				}
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !strings.Contains(result.Content, tt.wantContains) {
				t.Errorf("content = %q, want containing %q", result.Content, tt.wantContains)
			}
		})
	}
}

func TestTemplateProcessor_LazyEvaluation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a test file
	filePath := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(filePath, []byte("file content"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	t.Run("file not read when file_contents not used", func(t *testing.T) {
		fields := UTDFields{
			File:   "/nonexistent/file.md", // Would error if read
			Prompt: "Simple prompt without file placeholder",
		}

		processor := NewTemplateProcessor(nil, nil, tmpDir)
		result, err := processor.Process(fields, "")

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.FileRead {
			t.Error("file should not have been read")
		}
	})

	t.Run("command not executed when command_output not used", func(t *testing.T) {
		runner := &mockShellRunner{output: "output"}
		fields := UTDFields{
			Command: "echo hello",
			Prompt:  "Simple prompt without command placeholder",
		}

		processor := NewTemplateProcessor(nil, runner, tmpDir)
		result, err := processor.Process(fields, "")

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result.CommandExecuted {
			t.Error("command should not have been executed")
		}
		if len(runner.calls) > 0 {
			t.Errorf("expected 0 shell calls, got %d", len(runner.calls))
		}
	})

	t.Run("file read when file_contents used", func(t *testing.T) {
		fields := UTDFields{
			File:   filePath,
			Prompt: "Content: {{.file_contents}}",
		}

		processor := NewTemplateProcessor(nil, nil, tmpDir)
		result, err := processor.Process(fields, "")

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.FileRead {
			t.Error("file should have been read")
		}
		if !strings.Contains(result.Content, "file content") {
			t.Errorf("content = %q, expected to contain file content", result.Content)
		}
	})

	t.Run("command executed when command_output used", func(t *testing.T) {
		runner := &mockShellRunner{output: "hello world"}
		fields := UTDFields{
			Command: "echo hello",
			Prompt:  "Output: {{.command_output}}",
		}

		processor := NewTemplateProcessor(nil, runner, tmpDir)
		result, err := processor.Process(fields, "")

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !result.CommandExecuted {
			t.Error("command should have been executed")
		}
		if len(runner.calls) != 1 {
			t.Errorf("expected 1 shell call, got %d", len(runner.calls))
		}
	})
}

// TestTemplateProcessor_SourcePriorityContract pins the source-selection and
// lazy-expansion behaviour that internal/cli/read.go's readUTD trim block
// depends on. `start read` documents source priority as file > prompt >
// command, but Process picks Prompt > File > Command. read.go flips the
// order by clearing higher-priority fields before calling Process. If
// Process's source-selection or its lazy {{.command_output}} expansion ever
// changes, this test fails — at which point the maintainer must also revisit
// readUTD's trim block in read.go (it relies on the behaviour pinned here to
// implement file > prompt > command without shelling out).
func TestTemplateProcessor_SourcePriorityContract(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	filePath := filepath.Join(tmpDir, "src.md")
	if err := os.WriteFile(filePath, []byte("FILE-CONTENT"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	t.Run("prompt wins over file when both set", func(t *testing.T) {
		fields := UTDFields{
			File:   filePath,
			Prompt: "PROMPT-CONTENT",
		}
		processor := NewTemplateProcessor(nil, nil, tmpDir)
		result, err := processor.Process(fields, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, "PROMPT-CONTENT") {
			t.Errorf("expected prompt to win, got: %q", result.Content)
		}
		if strings.Contains(result.Content, "FILE-CONTENT") {
			t.Errorf("file content should not appear when prompt wins, got: %q", result.Content)
		}
	})

	t.Run("file wins over command when prompt empty", func(t *testing.T) {
		runner := &mockShellRunner{output: "COMMAND-CONTENT"}
		fields := UTDFields{
			File:    filePath,
			Command: "echo command",
		}
		processor := NewTemplateProcessor(nil, runner, tmpDir)
		result, err := processor.Process(fields, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, "FILE-CONTENT") {
			t.Errorf("expected file to win, got: %q", result.Content)
		}
		if len(runner.calls) > 0 {
			t.Errorf("command should not run when file is the source and {{.command_output}} is not referenced, got %d calls", len(runner.calls))
		}
	})

	t.Run("lazy command expansion fires for file source referencing command_output", func(t *testing.T) {
		// readUTD's trim block (read.go: file != "" → fields.Command = "")
		// deliberately neutralises this path. Pinning the underlying lazy
		// expansion here means a refactor that removes it makes the trim
		// block dead code and surfaces in this test.
		cmdRefFile := filepath.Join(tmpDir, "cmd-ref.md")
		if err := os.WriteFile(cmdRefFile, []byte("before {{.command_output}} after"), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}
		runner := &mockShellRunner{output: "INJECTED"}
		fields := UTDFields{
			File:    cmdRefFile,
			Command: "echo injected",
		}
		processor := NewTemplateProcessor(nil, runner, tmpDir)
		result, err := processor.Process(fields, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, "INJECTED") {
			t.Errorf("expected lazy command expansion to fire, got: %q", result.Content)
		}
		if len(runner.calls) != 1 {
			t.Errorf("expected 1 shell call from lazy expansion, got %d", len(runner.calls))
		}
	})
}

func TestEnvTemplateData(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	data := envTemplateData(tmpDir)

	t.Run("os is set", func(t *testing.T) {
		if data["os"] != runtime.GOOS {
			t.Errorf("os = %q, want %q", data["os"], runtime.GOOS)
		}
	})

	t.Run("cwd is set", func(t *testing.T) {
		if data["cwd"] == "" {
			t.Error("cwd should not be empty")
		}
	})

	t.Run("home is set", func(t *testing.T) {
		if data["home"] == "" {
			t.Error("home should not be empty")
		}
	})

	t.Run("hostname is set", func(t *testing.T) {
		if data["hostname"] == "" {
			t.Error("hostname should not be empty")
		}
	})

	t.Run("os_name is set", func(t *testing.T) {
		if data["os_name"] == "" {
			t.Error("os_name should not be empty")
		}
	})

	t.Run("git_branch empty outside repo", func(t *testing.T) {
		// tmpDir is not a git repo so git_branch should be absent or empty.
		if v, ok := data["git_branch"]; ok && v != "" {
			t.Logf("git_branch = %q (may be set if tmpDir is inside a repo)", v)
		}
	})
}

func TestDefaultFileReader_Read(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	reader := &DefaultFileReader{}

	t.Run("reads file successfully", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "test.txt")
		expected := "test content"
		if err := os.WriteFile(filePath, []byte(expected), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		content, err := reader.Read(filePath)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if content != expected {
			t.Errorf("content = %q, want %q", content, expected)
		}
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		_, err := reader.Read(filepath.Join(tmpDir, "nonexistent.txt"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("expands tilde in path", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("could not get home directory")
		}

		// Create a file in home directory for testing
		testFile := filepath.Join(home, ".start-test-file")
		if err := os.WriteFile(testFile, []byte("home content"), 0644); err != nil {
			t.Skip("could not write test file in home directory")
		}
		defer func() { _ = os.Remove(testFile) }()

		content, err := reader.Read("~/.start-test-file")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if content != "home content" {
			t.Errorf("content = %q, want %q", content, "home content")
		}
	})
}

func TestIsUTDValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		fields UTDFields
		want   bool
	}{
		{
			name:   "empty fields",
			fields: UTDFields{},
			want:   false,
		},
		{
			name:   "file only",
			fields: UTDFields{File: "test.md"},
			want:   true,
		},
		{
			name:   "command only",
			fields: UTDFields{Command: "echo hello"},
			want:   true,
		},
		{
			name:   "prompt only",
			fields: UTDFields{Prompt: "Hello"},
			want:   true,
		},
		{
			name:   "all fields",
			fields: UTDFields{File: "f", Command: "c", Prompt: "p"},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUTDValid(tt.fields); got != tt.want {
				t.Errorf("IsUTDValid() = %v, want %v", got, tt.want)
			}
		})
	}
}
