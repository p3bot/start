package cli

// End-to-end snapshot tests for the describe and config info rendering
// surfaces. Three of the eight baselines (describe agent, config info agent,
// object-form agent on both surfaces) are updated by this refactor; the
// remaining five act as byte-identical regression guards.
//
// Tests in this file use os.Chdir (via the chdir helper) and modify
// color.NoColor; they cannot run in parallel.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatih/color"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
)

// snapshotSeparator is the 79-rune box-drawing line emitted by printSeparator.
var snapshotSeparator = strings.Repeat("─", 79)

// snapshotAgentCue is the canonical fixture for snapshot baselines. Field
// order is preserved by CUE and therefore by formatCUEDefinition's output.
const snapshotAgentCue = `agents: {
	"claude": {
		bin:           "claude"
		command:       "claude --model {{.model}} \"{{.prompt}}\""
		default_model: "sonnet"
		description:   "Anthropic Claude"
		tags: ["anthropic", "ai"]
		models: {
			sonnet: "claude-sonnet-4-20250514"
			opus:   "claude-opus-4-20250514"
		}
	}
}
`

const snapshotRoleCue = `roles: {
	"code-reviewer": {
		description: "Reviews code carefully."
		prompt:      "You are an expert code reviewer."
		tags: ["review", "quality"]
	}
}
`

const snapshotContextCue = `contexts: {
	"environment": {
		description: "Environment details for the session."
		prompt:      "Environment context loaded."
		required:    true
		default:     true
		tags: ["system", "environment"]
	}
}
`

const snapshotTaskCue = `tasks: {
	"myreview": {
		description: "Review staged changes."
		prompt:      "Review the staged changes."
		role:        "code-reviewer"
		tags: ["review", "git"]
	}
}
`

// snapshotDescriptionlessContextCue exercises a context with no
// `description:` and no `required:` / `default:` set. The shared writer
// always emits Required and Default unconditionally, and after the M2
// refactor it also owns its own leading blank line — so this fixture pins
// the "blank line between header and metadata block" layout for items
// without Description.
const snapshotDescriptionlessContextCue = `contexts: {
	"barebones": {
		prompt: "No description here."
	}
}
`

// snapshotObjectFormAgentCue exercises the object-form models shape that the
// schema does not permit but two runtime sites accept defensively. The
// refactor widens the loader and describe paths to accept it too; pre-
// refactor the Models block renders empty for both surfaces.
const snapshotObjectFormAgentCue = `agents: {
	"objform": {
		bin:           "objform"
		command:       "objform --model {{.model}} \"{{.prompt}}\""
		default_model: "sonnet"
		description:   "Object-form agent"
		tags: ["object", "form"]
		models: {
			sonnet: { id: "obj-sonnet-id" }
			opus:   { id: "obj-opus-id" }
		}
	}
}
`

// setupSnapshotFixture writes a single .cue file into the global config dir,
// configures HOME / XDG_CONFIG_HOME so config.ResolvePaths sees only that
// global dir, chdirs to a clean temp dir (no .start), and disables fatih
// colour output for the duration of the test. Returns the absolute path to
// the written .cue file so snapshot literals can substitute it in.
func setupSnapshotFixture(t *testing.T, filename, content string) (cuePath string) {
	t.Helper()
	prevNoColor := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prevNoColor })

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	globalDir := filepath.Join(dir, ".config", "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("creating global dir: %v", err)
	}

	cuePath = filepath.Join(globalDir, filename)
	if err := os.WriteFile(cuePath, []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", filename, err)
	}

	// chdir into the temp dir so local config resolution sees no .start
	// directory and ScopeMerged collapses to global-only.
	chdir(t, dir)
	return cuePath
}

func assertSnapshot(t *testing.T, want, got string) {
	t.Helper()
	if want == got {
		return
	}
	t.Errorf("snapshot mismatch\n--- want ---\n%s--- got ---\n%s--- end ---\n", want, got)
}

// TestSnapshot_DescribeAgent pins the pre-refactor `start describe <agent>`
// output. The literal is updated in step 5 of the refactor when the shared
// writer inserts a blank line before the Models: section.
func TestSnapshot_DescribeAgent(t *testing.T) {
	cuePath := setupSnapshotFixture(t, "agents.cue", snapshotAgentCue)

	result, err := prepareDescribe("claude", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)

	want := fmt.Sprintf(`Agent: claude
%[1]s
Config: %[2]s (claude)

Description: Anthropic Claude
Bin: claude
Default Model: sonnet
Tags: anthropic, ai

Models:
  opus -> claude-opus-4-20250514
  sonnet -> claude-sonnet-4-20250514

{
	bin:           "claude"
	command:       "claude --model {{.model}} \"{{.prompt}}\""
	default_model: "sonnet"
	description:   "Anthropic Claude"
	tags: ["anthropic", "ai"]
	models: {
		sonnet: "claude-sonnet-4-20250514"
		opus:   "claude-opus-4-20250514"
	}
}

Command: claude --model claude-sonnet-4-20250514 "{{.prompt}}"
%[1]s
`, snapshotSeparator, cuePath)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_DescribeRole pins `start describe <role>`. Byte-identical
// across the refactor.
func TestSnapshot_DescribeRole(t *testing.T) {
	cuePath := setupSnapshotFixture(t, "roles.cue", snapshotRoleCue)

	result, err := prepareDescribe("code-reviewer", config.ScopeMerged, internalcue.KeyRoles, "Role")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)

	want := fmt.Sprintf(`Role: code-reviewer
%[1]s
Config: %[2]s (code-reviewer)

Description: Reviews code carefully.
Prompt: You are an expert code reviewer.
Tags: review, quality

{
	description: "Reviews code carefully."
	prompt:      "You are an expert code reviewer."
	tags: ["review", "quality"]
}
%[1]s
`, snapshotSeparator, cuePath)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_DescribeContext pins `start describe <context>`. Byte-identical
// across the refactor.
func TestSnapshot_DescribeContext(t *testing.T) {
	cuePath := setupSnapshotFixture(t, "contexts.cue", snapshotContextCue)

	result, err := prepareDescribe("environment", config.ScopeMerged, internalcue.KeyContexts, "Context")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)

	want := fmt.Sprintf(`Context: environment
%[1]s
Config: %[2]s (environment)

Description: Environment details for the session.
Prompt: Environment context loaded.
Required: true
Default: true
Tags: system, environment

{
	description: "Environment details for the session."
	prompt:      "Environment context loaded."
	required:    true
	default:     true
	tags: ["system", "environment"]
}
%[1]s
`, snapshotSeparator, cuePath)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_DescribeTask pins `start describe <task>`. Byte-identical
// across the refactor.
func TestSnapshot_DescribeTask(t *testing.T) {
	cuePath := setupSnapshotFixture(t, "tasks.cue", snapshotTaskCue)

	result, err := prepareDescribe("myreview", config.ScopeMerged, internalcue.KeyTasks, "Task")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)

	want := fmt.Sprintf(`Task: myreview
%[1]s
Config: %[2]s (myreview)

Description: Review staged changes.
Prompt: Review the staged changes.
Role: code-reviewer
Tags: review, git

{
	description: "Review staged changes."
	prompt:      "Review the staged changes."
	role:        "code-reviewer"
	tags: ["review", "git"]
}
%[1]s
`, snapshotSeparator, cuePath)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_ConfigInfoAgent pins `start config info <agent>`. The literal
// is updated in step 6 of the refactor when Bin and Default Model move from
// the header section into the shared writer (per requirement 7).
func TestSnapshot_ConfigInfoAgent(t *testing.T) {
	setupSnapshotFixture(t, "agents.cue", snapshotAgentCue)

	var buf bytes.Buffer
	if err := printAgentInfo(&buf, false, "claude"); err != nil {
		t.Fatalf("printAgentInfo: %v", err)
	}

	want := fmt.Sprintf(`
agents:claude
%[1]s
Source: global
Command: claude --model {{.model}} "{{.prompt}}"

Description: Anthropic Claude
Bin: claude
Default Model: sonnet
Tags: anthropic, ai

Models:
  opus -> claude-opus-4-20250514
  sonnet -> claude-sonnet-4-20250514
%[1]s
`, snapshotSeparator)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_ConfigInfoRole pins `start config info <role>`. Byte-identical
// across the refactor.
func TestSnapshot_ConfigInfoRole(t *testing.T) {
	setupSnapshotFixture(t, "roles.cue", snapshotRoleCue)

	var buf bytes.Buffer
	if err := printRoleInfo(&buf, false, "code-reviewer"); err != nil {
		t.Fatalf("printRoleInfo: %v", err)
	}

	want := fmt.Sprintf(`
roles:code-reviewer
%[1]s
Source: global

Description: Reviews code carefully.
Prompt: You are an expert code reviewer.
Tags: review, quality
%[1]s
`, snapshotSeparator)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_ConfigInfoContext pins `start config info <context>`.
// Byte-identical across the refactor.
func TestSnapshot_ConfigInfoContext(t *testing.T) {
	setupSnapshotFixture(t, "contexts.cue", snapshotContextCue)

	var buf bytes.Buffer
	if err := printContextInfo(&buf, false, "environment"); err != nil {
		t.Fatalf("printContextInfo: %v", err)
	}

	want := fmt.Sprintf(`
contexts:environment
%[1]s
Source: global

Description: Environment details for the session.
Prompt: Environment context loaded.
Required: true
Default: true
Tags: system, environment
%[1]s
`, snapshotSeparator)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_DescribeAgentObjectForm pins `start describe <agent>` for an
// agent declared with object-form `models: { sonnet: { id: "..." } }`. The
// schema does not permit this shape, but `executor.go` and
// `partialFillAgentCommand` accept it defensively; the refactor aligns the
// describe display path through `decodeAgentValue` so it picks up both
// forms.
func TestSnapshot_DescribeAgentObjectForm(t *testing.T) {
	cuePath := setupSnapshotFixture(t, "agents.cue", snapshotObjectFormAgentCue)

	result, err := prepareDescribe("objform", config.ScopeMerged, internalcue.KeyAgents, "Agent")
	if err != nil {
		t.Fatalf("prepareDescribe: %v", err)
	}

	var buf bytes.Buffer
	printVerboseDump(&buf, result)

	want := fmt.Sprintf(`Agent: objform
%[1]s
Config: %[2]s (objform)

Description: Object-form agent
Bin: objform
Default Model: sonnet
Tags: object, form

Models:
  opus -> obj-opus-id
  sonnet -> obj-sonnet-id

{
	bin:           "objform"
	command:       "objform --model {{.model}} \"{{.prompt}}\""
	default_model: "sonnet"
	description:   "Object-form agent"
	tags: ["object", "form"]
	models: {
		sonnet: {
			id: "obj-sonnet-id"
		}
		opus: {
			id: "obj-opus-id"
		}
	}
}

Command: objform --model obj-sonnet-id "{{.prompt}}"
%[1]s
`, snapshotSeparator, cuePath)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_ConfigInfoAgentObjectForm pins `start config info <agent>` for
// the object-form fixture. The literal reflects the populated Models block
// `decodeAgentValue` produces (post step 3 loader refactor). The header /
// metadata line order will reorder in step 6 alongside the simple-form
// agent.
func TestSnapshot_ConfigInfoAgentObjectForm(t *testing.T) {
	setupSnapshotFixture(t, "agents.cue", snapshotObjectFormAgentCue)

	var buf bytes.Buffer
	if err := printAgentInfo(&buf, false, "objform"); err != nil {
		t.Fatalf("printAgentInfo: %v", err)
	}

	want := fmt.Sprintf(`
agents:objform
%[1]s
Source: global
Command: objform --model {{.model}} "{{.prompt}}"

Description: Object-form agent
Bin: objform
Default Model: sonnet
Tags: object, form

Models:
  opus -> obj-opus-id
  sonnet -> obj-sonnet-id
%[1]s
`, snapshotSeparator)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_ConfigInfoTask pins `start config info <task>`. Byte-identical
// across the refactor.
func TestSnapshot_ConfigInfoTask(t *testing.T) {
	setupSnapshotFixture(t, "tasks.cue", snapshotTaskCue)

	var buf bytes.Buffer
	if err := printTaskInfo(&buf, false, "myreview"); err != nil {
		t.Fatalf("printTaskInfo: %v", err)
	}

	want := fmt.Sprintf(`
tasks:myreview
%[1]s
Source: global

Description: Review staged changes.
Prompt: Review the staged changes.
Role: code-reviewer
Tags: review, git
%[1]s
`, snapshotSeparator)

	assertSnapshot(t, want, buf.String())
}

// TestSnapshot_ConfigInfoContextWithoutDescription pins the layout for a
// context that has no `description:` set. Pre-Phase-2 (when callers gated
// the blank line on `Description != ""`), the writer's first line stuck
// against the header — `Required: false` rendered immediately under
// `Source: global`. After M2, the writer owns its own leading blank line,
// so the metadata block always sits visually separated from the header
// regardless of which fields are populated. This is a deliberate behaviour
// change and this test locks it in.
func TestSnapshot_ConfigInfoContextWithoutDescription(t *testing.T) {
	setupSnapshotFixture(t, "contexts.cue", snapshotDescriptionlessContextCue)

	var buf bytes.Buffer
	if err := printContextInfo(&buf, false, "barebones"); err != nil {
		t.Fatalf("printContextInfo: %v", err)
	}

	want := fmt.Sprintf(`
contexts:barebones
%[1]s
Source: global

Prompt: No description here.
Required: false
Default: false
%[1]s
`, snapshotSeparator)

	assertSnapshot(t, want, buf.String())
}
