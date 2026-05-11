package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"cuelang.org/go/cue"
	cueformat "cuelang.org/go/cue/format"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/tui"
)

// DescribeResult holds the result of preparing describe output.
type DescribeResult struct {
	ItemType string    // "Agent", "Role", "Context", "Task"
	Category string    // "agents", "roles", "contexts", "tasks"
	CueKey   string    // Top-level CUE key (e.g., "agents")
	Name     string    // Item name (when describing a specific item)
	Value    cue.Value // The CUE value for this item
	AllNames []string  // All available items of this type
	Reason   string    // Why this item is described (e.g., "first in config", "default")
}

// describeCategory maps category metadata used for cross-category operations.
type describeCategory struct {
	key      string // CUE key (e.g., "agents")
	category string // Category name (e.g., "agents")
	itemType string // Display type (e.g., "Agent")
}

var describeCategories = []describeCategory{
	{internalcue.KeyAgents, "agents", "Agent"},
	{internalcue.KeyRoles, "roles", "Role"},
	{internalcue.KeyContexts, "contexts", "Context"},
	{internalcue.KeyTasks, "tasks", "Task"},
}

// describeCategoryFor looks up a describeCategory by its category string.
// Returns nil only if category is not in describeCategories. All callers pass
// Category values that originate from iterating describeCategories, so nil is
// unreachable in practice. If a new ModuleMatch source is added, ensure its
// Category is drawn from describeCategories.
func describeCategoryFor(category string) *describeCategory {
	for i := range describeCategories {
		if describeCategories[i].category == category {
			return &describeCategories[i]
		}
	}
	return nil
}

// parsedAddress represents a user-facing module address. When the input
// contained an explicit category:name prefix, Category is set and HasPrefix
// is true. Otherwise Name holds the entire input and Category is empty.
type parsedAddress struct {
	Category  string
	Name      string
	HasPrefix bool
}

// parseAddress splits a user-facing address on the first colon. When a colon
// is present, the left segment must be one of the four known categories
// (agents, roles, contexts, tasks) — an unknown prefix returns an error
// listing the valid set. When no colon is present, the entire input is
// returned as the bare name with HasPrefix=false.
func parseAddress(input string) (parsedAddress, error) {
	before, after, ok := strings.Cut(input, ":")
	if !ok {
		return parsedAddress{Name: input}, nil
	}
	cat := before
	name := after
	if describeCategoryFor(cat) == nil {
		return parsedAddress{}, fmt.Errorf("unknown category %q (valid: %s)", cat, knownCategoriesList())
	}
	return parsedAddress{Category: cat, Name: name, HasPrefix: true}, nil
}

// knownCategoriesList returns the four valid categories as a comma-separated
// string for use in error messages.
func knownCategoriesList() string {
	names := make([]string, len(describeCategories))
	for i, c := range describeCategories {
		names[i] = c.category
	}
	return strings.Join(names, ", ")
}

// formatAddress returns the canonical user-facing address for a category and
// name pair: "category:name". Used at every display site that emits a
// fully-qualified module address.
func formatAddress(category, name string) string {
	return category + ":" + name
}

// addDescribeCommand adds the describe command and its subcommands to the parent command.
func addDescribeCommand(parent *cobra.Command) {
	describeCmd := &cobra.Command{
		Use:     "describe [name]",
		GroupID: "commands",
		Short:   "Display resolved configuration content",
		Long: `Display resolved configuration content from merged global and local config.

Without arguments, lists all configured items with descriptions.
With an argument, searches across all categories and displays a verbose dump.

Names may be bare (e.g. "claude") or fully qualified as "category:name"
(e.g. "agents:claude/interactive"). The category prefix scopes the search
to a single category; bare names continue to search across all four.

Use --global to restrict output to the global config (~/.config/start/) or
--local to restrict to the local config (./.start/). These flags are mutually
exclusive; omitting both shows the effective merged configuration.

Auto-installed modules always land in global config; the post-install lookup
widens to merged scope so a --local invocation can still see the new module.
To inspect strictly within --local, ensure the module is already installed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runDescribe,
	}

	// Add --global flag to describe command (describe-specific scope restriction)
	describeCmd.PersistentFlags().Bool("global", false, "Restrict to global config only")

	// Add describe to parent
	parent.AddCommand(describeCmd)
}

// runDescribe displays all configuration or searches for a specific item.
func runDescribe(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	if len(args) == 0 {
		return runDescribeListing(cmd)
	}

	query := args[0]
	prompted := false
	if len(query) < 3 {
		w := cmd.OutOrStdout()
		stdin := cmd.InOrStdin()
		if !isTerminal(stdin) {
			return fmt.Errorf("query must be at least 3 characters")
		}
		_, _ = fmt.Fprintln(w, "Query must be at least 3 characters")
		input, err := promptSearchQuery(w, stdin)
		if err != nil {
			return err
		}
		if input == "" {
			return nil
		}
		query = input
		prompted = true
	}

	if err := runDescribeSearch(cmd, query); err != nil {
		if prompted {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), capitalise(err.Error()))
			return nil
		}
		return err
	}
	return nil
}

// runDescribeListing displays all items grouped by category with descriptions.
func runDescribeListing(cmd *cobra.Command) error {
	w := cmd.OutOrStdout()
	stdin := cmd.InOrStdin()

	scope, err := describeScopeFromCmd(cmd)
	if err != nil {
		return err
	}

	// Show config paths and settings
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}
	_, _ = fmt.Fprintln(w)
	printConfigPaths(w, paths)
	_, _ = fmt.Fprintln(w)

	flags := getFlags(cmd)
	entries, err := config.ResolveAllSettings(paths, flags.Local)
	if err != nil {
		return err
	}
	_, _ = tui.ColorSettings.Fprint(w, "settings")
	_, _ = fmt.Fprintln(w, "/")
	printSettingsEntries(w, entries)
	_, _ = fmt.Fprintln(w)

	// Show categories
	cfg, err := loadConfig(scope)
	if err != nil {
		return err
	}

	for _, cat := range describeCategories {
		items := cfg.Value.LookupPath(cue.ParsePath(cat.key))
		if !items.Exists() {
			continue
		}

		type entry struct {
			name string
			desc string
		}

		var catEntries []entry
		maxNameLen := 0

		iter, err := items.Fields()
		if err != nil {
			continue
		}
		for iter.Next() {
			name := iter.Selector().Unquoted()
			desc := ""
			if d := iter.Value().LookupPath(cue.ParsePath("description")); d.Exists() {
				desc, _ = d.String()
			}
			catEntries = append(catEntries, entry{name, desc})
			if len(name) > maxNameLen {
				maxNameLen = len(name)
			}
		}

		if len(catEntries) == 0 {
			continue
		}

		_, _ = tui.CategoryColor(cat.category).Fprint(w, cat.category)
		_, _ = fmt.Fprintln(w, "/")

		for _, e := range catEntries {
			if e.desc != "" {
				padding := strings.Repeat(" ", maxNameLen-len(e.name)+2)
				_, _ = fmt.Fprintf(w, "  %s%s", e.name, padding)
				_, _ = tui.ColorDim.Fprintln(w, e.desc)
			} else {
				_, _ = fmt.Fprintf(w, "  %s\n", e.name)
			}
		}

		_, _ = fmt.Fprintln(w)
	}

	// In TTY mode, prompt for a search query
	if isTerminal(stdin) {
		query, err := promptSearchQuery(w, stdin)
		if err != nil {
			return err
		}
		if query == "" {
			return nil
		}
		if err := runDescribeSearch(cmd, query); err != nil {
			_, _ = fmt.Fprintln(w, capitalise(err.Error()))
			return nil
		}
		return nil
	}

	return nil
}

// runDescribeSearch handles cross-category search for `start describe <name>`.
func runDescribeSearch(cmd *cobra.Command, name string) error {
	w := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(w)
	stderr := cmd.ErrOrStderr()
	flags := getFlags(cmd)
	scope, err := describeScopeFromCmd(cmd)
	if err != nil {
		return err
	}
	stdin := cmd.InOrStdin()

	cfg, err := loadConfig(scope)
	if err != nil {
		return err
	}

	r := newResolver(cfg, flags, w, stderr, stdin)
	match, err := resolveCrossCategory(name, r)
	if err != nil {
		return err
	}

	// effectiveScope widens to merged after auto-install regardless of the
	// user's original --local/--global flag. autoInstall always writes to
	// global config (resolve.go's autoInstall), so merged is the smallest
	// scope guaranteed to see the new module. Under --global the lookup
	// result is identical (module only exists in global, merged is a
	// superset); under --local the widening is required for the lookup to
	// succeed and is signalled to the user via notifyScopeWidenedIfLocal.
	effectiveScope := scope
	if r.didInstall {
		effectiveScope = config.ScopeMerged
		notifyScopeWidenedIfLocal(stderr, flags, r.didInstall)
	}

	cat := describeCategoryFor(match.Category)
	if cat == nil {
		return fmt.Errorf("unknown category %q", match.Category)
	}
	return describeVerboseItem(w, match.Name, effectiveScope, cat.key, cat.itemType)
}

// describeVerboseItem prepares and displays a verbose dump for a single item.
func describeVerboseItem(w io.Writer, name string, scope config.Scope, cueKey, itemType string) error {
	result, err := prepareDescribe(name, scope, cueKey, itemType)
	if err != nil {
		return err
	}
	printVerboseDump(w, result)
	return nil
}

// prepareDescribe prepares describe output for an item type.
// cueKey is the top-level CUE key (e.g., internalcue.KeyRoles).
// itemType is the display name (e.g., "Role").
func prepareDescribe(name string, scope config.Scope, cueKey, itemType string) (DescribeResult, error) {
	cfg, err := loadConfig(scope)
	if err != nil {
		return DescribeResult{}, err
	}

	typePlural := strings.ToLower(itemType) + "s"

	items := cfg.Value.LookupPath(cue.ParsePath(cueKey))
	if !items.Exists() {
		return DescribeResult{}, fmt.Errorf("no %s defined in configuration", typePlural)
	}

	// Collect all names in config order
	var allNames []string
	iter, err := items.Fields()
	if err != nil {
		return DescribeResult{}, fmt.Errorf("reading %s: %w", typePlural, err)
	}
	for iter.Next() {
		allNames = append(allNames, iter.Selector().Unquoted())
	}
	if len(allNames) == 0 {
		return DescribeResult{}, fmt.Errorf("no %s defined in configuration", typePlural)
	}

	// Determine which item to describe and why
	reason := ""
	if name == "" {
		name = allNames[0]
		reason = "first in config"
	}

	resolvedName := name
	item := items.LookupPath(cue.MakePath(cue.Str(name)))
	if !item.Exists() {
		// Try substring match
		var matches []string
		for _, n := range allNames {
			if strings.Contains(n, name) {
				matches = append(matches, n)
			}
		}

		switch len(matches) {
		case 0:
			return DescribeResult{}, fmt.Errorf("%s %q not found", strings.ToLower(itemType), name)
		case 1:
			resolvedName = matches[0]
			item = items.LookupPath(cue.MakePath(cue.Str(resolvedName)))
		default:
			return DescribeResult{}, fmt.Errorf("ambiguous %s name %q matches: %s", strings.ToLower(itemType), name, strings.Join(matches, ", "))
		}
	}

	return DescribeResult{
		ItemType: itemType,
		Category: typePlural,
		CueKey:   cueKey,
		Name:     resolvedName,
		Value:    item,
		AllNames: allNames,
		Reason:   reason,
	}, nil
}

// notifyScopeWidenedIfLocal emits a one-line stderr notice when an
// auto-install during resolution silently widened --local resolution to
// merged scope. Auto-installs always land in global config (resolve.go's
// autoInstall), so a --local invocation that triggers an install will then
// look the module up against merged config — the user's literal --local
// contract is bypassed. The notice gives scripted callers a grep-able
// signal; no-op when --local was not set or when --quiet is in effect.
// Called from runGet and runDescribeSearch after the post-install reload.
//
// The post-install reload also widens --global to merged scope, but no
// notice fires for --global by design: the install lands in global config
// (matching the user's requested scope), so the widened lookup is a no-op
// in the common case. The corner case where it matters — a same-named
// local module whose fields unify with the freshly installed global one —
// is narrow enough that routine notices on every --global install would
// trade silent surprise for routine noise. If that edge case becomes a
// real problem, narrow r.reloadConfig to honour the original scope.
func notifyScopeWidenedIfLocal(stderr io.Writer, flags *Flags, didInstall bool) {
	if !didInstall || !flags.Local || flags.Quiet {
		return
	}
	printWarning(stderr, "--local widened to merged scope after registry install")
}

// describeScopeFromCmd derives the config scope from describe command flags.
// Returns an error if --local and --global are both set.
func describeScopeFromCmd(cmd *cobra.Command) (config.Scope, error) {
	var global bool
	if f := cmd.Flags().Lookup("global"); f != nil {
		global, _ = cmd.Flags().GetBool("global")
	}
	local := getFlags(cmd).Local
	if local && global {
		return config.ScopeMerged, fmt.Errorf("--local and --global are mutually exclusive")
	}
	if global {
		return config.ScopeGlobal, nil
	}
	if local {
		return config.ScopeLocal, nil
	}
	return config.ScopeMerged, nil
}

// loadConfig loads CUE configuration for the given scope.
func loadConfig(scope config.Scope) (internalcue.LoadResult, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return internalcue.LoadResult{}, fmt.Errorf("resolving config paths: %w", err)
	}

	dirs := paths.ForScope(scope)

	if len(dirs) == 0 {
		switch scope {
		case config.ScopeGlobal:
			return internalcue.LoadResult{}, fmt.Errorf("no global configuration found at %s", paths.Global)
		case config.ScopeLocal:
			return internalcue.LoadResult{}, fmt.Errorf("no local configuration found at %s", paths.Local)
		default:
			return internalcue.LoadResult{}, fmt.Errorf("no configuration found (checked %s and %s)", paths.Global, paths.Local)
		}
	}

	loader := internalcue.NewLoader()
	result, err := loader.Load(dirs)
	if err != nil && errors.Is(err, internalcue.ErrNoCUEFiles) {
		switch scope {
		case config.ScopeGlobal:
			return result, fmt.Errorf("no global configuration found at %s (directory exists but contains no .cue files)", paths.Global)
		case config.ScopeLocal:
			return result, fmt.Errorf("no local configuration found at %s (directory exists but contains no .cue files)", paths.Local)
		default:
			return result, fmt.Errorf("no configuration found (checked %s and %s; directories exist but contain no .cue files)", paths.Global, paths.Local)
		}
	}
	return result, err
}

// printVerboseDump writes the full verbose dump for a DescribeResult.
func printVerboseDump(w io.Writer, r DescribeResult) {
	cat := r.Category
	label := tui.ColorDim.Sprint

	// Header
	_, _ = tui.CategoryColor(cat).Fprint(w, r.ItemType)
	_, _ = fmt.Fprintf(w, ": %s", r.Name)
	if r.Reason != "" {
		_, _ = fmt.Fprint(w, " ")
		_, _ = fmt.Fprint(w, tui.Annotate("%s", r.Reason))
	}
	_, _ = fmt.Fprintln(w)
	printSeparator(w)

	// Config source
	configSource := findConfigSource(r.CueKey, r.Name)
	if configSource != "" {
		_, _ = fmt.Fprintf(w, "%s %s %s\n",
			label("Config:"), configSource,
			tui.Annotate("%s", r.Name))
	}

	// Origin and cache
	origin := orchestration.ExtractOrigin(r.Value)
	if origin != "" {
		_, _ = fmt.Fprintf(w, "%s %s\n", label("Origin:"), origin)
		cacheDir := deriveCacheDir(origin)
		if cacheDir != "" {
			_, _ = fmt.Fprintf(w, "%s %s\n", label("Cache:"), cacheDir)
		}
	}

	// CUE Definition
	cueDef := formatCUEDefinition(r.Value)
	if cueDef != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintln(w, cueDef)
	}

	// File contents
	fields := orchestration.ExtractUTDFields(r.Value)
	if fields.File != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s %s\n", label("File:"), fields.File)

		resolvedPath, content, readErr := resolveDescribeFile(fields.File, origin)
		if resolvedPath != "" && resolvedPath != fields.File {
			_, _ = fmt.Fprintf(w, "%s %s\n", label("Path:"), resolvedPath)
		}

		if readErr != nil {
			_, _ = fmt.Fprintf(w, "[error: %s]\n", readErr)
		} else if content != "" {
			_, _ = fmt.Fprintln(w)
			_, _ = fmt.Fprint(w, content)
			if !strings.HasSuffix(content, "\n") {
				_, _ = fmt.Fprintln(w)
			}
		}
	}

	// Command
	if fields.Command != "" {
		cmd := fields.Command
		if r.ItemType == "Agent" {
			cmd = partialFillAgentCommand(cmd, r.Value, "")
		}
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "%s %s\n", label("Command:"), cmd)
	}

	printSeparator(w)
}

// findConfigSource determines which config file defines an item.
// This loads each config dir separately via LoadSingle rather than reusing the
// merged config because merged CUE values lose per-file position information.
func findConfigSource(cueKey, name string) string {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return ""
	}

	loader := internalcue.NewLoader()

	// Check local first (higher priority - overrides global)
	if paths.LocalExists {
		if v, err := loader.LoadSingle(paths.Local); err == nil {
			item := v.LookupPath(cue.ParsePath(cueKey)).LookupPath(cue.MakePath(cue.Str(name)))
			if item.Exists() {
				if pos := item.Pos(); pos.IsValid() {
					return pos.Filename()
				}
			}
		}
	}

	// Check global
	if paths.GlobalExists {
		if v, err := loader.LoadSingle(paths.Global); err == nil {
			item := v.LookupPath(cue.ParsePath(cueKey)).LookupPath(cue.MakePath(cue.Str(name)))
			if item.Exists() {
				if pos := item.Pos(); pos.IsValid() {
					return pos.Filename()
				}
			}
		}
	}

	return ""
}

// formatCUEDefinition formats a CUE value as CUE syntax.
func formatCUEDefinition(v cue.Value) string {
	syn := v.Syntax(
		cue.Concrete(false),
		cue.Definitions(true),
		cue.Hidden(true),
		cue.Optional(true),
	)

	b, err := cueformat.Node(syn)
	if err != nil {
		return ""
	}
	return string(b)
}

// resolveDescribeFile resolves a file reference and reads its contents.
// Returns the resolved path, content, and any error.
func resolveDescribeFile(filePath, origin string) (resolvedPath, content string, err error) {
	if filePath == "" {
		return "", "", nil
	}

	// @module/ paths
	if strings.HasPrefix(filePath, "@module/") {
		if origin == "" {
			return "", "", fmt.Errorf("@module/ path requires origin field: %s", filePath)
		}
		resolved, err := orchestration.ResolveModulePath(filePath, origin)
		if err != nil {
			return "", "", err
		}
		data, err := os.ReadFile(resolved)
		if err != nil {
			return resolved, "", err
		}
		return resolved, string(data), nil
	}

	// ~/ and other paths
	expanded, err := orchestration.ExpandFilePath(filePath)
	if err != nil {
		return "", "", err
	}

	data, readErr := os.ReadFile(expanded)
	if readErr != nil {
		return expanded, "", readErr
	}
	return expanded, string(data), nil
}

// partialFillAgentCommand substitutes the static {{.bin}} and {{.model}}
// placeholders in an agent command template with their resolved values.
// Runtime placeholders ({{.prompt}}, {{.role}}, {{.role_file}}, {{.datetime}})
// are left as-is since they are only known at execution time.
//
// modelOverride, when non-empty, replaces the agent's default_model — the
// caller is expected to have already resolved it (e.g. via
// resolver.resolveModelName for `get`) so that exact and substring matches
// against the models map have been applied. With an empty override, the
// agent's default_model is used. Both paths look the resolved key up in the
// models map; unknown keys pass through as the literal id.
func partialFillAgentCommand(command string, v cue.Value, modelOverride string) string {
	bin := ""
	if f := v.LookupPath(cue.ParsePath("bin")); f.Exists() {
		bin, _ = f.String()
	}

	model := modelOverride
	if model == "" {
		if dm := v.LookupPath(cue.ParsePath("default_model")); dm.Exists() {
			model, _ = dm.String()
		}
	}
	if model != "" {
		if models := v.LookupPath(cue.ParsePath("models")); models.Exists() {
			entry := models.LookupPath(cue.MakePath(cue.Str(model)))
			if entry.Exists() {
				// Simple string format: models: { sonnet: "model-id" }
				if s, err := entry.String(); err == nil {
					model = s
				} else if idVal := entry.LookupPath(cue.ParsePath("id")); idVal.Exists() {
					// Object format: models: { sonnet: { id: "model-id" } }
					if s, err := idVal.String(); err == nil {
						model = s
					}
				}
			}
		}
	}

	result := command
	if bin != "" {
		result = strings.ReplaceAll(result, "{{.bin}}", bin)
	}
	if model != "" {
		result = strings.ReplaceAll(result, "{{.model}}", model)
	}
	return result
}

// deriveCacheDir constructs the CUE cache directory for an origin.
func deriveCacheDir(origin string) string {
	cacheDir, err := orchestration.GetCUECacheDir()
	if err != nil {
		return ""
	}

	idx := strings.LastIndex(origin, "@")
	if idx == -1 {
		return ""
	}

	modulePath := origin[:idx]
	version := origin[idx:]
	return filepath.Join(cacheDir, "mod", "extract",
		filepath.Dir(modulePath),
		filepath.Base(modulePath)+version)
}

// capitalise returns s with the first rune converted to upper case.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
