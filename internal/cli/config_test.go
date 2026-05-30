package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/config"
)

// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

func TestConfigListAgent_NoConfig(t *testing.T) {
	// Set up temp directory with no config
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "agent", "--local"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "agents") {
		t.Errorf("expected 'agents' section header, got: %s", output)
	}
}

func TestConfigListAgent_WithAgents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with an agent
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	agentsContent := `agents: {
	"claude": {
		bin: "claude"
		command: "claude \"{{.prompt}}\""
		description: "Anthropic Claude"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Save and restore working directory
	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "agent"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "claude") {
		t.Errorf("expected output to contain 'claude', got: %s", output)
	}
	if !strings.Contains(output, "Anthropic Claude") {
		t.Errorf("expected output to contain description, got: %s", output)
	}
}

func TestConfigGet_Agent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with an agent
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	agentsContent := `agents: {
	"claude": {
		bin: "claude"
		command: "claude --model {{.model}} \"{{.prompt}}\""
		default_model: "sonnet"
		description: "Anthropic Claude"
		models: {
			"sonnet": "claude-sonnet-4-20250514"
			"opus": "claude-opus-4-20250514"
		}
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "get", "claude"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "agents:claude") {
		t.Errorf("expected 'agents:claude', got: %s", output)
	}
	if !strings.Contains(output, "Bin:") {
		t.Errorf("expected 'Bin:', got: %s", output)
	}
	if !strings.Contains(output, "Default Model:") {
		t.Errorf("expected 'Default Model:', got: %s", output)
	}
	if !strings.Contains(output, "opus ->") {
		t.Errorf("expected models to include 'opus ->', got: %s", output)
	}
}

func TestConfigGet_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create empty global config
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte("agents: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "get", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent item")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestConfigRemove_Agent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with agents
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	agentsContent := `agents: {
	"claude": {
		bin: "claude"
		command: "claude \"{{.prompt}}\""
	}
	"gemini": {
		bin: "gemini"
		command: "gemini \"{{.prompt}}\""
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "remove", "gemini", "--force"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify gemini was removed
	content, err := os.ReadFile(filepath.Join(globalDir, "agents.cue"))
	if err != nil {
		t.Fatalf("failed to read agents.cue: %v", err)
	}

	if strings.Contains(string(content), `"gemini"`) {
		t.Errorf("expected gemini to be removed, but still present: %s", content)
	}
	if !strings.Contains(string(content), `"claude"`) {
		t.Errorf("expected claude to still be present: %s", content)
	}
}

func TestConfigSettings_DefaultAgentShow(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with settings
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `settings: {
	default_agent: "claude"
}`
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	// Use config settings to show the default_agent setting
	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "claude") {
		t.Errorf("expected 'claude' in output, got: %s", output)
	}
}

func TestConfigListRole_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "role", "--local"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "roles") {
		t.Errorf("expected 'roles' section header, got: %s", output)
	}
}

func TestConfigListContext_WithContexts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with a context
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	contextsContent := `contexts: {
	"project": {
		file: "PROJECT.md"
		description: "Project context"
		required: true
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "contexts.cue"), []byte(contextsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "context"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "project") {
		t.Errorf("expected output to contain 'project', got: %s", output)
	}
	if !strings.Contains(output, "[required]") {
		t.Errorf("expected output to contain '[required]' marker, got: %s", output)
	}
}

func TestConfigListContext_PreservesInjectionOrder(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with multiple contexts in specific order
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Define contexts in non-alphabetical order: zebra, alpha, middle
	contextsContent := `contexts: {
	"zebra": {
		file: "zebra.md"
		description: "Zebra context (defined first)"
	}
	"alpha": {
		file: "alpha.md"
		description: "Alpha context (defined second)"
	}
	"middle": {
		file: "middle.md"
		description: "Middle context (defined third)"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "contexts.cue"), []byte(contextsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "context"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()

	zebraIdx := strings.Index(output, "zebra")
	alphaIdx := strings.Index(output, "alpha")
	middleIdx := strings.Index(output, "middle")

	if zebraIdx == -1 || alphaIdx == -1 || middleIdx == -1 {
		t.Fatalf("expected all contexts in output, got: %s", output)
	}

	// Injection order: zebra < alpha < middle (matches config definition order)
	if zebraIdx >= alphaIdx || alphaIdx >= middleIdx {
		t.Errorf("context list not in injection order (expected zebra < alpha < middle): zebra=%d, alpha=%d, middle=%d\noutput: %s",
			zebraIdx, alphaIdx, middleIdx, output)
	}
}

func TestConfigListTask_WithTasks(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with a task
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	tasksContent := `tasks: {
	"review": {
		prompt: "Review this code"
		description: "Code review task"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "tasks.cue"), []byte(tasksContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "task"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "review") {
		t.Errorf("expected output to contain 'review', got: %s", output)
	}
	if !strings.Contains(output, "Code review task") {
		t.Errorf("expected output to contain description, got: %s", output)
	}
}

func TestConfigList_IncludesSettings(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config with a setting
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	settingsContent := `settings: {
	default_agent: "claude"
}`
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"), []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "settings") {
		t.Errorf("expected 'settings' section header, got: %s", output)
	}
	if !strings.Contains(output, "default_agent") {
		t.Errorf("expected 'default_agent' in settings output, got: %s", output)
	}
	if !strings.Contains(output, "claude") {
		t.Errorf("expected 'claude' value in settings output, got: %s", output)
	}
	if !strings.Contains(output, "global") {
		t.Errorf("expected 'global' source annotation in settings output, got: %s", output)
	}
}

func TestConfigList_SettingsDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "settings") {
		t.Errorf("expected 'settings' section header, got: %s", output)
	}
	if !strings.Contains(output, "timeout") {
		t.Errorf("expected 'timeout' in settings output, got: %s", output)
	}
	if !strings.Contains(output, "default") {
		t.Errorf("expected 'default' source annotation in settings output, got: %s", output)
	}
}

func TestWriteAgentsFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "agents.cue")

	agents := map[string]AgentConfig{
		"claude": {
			Name:         "claude",
			Bin:          "claude",
			Command:      `claude "{{.prompt}}"`,
			DefaultModel: "sonnet",
			Description:  "Anthropic Claude",
			Models: map[string]string{
				"sonnet": "claude-sonnet-4-20250514",
				"opus":   "claude-opus-4-20250514",
			},
		},
	}

	err := writeAgentsFile(path, agents)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, `"claude"`) {
		t.Errorf("expected file to contain 'claude', got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "bin:") {
		t.Errorf("expected file to contain 'bin:', got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "models:") {
		t.Errorf("expected file to contain 'models:', got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "Auto-generated") {
		t.Errorf("expected file to contain 'Auto-generated' header, got: %s", contentStr)
	}
}

func TestWriteRolesFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "roles.cue")

	roles := map[string]RoleConfig{
		"go-expert": {
			Name:        "go-expert",
			Description: "Go programming expert",
			File:        "~/.config/start/roles/go-expert.md",
		},
	}

	err := writeRolesFile(path, roles, []string{"go-expert"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, `"go-expert"`) {
		t.Errorf("expected file to contain 'go-expert', got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "file:") {
		t.Errorf("expected file to contain 'file:', got: %s", contentStr)
	}
}

func TestWriteContextsFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "contexts.cue")

	contexts := map[string]ContextConfig{
		"project": {
			Name:        "project",
			Description: "Project context",
			File:        "PROJECT.md",
			Required:    true,
		},
	}

	err := writeContextsFile(path, contexts, []string{"project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, `"project"`) {
		t.Errorf("expected file to contain 'project', got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "required: true") {
		t.Errorf("expected file to contain 'required: true', got: %s", contentStr)
	}
}

func TestWriteTasksFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "tasks.cue")

	tasks := map[string]TaskConfig{
		"review": {
			Name:        "review",
			Description: "Code review",
			Prompt:      "Review this code for bugs",
			Role:        "code-reviewer",
		},
	}

	err := writeTasksFile(path, tasks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, `"review"`) {
		t.Errorf("expected file to contain 'review', got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "prompt:") {
		t.Errorf("expected file to contain 'prompt:', got: %s", contentStr)
	}
	if !strings.Contains(contentStr, "role:") {
		t.Errorf("expected file to contain 'role:', got: %s", contentStr)
	}
}

func TestTruncatePrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		max      int
		expected string
	}{
		{"short string unchanged", "short", 10, "short"},
		{"long string truncated with ellipsis", "this is a longer string", 10, "this is..."},
		{"newlines replaced with spaces", "with\nnewlines", 20, "with newlines"},
		{"empty string unchanged", "", 10, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := truncatePrompt(tc.input, tc.max)
			if result != tc.expected {
				t.Errorf("truncatePrompt(%q, %d) = %q, want %q", tc.input, tc.max, result, tc.expected)
			}
		})
	}
}

func TestScopeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		local bool
		want  string
	}{
		{"local scope", true, "local"},
		{"global scope", false, "global"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopeString(tt.local)
			if got != tt.want {
				t.Errorf("scopeString(%v) = %q, want %q", tt.local, got, tt.want)
			}
		})
	}
}

// Settings command tests

func TestConfigSettingsList_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	// Should show config paths
	if !strings.Contains(output, "Configuration Paths:") {
		t.Errorf("expected config paths header, got: %s", output)
	}
	// Should show all settings with defaults or not set
	if !strings.Contains(output, "library_index:") {
		t.Errorf("expected library_index in output, got: %s", output)
	}
	if !strings.Contains(output, "default_agent:") {
		t.Errorf("expected default_agent in output, got: %s", output)
	}
	if !strings.Contains(output, "timeout:") {
		t.Errorf("expected timeout in output, got: %s", output)
	}
	// default_agent has no default
	if !strings.Contains(output, "(not set)") {
		t.Errorf("expected '(not set)' for default_agent, got: %s", output)
	}
	// timeout has a default
	if !strings.Contains(output, "(default)") {
		t.Errorf("expected '(default)' annotation, got: %s", output)
	}
}

func TestConfigSettingsList_NoCUEFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config dir with no CUE files
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write a non-CUE file so the dir exists but has no CUE
	if err := os.WriteFile(filepath.Join(globalDir, "README.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "library_index:") {
		t.Errorf("expected settings list even with no CUE files, got: %s", output)
	}
	if !strings.Contains(output, "(default)") {
		t.Errorf("expected default annotations, got: %s", output)
	}
}

func TestConfigSettingsList_WithSettings(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	settingsContent := `settings: {
	default_agent: "claude"
	timeout: 120
}`
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"), []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	// Configured settings show with global source
	if !strings.Contains(output, "default_agent: claude (global)") {
		t.Errorf("expected 'default_agent: claude (global)', got: %s", output)
	}
	if !strings.Contains(output, "timeout: 120 (global)") {
		t.Errorf("expected 'timeout: 120 (global)', got: %s", output)
	}
	// Unconfigured settings show defaults
	if !strings.Contains(output, "library_index:") {
		t.Errorf("expected library_index in output, got: %s", output)
	}
}

func TestConfigSettingsList_UnknownKey(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	settingsContent := `settings: {
	default_agent: "claude"
	custom_thing: "hello"
}`
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"), []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "custom_thing: hello (global, unknown key)") {
		t.Errorf("expected unknown key annotation, got: %s", output)
	}
	if !strings.Contains(output, "default_agent: claude (global)") {
		t.Errorf("expected known key without unknown annotation, got: %s", output)
	}
}

func TestConfigSettingsShow_SingleKey(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	settingsContent := `settings: {
	default_agent: "gemini"
}`
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"), []byte(settingsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "default_agent: gemini (global)") {
		t.Errorf("expected 'default_agent: gemini (global)', got: %s", output)
	}
}

func TestConfigSettingsShow_NotSet(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "default_agent: (not set)") {
		t.Errorf("expected 'default_agent: (not set)', got: %s", output)
	}
}

func TestConfigSettingsShow_Default(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "timeout"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "timeout: 30 (default)") {
		t.Errorf("expected 'timeout: 30 (default)', got: %s", output)
	}
}

func TestConfigSettingsShow_InvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"config", "settings", "invalid_key"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid key")
	}

	if !strings.Contains(err.Error(), "unknown setting") {
		t.Errorf("expected 'unknown setting' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Valid settings:") {
		t.Errorf("expected valid settings list in error, got: %v", err)
	}
}

func TestValidSettingsKeysString(t *testing.T) {
	result := config.ValidSettingsKeysString()

	// Must contain all known keys
	for key := range config.SettingsRegistry {
		if !strings.Contains(result, key) {
			t.Errorf("ValidSettingsKeysString() missing key %q, got: %s", key, result)
		}
	}

	// Must be sorted (first key alphabetically should appear before last)
	keys := strings.Split(result, ", ")
	if len(keys) != len(config.SettingsRegistry) {
		t.Errorf("ValidSettingsKeysString() returned %d keys, want %d", len(keys), len(config.SettingsRegistry))
	}
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			t.Errorf("ValidSettingsKeysString() not sorted: %q before %q", keys[i-1], keys[i])
		}
	}
}

func TestConfigSettingsSet_LibraryIndex(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "library_index", "github.com/example/custom/index@v0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file has quoted string value (not integer-style)
	settingsPath := filepath.Join(tmpDir, "start", "settings.cue")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	if !strings.Contains(string(content), `library_index: "github.com/example/custom/index@v0"`) {
		t.Errorf("settings file missing quoted library_index, content: %s", content)
	}
}

func TestResolveLibraryIndexPath(t *testing.T) {
	t.Run("returns configured value when set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		chdir(t, tmpDir)

		// Write settings via command
		cmd := NewRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs([]string{"config", "settings", "library_index", "github.com/example/custom/index@v0"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("set failed: %v", err)
		}

		got := resolveLibraryIndexPath()
		if got != "github.com/example/custom/index@v0" {
			t.Errorf("resolveLibraryIndexPath() = %q, want %q", got, "github.com/example/custom/index@v0")
		}
	})

	t.Run("returns empty string when not set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", tmpDir)

		chdir(t, tmpDir)

		got := resolveLibraryIndexPath()
		if got != "" {
			t.Errorf("resolveLibraryIndexPath() = %q, want empty string", got)
		}
	})
}

func TestConfigSettingsSet(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent", "claude"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, `Set default_agent to "claude"`) {
		t.Errorf("expected set confirmation, got: %s", output)
	}

	// Verify file was created
	settingsPath := filepath.Join(tmpDir, "start", "settings.cue")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	if !strings.Contains(string(content), `default_agent: "claude"`) {
		t.Errorf("settings file missing default_agent, content: %s", content)
	}
}

func TestConfigSettingsSet_Integer(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "timeout", "60"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file has integer value (no quotes)
	settingsPath := filepath.Join(tmpDir, "start", "settings.cue")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	if !strings.Contains(string(content), "timeout: 60") {
		t.Errorf("settings file missing timeout as integer, content: %s", content)
	}
	// Make sure it's NOT quoted
	if strings.Contains(string(content), `timeout: "60"`) {
		t.Errorf("timeout should not be quoted, content: %s", content)
	}
}

func TestConfigSettingsSet_InvalidValue(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"config", "settings", "timeout", "not-a-number"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-integer timeout")
	}

	if !strings.Contains(err.Error(), "requires an integer value") {
		t.Errorf("expected integer value error, got: %v", err)
	}
}

func TestHasNonSettingsContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "settings only",
			content: "settings: {\n\tdefault_agent: \"claude\"\n}",
			want:    false,
		},
		{
			name:    "with agents",
			content: "settings: {\n\tdefault_agent: \"claude\"\n}\nagents: {\n\tclaude: {}\n}",
			want:    true,
		},
		{
			name:    "with roles",
			content: "roles: {\n\tdev: {}\n}",
			want:    true,
		},
		{
			name:    "with contexts",
			content: "contexts: {\n\tenv: {}\n}",
			want:    true,
		},
		{
			name:    "with tasks",
			content: "tasks: {\n\tbuild: {}\n}",
			want:    true,
		},
		{
			name:    "empty file",
			content: "",
			want:    false,
		},
		{
			name:    "comments only",
			content: "// Auto-generated by start config\n// Settings file\n",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasNonSettingsContent(tt.content)
			if got != tt.want {
				t.Errorf("hasNonSettingsContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSettingsSet_RefusesOverwriteNonSettings(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create global config directory with mixed content
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a settings.cue file that also contains agents
	mixedContent := `settings: {
	default_agent: "claude"
}

agents: {
	claude: {
		bin: "claude"
		command: "{{.bin}}"
	}
}
`
	settingsPath := filepath.Join(globalDir, "settings.cue")
	if err := os.WriteFile(settingsPath, []byte(mixedContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent", "gemini"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when trying to overwrite file with non-settings content")
	}

	if !strings.Contains(err.Error(), "non-settings content") {
		t.Errorf("expected non-settings content error, got: %v", err)
	}

	// Verify original file is unchanged
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "agents:") {
		t.Error("original file content should be preserved")
	}
}

func TestConfigSettingsUnset(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	// First set a value
	setCmd := NewRootCmd()
	setCmd.SetOut(&bytes.Buffer{})
	setCmd.SetErr(&bytes.Buffer{})
	setCmd.SetArgs([]string{"config", "settings", "default_agent", "claude"})
	if err := setCmd.Execute(); err != nil {
		t.Fatalf("set failed: %v", err)
	}

	// Now unset it
	unsetCmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	unsetCmd.SetOut(stdout)
	unsetCmd.SetErr(&bytes.Buffer{})
	unsetCmd.SetArgs([]string{"config", "settings", "default_agent", "--unset"})
	if err := unsetCmd.Execute(); err != nil {
		t.Fatalf("unset failed: %v", err)
	}

	if !strings.Contains(stdout.String(), "Unset default_agent") {
		t.Errorf("expected unset confirmation, got: %s", stdout.String())
	}

	// Verify key is gone from file
	settingsPath := filepath.Join(tmpDir, "start", "settings.cue")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}
	if strings.Contains(string(content), "default_agent") {
		t.Errorf("settings.cue should not contain default_agent after unset, content: %s", content)
	}
}

func TestConfigSettingsUnset_NotSet(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	// Unset a key that was never set — should succeed gracefully
	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent", "--unset"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error unsetting non-existent key: %v", err)
	}

	if !strings.Contains(stdout.String(), "not set") {
		t.Errorf("expected 'not set' message, got: %s", stdout.String())
	}
}

func TestConfigSettingsUnset_MalformedCUE(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	// Write a malformed settings.cue directly
	configDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}
	malformed := []byte("settings: { this is not valid cue !!!")
	if err := os.WriteFile(filepath.Join(configDir, "settings.cue"), malformed, 0644); err != nil {
		t.Fatalf("writing malformed settings.cue: %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent", "--unset"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for malformed settings.cue, got nil")
	}
	if !strings.Contains(err.Error(), "loading settings") {
		t.Errorf("expected 'loading settings' error, got: %v", err)
	}
}

func TestConfigSettingsUnset_InvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "badkey", "--unset"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !strings.Contains(err.Error(), "unknown setting") {
		t.Errorf("expected unknown setting error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Valid settings:") {
		t.Errorf("expected valid settings list in error, got: %v", err)
	}
}

func TestConfigSettingsUnset_NoKey(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "--unset"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --unset used without a key")
	}
	if !strings.Contains(err.Error(), "--unset requires a setting key") {
		t.Errorf("expected requires key error, got: %v", err)
	}
}

// Tests for prompt helper functions

func TestPromptTags_KeepCurrent(t *testing.T) {
	current := []string{"tag1", "tag2"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("\n") // Just press Enter

	result, err := promptTags(stdout, stdin, current, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 || result[0] != "tag1" || result[1] != "tag2" {
		t.Errorf("expected current tags to be preserved, got: %v", result)
	}

	output := stdout.String()
	if !strings.Contains(output, "tag1, tag2") {
		t.Errorf("expected current tags in output, got: %s", output)
	}
}

func TestPromptTags_Clear(t *testing.T) {
	current := []string{"tag1", "tag2"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("-\n")

	result, err := promptTags(stdout, stdin, current, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty tags after clear, got: %v", result)
	}
}

func TestPromptTags_Replace(t *testing.T) {
	current := []string{"old1", "old2"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("new1, new2, new3\n")

	result, err := promptTags(stdout, stdin, current, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 || result[0] != "new1" || result[1] != "new2" || result[2] != "new3" {
		t.Errorf("expected new tags, got: %v", result)
	}
}

func TestPromptTags_Empty(t *testing.T) {
	var current []string
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("first, second\n")

	result, err := promptTags(stdout, stdin, current, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 || result[0] != "first" || result[1] != "second" {
		t.Errorf("expected new tags, got: %v", result)
	}

	output := stdout.String()
	if !strings.Contains(output, "(none)") {
		t.Errorf("expected '(none)' for empty current tags, got: %s", output)
	}
}

func TestPromptTags_NoShowCurrent(t *testing.T) {
	current := []string{"tag1", "tag2"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("new1\n")

	result, err := promptTags(stdout, stdin, current, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0] != "new1" {
		t.Errorf("expected [new1], got: %v", result)
	}
	output := stdout.String()
	if strings.Contains(output, "Current tags") {
		t.Errorf("expected no 'Current tags' line when showCurrent=false, got: %s", output)
	}
	if strings.Contains(output, "Enter to keep") {
		t.Errorf("expected no 'Enter to keep' hint when showCurrent=false, got: %s", output)
	}
	if !strings.Contains(output, "Enter to skip") {
		t.Errorf("expected 'Enter to skip' hint when showCurrent=false, got: %s", output)
	}
}

func TestPromptDefaultModel_NoModels(t *testing.T) {
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("gpt-4\n")

	result, err := promptDefaultModel(stdout, stdin, "gpt-3", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "gpt-4" {
		t.Errorf("expected 'gpt-4', got: %s", result)
	}

	output := stdout.String()
	if !strings.Contains(output, "Default model") {
		t.Errorf("expected promptString fallback, got: %s", output)
	}
}

func TestPromptDefaultModel_NoModelsKeepCurrent(t *testing.T) {
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("\n")

	result, err := promptDefaultModel(stdout, stdin, "gpt-3", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "gpt-3" {
		t.Errorf("expected 'gpt-3', got: %s", result)
	}
}

func TestPromptDefaultModel_SelectByNumber(t *testing.T) {
	models := map[string]string{
		"haiku":  "claude-3-5-haiku-20241022",
		"sonnet": "claude-3-7-sonnet-20250219",
		"opus":   "claude-opus-4-20250514",
	}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("3\n")

	result, err := promptDefaultModel(stdout, stdin, "sonnet", models)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Sorted: haiku=1, opus=2, sonnet=3
	if result != "sonnet" {
		t.Errorf("expected 'sonnet', got: %s", result)
	}

	output := stdout.String()
	if !strings.Contains(output, "1. haiku") {
		t.Errorf("expected numbered list, got: %s", output)
	}
	if !strings.Contains(output, "(current)") {
		t.Errorf("expected current marker, got: %s", output)
	}
}

func TestPromptDefaultModel_SelectByAlias(t *testing.T) {
	models := map[string]string{
		"haiku":  "claude-3-5-haiku-20241022",
		"sonnet": "claude-3-7-sonnet-20250219",
	}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("haiku\n")

	result, err := promptDefaultModel(stdout, stdin, "sonnet", models)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "haiku" {
		t.Errorf("expected 'haiku', got: %s", result)
	}
}

func TestPromptDefaultModel_SelectByAliasCaseInsensitive(t *testing.T) {
	models := map[string]string{
		"haiku":  "claude-3-5-haiku-20241022",
		"sonnet": "claude-3-7-sonnet-20250219",
	}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("HAIKU\n")

	result, err := promptDefaultModel(stdout, stdin, "sonnet", models)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "haiku" {
		t.Errorf("expected 'haiku', got: %s", result)
	}
}

func TestPromptDefaultModel_EnterKeepsCurrent(t *testing.T) {
	models := map[string]string{
		"haiku":  "claude-3-5-haiku-20241022",
		"sonnet": "claude-3-7-sonnet-20250219",
	}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("\n")

	result, err := promptDefaultModel(stdout, stdin, "sonnet", models)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "sonnet" {
		t.Errorf("expected 'sonnet', got: %s", result)
	}
}

func TestPromptDefaultModel_InvalidNumber(t *testing.T) {
	models := map[string]string{
		"haiku":  "claude-3-5-haiku-20241022",
		"sonnet": "claude-3-7-sonnet-20250219",
	}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("5\n")

	_, err := promptDefaultModel(stdout, stdin, "sonnet", models)
	if err == nil {
		t.Fatal("expected error for invalid number")
	}

	if !strings.Contains(err.Error(), "invalid selection") {
		t.Errorf("expected 'invalid selection' error, got: %v", err)
	}
}

func TestPromptDefaultModel_InvalidAlias(t *testing.T) {
	models := map[string]string{
		"haiku":  "claude-3-5-haiku-20241022",
		"sonnet": "claude-3-7-sonnet-20250219",
	}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("turbo\n")

	_, err := promptDefaultModel(stdout, stdin, "sonnet", models)
	if err == nil {
		t.Fatal("expected error for invalid alias")
	}

	if !strings.Contains(err.Error(), "not a known model alias") {
		t.Errorf("expected 'not a known model alias' error, got: %v", err)
	}
}

func TestPromptDefaultModel_NoCurrent(t *testing.T) {
	models := map[string]string{
		"haiku":  "claude-3-5-haiku-20241022",
		"sonnet": "claude-3-7-sonnet-20250219",
	}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("1\n")

	result, err := promptDefaultModel(stdout, stdin, "", models)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "haiku" {
		t.Errorf("expected 'haiku', got: %s", result)
	}

	output := stdout.String()
	if strings.Contains(output, "(current)") {
		t.Errorf("expected no current marker when no default set, got: %s", output)
	}
	if strings.Contains(output, "Enter to keep") {
		t.Errorf("expected no keep prompt when no default set, got: %s", output)
	}
}

func TestPromptModels_Keep(t *testing.T) {
	current := map[string]string{"fast": "gpt-4", "smart": "gpt-4-turbo"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("k\n")

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 || result["fast"] != "gpt-4" || result["smart"] != "gpt-4-turbo" {
		t.Errorf("expected current models preserved, got: %v", result)
	}
}

func TestPromptModels_KeepDefault(t *testing.T) {
	current := map[string]string{"fast": "gpt-4"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("\n") // Just Enter defaults to keep

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 || result["fast"] != "gpt-4" {
		t.Errorf("expected current models preserved, got: %v", result)
	}
}

func TestPromptModels_Clear(t *testing.T) {
	current := map[string]string{"fast": "gpt-4", "smart": "gpt-4-turbo"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("c\n\n") // clear then Enter to finish add-models loop

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected empty models after clear, got: %v", result)
	}
}

func TestPromptModels_ClearThenAdd(t *testing.T) {
	current := map[string]string{"fast": "gpt-4", "smart": "gpt-4-turbo"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("c\nnew=gpt-5\n\n") // clear, add one model, finish

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 || result["new"] != "gpt-5" {
		t.Errorf("expected one new model after clear-then-add, got: %v", result)
	}
}

func TestPromptModels_Edit_KeepExisting(t *testing.T) {
	current := map[string]string{"fast": "gpt-4"}
	stdout := &bytes.Buffer{}
	// Edit mode, keep fast, don't add new
	stdin := strings.NewReader("e\n\n\n")

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 || result["fast"] != "gpt-4" {
		t.Errorf("expected model kept, got: %v", result)
	}
}

func TestPromptModels_Edit_UpdateExisting(t *testing.T) {
	current := map[string]string{"fast": "gpt-4"}
	stdout := &bytes.Buffer{}
	// Edit mode, update fast to gpt-4-turbo, don't add new
	stdin := strings.NewReader("e\ngpt-4-turbo\n\n")

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 || result["fast"] != "gpt-4-turbo" {
		t.Errorf("expected model updated, got: %v", result)
	}
}

func TestPromptModels_Edit_DeleteExisting(t *testing.T) {
	current := map[string]string{"fast": "gpt-4", "slow": "gpt-3"}
	stdout := &bytes.Buffer{}
	// Edit mode, keep fast, delete slow, don't add new
	stdin := strings.NewReader("e\n\n-\n\n")

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 || result["fast"] != "gpt-4" {
		t.Errorf("expected only fast model kept, got: %v", result)
	}
	if _, exists := result["slow"]; exists {
		t.Errorf("expected slow model deleted, got: %v", result)
	}
}

func TestPromptModels_Edit_AddNew(t *testing.T) {
	current := map[string]string{"fast": "gpt-4"}
	stdout := &bytes.Buffer{}
	// Edit mode, keep fast, add new model
	stdin := strings.NewReader("e\n\nreasoning=o1\n\n")

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 models, got: %v", result)
	}
	if result["fast"] != "gpt-4" {
		t.Errorf("expected fast model preserved, got: %v", result)
	}
	if result["reasoning"] != "o1" {
		t.Errorf("expected reasoning model added, got: %v", result)
	}
}

func TestPromptModels_Empty(t *testing.T) {
	var current map[string]string
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("e\nnew=model-id\n\n")

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 || result["new"] != "model-id" {
		t.Errorf("expected new model added, got: %v", result)
	}

	output := stdout.String()
	if !strings.Contains(output, "(none)") {
		t.Errorf("expected '(none)' for empty current models, got: %s", output)
	}
}

func TestPromptModels_InvalidChoice(t *testing.T) {
	current := map[string]string{"fast": "gpt-4"}
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("x\n")

	_, err := promptModels(stdout, stdin, current)
	if err == nil {
		t.Fatal("expected error for invalid choice")
	}
	if !strings.Contains(err.Error(), "invalid choice") {
		t.Errorf("expected 'invalid choice' error, got: %v", err)
	}
}

func TestPromptModels_Edit_InvalidFormat(t *testing.T) {
	var current map[string]string
	stdout := &bytes.Buffer{}
	// Try invalid format, then valid, then finish
	stdin := strings.NewReader("e\ninvalid-no-equals\nvalid=model\n\n")

	result, err := promptModels(stdout, stdin, current)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Invalid format") {
		t.Errorf("expected invalid format message, got: %s", output)
	}

	if len(result) != 1 || result["valid"] != "model" {
		t.Errorf("expected valid model added despite earlier invalid input, got: %v", result)
	}
}

func TestPromptText_MultiLine(t *testing.T) {
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("line one\nline two\nline three\n\n")

	result, err := promptText(stdout, stdin, "Prompt text", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "line one\nline two\nline three"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}

	output := stdout.String()
	if !strings.Contains(output, "Prompt text") {
		t.Errorf("expected label in output, got: %s", output)
	}
	if !strings.Contains(output, "blank line to finish") {
		t.Errorf("expected instructions in output, got: %s", output)
	}
}

func TestPromptText_SingleLine(t *testing.T) {
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("just one line\n\n")

	result, err := promptText(stdout, stdin, "Prompt text", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "just one line" {
		t.Errorf("expected %q, got %q", "just one line", result)
	}
}

func TestPromptText_EmptyOpensEditor_FallsBackToDefault(t *testing.T) {
	// When first line is empty, promptText tries to open $EDITOR.
	// With an invalid editor, it falls back to returning defaultVal.
	t.Setenv("EDITOR", "/nonexistent-editor")
	t.Setenv("VISUAL", "")

	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("\n")

	result, err := promptText(stdout, stdin, "Prompt text", "fallback value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "fallback value" {
		t.Errorf("expected %q, got %q", "fallback value", result)
	}
}

func TestPromptText_EmptyNoDefault(t *testing.T) {
	// Empty input with no default returns empty string
	t.Setenv("EDITOR", "/nonexistent-editor")
	t.Setenv("VISUAL", "")

	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("\n")

	result, err := promptText(stdout, stdin, "Prompt text", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestPromptText_ShowsMultiLineDefault(t *testing.T) {
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("new text\n\n")

	_, err := promptText(stdout, stdin, "Prompt text", "line 1\nline 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Current value:") {
		t.Errorf("expected 'Current value:' for multi-line default, got: %s", output)
	}
	if !strings.Contains(output, "line 1\nline 2") {
		t.Errorf("expected default value shown, got: %s", output)
	}
}

func TestPromptText_ShowsSingleLineDefault(t *testing.T) {
	stdout := &bytes.Buffer{}
	stdin := strings.NewReader("new text\n\n")

	_, err := promptText(stdout, stdin, "Prompt text", "short default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "short default") {
		t.Errorf("expected single-line default in brackets, got: %s", output)
	}
	// Should NOT show "Current value:" for single-line defaults
	if strings.Contains(output, "Current value:") {
		t.Errorf("single-line default should not show 'Current value:', got: %s", output)
	}
}

func TestConfigListJSON_NoConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "--json", "--local"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output != "[]" {
		t.Errorf("expected empty JSON array, got: %s", output)
	}
}

func TestConfigListJSON_WithAgents(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	agentsContent := `agents: {
	"claude": {
		bin: "claude"
		command: "claude \"{{.prompt}}\""
		description: "Anthropic Claude"
		tags: ["ai", "llm"]
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &items); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	if len(items) == 0 {
		t.Fatal("expected at least one item in JSON output")
	}

	var found bool
	for _, item := range items {
		if item["name"] == "claude" {
			found = true
			if item["category"] != "agent" {
				t.Errorf("category = %q, want %q", item["category"], "agent")
			}
			if item["description"] != "Anthropic Claude" {
				t.Errorf("description = %q, want %q", item["description"], "Anthropic Claude")
			}
			if item["source"] == nil || item["source"] == "" {
				t.Error("source field should be non-empty")
			}
		}
	}
	if !found {
		t.Error("claude agent not found in JSON output")
	}
}

func TestConfigListJSON_CategoryFilter(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := `agents: {
	"claude": {
		bin: "claude"
		command: "claude"
	}
}
roles: {
	"assistant": {
		prompt: "You are helpful."
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "config.cue"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "list", "agent", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &items); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	for _, item := range items {
		if item["category"] != "agent" {
			t.Errorf("expected only agent items, got category %q", item["category"])
		}
	}
}

func TestConfigGetJSON_MultipleMatches(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// "go" matches both "golang" (role) and "go-review" (task)
	content := `roles: {
	"golang": {
		description: "Go programming expert"
		prompt: "You are a Go expert."
	}
}
tasks: {
	"go-review": {
		description: "Review Go code"
		prompt: "Review this Go code."
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "config.cue"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "get", "go", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &items); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	if len(items) < 2 {
		t.Fatalf("expected multiple matches, got %d: %s", len(items), stdout.String())
	}

	// All matches should have required fields
	for _, item := range items {
		if item["name"] == nil || item["name"] == "" {
			t.Error("item missing 'name' field")
		}
		if item["category"] == nil || item["category"] == "" {
			t.Error("item missing 'category' field")
		}
		if item["source"] == nil || item["source"] == "" {
			t.Error("item missing 'source' field")
		}
	}
}

func TestConfigGetJSON_WithMatch(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	agentsContent := `agents: {
	"claude": {
		bin: "claude"
		command: "claude \"{{.prompt}}\""
		description: "Anthropic Claude"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "get", "claude", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var items []map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &items); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	if items[0]["name"] != "claude" {
		t.Errorf("name = %q, want %q", items[0]["name"], "claude")
	}
	if items[0]["category"] != "agent" {
		t.Errorf("category = %q, want %q", items[0]["category"], "agent")
	}
}

func TestConfigGetJSON_NoArgs(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "get", "--json"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for no-args with --json")
	}
	if !strings.Contains(err.Error(), "query required") {
		t.Errorf("error should mention 'query required', got: %v", err)
	}
}

func TestConfigGetJSON_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "get", "nonexistent-item", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output != "[]" {
		t.Errorf("expected empty JSON array for not-found, got: %s", output)
	}
}

func TestConfigSettingsJSON_List(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var entries map[string]map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &entries); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	// All 4 valid setting keys should be present
	for _, key := range []string{"library_index", "default_agent", "shell", "timeout"} {
		if _, ok := entries[key]; !ok {
			t.Errorf("missing setting key %q in JSON output", key)
		}
	}

	// Each entry should have a source field
	for k, entry := range entries {
		if _, ok := entry["source"]; !ok {
			t.Errorf("setting %q missing 'source' field", k)
		}
	}
}

func TestConfigSettingsJSON_SingleKey(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"config", "settings", "default_agent", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &entry); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout.String())
	}

	if _, ok := entry["source"]; !ok {
		t.Error("entry missing 'source' field")
	}
}

// TestConfigGetScope verifies the four --local/--global scope outcomes for
// config get against a fixture that defines the same name in both global and
// local config with different field values.
func TestConfigGetScope(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Global config: agent "claude" with bin "claude-global".
	globalDir := filepath.Join(tmpDir, "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	globalCue := `agents: {
	"claude": {
		bin: "claude-global"
		command: "claude \"{{.prompt}}\""
		description: "Global Claude"
	}
}`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(globalCue), 0644); err != nil {
		t.Fatal(err)
	}

	// Local config: agent "claude" with bin "claude-local" (same name,
	// different field value), proving local-wins on merge.
	localDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	localCue := `agents: {
	"claude": {
		bin: "claude-local"
		command: "claude \"{{.prompt}}\""
		description: "Local Claude"
	}
}`
	if err := os.WriteFile(filepath.Join(localDir, "agents.cue"), []byte(localCue), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	run := func(t *testing.T, args ...string) (string, string, error) {
		t.Helper()
		cmd := NewRootCmd()
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		cmd.SetOut(stdout)
		cmd.SetErr(stderr)
		cmd.SetArgs(append([]string{"config", "get"}, args...))
		err := cmd.Execute()
		return stdout.String(), stderr.String(), err
	}

	t.Run("no flag returns merged with local winning", func(t *testing.T) {
		stdout, stderr, err := run(t, "claude")
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "claude-local") {
			t.Errorf("expected merged view to show local-wins bin, got: %s", stdout)
		}
		if strings.Contains(stdout, "claude-global") {
			t.Errorf("merged view should not show global bin when local overrides it, got: %s", stdout)
		}
		if !strings.Contains(stdout, "Source: local") {
			t.Errorf("expected 'Source: local' in merged view, got: %s", stdout)
		}
	})

	t.Run("--local returns local entry", func(t *testing.T) {
		stdout, stderr, err := run(t, "claude", "--local")
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "claude-local") {
			t.Errorf("expected local bin, got: %s", stdout)
		}
		if strings.Contains(stdout, "claude-global") {
			t.Errorf("--local should not show global bin, got: %s", stdout)
		}
		if !strings.Contains(stdout, "Source: local") {
			t.Errorf("expected 'Source: local', got: %s", stdout)
		}
	})

	t.Run("--global returns global entry", func(t *testing.T) {
		stdout, stderr, err := run(t, "claude", "--global")
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
		}
		if !strings.Contains(stdout, "claude-global") {
			t.Errorf("expected global bin, got: %s", stdout)
		}
		if strings.Contains(stdout, "claude-local") {
			t.Errorf("--global should not show local bin, got: %s", stdout)
		}
		if !strings.Contains(stdout, "Source: global") {
			t.Errorf("expected 'Source: global', got: %s", stdout)
		}
	})

	t.Run("--local --global is a mutual-exclusion error", func(t *testing.T) {
		stdout, _, err := run(t, "claude", "--local", "--global")
		if err == nil {
			t.Fatal("expected mutual-exclusion error, got nil")
		}
		if !strings.Contains(err.Error(), "local") || !strings.Contains(err.Error(), "global") {
			t.Errorf("error should mention both flags, got: %v", err)
		}
		if stdout != "" {
			t.Errorf("expected empty stdout on mutual-exclusion error, got: %q", stdout)
		}
	})

	t.Run("--json --global returns global entry with unchanged shape", func(t *testing.T) {
		stdout, stderr, err := run(t, "claude", "--json", "--global")
		if err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", err, stderr)
		}
		var items []map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &items); err != nil {
			t.Fatalf("invalid JSON: %v\noutput: %s", err, stdout)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0]["bin"] != "claude-global" {
			t.Errorf("expected global bin, got: %v", items[0]["bin"])
		}
		if items[0]["source"] != "global" {
			t.Errorf("expected source=global, got: %v", items[0]["source"])
		}
		for _, key := range []string{"name", "category", "source"} {
			if items[0][key] == nil || items[0][key] == "" {
				t.Errorf("required JSON field %q missing", key)
			}
		}
	})
}

func TestConfigGetScope_GlobalMissingFromLocalOnly(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	localDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	localOnly := `agents: {
	"only-local": {
		bin: "only-local"
		command: "only-local \"{{.prompt}}\""
	}
}`
	if err := os.WriteFile(filepath.Join(localDir, "agents.cue"), []byte(localOnly), 0644); err != nil {
		t.Fatal(err)
	}

	chdir(t, tmpDir)

	cmd := NewRootCmd()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"config", "get", "only-local", "--global"})
	err := cmd.Execute()

	if err == nil {
		t.Fatal("expected 'not found' error when item lives only in local")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
	if stdout.String() != "" {
		t.Errorf("expected empty stdout on not-found, got: %q", stdout.String())
	}
}

func TestConfigRemovedCommandPaths(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"config agent", []string{"config", "agent"}},
		{"config agent add", []string{"config", "agent", "add"}},
		{"config agent default", []string{"config", "agent", "default", "claude"}},
		{"config agent edit", []string{"config", "agent", "edit", "claude"}},
		{"config role add", []string{"config", "role", "add"}},
		{"config role list", []string{"config", "role", "list"}},
		{"config context order", []string{"config", "context", "order"}},
		{"config task remove review", []string{"config", "task", "remove", "review"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); err == nil {
				t.Errorf("expected error for removed command %v, got nil", tc.args)
			}
		})
	}
}
