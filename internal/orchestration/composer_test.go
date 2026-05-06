package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestComposer_Compose(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tests := []struct {
		name         string
		config       string
		selection    ContextSelection
		customText   string
		wantPrompt   string
		wantCtxs     []string
		wantStatuses map[string]string
		wantWarning  bool
	}{
		{
			name: "required contexts only",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment info"
					}
					project: {
						default: true
						prompt: "Project info"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				IncludeDefaults: false,
			},
			wantPrompt:   "Environment info",
			wantCtxs:     []string{"env", "project"},
			wantStatuses: map[string]string{"env": "loaded", "project": "skipped"},
		},
		{
			name: "required and default contexts",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment info"
					}
					project: {
						default: true
						prompt: "Project info"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				IncludeDefaults: true,
			},
			wantPrompt: "Environment info\n\nProject info",
			wantCtxs:   []string{"env", "project"},
		},
		{
			name: "tagged contexts",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
					security: {
						tags: ["security"]
						prompt: "Security context"
					}
					performance: {
						tags: ["performance"]
						prompt: "Performance context"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				Tags:            []string{"security"},
			},
			wantPrompt: "Environment\n\nSecurity context",
			wantCtxs:   []string{"env", "security"},
		},
		{
			name: "default pseudo-tag",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
					project: {
						default: true
						prompt: "Project"
					}
					debug: {
						tags: ["debug"]
						prompt: "Debug"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				Tags:            []string{"default", "debug"},
			},
			wantPrompt: "Environment\n\nProject\n\nDebug",
			wantCtxs:   []string{"env", "project", "debug"},
		},
		{
			name: "excluded defaults shown when tags specified",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
					project: {
						default: true
						prompt: "Project"
					}
					security: {
						tags: ["security"]
						prompt: "Security context"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				IncludeDefaults: true,
				Tags:            []string{"security"},
			},
			wantPrompt:   "Environment\n\nSecurity context",
			wantCtxs:     []string{"env", "security", "project"},
			wantStatuses: map[string]string{"env": "loaded", "security": "loaded", "project": "skipped"},
		},
		{
			name: "custom text appended",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
			},
			customText: "Please review this code",
			wantPrompt: "Environment\n\nPlease review this code",
			wantCtxs:   []string{"env"},
		},
		{
			name:   "no contexts defined",
			config: `{}`,
			selection: ContextSelection{
				IncludeRequired: true,
				IncludeDefaults: true,
			},
			wantPrompt: "",
			wantCtxs:   nil,
		},
		{
			name: "unmatched tag adds warning",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
					security: {
						tags: ["security"]
						prompt: "Security context"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				Tags:            []string{"nonexistent"},
			},
			wantPrompt:  "Environment",
			wantCtxs:    []string{"env"},
			wantWarning: true,
		},
		{
			name: "multiple unmatched tags add multiple warnings",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				Tags:            []string{"invalid1", "invalid2"},
			},
			wantPrompt:  "Environment",
			wantCtxs:    []string{"env"},
			wantWarning: true,
		},
		{
			name: "mix of valid and invalid tags warns only for invalid",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
					security: {
						tags: ["security"]
						prompt: "Security context"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				Tags:            []string{"security", "invalidtag"},
			},
			wantPrompt:  "Environment\n\nSecurity context",
			wantCtxs:    []string{"env", "security"},
			wantWarning: true,
		},
		{
			name: "default pseudo-tag does not warn",
			config: `
				contexts: {
					env: {
						required: true
						prompt: "Environment"
					}
				}
			`,
			selection: ContextSelection{
				IncludeRequired: true,
				Tags:            []string{"default"},
			},
			wantPrompt:  "Environment",
			wantCtxs:    []string{"env"},
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ctx.CompileString(tt.config)
			if err := cfg.Err(); err != nil {
				t.Fatalf("compile config: %v", err)
			}

			processor := NewTemplateProcessor(nil, nil, "")
			composer := NewComposer(processor, "")

			result, err := composer.Compose(cfg, tt.selection, tt.customText)
			if err != nil {
				t.Fatalf("Compose() error = %v", err)
			}

			if result.Prompt != tt.wantPrompt {
				t.Errorf("Prompt = %q, want %q", result.Prompt, tt.wantPrompt)
			}

			// Check contexts
			var gotCtxs []string
			for _, ctx := range result.Contexts {
				gotCtxs = append(gotCtxs, ctx.Name)
			}
			if len(gotCtxs) != len(tt.wantCtxs) {
				t.Errorf("Contexts = %v, want %v", gotCtxs, tt.wantCtxs)
			} else {
				for i, name := range tt.wantCtxs {
					if gotCtxs[i] != name {
						t.Errorf("Contexts[%d] = %q, want %q", i, gotCtxs[i], name)
					}
				}
			}

			// Check statuses
			if tt.wantStatuses != nil {
				for _, ctx := range result.Contexts {
					if want, ok := tt.wantStatuses[ctx.Name]; ok {
						if ctx.Status != want {
							t.Errorf("Context %q Status = %q, want %q", ctx.Name, ctx.Status, want)
						}
					}
				}
			}

			// Check warnings
			hasWarning := len(result.Warnings) > 0
			if hasWarning != tt.wantWarning {
				t.Errorf("Warnings present = %v, want %v (warnings: %v)", hasWarning, tt.wantWarning, result.Warnings)
			}
		})
	}
}

func TestComposer_ComposeWithRole(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	config := `
		contexts: {
			env: {
				required: true
				prompt: "Environment"
			}
		}
		roles: {
			assistant: {
				prompt: "You are a helpful assistant."
			}
			reviewer: {
				prompt: "You are a code reviewer."
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, "")
	composer := NewComposer(processor, "")

	t.Run("uses default role", func(t *testing.T) {
		result, err := composer.ComposeWithRole(cfg, ContextSelection{IncludeRequired: true}, "", "")
		if err != nil {
			t.Fatalf("ComposeWithRole() error = %v", err)
		}

		if result.RoleName != "assistant" {
			t.Errorf("RoleName = %q, want 'assistant'", result.RoleName)
		}
		if result.Role != "You are a helpful assistant." {
			t.Errorf("Role = %q, want 'You are a helpful assistant.'", result.Role)
		}
	})

	t.Run("uses specified role", func(t *testing.T) {
		result, err := composer.ComposeWithRole(cfg, ContextSelection{IncludeRequired: true}, "reviewer", "")
		if err != nil {
			t.Fatalf("ComposeWithRole() error = %v", err)
		}

		if result.RoleName != "reviewer" {
			t.Errorf("RoleName = %q, want 'reviewer'", result.RoleName)
		}
		if result.Role != "You are a code reviewer." {
			t.Errorf("Role = %q, want 'You are a code reviewer.'", result.Role)
		}
	})

	t.Run("explicit nonexistent role returns error", func(t *testing.T) {
		_, err := composer.ComposeWithRole(cfg, ContextSelection{IncludeRequired: true}, "nonexistent", "")
		if err == nil {
			t.Error("expected error for explicit nonexistent role")
		}
	})
}

func TestComposer_ResolveTask(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tmpDir := t.TempDir()

	config := `
		tasks: {
			"code-review": {
				command: "echo staged changes"
				prompt: """
					Review the following:

					{{.command_output}}

					Instructions: {{.instructions}}
					"""
			}
			"simple": {
				prompt: "Simple task prompt"
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	runner := &mockShellRunner{output: "diff output here"}
	processor := NewTemplateProcessor(nil, runner, tmpDir)
	composer := NewComposer(processor, tmpDir)

	t.Run("task with command and instructions", func(t *testing.T) {
		result, err := composer.ResolveTask(cfg, "code-review", "focus on security")
		if err != nil {
			t.Fatalf("ResolveTask() error = %v", err)
		}

		if !strings.Contains(result.Content, "diff output here") {
			t.Errorf("Content should contain command output")
		}
		if !strings.Contains(result.Content, "focus on security") {
			t.Errorf("Content should contain instructions")
		}
	})

	t.Run("simple task", func(t *testing.T) {
		result, err := composer.ResolveTask(cfg, "simple", "")
		if err != nil {
			t.Fatalf("ResolveTask() error = %v", err)
		}

		if result.Content != "Simple task prompt" {
			t.Errorf("Content = %q, want 'Simple task prompt'", result.Content)
		}
	})

	t.Run("nonexistent task", func(t *testing.T) {
		_, err := composer.ResolveTask(cfg, "nonexistent", "")
		if err == nil {
			t.Error("expected error for nonexistent task")
		}
	})
}

func TestComposer_ContextWithFile(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "context.md")
	if err := os.WriteFile(filePath, []byte("File-based context content"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	config := `
		contexts: {
			file_ctx: {
				required: true
				file: "` + filePath + `"
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, tmpDir)
	composer := NewComposer(processor, tmpDir)

	result, err := composer.Compose(cfg, ContextSelection{IncludeRequired: true}, "")
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}

	if result.Prompt != "File-based context content" {
		t.Errorf("Prompt = %q, want 'File-based context content'", result.Prompt)
	}
}

func TestGetTaskRole(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	config := `
		tasks: {
			"with-role": {
				role: "reviewer"
				prompt: "Review code"
			}
			"without-role": {
				prompt: "Simple task"
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	t.Run("task with role", func(t *testing.T) {
		role := GetTaskRole(cfg, "with-role")
		if role != "reviewer" {
			t.Errorf("GetTaskRole() = %q, want 'reviewer'", role)
		}
	})

	t.Run("task without role", func(t *testing.T) {
		role := GetTaskRole(cfg, "without-role")
		if role != "" {
			t.Errorf("GetTaskRole() = %q, want empty", role)
		}
	})

	t.Run("nonexistent task", func(t *testing.T) {
		role := GetTaskRole(cfg, "nonexistent")
		if role != "" {
			t.Errorf("GetTaskRole() = %q, want empty", role)
		}
	})
}

func TestExtractUTDFields(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	config := `
		item: {
			file: "test.md"
			command: "echo hello"
			prompt: "Test prompt"
			shell: "bash -c"
			timeout: 60
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	item := cfg.LookupPath(cue.ParsePath("item"))
	fields := ExtractUTDFields(item)

	if fields.File != "test.md" {
		t.Errorf("File = %q, want 'test.md'", fields.File)
	}
	if fields.Command != "echo hello" {
		t.Errorf("Command = %q, want 'echo hello'", fields.Command)
	}
	if fields.Prompt != "Test prompt" {
		t.Errorf("Prompt = %q, want 'Test prompt'", fields.Prompt)
	}
	if fields.Shell != "bash -c" {
		t.Errorf("Shell = %q, want 'bash -c'", fields.Shell)
	}
	if fields.Timeout != 60 {
		t.Errorf("Timeout = %d, want 60", fields.Timeout)
	}
}

func TestGetDefaultRole(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tests := []struct {
		name     string
		config   string
		wantRole string
	}{
		{
			name: "returns first role in definition order",
			config: `
				roles: {
					first: { prompt: "First role." }
					second: { prompt: "Second role." }
				}
			`,
			wantRole: "first",
		},
		{
			name:     "returns empty when no roles defined",
			config:   `{}`,
			wantRole: "",
		},
		{
			name: "returns empty when settings exists but no roles",
			config: `
				settings: {
					default_agent: "claude"
				}
			`,
			wantRole: "",
		},
		{
			name: "returns empty when roles is empty",
			config: `
				roles: {}
			`,
			wantRole: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ctx.CompileString(tt.config)
			if err := cfg.Err(); err != nil {
				t.Fatalf("compile config: %v", err)
			}

			processor := NewTemplateProcessor(nil, nil, "")
			composer := NewComposer(processor, "")

			result, _, err := composer.selectDefaultRole(cfg)
			if err != nil {
				t.Fatalf("selectDefaultRole() error: %v", err)
			}
			if result != tt.wantRole {
				t.Errorf("selectDefaultRole() = %q, want %q", result, tt.wantRole)
			}
		})
	}
}

func TestSelectDefaultRole(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	t.Run("optional role skipped when file missing", func(t *testing.T) {
		config := `
			roles: {
				"optional-missing": {
					file: "/nonexistent/path/role.md"
					optional: true
				}
				"fallback": {
					prompt: "Fallback role"
				}
			}
		`
		cfg := ctx.CompileString(config)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		processor := NewTemplateProcessor(nil, nil, "")
		composer := NewComposer(processor, "")

		roleName, resolutions, err := composer.selectDefaultRole(cfg)
		if err != nil {
			t.Fatalf("selectDefaultRole() error = %v", err)
		}

		if roleName != "fallback" {
			t.Errorf("roleName = %q, want 'fallback'", roleName)
		}

		if len(resolutions) != 2 {
			t.Fatalf("expected 2 resolutions, got %d", len(resolutions))
		}

		if resolutions[0].Status != "skipped" {
			t.Errorf("first resolution status = %q, want 'skipped'", resolutions[0].Status)
		}
		if resolutions[1].Status != "loaded" {
			t.Errorf("second resolution status = %q, want 'loaded'", resolutions[1].Status)
		}
	})

	t.Run("required role errors when file missing", func(t *testing.T) {
		config := `
			roles: {
				"required-missing": {
					file: "/nonexistent/path/role.md"
					optional: false
				}
			}
		`
		cfg := ctx.CompileString(config)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		processor := NewTemplateProcessor(nil, nil, "")
		composer := NewComposer(processor, "")

		_, resolutions, err := composer.selectDefaultRole(cfg)
		if err == nil {
			t.Error("expected error for required role with missing file")
		}

		if len(resolutions) == 0 {
			t.Fatal("expected at least one resolution")
		}
		if resolutions[0].Status != "error" {
			t.Errorf("resolution status = %q, want 'error'", resolutions[0].Status)
		}
	})

	t.Run("first valid role selected from chain", func(t *testing.T) {
		config := `
			roles: {
				"optional-missing-1": {
					file: "/nonexistent/1.md"
					optional: true
				}
				"optional-missing-2": {
					file: "/nonexistent/2.md"
					optional: true
				}
				"valid": {
					prompt: "Valid role"
				}
				"not-reached": {
					prompt: "Should not reach this"
				}
			}
		`
		cfg := ctx.CompileString(config)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		processor := NewTemplateProcessor(nil, nil, "")
		composer := NewComposer(processor, "")

		roleName, resolutions, err := composer.selectDefaultRole(cfg)
		if err != nil {
			t.Fatalf("selectDefaultRole() error = %v", err)
		}

		if roleName != "valid" {
			t.Errorf("roleName = %q, want 'valid'", roleName)
		}

		// Should have 3 resolutions (2 skipped + 1 loaded)
		if len(resolutions) != 3 {
			t.Fatalf("expected 3 resolutions, got %d", len(resolutions))
		}
	})

	t.Run("all optional missing returns error", func(t *testing.T) {
		config := `
			roles: {
				"optional-1": {
					file: "/nonexistent/1.md"
					optional: true
				}
				"optional-2": {
					file: "/nonexistent/2.md"
					optional: true
				}
			}
		`
		cfg := ctx.CompileString(config)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		processor := NewTemplateProcessor(nil, nil, "")
		composer := NewComposer(processor, "")

		_, _, err := composer.selectDefaultRole(cfg)
		if err == nil {
			t.Error("expected error when all optional roles fail")
		}
		if !strings.Contains(err.Error(), "no roles available") {
			t.Errorf("error = %q, want containing 'no roles available'", err.Error())
		}
	})

	t.Run("mixed optional and required chain", func(t *testing.T) {
		config := `
			roles: {
				"optional-missing": {
					file: "/nonexistent/optional.md"
					optional: true
				}
				"required-valid": {
					prompt: "Required role"
				}
			}
		`
		cfg := ctx.CompileString(config)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		processor := NewTemplateProcessor(nil, nil, "")
		composer := NewComposer(processor, "")

		roleName, resolutions, err := composer.selectDefaultRole(cfg)
		if err != nil {
			t.Fatalf("selectDefaultRole() error = %v", err)
		}

		if roleName != "required-valid" {
			t.Errorf("roleName = %q, want 'required-valid'", roleName)
		}

		if len(resolutions) != 2 {
			t.Fatalf("expected 2 resolutions, got %d", len(resolutions))
		}
		if resolutions[0].Status != "skipped" {
			t.Errorf("first status = %q, want 'skipped'", resolutions[0].Status)
		}
		if resolutions[1].Status != "loaded" {
			t.Errorf("second status = %q, want 'loaded'", resolutions[1].Status)
		}
	})
}

func TestComposeWithRole_OptionalBehavior(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()
	tmpDir := t.TempDir()

	// Create a real role file
	roleFile := filepath.Join(tmpDir, "role.md")
	if err := os.WriteFile(roleFile, []byte("Real role content"), 0644); err != nil {
		t.Fatalf("writing role file: %v", err)
	}

	t.Run("explicit role always errors on failure", func(t *testing.T) {
		config := `
			roles: {
				"optional-missing": {
					file: "/nonexistent/role.md"
					optional: true
				}
			}
		`
		cfg := ctx.CompileString(config)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		processor := NewTemplateProcessor(nil, nil, tmpDir)
		composer := NewComposer(processor, tmpDir)

		// Explicit role selection should error even if optional: true
		_, err := composer.ComposeWithRole(cfg, ContextSelection{}, "optional-missing", "")
		if err == nil {
			t.Error("expected error for explicit role with missing file, even if optional")
		}
	})

	t.Run("file path role resolution tracked", func(t *testing.T) {
		config := `{}`
		cfg := ctx.CompileString(config)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		processor := NewTemplateProcessor(nil, nil, tmpDir)
		composer := NewComposer(processor, tmpDir)

		result, err := composer.ComposeWithRole(cfg, ContextSelection{}, roleFile, "")
		if err != nil {
			t.Fatalf("ComposeWithRole() error = %v", err)
		}

		if len(result.RoleResolutions) == 0 {
			t.Error("expected role resolutions for file path role")
		}
		if result.Role != "Real role content" {
			t.Errorf("Role = %q, want 'Real role content'", result.Role)
		}
	})
}

func TestComposer_ResolveContext_Errors(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tests := []struct {
		name        string
		config      string
		contextName string
		wantErr     string
	}{
		{
			name: "context not found",
			config: `
				contexts: {
					env: { prompt: "Environment" }
				}
			`,
			contextName: "nonexistent",
			wantErr:     "context not found",
		},
		{
			name: "invalid UTD - no file, command, or prompt",
			config: `
				contexts: {
					invalid: {
						description: "This context has no UTD fields"
					}
				}
			`,
			contextName: "invalid",
			wantErr:     "invalid UTD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ctx.CompileString(tt.config)
			if err := cfg.Err(); err != nil {
				t.Fatalf("compile config: %v", err)
			}

			processor := NewTemplateProcessor(nil, nil, "")
			composer := NewComposer(processor, "")

			_, err := composer.resolveContext(cfg, tt.contextName)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestComposer_ResolveRole_Errors(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tests := []struct {
		name     string
		config   string
		roleName string
		wantErr  string
	}{
		{
			name: "role not found",
			config: `
				roles: {
					assistant: { prompt: "You are an assistant." }
				}
			`,
			roleName: "nonexistent",
			wantErr:  "role not found",
		},
		{
			name: "invalid UTD - no file, command, or prompt",
			config: `
				roles: {
					invalid: {
						description: "This role has no UTD fields"
					}
				}
			`,
			roleName: "invalid",
			wantErr:  "invalid UTD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ctx.CompileString(tt.config)
			if err := cfg.Err(); err != nil {
				t.Fatalf("compile config: %v", err)
			}

			processor := NewTemplateProcessor(nil, nil, "")
			composer := NewComposer(processor, "")

			_, _, err := composer.resolveRole(cfg, tt.roleName)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestComposer_ResolveTask_TempFile(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	// Create two separate temp directories:
	// - workingDir: the project working directory
	// - externalDir: simulates CUE cache (outside working directory)
	workingDir := t.TempDir()
	externalDir := t.TempDir()

	// Create a source file in the external directory (simulating CUE cache)
	sourceFile := filepath.Join(externalDir, "task.md")
	if err := os.WriteFile(sourceFile, []byte("Task content here"), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	config := fmt.Sprintf(`
		tasks: {
			"test-task": {
				file: %q
				prompt: "Read {{.file}} for instructions."
			}
		}
	`, sourceFile)

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	result, err := composer.ResolveTask(cfg, "test-task", "")
	if err != nil {
		t.Fatalf("ResolveTask() error = %v", err)
	}

	// Verify temp file was created (because source is outside working directory)
	expectedTempPath := filepath.Join(workingDir, ".start", "temp", "task-test-task.md")
	if result.TempFile != expectedTempPath {
		t.Errorf("TempFile = %q, want %q", result.TempFile, expectedTempPath)
	}

	// Verify temp file exists and has correct content
	content, err := os.ReadFile(result.TempFile)
	if err != nil {
		t.Fatalf("reading temp file: %v", err)
	}
	if string(content) != "Task content here" {
		t.Errorf("temp file content = %q, want %q", string(content), "Task content here")
	}

	// Verify {{.file}} in prompt contains temp file path (for external files).
	// External files (outside working directory) use temp path because the
	// original location may be inaccessible to AI agents (e.g., CUE cache).
	if !strings.Contains(result.Content, expectedTempPath) {
		t.Errorf("Content should contain temp file path %q, got: %s", expectedTempPath, result.Content)
	}
}

func TestComposer_ResolveTask_TempFile_WithSlashInName(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	// Create two separate temp directories:
	// - workingDir: the project working directory
	// - externalDir: simulates CUE cache (outside working directory)
	workingDir := t.TempDir()
	externalDir := t.TempDir()

	sourceFile := filepath.Join(externalDir, "task.md")
	if err := os.WriteFile(sourceFile, []byte("Nested task content"), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	config := fmt.Sprintf(`
		tasks: {
			"start/create-task": {
				file: %q
				prompt: "Read {{.file}} for instructions."
			}
		}
	`, sourceFile)

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	result, err := composer.ResolveTask(cfg, "start/create-task", "")
	if err != nil {
		t.Fatalf("ResolveTask() error = %v", err)
	}

	// Verify filename derivation handles slashes (converted to dashes)
	expectedTempPath := filepath.Join(workingDir, ".start", "temp", "task-start-create-task.md")
	if result.TempFile != expectedTempPath {
		t.Errorf("TempFile = %q, want %q", result.TempFile, expectedTempPath)
	}
}

func TestComposer_ResolveTask_NoFile_NoTempFile(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()
	tmpDir := t.TempDir()

	config := `
		tasks: {
			"prompt-only": {
				prompt: "This task has no file."
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, tmpDir)
	composer := NewComposer(processor, tmpDir)

	result, err := composer.ResolveTask(cfg, "prompt-only", "")
	if err != nil {
		t.Fatalf("ResolveTask() error = %v", err)
	}

	// Verify no temp file for prompt-only tasks
	if result.TempFile != "" {
		t.Errorf("TempFile should be empty for prompt-only task, got %q", result.TempFile)
	}
}

func TestComposer_ResolveContext_TempFile(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	// Create two separate temp directories:
	// - workingDir: the project working directory
	// - externalDir: simulates CUE cache (outside working directory)
	workingDir := t.TempDir()
	externalDir := t.TempDir()

	sourceFile := filepath.Join(externalDir, "context.md")
	if err := os.WriteFile(sourceFile, []byte("Context content"), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	config := fmt.Sprintf(`
		contexts: {
			"project-info": {
				required: true
				file: %q
			}
		}
	`, sourceFile)

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	result, err := composer.resolveContext(cfg, "project-info")
	if err != nil {
		t.Fatalf("resolveContext() error = %v", err)
	}

	// Verify temp file was created (because source is outside working directory)
	expectedTempPath := filepath.Join(workingDir, ".start", "temp", "context-project-info.md")
	if result.TempFile != expectedTempPath {
		t.Errorf("TempFile = %q, want %q", result.TempFile, expectedTempPath)
	}

	// Verify content
	if result.Content != "Context content" {
		t.Errorf("Content = %q, want %q", result.Content, "Context content")
	}
}

func TestComposer_CwdFile_NoTempFile(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()
	workingDir := t.TempDir()

	// Create a file within the working directory (cwd)
	sourceFile := filepath.Join(workingDir, "AGENTS.md")
	if err := os.WriteFile(sourceFile, []byte("Cwd file content"), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	config := fmt.Sprintf(`
		contexts: {
			"agents": {
				required: true
				file: %q
			}
		}
	`, sourceFile)

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compile config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	result, err := composer.resolveContext(cfg, "agents")
	if err != nil {
		t.Fatalf("resolveContext() error = %v", err)
	}

	// Verify NO temp file was created (files within cwd don't need temp copies)
	if result.TempFile != "" {
		t.Errorf("TempFile should be empty for cwd file, got %q", result.TempFile)
	}

	// Verify temp directory was not created
	tempDir := filepath.Join(workingDir, ".start", "temp")
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("temp directory should not exist for cwd files, but found: %s", tempDir)
	}

	// Verify content was still read correctly
	if result.Content != "Cwd file content" {
		t.Errorf("Content = %q, want %q", result.Content, "Cwd file content")
	}
}

func TestComposer_resolveFileToTemp(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create source file
	sourceFile := filepath.Join(tmpDir, "source.md")
	if err := os.WriteFile(sourceFile, []byte("Source content"), 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, tmpDir)
	composer := NewComposer(processor, tmpDir)

	t.Run("creates temp file with correct content", func(t *testing.T) {
		tempPath, err := composer.resolveFileToTemp("task", "test", sourceFile)
		if err != nil {
			t.Fatalf("resolveFileToTemp() error = %v", err)
		}

		content, err := os.ReadFile(tempPath)
		if err != nil {
			t.Fatalf("reading temp file: %v", err)
		}
		if string(content) != "Source content" {
			t.Errorf("content = %q, want %q", string(content), "Source content")
		}
	})

	t.Run("returns empty string for empty path", func(t *testing.T) {
		tempPath, err := composer.resolveFileToTemp("task", "test", "")
		if err != nil {
			t.Fatalf("resolveFileToTemp() error = %v", err)
		}
		if tempPath != "" {
			t.Errorf("tempPath = %q, want empty", tempPath)
		}
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		_, err := composer.resolveFileToTemp("task", "test", "/nonexistent/file.md")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}

func TestComposer_isCwdPath(t *testing.T) {
	t.Parallel()
	workingDir := "/home/user/project"
	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{
			name:     "empty path",
			filePath: "",
			want:     false,
		},
		{
			name:     "relative path - simple",
			filePath: "AGENTS.md",
			want:     true,
		},
		{
			name:     "relative path - with dot prefix",
			filePath: "./AGENTS.md",
			want:     true,
		},
		{
			name:     "relative path - subdirectory",
			filePath: "docs/guide.md",
			want:     true,
		},
		{
			name:     "relative path - with dot prefix subdirectory",
			filePath: "./docs/guide.md",
			want:     true,
		},
		{
			name:     "absolute path - child of working dir",
			filePath: "/home/user/project/docs/guide.md",
			want:     true,
		},
		{
			name:     "absolute path - deeply nested child",
			filePath: "/home/user/project/a/b/c/file.md",
			want:     true,
		},
		{
			name:     "absolute path - outside working dir",
			filePath: "/tmp/cache/file.md",
			want:     false,
		},
		{
			name:     "absolute path - sibling directory",
			filePath: "/home/user/other-project/file.md",
			want:     false,
		},
		{
			name:     "absolute path - parent directory",
			filePath: "/home/user/file.md",
			want:     false,
		},
		{
			name:     "absolute path - similar prefix but not child",
			filePath: "/home/user/project-other/file.md",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := composer.isCwdPath(tt.filePath)
			if got != tt.want {
				t.Errorf("isCwdPath(%q) = %v, want %v", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestComposer_TildeExpansion_Context(t *testing.T) {
	ctx := cuecontext.New()

	// Use isolated home directory to avoid writing to real home
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a temp file in fake home directory
	testFile := filepath.Join(home, ".start-test-context.md")
	if err := os.WriteFile(testFile, []byte("Tilde context content"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	workingDir := t.TempDir()

	// Config uses tilde path
	config := `
		contexts: {
			"tilde-test": {
				file: "~/.start-test-context.md"
				prompt: "Content: {{.file_contents}}"
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compiling config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	result, err := composer.resolveContext(cfg, "tilde-test")
	if err != nil {
		t.Fatalf("resolveContext() error = %v", err)
	}

	if !strings.Contains(result.Content, "Tilde context content") {
		t.Errorf("Content = %q, want to contain %q", result.Content, "Tilde context content")
	}
}

func TestComposer_TildeExpansion_Role(t *testing.T) {
	ctx := cuecontext.New()

	// Use isolated home directory to avoid writing to real home
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a temp file in fake home directory
	testFile := filepath.Join(home, ".start-test-role.md")
	if err := os.WriteFile(testFile, []byte("Tilde role content"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	workingDir := t.TempDir()

	// Config uses tilde path
	config := `
		roles: {
			"tilde-test": {
				file: "~/.start-test-role.md"
				prompt: "Role: {{.file_contents}}"
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compiling config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	content, _, err := composer.resolveRole(cfg, "tilde-test")
	if err != nil {
		t.Fatalf("resolveRole() error = %v", err)
	}

	if !strings.Contains(content, "Tilde role content") {
		t.Errorf("Content = %q, want to contain %q", content, "Tilde role content")
	}
}

func TestComposer_TildeExpansion_Task(t *testing.T) {
	ctx := cuecontext.New()

	// Use isolated home directory to avoid writing to real home
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create a temp file in fake home directory
	testFile := filepath.Join(home, ".start-test-task.md")
	if err := os.WriteFile(testFile, []byte("Tilde task content"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	workingDir := t.TempDir()

	// Config uses tilde path
	config := `
		tasks: {
			"tilde-test": {
				file: "~/.start-test-task.md"
				prompt: "Task: {{.file_contents}}"
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compiling config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	result, err := composer.ResolveTask(cfg, "tilde-test", "test instructions")
	if err != nil {
		t.Fatalf("ResolveTask() error = %v", err)
	}

	if !strings.Contains(result.Content, "Tilde task content") {
		t.Errorf("Content = %q, want to contain %q", result.Content, "Tilde task content")
	}
}

func TestComposer_TildeExpansion_FileNotFound(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()
	workingDir := t.TempDir()

	// Config uses tilde path to nonexistent file
	config := `
		contexts: {
			"missing": {
				file: "~/.start-nonexistent-file-12345.md"
				prompt: "Content: {{.file_contents}}"
			}
		}
	`

	cfg := ctx.CompileString(config)
	if err := cfg.Err(); err != nil {
		t.Fatalf("compiling config: %v", err)
	}

	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	_, err := composer.resolveContext(cfg, "missing")
	if err == nil {
		t.Error("expected error for nonexistent tilde path file")
	}
}

func TestComposer_ProcessContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		instructions string
		wantContains string
	}{
		{
			name:         "plain content without placeholders",
			content:      "Review this code for bugs.",
			instructions: "",
			wantContains: "Review this code for bugs.",
		},
		{
			name:         "content with instructions placeholder",
			content:      "Review code. Focus on: {{.instructions}}",
			instructions: "security issues",
			wantContains: "security issues",
		},
		{
			name:         "content with no matching placeholders",
			content:      "Simple content with no templates.",
			instructions: "ignored instructions",
			wantContains: "Simple content with no templates.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			processor := NewTemplateProcessor(nil, nil, "")
			composer := NewComposer(processor, "")

			result, err := composer.ProcessContent(tt.content, tt.instructions)
			if err != nil {
				t.Fatalf("ProcessContent() error: %v", err)
			}
			if tt.wantContains != "" && !strings.Contains(result.Content, tt.wantContains) {
				t.Errorf("ProcessContent() content = %q, want to contain %q", result.Content, tt.wantContains)
			}
		})
	}
}

func TestExtractOrigin(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	tests := []struct {
		name   string
		cueStr string
		want   string
	}{
		{
			name:   "extracts origin field",
			cueStr: `{ origin: "github.com/test/module@v0.1.0" }`,
			want:   "github.com/test/module@v0.1.0",
		},
		{
			name:   "returns empty for missing origin",
			cueStr: `{ description: "no origin here" }`,
			want:   "",
		},
		{
			name:   "returns empty for non-string origin",
			cueStr: `{ origin: 42 }`,
			want:   "",
		},
		{
			name:   "returns empty for empty struct",
			cueStr: `{}`,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := ctx.CompileString(tt.cueStr)
			if err := v.Err(); err != nil {
				t.Fatalf("compiling CUE: %v", err)
			}

			got := ExtractOrigin(v)
			if got != tt.want {
				t.Errorf("ExtractOrigin() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetCUECacheDir(t *testing.T) {
	t.Run("respects CUE_CACHE_DIR env var", func(t *testing.T) {
		t.Setenv("CUE_CACHE_DIR", "/custom/cue/cache")

		dir, err := GetCUECacheDir()
		if err != nil {
			t.Fatalf("GetCUECacheDir() error: %v", err)
		}
		if dir != "/custom/cue/cache" {
			t.Errorf("GetCUECacheDir() = %q, want %q", dir, "/custom/cue/cache")
		}
	})

	t.Run("falls back to user cache dir", func(t *testing.T) {
		t.Setenv("CUE_CACHE_DIR", "")

		dir, err := GetCUECacheDir()
		if err != nil {
			t.Fatalf("GetCUECacheDir() error: %v", err)
		}
		if !strings.HasSuffix(dir, string(filepath.Separator)+"cue") {
			t.Errorf("GetCUECacheDir() = %q, want suffix /cue", dir)
		}
	})
}

func TestResolveModulePath(t *testing.T) {
	t.Run("non-module path returned unchanged", func(t *testing.T) {
		result, err := ResolveModulePath("/some/absolute/path.md", "any-origin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "/some/absolute/path.md" {
			t.Errorf("got %q, want %q", result, "/some/absolute/path.md")
		}
	})

	t.Run("relative path returned unchanged", func(t *testing.T) {
		result, err := ResolveModulePath("relative/path.md", "any-origin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "relative/path.md" {
			t.Errorf("got %q, want %q", result, "relative/path.md")
		}
	})

	t.Run("resolves module path from cache", func(t *testing.T) {
		cacheDir := t.TempDir()
		t.Setenv("CUE_CACHE_DIR", cacheDir)

		origin := "github.com/test/tasks/mytask@v0.1.0"
		moduleDir := filepath.Join(cacheDir, "mod", "extract", "github.com", "test", "tasks", "mytask@v0.1.0")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("creating cache dir: %v", err)
		}
		promptFile := filepath.Join(moduleDir, "prompt.md")
		if err := os.WriteFile(promptFile, []byte("test"), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		result, err := ResolveModulePath("@module/prompt.md", origin)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != promptFile {
			t.Errorf("got %q, want %q", result, promptFile)
		}
	})

	t.Run("module not found in cache", func(t *testing.T) {
		cacheDir := t.TempDir()
		t.Setenv("CUE_CACHE_DIR", cacheDir)

		parentDir := filepath.Join(cacheDir, "mod", "extract", "github.com", "test", "tasks")
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			t.Fatalf("creating parent dir: %v", err)
		}

		_, err := ResolveModulePath("@module/prompt.md", "github.com/test/tasks/mytask@v0.1.0")
		if err == nil {
			t.Fatal("expected error for missing module")
		}
		if !strings.Contains(err.Error(), "not found in cache") {
			t.Errorf("error %q should mention 'not found in cache'", err.Error())
		}
	})

	t.Run("cache directory does not exist", func(t *testing.T) {
		cacheDir := t.TempDir()
		t.Setenv("CUE_CACHE_DIR", cacheDir)

		_, err := ResolveModulePath("@module/prompt.md", "github.com/test/tasks/mytask@v0.1.0")
		if err == nil {
			t.Fatal("expected error for missing cache directory")
		}
	})

	t.Run("origin without version", func(t *testing.T) {
		cacheDir := t.TempDir()
		t.Setenv("CUE_CACHE_DIR", cacheDir)

		moduleDir := filepath.Join(cacheDir, "mod", "extract", "github.com", "test", "tasks", "mytask@v0.2.0")
		if err := os.MkdirAll(moduleDir, 0755); err != nil {
			t.Fatalf("creating cache dir: %v", err)
		}

		result, err := ResolveModulePath("@module/data.cue", "github.com/test/tasks/mytask")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(moduleDir, "data.cue")
		if result != want {
			t.Errorf("got %q, want %q", result, want)
		}
	})

	t.Run("multiple cached versions uses exact origin version", func(t *testing.T) {
		cacheDir := t.TempDir()
		t.Setenv("CUE_CACHE_DIR", cacheDir)

		// Create two cached versions — v0.1.1 (old) and v0.1.2 (new)
		parentDir := filepath.Join(cacheDir, "mod", "extract", "github.com", "test", "tasks", "review")
		oldDir := filepath.Join(parentDir, "holistic@v0.1.1")
		newDir := filepath.Join(parentDir, "holistic@v0.1.2")
		if err := os.MkdirAll(oldDir, 0755); err != nil {
			t.Fatalf("creating old cache dir: %v", err)
		}
		if err := os.MkdirAll(newDir, 0755); err != nil {
			t.Fatalf("creating new cache dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(oldDir, "task.md"), []byte("old"), 0644); err != nil {
			t.Fatalf("writing old file: %v", err)
		}
		if err := os.WriteFile(filepath.Join(newDir, "task.md"), []byte("new"), 0644); err != nil {
			t.Fatalf("writing new file: %v", err)
		}

		// Origin points to v0.1.2 — should resolve to the new version
		origin := "github.com/test/tasks/review/holistic@v0.1.2"
		result, err := ResolveModulePath("@module/task.md", origin)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(newDir, "task.md")
		if result != want {
			t.Errorf("got %q, want %q", result, want)
		}

		// Verify it reads the new content, not the old
		content, err := os.ReadFile(result)
		if err != nil {
			t.Fatalf("reading resolved file: %v", err)
		}
		if string(content) != "new" {
			t.Errorf("got content %q, want %q", string(content), "new")
		}
	})

	t.Run("fallback scans for latest when exact version missing", func(t *testing.T) {
		cacheDir := t.TempDir()
		t.Setenv("CUE_CACHE_DIR", cacheDir)

		// Create two cached versions but origin points to a version not in cache
		parentDir := filepath.Join(cacheDir, "mod", "extract", "github.com", "test", "tasks")
		v1Dir := filepath.Join(parentDir, "mytask@v0.1.0")
		v2Dir := filepath.Join(parentDir, "mytask@v0.2.0")
		if err := os.MkdirAll(v1Dir, 0755); err != nil {
			t.Fatalf("creating v1 dir: %v", err)
		}
		if err := os.MkdirAll(v2Dir, 0755); err != nil {
			t.Fatalf("creating v2 dir: %v", err)
		}

		// Origin points to v0.3.0 which isn't cached — fallback should pick v0.2.0 (latest)
		origin := "github.com/test/tasks/mytask@v0.3.0"
		result, err := ResolveModulePath("@module/data.cue", origin)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(v2Dir, "data.cue")
		if result != want {
			t.Errorf("got %q, want %q", result, want)
		}
	})
}

func TestComposer_ModulePathResolutionError(t *testing.T) {
	// Use an empty cache dir so all @module/ resolution fails.
	cacheDir := t.TempDir()
	t.Setenv("CUE_CACHE_DIR", cacheDir)

	cctx := cuecontext.New()
	workingDir := t.TempDir()
	processor := NewTemplateProcessor(nil, nil, workingDir)
	composer := NewComposer(processor, workingDir)

	t.Run("context module path error captured in status", func(t *testing.T) {
		cfg := cctx.CompileString(`
			contexts: {
				"golang/assistant": {
					origin: "github.com/test/contexts/golang/assistant@v0.1.0"
					file: "@module/context.md"
				}
			}
		`)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		result, err := composer.Compose(cfg, ContextSelection{
			Tags: []string{"golang/assistant"},
		}, "")
		if err != nil {
			t.Fatalf("Compose() unexpected error: %v", err)
		}

		if len(result.Contexts) == 0 {
			t.Fatal("expected at least one context in result")
		}
		ctx := result.Contexts[0]
		if ctx.Status != "error" {
			t.Errorf("context status = %q, want %q", ctx.Status, "error")
		}
		if !strings.Contains(ctx.Error, "resolving module path") {
			t.Errorf("context error = %q, want it to contain %q", ctx.Error, "resolving module path")
		}
		if !strings.Contains(ctx.Error, "start assets add") {
			t.Errorf("context error = %q, want it to contain actionable hint", ctx.Error)
		}
	})

	t.Run("role module path error is fatal for explicit role", func(t *testing.T) {
		cfg := cctx.CompileString(`
			roles: {
				"golang/assistant": {
					origin: "github.com/test/roles/golang/assistant@v0.1.0"
					file: "@module/role.md"
				}
			}
		`)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		_, err := composer.ComposeWithRole(cfg, ContextSelection{}, "golang/assistant", "")
		if err == nil {
			t.Fatal("ComposeWithRole() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "resolving module path") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "resolving module path")
		}
		if !strings.Contains(err.Error(), "start assets add") {
			t.Errorf("error = %q, want it to contain actionable hint", err.Error())
		}
	})

	t.Run("task module path error is returned", func(t *testing.T) {
		cfg := cctx.CompileString(`
			tasks: {
				"golang/review": {
					origin: "github.com/test/tasks/golang/review@v0.1.0"
					file: "@module/task.md"
				}
			}
		`)
		if err := cfg.Err(); err != nil {
			t.Fatalf("compile config: %v", err)
		}

		_, err := composer.ResolveTask(cfg, "golang/review", "")
		if err == nil {
			t.Fatal("ResolveTask() expected error, got nil")
		}
		if !strings.Contains(err.Error(), "resolving module path") {
			t.Errorf("error = %q, want it to contain %q", err.Error(), "resolving module path")
		}
		if !strings.Contains(err.Error(), "start assets add") {
			t.Errorf("error = %q, want it to contain actionable hint", err.Error())
		}
	})
}
