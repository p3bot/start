package cli

import (
	"io"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/registry"
)

// buildTestCfg creates a LoadResult from a CUE string for testing.
func buildTestCfg(t *testing.T, cueStr string) internalcue.LoadResult {
	t.Helper()
	cctx := cuecontext.New()
	v := cctx.CompileString(cueStr)
	if v.Err() != nil {
		t.Fatalf("compiling test CUE: %v", v.Err())
	}
	return internalcue.LoadResult{Value: v}
}

func TestFindExactInstalledName(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			claude: {
				bin: "claude"
				command: "{{.bin}}"
			}
			"gemini-non-interactive": {
				bin: "gemini"
				command: "{{.bin}}"
			}
		}
		roles: {
			"golang/assistant": {
				prompt: "You are a Go expert"
			}
		}
		tasks: {
			"review/git-diff": {
				prompt: "Review the git diff"
			}
			"review/code": {
				prompt: "Review code"
			}
			"standalone": {
				prompt: "A standalone task"
			}
		}
		contexts: {
			env: {
				required: true
				prompt: "environment"
			}
		}
	}`)

	tests := []struct {
		name      string
		cueKey    string
		moduleKey string
		wantName  string
		wantErr   bool
	}{
		{"exact agent match", internalcue.KeyAgents, "claude", "claude", false},
		{"exact agent with hyphen", internalcue.KeyAgents, "gemini-non-interactive", "gemini-non-interactive", false},
		{"partial agent no match", internalcue.KeyAgents, "clau", "", false},
		{"nonexistent agent", internalcue.KeyAgents, "nonexistent", "", false},
		{"exact role full name", internalcue.KeyRoles, "golang/assistant", "golang/assistant", false},
		{"role short name match", internalcue.KeyRoles, "assistant", "golang/assistant", false},
		{"partial role no match", internalcue.KeyRoles, "golang", "", false},
		{"exact context match", internalcue.KeyContexts, "env", "env", false},
		{"missing category", "missing", "anything", "", false},
		{"task short name match", internalcue.KeyTasks, "git-diff", "review/git-diff", false},
		{"task exact full name", internalcue.KeyTasks, "review/code", "review/code", false},
		{"task standalone exact", internalcue.KeyTasks, "standalone", "standalone", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := findExactInstalledName(cfg.Value, tt.cueKey, tt.moduleKey)
			if tt.wantErr {
				if err == nil {
					t.Errorf("findExactInstalledName(%q, %q) expected error, got nil", tt.cueKey, tt.moduleKey)
				}
				return
			}
			if err != nil {
				t.Fatalf("findExactInstalledName(%q, %q) unexpected error: %v", tt.cueKey, tt.moduleKey, err)
			}
			if got != tt.wantName {
				t.Errorf("findExactInstalledName(%q, %q) = %q, want %q", tt.cueKey, tt.moduleKey, got, tt.wantName)
			}
		})
	}
}

func TestFindExactInstalledName_Ambiguous(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		tasks: {
			"review/debug": {
				prompt: "Review debug"
			}
			"golang/debug": {
				prompt: "Debug Go code"
			}
		}
	}`)

	_, err := findExactInstalledName(cfg.Value, internalcue.KeyTasks, "debug")
	if err == nil {
		t.Error("expected ambiguity error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected ambiguity error, got: %v", err)
	}
}

func TestFindExactInRegistry(t *testing.T) {
	t.Parallel()

	entries := map[string]registry.IndexEntry{
		"golang/assistant": {
			Module:      "github.com/test/roles/golang/assistant@v0",
			Description: "Go programming expert",
		},
		"python/debug": {
			Module:      "github.com/test/roles/python/debug@v0",
			Description: "Python debugger",
		},
	}

	tests := []struct {
		name     string
		query    string
		wantName string
		wantNil  bool
	}{
		{"full name match", "golang/assistant", "golang/assistant", false},
		{"short name match", "assistant", "golang/assistant", false},
		{"no match", "nonexistent", "", true},
		{"partial no match", "gol", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := findExactInRegistry(entries, "roles", tt.query)
			if err != nil {
				t.Fatalf("findExactInRegistry(%q) unexpected error: %v", tt.query, err)
			}
			if tt.wantNil {
				if result != nil {
					t.Errorf("findExactInRegistry(%q) = %v, want nil", tt.query, result)
				}
				return
			}
			if result == nil {
				t.Fatalf("findExactInRegistry(%q) = nil, want %q", tt.query, tt.wantName)
				return
			}
			if result.Name != tt.wantName {
				t.Errorf("findExactInRegistry(%q).Name = %q, want %q", tt.query, result.Name, tt.wantName)
			}
		})
	}
}

func TestFindExactInRegistryAmbiguous(t *testing.T) {
	t.Parallel()

	entries := map[string]registry.IndexEntry{
		"golang/assistant": {
			Module:      "github.com/test/roles/golang/assistant@v0",
			Description: "Go programming expert",
		},
		"python/assistant": {
			Module:      "github.com/test/roles/python/assistant@v0",
			Description: "Python programming expert",
		},
	}

	// Short name "assistant" matches both entries
	result, err := findExactInRegistry(entries, "roles", "assistant")
	if err == nil {
		t.Fatal("findExactInRegistry(\"assistant\") expected ambiguity error, got nil")
	}
	if result != nil {
		t.Errorf("findExactInRegistry(\"assistant\") result = %v, want nil", result)
	}

	// Full name is unambiguous
	result, err = findExactInRegistry(entries, "roles", "golang/assistant")
	if err != nil {
		t.Fatalf("findExactInRegistry(\"golang/assistant\") unexpected error: %v", err)
	}
	if result == nil || result.Name != "golang/assistant" {
		t.Errorf("findExactInRegistry(\"golang/assistant\") = %v, want golang/assistant", result)
	}
}

func TestMergeModuleMatches(t *testing.T) {
	t.Parallel()

	installed := []ModuleMatch{
		{Name: "claude", Category: "agents", Source: ModuleSourceInstalled, Score: 3},
		{Name: "gemini", Category: "agents", Source: ModuleSourceInstalled, Score: 3},
	}
	reg := []ModuleMatch{
		{Name: "claude", Category: "agents", Source: ModuleSourceRegistry, Score: 5},  // dup
		{Name: "openai", Category: "agents", Source: ModuleSourceRegistry, Score: 3},  // new
		{Name: "copilot", Category: "agents", Source: ModuleSourceRegistry, Score: 1}, // new, low score
	}

	merged := mergeModuleMatches(installed, reg)

	// Should have 4 unique entries (claude deduplicated)
	if len(merged) != 4 {
		t.Fatalf("mergeModuleMatches returned %d results, want 4", len(merged))
	}

	// Claude should be from installed (installed wins)
	for _, m := range merged {
		if m.Name == "claude" && m.Source != ModuleSourceInstalled {
			t.Errorf("claude should be from installed, got %q", m.Source)
		}
	}

	// Should be sorted by score desc, then name
	for i := 1; i < len(merged); i++ {
		if merged[i-1].Score < merged[i].Score {
			t.Errorf("results not sorted by score: %d < %d at positions %d,%d",
				merged[i-1].Score, merged[i].Score, i-1, i)
		}
	}
}

func TestMergeModuleMatches_Empty(t *testing.T) {
	t.Parallel()

	merged := mergeModuleMatches(nil, nil)
	if len(merged) != 0 {
		t.Errorf("expected 0 results for empty inputs, got %d", len(merged))
	}
}

// newTestResolver creates a resolver for testing that skips registry access.
func newTestResolver(cfg internalcue.LoadResult) *resolver {
	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	r.skipRegistry = true
	return r
}

func TestResolveAgent_ExactMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			claude: {
				bin: "claude"
				command: "{{.bin}}"
			}
		}
	}`)

	r := newTestResolver(cfg)
	name, err := r.resolveAgent("claude")
	if err != nil {
		t.Fatalf("resolveAgent() error = %v", err)
	}
	if name != "claude" {
		t.Errorf("resolveAgent() = %q, want %q", name, "claude")
	}
}

func TestResolveAgent_SubstringMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			"gemini-non-interactive": {
				description: "Google Gemini agent"
				bin: "gemini"
				command: "{{.bin}}"
			}
		}
	}`)

	r := newTestResolver(cfg)
	name, err := r.resolveAgent("gemini")
	if err != nil {
		t.Fatalf("resolveAgent() error = %v", err)
	}
	if name != "gemini-non-interactive" {
		t.Errorf("resolveAgent() = %q, want %q", name, "gemini-non-interactive")
	}
}

func TestResolveAgent_NoMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			claude: {
				bin: "claude"
				command: "{{.bin}}"
			}
		}
	}`)

	r := newTestResolver(cfg)
	_, err := r.resolveAgent("nonexistent")
	if err == nil {
		t.Fatal("expected error for no match")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want containing 'not found'", err.Error())
	}
}

func TestResolveAgent_MultipleMatches_NonTTY(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			"claude-code": {
				description: "Claude for coding"
				bin: "claude"
				command: "{{.bin}}"
			}
			"claude-chat": {
				description: "Claude for chatting"
				bin: "claude"
				command: "{{.bin}}"
			}
		}
	}`)

	r := newTestResolver(cfg)
	_, err := r.resolveAgent("claude")
	if err == nil {
		t.Fatal("expected error for multiple matches in non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
	if !strings.Contains(err.Error(), "claude-code") {
		t.Errorf("error should list claude-code: %v", err)
	}
	if !strings.Contains(err.Error(), "claude-chat") {
		t.Errorf("error should list claude-chat: %v", err)
	}
}

func TestResolveAgent_Empty(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{agents: {}}`)
	r := newTestResolver(cfg)
	name, err := r.resolveAgent("")
	if err != nil {
		t.Fatalf("resolveAgent('') error = %v", err)
	}
	if name != "" {
		t.Errorf("resolveAgent('') = %q, want empty", name)
	}
}

func TestResolveRole_ExactMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		roles: {
			"golang/assistant": {
				prompt: "You are a Go expert"
			}
		}
	}`)

	r := newTestResolver(cfg)
	name, err := r.resolveRole("golang/assistant")
	if err != nil {
		t.Fatalf("resolveRole() error = %v", err)
	}
	if name != "golang/assistant" {
		t.Errorf("resolveRole() = %q, want %q", name, "golang/assistant")
	}
}

func TestResolveRole_FilePathBypass(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{roles: {}}`)
	r := newTestResolver(cfg)

	tests := []string{"./my-role.md", "/tmp/role.md", "~/roles/test.md"}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			name, err := r.resolveRole(path)
			if err != nil {
				t.Fatalf("resolveRole(%q) error = %v", path, err)
			}
			if name != path {
				t.Errorf("resolveRole(%q) = %q, want %q", path, name, path)
			}
		})
	}
}

func TestResolveRole_SubstringMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		roles: {
			"golang/assistant": {
				description: "Go programming expert"
				prompt: "You are a Go expert"
			}
		}
	}`)

	r := newTestResolver(cfg)
	name, err := r.resolveRole("golang")
	if err != nil {
		t.Fatalf("resolveRole() error = %v", err)
	}
	if name != "golang/assistant" {
		t.Errorf("resolveRole() = %q, want %q", name, "golang/assistant")
	}
}

func TestResolveModelName_ExactMatch(t *testing.T) {
	t.Parallel()

	agent := orchestration.Agent{
		Models: map[string]string{
			"sonnet": "claude-sonnet-4-5",
			"opus":   "claude-opus-4-6",
			"haiku":  "claude-haiku-4-5",
		},
	}

	r := newTestResolver(internalcue.LoadResult{})
	name := r.resolveModelName("sonnet", agent)
	if name != "sonnet" {
		t.Errorf("resolveModelName() = %q, want %q", name, "sonnet")
	}
}

func TestResolveModelName_SubstringMatch(t *testing.T) {
	t.Parallel()

	agent := orchestration.Agent{
		Models: map[string]string{
			"sonnet": "claude-sonnet-4-5",
			"opus":   "claude-opus-4-6",
			"haiku":  "claude-haiku-4-5",
		},
	}

	r := newTestResolver(internalcue.LoadResult{})
	name := r.resolveModelName("son", agent)
	if name != "sonnet" {
		t.Errorf("resolveModelName() = %q, want %q", name, "sonnet")
	}
}

func TestResolveModelName_Passthrough(t *testing.T) {
	t.Parallel()

	agent := orchestration.Agent{
		Models: map[string]string{
			"sonnet": "claude-sonnet-4-5",
		},
	}

	r := newTestResolver(internalcue.LoadResult{})
	name := r.resolveModelName("gpt-4o", agent)
	if name != "gpt-4o" {
		t.Errorf("resolveModelName() = %q, want %q (passthrough)", name, "gpt-4o")
	}
}

func TestResolveModelName_MultipleMatches_Passthrough(t *testing.T) {
	t.Parallel()

	agent := orchestration.Agent{
		Models: map[string]string{
			"sonnet-4":   "claude-sonnet-4",
			"sonnet-4-5": "claude-sonnet-4-5",
		},
	}

	r := newTestResolver(internalcue.LoadResult{})
	name := r.resolveModelName("sonnet", agent)
	// Multiple substring matches -> passthrough
	if name != "sonnet" {
		t.Errorf("resolveModelName() = %q, want %q (passthrough on multiple)", name, "sonnet")
	}
}

func TestResolveModelName_NilModels(t *testing.T) {
	t.Parallel()

	agent := orchestration.Agent{} // no models map
	r := newTestResolver(internalcue.LoadResult{})
	name := r.resolveModelName("sonnet", agent)
	if name != "sonnet" {
		t.Errorf("resolveModelName() = %q, want %q (passthrough)", name, "sonnet")
	}
}

func TestResolveModelName_Empty(t *testing.T) {
	t.Parallel()

	agent := orchestration.Agent{
		Models: map[string]string{"sonnet": "claude-sonnet"},
	}
	r := newTestResolver(internalcue.LoadResult{})
	name := r.resolveModelName("", agent)
	if name != "" {
		t.Errorf("resolveModelName('') = %q, want empty", name)
	}
}

func TestResolveContexts_ExactName(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		contexts: {
			env: {
				required: true
				prompt: "environment"
			}
			project: {
				default: true
				prompt: "project info"
			}
		}
	}`)

	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"env"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "env" {
		t.Errorf("resolveContexts([env]) = %v, want [env]", resolved)
	}
}

func TestResolveContexts_FilePathBypass(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{contexts: {}}`)
	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"./docs/guide.md"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "./docs/guide.md" {
		t.Errorf("resolveContexts([./docs/guide.md]) = %v, want [./docs/guide.md]", resolved)
	}
}

func TestResolveContexts_DefaultPassthrough(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{contexts: {}}`)
	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"default"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "default" {
		t.Errorf("resolveContexts([default]) = %v, want [default]", resolved)
	}
}

func TestResolveContexts_SearchMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		contexts: {
			"golang-env": {
				description: "Go development environment"
				tags: ["golang", "development"]
				prompt: "Go env context"
			}
		}
	}`)

	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "golang-env" {
		t.Errorf("resolveContexts([golang]) = %v, want [golang-env]", resolved)
	}
}

func TestResolveContexts_NoMatchPassthrough(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		contexts: {
			env: {
				prompt: "environment"
			}
		}
	}`)

	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No matches -> pass through as-is (composer will warn)
	if len(resolved) != 1 || resolved[0] != "nonexistent" {
		t.Errorf("resolveContexts([nonexistent]) = %v, want [nonexistent]", resolved)
	}
}

func TestResolveContexts_Mixed(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		contexts: {
			env: {
				required: true
				prompt: "environment"
			}
			"golang-env": {
				description: "Go development environment"
				tags: ["golang"]
				prompt: "Go env"
			}
		}
	}`)

	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"./custom.md", "default", "env"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resolved) != 3 {
		t.Fatalf("resolveContexts() returned %d items, want 3: %v", len(resolved), resolved)
	}
	if resolved[0] != "./custom.md" {
		t.Errorf("resolved[0] = %q, want %q", resolved[0], "./custom.md")
	}
	if resolved[1] != "default" {
		t.Errorf("resolved[1] = %q, want %q", resolved[1], "default")
	}
	if resolved[2] != "env" {
		t.Errorf("resolved[2] = %q, want %q", resolved[2], "env")
	}
}

func TestSelectSingleMatch_Zero(t *testing.T) {
	t.Parallel()

	r := newTestResolver(internalcue.LoadResult{})
	_, err := r.selectSingleMatch(nil, "agent", "test")
	if err == nil {
		t.Fatal("expected error for zero matches")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want containing 'not found'", err.Error())
	}
}

func TestSelectSingleMatch_Single(t *testing.T) {
	t.Parallel()

	matches := []ModuleMatch{{Name: "claude", Source: ModuleSourceInstalled, Score: 3}}
	r := newTestResolver(internalcue.LoadResult{})
	selected, err := r.selectSingleMatch(matches, "agent", "clau")
	if err != nil {
		t.Fatalf("selectSingleMatch() error = %v", err)
	}
	if selected.Name != "claude" {
		t.Errorf("selectSingleMatch().Name = %q, want %q", selected.Name, "claude")
	}
}

func TestPromptModuleSelection_NonTTY(t *testing.T) {
	t.Parallel()

	matches := []ModuleMatch{
		{Name: "claude-code", Source: ModuleSourceInstalled, Score: 3},
		{Name: "claude-chat", Source: ModuleSourceRegistry, Score: 3},
	}

	r := newTestResolver(internalcue.LoadResult{})
	_, err := r.promptModuleSelection(matches, "agent", "claude")
	if err == nil {
		t.Fatal("expected error for non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
}

func TestSearchInstalled(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			claude: {
				description: "Anthropic Claude"
				tags: ["ai"]
			}
		}
	}`)

	matches, err := searchInstalled(cfg.Value, "agents", "agents", "claude")
	if err != nil {
		t.Fatalf("searchInstalled() error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("searchInstalled() returned %d matches, want 1", len(matches))
	}
	if matches[0].Name != "claude" {
		t.Errorf("match.Name = %q, want %q", matches[0].Name, "claude")
	}
	if matches[0].Source != ModuleSourceInstalled {
		t.Errorf("match.Source = %q, want %q", matches[0].Source, ModuleSourceInstalled)
	}
}

func TestSearchRegistryCategory(t *testing.T) {
	t.Parallel()

	entries := map[string]registry.IndexEntry{
		"claude": {
			Module:      "github.com/test/agents/claude@v0",
			Description: "Anthropic Claude",
			Tags:        []string{"ai"},
		},
	}

	matches, err := searchRegistryCategory(entries, "agents", "claude")
	if err != nil {
		t.Fatalf("searchRegistryCategory() error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("searchRegistryCategory() returned %d matches, want 1", len(matches))
	}
	if matches[0].Name != "claude" {
		t.Errorf("match.Name = %q, want %q", matches[0].Name, "claude")
	}
	if matches[0].Source != ModuleSourceRegistry {
		t.Errorf("match.Source = %q, want %q", matches[0].Source, ModuleSourceRegistry)
	}
}

// TestResolveModule_SingleInstalledPlusRegistryMatch verifies that when a single
// installed substring match exists and the registry also has a matching module,
// both are presented for selection (error in non-TTY) rather than silently
// returning the installed match. This covers the structural bug fixed in p-041.
func TestResolveModule_SingleInstalledPlusRegistryMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		roles: {
			"golang/assistant": {
				description: "Go programming expert"
				prompt: "You are a Go expert"
			}
		}
	}`)

	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	// Inject fake registry index - "assistant" matches both installed and registry.
	r.didFetch = true
	r.index = &registry.Index{
		Roles: map[string]registry.IndexEntry{
			"python/assistant": {
				Module:      "github.com/test/roles/python/assistant@v0",
				Description: "Python programming expert",
			},
		},
	}

	_, err := r.resolveModule("assistant", internalcue.KeyRoles, "roles", "Role", true)
	if err == nil {
		t.Fatal("expected ambiguous error for multiple matches, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
	if !strings.Contains(err.Error(), "golang/assistant") {
		t.Errorf("error should list golang/assistant: %v", err)
	}
	if !strings.Contains(err.Error(), "python/assistant") {
		t.Errorf("error should list python/assistant: %v", err)
	}
}

// TestResolveModule_SingleInstalledNoRegistryMatch verifies that a single
// installed substring match resolves directly when the registry has no match.
func TestResolveModule_SingleInstalledNoRegistryMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		roles: {
			"golang/assistant": {
				description: "Go programming expert"
				prompt: "You are a Go expert"
			}
		}
	}`)

	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	// Registry has no assistant match.
	r.didFetch = true
	r.index = &registry.Index{
		Roles: map[string]registry.IndexEntry{
			"python/debug": {
				Module:      "github.com/test/roles/python/debug@v0",
				Description: "Python debugger",
			},
		},
	}

	name, err := r.resolveModule("assistant", internalcue.KeyRoles, "roles", "Role", true)
	if err != nil {
		t.Fatalf("resolveModule() error = %v", err)
	}
	if name != "golang/assistant" {
		t.Errorf("resolveModule() = %q, want %q", name, "golang/assistant")
	}
}

// TestResolveModule_ExactFullNameSkipsRegistry verifies that an exact full name
// match in installed config resolves via Phase 1 without touching the registry.
func TestResolveModule_ExactFullNameSkipsRegistry(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		roles: {
			"golang/assistant": {
				prompt: "You are a Go expert"
			}
		}
	}`)

	// skipRegistry=true: any registry call would return nil. Phase 1 must catch it.
	r := newTestResolver(cfg)
	name, err := r.resolveModule("golang/assistant", internalcue.KeyRoles, "roles", "Role", true)
	if err != nil {
		t.Fatalf("resolveModule() error = %v", err)
	}
	if name != "golang/assistant" {
		t.Errorf("resolveModule() = %q, want %q", name, "golang/assistant")
	}
}

// TestResolveModule_AgentSingleInstalledPlusRegistryMatch verifies the same
// fix applies for agent resolution via resolveAgent.
func TestResolveModule_AgentSingleInstalledPlusRegistryMatch(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			"claude-code": {
				description: "Claude for coding"
				bin: "claude"
				command: "{{.bin}}"
			}
		}
	}`)

	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	r.didFetch = true
	r.index = &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"claude-chat": {
				Module:      "github.com/test/agents/claude-chat@v0",
				Description: "Claude for chatting",
			},
		},
	}

	_, err := r.resolveAgent("claude")
	if err == nil {
		t.Fatal("expected ambiguous error for multiple matches, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
}

// TestContextScoreThreshold_LowScoreExcluded verifies context results below
// threshold are excluded.
func TestContextScoreThreshold_LowScoreExcluded(t *testing.T) {
	t.Parallel()

	// "env" prompt doesn't contain "golang" so it should score 0.
	// "golang-env" has "golang" in name (3 points) + tag (1 point) = 4.
	cfg := buildTestCfg(t, `{
		contexts: {
			env: {
				prompt: "basic environment"
			}
			"golang-env": {
				description: "Go development environment"
				tags: ["golang"]
				prompt: "Go env"
			}
		}
	}`)

	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"golang"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only find golang-env (score 4), not env (score 0)
	if len(resolved) != 1 {
		t.Fatalf("resolveContexts() returned %d items, want 1: %v", len(resolved), resolved)
	}
	if resolved[0] != "golang-env" {
		t.Errorf("resolved[0] = %q, want %q", resolved[0], "golang-env")
	}
}

// TestResolveAgent_PrefixMatching verifies that agents:<name> strips the
// matching category prefix and resolves the bare name as usual.
func TestResolveAgent_PrefixMatching(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			claude: {
				bin: "claude"
				command: "{{.bin}}"
			}
		}
	}`)

	r := newTestResolver(cfg)
	got, err := r.resolveAgent("agents:claude")
	if err != nil {
		t.Fatalf("resolveAgent(agents:claude) error: %v", err)
	}
	if got != "claude" {
		t.Errorf("resolveAgent = %q, want %q", got, "claude")
	}
}

// TestResolveAgent_PrefixMismatchError verifies that a mismatched prefix
// (e.g. roles:foo passed via --agent) returns an error naming the mismatch.
func TestResolveAgent_PrefixMismatchError(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	_, err := r.resolveAgent("roles:assistant")
	if err == nil {
		t.Fatal("expected mismatch error from --agent receiving a roles: address")
	}
	msg := err.Error()
	if !strings.Contains(msg, "agents") || !strings.Contains(msg, "roles") {
		t.Errorf("error should name both expected and given category, got: %v", err)
	}
}

// TestResolveAgent_UnknownPrefixError verifies an unknown category prefix
// returns the unknown-category error.
func TestResolveAgent_UnknownPrefixError(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	_, err := r.resolveAgent("foo:bar")
	if err == nil {
		t.Fatal("expected unknown-category error")
	}
	if !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("error should mention 'unknown category', got: %v", err)
	}
}

// TestResolveContexts_PrefixMatching verifies a contexts: prefix on a context
// term is stripped and resolution proceeds normally.
func TestResolveContexts_PrefixMatching(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		contexts: {
			"golang-env": {
				prompt: "Go environment"
				tags: ["env"]
			}
		}
	}`)

	r := newTestResolver(cfg)
	resolved, err := r.resolveContexts([]string{"contexts:golang-env"})
	if err != nil {
		t.Fatalf("resolveContexts(contexts:golang-env) error: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "golang-env" {
		t.Errorf("resolved = %v, want [golang-env]", resolved)
	}
}

// TestResolveContexts_PrefixMismatchError verifies a non-contexts prefix on a
// context term returns a mismatch error.
func TestResolveContexts_PrefixMismatchError(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	_, err := r.resolveContexts([]string{"agents:claude"})
	if err == nil {
		t.Fatal("expected mismatch error from -c receiving an agents: address")
	}
	if !strings.Contains(err.Error(), "contexts") {
		t.Errorf("error should name the expected category 'contexts', got: %v", err)
	}
}
