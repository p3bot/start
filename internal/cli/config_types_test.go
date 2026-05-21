package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	"github.com/start-cli/start/internal/config"
)

func TestDecodeAgentValue_FullMetadata(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		bin:           "claude"
		command:       "claude --model {{.model}} \"{{.prompt}}\""
		default_model: "sonnet"
		description:   "Anthropic Claude"
		tags: ["anthropic", "ai"]
		models: {
			sonnet: "claude-sonnet-4"
			opus:   "claude-opus-4"
		}
		origin: "github.com/example/start-claude@v1"
	}`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeAgentValue(val)

	if got.Bin != "claude" {
		t.Errorf("Bin: got %q want %q", got.Bin, "claude")
	}
	if got.Command != `claude --model {{.model}} "{{.prompt}}"` {
		t.Errorf("Command: got %q", got.Command)
	}
	if got.DefaultModel != "sonnet" {
		t.Errorf("DefaultModel: got %q want %q", got.DefaultModel, "sonnet")
	}
	if got.Description != "Anthropic Claude" {
		t.Errorf("Description: got %q", got.Description)
	}
	if !reflect.DeepEqual(got.Tags, []string{"anthropic", "ai"}) {
		t.Errorf("Tags: got %v", got.Tags)
	}
	wantModels := map[string]string{
		"sonnet": "claude-sonnet-4",
		"opus":   "claude-opus-4",
	}
	if !reflect.DeepEqual(got.Models, wantModels) {
		t.Errorf("Models: got %v want %v", got.Models, wantModels)
	}
	if got.Origin != "github.com/example/start-claude@v1" {
		t.Errorf("Origin: got %q", got.Origin)
	}
	if got.Name != "" {
		t.Errorf("Name should be zero (caller assigns), got %q", got.Name)
	}
	if got.Source != "" {
		t.Errorf("Source should be zero (caller assigns), got %q", got.Source)
	}
}

// TestDecodeAgentValue_ObjectFormModels exercises the both-forms walk: an
// agent declared with `models: { sonnet: { id: "..." } }` must populate
// Models with the resolved ids. The schema does not permit this shape but
// two runtime sites accept it, and this decoder is the single place the
// display + edit + list + resolve sites pick it up.
func TestDecodeAgentValue_ObjectFormModels(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		bin: "objform"
		models: {
			sonnet: { id: "obj-sonnet-id" }
			opus:   { id: "obj-opus-id" }
		}
	}`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeAgentValue(val)

	wantModels := map[string]string{
		"sonnet": "obj-sonnet-id",
		"opus":   "obj-opus-id",
	}
	if !reflect.DeepEqual(got.Models, wantModels) {
		t.Errorf("Models: got %v want %v", got.Models, wantModels)
	}
}

// TestDecodeAgentValue_MixedFormModels confirms simple- and object-form
// entries can coexist inside a single `models:` map.
func TestDecodeAgentValue_MixedFormModels(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		bin: "mixed"
		models: {
			sonnet: "simple-sonnet-id"
			opus:   { id: "object-opus-id" }
		}
	}`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeAgentValue(val)

	wantModels := map[string]string{
		"sonnet": "simple-sonnet-id",
		"opus":   "object-opus-id",
	}
	if !reflect.DeepEqual(got.Models, wantModels) {
		t.Errorf("Models: got %v want %v", got.Models, wantModels)
	}
}

func TestDecodeAgentValue_Empty(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{}`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeAgentValue(val)

	if got.Bin != "" || got.Command != "" || got.DefaultModel != "" ||
		got.Description != "" || got.Origin != "" ||
		len(got.Tags) != 0 || len(got.Models) != 0 {
		t.Errorf("expected zero-value AgentConfig, got %+v", got)
	}
}

func TestDecodeRoleValue_FullMetadata(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		description: "Reviews code carefully."
		file:        "@module/prompts/reviewer.md"
		command:     "echo reviewer"
		prompt:      "You are an expert code reviewer."
		optional:    true
		tags: ["review", "quality"]
		origin: "github.com/example/start-reviewer@v1"
	}`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeRoleValue(val)

	if got.Description != "Reviews code carefully." {
		t.Errorf("Description: got %q", got.Description)
	}
	if got.File != "@module/prompts/reviewer.md" {
		t.Errorf("File: got %q", got.File)
	}
	if got.Command != "echo reviewer" {
		t.Errorf("Command: got %q", got.Command)
	}
	if got.Prompt != "You are an expert code reviewer." {
		t.Errorf("Prompt: got %q", got.Prompt)
	}
	if !got.Optional {
		t.Error("Optional: got false, want true")
	}
	if !reflect.DeepEqual(got.Tags, []string{"review", "quality"}) {
		t.Errorf("Tags: got %v", got.Tags)
	}
	if got.Origin != "github.com/example/start-reviewer@v1" {
		t.Errorf("Origin: got %q", got.Origin)
	}
	if got.Name != "" {
		t.Errorf("Name should be zero (caller assigns), got %q", got.Name)
	}
}

func TestDecodeRoleValue_OptionalDefaultsFalse(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{ prompt: "hi" }`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeRoleValue(val)

	if got.Optional {
		t.Error("Optional: expected false when field absent, got true")
	}
}

func TestDecodeContextValue_FullMetadata(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		description: "Environment details."
		file:        "@module/env.md"
		command:     "uname -a"
		prompt:      "Environment loaded."
		required:    true
		default:     true
		tags: ["system", "env"]
		origin: "github.com/example/start-env@v1"
	}`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeContextValue(val)

	if got.Description != "Environment details." {
		t.Errorf("Description: got %q", got.Description)
	}
	if got.File != "@module/env.md" {
		t.Errorf("File: got %q", got.File)
	}
	if got.Command != "uname -a" {
		t.Errorf("Command: got %q", got.Command)
	}
	if got.Prompt != "Environment loaded." {
		t.Errorf("Prompt: got %q", got.Prompt)
	}
	if !got.Required || !got.Default {
		t.Errorf("Required/Default: got %v/%v, want true/true", got.Required, got.Default)
	}
	if !reflect.DeepEqual(got.Tags, []string{"system", "env"}) {
		t.Errorf("Tags: got %v", got.Tags)
	}
	if got.Origin != "github.com/example/start-env@v1" {
		t.Errorf("Origin: got %q", got.Origin)
	}
}

func TestDecodeContextValue_BooleansDefaultFalse(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{ prompt: "hi" }`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeContextValue(val)

	if got.Required || got.Default {
		t.Errorf("Required/Default: got %v/%v, want false/false when fields absent",
			got.Required, got.Default)
	}
}

func TestDecodeTaskValue_FullMetadata(t *testing.T) {
	ctx := cuecontext.New()
	val := ctx.CompileString(`{
		description: "Review staged changes."
		file:        "@module/tasks/review.md"
		command:     "git diff --staged"
		prompt:      "Review the staged changes."
		role:        "code-reviewer"
		tags: ["review", "git"]
		origin: "github.com/example/start-review@v1"
	}`)
	if err := val.Err(); err != nil {
		t.Fatalf("CompileString: %v", err)
	}

	got := decodeTaskValue(val)

	if got.Description != "Review staged changes." {
		t.Errorf("Description: got %q", got.Description)
	}
	if got.File != "@module/tasks/review.md" {
		t.Errorf("File: got %q", got.File)
	}
	if got.Command != "git diff --staged" {
		t.Errorf("Command: got %q", got.Command)
	}
	if got.Prompt != "Review the staged changes." {
		t.Errorf("Prompt: got %q", got.Prompt)
	}
	if got.Role != "code-reviewer" {
		t.Errorf("Role: got %q", got.Role)
	}
	if !reflect.DeepEqual(got.Tags, []string{"review", "git"}) {
		t.Errorf("Tags: got %v", got.Tags)
	}
	if got.Origin != "github.com/example/start-review@v1" {
		t.Errorf("Origin: got %q", got.Origin)
	}
}

// TestConfigList_ObjectFormAgent_JSON_EmitsAliases verifies the
// `config list agent --json` surface emits every object-form alias declared
// in `models`. Pre-refactor `loadAgentsFromDir` dropped object-form entries;
// the step-3 loader refactor (decodeAgentValue) makes both aliases land in
// the JSON output via the chain loadAgentsForScope -> ConfigListItem.Models.
//
// Tests in this function use os.Chdir (via the chdir helper) and modify
// color.NoColor; they cannot run in parallel.
func TestConfigList_ObjectFormAgent_JSON_EmitsAliases(t *testing.T) {
	setupSnapshotFixture(t, "agents.cue", snapshotObjectFormAgentCue)

	items, err := collectConfigListItems(false, "agent")
	if err != nil {
		t.Fatalf("collectConfigListItems: %v", err)
	}

	var objform *ConfigListItem
	for i := range items {
		if items[i].Name == "objform" {
			objform = &items[i]
			break
		}
	}
	if objform == nil {
		t.Fatalf("objform not in config list items; got %v", items)
	}

	wantModels := map[string]string{
		"sonnet": "obj-sonnet-id",
		"opus":   "obj-opus-id",
	}
	if !reflect.DeepEqual(objform.Models, wantModels) {
		t.Errorf("Models: got %v want %v", objform.Models, wantModels)
	}
}

// TestPromptModels_ObjectFormAgent_ListsAllAliases verifies the
// `config edit` model-prompt surface lists every alias declared in an
// object-form agent. Pre-refactor `loadAgentsFromDir` dropped object-form
// entries, so `agent.Models` was empty and the prompt printed "(none)"; the
// step-3 loader refactor (decodeAgentValue) makes both aliases appear.
//
// Tests in this function use os.Chdir (via the chdir helper) and modify
// color.NoColor; they cannot run in parallel.
func TestPromptModels_ObjectFormAgent_ListsAllAliases(t *testing.T) {
	setupSnapshotFixture(t, "agents.cue", snapshotObjectFormAgentCue)

	agents, _, err := loadAgentsForScope(config.ScopeMerged)
	if err != nil {
		t.Fatalf("loadAgentsForScope: %v", err)
	}
	agent, ok := agents["objform"]
	if !ok {
		t.Fatalf("agent objform not loaded; got %v", agents)
	}

	var stdout bytes.Buffer
	stdin := strings.NewReader("k\n")
	result, err := promptModels(&stdout, stdin, agent.Models)
	if err != nil {
		t.Fatalf("promptModels: %v", err)
	}

	out := stdout.String()
	for _, alias := range []string{"sonnet", "opus"} {
		if !strings.Contains(out, alias) {
			t.Errorf("expected prompt output to contain alias %q\n--- output ---\n%s", alias, out)
		}
	}

	wantMap := map[string]string{
		"sonnet": "obj-sonnet-id",
		"opus":   "obj-opus-id",
	}
	if !reflect.DeepEqual(result, wantMap) {
		t.Errorf("promptModels result: got %v want %v", result, wantMap)
	}
}

// TestLoadAgentsForScope_ScopeMatrix exercises loadForScope[T]'s path-based
// source labelling across all three scopes. The fixture stages "alpha" and
// "beta" globally and "beta" (override) plus "gamma" locally. ScopeMerged
// must report alpha=global, beta=local (override), gamma=local with order
// [alpha, beta, gamma]; ScopeGlobal must yield only the global pair;
// ScopeLocal must yield only the local pair. Locking ScopeGlobal here
// prevents drift before --global is rolled out to any CLI surface.
//
// Tests in this function use os.Chdir (via the chdir helper) and modify
// $HOME / $XDG_CONFIG_HOME; they cannot run in parallel.
func TestLoadAgentsForScope_ScopeMatrix(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))

	globalDir := filepath.Join(dir, ".config", "start")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("creating global dir: %v", err)
	}
	globalCUE := `agents: {
	"alpha": { bin: "alpha-bin" }
	"beta":  { bin: "beta-global" }
}
`
	if err := os.WriteFile(filepath.Join(globalDir, "agents.cue"), []byte(globalCUE), 0644); err != nil {
		t.Fatalf("writing global agents.cue: %v", err)
	}

	localDir := filepath.Join(dir, "project", ".start")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("creating local dir: %v", err)
	}
	localCUE := `agents: {
	"beta":  { bin: "beta-local" }
	"gamma": { bin: "gamma-bin" }
}
`
	if err := os.WriteFile(filepath.Join(localDir, "agents.cue"), []byte(localCUE), 0644); err != nil {
		t.Fatalf("writing local agents.cue: %v", err)
	}

	chdir(t, filepath.Join(dir, "project"))

	tests := []struct {
		name        string
		scope       config.Scope
		wantOrder   []string
		wantSources map[string]string
		wantBins    map[string]string
	}{
		{
			name:        "merged",
			scope:       config.ScopeMerged,
			wantOrder:   []string{"alpha", "beta", "gamma"},
			wantSources: map[string]string{"alpha": "global", "beta": "local", "gamma": "local"},
			wantBins:    map[string]string{"alpha": "alpha-bin", "beta": "beta-local", "gamma": "gamma-bin"},
		},
		{
			name:        "global",
			scope:       config.ScopeGlobal,
			wantOrder:   []string{"alpha", "beta"},
			wantSources: map[string]string{"alpha": "global", "beta": "global"},
			wantBins:    map[string]string{"alpha": "alpha-bin", "beta": "beta-global"},
		},
		{
			name:        "local",
			scope:       config.ScopeLocal,
			wantOrder:   []string{"beta", "gamma"},
			wantSources: map[string]string{"beta": "local", "gamma": "local"},
			wantBins:    map[string]string{"beta": "beta-local", "gamma": "gamma-bin"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			agents, order, err := loadAgentsForScope(tc.scope)
			if err != nil {
				t.Fatalf("loadAgentsForScope(%s): %v", tc.scope, err)
			}
			if !reflect.DeepEqual(order, tc.wantOrder) {
				t.Errorf("order: got %v want %v", order, tc.wantOrder)
			}
			if len(agents) != len(tc.wantSources) {
				t.Errorf("agent count: got %d want %d", len(agents), len(tc.wantSources))
			}
			for name, wantSource := range tc.wantSources {
				agent, ok := agents[name]
				if !ok {
					t.Errorf("missing agent %q", name)
					continue
				}
				if agent.Source != wantSource {
					t.Errorf("agent %q Source: got %q want %q", name, agent.Source, wantSource)
				}
				if agent.Bin != tc.wantBins[name] {
					t.Errorf("agent %q Bin: got %q want %q", name, agent.Bin, tc.wantBins[name])
				}
			}
		})
	}
}
