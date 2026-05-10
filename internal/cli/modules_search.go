package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/tui"
)

// NOTE(design): The shared registry-client + fetch + cache-write sequence used
// at the top of runModulesSearch is centralised in fetchIndex (modules.go).
// The post-fetch flow (modules.SearchIndex + output) is search-specific and
// not worth sharing. The collectInstalledScopes helper below repeats the
// config-loading shape used in modules_list.go and modules_update.go for
// installed-marking; it is kept inline because each command has
// command-specific empty-state UX.

// addModulesSearchCommand adds the search subcommand to the modules command.
func addModulesSearchCommand(parent *cobra.Command) {
	searchCmd := &cobra.Command{
		Use:     "search [query]...",
		Aliases: []string{"find"},
		Short:   "Search registry for modules",
		Long: `Search the module registry index by keyword.

Searches module names, descriptions, and tags. Multiple words are combined
with AND logic - all terms must match. Terms can be space-separated or
comma-separated. Total query must be at least 3 characters.
Terms support regex patterns (e.g. '^home', 'expert$', 'go.*review').
Results are grouped by type (agents, roles, contexts, tasks).

Use --tag to filter by tags. Tags can be used alone or combined with a query.

Use 'start search' to also include local and global config in results.`,
		Args: cobra.MinimumNArgs(0),
		RunE: runModulesSearch,
	}
	searchCmd.Flags().StringSlice("tag", nil, "Filter by tags (comma-separated)")
	searchCmd.Flags().Bool("json", false, "Output as JSON")

	parent.AddCommand(searchCmd)
}

// runModulesSearch searches the registry index for matching modules.
func runModulesSearch(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	query := strings.Join(args, " ")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	tagFlags, _ := cmd.Flags().GetStringSlice("tag")
	tags := modules.ParseSearchTerms(strings.Join(tagFlags, ","))

	terms := modules.ParseSearchPatterns(query)
	if err := modules.ValidateSearchQuery(terms, tags); err != nil {
		if jsonFlag {
			return err
		}
		w := cmd.OutOrStdout()
		stdin := cmd.InOrStdin()
		if !isTerminal(stdin) {
			return err
		}
		if query != "" {
			_, _ = fmt.Fprintln(w, "Query must be at least 3 characters")
		}
		input, promptErr := promptSearchQuery(w, stdin)
		if promptErr != nil {
			return promptErr
		}
		if input == "" {
			return nil
		}
		query = input
	}

	ctx := context.Background()
	flags := getFlags(cmd)

	prog := tui.NewProgress(cmd.ErrOrStderr(), flags.Quiet)
	defer prog.Done()

	index, _, err := fetchIndex(ctx, cmd, prog, "Fetching index...")
	if err != nil {
		return err
	}
	prog.Done()

	// Search index
	results, err := modules.SearchIndex(index, query, tags)
	if err != nil {
		return err
	}

	displayQuery := query
	if displayQuery == "" && len(tags) > 0 {
		displayQuery = "--tag " + strings.Join(tags, ",")
	}

	if len(results) == 0 {
		if jsonFlag {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[]")
			return nil
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No matches found for %q\n", displayQuery)
		return nil
	}

	if jsonFlag {
		if err := writeJSON(cmd.OutOrStdout(), results); err != nil {
			return fmt.Errorf("marshalling search results: %w", err)
		}
		return nil
	}

	// Collect installed module names for marking in output
	installed := collectInstalledNames()

	// Print results
	printSearchResults(cmd.OutOrStdout(), results, flags.Verbose, installed)

	return nil
}

// collectInstalledNames returns a set of "category/name" keys for installed modules.
func collectInstalledNames() map[string]bool {
	scopes := collectInstalledScopes()
	if scopes == nil {
		return nil
	}
	names := make(map[string]bool, len(scopes))
	for k := range scopes {
		names[k] = true
	}
	return names
}

// collectInstalledScopes returns a map of "category/name" to scope for installed modules.
func collectInstalledScopes() map[string]string {
	paths, err := config.ResolvePaths("")
	if err != nil || !paths.AnyExists() {
		return nil
	}

	dirs := paths.ForScope(config.ScopeMerged)
	loader := internalcue.NewLoader()
	cfg, err := loader.Load(dirs)
	if err != nil {
		return nil
	}

	var localCfg cue.Value
	if paths.LocalExists {
		if v, loadErr := loader.LoadSingle(paths.Local); loadErr == nil {
			localCfg = v
		}
	}

	installedModules := collectInstalledModules(cfg.Value, paths, localCfg)
	scopes := make(map[string]string, len(installedModules))
	for _, a := range installedModules {
		scopes[a.Category+"/"+a.Name] = a.Scope
	}
	return scopes
}

// printSearchResults prints search results grouped by category.
// installed is an optional set of "category/name" keys for marking installed modules.
func printSearchResults(w io.Writer, results []modules.SearchResult, verbose bool, installed map[string]bool) {
	_, _ = fmt.Fprintf(w, "\nFound %d matches:\n\n", len(results))

	// Group by category for display
	grouped := make(map[string][]modules.SearchResult)
	for _, r := range results {
		grouped[r.Category] = append(grouped[r.Category], r)
	}

	// Print in category order
	categories := []string{"agents", "roles", "contexts", "tasks"}
	for _, cat := range categories {
		catResults := grouped[cat]
		if len(catResults) == 0 {
			continue
		}

		_, _ = tui.CategoryColor(cat).Fprint(w, cat)
		_, _ = fmt.Fprintln(w, "/")
		for _, r := range catResults {
			marker := "  "
			if installed[r.Category+"/"+r.Name] {
				marker = tui.ColorInstalled.Sprint("★") + " "
			}

			if verbose {
				_, _ = fmt.Fprintf(w, "  %s%-25s %s\n", marker, r.Name, tui.ColorDim.Sprint(r.Entry.Description))
				_, _ = fmt.Fprintf(w, "      Module: %s\n", tui.ColorDim.Sprint(r.Entry.Module))
				if len(r.Entry.Tags) > 0 {
					_, _ = fmt.Fprintf(w, "      Tags: %s\n", tui.ColorDim.Sprint(strings.Join(r.Entry.Tags, ", ")))
				}
			} else {
				_, _ = fmt.Fprintf(w, "  %s%-25s %s\n", marker, r.Name, tui.ColorDim.Sprint(r.Entry.Description))
			}
		}
		_, _ = fmt.Fprintln(w)
	}
}
