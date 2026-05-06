package orchestration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/detection"
	"github.com/start-cli/start/internal/registry"
)

func TestNeedsSetup(t *testing.T) {
	tests := []struct {
		name     string
		paths    config.Paths
		expected bool
	}{
		{
			name:     "no config exists",
			paths:    config.Paths{GlobalExists: false, LocalExists: false},
			expected: true,
		},
		{
			name:     "global exists",
			paths:    config.Paths{GlobalExists: true, LocalExists: false},
			expected: false,
		},
		{
			name:     "local exists",
			paths:    config.Paths{GlobalExists: false, LocalExists: true},
			expected: false,
		},
		{
			name:     "both exist",
			paths:    config.Paths{GlobalExists: true, LocalExists: true},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NeedsSetup(tt.paths)
			if got != tt.expected {
				t.Errorf("NeedsSetup() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGenerateAgentCUE(t *testing.T) {
	agent := Agent{
		Name:         "claude",
		Bin:          "claude",
		Command:      "{{.bin}} --model {{.model}}",
		DefaultModel: "sonnet",
		Description:  "Anthropic Claude",
		Models: map[string]string{
			"sonnet": "claude-sonnet-4",
			"opus":   "claude-opus-4",
		},
	}

	content := generateAgentCUE(agent)

	// Check essential parts
	if !strings.Contains(content, `"claude"`) {
		t.Error("missing agent name")
	}
	if !strings.Contains(content, `bin:`) {
		t.Error("missing bin field")
	}
	if !strings.Contains(content, `command:`) {
		t.Error("missing command field")
	}
	if !strings.Contains(content, `default_model:`) {
		t.Error("missing default_model field")
	}
	if !strings.Contains(content, `models:`) {
		t.Error("missing models field")
	}
	if !strings.Contains(content, "Auto-generated") {
		t.Error("missing auto-generated comment")
	}
	// Settings should NOT be in agents.cue (it goes in settings.cue)
	if strings.Contains(content, `default_agent:`) {
		t.Error("default_agent should not be in agents.cue")
	}
}

func TestGenerateSettingsCUE(t *testing.T) {
	content := generateSettingsCUE("claude")

	if !strings.Contains(content, `default_agent: "claude"`) {
		t.Error("missing default_agent in settings")
	}
	if !strings.Contains(content, "Auto-generated") {
		t.Error("missing auto-generated comment")
	}
	if !strings.Contains(content, "settings:") {
		t.Error("missing settings block")
	}
}

func TestGenerateAgentCUE_MinimalAgent(t *testing.T) {
	agent := Agent{
		Name:    "test",
		Bin:     "test-bin",
		Command: "{{.bin}}",
	}

	content := generateAgentCUE(agent)

	// Check required fields are present
	if !strings.Contains(content, `bin:`) {
		t.Error("missing bin field")
	}
	if !strings.Contains(content, `command:`) {
		t.Error("missing command field")
	}

	// Check optional fields are not present when empty
	if strings.Contains(content, `default_model:`) {
		t.Error("should not have default_model when empty")
	}
	if strings.Contains(content, `description:`) {
		t.Error("should not have description when empty")
	}
}

func TestAutoSetup_NewAutoSetup(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	if as == nil {
		t.Fatal("NewAutoSetup returned nil")
	}
	if as.stdout != stdout {
		t.Error("stdout not set correctly")
	}
	if as.stderr != stderr {
		t.Error("stderr not set correctly")
	}
	if as.stdin != stdin {
		t.Error("stdin not set correctly")
	}
	if !as.isTTY {
		t.Error("isTTY not set correctly")
	}
}

func TestExtractAgentFromValue_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		cue     string
		wantErr string
	}{
		{
			name:    "missing bin",
			cue:     `command: "test"`,
			wantErr: "missing required 'bin' field",
		},
		{
			name:    "missing command",
			cue:     `bin: "test"`,
			wantErr: "missing required 'command' field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(tt.cue)
			if err := v.Err(); err != nil {
				t.Fatalf("failed to compile test CUE: %v", err)
			}

			_, err := extractAgentFromValue(v, "test")
			if err == nil {
				t.Error("expected error for missing required field")
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestExtractAgentFromValue_ValidAgent(t *testing.T) {
	cueSrc := `
bin: "claude"
command: "{{.bin}} --model {{.model}}"
default_model: "sonnet"
description: "Anthropic Claude"
models: {
	sonnet: "claude-sonnet-4"
	opus: "claude-opus-4"
}
`
	ctx := cuecontext.New()
	v := ctx.CompileString(cueSrc)
	if err := v.Err(); err != nil {
		t.Fatalf("failed to compile test CUE: %v", err)
	}

	agent, err := extractAgentFromValue(v, "claude")
	if err != nil {
		t.Fatalf("extractAgentFromValue failed: %v", err)
	}

	if agent.Name != "claude" {
		t.Errorf("wrong name: %s", agent.Name)
	}
	if agent.Bin != "claude" {
		t.Errorf("wrong bin: %s", agent.Bin)
	}
	if agent.DefaultModel != "sonnet" {
		t.Errorf("wrong default_model: %s", agent.DefaultModel)
	}
	if len(agent.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(agent.Models))
	}
}

func TestNoAgentsError(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false)

	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"ai/claude": {
				Module:      "github.com/test/claude@v0",
				Bin:         "claude",
				Description: "Anthropic Claude CLI",
			},
			"ai/gemini": {
				Module:      "github.com/test/gemini@v0",
				Bin:         "gemini",
				Description: "Google Gemini CLI",
			},
		},
	}

	err := as.noAgentsError(index)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()

	// Check for helpful message components
	if !strings.Contains(errMsg, "No AI CLI tools detected") {
		t.Error("error should mention no tools detected")
	}
	if !strings.Contains(errMsg, "Install one of") {
		t.Error("error should suggest installation")
	}
	if !strings.Contains(errMsg, "claude") {
		t.Error("error should list claude")
	}
	if !strings.Contains(errMsg, "gemini") {
		t.Error("error should list gemini")
	}
	if !strings.Contains(errMsg, "run 'start' again") {
		t.Error("error should suggest running start again")
	}
}

func TestNoAgentsError_EmptyIndex(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false)

	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{},
	}

	err := as.noAgentsError(index)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "No AI CLI tools detected") {
		t.Error("error should mention no tools detected")
	}
}

func TestPromptSelection_NonTTY(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false) // isTTY = false

	detected := []detection.DetectedAgent{
		{
			Key:   "ai/claude",
			Entry: registry.IndexEntry{Bin: "claude", Description: "Claude CLI"},
		},
		{
			Key:   "ai/gemini",
			Entry: registry.IndexEntry{Bin: "gemini", Description: "Gemini CLI"},
		},
	}

	_, err := as.promptSelection(detected)

	if err == nil {
		t.Fatal("expected error for non-TTY with multiple agents")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "multiple AI CLI tools detected") {
		t.Error("error should mention multiple tools detected")
	}
	if !strings.Contains(errMsg, "claude") {
		t.Error("error should list claude")
	}
	if !strings.Contains(errMsg, "gemini") {
		t.Error("error should list gemini")
	}
}

func TestPromptSelection_TTY_ValidNumber(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("2\n")

	as := NewAutoSetup(stdout, stderr, stdin, true) // isTTY = true

	detected := []detection.DetectedAgent{
		{
			Key:   "ai/claude",
			Entry: registry.IndexEntry{Bin: "claude", Description: "Claude CLI"},
		},
		{
			Key:   "ai/gemini",
			Entry: registry.IndexEntry{Bin: "gemini", Description: "Gemini CLI"},
		},
	}

	selected, err := as.promptSelection(detected)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if selected.Key != "ai/gemini" {
		t.Errorf("expected ai/gemini, got %s", selected.Key)
	}
}

func TestPromptSelection_TTY_ValidName(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("claude\n")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	detected := []detection.DetectedAgent{
		{
			Key:   "ai/claude",
			Entry: registry.IndexEntry{Bin: "claude", Description: "Claude CLI"},
		},
		{
			Key:   "ai/gemini",
			Entry: registry.IndexEntry{Bin: "gemini", Description: "Gemini CLI"},
		},
	}

	selected, err := as.promptSelection(detected)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if selected.Entry.Bin != "claude" {
		t.Errorf("expected claude, got %s", selected.Entry.Bin)
	}
}

func TestPromptSelection_TTY_InvalidNumber(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("5\n")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	detected := []detection.DetectedAgent{
		{
			Key:   "ai/claude",
			Entry: registry.IndexEntry{Bin: "claude"},
		},
		{
			Key:   "ai/gemini",
			Entry: registry.IndexEntry{Bin: "gemini"},
		},
	}

	_, err := as.promptSelection(detected)

	if err == nil {
		t.Fatal("expected error for invalid number")
	}

	if !strings.Contains(err.Error(), "invalid selection") {
		t.Errorf("expected 'invalid selection' error, got: %v", err)
	}
}

func TestPromptSelection_TTY_InvalidName(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("nonexistent\n")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	detected := []detection.DetectedAgent{
		{
			Key:   "ai/claude",
			Entry: registry.IndexEntry{Bin: "claude"},
		},
	}

	_, err := as.promptSelection(detected)

	if err == nil {
		t.Fatal("expected error for invalid name")
	}

	if !strings.Contains(err.Error(), "invalid selection") {
		t.Errorf("expected 'invalid selection' error, got: %v", err)
	}
}

func TestWriteConfig(t *testing.T) {
	// Create a temporary home directory
	tmpDir := t.TempDir()

	// Override HOME and XDG_CONFIG_HOME to use temp directory
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false)

	agent := Agent{
		Name:         "test-agent",
		Bin:          "test-bin",
		Command:      "{{.bin}} --model {{.model}}",
		DefaultModel: "default",
		Description:  "Test agent for unit tests",
		Models: map[string]string{
			"fast": "fast-model-id",
			"slow": "slow-model-id",
		},
	}

	configPath, err := as.writeConfig(agent)
	if err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}

	// Verify config path is returned
	if configPath == "" {
		t.Error("expected non-empty config path")
	}

	// Verify agents.cue was created
	agentsPath := filepath.Join(filepath.Dir(configPath), "agents.cue")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("reading agents.cue: %v", err)
	}

	agentsStr := string(agentsContent)
	if !strings.Contains(agentsStr, `"test-agent"`) {
		t.Error("agents.cue should contain agent name")
	}
	if !strings.Contains(agentsStr, `bin:`) {
		t.Error("agents.cue should contain bin field")
	}
	if !strings.Contains(agentsStr, `command:`) {
		t.Error("agents.cue should contain command field")
	}
	if !strings.Contains(agentsStr, `default_model:`) {
		t.Error("agents.cue should contain default_model field")
	}
	if !strings.Contains(agentsStr, `models:`) {
		t.Error("agents.cue should contain models field")
	}
	if !strings.Contains(agentsStr, `"fast"`) {
		t.Error("agents.cue should contain fast model")
	}

	// Verify settings.cue was created
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading settings.cue: %v", err)
	}

	configStr := string(configContent)
	if !strings.Contains(configStr, `default_agent: "test-agent"`) {
		t.Error("settings.cue should set default_agent")
	}
	if !strings.Contains(configStr, `settings:`) {
		t.Error("settings.cue should contain settings block")
	}
}

func TestWriteConfig_MinimalAgent(t *testing.T) {
	tmpDir := t.TempDir()

	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false)

	// Minimal agent with only required fields
	agent := Agent{
		Name:    "minimal",
		Bin:     "minimal-bin",
		Command: "{{.bin}}",
	}

	configPath, err := as.writeConfig(agent)
	if err != nil {
		t.Fatalf("writeConfig() error = %v", err)
	}

	// Verify agents.cue was created
	agentsPath := filepath.Join(filepath.Dir(configPath), "agents.cue")
	agentsContent, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("reading agents.cue: %v", err)
	}

	agentsStr := string(agentsContent)

	// Should have required fields
	if !strings.Contains(agentsStr, `bin:`) {
		t.Error("agents.cue should contain bin field")
	}
	if !strings.Contains(agentsStr, `command:`) {
		t.Error("agents.cue should contain command field")
	}

	// Should NOT have optional fields when empty
	if strings.Contains(agentsStr, `default_model:`) {
		t.Error("agents.cue should not have default_model when empty")
	}
	if strings.Contains(agentsStr, `description:`) {
		t.Error("agents.cue should not have description when empty")
	}
	if strings.Contains(agentsStr, `models:`) {
		t.Error("agents.cue should not have models when empty")
	}
}

func TestExtractAgentFromValue_NestedAgentsMap(t *testing.T) {
	// Test extraction from nested agents map (user config style)
	cueSrc := `
agents: {
	claude: {
		bin: "claude"
		command: "{{.bin}} chat"
		default_model: "sonnet"
	}
}
`
	ctx := cuecontext.New()
	v := ctx.CompileString(cueSrc)
	if err := v.Err(); err != nil {
		t.Fatalf("failed to compile test CUE: %v", err)
	}

	agent, err := extractAgentFromValue(v, "claude")
	if err != nil {
		t.Fatalf("extractAgentFromValue failed: %v", err)
	}

	if agent.Bin != "claude" {
		t.Errorf("wrong bin: %s", agent.Bin)
	}
	if agent.Command != "{{.bin}} chat" {
		t.Errorf("wrong command: %s", agent.Command)
	}
}

func TestExtractAgentFromValue_SingularAgentField(t *testing.T) {
	// Test extraction from singular agent field (registry module style)
	cueSrc := `
agent: {
	bin: "gemini"
	command: "{{.bin}} --model {{.model}}"
	default_model: "pro"
}
`
	ctx := cuecontext.New()
	v := ctx.CompileString(cueSrc)
	if err := v.Err(); err != nil {
		t.Fatalf("failed to compile test CUE: %v", err)
	}

	agent, err := extractAgentFromValue(v, "gemini")
	if err != nil {
		t.Fatalf("extractAgentFromValue failed: %v", err)
	}

	if agent.Bin != "gemini" {
		t.Errorf("wrong bin: %s", agent.Bin)
	}
	if agent.DefaultModel != "pro" {
		t.Errorf("wrong default_model: %s", agent.DefaultModel)
	}
}

func TestExtractAgentFromValue_NestedModelID(t *testing.T) {
	// Test extraction of models with nested id field (object format)
	cueSrc := `
bin: "test"
command: "{{.bin}}"
models: {
	fast: {
		id: "fast-model-id"
		description: "Fast model"
	}
	slow: {
		id: "slow-model-id"
	}
}
`
	ctx := cuecontext.New()
	v := ctx.CompileString(cueSrc)
	if err := v.Err(); err != nil {
		t.Fatalf("failed to compile test CUE: %v", err)
	}

	agent, err := extractAgentFromValue(v, "test")
	if err != nil {
		t.Fatalf("extractAgentFromValue failed: %v", err)
	}

	if len(agent.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(agent.Models))
	}
	if agent.Models["fast"] != "fast-model-id" {
		t.Errorf("wrong fast model: %s", agent.Models["fast"])
	}
	if agent.Models["slow"] != "slow-model-id" {
		t.Errorf("wrong slow model: %s", agent.Models["slow"])
	}
}
