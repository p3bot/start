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

// setupTestConfig creates a temp directory with CUE config. Tests calling it
// use os.Chdir, so they must not run in parallel.
func setupTestConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// CUE extracts modules read-only; chmod before TempDir cleanup.
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

// setupTestConfigWithFiles creates a config with readable file-based resources.
func setupTestConfigWithFiles(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	roleFile := filepath.Join(dir, "role.md")
	if err := os.WriteFile(roleFile, []byte("You are a Go expert."), 0644); err != nil {
		t.Fatalf("writing role file: %v", err)
	}

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

// setupTestConfigWithOrigin creates a config with origin fields for testing the
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

func TestPrepareDescribeAgent(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		agentName    string
		scope        config.Scope
		wantType     string
		wantName     string
		wantAllNames []string
		wantErr      bool
	}{
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
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
			if !result.Value.Exists() {
				t.Error("Value should exist")
			}
		})
	}
}

func TestPrepareDescribeRole(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		roleName     string
		wantType     string
		wantName     string
		wantAllNames []string
		wantErr      bool
	}{
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
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
		})
	}
}

func TestPrepareDescribeContext(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		contextName  string
		wantType     string
		wantName     string
		wantAllNames []string
		wantErr      bool
	}{
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
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
		})
	}
}

func TestPrepareDescribeTask(t *testing.T) {
	setupTestConfig(t)

	tests := []struct {
		name         string
		taskName     string
		wantType     string
		wantName     string
		wantAllNames []string
		wantErr      bool
	}{
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
			if len(tt.wantAllNames) > 0 {
				if len(result.AllNames) != len(tt.wantAllNames) {
					t.Errorf("AllNames length = %d, want %d", len(result.AllNames), len(tt.wantAllNames))
				}
			}
		})
	}
}

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

// --global restricts listing and subcommands to global config, excluding local items.
func TestDescribeGlobalFlag(t *testing.T) {
	dir := t.TempDir()

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

// The settings block in the describe listing must honour --local/--global/no-flag.
func TestDescribeListingSettingsScope(t *testing.T) {
	dir := t.TempDir()

	globalStartDir := filepath.Join(dir, ".config", "start")
	if err := os.MkdirAll(globalStartDir, 0755); err != nil {
		t.Fatalf("creating global config dir: %v", err)
	}
	globalCueConfig := `
settings: {
	default_agent: "claude-global"
}
`
	if err := os.WriteFile(filepath.Join(globalStartDir, "settings.cue"), []byte(globalCueConfig), 0644); err != nil {
		t.Fatalf("writing global config: %v", err)
	}

	localStartDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(localStartDir, 0755); err != nil {
		t.Fatalf("creating local config dir: %v", err)
	}
	localCueConfig := `
settings: {
	default_agent: "gemini-local"
}
`
	if err := os.WriteFile(filepath.Join(localStartDir, "settings.cue"), []byte(localCueConfig), 0644); err != nil {
		t.Fatalf("writing local config: %v", err)
	}

	chdir(t, dir)
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	cases := []struct {
		name    string
		args    []string
		want    string
		notWant string
	}{
		{
			name:    "merged shows local (local wins)",
			args:    []string{"describe"},
			want:    "gemini-local",
			notWant: "claude-global",
		},
		{
			name:    "--local shows local only",
			args:    []string{"describe", "--local"},
			want:    "gemini-local",
			notWant: "claude-global",
		},
		{
			name:    "--global shows global only",
			args:    []string{"describe", "--global"},
			want:    "claude-global",
			notWant: "gemini-local",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			cmd := NewRootCmd()
			cmd.SetOut(buf)
			cmd.SetErr(buf)
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			output := buf.String()
			// Scope to the settings/ block so an agent name elsewhere in the
			// listing can't make the negative assertion pass spuriously.
			settingsBlock := extractDescribeSection(output, "settings")
			if settingsBlock == "" {
				t.Fatalf("settings block not found in output:\n%s", output)
			}
			if !strings.Contains(settingsBlock, tc.want) {
				t.Errorf("settings block missing %q\nblock:\n%s\nfull:\n%s", tc.want, settingsBlock, output)
			}
			if strings.Contains(settingsBlock, tc.notWant) {
				t.Errorf("settings block should not contain %q\nblock:\n%s", tc.notWant, settingsBlock)
			}
		})
	}
}

// extractDescribeSection returns the listing slice from the given category
// header to the next blank line, "" if absent. Anchoring on "\n<header>/\n"
// avoids collisions with filesystem paths in the Configuration Paths block.
func extractDescribeSection(output, header string) string {
	anchor := "\n" + header + "/\n"
	idx := strings.Index(output, anchor)
	if idx == -1 {
		return ""
	}
	rest := output[idx+1:] // skip the leading newline so the slice starts at the header
	if endIdx := strings.Index(rest, "\n\n"); endIdx != -1 {
		return rest[:endIdx]
	}
	return rest
}

func TestVerboseDumpCUEDefinition(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

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

func TestVerboseDumpAgentCommand(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	if !strings.Contains(output, "Command: claude --model claude-sonnet-4-20250514") {
		t.Errorf("Command line missing resolved bin and model\ngot:\n%s", output)
	}

	if !strings.Contains(output, "{{.prompt}}") {
		t.Errorf("Command line should retain {{.prompt}} placeholder\ngot:\n%s", output)
	}
}

func TestVerboseDumpConfigSource(t *testing.T) {
	dir := setupTestConfig(t)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	expectedPath := filepath.Join(dir, ".start", "settings.cue")
	if !strings.Contains(output, expectedPath) {
		t.Errorf("output missing config source path %q\ngot:\n%s", expectedPath, output)
	}

	if !strings.Contains(output, "claude") {
		t.Errorf("output missing item name 'claude'\ngot:\n%s", output)
	}
}

func TestVerboseDumpOriginCache(t *testing.T) {
	setupTestConfigWithOrigin(t)

	result, err := prepareDescribe("golang/assistant", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	if !strings.Contains(output, "github.com/start-cli/library/roles/golang@v1.0.0") {
		t.Errorf("output missing origin\ngot:\n%s", output)
	}

	if !strings.Contains(output, "mod/extract") {
		t.Errorf("output missing cache path\ngot:\n%s", output)
	}
}

func TestVerboseDumpFileContent(t *testing.T) {
	setupTestConfigWithFiles(t)

	result, err := prepareDescribe("go-expert", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	if !strings.Contains(output, "You are a Go expert.") {
		t.Errorf("output missing file content\ngot:\n%s", output)
	}
}

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

	if !strings.Contains(output, "[error:") {
		t.Errorf("output missing inline error\ngot:\n%s", output)
	}
}

func TestVerboseDumpCommand(t *testing.T) {
	setupTestConfig(t)

	result, err := prepareDescribe("review", config.ScopeMerged, internalcue.KeyTasks, "Task")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	if !strings.Contains(output, "git diff --staged") {
		t.Errorf("output missing command\ngot:\n%s", output)
	}
}

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

	if !strings.Contains(output, "claude") {
		t.Error("output missing agent name 'claude'")
	}
	if !strings.Contains(output, "assistant") {
		t.Error("output missing role name 'assistant'")
	}

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

	if !strings.Contains(output, "Agent: claude") {
		t.Errorf("output missing 'Agent: claude'\ngot:\n%s", output)
	}

	if !strings.Contains(output, "bin:") {
		t.Errorf("output missing CUE definition\ngot:\n%s", output)
	}
}

func TestDescribeCrossCategoryMultipleExact(t *testing.T) {
	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}

	// "helper" exists as both a role and a task.
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
	cmd.SetArgs([]string{"describe", "helper"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for ambiguous exact match in non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error should mention ambiguity, got: %v", err)
	}
}

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

func TestResolveDescribeFile(t *testing.T) {
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

// Pins when the notice fires; its grep-able text lets scripted callers detect
// silent --local widening.
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

// Asserts the labelled lines each category emits, and that the empty-fields
// case emits no block where every field is optional.
func TestVerboseDumpMetadataBlock(t *testing.T) {
	cases := []struct {
		name      string
		config    string
		queryName string
		cueKey    string
		itemType  string
		want      []string
		notWant   []string
	}{
		{
			name: "agent with all metadata fields",
			config: `
agents: {
	claude: {
		bin:           "claude"
		command:       "{{.bin}}"
		description:   "Claude by Anthropic"
		default_model: "sonnet"
		tags: ["anthropic", "ai"]
		models: {
			sonnet: "claude-sonnet-4-20250514"
			opus:   "claude-opus-4-20250514"
		}
	}
}`,
			queryName: "claude",
			cueKey:    internalcue.KeyAgents,
			itemType:  "Agent",
			want: []string{
				"Description: Claude by Anthropic",
				"Bin: claude",
				"Default Model: sonnet",
				"Tags: anthropic, ai",
				"Models:",
				"opus",
				"sonnet",
				"claude-sonnet-4-20250514",
			},
		},
		{
			name: "agent with no metadata fields emits no block",
			config: `
agents: {
	bare: {
		command: "bare"
	}
}`,
			queryName: "bare",
			cueKey:    internalcue.KeyAgents,
			itemType:  "Agent",
			notWant: []string{
				"Description:",
				"Bin:",
				"Default Model:",
				"Tags:",
				"Models:",
			},
		},
		{
			name: "role with all metadata fields",
			config: `
roles: {
	"code-reviewer": {
		description: "Reviews code"
		prompt:      "You are an expert code reviewer."
		optional:    true
		tags: ["review", "quality"]
	}
}`,
			queryName: "code-reviewer",
			cueKey:    internalcue.KeyRoles,
			itemType:  "Role",
			want: []string{
				"Description: Reviews code",
				"Prompt: You are an expert code reviewer.",
				"Optional: true",
				"Tags: review, quality",
			},
		},
		{
			name: "role with no metadata fields emits no block",
			config: `
roles: {
	bare: {
		prompt: ""
	}
}`,
			queryName: "bare",
			cueKey:    internalcue.KeyRoles,
			itemType:  "Role",
			notWant: []string{
				"Description:",
				"Prompt:",
				"Optional:",
				"Tags:",
			},
		},
		{
			name: "context with all metadata fields",
			config: `
contexts: {
	environment: {
		description: "Environment details"
		prompt:      "Environment context loaded."
		required:    true
		default:     true
		tags: ["system", "environment"]
	}
}`,
			queryName: "environment",
			cueKey:    internalcue.KeyContexts,
			itemType:  "Context",
			want: []string{
				"Description: Environment details",
				"Prompt: Environment context loaded.",
				"Required: true",
				"Default: true",
				"Tags: system, environment",
			},
		},
		{
			name: "context emits required and default even when fields absent",
			config: `
contexts: {
	bare: {
		prompt: ""
	}
}`,
			queryName: "bare",
			cueKey:    internalcue.KeyContexts,
			itemType:  "Context",
			want: []string{
				"Required: false",
				"Default: false",
			},
			notWant: []string{
				"Description:",
				"Tags:",
			},
		},
		{
			name: "task with all metadata fields",
			config: `
tasks: {
	review: {
		description: "Review staged changes"
		prompt:      "Review: {{.command_output}}"
		role:        "code-reviewer"
		tags: ["review", "git"]
	}
}`,
			queryName: "review",
			cueKey:    internalcue.KeyTasks,
			itemType:  "Task",
			want: []string{
				"Description: Review staged changes",
				"Prompt: Review: {{.command_output}}",
				"Role: code-reviewer",
				"Tags: review, git",
			},
		},
		{
			name: "task with no metadata fields emits no block",
			config: `
tasks: {
	bare: {
		prompt: ""
	}
}`,
			queryName: "bare",
			cueKey:    internalcue.KeyTasks,
			itemType:  "Task",
			notWant: []string{
				"Description:",
				"Prompt:",
				"Role:",
				"Tags:",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			startDir := filepath.Join(dir, ".start")
			if err := os.MkdirAll(startDir, 0755); err != nil {
				t.Fatalf("creating .start dir: %v", err)
			}
			configPath := filepath.Join(startDir, "settings.cue")
			if err := os.WriteFile(configPath, []byte(tc.config), 0644); err != nil {
				t.Fatalf("writing config: %v", err)
			}
			chdir(t, dir)
			t.Setenv("HOME", dir)

			result, err := prepareDescribe(tc.queryName, config.ScopeMerged, tc.cueKey, tc.itemType)
			if err != nil {
				t.Fatalf("prepareDescribe: %v", err)
			}

			var buf bytes.Buffer
			printMetadataBlock(&buf, result)
			output := buf.String()

			for _, want := range tc.want {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q\ngot:\n%s", want, output)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(output, notWant) {
					t.Errorf("output should not contain %q\ngot:\n%s", notWant, output)
				}
			}
		})
	}
}

func TestVerboseDumpMetadataBlock_ModelsSortedByAlias(t *testing.T) {
	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}
	cueConfig := `
agents: {
	multi: {
		command: "multi"
		models: {
			sonnet: "claude-sonnet"
			haiku:  "claude-haiku"
			opus:   "claude-opus"
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)
	t.Setenv("HOME", dir)

	result, err := prepareDescribe("multi", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printMetadataBlock(&buf, result)
	output := buf.String()

	haikuIdx := strings.Index(output, "haiku")
	opusIdx := strings.Index(output, "opus")
	sonnetIdx := strings.Index(output, "sonnet")

	if haikuIdx == -1 || opusIdx == -1 || sonnetIdx == -1 {
		t.Fatalf("output missing one or more aliases\n%s", output)
	}
	if haikuIdx >= opusIdx || opusIdx >= sonnetIdx {
		t.Errorf("models not sorted by alias: haiku=%d opus=%d sonnet=%d\n%s",
			haikuIdx, opusIdx, sonnetIdx, output)
	}
}

// The metadata block must land between the Cache line and the CUE Definition.
func TestVerboseDumpMetadataBlock_PlacementBetweenCacheAndCUE(t *testing.T) {
	setupTestConfigWithOrigin(t)

	result, err := prepareDescribe("golang/assistant", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	cacheIdx := strings.Index(output, "Cache:")
	if cacheIdx == -1 {
		t.Fatalf("output missing Cache line\n%s", output)
	}

	cueDefMarker := "{"
	cueIdx := strings.Index(output[cacheIdx:], cueDefMarker)
	if cueIdx == -1 {
		t.Fatalf("output missing CUE definition after Cache\n%s", output)
	}
	cueIdx += cacheIdx

	between := output[cacheIdx:cueIdx]
	if !strings.Contains(between, "Description:") {
		t.Errorf("metadata Description: should appear between Cache and CUE definition\nbetween:\n%s\nfull:\n%s", between, output)
	}
}

// With no metadata fields, printVerboseDump must emit no extra blank line before
// the CUE Definition. printMetadataBlock's empty case once emitted a bare "\n"
// producing a double blank line; this pins the buffer-length guard against it.
func TestVerboseDumpMetadataBlock_EmptyDoesNotInsertBlankLine(t *testing.T) {
	dir := t.TempDir()
	startDir := filepath.Join(dir, ".start")
	if err := os.MkdirAll(startDir, 0755); err != nil {
		t.Fatalf("creating .start dir: %v", err)
	}
	cueConfig := `
agents: {
	bare: {
		command: "bare"
	}
}
`
	if err := os.WriteFile(filepath.Join(startDir, "settings.cue"), []byte(cueConfig), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	chdir(t, dir)
	t.Setenv("HOME", dir)

	result, err := prepareDescribe("bare", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)
	output := buf.String()

	// Inspect the window from just after the separator line to the CUE
	// definition brace — the only segment printMetadataBlock could pad.
	sepIdx := strings.Index(output, "─")
	if sepIdx == -1 {
		t.Fatalf("output missing separator\n%s", output)
	}
	nlAfterSep := strings.IndexByte(output[sepIdx:], '\n')
	if nlAfterSep == -1 {
		t.Fatalf("separator without trailing newline\n%s", output)
	}
	windowStart := sepIdx + nlAfterSep + 1

	cueIdx := strings.Index(output[windowStart:], "{")
	if cueIdx == -1 {
		t.Fatalf("output missing CUE definition\n%s", output)
	}
	cueIdx += windowStart

	window := output[windowStart:cueIdx]
	// Three newlines = two stacked blank lines, the empty-metadata bug signature.
	if strings.Contains(window, "\n\n\n") {
		t.Errorf("found double blank line between separator and CUE definition:\nwindow:\n%q\nfull:\n%s", window, output)
	}
}
