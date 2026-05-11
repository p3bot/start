package cli

import (
	"bufio"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/tui"
)

// resolveCrossCategory resolves a module query across all categories via
// three-tier search (exact installed → substring installed → registry) with
// interactive selection on ambiguity. Auto-installs registry matches and
// sets r.didInstall = true when an install occurs.
//
// Writes:
//
//	r.stdout — registry fetch progress, install notices, interactive selection prompts
//	r.stderr — debug output only
//
// Callers needing clean stdout (e.g. `start get` piping content) should construct
// the resolver with stderr in the stdout slot: newResolver(cfg, flags, stderr, stderr, stdin).
//
// Post-call contract: if r.didInstall is true and the caller subsequently reads
// r.cfg.Value (e.g. to look up the resolved module's CUE value), the caller must
// first call r.reloadConfig(workingDir) — the installed module is written to disk
// but r.cfg is not refreshed in place. See runStart and runTask for the
// established reload-after-install pattern. describe is exempt because
// describeVerboseItem loads config independently via prepareDescribe.
func resolveCrossCategory(query string, r *resolver) (ModuleMatch, error) {
	addr, err := parseAddress(query)
	if err != nil {
		return ModuleMatch{}, err
	}

	// When the address carries a category prefix, every per-category loop
	// below scopes to that single category and the bare name is used for
	// matching. With no prefix, all four categories are searched.
	cats := describeCategories
	if addr.HasPrefix {
		if c := describeCategoryFor(addr.Category); c != nil {
			cats = []describeCategory{*c}
		}
	}
	name := addr.Name

	// Step 1: Exact match in installed config across the scoped categories.
	var exactMatches []ModuleMatch
	var ambiguousMatches []ModuleMatch
	for _, cat := range cats {
		resolved, err := findExactInstalledName(r.cfg.Value, cat.key, name)
		if err != nil {
			// Ambiguous short name within one category — collect all matches
			// for interactive selection instead of erroring out.
			matches, searchErr := searchInstalled(r.cfg.Value, cat.key, cat.category, name)
			if searchErr != nil {
				return ModuleMatch{}, searchErr
			}
			ambiguousMatches = append(ambiguousMatches, matches...)
			continue
		}
		if resolved != "" {
			exactMatches = append(exactMatches, ModuleMatch{
				Name:     resolved,
				Category: cat.category,
				Source:   ModuleSourceInstalled,
				Score:    100,
			})
		}
	}

	if len(ambiguousMatches) > 0 {
		// exactMatches and ambiguousMatches are installed-only by construction
		// (Step 1 only). No installIfRegistry call is needed; if a future change
		// mixes registry hits into either slice, restore the auto-install here.
		allMatches := make([]ModuleMatch, 0, len(exactMatches)+len(ambiguousMatches))
		allMatches = append(allMatches, exactMatches...)
		allMatches = append(allMatches, ambiguousMatches...)
		selected, err := promptCrossCategorySelection(r, allMatches, query)
		if err != nil {
			return ModuleMatch{}, err
		}
		return selected, nil
	}

	if len(exactMatches) > 1 {
		// exactMatches is installed-only by construction (Step 1 only). No
		// installIfRegistry call is needed; if a future change mixes registry
		// hits into this slice, restore the auto-install here.
		selected, err := promptCrossCategorySelection(r, exactMatches, query)
		if err != nil {
			return ModuleMatch{}, err
		}
		return selected, nil
	}

	// Step 2: Substring search in installed config. Computed once and reused
	// for the single-exact-match disambiguation gate below and for the
	// combined-search path further down. The exact match from Step 1, if any,
	// also appears here as a self-substring — len(installedMatches) <= 1
	// therefore means "no neighbours alongside the exact match".
	var installedMatches []ModuleMatch
	for _, cat := range cats {
		matches, err := searchInstalled(r.cfg.Value, cat.key, cat.category, name)
		if err != nil {
			continue
		}
		installedMatches = append(installedMatches, matches...)
	}

	// Single exact match with no other installed neighbours (in any category)
	// — return directly. Any neighbour, even a single substring hit in a
	// different category, falls through to the combined-search prompt so the
	// user can disambiguate rather than silently getting the exact match.
	if len(exactMatches) == 1 && len(installedMatches) <= 1 {
		return exactMatches[0], nil
	}

	// No exact match, single substring match — return directly.
	if len(exactMatches) == 0 && len(installedMatches) == 1 {
		return installedMatches[0], nil
	}

	// Step 3: Registry search. Only reached when installed-only resolution
	// failed to produce a single match — this gates the network call.
	index, _, _ := r.ensureIndex()

	// Exact registry match (only when no installed matches).
	if len(installedMatches) == 0 && index != nil {
		for _, cat := range cats {
			entries := registryEntries(index, cat.category)
			if entries == nil {
				continue
			}
			result, err := findExactInRegistry(entries, cat.category, name)
			if err != nil {
				return ModuleMatch{}, err
			}
			if result != nil {
				match := ModuleMatch{
					Name:     result.Name,
					Category: cat.category,
					Source:   ModuleSourceRegistry,
					Entry:    result.Entry,
					Score:    100,
				}
				if err := r.installIfRegistry(match); err != nil {
					return ModuleMatch{}, err
				}
				return match, nil
			}
		}
	}

	// Combined search across installed + registry.
	var registryMatches []ModuleMatch
	if index != nil {
		for _, cat := range cats {
			entries := registryEntries(index, cat.category)
			if entries == nil {
				continue
			}
			regMatches, err := searchRegistryCategory(entries, cat.category, name)
			if err != nil {
				continue
			}
			registryMatches = append(registryMatches, regMatches...)
		}
	}

	allMatches := mergeModuleMatches(installedMatches, registryMatches)

	switch len(allMatches) {
	case 0:
		return ModuleMatch{}, fmt.Errorf("no matches found for %q", query)
	case 1:
		if err := r.installIfRegistry(allMatches[0]); err != nil {
			return ModuleMatch{}, err
		}
		return allMatches[0], nil
	default:
		selected, err := promptCrossCategorySelection(r, allMatches, query)
		if err != nil {
			return ModuleMatch{}, err
		}
		if err := r.installIfRegistry(selected); err != nil {
			return ModuleMatch{}, err
		}
		return selected, nil
	}
}

// installIfRegistry auto-installs the match when it originates from the
// registry. On success r.autoInstall sets r.didInstall = true (resolve.go:631),
// and callers flip their scope to config.ScopeMerged to see the new module.
func (r *resolver) installIfRegistry(match ModuleMatch) error {
	if match.Source != ModuleSourceRegistry {
		return nil
	}
	return r.autoInstall(r.client, modules.SearchResult{
		Category: match.Category,
		Name:     match.Name,
		Entry:    match.Entry,
	})
}

// promptCrossCategorySelection asks the user to pick from multiple
// cross-category matches and returns the chosen match. In non-TTY mode it
// returns an ambiguity error listing all matches as "category:name". Each
// candidate string round-trips: pasting it back as the command argument
// resolves to that exact match.
func promptCrossCategorySelection(r *resolver, matches []ModuleMatch, query string) (ModuleMatch, error) {
	sort.SliceStable(matches, func(i, j int) bool {
		return formatAddress(matches[i].Category, matches[i].Name) < formatAddress(matches[j].Category, matches[j].Name)
	})

	w := r.stdout
	stdin := r.stdin
	isTTY := isTerminal(stdin)

	if !isTTY {
		shown := matches
		truncated := false
		if len(shown) > maxModuleResults {
			shown = shown[:maxModuleResults]
			truncated = true
		}
		var b strings.Builder
		fmt.Fprintf(&b, "ambiguous name %q matches:", query)
		for _, m := range shown {
			fmt.Fprintf(&b, "\n  %s", formatAddress(m.Category, m.Name))
		}
		if truncated {
			fmt.Fprintf(&b, "\n(showing %d of %d; refine search for more specific results)", len(shown), len(matches))
		}
		b.WriteString("\nSpecify exact name or run interactively")
		return ModuleMatch{}, errors.New(b.String())
	}

	displayCount := min(len(matches), maxModuleResults)

	_, _ = fmt.Fprintf(w, "Found %d matches for %q:\n\n", len(matches), query)

	maxDisplayLen := 0
	for i := range displayCount {
		display := formatAddress(matches[i].Category, matches[i].Name)
		if len(display) > maxDisplayLen {
			maxDisplayLen = len(display)
		}
	}

	for i := range displayCount {
		m := matches[i]
		display := formatAddress(m.Category, m.Name)
		padding := strings.Repeat(" ", maxDisplayLen-len(display)+2)
		var sourceLabel string
		if m.Source == ModuleSourceInstalled {
			sourceLabel = tui.ColorInstalled.Sprint(m.Source)
		} else {
			sourceLabel = tui.ColorRegistry.Sprint(m.Source)
		}
		_, _ = fmt.Fprintf(w, "  %2d. %s%s%s\n", i+1, display, padding, sourceLabel)
	}

	if displayCount < len(matches) {
		_, _ = fmt.Fprintf(w, "\nShowing %d of %d matches. Refine search for more specific results.\n",
			displayCount, len(matches))
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", displayCount))

	reader := bufio.NewReader(stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ModuleMatch{}, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= displayCount {
			return matches[choice-1], nil
		}
		return ModuleMatch{}, fmt.Errorf("invalid selection: %s (choose 1-%d)", input, displayCount)
	}

	return ModuleMatch{}, fmt.Errorf("invalid selection: %s", input)
}
