package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"cuelang.org/go/mod/modconfig"
	"github.com/start-cli/start/internal/registry"
)

// registryStub is an offline registry.Client for tests. It serves a canned
// in-memory index for FetchIndex consumers (search, update) and a fixture
// SourceDir for Fetch consumers (library's load-from-disk path). Lookups
// match on the module base path (version suffix stripped) so callers that
// resolve a path before fetching still hit the same entry.
//
// The stub is keyed by base path with a default, so per-test overrides via
// SetFetch/SetResolve do not require rebuilding it.
type registryStub struct {
	idx       *registry.Index
	sourceDir string
	fetches   map[string]fetchResponse
	resolves  map[string]resolveResponse
	versions  map[string]versionsResponse

	// fetchIndexErr, when set, makes FetchIndex fail so the registry-backed
	// commands' graceful-degradation paths can be exercised offline.
	fetchIndexErr error

	// providerCalls counts how many times captureJSON's provider closure
	// handed out this stub. A test asserting this is non-zero proves the seam
	// was consulted rather than silently falling back to the live registry,
	// and asserting the exact count pins each command's client-construction
	// count (e.g. doctor builds two per invocation).
	providerCalls int
}

type fetchResponse struct {
	result registry.FetchResult
	err    error
}

type resolveResponse struct {
	version string
	err     error
}

type versionsResponse struct {
	versions []string
	err      error
}

// stubVersion is the canonical version the stub resolves index/schema paths to.
const stubVersion = "v1.0.0"

// newRegistryStub builds a stub serving idx for FetchIndex consumers and
// sourceDir (an on-disk index CUE fixture) for library's Fetch+LoadIndex path.
// The in-memory index and the on-disk fixture must carry the same entries so
// the documented shapes agree; setupStartTestConfigWithRegistry keeps them in
// lockstep.
func newRegistryStub(idx *registry.Index, sourceDir string) *registryStub {
	return &registryStub{
		idx:       idx,
		sourceDir: sourceDir,
		fetches:   make(map[string]fetchResponse),
		resolves:  make(map[string]resolveResponse),
		versions:  make(map[string]versionsResponse),
	}
}

// SetFetch overrides the Fetch response for the given module base path.
func (s *registryStub) SetFetch(path string, result registry.FetchResult, err error) {
	s.fetches[stubBasePath(path)] = fetchResponse{result: result, err: err}
}

// SetResolve overrides the ResolveLatestVersion response for the given base path.
func (s *registryStub) SetResolve(path string, version string, err error) {
	s.resolves[stubBasePath(path)] = resolveResponse{version: version, err: err}
}

// SetFetchIndexError makes FetchIndex fail, driving search and update through
// their registry-unavailable degradation paths offline.
func (s *registryStub) SetFetchIndexError(err error) {
	s.fetchIndexErr = err
}

// FetchIndex returns the canned in-memory index. search and update consume the
// parsed *registry.Index directly through this method. The returned version
// string mirrors a canonical resolved path (what the real client produces).
func (s *registryStub) FetchIndex(ctx context.Context, indexPath string) (*registry.Index, string, error) {
	if s.fetchIndexErr != nil {
		return nil, "", s.fetchIndexErr
	}
	return s.idx, stubBasePath(registry.IndexModulePath) + "@" + stubVersion, nil
}

// Fetch serves the on-disk index fixture for the index module and an error for
// everything else (so doctor's schema-validation section falls through to its
// documented "Skipped" shape). Per-test overrides registered via SetFetch win.
func (s *registryStub) Fetch(ctx context.Context, modulePath string) (registry.FetchResult, error) {
	base := stubBasePath(modulePath)
	if resp, ok := s.fetches[base]; ok {
		return resp.result, resp.err
	}
	if base == stubBasePath(registry.IndexModulePath) {
		return registry.FetchResult{SourceDir: s.sourceDir}, nil
	}
	return registry.FetchResult{}, fmt.Errorf("registry stub: no fetch response for %q", modulePath)
}

// ModuleVersions returns a canned single version unless overridden. None of the
// four target commands' offline paths call this; it exists for interface
// completeness (doctor validate, which does call it, runs against the real
// registry and is carved out of the offline guard).
func (s *registryStub) ModuleVersions(ctx context.Context, modulePath string) ([]string, error) {
	if resp, ok := s.versions[stubBasePath(modulePath)]; ok {
		return resp.versions, resp.err
	}
	return []string{stubVersion}, nil
}

// ResolveLatestVersion returns a canonical version for any path so callers that
// resolve before fetching get a deterministic result without network access.
func (s *registryStub) ResolveLatestVersion(ctx context.Context, modulePath string) (string, error) {
	base := stubBasePath(modulePath)
	if resp, ok := s.resolves[base]; ok {
		return resp.version, resp.err
	}
	return base + "@" + stubVersion, nil
}

// Registry returns nil. LoadIndex accepts a nil registry for a self-contained
// index package, which the on-disk fixture is.
func (s *registryStub) Registry() modconfig.Registry {
	return nil
}

// TestRegistryStubOverrides verifies the stub's default responses and the
// SetFetch/SetResolve/ModuleVersions surfaces the parent project's drift-guard
// tests rely on. Matching is by module base path (version suffix stripped).
func TestRegistryStubOverrides(t *testing.T) {
	t.Parallel()
	stub := newRegistryStub(&registry.Index{}, t.TempDir())
	ctx := context.Background()

	// Default Fetch errors for unknown modules.
	if _, err := stub.Fetch(ctx, "github.com/x/y@v1"); err == nil {
		t.Error("default Fetch should error for an unknown module")
	}
	// SetFetch override is honoured, matching across version suffixes.
	stub.SetFetch("github.com/x/y@v1", registry.FetchResult{SourceDir: "/tmp/x"}, nil)
	res, err := stub.Fetch(ctx, "github.com/x/y@v2")
	if err != nil || res.SourceDir != "/tmp/x" {
		t.Errorf("SetFetch override not honoured: res=%+v err=%v", res, err)
	}

	// Default ResolveLatestVersion appends the canonical stub version.
	if got, _ := stub.ResolveLatestVersion(ctx, "github.com/x/y@v1"); got != "github.com/x/y@"+stubVersion {
		t.Errorf("default ResolveLatestVersion = %q, want base@%s", got, stubVersion)
	}
	// SetResolve override is honoured.
	stub.SetResolve("github.com/x/y@v1", "github.com/x/y@v9.9.9", nil)
	if got, _ := stub.ResolveLatestVersion(ctx, "github.com/x/y@v1"); got != "github.com/x/y@v9.9.9" {
		t.Errorf("SetResolve override not honoured: %q", got)
	}

	// Default ModuleVersions returns the single canned version.
	vers, err := stub.ModuleVersions(ctx, "github.com/x/y@v1")
	if err != nil || len(vers) != 1 || vers[0] != stubVersion {
		t.Errorf("default ModuleVersions = %v err=%v, want [%s]", vers, err, stubVersion)
	}
}

// stubBasePath strips the @version suffix from a module path.
func stubBasePath(modulePath string) string {
	if i := strings.LastIndex(modulePath, "@"); i >= 0 {
		return modulePath[:i]
	}
	return modulePath
}

// setupStartTestConfigWithRegistry performs the standard setupStartTestConfig
// isolation, writes an on-disk index CUE fixture derived from idx, and returns
// the stub wired to both the in-memory index and that fixture directory.
// Provider injection is captureJSON's responsibility, not this helper's.
func setupStartTestConfigWithRegistry(t *testing.T, idx *registry.Index) (tmpDir string, stub *registryStub) {
	t.Helper()
	tmpDir = setupStartTestConfig(t)
	// chdir so the written .start dir resolves as local config, matching how
	// command-level tests (e.g. TestExecuteStart_DryRun) consume this setup.
	chdir(t, tmpDir)

	fixtureDir := filepath.Join(tmpDir, "index-fixture")
	if err := os.MkdirAll(fixtureDir, 0o755); err != nil {
		t.Fatalf("creating index fixture dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "index.cue"), []byte(renderIndexCUE(idx)), 0o644); err != nil {
		t.Fatalf("writing index fixture: %v", err)
	}

	// Guard the lockstep invariant: the on-disk fixture (consumed by library)
	// and the in-memory index (consumed by search/update) must decode to the
	// same thing. This fails loudly if renderIndexCUE drifts from the fields of
	// registry.IndexEntry, rather than letting the two representations diverge
	// silently.
	loaded, err := registry.LoadIndex(fixtureDir, nil)
	if err != nil {
		t.Fatalf("loading index fixture: %v", err)
	}
	if !reflect.DeepEqual(loaded, idx) {
		t.Fatalf("on-disk index fixture diverged from in-memory index; renderIndexCUE is out of sync with registry.IndexEntry\nin-memory: %+v\non-disk:   %+v", idx, loaded)
	}

	return tmpDir, newRegistryStub(idx, fixtureDir)
}

// renderIndexCUE serialises an Index into the `package index` CUE form that
// registry.LoadIndex parses, keeping the on-disk fixture in lockstep with the
// in-memory index passed to the stub.
func renderIndexCUE(idx *registry.Index) string {
	var b strings.Builder
	b.WriteString("package index\n")
	renderIndexCategory(&b, "agents", idx.Agents)
	renderIndexCategory(&b, "roles", idx.Roles)
	renderIndexCategory(&b, "contexts", idx.Contexts)
	renderIndexCategory(&b, "tasks", idx.Tasks)
	return b.String()
}

func renderIndexCategory(b *strings.Builder, name string, entries map[string]registry.IndexEntry) {
	if len(entries) == 0 {
		return
	}
	names := make([]string, 0, len(entries))
	for n := range entries {
		names = append(names, n)
	}
	sort.Strings(names)

	fmt.Fprintf(b, "\n%s: {\n", name)
	for _, n := range names {
		e := entries[n]
		fmt.Fprintf(b, "\t%q: {\n", n)
		if e.Module != "" {
			fmt.Fprintf(b, "\t\tmodule:      %q\n", e.Module)
		}
		if e.Description != "" {
			fmt.Fprintf(b, "\t\tdescription: %q\n", e.Description)
		}
		if e.Bin != "" {
			fmt.Fprintf(b, "\t\tbin:         %q\n", e.Bin)
		}
		if e.Version != "" {
			fmt.Fprintf(b, "\t\tversion:     %q\n", e.Version)
		}
		if len(e.Tags) > 0 {
			quoted := make([]string, len(e.Tags))
			for i, tag := range e.Tags {
				quoted[i] = fmt.Sprintf("%q", tag)
			}
			fmt.Fprintf(b, "\t\ttags: [%s]\n", strings.Join(quoted, ", "))
		}
		b.WriteString("\t}\n")
	}
	b.WriteString("}\n")
}
