package orchestration

import (
	"bufio"
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

	// NewAutoSetup is a value-constructor (returns &AutoSetup{...}); a nil
	// return is impossible, so we only check field wiring.
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

// TestGenerateConfig_SlashKeyLabelAlignment verifies that when auto-setup uses
// the registry key as the agent name, the agents.cue label and the settings.cue
// default_agent value are byte-for-byte identical. This is the alignment
// requirement that lets auto-setup and 'start assets add' coexist without drift.
func TestGenerateConfig_SlashKeyLabelAlignment(t *testing.T) {
	const key = "claude/interactive"
	agent := Agent{
		Name:    key,
		Bin:     "claude",
		Command: "{{.bin}}",
	}

	agentCUE := generateAgentCUE(agent)
	settingsCUE := generateSettingsCUE(agent.Name)

	wantLabel := `"claude/interactive": {`
	if !strings.Contains(agentCUE, wantLabel) {
		t.Errorf("agents.cue should use the registry key as label (%q), got:\n%s", wantLabel, agentCUE)
	}

	wantDefault := `default_agent: "claude/interactive"`
	if !strings.Contains(settingsCUE, wantDefault) {
		t.Errorf("settings.cue should set default_agent to the registry key (%q), got:\n%s", wantDefault, settingsCUE)
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

func TestPickVariant(t *testing.T) {
	mk := func(keys ...string) []detection.DetectedAgent {
		out := make([]detection.DetectedAgent, len(keys))
		for i, k := range keys {
			out[i] = detection.DetectedAgent{Key: k, Entry: registry.IndexEntry{Bin: "claude"}}
		}
		return out
	}

	tests := []struct {
		name     string
		variants []detection.DetectedAgent
		wantKey  string
	}{
		{
			name:     "interactive wins over lex-first",
			variants: mk("claude/zzz", "claude/interactive", "claude/aaa"),
			wantKey:  "claude/interactive",
		},
		{
			name:     "bare name wins when no interactive variant",
			variants: mk("claude/zzz", "claude", "claude/aaa"),
			wantKey:  "claude",
		},
		{
			name:     "interactive beats bare name",
			variants: mk("claude/zzz", "claude", "claude/interactive"),
			wantKey:  "claude/interactive",
		},
		{
			name:     "lex-first when neither interactive nor bare",
			variants: mk("claude/zzz", "claude/aaa", "claude/middle"),
			wantKey:  "claude/aaa",
		},
		{
			name:     "single variant",
			variants: mk("claude/edit"),
			wantKey:  "claude/edit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickVariant(tt.variants)
			if got.Key != tt.wantKey {
				t.Errorf("pickVariant(): got %q, want %q", got.Key, tt.wantKey)
			}
		})
	}
}

func TestSelectAgent_NonTTYMultiBinPicksFirstWithFeedback(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false)

	detected := []detection.DetectedAgent{
		{Key: "gemini/interactive", Entry: registry.IndexEntry{Bin: "gemini"}},
		{Key: "claude/edit", Entry: registry.IndexEntry{Bin: "claude"}},
		{Key: "claude/interactive", Entry: registry.IndexEntry{Bin: "claude"}},
	}

	selected, err := as.selectAgent(detected)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if selected.Key != "claude/interactive" {
		t.Errorf("expected lex-first bin's heuristic winner 'claude/interactive', got %q", selected.Key)
	}

	out := stdout.String()
	if !strings.Contains(out, "claude") || !strings.Contains(out, "gemini") {
		t.Errorf("feedback should name all detected bins:\n%s", out)
	}
	if !strings.Contains(out, "using claude/interactive") {
		t.Errorf("feedback should name the chosen variant key (not just the bin):\n%s", out)
	}
	if !strings.Contains(out, "default_agent") {
		t.Errorf("feedback should mention default_agent override:\n%s", out)
	}
}

func TestSelectAgent_NonTTYSingleBinMultipleVariants(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false)

	detected := []detection.DetectedAgent{
		{Key: "claude/edit", Entry: registry.IndexEntry{Bin: "claude"}},
		{Key: "claude/interactive", Entry: registry.IndexEntry{Bin: "claude"}},
		{Key: "claude/unattended", Entry: registry.IndexEntry{Bin: "claude"}},
	}

	selected, err := as.selectAgent(detected)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if selected.Key != "claude/interactive" {
		t.Errorf("expected heuristic winner 'claude/interactive', got %q", selected.Key)
	}
	out := stdout.String()
	if !strings.Contains(out, "claude/interactive") {
		t.Errorf("feedback should name the chosen variant:\n%s", out)
	}
	if !strings.Contains(out, "default_agent") {
		t.Errorf("feedback should mention default_agent override:\n%s", out)
	}
}

func TestSelectAgent_SingleAgentPrintsSlashKey(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := NewAutoSetup(stdout, stderr, stdin, false)

	detected := []detection.DetectedAgent{
		{Key: "aichat/interactive", Entry: registry.IndexEntry{Bin: "aichat"}},
	}

	selected, err := as.selectAgent(detected)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Key != "aichat/interactive" {
		t.Errorf("expected single detected agent to be selected, got %q", selected.Key)
	}
	if !strings.Contains(stdout.String(), "Detected: aichat/interactive") {
		t.Errorf("expected 'Detected: aichat/interactive' in output:\n%s", stdout.String())
	}
}

func TestSelectAgent_TTYMultiBinCascadesToVariantPrompt(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	// Two prompts: tool prompt picks "claude" by name, then variant prompt
	// picks #2 ("claude/interactive" after lex sort of [edit, interactive]).
	stdin := strings.NewReader("claude\n2\n")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	detected := []detection.DetectedAgent{
		{Key: "claude/edit", Entry: registry.IndexEntry{Bin: "claude", Description: "auto-edit"}},
		{Key: "claude/interactive", Entry: registry.IndexEntry{Bin: "claude", Description: "default"}},
		{Key: "gemini/interactive", Entry: registry.IndexEntry{Bin: "gemini", Description: "Google Gemini"}},
	}

	selected, err := as.selectAgent(detected)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Key != "claude/interactive" {
		// claude variants sorted lex: edit, interactive. Input "2" → interactive.
		t.Errorf("expected cascade to land on 'claude/interactive', got %q", selected.Key)
	}

	out := stdout.String()
	if !strings.Contains(out, "Multiple AI CLI tools detected") {
		t.Errorf("expected tool prompt header:\n%s", out)
	}
	if !strings.Contains(out, "Multiple variants of claude detected") {
		t.Errorf("expected cascade variant prompt header:\n%s", out)
	}
}

func TestSelectAgent_TTYMultiBinSingleVariantSkipsCascade(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("aichat\n")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	detected := []detection.DetectedAgent{
		{Key: "aichat/interactive", Entry: registry.IndexEntry{Bin: "aichat"}},
		{Key: "claude/edit", Entry: registry.IndexEntry{Bin: "claude"}},
		{Key: "claude/interactive", Entry: registry.IndexEntry{Bin: "claude"}},
	}

	selected, err := as.selectAgent(detected)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Key != "aichat/interactive" {
		t.Errorf("expected single-variant bin selection without cascade, got %q", selected.Key)
	}
	if strings.Contains(stdout.String(), "Multiple variants of") {
		t.Errorf("variant prompt should not appear for single-variant bin:\n%s", stdout.String())
	}
}

func TestSelectAgent_TTYVariantPrompt(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("2\n")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	detected := []detection.DetectedAgent{
		{Key: "claude/edit", Entry: registry.IndexEntry{Bin: "claude", Description: "auto-edit"}},
		{Key: "claude/interactive", Entry: registry.IndexEntry{Bin: "claude", Description: "default"}},
	}

	selected, err := as.selectAgent(detected)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selected.Key != "claude/interactive" {
		t.Errorf("expected variant prompt selection 'claude/interactive', got %q", selected.Key)
	}
	if !strings.Contains(stdout.String(), "Multiple variants of claude detected") {
		t.Errorf("expected variant prompt header:\n%s", stdout.String())
	}
}

func TestPromptSelection_TTY_ValidNumber(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("2\n")

	as := NewAutoSetup(stdout, stderr, stdin, true) // isTTY = true

	detected := []detection.DetectedAgent{
		{
			Key:   "claude/interactive",
			Entry: registry.IndexEntry{Bin: "claude", Description: "Claude CLI"},
		},
		{
			Key:   "gemini/interactive",
			Entry: registry.IndexEntry{Bin: "gemini", Description: "Gemini CLI"},
		},
	}

	selected, err := as.promptSelection(detected, bufio.NewReader(stdin))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if selected.Key != "gemini/interactive" {
		t.Errorf("expected gemini/interactive, got %s", selected.Key)
	}
}

func TestPromptSelection_TTY_ValidName(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("claude\n")

	as := NewAutoSetup(stdout, stderr, stdin, true)

	detected := []detection.DetectedAgent{
		{
			Key:   "claude/interactive",
			Entry: registry.IndexEntry{Bin: "claude", Description: "Claude CLI"},
		},
		{
			Key:   "gemini/interactive",
			Entry: registry.IndexEntry{Bin: "gemini", Description: "Gemini CLI"},
		},
	}

	selected, err := as.promptSelection(detected, bufio.NewReader(stdin))

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
			Key:   "claude/interactive",
			Entry: registry.IndexEntry{Bin: "claude"},
		},
		{
			Key:   "gemini/interactive",
			Entry: registry.IndexEntry{Bin: "gemini"},
		},
	}

	_, err := as.promptSelection(detected, bufio.NewReader(stdin))

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
			Key:   "claude/interactive",
			Entry: registry.IndexEntry{Bin: "claude"},
		},
	}

	_, err := as.promptSelection(detected, bufio.NewReader(stdin))

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

// TestLoadAgentFromModule_HyphenatedDir verifies that a registry key with a
// hyphenated leaf (e.g. "claude/bypass-permissions") loads successfully when
// the on-disk module uses the underscored package name CUE requires
// (bypass_permissions). The directory is hyphenated; the package identifier
// inside the .cue file substitutes underscore. Previously the loader was given
// Package: "bypass-permissions" derived from the key leaf, which never matched
// the actual package and caused the load to fail.
func TestLoadAgentFromModule_HyphenatedDir(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "bypass-permissions")
	if err := os.MkdirAll(filepath.Join(moduleDir, "cue.mod"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	moduleCUE := `module: "test.example/bypass-permissions@v0"
language: {
	version: "v0.16.0"
}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "cue.mod", "module.cue"), []byte(moduleCUE), 0o644); err != nil {
		t.Fatalf("write module.cue: %v", err)
	}

	agentCUE := `package bypass_permissions

agent: {
	bin:     "claude"
	command: "{{.bin}} --bypass {{.prompt}}"
}
`
	if err := os.WriteFile(filepath.Join(moduleDir, "agent.cue"), []byte(agentCUE), 0o644); err != nil {
		t.Fatalf("write agent.cue: %v", err)
	}

	agent, err := loadAgentFromModule(moduleDir, "claude/bypass-permissions", nil)
	if err != nil {
		t.Fatalf("loadAgentFromModule failed: %v", err)
	}
	if agent.Bin != "claude" {
		t.Errorf("wrong bin: %q", agent.Bin)
	}
	if agent.Command != "{{.bin}} --bypass {{.prompt}}" {
		t.Errorf("wrong command: %q", agent.Command)
	}
}
