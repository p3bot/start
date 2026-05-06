package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/registry"
)

// TestResolveCrossCategory_ZeroMatches verifies zero matches across all
// categories returns a "no matches" error.
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

// TestResolveCrossCategory_SingleInstalledExact verifies a single installed
// exact match is returned without prompting.
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
	if match.Source != AssetSourceInstalled {
		t.Errorf("match.Source = %q, want %q", match.Source, AssetSourceInstalled)
	}
	if r.didInstall {
		t.Error("didInstall should be false for an installed match")
	}
}

// TestResolveCrossCategory_AmbiguousShortNameNonTTY verifies an ambiguous
// short-name exact match returns an ambiguity error in non-TTY mode.
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

// TestResolveCrossCategory_SingleInstalledSubstring verifies a single installed
// substring match is returned without prompting.
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

// TestResolveCrossCategory_CombinedSearchMultipleNonTTY verifies multiple
// installed matches (combined-search path) return an ambiguity error in
// non-TTY mode.
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

// TestResolveCrossCategory_ExactPlusSubstringFallThrough verifies the
// fall-through case: a single exact match that coexists with additional
// substring matches must surface a selection (ambiguity error in non-TTY)
// rather than silently returning the exact match.
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

// TestResolveCrossCategory_AmbiguousAcrossCategories verifies that short-name
// ambiguity collected from multiple categories produces one combined list.
func TestResolveCrossCategory_AmbiguousAcrossCategories(t *testing.T) {
	t.Parallel()

	// "debug" is ambiguous within tasks (two review/* entries) AND within
	// roles (two *-expert entries). Both categories contribute to the
	// aggregated ambiguousMatches slice.
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
	if !strings.Contains(err.Error(), "tasks/") {
		t.Errorf("error should list tasks entries: %v", err)
	}
	if !strings.Contains(err.Error(), "roles/") {
		t.Errorf("error should list roles entries: %v", err)
	}
}

// TestInstallIfRegistry_InstalledIsNoop verifies the helper short-circuits
// without touching the registry client for installed matches.
func TestInstallIfRegistry_InstalledIsNoop(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	match := AssetMatch{Name: "foo", Category: "roles", Source: AssetSourceInstalled}

	if err := r.installIfRegistry(match); err != nil {
		t.Fatalf("installIfRegistry(installed) error = %v, want nil", err)
	}
	if r.didInstall {
		t.Error("didInstall should remain false for installed match")
	}
}

// TestInstallIfRegistry_RegistryWithoutClient verifies the helper returns a
// clear error when asked to install a registry match but no client is present.
// This is the fail-fast guard for the "index loaded, client missing" state.
func TestInstallIfRegistry_RegistryWithoutClient(t *testing.T) {
	t.Parallel()

	r := newTestResolver(buildTestCfg(t, `{}`))
	// r.client is nil by construction.
	match := AssetMatch{Name: "foo", Category: "roles", Source: AssetSourceRegistry}

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

// TestResolveCrossCategory_ExactRegistryBranchInstalls verifies the exact
// registry match branch is reached and installIfRegistry is invoked.
// Install fails deterministically because r.client is nil; that failure
// confirms the code path executed as intended.
func TestResolveCrossCategory_ExactRegistryBranchInstalls(t *testing.T) {
	t.Parallel()

	cfg := buildTestCfg(t, `{roles: {}}`)
	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	// Inject index with a registry entry whose short name matches the query;
	// mark didFetch so ensureIndex short-circuits and returns (index, nil, nil).
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

// TestResolveCrossCategory_CombinedSingleRegistryBranchInstalls verifies the
// combined-search single-registry match branch is reached and triggers
// installIfRegistry. The query is a substring that does not match via
// findExactInRegistry (full name nor short name) but matches via
// searchRegistryCategory, forcing execution into the combined-search path.
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

	// "golang" is not a full key and not a short name ("assistant" is), so
	// the exact-registry branch misses. Substring search against the key
	// and tags must find it, leading to the single-match combined-search path.
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

// TestResolveCrossCategory_CombinedMultipleRegistryNonTTY verifies that
// multiple combined-search registry matches surface as an ambiguity error
// in non-TTY mode without attempting to install.
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

// TestResolveCrossCategory_MultipleExactAcrossCategoriesNonTTY verifies that a
// query matching an exact top-level key in more than one category reaches the
// `len(exactMatches) > 1` branch and surfaces an ambiguity error in non-TTY
// mode listing every category-qualified match.
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
	if !strings.Contains(err.Error(), "agents/foo") {
		t.Errorf("error should list agents entry: %v", err)
	}
	if !strings.Contains(err.Error(), "roles/foo") {
		t.Errorf("error should list roles entry: %v", err)
	}
	if r.didInstall {
		t.Error("didInstall should remain false when ambiguous")
	}
}

// TestResolveCrossCategory_ExactPlusCrossCategorySubstring verifies the bug
// fix: when a single exact match coexists with a single substring match in a
// *different* category, the resolver must surface a selection rather than
// silently picking the exact match. Pre-fix, the per-category ambiguity gate
// only triggered selection when one category had >1 substring hit, missing
// cross-category neighbours entirely.
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
		t.Fatal("expected ambiguity error: exact agents/claude alongside roles/claude-expert should not silently pick the exact match")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want containing 'ambiguous'", err.Error())
	}
	if !strings.Contains(err.Error(), "agents/claude") {
		t.Errorf("error should list the exact match entry: %v", err)
	}
	if !strings.Contains(err.Error(), "roles/claude-expert") {
		t.Errorf("error should list the substring neighbour: %v", err)
	}
}
