package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/registry"
)

func TestResolveCrossCategory_ZeroMatches(t *testing.T) {
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
	_, err := resolveCrossCategory("nonexistent", r)
	if err == nil {
		t.Fatal("expected error for zero matches")
	}
	if !strings.Contains(err.Error(), "no matches found") {
		t.Errorf("error = %q, want containing 'no matches found'", err.Error())
	}
}

func TestResolveCrossCategory_SingleInstalledExact(t *testing.T) {
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
	match, err := resolveCrossCategory("claude", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.Name != "claude" {
		t.Errorf("match.Name = %q, want %q", match.Name, "claude")
	}
	if match.Category != "agents" {
		t.Errorf("match.Category = %q, want %q", match.Category, "agents")
	}
	if match.Source != ModuleSourceInstalled {
		t.Errorf("match.Source = %q, want %q", match.Source, ModuleSourceInstalled)
	}
	if r.didInstall {
		t.Error("didInstall should be false for an installed match")
	}
}

func TestResolveCrossCategory_AmbiguousShortNameNonTTY(t *testing.T) {
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

	r := newTestResolver(cfg)
	_, err := resolveCrossCategory("debug", r)
	if err == nil {
		t.Fatal("expected ambiguity error for non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
}

func TestResolveCrossCategory_SingleInstalledSubstring(t *testing.T) {
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
	match, err := resolveCrossCategory("gemini", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if match.Name != "gemini-non-interactive" {
		t.Errorf("match.Name = %q, want %q", match.Name, "gemini-non-interactive")
	}
	if match.Category != "agents" {
		t.Errorf("match.Category = %q, want %q", match.Category, "agents")
	}
}

func TestResolveCrossCategory_CombinedSearchMultipleNonTTY(t *testing.T) {
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
	_, err := resolveCrossCategory("claude", r)
	if err == nil {
		t.Fatal("expected ambiguity error for multiple matches in non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
}

// A single exact match coexisting with substring matches must surface a
// selection rather than silently returning the exact match.
func TestResolveCrossCategory_ExactPlusSubstringFallThrough(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			claude: {
				bin: "claude"
				command: "{{.bin}}"
			}
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
	_, err := resolveCrossCategory("claude", r)
	if err == nil {
		t.Fatal("expected ambiguity error for exact-plus-substring fall-through in non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
}

// Short-name ambiguity collected from multiple categories produces one list.
func TestResolveCrossCategory_AmbiguousAcrossCategories(t *testing.T) {
	t.Parallel()

	// "debug" is ambiguous within both tasks and roles; both contribute to
	// the aggregated ambiguousMatches slice.
	cfg := buildTestCfg(t, `{
		tasks: {
			"review/debug": {prompt: "Review debug"}
			"golang/debug": {prompt: "Debug Go code"}
		}
		roles: {
			"frontend/debug": {prompt: "Frontend debugger"}
			"backend/debug": {prompt: "Backend debugger"}
		}
	}`)

	r := newTestResolver(cfg)
	_, err := resolveCrossCategory("debug", r)
	if err == nil {
		t.Fatal("expected ambiguity error spanning categories")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
	if !strings.Contains(err.Error(), "tasks:") {
		t.Errorf("error should list tasks entries: %v", err)
	}
	if !strings.Contains(err.Error(), "roles:") {
		t.Errorf("error should list roles entries: %v", err)
	}
}

func TestInstallIfRegistry_InstalledIsNoop(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	match := ModuleMatch{Name: "foo", Category: "roles", Source: ModuleSourceInstalled}

	if err := r.installIfRegistry(match); err != nil {
		t.Fatalf("installIfRegistry(installed) error = %v, want nil", err)
	}
	if r.didInstall {
		t.Error("didInstall should remain false for installed match")
	}
}

// Fail-fast guard for the "index loaded, client missing" state.
func TestInstallIfRegistry_RegistryWithoutClient(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	// r.client is nil by construction.
	match := ModuleMatch{Name: "foo", Category: "roles", Source: ModuleSourceRegistry}

	err := r.installIfRegistry(match)
	if err == nil {
		t.Fatal("expected error when installing registry match without client")
	}
	if !strings.Contains(err.Error(), "registry client unavailable") {
		t.Errorf("error = %q, want containing 'registry client unavailable'", err.Error())
	}
	if r.didInstall {
		t.Error("didInstall should remain false when install fails")
	}
}

// The exact-registry branch reaches installIfRegistry; install fails
// deterministically (r.client is nil), confirming the path executed.
func TestResolveCrossCategory_ExactRegistryBranchInstalls(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{roles: {}}`)
	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	// didFetch makes ensureIndex short-circuit and return the injected index.
	r.didFetch = true
	r.index = &registry.Index{
		Roles: map[string]registry.IndexEntry{
			"golang/assistant": {
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go programming expert",
			},
		},
	}

	_, err := resolveCrossCategory("assistant", r)
	if err == nil {
		t.Fatal("expected error from install attempt in exact-registry branch")
	}
	if !strings.Contains(err.Error(), "registry client unavailable") {
		t.Errorf("error = %q, want containing 'registry client unavailable'", err.Error())
	}
	if r.didInstall {
		t.Error("didInstall should remain false when install fails")
	}
}

// A query that misses findExactInRegistry but matches searchRegistryCategory
// forces the combined-search single-registry branch into installIfRegistry.
func TestResolveCrossCategory_CombinedSingleRegistryBranchInstalls(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{roles: {}}`)
	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	r.didFetch = true
	r.index = &registry.Index{
		Roles: map[string]registry.IndexEntry{
			"golang/assistant": {
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go programming expert",
				Tags:        []string{"golang"},
			},
		},
	}

	// "golang" is neither a full key nor a short name, so the exact-registry
	// branch misses and only substring search against key/tags finds it.
	_, err := resolveCrossCategory("golang", r)
	if err == nil {
		t.Fatal("expected error from install attempt in combined-search branch")
	}
	if !strings.Contains(err.Error(), "registry client unavailable") {
		t.Errorf("error = %q, want containing 'registry client unavailable'", err.Error())
	}
	if r.didInstall {
		t.Error("didInstall should remain false when install fails")
	}
}

// Multiple combined-search registry matches surface as a non-TTY ambiguity
// error without attempting to install.
func TestResolveCrossCategory_CombinedMultipleRegistryNonTTY(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{roles: {}}`)
	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	r.didFetch = true
	r.index = &registry.Index{
		Roles: map[string]registry.IndexEntry{
			"frontend/debugger": {
				Module:      "github.com/test/roles/frontend/debugger@v0",
				Description: "Frontend debugger",
				Tags:        []string{"debugger"},
			},
			"backend/debugger": {
				Module:      "github.com/test/roles/backend/debugger@v0",
				Description: "Backend debugger",
				Tags:        []string{"debugger"},
			},
		},
	}

	_, err := resolveCrossCategory("debugger", r)
	if err == nil {
		t.Fatal("expected ambiguity error for multiple registry matches in non-TTY")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
	if r.didInstall {
		t.Error("didInstall should remain false — no install should occur when ambiguous")
	}
}

// A query matching an exact key in more than one category reaches the
// len(exactMatches) > 1 branch and surfaces a non-TTY ambiguity error.
func TestResolveCrossCategory_MultipleExactAcrossCategoriesNonTTY(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			foo: {
				bin: "foo"
				command: "{{.bin}}"
			}
		}
		roles: {
			foo: {
				prompt: "Foo role"
			}
		}
	}`)

	r := newTestResolver(cfg)
	_, err := resolveCrossCategory("foo", r)
	if err == nil {
		t.Fatal("expected ambiguity error for multiple exact matches across categories")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
	if !strings.Contains(err.Error(), "agents:foo") {
		t.Errorf("error should list agents entry: %v", err)
	}
	if !strings.Contains(err.Error(), "roles:foo") {
		t.Errorf("error should list roles entry: %v", err)
	}
	if r.didInstall {
		t.Error("didInstall should remain false when ambiguous")
	}
}

// An exact match coexisting with a substring match in a *different* category
// must surface a selection rather than silently picking the exact match.
func TestResolveCrossCategory_ExactPlusCrossCategorySubstring(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			claude: {
				bin: "claude"
				command: "{{.bin}}"
			}
		}
		roles: {
			"claude-expert": {
				prompt: "Claude expert role"
			}
		}
	}`)

	r := newTestResolver(cfg)
	_, err := resolveCrossCategory("claude", r)
	if err == nil {
		t.Fatal("expected ambiguity error: exact agents:claude alongside roles:claude-expert should not silently pick the exact match")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
	if !strings.Contains(err.Error(), "agents:claude") {
		t.Errorf("error should list the exact match entry: %v", err)
	}
	if !strings.Contains(err.Error(), "roles:claude-expert") {
		t.Errorf("error should list the substring neighbour: %v", err)
	}
}

// A matching category prefix scopes the search even when the bare name exists
// in another category.
func TestResolveCrossCategory_PrefixMatching(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			foo: {
				bin: "foo"
				command: "{{.bin}}"
			}
		}
		roles: {
			foo: {
				prompt: "Foo role"
			}
		}
	}`)

	r := newTestResolver(cfg)
	match, err := resolveCrossCategory("agents:foo", r)
	if err != nil {
		t.Fatalf("unexpected error with matching prefix: %v", err)
	}
	if match.Category != "agents" {
		t.Errorf("match.Category = %q, want %q", match.Category, "agents")
	}
	if match.Name != "foo" {
		t.Errorf("match.Name = %q, want %q", match.Name, "foo")
	}
}

// A prefix with a name that exists only in a different category returns "no
// matches" rather than silently falling back.
func TestResolveCrossCategory_PrefixScopesAbsence(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		roles: {
			"golang/assistant": {
				prompt: "Go assistant"
			}
		}
	}`)

	r := newTestResolver(cfg)
	_, err := resolveCrossCategory("contexts:golang/assistant", r)
	if err == nil {
		t.Fatal("expected 'no matches' error when the prefix scopes away the only candidate")
	}
	if !strings.Contains(err.Error(), "no matches found") {
		t.Errorf("error = %q, want 'no matches found' (no fallback to roles)", err.Error())
	}
}

func TestResolveCrossCategory_UnknownPrefix(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	_, err := resolveCrossCategory("foo:bar", r)
	if err == nil {
		t.Fatal("expected unknown-category error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown category") || !strings.Contains(msg, `"foo"`) {
		t.Errorf("error should name the unknown category, got: %v", err)
	}
	for _, want := range []string{"agents", "roles", "contexts", "tasks"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error should list valid category %q, got: %v", want, err)
		}
	}
}

// A non-TTY ambiguity-error candidate, fed back as input, resolves to one
// match — the round-trip property required by the address scheme.
func TestResolveCrossCategory_AmbiguityRoundTrip(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{
		agents: {
			foo: {
				bin: "foo"
				command: "{{.bin}}"
			}
		}
		roles: {
			foo: {
				prompt: "Foo role"
			}
		}
	}`)

	r := newTestResolver(cfg)
	_, err := resolveCrossCategory("foo", r)
	if err == nil {
		t.Fatal("expected ambiguity error for cross-category exact matches")
	}
	for _, candidate := range []string{"agents:foo", "roles:foo"} {
		r2 := newTestResolver(cfg)
		match, err := resolveCrossCategory(candidate, r2)
		if err != nil {
			t.Fatalf("round-trip resolve(%q) error: %v", candidate, err)
		}
		if got := formatAddress(match.Category, match.Name); got != candidate {
			t.Errorf("round-trip resolve(%q) returned %q", candidate, got)
		}
	}
}

func TestParseAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input        string
		wantCategory string
		wantName     string
		wantPrefix   bool
		wantErr      bool
	}{
		{"claude", "", "claude", false, false},
		{"claude/interactive", "", "claude/interactive", false, false},
		{"agents:claude", "agents", "claude", true, false},
		{"agents:claude/interactive", "agents", "claude/interactive", true, false},
		{"contexts:cwd/agents-md", "contexts", "cwd/agents-md", true, false},
		{"foo:bar", "", "", false, true},
		{"agent:claude", "", "", false, true}, // singular typo, not in valid set
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			addr, err := parseAddress(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseAddress(%q) expected error, got %+v", tt.input, addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAddress(%q) unexpected error: %v", tt.input, err)
			}
			if addr.Category != tt.wantCategory {
				t.Errorf("Category = %q, want %q", addr.Category, tt.wantCategory)
			}
			if addr.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", addr.Name, tt.wantName)
			}
			if addr.HasPrefix != tt.wantPrefix {
				t.Errorf("HasPrefix = %v, want %v", addr.HasPrefix, tt.wantPrefix)
			}
		})
	}
}
