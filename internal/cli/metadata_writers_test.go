package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/p3bot/start/internal/config"
	internalcue "github.com/p3bot/start/internal/cue"
)

// noColorForTest disables fatih/color so writer output is byte-exact;
// tui.ColorDim.Sprint emits ANSI escapes when colour is enabled.
func noColorForTest(t *testing.T) {
	t.Helper()
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })
}

func TestWriteAgentMetadata_AllFields(t *testing.T) {
	noColorForTest(t)
	agent := AgentConfig{
		Name:         "claude",
		Bin:          "claude",
		Command:      "claude {{.prompt}}",
		DefaultModel: "sonnet",
		Description:  "Anthropic Claude",
		Tags:         []string{"anthropic", "ai"},
		Models: map[string]string{
			"sonnet": "claude-sonnet-4",
			"opus":   "claude-opus-4",
		},
	}

	var buf bytes.Buffer
	writeAgentMetadata(&buf, agent)

	want := `
Description: Anthropic Claude
Bin: claude
Default Model: sonnet
Tags: anthropic, ai

Models:
  opus -> claude-opus-4
  sonnet -> claude-sonnet-4
`
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%s--- got ---\n%s", want, buf.String())
	}
}

func TestWriteAgentMetadata_NoFields_EmitsNothing(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeAgentMetadata(&buf, AgentConfig{})
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty AgentConfig, got %q", buf.String())
	}
}

func TestWriteAgentMetadata_ModelsSortedByAlias(t *testing.T) {
	noColorForTest(t)
	agent := AgentConfig{
		Models: map[string]string{
			"sonnet": "claude-sonnet",
			"haiku":  "claude-haiku",
			"opus":   "claude-opus",
		},
	}

	var buf bytes.Buffer
	writeAgentMetadata(&buf, agent)
	out := buf.String()

	haikuIdx := strings.Index(out, "haiku")
	opusIdx := strings.Index(out, "opus")
	sonnetIdx := strings.Index(out, "sonnet")
	if haikuIdx == -1 || opusIdx == -1 || sonnetIdx == -1 {
		t.Fatalf("missing alias in output\n%s", out)
	}
	if haikuIdx >= opusIdx || opusIdx >= sonnetIdx {
		t.Errorf("aliases not sorted: haiku=%d opus=%d sonnet=%d\n%s",
			haikuIdx, opusIdx, sonnetIdx, out)
	}
}

func TestWriteAgentMetadata_BlankLineBeforeModels(t *testing.T) {
	noColorForTest(t)
	agent := AgentConfig{
		Description: "with tags and models",
		Tags:        []string{"a"},
		Models:      map[string]string{"sonnet": "claude-sonnet"},
	}

	var buf bytes.Buffer
	writeAgentMetadata(&buf, agent)
	out := buf.String()

	idx := strings.Index(out, "Models:")
	if idx == -1 {
		t.Fatalf("missing Models: line\n%s", out)
	}
	if idx < 2 || out[idx-2:idx] != "\n\n" {
		t.Errorf("expected blank line directly before Models:, got %q before it\n%s",
			out[max(0, idx-4):idx], out)
	}
}

// TestWriteAgentMetadata_LeadingBlankWhenDescriptionEmpty verifies the leading
// `\n` fires for any populated header field, not just Description.
func TestWriteAgentMetadata_LeadingBlankWhenDescriptionEmpty(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeAgentMetadata(&buf, AgentConfig{Bin: "claude"})

	want := "\nBin: claude\n"
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%q\n--- got ---\n%q", want, buf.String())
	}
}

// TestWriteAgentMetadata_InnerSeparatorWithBinAndModels proves hasHeader counts
// non-Description fields: Bin + Models must fire the inner blank line.
func TestWriteAgentMetadata_InnerSeparatorWithBinAndModels(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeAgentMetadata(&buf, AgentConfig{
		Bin:    "claude",
		Models: map[string]string{"sonnet": "claude-sonnet"},
	})

	want := "\nBin: claude\n\nModels:\n  sonnet -> claude-sonnet\n"
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%q\n--- got ---\n%q", want, buf.String())
	}
}

// TestPrintMetadataBlock_OnlyModelsAgent_NoDoubleBlankLine verifies a models-only
// agent surfaces exactly one blank line before `Models:` end-to-end: the leading
// `\n` separator with the inner one suppressed (no header fields emitted).
func TestPrintMetadataBlock_OnlyModelsAgent_NoDoubleBlankLine(t *testing.T) {
	noColorForTest(t)

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	globalDir := filepath.Join(dir, ".config", "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cueSrc := `agents: {
	"onlymodels": {
		command: "onlymodels"
		models: {
			sonnet: "claude-sonnet"
		}
	}
}
`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(cueSrc), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	chdir(t, dir)

	result, err := prepareDescribe("onlymodels", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printMetadataBlock(&buf, result)
	out := buf.String()

	idx := strings.Index(out, "Models:")
	if idx == -1 {
		t.Fatalf("missing Models: line\n%s", out)
	}
	if idx == 0 || out[idx-1] != '\n' {
		t.Errorf("expected `\\n` immediately before `Models:`, got:\n%q", out)
	}
	if idx >= 2 && out[idx-2] == '\n' {
		t.Errorf("expected exactly one `\\n` before `Models:`, got two:\n%q", out)
	}
}

// TestPrintMetadataBlock_StripsDescribeOwnedFields verifies printMetadataBlock
// zeroes File/Command for role/context/task so describe.go's printVerboseDump
// emits them once via ExtractUTDFields without double-rendering.
func TestPrintMetadataBlock_StripsDescribeOwnedFields(t *testing.T) {
	noColorForTest(t)

	cases := []struct {
		itemType string
		cueKey   string
		cueSrc   string
	}{
		{
			itemType: "Role",
			cueKey:   internalcue.KeyRoles,
			cueSrc: `roles: {
	"strip-me": {
		description: "stub"
		file:        "/should/not/render/here.md"
		command:     "should-not-render-here"
	}
}
`,
		},
		{
			itemType: "Context",
			cueKey:   internalcue.KeyContexts,
			cueSrc: `contexts: {
	"strip-me": {
		description: "stub"
		file:        "/should/not/render/here.md"
		command:     "should-not-render-here"
	}
}
`,
		},
		{
			itemType: "Task",
			cueKey:   internalcue.KeyTasks,
			cueSrc: `tasks: {
	"strip-me": {
		description: "stub"
		file:        "/should/not/render/here.md"
		command:     "should-not-render-here"
	}
}
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.itemType, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOME", dir)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
			globalDir := filepath.Join(dir, ".config", "start")
			if err := os.MkdirAll(globalDir, 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			filename := strings.ToLower(tc.itemType) + "s.cue"
			if err := os.WriteFile(filepath.Join(globalDir, filename), []byte(tc.cueSrc), 0644); err != nil {
				t.Fatalf("write: %v", err)
			}
			chdir(t, dir)

			result, err := prepareDescribe("strip-me", config.ScopeMerged, tc.cueKey, tc.itemType)
			if err != nil {
				t.Fatalf("prepareDescribe: %v", err)
			}

			var buf bytes.Buffer
			printMetadataBlock(&buf, result)
			out := buf.String()

			if strings.Contains(out, "File:") {
				t.Errorf("expected File: to be stripped from printMetadataBlock output, got:\n%s", out)
			}
			if strings.Contains(out, "Command:") {
				t.Errorf("expected Command: to be stripped from printMetadataBlock output, got:\n%s", out)
			}
			if !strings.Contains(out, "Description:") {
				t.Errorf("expected Description: to render (sanity check); writer did not run as expected:\n%s", out)
			}
		})
	}
}

func TestWriteRoleMetadata_AllFields(t *testing.T) {
	noColorForTest(t)
	role := RoleConfig{
		Description: "Reviews code carefully.",
		File:        "@module/prompts/reviewer.md",
		Command:     "echo reviewer",
		Prompt:      "You are an expert code reviewer.",
		Optional:    true,
		Tags:        []string{"review", "quality"},
	}

	var buf bytes.Buffer
	writeRoleMetadata(&buf, role)

	want := `
Description: Reviews code carefully.
File: @module/prompts/reviewer.md
Command: echo reviewer
Prompt: You are an expert code reviewer.
Optional: true
Tags: review, quality
`
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%s--- got ---\n%s", want, buf.String())
	}
}

func TestWriteRoleMetadata_OptionalSkippedWhenFalse(t *testing.T) {
	noColorForTest(t)
	role := RoleConfig{
		Description: "no optional",
		Optional:    false,
	}

	var buf bytes.Buffer
	writeRoleMetadata(&buf, role)

	if strings.Contains(buf.String(), "Optional:") {
		t.Errorf("expected no Optional line when false, got %q", buf.String())
	}
}

func TestWriteRoleMetadata_FileCommandSkippedWhenEmpty(t *testing.T) {
	noColorForTest(t)
	role := RoleConfig{
		Description: "Has neither File nor Command",
		Prompt:      "hi",
		Tags:        []string{"t"},
	}

	var buf bytes.Buffer
	writeRoleMetadata(&buf, role)
	out := buf.String()

	if strings.Contains(out, "File:") {
		t.Errorf("expected no File: line\n%s", out)
	}
	if strings.Contains(out, "Command:") {
		t.Errorf("expected no Command: line\n%s", out)
	}
}

func TestWriteRoleMetadata_NoFields_EmitsNothing(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeRoleMetadata(&buf, RoleConfig{})
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

// TestWriteRoleMetadata_LeadingBlankWhenDescriptionEmpty verifies the leading
// `\n` fires for any populated field, not just Description.
func TestWriteRoleMetadata_LeadingBlankWhenDescriptionEmpty(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeRoleMetadata(&buf, RoleConfig{Tags: []string{"only-tags"}})

	want := "\nTags: only-tags\n"
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%q\n--- got ---\n%q", want, buf.String())
	}
}

func TestWriteContextMetadata_AllFields(t *testing.T) {
	noColorForTest(t)
	ctx := ContextConfig{
		Description: "Environment details.",
		File:        "@module/env.md",
		Command:     "uname -a",
		Prompt:      "Environment loaded.",
		Required:    true,
		Default:     true,
		Tags:        []string{"system", "env"},
	}

	var buf bytes.Buffer
	writeContextMetadata(&buf, ctx)

	want := `
Description: Environment details.
File: @module/env.md
Command: uname -a
Prompt: Environment loaded.
Required: true
Default: true
Tags: system, env
`
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%s--- got ---\n%s", want, buf.String())
	}
}

// TestWriteContextMetadata_RequiredDefaultAlwaysEmitted verifies Required and
// Default print unconditionally, even when false and other fields are empty.
func TestWriteContextMetadata_RequiredDefaultAlwaysEmitted(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeContextMetadata(&buf, ContextConfig{})

	want := `
Required: false
Default: false
`
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%s--- got ---\n%s", want, buf.String())
	}
}

func TestWriteContextMetadata_FileCommandSkippedWhenEmpty(t *testing.T) {
	noColorForTest(t)
	ctx := ContextConfig{
		Description: "no file/command",
		Required:    true,
	}

	var buf bytes.Buffer
	writeContextMetadata(&buf, ctx)
	out := buf.String()

	if strings.Contains(out, "File:") {
		t.Errorf("expected no File: line\n%s", out)
	}
	if strings.Contains(out, "Command:") {
		t.Errorf("expected no Command: line\n%s", out)
	}
}

func TestWriteTaskMetadata_AllFields(t *testing.T) {
	noColorForTest(t)
	task := TaskConfig{
		Description: "Review staged changes.",
		File:        "@module/tasks/review.md",
		Command:     "git diff --staged",
		Prompt:      "Review the staged changes.",
		Role:        "code-reviewer",
		Tags:        []string{"review", "git"},
	}

	var buf bytes.Buffer
	writeTaskMetadata(&buf, task)

	want := `
Description: Review staged changes.
File: @module/tasks/review.md
Command: git diff --staged
Prompt: Review the staged changes.
Role: code-reviewer
Tags: review, git
`
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%s--- got ---\n%s", want, buf.String())
	}
}

func TestWriteTaskMetadata_NoFields_EmitsNothing(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeTaskMetadata(&buf, TaskConfig{})
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

// TestWriteTaskMetadata_LeadingBlankWhenDescriptionEmpty verifies the leading
// `\n` fires for any populated field, not just Description.
func TestWriteTaskMetadata_LeadingBlankWhenDescriptionEmpty(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeTaskMetadata(&buf, TaskConfig{Role: "code-reviewer"})

	want := "\nRole: code-reviewer\n"
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%q\n--- got ---\n%q", want, buf.String())
	}
}

func TestWriteTaskMetadata_FileCommandSkippedWhenEmpty(t *testing.T) {
	noColorForTest(t)
	task := TaskConfig{
		Description: "no file/command",
		Role:        "r",
	}

	var buf bytes.Buffer
	writeTaskMetadata(&buf, task)
	out := buf.String()

	if strings.Contains(out, "File:") {
		t.Errorf("expected no File: line\n%s", out)
	}
	if strings.Contains(out, "Command:") {
		t.Errorf("expected no Command: line\n%s", out)
	}
}

func TestPromptTruncationLimit_AppliesToRoleContextTask(t *testing.T) {
	noColorForTest(t)
	longPrompt := strings.Repeat("x", 200)

	rolesBuf := &bytes.Buffer{}
	writeRoleMetadata(rolesBuf, RoleConfig{Prompt: longPrompt})
	ctxBuf := &bytes.Buffer{}
	writeContextMetadata(ctxBuf, ContextConfig{Prompt: longPrompt})
	taskBuf := &bytes.Buffer{}
	writeTaskMetadata(taskBuf, TaskConfig{Prompt: longPrompt})

	for name, b := range map[string]*bytes.Buffer{
		"role":    rolesBuf,
		"context": ctxBuf,
		"task":    taskBuf,
	} {
		if !strings.Contains(b.String(), "...") {
			t.Errorf("%s: expected truncated prompt to contain '...', got %q", name, b.String())
		}
		if strings.Contains(b.String(), longPrompt) {
			t.Errorf("%s: prompt should have been truncated", name)
		}
	}
}
