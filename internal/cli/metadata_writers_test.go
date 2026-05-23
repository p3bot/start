package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
)

// noColorForTest forces fatih/color off for the duration of a test and
// restores the original setting on cleanup. Required for byte-exact
// assertions against the writer output because callers like
// `tui.ColorDim.Sprint` emit ANSI escapes when colour is enabled.
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

// TestWriteAgentMetadata_ModelsSortedByAlias verifies aliases render in
// ascending order regardless of map iteration order.
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

// TestWriteAgentMetadata_BlankLineBeforeModels verifies the writer
// unconditionally emits a blank line immediately before `Models:` whenever
// the map is non-empty. This is the visual separator that pairs Tags with
// Models for both render surfaces.
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
	// The two bytes immediately preceding `Models:` should be `\n\n` (i.e.
	// the writer's blank line plus the newline ending the previous line).
	if idx < 2 || out[idx-2:idx] != "\n\n" {
		t.Errorf("expected blank line directly before Models:, got %q before it\n%s",
			out[max(0, idx-4):idx], out)
	}
}

// TestWriteAgentMetadata_LeadingBlankWhenDescriptionEmpty verifies the
// writer emits its leading `\n` for any populated header field, not just
// Description. Locks in the layout for agents that have Bin (or another
// non-Description field) but no Description.
func TestWriteAgentMetadata_LeadingBlankWhenDescriptionEmpty(t *testing.T) {
	noColorForTest(t)
	var buf bytes.Buffer
	writeAgentMetadata(&buf, AgentConfig{Bin: "claude"})

	want := "\nBin: claude\n"
	if buf.String() != want {
		t.Errorf("output mismatch\n--- want ---\n%q\n--- got ---\n%q", want, buf.String())
	}
}

// TestWriteAgentMetadata_InnerSeparatorWithBinAndModels verifies the
// hasHeader/hasModels split: when Bin (a non-Description header field)
// and Models are both set, the inner blank line before `Models:` must
// fire — proving hasHeader counts non-Description fields.
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

// TestPrintMetadataBlock_OnlyModelsAgent_NoDoubleBlankLine verifies the
// agent writer's hasHeader/hasModels split: an agent with only the
// `models` map populated must surface exactly one blank line before
// `Models:` end-to-end. The leading `\n` is the writer's own separator;
// the inner `Models:` separator is suppressed because no header fields
// were emitted.
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
	// Exactly one `\n` should precede `Models:` (the writer's leading
	// blank line). Zero would mean the leading `\n` was dropped; two
	// would mean the writer's inner `Models:` separator fired even
	// though no header fields preceded it.
	if idx == 0 || out[idx-1] != '\n' {
		t.Errorf("expected `\\n` immediately before `Models:`, got:\n%q", out)
	}
	if idx >= 2 && out[idx-2] == '\n' {
		t.Errorf("expected exactly one `\\n` before `Models:`, got two:\n%q", out)
	}
}

// TestPrintMetadataBlock_StripsDescribeOwnedFields verifies that for
// role/context/task, printMetadataBlock zeroes File and Command on the
// decoded struct before invoking the writer, so describe.go's
// printVerboseDump can emit them once via ExtractUTDFields without
// double-rendering. Removing the File/Command zeroing in printMetadataBlock
// would cause this test to fail.
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
			// Sanity: the writer ran (Description rendered) so the test
			// is exercising the right code path.
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

// TestWriteRoleMetadata_FileCommandSkippedWhenEmpty exercises the new
// describe-side discard: the caller zeroes File/Command on the typed struct
// so the writer can skip both lines.
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

// TestWriteRoleMetadata_LeadingBlankWhenDescriptionEmpty verifies the
// writer emits its leading `\n` for any populated field, not just
// Description. Catches a regression where someone gates the leading `\n`
// on `role.Description != ""`.
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

// TestWriteContextMetadata_RequiredDefaultAlwaysEmitted verifies that for
// contexts the writer prints Required: <bool> and Default: <bool>
// unconditionally — even when the underlying value is false and other
// fields are empty.
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

// TestWriteContextMetadata_FileCommandSkippedWhenEmpty matches the role
// case: the describe-side caller zeroes File/Command, so the writer must
// skip them.
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

// TestWriteTaskMetadata_LeadingBlankWhenDescriptionEmpty verifies the
// writer emits its leading `\n` for any populated field, not just
// Description.
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
		// truncatePrompt(s, 100) means the visible body is at most 100 chars
		// including the trailing "..."; the rendered line wraps the body so
		// the longPrompt's full 200 x's cannot appear verbatim.
		if strings.Contains(b.String(), longPrompt) {
			t.Errorf("%s: prompt should have been truncated", name)
		}
	}
}
