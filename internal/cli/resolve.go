package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
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
	Score    int
}

// contextScoreThreshold is the minimum match score for context inclusion.
const contextScoreThreshold = 2

// maxModuleResults is the maximum number of results to display in interactive selection.
const maxModuleResults = 20

// resolver performs two-phase resolution for module-selecting flags.
// It lazily fetches the registry index and tracks whether any installs occurred.
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
	skipRegistry bool // skip registry fetch (for testing)
}

func newResolver(cfg internalcue.LoadResult, flags *Flags, stdout, stderr io.Writer, stdin io.Reader) *resolver {
	return &resolver{
		cfg:    cfg,
		flags:  flags,
		stderr: stderr,
		stdout: stdout,
		stdin:  stdin,
	}
}

func (r *resolver) resolveAgent(name string) (string, error) {
	return r.resolveModule(name, internalcue.KeyAgents, "agents", "Agent", false)
}

func (r *resolver) resolveRole(name string) (string, error) {
	return r.resolveModule(name, internalcue.KeyRoles, "roles", "Role", true)
}

// resolveModule resolves a module in two phases: exact installed-config match
// (no registry needed), else merge installed + registry candidates and select.
// A "category:name" prefix matching category is stripped; a mismatched prefix
// errors. allowFilePath lets file paths bypass resolution.
func (r *resolver) resolveModule(name, cueKey, category, displayType string, allowFilePath bool) (string, error) {
	if name == "" {
		return "", nil
	}

	if allowFilePath && orchestration.IsFilePath(name) {
		debugf(r.stderr, r.flags, dbgResolve, "%s %q: file path bypass", displayType, name)
		return name, nil
	}

	addr, err := parseAddress(name)
	if err != nil {
		return "", err
	}
	if addr.HasPrefix && addr.Category != category {
		return "", usageError(fmt.Errorf("%s expects category %q, got %q in %q", displayType, category, addr.Category, name))
	}
	name = addr.Name

	searchType := strings.ToLower(displayType)

	// Phase 1: exact installed match is unambiguous, no registry needed.
	if isExactInstalledKey(r.cfg.Value, cueKey, name) {
		debugf(r.stderr, r.flags, dbgResolve, "%s %q: exact installed match", displayType, name)
		return name, nil
	}

	// Phase 2: merge installed + registry candidates, then select.
	installedMatches, err := searchInstalled(r.cfg.Value, cueKey, category, name)
	if err != nil {
		return "", err
	}

	if len(installedMatches) == 0 && !r.flags.Quiet {
		fmt.Fprintf(r.stdout, "%s %q not found in configuration\n", displayType, name)
	}

	// ensureIndex returns nil error; failures land in r.indexErr (graceful fallback).
	index, client, _ := r.ensureIndex()
	var registryMatches []ModuleMatch
	if index != nil {
		entries := registryEntries(index, category)
		registryMatches, err = searchRegistryCategory(entries, category, name)
		if err != nil {
			return "", err
		}
	}

	allMatches := mergeModuleMatches(installedMatches, registryMatches)
	debugf(r.stderr, r.flags, dbgResolve, "%s %q: %d installed, %d registry, %d total matches",
		displayType, name, len(installedMatches), len(registryMatches), len(allMatches))

	selected, err := r.selectSingleMatch(allMatches, searchType, name)
	if err != nil {
		return "", err
	}

	if selected.Source == ModuleSourceRegistry {
		if err := r.autoInstall(client, modules.SearchResult{
			Category: selected.Category,
			Name:     selected.Name,
			Entry:    selected.Entry,
		}); err != nil {
			return "", err
		}
	}

	return selected.Name, nil
}

// isExactInstalledKey returns true if name is a verbatim key in the config category.
func isExactInstalledKey(cfg cue.Value, cueKey, name string) bool {
	catVal := cfg.LookupPath(cue.ParsePath(cueKey))
	if !catVal.Exists() {
		return false
	}
	return catVal.LookupPath(cue.MakePath(cue.Str(name))).Exists()
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
// then multi-term AND substring match, then passthrough.
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

// resolveContexts resolves context flag values per-term: file path bypass ->
// "default" passthrough -> exact name -> search (all above threshold).
func (r *resolver) resolveContexts(terms []string) ([]string, error) {
	if len(terms) == 0 {
		return nil, nil
	}

	var resolved []string
	for _, term := range terms {
		if orchestration.IsFilePath(term) {
			debugf(r.stderr, r.flags, dbgResolve, "Context %q: file path bypass", term)
			resolved = append(resolved, term)
			continue
		}

		if term == "default" {
			debugf(r.stderr, r.flags, dbgResolve, "Context %q: default passthrough", term)
			resolved = append(resolved, term)
			continue
		}

		addr, err := parseAddress(term)
		if err != nil {
			return nil, err
		}
		if addr.HasPrefix && addr.Category != "contexts" {
			return nil, usageError(fmt.Errorf("context expects category %q, got %q in %q", "contexts", addr.Category, term))
		}
		term = addr.Name

		if resolvedCtx, err := findExactInstalledName(r.cfg.Value, internalcue.KeyContexts, term); err != nil {
			// Ambiguous short name: fall through to substring search.
			debugf(r.stderr, r.flags, dbgResolve, "Context %q: short name ambiguous, falling through: %v", term, err)
		} else if resolvedCtx != "" {
			debugf(r.stderr, r.flags, dbgResolve, "Context %q: installed match -> %q", term, resolvedCtx)
			resolved = append(resolved, resolvedCtx)
			continue
		}

		installedMatches, err := searchInstalled(r.cfg.Value, internalcue.KeyContexts, "contexts", term)
		if err != nil {
			// Invalid regex: pass through as-is.
			debugf(r.stderr, r.flags, dbgResolve, "Context %q: invalid pattern, passing through", term)
			resolved = append(resolved, term)
			continue
		}
		hasInstalledMatches := len(installedMatches) > 0

		if !hasInstalledMatches {
			if !r.flags.Quiet {
				fmt.Fprintf(r.stdout, "Context %q not found in configuration\n", term)
			}
		}

		// Exact registry match only when no installed matches.
		index, client, indexErr := r.ensureIndex()
		if indexErr != nil {
			debugf(r.stderr, r.flags, dbgResolve, "Registry unavailable for context search: %v", indexErr)
		}
		if !hasInstalledMatches && index != nil {
			result, err := findExactInRegistry(index.Contexts, "contexts", term)
			if err != nil {
				if !r.flags.Quiet {
					printWarning(r.stdout, "%s", err)
				}
				resolved = append(resolved, term)
				continue
			}
			if result != nil {
				debugf(r.stderr, r.flags, dbgResolve, "Context %q: exact registry match %q", term, result.Name)
				if err := r.autoInstall(client, *result); err != nil {
					if !r.flags.Quiet {
						printWarning(r.stdout, "context %q: auto-install failed: %s", term, err)
					}
				} else {
					resolved = append(resolved, result.Name)
					continue
				}
			}
		}

		// Combined search across installed + registry (matches above threshold).
		var registryMatches []ModuleMatch
		if index != nil {
			registryMatches, err = searchRegistryCategory(index.Contexts, "contexts", term)
			if err != nil {
				debugf(r.stderr, r.flags, dbgResolve, "Context %q: invalid pattern, passing through", term)
				resolved = append(resolved, term)
				continue
			}
		}
		allMatches := mergeModuleMatches(installedMatches, registryMatches)

		var qualified []ModuleMatch
		for _, m := range allMatches {
			if m.Score >= contextScoreThreshold {
				qualified = append(qualified, m)
			}
		}

		debugf(r.stderr, r.flags, dbgResolve, "Context %q: %d matches above threshold", term, len(qualified))

		if len(qualified) == 0 {
			// Composer will warn on the unresolved term.
			debugf(r.stderr, r.flags, dbgResolve, "Context %q: no matches, passing through", term)
			resolved = append(resolved, term)
			continue
		}

		for _, m := range qualified {
			if m.Source == ModuleSourceRegistry && client != nil {
				if err := r.autoInstall(client, modules.SearchResult{
					Category: m.Category,
					Name:     m.Name,
					Entry:    m.Entry,
				}); err != nil {
					if !r.flags.Quiet {
						printWarning(r.stdout, "context %q: auto-install failed: %s", m.Name, err)
					}
					continue
				}
			}
			resolved = append(resolved, m.Name)
		}
	}

	return resolved, nil
}

// findExactInstalledName matches a module by full or short name in installed
// config, returning the full name (empty if none) or an error if ambiguous.
func findExactInstalledName(cfg cue.Value, cueKey, name string) (string, error) {
	catVal := cfg.LookupPath(cue.ParsePath(cueKey))
	if !catVal.Exists() {
		return "", nil
	}

	// Full name match is always unambiguous.
	if catVal.LookupPath(cue.MakePath(cue.Str(name))).Exists() {
		return name, nil
	}

	// Short name: collect all matches to detect ambiguity.
	iter, err := catVal.Fields()
	if err != nil {
		return "", nil
	}
	var matches []string
	for iter.Next() {
		entryName := iter.Selector().Unquoted()
		if idx := strings.LastIndex(entryName, "/"); idx != -1 {
			if entryName[idx+1:] == name {
				matches = append(matches, entryName)
			}
		}
	}

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	default:
		sort.Strings(matches)
		return "", fmt.Errorf("ambiguous %s name %q matches multiple entries: %s",
			cueKey, name, strings.Join(matches, ", "))
	}
}

// findExactInRegistry matches a registry entry by full or short name, erroring
// if multiple entries share the same short name.
func findExactInRegistry(entries map[string]registry.IndexEntry, category, name string) (*modules.SearchResult, error) {
	// Full name match is always unambiguous.
	if entry, ok := entries[name]; ok {
		return &modules.SearchResult{
			Category: category,
			Name:     name,
			Entry:    entry,
		}, nil
	}

	// Short name: collect all matches to detect ambiguity.
	var matches []string
	for entryName := range entries {
		if idx := strings.LastIndex(entryName, "/"); idx != -1 {
			if entryName[idx+1:] == name {
				matches = append(matches, entryName)
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, nil
	case 1:
		return &modules.SearchResult{
			Category: category,
			Name:     matches[0],
			Entry:    entries[matches[0]],
		}, nil
	default:
		sort.Strings(matches)
		return nil, fmt.Errorf("ambiguous %s name %q matches multiple entries: %s", category, name, strings.Join(matches, ", "))
	}
}

func searchInstalled(cfg cue.Value, cueKey, category, query string) ([]ModuleMatch, error) {
	results, err := modules.SearchInstalledConfig(cfg, cueKey, category, query, nil)
	if err != nil {
		return nil, err
	}
	var matches []ModuleMatch
	for _, r := range results {
		matches = append(matches, ModuleMatch{
			Name:     r.Name,
			Category: r.Category,
			Source:   ModuleSourceInstalled,
			Entry:    r.Entry,
			Score:    r.MatchScore,
		})
	}
	return matches, nil
}

func searchRegistryCategory(entries map[string]registry.IndexEntry, category, query string) ([]ModuleMatch, error) {
	results, err := modules.SearchCategoryEntries(category, entries, query, nil)
	if err != nil {
		return nil, err
	}
	var matches []ModuleMatch
	for _, r := range results {
		matches = append(matches, ModuleMatch{
			Name:     r.Name,
			Category: r.Category,
			Source:   ModuleSourceRegistry,
			Entry:    r.Entry,
			Score:    r.MatchScore,
		})
	}
	return matches, nil
}

// mergeModuleMatches dedupes by name (installed wins) and sorts by score
// descending, then name.
func mergeModuleMatches(installed, reg []ModuleMatch) []ModuleMatch {
	seen := make(map[string]bool)
	var merged []ModuleMatch

	for _, m := range installed {
		seen[m.Name] = true
		merged = append(merged, m)
	}

	for _, m := range reg {
		if !seen[m.Name] {
			merged = append(merged, m)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		return merged[i].Name < merged[j].Name
	})

	return merged
}

// selectSingleMatch auto-selects one match, prompts on multiple (TTY), errors
// on multiple (non-TTY), errors on zero.
func (r *resolver) selectSingleMatch(matches []ModuleMatch, categoryType, query string) (ModuleMatch, error) {
	switch len(matches) {
	case 0:
		return ModuleMatch{}, notFoundError(fmt.Errorf("%s %q not found", categoryType, query))
	case 1:
		return matches[0], nil
	default:
		return r.promptModuleSelection(matches, categoryType, query)
	}
}

// promptModuleSelection prompts to select from multiple matches; in non-TTY
// mode it returns an error listing the matches.
func (r *resolver) promptModuleSelection(matches []ModuleMatch, categoryType, query string) (ModuleMatch, error) {
	isTTY := isTerminal(r.stdin)

	if !isTTY {
		var names []string
		for _, m := range matches {
			names = append(names, m.Name)
		}
		return ModuleMatch{}, usageError(fmt.Errorf("ambiguous %s %q matches: %s\nSpecify exact name or run interactively",
			categoryType, query, strings.Join(names, ", ")))
	}

	displayCount := len(matches)
	truncated := false
	if displayCount > maxModuleResults {
		displayCount = maxModuleResults
		truncated = true
	}

	fmt.Fprintf(r.stdout, "Found %d %ss matching %q:\n\n", len(matches), categoryType, query)

	maxNameLen := 0
	for i := 0; i < displayCount; i++ {
		if len(matches[i].Name) > maxNameLen {
			maxNameLen = len(matches[i].Name)
		}
	}

	for i := 0; i < displayCount; i++ {
		m := matches[i]
		padding := strings.Repeat(" ", maxNameLen-len(m.Name)+2)
		var sourceLabel string
		if m.Source == ModuleSourceInstalled {
			sourceLabel = tui.ColorInstalled.Sprint(m.Source)
		} else {
			sourceLabel = tui.ColorRegistry.Sprint(m.Source)
		}
		fmt.Fprintf(r.stdout, "  %2d. %s%s%s\n", i+1, m.Name, padding, sourceLabel)
	}

	if truncated {
		fmt.Fprintf(r.stdout, "\nShowing %d of %d matches. Refine search for more specific results.\n",
			displayCount, len(matches))
	}

	fmt.Fprintln(r.stdout)
	fmt.Fprintf(r.stdout, "Select %s: ", tui.Annotate("1-%d", displayCount))

	reader := bufio.NewReader(r.stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ModuleMatch{}, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= displayCount {
			fmt.Fprintln(r.stdout)
			return matches[choice-1], nil
		}
		return ModuleMatch{}, fmt.Errorf("invalid selection: %s (choose 1-%d)", input, displayCount)
	}

	inputLower := strings.ToLower(input)
	for i := 0; i < displayCount; i++ {
		if strings.ToLower(matches[i].Name) == inputLower {
			fmt.Fprintln(r.stdout)
			return matches[i], nil
		}
	}

	var subMatches []ModuleMatch
	for i := 0; i < displayCount; i++ {
		if strings.Contains(strings.ToLower(matches[i].Name), inputLower) {
			subMatches = append(subMatches, matches[i])
		}
	}
	if len(subMatches) == 1 {
		fmt.Fprintln(r.stdout)
		return subMatches[0], nil
	}

	return ModuleMatch{}, fmt.Errorf("invalid selection: %s", input)
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
	return nil
}

// ensureIndex lazily fetches the registry index, returning a nil index with nil
// error when the registry is unavailable (graceful fallback). A fresh cache
// (< 24h) supplies a canonical version that lets FetchIndex serve from CUE's
// module cache without a network call; a stale or missing cache triggers a full
// fetch and cache update.
func (r *resolver) ensureIndex() (*registry.Index, registry.Client, error) {
	if r.skipRegistry {
		return nil, nil, nil
	}

	if r.didFetch {
		return r.index, r.client, r.indexErr
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
	return nil
}
