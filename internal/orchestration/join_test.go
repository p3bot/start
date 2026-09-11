package orchestration

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"cuelang.org/go/mod/modcache"
	"cuelang.org/go/mod/modregistrytest"
	"github.com/p3bot/agentdex"
	"github.com/p3bot/start/internal/fault"
)

func TestMatchLiveModel_IDsOnly(t *testing.T) {
	t.Parallel()
	models := []LiveModel{
		{ID: "claude-sonnet-4-5", CanonicalID: "anthropic/claude-sonnet-4-5"},
		{ID: "claude-haiku-4-5", CanonicalID: "anthropic/claude-haiku-4-5"},
	}

	if got := MatchLiveModel("claude-haiku-4-5", models); got != "claude-haiku-4-5" {
		t.Errorf("exact id = %q", got)
	}
	if got := MatchLiveModel("anthropic/claude-haiku-4-5", models); got != "claude-haiku-4-5" {
		t.Errorf("exact canonical = %q, want agent-facing id", got)
	}
	if got := MatchLiveModel("haiku", models); got != "claude-haiku-4-5" {
		t.Errorf("unique id substring = %q, want claude-haiku-4-5", got)
	}
	if got := MatchLiveModel("sonnet", models); got != "claude-sonnet-4-5" {
		t.Errorf("id+canonical of one model must unique-match, got %q", got)
	}
	if got := MatchLiveModel("Claude Sonnet", models); got != "Claude Sonnet" {
		t.Errorf("display name must passthrough, got %q", got)
	}
	if got := MatchLiveModel("claude", models); got != "claude" {
		t.Errorf("ambiguous id substring must passthrough, got %q", got)
	}
}

func TestMatchLiveModel_TwoGenerationsPassthrough(t *testing.T) {
	t.Parallel()
	models := []LiveModel{
		{ID: "claude-sonnet-4-5", CanonicalID: "anthropic/claude-sonnet-4-5"},
		{ID: "claude-sonnet-4-6", CanonicalID: "anthropic/claude-sonnet-4-6"},
	}
	if got := MatchLiveModel("sonnet", models); got != "sonnet" {
		t.Errorf("two sonnet generations must passthrough, got %q", got)
	}
}

func TestMapLaunchCatalogErr(t *testing.T) {
	t.Parallel()

	unavailable := mapLaunchCatalogErr(agentdex.ErrCatalogUnavailable)
	if unavailable == nil {
		t.Fatal("unavailable: nil")
	}
	if !errors.Is(unavailable, fault.ErrTransient) {
		t.Errorf("unavailable domain = %v, want ErrTransient", unavailable)
	}
	if !errors.Is(unavailable, agentdex.ErrCatalogUnavailable) {
		t.Error("unavailable should keep the agentdex cause")
	}
	if !strings.Contains(unavailable.Error(), "agent catalog unavailable") {
		t.Errorf("unavailable message = %v", unavailable)
	}
	if !strings.Contains(unavailable.Error(), "cannot resolve agentdex join") {
		t.Errorf("launch message = %v, want join wording", unavailable)
	}
	if strings.Contains(unavailable.Error(), "--refresh") {
		t.Errorf("unavailable must not name --refresh (join is Cached): %v", unavailable)
	}
	if !strings.Contains(unavailable.Error(), "start doctor") {
		t.Errorf("unavailable must name start doctor: %v", unavailable)
	}

	setup := mapSetupCatalogErr(agentdex.ErrCatalogUnavailable)
	if !strings.Contains(setup.Error(), "cannot detect catalogued CLI tools") {
		t.Errorf("setup message = %v, want detection wording", setup)
	}
	if strings.Contains(setup.Error(), "join") {
		t.Errorf("setup message must not say join: %v", setup)
	}
	if strings.Contains(setup.Error(), "--refresh") {
		t.Errorf("setup message must not name --refresh: %v", setup)
	}

	invalid := mapLaunchCatalogErr(agentdex.ErrCatalogInvalid)
	if invalid == nil {
		t.Fatal("invalid: nil")
	}
	if !errors.Is(invalid, fault.ErrUserConfig) {
		t.Errorf("invalid domain = %v, want ErrUserConfig", invalid)
	}
	if !errors.Is(invalid, agentdex.ErrCatalogInvalid) {
		t.Error("invalid should keep the agentdex cause")
	}
	if strings.Contains(invalid.Error(), "--refresh") {
		t.Errorf("invalid catalog must not suggest --refresh: %v", invalid)
	}

	if got := mapLaunchCatalogErr(nil); got != nil {
		t.Errorf("nil = %v, want nil", got)
	}
	plain := errors.New("other")
	if got := mapLaunchCatalogErr(plain); got != plain {
		t.Errorf("passthrough = %v, want same error", got)
	}
}

// Cold catalog, local registry up, no directory fixture: join's Cached fetch
// must resolve a bin. CacheOnly on the same cache would not (no ModuleVersions).
// CUE_REGISTRY is process-wide; this test must not be parallel.
func TestJoinAgent_ColdCatalogCachedFetches(t *testing.T) {
	startJoinCatalogRegistry(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	bin := filepath.Join(home, "bin-claude-code")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var modelsHits atomic.Int32
	models := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		modelsHits.Add(1)
	}))
	t.Cleanup(models.Close)

	base := func() []agentdex.Option {
		return []agentdex.Option{
			agentdex.WithCacheDir(t.TempDir()),
			agentdex.WithLookPath(func(string) (string, error) { return "", os.ErrNotExist }),
			agentdex.WithBinPaths(map[string]string{"claude-code": bin}),
			agentdex.WithEnvLookup(func(k string) (string, bool) {
				if k == "HOME" {
					return home, true
				}
				return "", false
			}),
			agentdex.WithWorkingDir(home),
			agentdex.WithModelsURL(models.URL),
		}
	}

	agent := Agent{Name: "claude-code/interactive", Agentdex: "claude-code", Command: "{{.bin}}"}

	got, err := JoinAgent(t.Context(), agent, home, base()...)
	if err != nil {
		t.Fatalf("Cached join on a cold catalog: %v", err)
	}
	if got.Bin != bin {
		t.Errorf("Bin = %q, want catalog path %q", got.Bin, bin)
	}

	_, err = JoinAgent(t.Context(), agent, home, append(base(), agentdex.WithCatalogFetch(agentdex.FetchCacheOnly))...)
	if err == nil {
		t.Fatal("CacheOnly on a cold catalog must fail; join's default Cached is what made the first call succeed")
	}
	if !errors.Is(err, agentdex.ErrCatalogUnavailable) {
		t.Errorf("CacheOnly error = %v, want ErrCatalogUnavailable", err)
	}
	if n := modelsHits.Load(); n != 0 {
		t.Errorf("models.dev HTTP hits = %d, want 0", n)
	}
}

func startJoinCatalogRegistry(t *testing.T) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	dir := filepath.Join(filepath.Dir(file), "..", "skills", "testdata", "catalog")
	const moduleDir = "github.com_p3bot_agentdex_catalog_v1.0.0"

	fsys := fstest.MapFS{}
	for _, rel := range []string{"cue.mod/module.cue", "schema.cue", "agents.cue"} {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("read catalog fixture %s: %v", rel, err)
		}
		fsys[path.Join(moduleDir, rel)] = &fstest.MapFile{Data: data}
	}
	reg, err := modregistrytest.New(fsys, "")
	if err != nil {
		t.Fatalf("start local catalog registry: %v", err)
	}
	var once sync.Once
	closeReg := func() { once.Do(reg.Close) }
	t.Cleanup(closeReg)

	t.Setenv("CUE_REGISTRY", reg.Host()+"+insecure")
	cueCache, err := os.MkdirTemp("", "start-join-cue-cache")
	if err != nil {
		t.Fatalf("create cue cache dir: %v", err)
	}
	t.Cleanup(func() { _ = modcache.RemoveAll(cueCache) })
	t.Setenv("CUE_CACHE_DIR", cueCache)
}
