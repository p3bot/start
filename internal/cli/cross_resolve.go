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

// resolveCrossCategory resolves a module query across categories via three-tier
// search (exact installed → substring installed → registry), prompting on
// ambiguity. Auto-installs registry matches and sets r.didInstall on install.
//
// Callers needing clean stdout (e.g. `start get` piping content) put stderr in
// the stdout slot: newResolver(cfg, flags, stderr, stderr, stdin).
//
// Post-call: when r.didInstall is true, the module is on disk but r.cfg is not
// refreshed; a caller that then reads r.cfg.Value must call r.reloadConfig
// first (see runStart, runTask). describe is exempt — describeVerboseItem loads
// config independently via prepareDescribe.
func resolveCrossCategory(query string, r *resolver) (ModuleMatch, error) {
	addr, err := parseAddress(query)
	if err != nil {
		return ModuleMatch{}, err
	}

	// A category prefix scopes every per-category loop below to that one
	// category; with no prefix, all four categories are searched.
	cats := describeCategories
	if addr.HasPrefix {
		if c := describeCategoryFor(addr.Category); c != nil {
			cats = []describeCategory{*c}
		}
	}
	name := addr.Name

	// Step 1: exact match in installed config across the scoped categories.
	var exactMatches []ModuleMatch
	var ambiguousMatches []ModuleMatch
	for _, cat := range cats {
		resolved, err := findExactInstalledName(r.cfg.Value, cat.key, name)
		if err != nil {
			// Ambiguous short name — collect all matches for selection.
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
		// Both slices are installed-only here, so no install is needed.
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
		// exactMatches is installed-only here, so no install is needed.
		selected, err := promptCrossCategorySelection(r, exactMatches, query)
		if err != nil {
			return ModuleMatch{}, err
		}
		return selected, nil
	}

	// Step 2: substring search in installed config, reused by the gate below
	// and the combined-search path. An exact match also appears here as a
	// self-substring, so len(installedMatches) <= 1 means "no neighbours".
	var installedMatches []ModuleMatch
	for _, cat := range cats {
		matches, err := searchInstalled(r.cfg.Value, cat.key, cat.category, name)
		if err != nil {
			continue
		}
		installedMatches = append(installedMatches, matches...)
	}

	// Single exact match with no neighbours — return directly. Any neighbour
	// falls through to the prompt so the user disambiguates rather than
	// silently getting the exact match.
	if len(exactMatches) == 1 && len(installedMatches) <= 1 {
		return exactMatches[0], nil
	}

	if len(exactMatches) == 0 && len(installedMatches) == 1 {
		return installedMatches[0], nil
	}

	// Step 3: registry search. Only reached when installed-only resolution
	// failed to produce a single match — this gates the network call.
	index, _, _ := r.ensureIndex()

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
		return ModuleMatch{}, notFoundError(fmt.Errorf("no matches found for %q", query))
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

// installIfRegistry auto-installs the match when it comes from the registry. On
// success r.autoInstall sets r.didInstall, and callers flip their scope to
// config.ScopeMerged to see the new module.
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

// promptCrossCategorySelection asks the user to pick from multiple matches. In
// non-TTY mode it returns an ambiguity error listing matches as "category:name";
// each candidate round-trips back to that exact match as a command argument.
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
		return ModuleMatch{}, usageError(errors.New(b.String()))
	}

	displayCount := min(len(matches), maxModuleResults)

	fmt.Fprintf(w, "Found %d matches for %q:\n\n", len(matches), query)

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
		fmt.Fprintf(w, "  %2d. %s%s%s\n", i+1, display, padding, sourceLabel)
	}

	if displayCount < len(matches) {
		fmt.Fprintf(w, "\nShowing %d of %d matches. Refine search for more specific results.\n",
			displayCount, len(matches))
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", displayCount))

	reader := bufio.NewReader(stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return ModuleMatch{}, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= displayCount {
			fmt.Fprintln(w)
			return matches[choice-1], nil
		}
		return ModuleMatch{}, fmt.Errorf("invalid selection: %s (choose 1-%d)", input, displayCount)
	}

	return ModuleMatch{}, fmt.Errorf("invalid selection: %s", input)
}
