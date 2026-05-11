package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
)

// setupTestConfig creates a temp directory with CUE config for testing.
//
// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.
func setupTestConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// CUE extracts registry modules with read-only permissions; chmod before TempDir cleanup.
	t.Cleanup(func() {
		_ = filepath.Walk(filepath.Join(dir, ".cache"), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			return os.Chmod(path, 0755)
		})
	})

	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
agents: {
	claude: {
		bin:         "claude"
		command:     "{{.bin}} --model {{.model}} '{{.prompt}}'"
		description: "Claude by Anthropic"
		default_model: "sonnet"
		models: {
			sonnet: "claude-sonnet-4-20250514"
			opus:   "claude-opus-4-20250514"
		}
		tags: ["anthropic", "claude", "ai"]
	}
}

contexts: {
	environment: {
		required: true
		file:     "~/context/ENVIRONMENT.md"
		prompt:   "Environment context loaded."
		tags:     ["system", "environment"]
	}
	"git-status": {
		default: true
		tags:    ["git", "vcs"]
		command: "git status --short"
		prompt:  "Git status output."
	}
}

roles: {
	assistant: {
		description: "General assistant"
		prompt:      "You are a helpful assistant."
	}
	"code-reviewer": {
		description: "Code reviewer"
		prompt:      "You are an expert code reviewer."
	}
}

tasks: {
	review: {
		description: "Review changes"
		role:        "code-reviewer"
		command:     "git diff --staged"
		prompt:      "Review: {{.command_output}}"
	}
}
`
	configPath := filepath.Join(startDir, "settings.cue")
	if err := os.WriteFile(configPath, []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)

	return dir
}

// setupTestConfigWithFiles creates a test config that includes file-based resources
// with actual readable files.
func setupTestConfigWithFiles(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	// Create a file-based role
	roleFile := filepath.Join(dir, "role.md")
	if err := os.WriteFile(roleFile, []byte("You are a Go expert."), 0644); err != nil {
		t.Fatalf("writing role file: %v", err)
	}

	// Create a file-based context
	contextFile := filepath.Join(dir, "context.md")
	if err := os.WriteFile(contextFile, []byte("Project context info."), 0644); err != nil {
		t.Fatalf("writing context file: %v", err)
	}

	cueConfig := `
agents: {
	claude: {
		bin:         "claude"
		command:     "{{.bin}} '{{.prompt}}'"
		description: "Claude by Anthropic"
	}
}

roles: {
	"go-expert": {
		description: "Go language expert"
		file:        "` + roleFile + `"
	}
}

contexts: {
	project: {
		description: "Project context"
		file:        "` + contextFile + `"
		tags:        ["project"]
	}
}

tasks: {
	review: {
		description: "Review changes"
		command:     "git diff --staged"
		prompt:      "Review the changes."
	}
}
`
	configPath := filepath.Join(startDir, "settings.cue")
	if err := os.WriteFile(configPath, []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)

	return dir
}

// setupTestConfigWithOrigin creates a test config with origin fields for testing
// verbose dump of registry-installed modules.
func setupTestConfigWithOrigin(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
roles: {
	"golang/assistant": {
		description: "Go assistant"
		origin:      "github.com/start-cli/library/roles/golang@v1.0.0"
		prompt:      "You are a Go assistant."
	}
}
`
	configPath := filepath.Join(startDir, "settings.cue")
	if err := os.WriteFile(configPath, []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)

	return dir
}

// TestPrepareDescribeAgent tests the prepareDescribe function for agents.
func TestPrepareDescribeAgent(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		agentName    string
		scope        config.Scope
		wantType     string
		wantName     string
		wantAllNames []string
		wantReason   string
		wantErr      bool
	}{
		{
			name:         "no name describes first agent",
			agentName:    "",
			wantType:     "Agent",
			wantName:     "claude",
			wantAllNames: []string{"claude"},
			wantReason:   "first in config",
		},
		{
			name:      "named agent",
			agentName: "claude",
			wantType:  "Agent",
			wantName:  "claude",
		},
		{
			name:      "substring match",
			agentName: "clau",
			wantType:  "Agent",
			wantName:  "claude",
		},
		{
			name:      "nonexistent agent",
			agentName: "nonexistent",
			wantErr:   true,
		},
		{
			name:      "local flag returns local config",
			agentName: "claude",
			scope:     config.ScopeLocal,
			wantType:  "Agent",
			wantName:  "claude",
		},
		{
			name:      "global scope with no global config errors",
			agentName: "claude",
			scope:     config.ScopeGlobal,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := prepareDescribe(tt.agentName, tt.scope, internalcue.KeyAgents, "Agent")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ItemType != tt.wantType {
				t.Errorf("ItemType = %q, want %q", result.ItemType, tt.wantType)
			}
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if tt.wantReason != "" && result.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
			}
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
			// Verify Value is populated
			if !result.Value.Exists() {
				t.Error("Value should exist")
			}
		})
	}
}

// TestPrepareDescribeRole tests the prepareDescribe function for roles.
func TestPrepareDescribeRole(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		roleName     string
		wantType     string
		wantName     string
		wantAllNames []string
		wantReason   string
		wantErr      bool
	}{
		{
			name:         "no name describes first role",
			roleName:     "",
			wantType:     "Role",
			wantName:     "assistant",
			wantAllNames: []string{"assistant", "code-reviewer"},
			wantReason:   "first in config",
		},
		{
			name:     "named role with hyphen",
			roleName: "code-reviewer",
			wantType: "Role",
			wantName: "code-reviewer",
		},
		{
			name:     "substring match",
			roleName: "code",
			wantType: "Role",
			wantName: "code-reviewer",
		},
		{
			name:     "nonexistent role",
			roleName: "nonexistent",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := prepareDescribe(tt.roleName, config.ScopeMerged, internalcue.KeyRoles, "Role")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ItemType != tt.wantType {
				t.Errorf("ItemType = %q, want %q", result.ItemType, tt.wantType)
			}
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if tt.wantReason != "" && result.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
			}
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
		})
	}
}

// TestPrepareDescribeContext tests the prepareDescribe function for contexts.
func TestPrepareDescribeContext(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		contextName  string
		wantType     string
		wantName     string
		wantAllNames []string
		wantReason   string
		wantErr      bool
	}{
		{
			name:         "no name describes first context",
			contextName:  "",
			wantType:     "Context",
			wantName:     "environment",
			wantAllNames: []string{"environment", "git-status"},
			wantReason:   "first in config",
		},
		{
			name:        "context by name",
			contextName: "environment",
			wantType:    "Context",
			wantName:    "environment",
		},
		{
			name:        "substring match",
			contextName: "git",
			wantType:    "Context",
			wantName:    "git-status",
		},
		{
			name:        "nonexistent context",
			contextName: "nonexistent",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := prepareDescribe(tt.contextName, config.ScopeMerged, internalcue.KeyContexts, "Context")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ItemType != tt.wantType {
				t.Errorf("ItemType = %q, want %q", result.ItemType, tt.wantType)
			}
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if tt.wantReason != "" && result.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
			}
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
		})
	}
}

// TestPrepareDescribeTask tests the prepareDescribe function for tasks.
func TestPrepareDescribeTask(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		taskName     string
		wantType     string
		wantName     string
		wantAllNames []string
		wantReason   string
		wantErr      bool
	}{
		{
			name:         "no name describes first task",
			taskName:     "",
			wantType:     "Task",
			wantName:     "review",
			wantAllNames: []string{"review"},
			wantReason:   "first in config",
		},
		{
			name:     "task by name",
			taskName: "review",
			wantType: "Task",
			wantName: "review",
		},
		{
			name:     "nonexistent task",
			taskName: "nonexistent",
			wantErr:  true,
		},
		{
			name:     "substring match",
			taskName: "rev",
			wantType: "Task",
			wantName: "review",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := prepareDescribe(tt.taskName, config.ScopeMerged, internalcue.KeyTasks, "Task")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.ItemType != tt.wantType {
				t.Errorf("ItemType = %q, want %q", result.ItemType, tt.wantType)
			}
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if tt.wantReason != "" && result.Reason != tt.wantReason {
				t.Errorf("Reason = %q, want %q", result.Reason, tt.wantReason)
			}
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
		})
	}
}

// TestPrepareDescribeLocalNoConfig verifies that ScopeLocal returns an error when no local config exists.
func TestPrepareDescribeLocalNoConfig(t *testing.T) {
	dir := t.TempDir()
	// No .start directory created — local config is absent
	chdir(t, dir)
	t.Setenv("HOME", dir)

	_, err := prepareDescribe("claude", config.ScopeLocal, internalcue.KeyAgents, "Agent")
	if err == nil {
		t.Fatal("expected error when no local config exists")
	}
	if !strings.Contains(err.Error(), "no local configuration found") {
		t.Errorf("error should mention missing local config, got: %v", err)
	}
}

// TestPrepareDescribeGlobalNoConfig verifies that ScopeGlobal returns an error when no global config exists.
func TestPrepareDescribeGlobalNoConfig(t *testing.T) {
	dir := t.TempDir()
	// No ~/.config/start directory — global config is absent
	chdir(t, dir)
	t.Setenv("HOME", dir)

	_, err := prepareDescribe("claude", config.ScopeGlobal, internalcue.KeyAgents, "Agent")
	if err == nil {
		t.Fatal("expected error when no global config exists")
	}
	if !strings.Contains(err.Error(), "no global configuration found") {
		t.Errorf("error should mention missing global config, got: %v", err)
	}
}

// TestDescribeGlobalFlag verifies --global flag behaviour: listing and subcommands
// describe only global config, excluding local items.
func TestDescribeGlobalFlag(t *testing.T) {
	dir := t.TempDir()

	// Global config at ~/.config/start/
	globalStartDir := filepath.Join(dir, ".config", "start")
	if err := os.MkdirAll(globalStartDir, 0755); err != nil {
		t.Fatalf("creating global config dir: %v", err)
	}
	globalCueConfig := `
agents: {
	"global-agent": {
		bin:         "global"
		command:     "{{.bin}}"
		description: "Global agent"
	}
}
`
	if err := os.WriteFile(filepath.Join(globalStartDir, "settings.cue"), []byte(globalCueConfig), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	// Local config at ./.start/ with a different agent
	localStartDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(localStartDir, 0755); err != nil {
		t.Fatalf("creating local config dir: %v", err)
	}
	localCueConfig := `
agents: {
	"local-agent": {
		bin:         "local"
		command:     "{{.bin}}"
		description: "Local agent"
	}
}
`
	if err := os.WriteFile(filepath.Join(localStartDir, "settings.cue"), []byte(localCueConfig), 0644); err != nil {
		t.Fatalf("writing local config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)

	t.Run("listing shows only global items", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"describe", "--global"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "global-agent") {
			t.Errorf("output missing global-agent\ngot: %s", output)
		}
		if strings.Contains(output, "local-agent") {
			t.Errorf("output should not contain local-agent\ngot: %s", output)
		}
	})

	t.Run("describe agent name search with --global matches global-agent", func(t *testing.T) {
		buf := new(bytes.Buffer)
		cmd := NewRootCmd()
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"describe", "agent", "--global"})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "global-agent") {
			t.Errorf("output missing global-agent\ngot: %s", output)
		}
		if strings.Contains(output, "local-agent") {
			t.Errorf("output should not contain local-agent\ngot: %s", output)
		}
	})
}

// TestVerboseDumpCUEDefinition verifies CUE definition output in verbose dump.
func TestVerboseDumpCUEDefinition(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// CUE definition should contain struct markers and field names
	wantStrings := []string{
		"bin:",
		"command:",
		`"claude"`,
		"models:",
		"sonnet:",
		"opus:",
		"tags:",
	}
	for _, want := range wantStrings {
		if !strings.Contains(output, want) {
			t.Errorf("output missing CUE definition element %q\ngot:\n%s", want, output)
		}
	}
}

// TestVerboseDumpAgentCommand verifies that agent command templates have
// {{.bin}} and {{.model}} filled with resolved values in the verbose dump.
func TestVerboseDumpAgentCommand(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// bin and resolved model should appear in the Command line
	if !strings.Contains(output, "Command: claude --model claude-sonnet-4-20250514") {
		t.Errorf("Command line missing resolved bin and model\ngot:\n%s", output)
	}

	// Runtime placeholder should remain unfilled
	if !strings.Contains(output, "{{.prompt}}") {
		t.Errorf("Command line should retain {{.prompt}} placeholder\ngot:\n%s", output)
	}
}

// TestVerboseDumpConfigSource verifies config source path in verbose dump.
func TestVerboseDumpConfigSource(t *testing.T) {
	dir := setupTestConfig(t)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// Config source should show the .cue file path
	expectedPath := filepath.Join(dir, ".start", "settings.cue")
	if !strings.Contains(output, expectedPath) {
		t.Errorf("output missing config source path %q\ngot:\n%s", expectedPath, output)
	}

	// Config line should contain item name in parentheses
	if !strings.Contains(output, "claude") {
		t.Errorf("output missing item name 'claude'\ngot:\n%s", output)
	}
}

// TestVerboseDumpOriginCache verifies origin and cache display for registry modules.
func TestVerboseDumpOriginCache(t *testing.T) {
	setupTestConfigWithOrigin(t)

	result, err := prepareDescribe("golang/assistant", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// Origin should be displayed
	if !strings.Contains(output, "github.com/start-cli/library/roles/golang@v1.0.0") {
		t.Errorf("output missing origin\ngot:\n%s", output)
	}

	// Cache directory should contain mod/extract path
	if !strings.Contains(output, "mod/extract") {
		t.Errorf("output missing cache path\ngot:\n%s", output)
	}
}

// TestVerboseDumpFileContent verifies file content display.
func TestVerboseDumpFileContent(t *testing.T) {
	setupTestConfigWithFiles(t)

	result, err := prepareDescribe("go-expert", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// Should display the file content
	if !strings.Contains(output, "You are a Go expert.") {
		t.Errorf("output missing file content\ngot:\n%s", output)
	}
}

// TestVerboseDumpFileError verifies inline error for unreadable files.
func TestVerboseDumpFileError(t *testing.T) {
	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
roles: {
	broken: {
		description: "Broken role"
		file:        "/nonexistent/path/role.md"
	}
}
`
	configPath := filepath.Join(startDir, "settings.cue")
	if err := os.WriteFile(configPath, []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)

	result, err := prepareDescribe("broken", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// Should show inline error, not crash
	if !strings.Contains(output, "[error:") {
		t.Errorf("output missing inline error\ngot:\n%s", output)
	}
}

// TestVerboseDumpCommand verifies command display.
func TestVerboseDumpCommand(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("review", config.ScopeMerged, internalcue.KeyTasks, "Task")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// Should display command as string
	if !strings.Contains(output, "git diff --staged") {
		t.Errorf("output missing command\ngot:\n%s", output)
	}
}

// TestVerboseDumpSeparators verifies separator lines in verbose dump.
func TestVerboseDumpSeparators(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	separator := strings.Repeat("─", 79)
	count := strings.Count(output, separator)
	if count < 2 {
		t.Errorf("expected at least 2 separator lines, got %d\n%s", count, output)
	}
}

// TestDescribeListingDescriptions verifies enhanced listing with descriptions.
func TestDescribeListingDescriptions(t *testing.T) {
	setupTestConfig(t)

	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"describe"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Check category headers
	if !strings.Contains(output, "agents/") {
		t.Error("output missing agents/ header")
	}
	if !strings.Contains(output, "roles/") {
		t.Error("output missing roles/ header")
	}
	if !strings.Contains(output, "contexts/") {
		t.Error("output missing contexts/ header")
	}
	if !strings.Contains(output, "tasks/") {
		t.Error("output missing tasks/ header")
	}

	// Check names are present
	if !strings.Contains(output, "claude") {
		t.Error("output missing agent name 'claude'")
	}
	if !strings.Contains(output, "assistant") {
		t.Error("output missing role name 'assistant'")
	}

	// Check descriptions are present alongside names
	if !strings.Contains(output, "Claude by Anthropic") {
		t.Error("output missing description 'Claude by Anthropic'")
	}
	if !strings.Contains(output, "General assistant") {
		t.Error("output missing description 'General assistant'")
	}
	if !strings.Contains(output, "Review changes") {
		t.Error("output missing description 'Review changes'")
	}
}

// TestDescribeListingNoDescriptions verifies items without descriptions are listed.
func TestDescribeListingNoDescriptions(t *testing.T) {
	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	cueConfig := `
roles: {
	minimal: {
		prompt: "You are minimal."
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)

	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"describe"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "minimal") {
		t.Error("output missing item name 'minimal'")
	}
}

// TestDescribeCrossCategory verifies cross-category search with a single match.
func TestDescribeCrossCategory(t *testing.T) {
	setupTestConfig(t)

	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"describe", "claude"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Should show verbose dump for claude agent
	if !strings.Contains(output, "Agent: claude") {
		t.Errorf("output missing 'Agent: claude'\ngot:\n%s", output)
	}

	// Should contain CUE definition
	if !strings.Contains(output, "bin:") {
		t.Errorf("output missing CUE definition\ngot:\n%s", output)
	}
}

// TestDescribeCrossCategoryMultipleExact verifies ambiguity when an exact name exists
// in multiple categories (non-TTY returns error).
func TestDescribeCrossCategoryMultipleExact(t *testing.T) {
	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	// "helper" exists as both a role and a task
	cueConfig := `
roles: {
	helper: {
		description: "Helper role"
		prompt:      "You help."
	}
}
tasks: {
	helper: {
		description: "Helper task"
		prompt:      "Help."
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)

	buf := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	// stdin is not a TTY in tests, so multiple matches should return an error
	cmd.SetArgs([]string{"describe", "helper"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for ambiguous exact match in non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention ambiguity, got: %v", err)
	}
}

// TestDescribeCommandIntegration tests the full command flow via Cobra.
func TestDescribeCommandIntegration(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name           string
		args           []string
		wantOutput     []string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:           "describe agent is a name search returning not-found",
			args:           []string{"describe", "agent"},
			wantErr:        true,
			wantErrContain: "agent",
		},
		{
			name:           "describe role is a name search returning not-found",
			args:           []string{"describe", "role"},
			wantErr:        true,
			wantErrContain: "role",
		},
		{
			name:           "describe context is a name search returning not-found",
			args:           []string{"describe", "context"},
			wantErr:        true,
			wantErrContain: "context",
		},
		{
			name:           "describe task is a name search returning not-found",
			args:           []string{"describe", "task"},
			wantErr:        true,
			wantErrContain: "task",
		},
		{
			name:    "describe with two args is rejected",
			args:    []string{"describe", "claude", "extra"},
			wantErr: true,
		},
		{
			name:       "describe no args lists all items",
			args:       []string{"describe"},
			wantOutput: []string{"agents/", "roles/", "contexts/", "tasks/", "claude", "assistant"},
		},
		{
			name:       "describe cross-category search single match",
			args:       []string{"describe", "claude"},
			wantOutput: []string{"Agent: claude"},
		},
		{
			name:       "describe --local lists only local items",
			args:       []string{"describe", "--local"},
			wantOutput: []string{"agents/", "claude"},
		},
		{
			name:    "describe --global errors when no global config",
			args:    []string{"describe", "--global"},
			wantErr: true,
		},
		{
			name:    "describe --local and --global are mutually exclusive",
			args:    []string{"describe", "--local", "--global"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.wantErrContain != "" && !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			for _, want := range tt.wantOutput {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q\ngot: %s", want, output)
				}
			}
		})
	}
}

// TestFormatCUEDefinition verifies the CUE definition formatter.
func TestFormatCUEDefinition(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("assistant", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	def := formatCUEDefinition(result.Value)
	if def == "" {
		t.Fatal("formatCUEDefinition returned empty string")
	}

	// Should contain CUE syntax markers
	if !strings.Contains(def, "{") {
		t.Error("CUE definition missing struct marker '{'")
	}
	if !strings.Contains(def, "description:") {
		t.Error("CUE definition missing 'description:' field")
	}
	if !strings.Contains(def, "prompt:") {
		t.Error("CUE definition missing 'prompt:' field")
	}
}

// TestResolveDescribeFile verifies file resolution for different path types.
func TestResolveDescribeFile(t *testing.T) {
	// Create a temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	t.Run("absolute path", func(t *testing.T) {
		resolvedPath, content, err := resolveDescribeFile(testFile, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if content != "test content" {
			t.Errorf("content = %q, want %q", content, "test content")
		}
		if resolvedPath != testFile {
			t.Errorf("resolvedPath = %q, want %q", resolvedPath, testFile)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, _, err := resolveDescribeFile("/nonexistent/file.md", "")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("empty file path", func(t *testing.T) {
		resolvedPath, content, err := resolveDescribeFile("", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resolvedPath != "" || content != "" {
			t.Errorf("expected empty results for empty path, got path=%q content=%q", resolvedPath, content)
		}
	})
}

// TestDeriveCacheDir verifies cache directory derivation from origin.
func TestDeriveCacheDir(t *testing.T) {
	t.Run("origin with version", func(t *testing.T) {
		result := deriveCacheDir("github.com/start-cli/library/roles/golang@v1.0.0")
		if result == "" {
			t.Error("expected non-empty cache dir")
		}
		if !strings.Contains(result, "mod/extract") {
			t.Errorf("cache dir missing mod/extract: %s", result)
		}
		if !strings.Contains(result, "golang@v1.0.0") {
			t.Errorf("cache dir missing versioned module name: %s", result)
		}
	})

	t.Run("origin without version", func(t *testing.T) {
		result := deriveCacheDir("github.com/start-cli/library/roles/golang")
		if result != "" {
			t.Errorf("expected empty cache dir for unversioned origin, got %q", result)
		}
	})
}

// TestNotifyScopeWidenedIfLocal pins the conditions under which the notice
// fires. The message itself carries grep-able text that scripted callers may
// use to detect silent --local widening; the test asserts both presence of
// that text in positive cases and absence in negative cases.
func TestNotifyScopeWidenedIfLocal(t *testing.T) {
	t.Parallel()

	const wantText = "--local widened to merged scope"

	cases := []struct {
		name       string
		flags      *Flags
		didInstall bool
		wantNotice bool
	}{
		{"local + install -> notice", &Flags{Local: true}, true, true},
		{"local + no install -> no notice", &Flags{Local: true}, false, false},
		{"no flag + install -> no notice", &Flags{}, true, false},
		{"local + install + quiet -> no notice", &Flags{Local: true, Quiet: true}, true, false},
		{"global + install -> no notice", &Flags{}, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			buf := new(bytes.Buffer)
			notifyScopeWidenedIfLocal(buf, tc.flags, tc.didInstall)
			got := buf.String()
			if tc.wantNotice {
				if !strings.Contains(got, wantText) {
					t.Errorf("expected notice containing %q, got %q", wantText, got)
				}
			} else {
				if got != "" {
					t.Errorf("expected no output, got %q", got)
				}
			}
		})
	}
}
