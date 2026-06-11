package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/registry"
)

// ModuleSource indicates where a module was found.
type ModuleSource string

const (
	ModuleSourceInstalled ModuleSource = "installed"
	ModuleSourceRegistry  ModuleSource = "registry"
)

// ModuleMatch represents a single matched module during resolution.
type ModuleMatch struct {
	Name     string
	Category string
	Source   ModuleSource
	Entry    registry.IndexEntry
}

// maxModuleResults is the maximum number of results to display in interactive selection.
const maxModuleResults = 20

// offlineRegistryForTests makes every newResolver skip the registry fetch. The
// cross-category surfaces (get, describe) now consult the registry through a
// resolver they build internally, which the provider seam cannot stub, so the
// cli test binary sets this in TestMain to keep content and scope tests offline.
// Production never sets it; resolvers that inject an index (didFetch) are
// honoured regardless, since ensureIndex checks didFetch first.
var offlineRegistryForTests bool

// resolver performs name-only resolution for module-selecting surfaces. It
// lazily fetches the registry index and tracks whether any installs occurred.
//
// Two install-tracking fields with distinct lifetimes: didInstall is sticky for
// the resolver's life (an install happened at all — drives the --local scope-
// widening notice), while cfgStale flips true on each install and clears on each
// reloadConfig (r.cfg no longer matches disk — drives reload decisions). A
// surface that installs across multiple stages (task: flags → task → task-role)
// must gate every reload on cfgStale, never on the sticky didInstall, or a later
// stage's install is missed once an earlier one already set didInstall.
type resolver struct {
	cfg          internalcue.LoadResult
	flags        *Flags
	stderr       io.Writer
	stdout       io.Writer
	stdin        io.Reader
	index        *registry.Index
	client       registry.Client
	indexErr     error
	didFetch     bool
	didInstall   bool
	cfgStale     bool
	skipRegistry bool // skip registry fetch (for testing)
}

func newResolver(cfg internalcue.LoadResult, flags *Flags, stdout, stderr io.Writer, stdin io.Reader) *resolver {
	return &resolver{
		cfg:          cfg,
		flags:        flags,
		stderr:       stderr,
		stdout:       stdout,
		stdin:        stdin,
		skipRegistry: offlineRegistryForTests,
	}
}

// resolveAgent resolves the --agent identifier to an installed agent name. An
// agent is a structured configuration, not a document body, so a filesystem
// path is rejected.
func (r *resolver) resolveAgent(name string) (string, error) {
	return r.resolveSingle(name, singleCategoryScope("agents", "agent", false))
}

// resolveRole resolves the --role identifier to an installed role name or a
// filesystem path.
func (r *resolver) resolveRole(name string) (string, error) {
	return r.resolveSingle(name, singleCategoryScope("roles", "role", true))
}

// resolveSingle resolves a category-specific identifier, returning either the
// resolved module name or the filesystem path it bypassed to. An empty
// identifier passes through unchanged (the caller's "use the default" signal).
func (r *resolver) resolveSingle(name string, scope resolveScope) (string, error) {
	if name == "" {
		return "", nil
	}
	outcome, err := r.resolve(name, scope)
	if err != nil {
		return "", err
	}
	if outcome.filePath != "" {
		return outcome.filePath, nil
	}
	return outcome.match.Name, nil
}

func registryEntries(index *registry.Index, category string) map[string]registry.IndexEntry {
	switch category {
	case "agents":
		return index.Agents
	case "roles":
		return index.Roles
	case "contexts":
		return index.Contexts
	case "tasks":
		return index.Tasks
	default:
		return nil
	}
}

// resolveModelName resolves a model name against agent.Models: exact match,
// then multi-term AND substring match, then passthrough. Model resolution is
// deliberately outside the unified module match rule — its target is an agent's
// model map, not the module sources — so it keeps the search-style match.
func (r *resolver) resolveModelName(name string, agent orchestration.Agent) string {
	if name == "" {
		return ""
	}

	if _, ok := agent.Models[name]; ok {
		debugf(r.stderr, r.flags, dbgResolve, "Model %q: exact match in models map", name)
		return name
	}

	terms := modules.ParseSearchTerms(name)
	if len(terms) == 0 {
		return name
	}

	var matches []string
	for key := range agent.Models {
		keyLower := strings.ToLower(key)
		allMatch := true
		for _, term := range terms {
			if !strings.Contains(keyLower, term) {
				allMatch = false
				break
			}
		}
		if allMatch {
			matches = append(matches, key)
		}
	}

	sort.Strings(matches) // deterministic ordering

	if len(matches) == 1 {
		debugf(r.stderr, r.flags, dbgResolve, "Model %q: match %q", name, matches[0])
		return matches[0]
	}

	if len(matches) > 1 {
		debugf(r.stderr, r.flags, dbgResolve, "Model %q: multiple matches %v, using passthrough", name, matches)
	}

	debugf(r.stderr, r.flags, dbgResolve, "Model %q: passthrough", name)
	return name
}

// resolveContexts resolves each --context term independently through the unified
// match rule. A filesystem path is read directly; the "default" sentinel passes
// through unsearched ("none" is consumed upstream and never reaches here). Every
// other term resolves to exactly one context, erroring when it matches nothing.
func (r *resolver) resolveContexts(terms []string) ([]string, error) {
	if len(terms) == 0 {
		return nil, nil
	}

	scope := singleCategoryScope("contexts", "context", true)

	var resolved []string
	for _, term := range terms {
		if term == "default" {
			debugf(r.stderr, r.flags, dbgResolve, "Context %q: default passthrough", term)
			resolved = append(resolved, term)
			continue
		}

		outcome, err := r.resolve(term, scope)
		if err != nil {
			return nil, err
		}
		if outcome.filePath != "" {
			resolved = append(resolved, outcome.filePath)
			continue
		}
		resolved = append(resolved, outcome.match.Name)
	}

	return resolved, nil
}

func (r *resolver) autoInstall(client registry.Client, result modules.SearchResult) error {
	if client == nil {
		return fmt.Errorf("registry client unavailable")
	}

	ctx := context.Background()

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	debugf(r.stderr, r.flags, dbgResolve, "Auto-installing %s from registry", formatAddress(result.Category, result.Name))

	if !r.flags.Quiet {
		fmt.Fprintf(r.stdout, "Installing %s from registry...\n", result.Name)
	}

	version, err := modules.InstallModule(ctx, client, r.index, result, paths.Global)
	if err != nil {
		return err
	}

	if !r.flags.Quiet {
		if version != "" {
			fmt.Fprintf(r.stdout, "Installed %s@%s to global config\n\n", result.Name, version)
		} else {
			fmt.Fprintf(r.stdout, "Installed %s to global config\n\n", result.Name)
		}
	}

	debugf(r.stderr, r.flags, dbgResolve, "Auto-installed %s", formatAddress(result.Category, result.Name))
	r.didInstall = true
	r.cfgStale = true
	return nil
}

// ensureIndex lazily fetches the registry index, returning a nil index with nil
// error when the registry is unavailable (graceful fallback); the underlying
// failure is recorded in r.indexErr so callers can apply the certainty split. A
// fresh cache (< 24h) supplies a canonical version that lets FetchIndex serve
// from CUE's module cache without a network call; a stale or missing cache
// triggers a full fetch and cache update.
func (r *resolver) ensureIndex() (*registry.Index, registry.Client, error) {
	// An injected index (didFetch set by a test) is honoured even under
	// skipRegistry, so resolvers that pre-load an index resolve against it.
	if r.didFetch {
		return r.index, r.client, r.indexErr
	}

	if r.skipRegistry {
		return nil, nil, nil
	}
	r.didFetch = true

	// Use the cache only when it belongs to the same module as the configured index.
	indexPath := resolveLibraryIndexPath()
	effectivePath := registry.EffectiveIndexPath(indexPath)
	usedCache := false
	cached, cacheErr := cache.ReadIndex()
	if cacheErr == nil && cached.IsFresh(cache.DefaultMaxAge) &&
		modules.ModuleFromOrigin(cached.Version) == modules.ModuleFromOrigin(effectivePath) {
		debugf(r.stderr, r.flags, dbgResolve, "Using cached index version: %s", cached.Version)
		indexPath = cached.Version
		usedCache = true
	} else {
		if !r.flags.Quiet {
			fmt.Fprintf(r.stdout, "Fetching registry index...\n")
		}
	}

	client, err := registry.NewClient()
	if err != nil {
		debugf(r.stderr, r.flags, dbgResolve, "Registry unavailable: %v", err)
		r.indexErr = err
		return nil, nil, nil // graceful fallback
	}
	r.client = client

	const fetchTimeout = 60 * time.Second
	const slowWarning = 10 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	if !r.flags.Quiet {
		go func() {
			select {
			case <-time.After(slowWarning):
				remaining := fetchTimeout - slowWarning
				printWarning(r.stdout, "registry is taking longer than expected, timeout in %d seconds", int(remaining.Seconds()))
			case <-ctx.Done():
			}
		}()
	}

	index, indexVersion, err := client.FetchIndex(ctx, indexPath)
	if err != nil {
		debugf(r.stderr, r.flags, dbgResolve, "Index fetch failed: %v", err)
		r.indexErr = err
		return nil, client, nil // graceful fallback
	}
	if !usedCache {
		if err := cache.WriteIndex(indexVersion); err != nil {
			debugf(r.stderr, r.flags, dbgResolve, "Cache write failed: %v", err)
		}
	}

	r.index = index
	debugf(r.stderr, r.flags, dbgResolve, "Index fetched: version %s", indexVersion)
	return index, client, nil
}

// resolveLibraryIndexPath returns the library_index setting (empty on unset or
// error); pass the result to registry.EffectiveIndexPath for the module path.
func resolveLibraryIndexPath() string {
	settings, err := loadSettingsForScope(config.ScopeMerged)
	if err != nil {
		return ""
	}
	return settings["library_index"]
}

func (r *resolver) reloadConfig(workingDir string) error {
	cfg, err := loadMergedConfigFromDirWithDebug(r.stdout, r.stderr, r.stdin, workingDir, r.flags)
	if err != nil {
		return fmt.Errorf("reloading configuration: %w", err)
	}
	r.cfg = cfg
	r.cfgStale = false
	return nil
}
