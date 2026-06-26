package modules

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"github.com/start-cli/start/internal/fault"
	"github.com/start-cli/start/internal/registry"
)

// searchCategories lists the four module categories in display order. It is the
// modules-package source of truth for category-prefix validation, mirroring the
// CLI's describeCategories ordering so error messages match across surfaces.
var searchCategories = []string{"agents", "roles", "contexts", "tasks"}

// SplitCategoryQuery peels an optional "category:" prefix off a search query so
// install and search honour the same category:name addressing the resolution
// engine uses. With no colon the whole input is the query and category is "".
// With a colon the prefix must name a known category — an unknown one is a usage
// fault (exit 2), matching how get/describe/--role reject a bad category — and
// the remainder becomes the query, scoped to that category. Comparison is
// case-sensitive against the lowercase category names, consistent with the CLI's
// parseAddress. Module names never contain a colon (lowercase kebab-case with
// slashes), so a colon is always a category delimiter, never part of a name.
//
// The CLI also calls this to measure its 3-character floor against the returned
// query (the name only, prefix excluded), keeping search and install consistent
// with the engine's floor rule; SearchIndex and SearchInstalledConfig re-split
// internally so they stay correct regardless of any caller-side split.
func SplitCategoryQuery(input string) (category, query string, err error) {
	before, after, ok := strings.Cut(input, ":")
	if !ok {
		return "", input, nil
	}
	if !slices.Contains(searchCategories, before) {
		return "", "", fault.Usage(fmt.Errorf("unknown category %q (valid: %s)", before, strings.Join(searchCategories, ", ")))
	}
	return before, after, nil
}

// SearchResult holds a matched index entry with its category and name.
type SearchResult struct {
	Category string              `json:"category"`
	Name     string              `json:"name"`
	Entry    registry.IndexEntry `json:"entry"`
}

// ParseSearchTerms splits input into unique, lowercased terms (on whitespace and commas).
// Use ParseSearchPatterns instead when terms will be compiled as regex patterns.
func ParseSearchTerms(input string) []string {
	normalized := strings.ReplaceAll(input, ",", " ")
	parts := strings.Fields(normalized)

	seen := make(map[string]bool, len(parts))
	var terms []string
	for _, p := range parts {
		lower := strings.ToLower(p)
		if !seen[lower] {
			seen[lower] = true
			terms = append(terms, lower)
		}
	}
	return terms
}

// ParseSearchPatterns splits input into unique patterns, preserving original case.
// Dedup is case-insensitive but keeps the first casing, so case-sensitive regex
// escapes like \S, \D, \W, \B are not corrupted.
func ParseSearchPatterns(input string) []string {
	normalized := strings.ReplaceAll(input, ",", " ")
	parts := strings.Fields(normalized)

	seen := make(map[string]bool, len(parts))
	var patterns []string
	for _, p := range parts {
		lower := strings.ToLower(p)
		if !seen[lower] {
			seen[lower] = true
			patterns = append(patterns, p)
		}
	}
	return patterns
}

// CompileSearchTerms compiles search terms into case-insensitive regular expressions.
func CompileSearchTerms(terms []string) ([]*regexp.Regexp, error) {
	patterns := make([]*regexp.Regexp, len(terms))
	for i, term := range terms {
		re, err := regexp.Compile("(?i)" + term)
		if err != nil {
			return nil, fmt.Errorf("invalid search pattern %q: %w", term, err)
		}
		patterns[i] = re
	}
	return patterns, nil
}

// ValidateSearchQuery checks query length: empty allowed only with tags, else >= 3 chars total.
func ValidateSearchQuery(terms, tags []string) error {
	totalLen := 0
	for _, t := range terms {
		totalLen += len(t)
	}
	if totalLen > 0 && totalLen < 3 {
		return fmt.Errorf("query must be at least 3 characters")
	}
	if totalLen == 0 && len(tags) == 0 {
		return fmt.Errorf("query must be at least 3 characters")
	}
	return nil
}

// SearchIndex searches all index categories. Query terms are regex patterns with AND
// semantics; tags (if any) additionally require an OR tag match.
func SearchIndex(index *registry.Index, query string, tags []string) ([]SearchResult, error) {
	if index == nil {
		return nil, nil
	}

	category, query, err := SplitCategoryQuery(query)
	if err != nil {
		return nil, err
	}

	terms := ParseSearchPatterns(query)
	if len(terms) == 0 && len(tags) == 0 {
		return nil, nil
	}

	var patterns []*regexp.Regexp
	if len(terms) > 0 {
		patterns, err = CompileSearchTerms(terms)
		if err != nil {
			return nil, err
		}
	}

	scoped := []struct {
		name    string
		entries map[string]registry.IndexEntry
	}{
		{"agents", index.Agents},
		{"roles", index.Roles},
		{"contexts", index.Contexts},
		{"tasks", index.Tasks},
	}

	var results []SearchResult
	for _, c := range scoped {
		if category != "" && c.name != category {
			continue
		}
		results = append(results, searchCategory(c.name, c.entries, patterns, tags)...)
	}

	// Sort by category order, then name.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Category != results[j].Category {
			return CategoryOrder(results[i].Category) < CategoryOrder(results[j].Category)
		}
		return results[i].Name < results[j].Name
	})

	return results, nil
}

// searchCategory searches a single category map for matching entries.
// The three branches stay explicit rather than collapsed into a single
// matchesPatterns(...) && matchesAnyTag(...): tags-only mode has nil patterns
// (matchesPatterns reports false) and patterns-only mode has nil tags
// (matchesAnyTag reports false), so a merged expression would wrongly exclude both.
func searchCategory(category string, entries map[string]registry.IndexEntry, patterns []*regexp.Regexp, tags []string) []SearchResult {
	var results []SearchResult

	for name, entry := range entries {
		var match bool
		switch {
		case len(patterns) > 0 && len(tags) > 0:
			match = matchesPatterns(name, entry, patterns) && matchesAnyTag(entry.Tags, tags)
		case len(tags) > 0:
			match = matchesAnyTag(entry.Tags, tags)
		default:
			match = matchesPatterns(name, entry, patterns)
		}

		if match {
			results = append(results, SearchResult{
				Category: category,
				Name:     name,
				Entry:    entry,
			})
		}
	}

	return results
}

// matchesAnyTag reports whether any entry tag case-insensitively equals any filter tag.
func matchesAnyTag(entryTags, filterTags []string) bool {
	for _, ft := range filterTags {
		ftLower := strings.ToLower(ft)
		for _, et := range entryTags {
			if strings.ToLower(et) == ftLower {
				return true
			}
		}
	}
	return false
}

// matchesPatterns reports whether every pattern matches the name, description, or any
// tag (AND across patterns). Returns false for nil patterns.
func matchesPatterns(name string, entry registry.IndexEntry, patterns []*regexp.Regexp) bool {
	if len(patterns) == 0 {
		return false
	}

	for _, pattern := range patterns {
		matched := pattern.MatchString(name) ||
			pattern.MatchString(entry.Description) ||
			slices.ContainsFunc(entry.Tags, pattern.MatchString)
		if !matched {
			return false // AND: every pattern must match something
		}
	}

	return true
}

// SearchInstalledConfig searches installed config entries under cueKey (e.g. "agents"),
// matching them the same way as the registry index search.
func SearchInstalledConfig(cfg cue.Value, cueKey, category, query string, tags []string) ([]SearchResult, error) {
	scopeCat, query, err := SplitCategoryQuery(query)
	if err != nil {
		return nil, err
	}
	// A category-scoped query restricts the installed search to its category;
	// this per-category call self-skips when the scope names another one.
	if scopeCat != "" && scopeCat != category {
		return nil, nil
	}

	catVal := cfg.LookupPath(cue.ParsePath(cueKey))
	if !catVal.Exists() {
		return nil, nil
	}

	iter, err := catVal.Fields()
	if err != nil {
		return nil, fmt.Errorf("iterating %s fields: %w", cueKey, err)
	}

	terms := ParseSearchPatterns(query)
	if len(terms) == 0 && len(tags) == 0 {
		return nil, nil
	}

	var patterns []*regexp.Regexp
	if len(terms) > 0 {
		patterns, err = CompileSearchTerms(terms)
		if err != nil {
			return nil, err
		}
	}

	entries := make(map[string]registry.IndexEntry)
	for iter.Next() {
		name := iter.Selector().Unquoted()
		entries[name] = extractIndexEntryFromCUE(iter.Value())
	}

	results := searchCategory(category, entries, patterns, tags)

	sortResults(results)

	return results, nil
}

// sortResults sorts by name ascending.
func sortResults(results []SearchResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
}

// extractIndexEntryFromCUE extracts description, tags, and origin from a CUE value
// into an IndexEntry for matching.
func extractIndexEntryFromCUE(v cue.Value) registry.IndexEntry {
	var entry registry.IndexEntry

	if desc := v.LookupPath(cue.ParsePath("description")); desc.Exists() {
		entry.Description, _ = desc.String()
	}

	if tags := v.LookupPath(cue.ParsePath("tags")); tags.Exists() {
		tagIter, err := tags.List()
		if err == nil {
			for tagIter.Next() {
				if s, err := tagIter.Value().String(); err == nil {
					entry.Tags = append(entry.Tags, s)
				}
			}
		}
	}

	if origin := v.LookupPath(cue.ParsePath("origin")); origin.Exists() {
		entry.Module, _ = origin.String()
	}

	return entry
}

// CategoryOrder returns the display order for a category.
func CategoryOrder(category string) int {
	switch category {
	case "agents":
		return 0
	case "roles":
		return 1
	case "contexts":
		return 2
	case "tasks":
		return 3
	default:
		return 4
	}
}
