package orchestration

import (
	"os"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestExecutor_BuildCommand(t *testing.T) {
	t.Parallel()
	executor := NewExecutor("")

	tests := []struct {
		name        string
		config      ExecuteConfig
		wantContain string
		wantErr     bool
	}{
		{
			name: "simple command with bin",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "claude",
					Command: "{{.bin}} chat",
				},
			},
			// bin is now shell-escaped and quoted
			wantContain: "'claude' chat",
		},
		{
			name: "command with model",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "claude",
					Command: "{{.bin}} --model {{.model}}",
					Models: map[string]string{
						"sonnet": "claude-sonnet-4-20250514",
					},
					DefaultModel: "sonnet",
				},
			},
			// All placeholders are now shell-escaped and quoted
			wantContain: "'claude-sonnet-4-20250514'",
		},
		{
			name: "command with model override",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "claude",
					Command: "{{.bin}} --model {{.model}}",
					Models: map[string]string{
						"sonnet": "claude-sonnet-4-20250514",
						"opus":   "claude-opus-4-20250514",
					},
					DefaultModel: "sonnet",
				},
				Model: "opus",
			},
			wantContain: "'claude-opus-4-20250514'",
		},
		{
			name: "command with role",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "claude",
					Command: "{{.bin}} --system {{.role}}",
				},
				Role: "You are a code reviewer.",
			},
			// escapeForShell wraps in single quotes
			wantContain: "'You are a code reviewer.'",
		},
		{
			name: "command with role file",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "claude",
					Command: "{{.bin}} --system-file {{.role_file}}",
				},
				RoleFile: "/tmp/role.md",
			},
			// File paths are also quoted now
			wantContain: "'/tmp/role.md'",
		},
		{
			name: "conditional model in template",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "claude",
					Command: "{{.bin}}{{if .model}} --model {{.model}}{{end}}",
				},
			},
			// bin is also quoted now
			wantContain: "'claude'",
		},
		{
			name: "invalid template",
			config: ExecuteConfig{
				Agent: Agent{
					Command: "{{.Invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "empty bin field",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "",
					Command: "{{.bin}} --print {{.prompt}}",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := executor.BuildCommand(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !strings.Contains(cmd, tt.wantContain) {
				t.Errorf("command = %q, want containing %q", cmd, tt.wantContain)
			}
		})
	}
}

func TestEscapeForShell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple text", "simple text", "'simple text'"},
		{"single quote escaping", "it's a test", "'it'\"'\"'s a test'"},
		{"no quotes", "no quotes here", "'no quotes here'"},
		{"empty string", "", "''"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeForShell(tt.input)
			if got != tt.want {
				t.Errorf("escapeForShell(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeForShell_NoEnvExpansion(t *testing.T) {
	t.Parallel()
	// Environment variables should NOT be expanded (they're safely quoted).
	// Single quotes prevent shell expansion of $VAR, $(cmd), and `cmd`.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "preserves $VAR syntax",
			input: "Hello $TEST_VAR",
			want:  "'Hello $TEST_VAR'",
		},
		{
			name:  "preserves ${VAR} syntax",
			input: "Hello ${TEST_USER}",
			want:  "'Hello ${TEST_USER}'",
		},
		{
			name:  "preserves undefined var",
			input: "Hello $UNDEFINED_VAR_XYZ",
			want:  "'Hello $UNDEFINED_VAR_XYZ'",
		},
		{
			name:  "command substitution not executed",
			input: "$(echo pwned)",
			want:  "'$(echo pwned)'",
		},
		{
			name:  "backticks not executed",
			input: "`echo pwned`",
			want:  "'`echo pwned`'",
		},
		{
			name:  "preserves dollar sign with quotes",
			input: "$TEST_USER's files",
			want:  "'$TEST_USER'\"'\"'s files'",
		},
		{
			name:  "preserves literal dollar amounts",
			input: "Cost is $100",
			want:  "'Cost is $100'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeForShell(tt.input)
			if got != tt.want {
				t.Errorf("escapeForShell(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateCommandTemplate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		tmpl    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid template without quotes",
			tmpl:    "{{.bin}} --prompt {{.prompt}}",
			wantErr: false,
		},
		{
			name:    "valid template with all placeholders",
			tmpl:    "{{.bin}} --model {{.model}} --system {{.role}} --prompt {{.prompt}}",
			wantErr: false,
		},
		{
			name:    "invalid single-brace prompt",
			tmpl:    "--print {prompt}",
			wantErr: true,
			errMsg:  "template uses {prompt} but Go templates require {{.prompt}}",
		},
		{
			name:    "invalid single-brace role",
			tmpl:    "--system {role} --prompt {{.prompt}}",
			wantErr: true,
			errMsg:  "template uses {role} but Go templates require {{.role}}",
		},
		{
			name:    "invalid single-brace multiple",
			tmpl:    "{bin} --model {model}",
			wantErr: true,
			errMsg:  "template uses {bin} but Go templates require {{.bin}}",
		},
		{
			name:    "invalid single-quoted prompt",
			tmpl:    "{{.bin}} --prompt '{{.prompt}}'",
			wantErr: true,
			errMsg:  "quoted placeholder",
		},
		{
			name:    "invalid double-quoted prompt",
			tmpl:    `{{.bin}} --prompt "{{.prompt}}"`,
			wantErr: true,
			errMsg:  "quoted placeholder",
		},
		{
			name:    "invalid single-quoted role",
			tmpl:    "{{.bin}} --system '{{.role}}'",
			wantErr: true,
			errMsg:  "quoted placeholder",
		},
		{
			name:    "invalid single-quoted model",
			tmpl:    "{{.bin}} --model '{{.model}}'",
			wantErr: true,
			errMsg:  "quoted placeholder",
		},
		{
			name:    "invalid single-quoted bin",
			tmpl:    "'{{.bin}}' --prompt {{.prompt}}",
			wantErr: true,
			errMsg:  "quoted placeholder",
		},
		{
			name:    "invalid single-quoted role_file",
			tmpl:    "{{.bin}} --system-file '{{.role_file}}'",
			wantErr: true,
			errMsg:  "quoted placeholder",
		},
		{
			name:    "invalid single-quoted datetime",
			tmpl:    "{{.bin}} --timestamp '{{.datetime}}'",
			wantErr: true,
			errMsg:  "quoted placeholder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommandTemplate(tt.tmpl)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCommandTemplate() expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateCommandTemplate() error = %q, want containing %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCommandTemplate() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestExtractAgent(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	config := `
		agents: {
			claude: {
				bin: "claude"
				command: "{{.bin}} --model {{.model}}"
				default_model: "sonnet"
				description: "Anthropic Claude"
				models: {
					sonnet: "claude-sonnet-4-20250514"
					opus: "claude-opus-4-20250514"
				}
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	t.Run("extracts all fields", func(t *testing.T) {
		agent, err := ExtractAgent(cfg, "claude")
		if err != nil {
			t.Fatalf("ExtractAgent() error = %v", err)
		}

		if agent.Name != "claude" {
			t.Errorf("Name = %q, want 'claude'", agent.Name)
		}
		if agent.Bin != "claude" {
			t.Errorf("Bin = %q, want 'claude'", agent.Bin)
		}
		if agent.DefaultModel != "sonnet" {
			t.Errorf("DefaultModel = %q, want 'sonnet'", agent.DefaultModel)
		}
		if agent.Description != "Anthropic Claude" {
			t.Errorf("Description = %q, want 'Anthropic Claude'", agent.Description)
		}
		if len(agent.Models) != 2 {
			t.Errorf("Models count = %d, want 2", len(agent.Models))
		}
		if agent.Models["sonnet"] != "claude-sonnet-4-20250514" {
			t.Errorf("Models[sonnet] = %q, want 'claude-sonnet-4-20250514'", agent.Models["sonnet"])
		}
	})

	t.Run("nonexistent agent", func(t *testing.T) {
		_, err := ExtractAgent(cfg, "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent agent")
		}
	})
}

func TestGenerateDryRunCommand(t *testing.T) {
	t.Parallel()
	agent := Agent{
		Name: "claude",
	}
	contexts := []string{"env", "project"}
	workingDir := "/home/user/project"
	cmdStr := "claude --model sonnet 'hello'"

	result := GenerateDryRunCommand(agent, "sonnet", "code-reviewer", contexts, workingDir, cmdStr)

	expectedContains := []string{
		"# Agent: claude",
		"# Model: sonnet",
		"# Role: code-reviewer",
		"# Contexts: env, project",
		"# Working Directory: /home/user/project",
		"claude --model sonnet",
	}

	for _, expected := range expectedContains {
		if !strings.Contains(result, expected) {
			t.Errorf("result should contain %q", expected)
		}
	}
}

func TestBuildCommand_BinWithSpaces(t *testing.T) {
	t.Parallel()

	// Create a real executable in a temp directory whose path contains a space.
	dir := t.TempDir()
	binDir := dir + "/my tools"
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("creating bin dir: %v", err)
	}
	binPath := binDir + "/mytool"
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatalf("writing fake binary: %v", err)
	}

	executor := NewExecutor("")
	cfg := ExecuteConfig{
		Agent: Agent{
			Bin:     binPath,
			Command: "{{.bin}} --print {{.prompt}}",
		},
		Prompt: "hello",
	}

	cmd, err := executor.BuildCommand(cfg)
	if err != nil {
		t.Fatalf("BuildCommand() unexpected error: %v", err)
	}
	// The bin path should appear shell-escaped (single-quoted) in the output.
	if !strings.Contains(cmd, "'"+binPath+"'") {
		t.Errorf("command = %q, want it to contain quoted bin path", cmd)
	}
}

func TestExecutor_ExecuteWithoutReplace(t *testing.T) {
	t.Parallel()
	executor := NewExecutor("")

	t.Run("simple echo command", func(t *testing.T) {
		config := ExecuteConfig{
			Agent: Agent{
				Bin:     "echo",
				Command: "{{.bin}} hello world",
			},
		}

		output, err := executor.ExecuteWithoutReplace(config)
		if err != nil {
			t.Fatalf("ExecuteWithoutReplace() error = %v", err)
		}

		if !strings.Contains(output, "hello world") {
			t.Errorf("output = %q, want containing 'hello world'", output)
		}
	})

	t.Run("failing command returns error with stdout", func(t *testing.T) {
		t.Parallel()
		// Use a compound command: echo to stdout, then exit non-zero.
		// sh -c handles this directly without needing a binary named "false".
		dir := t.TempDir()
		// Write a tiny script that prints and exits non-zero.
		script := dir + "/fail.sh"
		if err := os.WriteFile(script, []byte("#!/bin/sh\necho failing\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}

		config := ExecuteConfig{
			Agent: Agent{
				Bin:     script,
				Command: "{{.bin}}",
			},
		}

		output, err := executor.ExecuteWithoutReplace(config)
		if err == nil {
			t.Error("ExecuteWithoutReplace() should return error for non-zero exit")
		}
		if !strings.Contains(output, "failing") {
			t.Errorf("stdout should be captured even on failure, got %q", output)
		}
	})

	t.Run("with working directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		config := ExecuteConfig{
			Agent: Agent{
				Bin:     "pwd",
				Command: "{{.bin}}",
			},
			WorkingDir: dir,
		}

		output, err := executor.ExecuteWithoutReplace(config)
		if err != nil {
			t.Fatalf("ExecuteWithoutReplace() error = %v", err)
		}
		// pwd output should contain the working directory path.
		if !strings.Contains(strings.TrimSpace(output), dir) {
			t.Errorf("output = %q, want containing %q", output, dir)
		}
	})
}

func TestIsValidEnvVarName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple uppercase", "FOO", true},
		{"simple lowercase", "foo", true},
		{"mixed case", "FooBar", true},
		{"with underscore", "FOO_BAR", true},
		{"starts with underscore", "_FOO", true},
		{"with numbers", "FOO123", true},
		{"underscore and numbers", "_FOO_123", true},
		{"empty string", "", false},
		{"starts with number", "123FOO", false},
		{"contains hyphen", "FOO-BAR", false},
		{"contains dot", "FOO.BAR", false},
		{"contains space", "FOO BAR", false},
		{"contains equals", "FOO=BAR", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidEnvVarName(tt.input)
			if got != tt.want {
				t.Errorf("isValidEnvVarName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsEnvVarAssignment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		token string
		want  bool
	}{
		{"simple assignment", "FOO=bar", true},
		{"empty value", "FOO=", true},
		{"quoted value single", "FOO='bar'", true},
		{"quoted value double", `FOO="bar"`, true},
		{"quoted path", "GEMINI_SYSTEM_MD='/path/to/file.md'", true},
		{"underscore var", "_FOO=bar", true},
		{"mixed case var", "FooBar=value", true},
		{"no equals", "FOO", false},
		{"starts with equals", "=FOO", false},
		{"starts with number", "123=bar", false},
		{"hyphen in name", "FOO-BAR=value", false},
		{"executable path", "/usr/bin/foo", false},
		{"quoted executable", "'/usr/bin/foo'", false},
		{"flag", "--model", false},
		{"flag with value", "--model=sonnet", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEnvVarAssignment(tt.token)
			if got != tt.want {
				t.Errorf("isEnvVarAssignment(%q) = %v, want %v", tt.token, got, tt.want)
			}
		})
	}
}

func TestBuildCommand_WithEnvVarPrefix(t *testing.T) {
	t.Parallel()
	executor := NewExecutor("")

	tests := []struct {
		name        string
		config      ExecuteConfig
		wantContain string
		wantErr     bool
	}{
		{
			name: "command with env var prefix",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "echo",
					Command: "SOME_VAR={{.role_file}} {{.bin}} --model {{.model}}",
					Models:  map[string]string{"pro": "model-id"},
				},
				RoleFile: "/tmp/role.md",
				Model:    "pro",
			},
			wantContain: "'echo'",
		},
		{
			name: "command with empty env var value",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "echo",
					Command: "SOME_VAR={{.role_file}} {{.bin}} --model {{.model}}",
					Models:  map[string]string{"pro": "model-id"},
				},
				RoleFile: "", // empty role file
				Model:    "pro",
			},
			wantContain: "'echo'",
		},
		{
			name: "command with multiple env vars",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "echo",
					Command: "FOO=bar BAZ=qux {{.bin}} hello",
				},
			},
			wantContain: "'echo'",
		},
		{
			name: "only env vars no command",
			config: ExecuteConfig{
				Agent: Agent{
					Bin:     "gemini",
					Command: "FOO=bar BAZ=qux",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := executor.BuildCommand(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !strings.Contains(cmd, tt.wantContain) {
				t.Errorf("command = %q, want containing %q", cmd, tt.wantContain)
			}
		})
	}
}
