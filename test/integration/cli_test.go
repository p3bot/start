//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/shell"
)

// chdir changes to the given directory and registers a cleanup to restore the original.
func chdir(t *testing.T, dir string) {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getting cwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("changing to dir %s: %v", dir, err)
	}
}

// setupTestConfig creates a temporary directory with a valid CUE config.
func setupTestConfig(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	config := `
agents: {
	echo: {
		bin: "echo"
		command: "{{.bin}} 'Agent executed'"
		default_model: "default"
		models: {
			default: "echo-model"
		}
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

contexts: {
	env: {
		required: true
		prompt: "Environment context"
	}
	project: {
		default: true
		prompt: "Project context"
	}
	debug: {
		tags: ["debug"]
		prompt: "Debug context"
	}
}

tasks: {
	"code-review": {
		role: "reviewer"
		prompt: """
			Review the code.
			Instructions: {{.instructions}}
			"""
	}
	"simple-task": {
		prompt: "Simple task prompt"
	}
}

settings: {
	default_agent: "echo"
}
`
	configFile := filepath.Join(configDir, "settings.cue")
	if err := os.WriteFile(configFile, []byte(config), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return tmpDir
}

// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

func TestIntegration_CUELoaderWithComposer(t *testing.T) {
	tmpDir := setupTestConfig(t)

	chdir(t, tmpDir)

	// Load CUE configuration
	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{filepath.Join(tmpDir, ".start")})
	if err != nil {
		t.Fatalf("loading CUE config: %v", err)
	}

	// Create composer with shell runner
	shellRunner := shell.NewRunner()
	processor := orchestration.NewTemplateProcessor(nil, shellRunner, tmpDir)
	composer := orchestration.NewComposer(processor, tmpDir)

	// Test composing with required contexts
	selection := orchestration.ContextSelection{
		IncludeRequired: true,
		IncludeDefaults: false,
	}

	composeResult, err := composer.Compose(result.Value, selection, "")
	if err != nil {
		t.Fatalf("composing: %v", err)
	}

	// Should have required context
	if len(composeResult.Contexts) != 1 {
		t.Errorf("expected 1 context, got %d", len(composeResult.Contexts))
	}
	if composeResult.Contexts[0].Name != "env" {
		t.Errorf("expected 'env' context, got %q", composeResult.Contexts[0].Name)
	}

	// Prompt should contain context content
	if !strings.Contains(composeResult.Prompt, "Environment context") {
		t.Errorf("prompt should contain 'Environment context'")
	}
}

func TestIntegration_CUELoaderWithComposer_DefaultContexts(t *testing.T) {
	tmpDir := setupTestConfig(t)

	chdir(t, tmpDir)

	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{filepath.Join(tmpDir, ".start")})
	if err != nil {
		t.Fatalf("loading CUE config: %v", err)
	}

	shellRunner := shell.NewRunner()
	processor := orchestration.NewTemplateProcessor(nil, shellRunner, tmpDir)
	composer := orchestration.NewComposer(processor, tmpDir)

	// Test composing with required and default contexts
	selection := orchestration.ContextSelection{
		IncludeRequired: true,
		IncludeDefaults: true,
	}

	composeResult, err := composer.Compose(result.Value, selection, "")
	if err != nil {
		t.Fatalf("composing: %v", err)
	}

	// Should have both required and default contexts
	if len(composeResult.Contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(composeResult.Contexts))
	}

	// Check both contexts are present
	var hasEnv, hasProject bool
	for _, ctx := range composeResult.Contexts {
		if ctx.Name == "env" {
			hasEnv = true
		}
		if ctx.Name == "project" {
			hasProject = true
		}
	}
	if !hasEnv {
		t.Error("expected 'env' context")
	}
	if !hasProject {
		t.Error("expected 'project' context")
	}
}

func TestIntegration_CUELoaderWithComposer_TaggedContexts(t *testing.T) {
	tmpDir := setupTestConfig(t)

	chdir(t, tmpDir)

	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{filepath.Join(tmpDir, ".start")})
	if err != nil {
		t.Fatalf("loading CUE config: %v", err)
	}

	shellRunner := shell.NewRunner()
	processor := orchestration.NewTemplateProcessor(nil, shellRunner, tmpDir)
	composer := orchestration.NewComposer(processor, tmpDir)

	// Test composing with tagged contexts
	selection := orchestration.ContextSelection{
		IncludeRequired: true,
		Tags:            []string{"debug"},
	}

	composeResult, err := composer.Compose(result.Value, selection, "")
	if err != nil {
		t.Fatalf("composing: %v", err)
	}

	// Should have required + debug tagged context
	if len(composeResult.Contexts) != 2 {
		t.Errorf("expected 2 contexts, got %d", len(composeResult.Contexts))
	}

	var hasDebug bool
	for _, ctx := range composeResult.Contexts {
		if ctx.Name == "debug" {
			hasDebug = true
		}
	}
	if !hasDebug {
		t.Error("expected 'debug' context from tag selection")
	}
}

func TestIntegration_ComposeWithRole(t *testing.T) {
	tmpDir := setupTestConfig(t)

	chdir(t, tmpDir)

	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{filepath.Join(tmpDir, ".start")})
	if err != nil {
		t.Fatalf("loading CUE config: %v", err)
	}

	shellRunner := shell.NewRunner()
	processor := orchestration.NewTemplateProcessor(nil, shellRunner, tmpDir)
	composer := orchestration.NewComposer(processor, tmpDir)

	selection := orchestration.ContextSelection{
		IncludeRequired: true,
	}

	// Test with specific role
	composeResult, err := composer.ComposeWithRole(result.Value, selection, "reviewer", "")
	if err != nil {
		t.Fatalf("composing with role: %v", err)
	}

	if composeResult.RoleName != "reviewer" {
		t.Errorf("expected role 'reviewer', got %q", composeResult.RoleName)
	}
	if !strings.Contains(composeResult.Role, "code reviewer") {
		t.Errorf("role content should contain 'code reviewer'")
	}
}

func TestIntegration_ResolveTask(t *testing.T) {
	tmpDir := setupTestConfig(t)

	chdir(t, tmpDir)

	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{filepath.Join(tmpDir, ".start")})
	if err != nil {
		t.Fatalf("loading CUE config: %v", err)
	}

	shellRunner := shell.NewRunner()
	processor := orchestration.NewTemplateProcessor(nil, shellRunner, tmpDir)
	composer := orchestration.NewComposer(processor, tmpDir)

	// Test resolving task with instructions
	taskResult, err := composer.ResolveTask(result.Value, "code-review", "focus on security")
	if err != nil {
		t.Fatalf("resolving task: %v", err)
	}

	if !strings.Contains(taskResult.Content, "Review the code") {
		t.Errorf("task content should contain 'Review the code'")
	}
	if !strings.Contains(taskResult.Content, "focus on security") {
		t.Errorf("task content should contain instructions 'focus on security'")
	}

	// Test simple task
	simpleResult, err := composer.ResolveTask(result.Value, "simple-task", "")
	if err != nil {
		t.Fatalf("resolving simple task: %v", err)
	}

	if simpleResult.Content != "Simple task prompt" {
		t.Errorf("expected 'Simple task prompt', got %q", simpleResult.Content)
	}
}

func TestIntegration_ExecutorBuildCommand(t *testing.T) {
	tmpDir := setupTestConfig(t)

	chdir(t, tmpDir)

	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{filepath.Join(tmpDir, ".start")})
	if err != nil {
		t.Fatalf("loading CUE config: %v", err)
	}

	// Extract agent
	agent, err := orchestration.ExtractAgent(result.Value, "echo")
	if err != nil {
		t.Fatalf("extracting agent: %v", err)
	}

	executor := orchestration.NewExecutor(tmpDir)

	cfg := orchestration.ExecuteConfig{
		Agent:      agent,
		Model:      "default",
		Role:       "Test role",
		Prompt:     "Test prompt",
		WorkingDir: tmpDir,
	}

	cmdStr, err := executor.BuildCommand(cfg)
	if err != nil {
		t.Fatalf("building command: %v", err)
	}

	if !strings.Contains(cmdStr, "echo") {
		t.Errorf("command should contain 'echo'")
	}
}

func TestIntegration_GetTaskRole(t *testing.T) {
	tmpDir := setupTestConfig(t)

	chdir(t, tmpDir)

	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{filepath.Join(tmpDir, ".start")})
	if err != nil {
		t.Fatalf("loading CUE config: %v", err)
	}

	// Task with role
	role := orchestration.GetTaskRole(result.Value, "code-review")
	if role != "reviewer" {
		t.Errorf("expected role 'reviewer', got %q", role)
	}

	// Task without role
	role = orchestration.GetTaskRole(result.Value, "simple-task")
	if role != "" {
		t.Errorf("expected empty role, got %q", role)
	}
}
