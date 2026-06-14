package cli

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
)

// matchMode selects how a candidate name is compared against the query. The
// exact tier always uses modeExact; the fallback tier uses modeSubstring for a
// bare term and modePrefix for a category-qualified one. Matching is literal
// (no regex) and case-insensitive throughout.
type matchMode int

const (
	modeExact matchMode = iota
	modeSubstring
	modePrefix
)

// nameMatches reports whether candidate matches query under mode, comparing
// case-insensitively over the names only. Both operands are lower-cased so the
// slash in a path-shaped name is an ordinary character, not a separator.
func nameMatches(query, candidate string, mode matchMode) bool {
	q := strings.ToLower(query)
	c := strings.ToLower(candidate)
	switch mode {
	case modeExact:
		return q == c
	case modePrefix:
		return strings.HasPrefix(c, q)
	default:
		return strings.Contains(c, q)
	}
}

// resolveScope parameterises the unified resolver for one surface: which
// categories it spans, whether it is a cross-category surface (display as
// category:name and consult the registry in the exact tier to detect twins),
// whether a locator (local path or http(s) URL) is accepted, and the noun used
// in messages.
type resolveScope struct {
	categories    []describeCategory
	crossCategory bool
	allowLocator  bool
	displayType   string
}

// resolveOutcome is the result of resolution: either a locator (a local path or
// http(s) URL, read directly with no search) or a resolved module match.
type resolveOutcome struct {
	locator string
	match   ModuleMatch
}

// singleCategoryScope builds a scope for a category-specific surface
// (--role, --agent, start task, --context).
func singleCategoryScope(category, displayType string, allowLocator bool) resolveScope {
	return resolveScope{
		categories:   []describeCategory{*describeCategoryFor(category)},
		allowLocator: allowLocator,
		displayType:  displayType,
	}
}

// crossCategoryScope builds the scope for the cross-category surfaces
// (start get, start describe), spanning all four categories.
func crossCategoryScope() resolveScope {
	return resolveScope{
		categories:    describeCategories,
		crossCategory: true,
		allowLocator:  true,
		displayType:   "module",
	}
}

// surfaceKind classifies how a surface identifier is handled before any lookup.
type surfaceKind int

const (
	surfaceName    surfaceKind = iota // a module name to look up (cats/mode/name set)
	surfaceLocator                    // a local path or http(s) URL, read directly
	surfaceSkip                       // empty or a none/default sentinel — no index touch
)

// surfaceInterpretation is the result of classifying a surface identifier: the
// shared front-half of resolution that both resolve() and the up-front liveness
// union consume, so the two cannot drift on what an identifier means.
type surfaceInterpretation struct {
	kind    surfaceKind
	locator string             // when kind == surfaceLocator
	cats    []describeCategory // when kind == surfaceName: scoped categories
	mode    matchMode          // when kind == surfaceName: fallback match mode
	name    string             // when kind == surfaceName: the bare name
}

// interpretSurface classifies an identifier exactly as resolve() does before it
// looks anything up: an empty value or a context none/default sentinel is a skip
// (the surface never touches the index), a path or http(s) URL is a locator
// (read directly), and anything else is a name whose category prefix scopes the
// lookup and selects prefix vs substring fallback. A malformed category prefix
// returns a usage error.
//
// This is the single source of the locator/prefix/sentinel interpretation. Both
// resolve()'s own dispatch and computeWantLive call it, so a future change to
// what an identifier means updates both at once.
func interpretSurface(input string, scope resolveScope) (surfaceInterpretation, error) {
	// Empty is the universal "use the default" signal. none/default are context
	// sentinels (role's none is normalised to empty upstream); they apply only to
	// the category-specific contexts surface, never to a cross-category get/
	// describe query where "default" or "none" could name a real module.
	if input == "" {
		return surfaceInterpretation{kind: surfaceSkip}, nil
	}
	if scope.displayType == "context" && (isNoneToken(input) || input == "default") {
		return surfaceInterpretation{kind: surfaceSkip}, nil
	}
	if orchestration.IsLocator(input) {
		return surfaceInterpretation{kind: surfaceLocator, locator: input}, nil
	}

	addr, err := parseAddress(input)
	if err != nil {
		return surfaceInterpretation{}, err
	}

	cats := scope.categories
	mode := modeSubstring
	if addr.HasPrefix {
		mode = modePrefix
		if scope.crossCategory {
			// parseAddress already validated the category, so the lookup is non-nil.
			cats = []describeCategory{*describeCategoryFor(addr.Category)}
		} else if addr.Category != scope.categories[0].category {
			return surfaceInterpretation{}, usageError(fmt.Errorf("%s expects category %q, got %q in %q",
				scope.displayType, scope.categories[0].category, addr.Category, input))
		}
	}
	return surfaceInterpretation{kind: surfaceName, cats: cats, mode: mode, name: addr.Name}, nil
}

// resolve turns an identifier into an outcome through the unified match rule:
// locator bypass, category-prefix interpretation, the exact-whole-name tier,
// the three-character floor, then the fallback tier, reducing to a single
// decision and installing a chosen registry-only match.
func (r *resolver) resolve(input string, scope resolveScope) (resolveOutcome, error) {
	interp, err := interpretSurface(input, scope)
	if err != nil {
		return resolveOutcome{}, err
	}
	switch interp.kind {
	case surfaceLocator:
		if !scope.allowLocator {
			return resolveOutcome{}, usageError(fmt.Errorf("%s does not accept a file path or URL: %q", scope.displayType, input))
		}
		debugf(r.stderr, r.flags, dbgResolve, "%s %q: locator bypass", scope.displayType, input)
		return resolveOutcome{locator: input}, nil
	case surfaceSkip:
		// A sentinel/empty reaching resolve() is a caller bug: resolveSingle and
		// resolveContexts filter these before dispatching. Treat defensively as
		// not-found rather than silently mis-resolving.
		return resolveOutcome{}, notFoundError(fmt.Errorf("%s %q not found", scope.displayType, input))
	}

	cats := interp.cats
	mode := interp.mode
	name := interp.name

	// Exact tier first, exempt from the floor.
	match, resolved, err := r.resolveExact(name, cats, scope)
	if err != nil {
		return resolveOutcome{}, err
	}
	if resolved {
		return resolveOutcome{match: match}, nil
	}

	// Fallback tier: floor counts the name only, both modes. When the index was
	// unreachable the exact tier could not confirm absence, so a short name is a
	// transient (retry) condition rather than a usage violation — the same
	// certainty split resolveFallback applies below.
	if len(name) < 3 {
		if r.indexErr != nil {
			return resolveOutcome{}, fmt.Errorf("%s %q: registry unavailable: %w", scope.displayType, name, r.indexErr)
		}
		return resolveOutcome{}, usageError(fmt.Errorf("query %q must be at least 3 characters", name))
	}
	return r.resolveFallback(name, cats, mode, scope)
}

// resolveExact runs the exact-whole-name tier over the scoped categories. A lone
// installed exact resolves directly without the registry on category-specific
// surfaces (names are unique within a category); cross-category surfaces always
// consult the index to detect a same-name twin in another category. More than
// one exact falls to selection.
func (r *resolver) resolveExact(name string, cats []describeCategory, scope resolveScope) (ModuleMatch, bool, error) {
	var installedExact []ModuleMatch
	for _, cat := range cats {
		installedExact = append(installedExact, r.collectInstalled(cat.key, cat.category, name, modeExact)...)
	}

	if !scope.crossCategory && len(installedExact) == 1 {
		debugf(r.stderr, r.flags, dbgResolve, "%s %q: exact installed match", scope.displayType, name)
		return installedExact[0], true, nil
	}

	var registryExact []ModuleMatch
	if index, _, _ := r.ensureIndex(); index != nil {
		for _, cat := range cats {
			registryExact = append(registryExact, collectRegistry(registryEntries(index, cat.category), cat.category, name, modeExact)...)
		}
	}

	exact := mergeMatches(installedExact, registryExact)
	switch len(exact) {
	case 0:
		return ModuleMatch{}, false, nil
	case 1:
		m, err := r.use(exact[0])
		if err != nil {
			return ModuleMatch{}, false, err
		}
		return m, true, nil
	default:
		selected, err := r.selectMatch(exact, scope, name)
		if err != nil {
			return ModuleMatch{}, false, err
		}
		m, err := r.use(selected)
		if err != nil {
			return ModuleMatch{}, false, err
		}
		return m, true, nil
	}
}

// resolveFallback runs the fallback tier over the scoped categories under mode,
// reducing the merged match set to one decision. Zero matches are a not-found
// error when the index was reachable, and a transient (retry) error when it was
// not, since absence cannot be confirmed.
func (r *resolver) resolveFallback(name string, cats []describeCategory, mode matchMode, scope resolveScope) (resolveOutcome, error) {
	var installed []ModuleMatch
	for _, cat := range cats {
		installed = append(installed, r.collectInstalled(cat.key, cat.category, name, mode)...)
	}

	var reg []ModuleMatch
	if index, _, _ := r.ensureIndex(); index != nil {
		for _, cat := range cats {
			reg = append(reg, collectRegistry(registryEntries(index, cat.category), cat.category, name, mode)...)
		}
	}

	matches := mergeMatches(installed, reg)
	debugf(r.stderr, r.flags, dbgResolve, "%s %q: %d installed, %d registry, %d total matches",
		scope.displayType, name, len(installed), len(reg), len(matches))

	switch len(matches) {
	case 0:
		if r.indexErr != nil {
			return resolveOutcome{}, fmt.Errorf("%s %q: registry unavailable: %w", scope.displayType, name, r.indexErr)
		}
		return resolveOutcome{}, notFoundError(fmt.Errorf("%s %q not found", scope.displayType, name))
	case 1:
		m, err := r.use(matches[0])
		if err != nil {
			return resolveOutcome{}, err
		}
		return resolveOutcome{match: m}, nil
	default:
		selected, err := r.selectMatch(matches, scope, name)
		if err != nil {
			return resolveOutcome{}, err
		}
		m, err := r.use(selected)
		if err != nil {
			return resolveOutcome{}, err
		}
		return resolveOutcome{match: m}, nil
	}
}

// use installs a registry-only match (a no-op for an installed one) and returns
// it as the resolved decision.
func (r *resolver) use(m ModuleMatch) (ModuleMatch, error) {
	if err := r.installIfRegistry(m); err != nil {
		return ModuleMatch{}, err
	}
	return m, nil
}

// collectInstalled returns matches under cueKey whose names satisfy mode.
func (r *resolver) collectInstalled(cueKey, category, query string, mode matchMode) []ModuleMatch {
	catVal := r.cfg.Value.LookupPath(cue.ParsePath(cueKey))
	if !catVal.Exists() {
		return nil
	}
	iter, err := catVal.Fields()
	if err != nil {
		return nil
	}
	var out []ModuleMatch
	for iter.Next() {
		name := iter.Selector().Unquoted()
		if nameMatches(query, name, mode) {
			out = append(out, ModuleMatch{Name: name, Category: category, Source: ModuleSourceInstalled})
		}
	}
	return out
}

// collectRegistry returns registry entries whose names satisfy mode.
func collectRegistry(entries map[string]registry.IndexEntry, category, query string, mode matchMode) []ModuleMatch {
	var out []ModuleMatch
	for name, entry := range entries {
		if nameMatches(query, name, mode) {
			out = append(out, ModuleMatch{Name: name, Category: category, Source: ModuleSourceRegistry, Entry: entry})
		}
	}
	return out
}

// mergeMatches de-duplicates by category:name with the installed entry winning,
// and orders installed before registry, then lexically by category:name. There
// is no score-based ordering: resolution does not score.
func mergeMatches(installed, reg []ModuleMatch) []ModuleMatch {
	seen := make(map[string]bool)
	var out []ModuleMatch
	for _, group := range [][]ModuleMatch{installed, reg} {
		for _, m := range group {
			key := formatAddress(m.Category, m.Name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		iInstalled := out[i].Source == ModuleSourceInstalled
		jInstalled := out[j].Source == ModuleSourceInstalled
		if iInstalled != jInstalled {
			return iInstalled
		}
		return formatAddress(out[i].Category, out[i].Name) < formatAddress(out[j].Category, out[j].Name)
	})
	return out
}

// matchLabel renders a match as the user-facing token that round-trips back to
// it: a bare name on a category-specific surface, category:name on a
// cross-category one.
func matchLabel(m ModuleMatch, scope resolveScope) string {
	if scope.crossCategory {
		return formatAddress(m.Category, m.Name)
	}
	return m.Name
}

// selectMatch reduces more than one match to one: a selection menu on a TTY, an
// error listing the matches otherwise. The listed forms are valid arguments
// that round-trip back to the same entry.
func (r *resolver) selectMatch(matches []ModuleMatch, scope resolveScope, query string) (ModuleMatch, error) {
	sort.SliceStable(matches, func(i, j int) bool {
		return matchLabel(matches[i], scope) < matchLabel(matches[j], scope)
	})

	if !isTerminal(r.stdin) {
		labels := make([]string, len(matches))
		for i, m := range matches {
			labels[i] = matchLabel(m, scope)
		}
		return ModuleMatch{}, usageError(fmt.Errorf("ambiguous %s %q matches: %s\nSpecify an exact name, category-qualify it (category:name), or run interactively",
			scope.displayType, query, strings.Join(labels, ", ")))
	}

	displayCount := min(len(matches), maxModuleResults)

	fmt.Fprintf(r.stdout, "Found %d %ss matching %q:\n\n", len(matches), scope.displayType, query)

	maxLabelLen := 0
	for i := range displayCount {
		if l := len(matchLabel(matches[i], scope)); l > maxLabelLen {
			maxLabelLen = l
		}
	}
	for i := range displayCount {
		m := matches[i]
		label := matchLabel(m, scope)
		padding := strings.Repeat(" ", maxLabelLen-len(label)+2)
		sourceColor := tui.ColorRegistry
		if m.Source == ModuleSourceInstalled {
			sourceColor = tui.ColorInstalled
		}
		fmt.Fprintf(r.stdout, "  %2d. %s%s%s\n", i+1, label, padding, sourceColor.Sprint(m.Source))
	}
	if displayCount < len(matches) {
		fmt.Fprintf(r.stdout, "\nShowing %d of %d matches. Refine search for more specific results.\n", displayCount, len(matches))
	}

	fmt.Fprintln(r.stdout)
	fmt.Fprintf(r.stdout, "Select %s: ", tui.Annotate("1-%d", displayCount))

	input, err := bufio.NewReader(r.stdin).ReadString('\n')
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

	selected, err := matchByTypedName(matches[:displayCount], scope, input)
	if err != nil {
		return ModuleMatch{}, err
	}
	fmt.Fprintln(r.stdout)
	return selected, nil
}

// matchByTypedName selects the one shown entry a typed name uniquely
// identifies, in precedence order: an exact label (category:name on a
// cross-category menu) wins outright; otherwise an exact bare name wins when it
// is unique; otherwise a single substring match over the labels is accepted. A
// bare name shared by two cross-category twins is ambiguous and rejected, so the
// user re-enters the full category:name or picks by number.
func matchByTypedName(shown []ModuleMatch, scope resolveScope, input string) (ModuleMatch, error) {
	inputLower := strings.ToLower(input)
	for _, m := range shown {
		if strings.ToLower(matchLabel(m, scope)) == inputLower {
			return m, nil
		}
	}
	var nameMatches []ModuleMatch
	for _, m := range shown {
		if strings.ToLower(m.Name) == inputLower {
			nameMatches = append(nameMatches, m)
		}
	}
	if len(nameMatches) == 1 {
		return nameMatches[0], nil
	}
	if len(nameMatches) == 0 {
		var subMatches []ModuleMatch
		for _, m := range shown {
			if strings.Contains(strings.ToLower(matchLabel(m, scope)), inputLower) {
				subMatches = append(subMatches, m)
			}
		}
		if len(subMatches) == 1 {
			return subMatches[0], nil
		}
	}
	return ModuleMatch{}, fmt.Errorf("invalid selection: %s", input)
}

// installIfRegistry auto-installs the match when it is registry-only. On success
// r.autoInstall sets r.didInstall and r.cfgStale; installed matches are a no-op.
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
