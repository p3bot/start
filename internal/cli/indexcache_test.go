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

	"github.com/p3bot/start/internal/cache"
	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/registry"
)

// canonicalIndexVersion is the fresh-cache version the stub and seeded cache
// agree on: the index module path at a canonical (non-bare-major) version.
const canonicalIndexVersion = "github.com/p3bot/library/index@" + stubVersion

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

// recordingIndexSource is a test index source that consumes the real
// decideCachedIndex cache-gating decision and records, per acquisition, whether
// it resolved live (bare-major) or cache-gated (canonical). It returns a canned
// index, or a soft failure when fetchErr is set, so resolver-path liveness tests
// assert the decision on the fake rather than through a smuggled client.
type recordingIndexSource struct {
	index    *registry.Index
	fetchErr error

	// live records one entry per fetch: true when the acquisition resolved live
	// (bare-major), false when it was cache-gated (canonical).
	live []bool
}

func (s *recordingIndexSource) fetch(_ context.Context, wantLive bool) (*registry.Index, registry.Client, error) {
	effectivePath := registry.EffectiveIndexPath(resolveLibraryIndexPath())
	_, useCache := decideCachedIndex(effectivePath, wantLive)
	s.live = append(s.live, !useCache)
	if s.fetchErr != nil {
		return nil, nil, s.fetchErr
	}
	return s.index, nil, nil
}

// newRecordingResolver builds a resolver wired to a recording index source that
// returns idx and records the live-vs-cache-gated decision each acquisition
// took. ensureIndex runs its full memoization path and delegates to the source,
// so tests assert liveness on src.live. The driver's up-front union is not run
// here; tests set r.wantLive (or call computeWantLive) to simulate the decision
// under test.
func newRecordingResolver(cfg internalcue.LoadResult, idx *registry.Index) (*resolver, *recordingIndexSource) {
	src := &recordingIndexSource{index: idx}
	r := newResolver(cfg, &Flags{}, io.Discard, io.Discard, strings.NewReader(""))
	r.indexSrc = src
	return r, src
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

// --- Resolver-path liveness (observed via the recording index source) ---

func TestEnsureIndex_CacheGatedFetchesCanonical(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	r, src := newRecordingResolver(buildTestCfg(t, `{}`), stubLibraryIndex())
	r.wantLive = false

	idx, _, err := r.ensureIndex()
	if err != nil || idx == nil {
		t.Fatalf("ensureIndex() = (%v, %v), want index", idx, err)
	}
	if len(src.live) != 1 || src.live[0] {
		t.Errorf("cache-gated resolve should record a cache-gated acquisition, got %v", src.live)
	}
}

func TestEnsureIndex_LiveFetchesBareMajor(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	r, src := newRecordingResolver(buildTestCfg(t, `{}`), stubLibraryIndex())
	r.wantLive = true

	if _, _, err := r.ensureIndex(); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}
	if len(src.live) != 1 || !src.live[0] {
		t.Errorf("live resolve should record a live acquisition, got %v", src.live)
	}
}

func TestEnsureIndex_LiveResolveErrorDegrades(t *testing.T) {
	isolateConfigEnv(t) // empty cache → live regardless
	r, src := newRecordingResolver(buildTestCfg(t, `{}`), stubLibraryIndex())
	src.fetchErr = fmt.Errorf("registry down")
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
	r, src := newRecordingResolver(buildTestCfg(t, `{}`), stubLibraryIndex())

	r.wantLive = false
	if _, _, err := r.ensureIndex(); err != nil { // cache-gated
		t.Fatalf("first ensureIndex: %v", err)
	}
	r.forceLiveReResolve()
	if _, _, err := r.ensureIndex(); err != nil { // live
		t.Fatalf("re-resolve ensureIndex: %v", err)
	}

	if len(src.live) != 2 {
		t.Fatalf("expected two acquisitions, got %v", src.live)
	}
	if src.live[0] {
		t.Errorf("first acquisition should be cache-gated, got live")
	}
	if !src.live[1] {
		t.Errorf("forced re-resolve should be live, got cache-gated")
	}
}

// TestResolveExactInstalled_NoRegistryCall asserts a category-specific lone
// installed exact match resolves without touching the index at all (it
// short-circuits before ensureIndex), so the index source sees no acquisition.
func TestResolveExactInstalled_NoRegistryCall(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	cfg := buildTestCfg(t, `{roles: {"go-expert": {prompt: "Go"}}}`)
	r, src := newRecordingResolver(cfg, stubLibraryIndex())
	r.wantLive = r.computeWantLive([]pendingSurface{{"go-expert", roleScope()}})

	name, err := r.resolveRole("go-expert")
	if err != nil {
		t.Fatalf("resolveRole(go-expert) error = %v", err)
	}
	if name != "go-expert" {
		t.Errorf("resolved %q, want go-expert", name)
	}
	if len(src.live) != 0 {
		t.Errorf("an installed exact match should make no registry call, got %v", src.live)
	}
}

// --- resolveCross liveness ---

func TestResolveCross_UninstalledResolvesLive(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	r, src := newRecordingResolver(buildTestCfg(t, `{}`), &registry.Index{})

	// Not installed and absent from the index: not-found, but the path taken to
	// confirm that must be live so a freshly published module would be seen.
	if _, err := r.resolveCross("missing-module"); err == nil {
		t.Fatal("expected not-found for an uninstalled, absent query")
	}
	if len(src.live) == 0 || !src.live[0] {
		t.Errorf("uninstalled get/describe query should resolve live, got %v", src.live)
	}
}

func TestResolveCross_InstalledStaysCacheGated(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	cfg := buildTestCfg(t, `{agents: {claude: {bin: "claude", command: "{{.bin}} run"}}}`)
	r, src := newRecordingResolver(cfg, &registry.Index{})

	outcome, err := r.resolveCross("claude")
	if err != nil {
		t.Fatalf("resolveCross(claude) error = %v", err)
	}
	if outcome.match.Name != "claude" {
		t.Errorf("resolved %q, want claude", outcome.match.Name)
	}
	if len(src.live) != 1 || src.live[0] {
		t.Errorf("installed get/describe query should stay cache-gated, got %v", src.live)
	}
}

func TestResolveCross_RefreshForcesLiveForInstalled(t *testing.T) {
	isolateConfigEnv(t)
	seedFreshIndexCache(t)
	cfg := buildTestCfg(t, `{agents: {claude: {bin: "claude", command: "{{.bin}} run"}}}`)
	r, src := newRecordingResolver(cfg, &registry.Index{})
	r.flags.Refresh = true

	if _, err := r.resolveCross("claude"); err != nil {
		t.Fatalf("resolveCross(claude) error = %v", err)
	}
	if len(src.live) == 0 || !src.live[0] {
		t.Errorf("--refresh should force a live resolve even for an installed query, got %v", src.live)
	}
}

// --- Substring-shadow boundary (liveness observable) ---

// TestSubstringShadow_CacheGatedThenRefreshLive covers the accepted limitation:
// a substring-installed surface (--role expert with go-expert installed) stays
// cache-gated under a fresh cache and resolves to the installed go-expert with
// no live resolve, while --refresh flips it live.
func TestSubstringShadow_CacheGatedThenRefreshLive(t *testing.T) {
	cfg := buildTestCfg(t, `{roles: {"go-expert": {prompt: "Go"}}}`)

	t.Run("cache-gated", func(t *testing.T) {
		isolateConfigEnv(t)
		seedFreshIndexCache(t)
		r, src := newRecordingResolver(cfg, &registry.Index{})
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
		for _, live := range src.live {
			if live {
				t.Error("substring-installed surface should stay cache-gated, saw live acquisition")
			}
		}
	})

	t.Run("refresh-live", func(t *testing.T) {
		isolateConfigEnv(t)
		seedFreshIndexCache(t)
		r, src := newRecordingResolver(cfg, &registry.Index{})
		r.flags.Refresh = true
		r.wantLive = r.computeWantLive([]pendingSurface{{"expert", singleCategoryScope("roles", "role", true)}})

		if _, err := r.resolveRole("expert"); err != nil {
			t.Fatalf("resolveRole(expert) error = %v", err)
		}
		if len(src.live) == 0 || !src.live[0] {
			t.Errorf("--refresh should flip the substring-installed surface live, got %v", src.live)
		}
	})
}
