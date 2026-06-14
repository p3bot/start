package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/start-cli/start/internal/cache"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/registry"
	"golang.org/x/mod/semver"
)

// canonicalIndexVersion is the fresh-cache version the stub and seeded cache
// agree on: the index module path at a canonical (non-bare-major) version.
const canonicalIndexVersion = "github.com/start-cli/library/index@" + stubVersion

// seedFreshIndexCache writes a fresh index cache pointing at the canonical index
// version, so decideCachedIndex's module-match guard and IsFresh both hold. The
// caller must already have an isolated XDG_CACHE_HOME (setupStartTestConfig*/
// isolateConfigEnv).
func seedFreshIndexCache(t *testing.T) {
	t.Helper()
	if err := cache.WriteIndex(canonicalIndexVersion); err != nil {
		t.Fatalf("seeding fresh index cache: %v", err)
	}
}

// seedStaleIndexCache writes an index cache for the canonical version stamped
// 25h ago, so IsFresh(DefaultMaxAge) is false and decideCachedIndex must resolve
// live. WriteIndex always stamps time.Now(), so the file is written directly in
// the same `index_updated: ".."` / `index_version: ".."` format. The caller must
// already have an isolated XDG_CACHE_HOME.
func seedStaleIndexCache(t *testing.T) {
	t.Helper()
	dir, err := cache.Dir()
	if err != nil {
		t.Fatalf("resolving cache dir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating cache dir: %v", err)
	}
	stale := time.Now().Add(-25 * time.Hour).Format(time.RFC3339)
	content := fmt.Sprintf("index_updated: %q\nindex_version: %q\n", stale, canonicalIndexVersion)
	if err := os.WriteFile(filepath.Join(dir, "cache.cue"), []byte(content), 0o644); err != nil {
		t.Fatalf("seeding stale index cache: %v", err)
	}
}

// newRecordingResolver builds a resolver wired to a recording stub via the
// testClient seam, with the offline default disabled so ensureIndex runs its
// full cache/wantLive decision and resolves through the stub. The driver's
// up-front union is not run here; tests set r.wantLive (or call computeWantLive)
// to simulate the decision under test.
func newRecordingResolver(cfg internalcue.LoadResult, stub *registryStub) *resolver {
	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	r.skipRegistry = false
	r.testClient = stub
	return r
}

// isBareMajor reports whether path carries a bare major version (e.g. @v1) — the
// live-resolve marker — rather than a canonical one (@v1.0.0), the cache-gated
// marker.
func isBareMajor(path string) bool {
	v := path[strings.LastIndex(path, "@")+1:]
	return semver.Canonical(v) != v
}

// --- Display-command cache-gating (observed via stub.resolvePaths) ---

func TestDisplay_FreshCacheNoMetadataResolve(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		prep func(t *testing.T, tmpDir string)
	}{
		{"library", []string{"library", "--json"}, nil},
		{"search", []string{"search", "sentinel", "--json"}, nil},
		{"list-verbose", []string{"list", "--verbose", "--json"}, writeInstalledRegistryAgent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
			if tc.prep != nil {
				tc.prep(t, tmpDir)
			}
			seedFreshIndexCache(t)

			if _, err := captureText(t, stub, tc.args...); err != nil {
				t.Fatalf("%v: %v", tc.args, err)
			}
			if len(stub.resolvePaths) != 0 {
				t.Errorf("fresh cache should make no metadata resolve, got %v", stub.resolvePaths)
			}
		})
	}
}

func TestDisplay_MissingCacheResolvesAndWrites(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	// No seed: the isolated cache is empty.

	if _, err := captureText(t, stub, "library", "--json"); err != nil {
		t.Fatalf("library --json: %v", err)
	}
	if len(stub.resolvePaths) != 1 {
		t.Errorf("missing cache should trigger exactly one metadata resolve, got %v", stub.resolvePaths)
	}
	cached, err := cache.ReadIndex()
	if err != nil {
		t.Fatalf("cache should have been written: %v", err)
	}
	if !cached.IsFresh(time.Hour) {
		t.Errorf("written cache should be fresh, updated=%v", cached.Updated)
	}
}

func TestDisplay_StaleCacheResolvesAndWrites(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	seedStaleIndexCache(t)

	if _, err := captureText(t, stub, "library", "--json"); err != nil {
		t.Fatalf("library --json: %v", err)
	}
	if len(stub.resolvePaths) != 1 {
		t.Errorf("stale cache should trigger exactly one metadata resolve, got %v", stub.resolvePaths)
	}
	cached, err := cache.ReadIndex()
	if err != nil {
		t.Fatalf("cache should have been rewritten: %v", err)
	}
	if !cached.IsFresh(time.Hour) {
		t.Errorf("rewritten cache should be fresh, updated=%v", cached.Updated)
	}
}

// TestResolveDisplayIndexVersion_ResolveFailureReturnsError covers the display
// helper's own ResolveLatestVersion-failure branch (distinct from a FetchIndex
// failure): an empty cache forces the live resolve, which errors, and the helper
// must surface that error to the caller (search/library/checkForUpdates then
// degrade to local-only).
func TestResolveDisplayIndexVersion_ResolveFailureReturnsError(t *testing.T) {
	isolateConfigEnv(t) // empty cache → the helper takes the live-resolve branch
	stub := newRegistryStub(stubLibraryIndex(), "")
	stub.SetResolve(registry.IndexModulePath, "", fmt.Errorf("resolve boom"))

	_, err := resolveDisplayIndexVersion(context.Background(), stub, "", io.Discard, &Flags{})
	if err == nil {
		t.Fatal("a ResolveLatestVersion failure must propagate as an error")
	}
}

func TestDisplay_RefreshForcesLiveAcrossModes(t *testing.T) {
	for _, args := range [][]string{
		{"library", "--refresh"},
		{"library", "--refresh", "--json"},
		{"library", "--refresh", "--export"},
		{"search", "sentinel", "--refresh", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
			seedFreshIndexCache(t)

			if _, err := captureText(t, stub, args...); err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			if len(stub.resolvePaths) != 1 {
				t.Errorf("--refresh should force one live resolve despite a fresh cache, got %v", stub.resolvePaths)
			}
		})
	}
}

func TestList_RefreshForcesLive(t *testing.T) {
	tmpDir, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	writeInstalledRegistryAgent(t, tmpDir)
	seedFreshIndexCache(t)

	if _, err := captureText(t, stub, "list", "--verbose", "--refresh", "--json"); err != nil {
		t.Fatalf("list --verbose --refresh: %v", err)
	}
	if len(stub.resolvePaths) != 1 {
		t.Errorf("list --verbose --refresh should force a live resolve, got %v", stub.resolvePaths)
	}
}

// --- Resolver-path liveness (observed via stub.fetchIndexPaths) ---

func TestEnsureIndex_CacheGatedFetchesCanonical(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	stub := newRegistryStub(stubLibraryIndex(), "")
	r := newRecordingResolver(buildTestCfg(t, `{}`), stub)
	r.wantLive = false

	idx, _, err := r.ensureIndex()
	if err != nil || idx == nil {
		t.Fatalf("ensureIndex() = (%v, %v), want index", idx, err)
	}
	if len(stub.fetchIndexPaths) != 1 || isBareMajor(stub.fetchIndexPaths[0]) {
		t.Errorf("cache-gated resolve should fetch the canonical version, got %v", stub.fetchIndexPaths)
	}
}

func TestEnsureIndex_LiveFetchesBareMajor(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	stub := newRegistryStub(stubLibraryIndex(), "")
	r := newRecordingResolver(buildTestCfg(t, `{}`), stub)
	r.wantLive = true

	if _, _, err := r.ensureIndex(); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}
	if len(stub.fetchIndexPaths) != 1 || !isBareMajor(stub.fetchIndexPaths[0]) {
		t.Errorf("live resolve should fetch the bare-major path, got %v", stub.fetchIndexPaths)
	}
}

func TestEnsureIndex_LiveResolveErrorDegrades(t *testing.T) {
	isolateConfigEnv(t) // empty cache → live regardless
	stub := newRegistryStub(stubLibraryIndex(), "")
	stub.SetFetchIndexError(fmt.Errorf("registry down"))
	r := newRecordingResolver(buildTestCfg(t, `{}`), stub)
	r.wantLive = true

	idx, _, err := r.ensureIndex()
	if err != nil {
		t.Errorf("a live-resolve failure must degrade gracefully, got err %v", err)
	}
	if idx != nil {
		t.Errorf("expected nil index on failure, got %v", idx)
	}
	if r.indexErr == nil {
		t.Error("indexErr should record the failure")
	}
}

// TestForceLiveReResolve_RefetchesLive models the late task-declared-role path:
// a cache-gated resolve already happened, then the late check forces one live
// re-resolve.
func TestForceLiveReResolve_RefetchesLive(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	stub := newRegistryStub(stubLibraryIndex(), "")
	r := newRecordingResolver(buildTestCfg(t, `{}`), stub)

	r.wantLive = false
	if _, _, err := r.ensureIndex(); err != nil { // cache-gated → canonical
		t.Fatalf("first ensureIndex: %v", err)
	}
	r.forceLiveReResolve()
	if _, _, err := r.ensureIndex(); err != nil { // live → bare-major
		t.Fatalf("re-resolve ensureIndex: %v", err)
	}

	if len(stub.fetchIndexPaths) != 2 {
		t.Fatalf("expected two fetches, got %v", stub.fetchIndexPaths)
	}
	if isBareMajor(stub.fetchIndexPaths[0]) {
		t.Errorf("first fetch should be cache-gated (canonical), got %q", stub.fetchIndexPaths[0])
	}
	if !isBareMajor(stub.fetchIndexPaths[1]) {
		t.Errorf("forced re-resolve should be live (bare-major), got %q", stub.fetchIndexPaths[1])
	}
}

// TestResolveExactInstalled_NoRegistryCall asserts a category-specific lone
// installed exact match resolves without touching the index at all (it
// short-circuits before ensureIndex), so even a recording client sees no fetch.
func TestResolveExactInstalled_NoRegistryCall(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	stub := newRegistryStub(stubLibraryIndex(), "")
	cfg := buildTestCfg(t, `{roles: {"go-expert": {prompt: "Go"}}}`)
	r := newRecordingResolver(cfg, stub)
	r.wantLive = r.computeWantLive([]pendingSurface{{"go-expert", roleScope()}})

	name, err := r.resolveRole("go-expert")
	if err != nil {
		t.Fatalf("resolveRole(go-expert) error = %v", err)
	}
	if name != "go-expert" {
		t.Errorf("resolved %q, want go-expert", name)
	}
	if len(stub.fetchIndexPaths) != 0 {
		t.Errorf("an installed exact match should make no registry call, got %v", stub.fetchIndexPaths)
	}
}

// --- resolveCross liveness ---

func TestResolveCross_UninstalledResolvesLive(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	stub := newRegistryStub(&registry.Index{}, "")
	r := newRecordingResolver(buildTestCfg(t, `{}`), stub)

	// Not installed and absent from the index: not-found, but the path taken to
	// confirm that must be live so a freshly published module would be seen.
	if _, err := r.resolveCross("missing-module"); err == nil {
		t.Fatal("expected not-found for an uninstalled, absent query")
	}
	if len(stub.fetchIndexPaths) == 0 || !isBareMajor(stub.fetchIndexPaths[0]) {
		t.Errorf("uninstalled get/describe query should resolve live, got %v", stub.fetchIndexPaths)
	}
}

func TestResolveCross_InstalledStaysCacheGated(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	stub := newRegistryStub(&registry.Index{}, "")
	cfg := buildTestCfg(t, `{agents: {claude: {bin: "claude", command: "{{.bin}} run"}}}`)
	r := newRecordingResolver(cfg, stub)

	outcome, err := r.resolveCross("claude")
	if err != nil {
		t.Fatalf("resolveCross(claude) error = %v", err)
	}
	if outcome.match.Name != "claude" {
		t.Errorf("resolved %q, want claude", outcome.match.Name)
	}
	if len(stub.fetchIndexPaths) != 1 || isBareMajor(stub.fetchIndexPaths[0]) {
		t.Errorf("installed get/describe query should stay cache-gated (canonical), got %v", stub.fetchIndexPaths)
	}
}

func TestResolveCross_RefreshForcesLiveForInstalled(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	stub := newRegistryStub(&registry.Index{}, "")
	cfg := buildTestCfg(t, `{agents: {claude: {bin: "claude", command: "{{.bin}} run"}}}`)
	r := newRecordingResolver(cfg, stub)
	r.flags.Refresh = true

	if _, err := r.resolveCross("claude"); err != nil {
		t.Fatalf("resolveCross(claude) error = %v", err)
	}
	if len(stub.fetchIndexPaths) == 0 || !isBareMajor(stub.fetchIndexPaths[0]) {
		t.Errorf("--refresh should force a live resolve even for an installed query, got %v", stub.fetchIndexPaths)
	}
}

// --- Substring-shadow boundary (path observable) ---

// TestSubstringShadow_StaysCacheGated covers the accepted limitation: a
// substring-installed surface (--role expert with go-expert installed) stays
// cache-gated under a fresh cache and resolves to the installed go-expert with
// no live resolve, while --refresh flips it live.
func TestSubstringShadow_CacheGatedThenRefreshLive(t *testing.T) {
	cfg := buildTestCfg(t, `{roles: {"go-expert": {prompt: "Go"}}}`)

	t.Run("cache-gated", func(t *testing.T) {
		isolateConfigEnv(t)
		seedFreshIndexCache(t)
		stub := newRegistryStub(&registry.Index{}, "")
		r := newRecordingResolver(cfg, stub)
		// Simulate the up-front union: "expert" is substring-satisfied by the
		// installed go-expert, so the invocation stays cache-gated.
		r.wantLive = r.computeWantLive([]pendingSurface{{"expert", singleCategoryScope("roles", "role", true)}})

		name, err := r.resolveRole("expert")
		if err != nil {
			t.Fatalf("resolveRole(expert) error = %v", err)
		}
		if name != "go-expert" {
			t.Errorf("resolved %q, want go-expert", name)
		}
		for _, p := range stub.fetchIndexPaths {
			if isBareMajor(p) {
				t.Errorf("substring-installed surface should stay cache-gated, saw live fetch %q", p)
			}
		}
	})

	t.Run("refresh-live", func(t *testing.T) {
		isolateConfigEnv(t)
		seedFreshIndexCache(t)
		stub := newRegistryStub(&registry.Index{}, "")
		r := newRecordingResolver(cfg, stub)
		r.flags.Refresh = true
		r.wantLive = r.computeWantLive([]pendingSurface{{"expert", singleCategoryScope("roles", "role", true)}})

		if _, err := r.resolveRole("expert"); err != nil {
			t.Fatalf("resolveRole(expert) error = %v", err)
		}
		if len(stub.fetchIndexPaths) == 0 || !isBareMajor(stub.fetchIndexPaths[0]) {
			t.Errorf("--refresh should flip the substring-installed surface live, got %v", stub.fetchIndexPaths)
		}
	})
}
