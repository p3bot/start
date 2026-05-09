package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/modules"
)

// addConfigSearchCommand adds the search subcommand to the config command group.
func addConfigSearchCommand(parent *cobra.Command) {
	searchCmd := &cobra.Command{
		Use:     "search [query]...",
		Aliases: []string{"find"},
		Short:   "Search installed config for modules",
		Long: `Search local and global config for installed modules by keyword.

Searches module names, descriptions, and tags. Multiple words are combined
with AND logic - all terms must match. Terms can be space-separated or
comma-separated. Total query must be at least 3 characters.
Terms support regex patterns (e.g. '^home', 'expert$', 'go.*review').
Results are grouped by scope (local, global) and category.

Use --local to search only project-local config (./.start/).
Use --tag to filter by tags. Tags can be used alone or combined with a query.

Use 'start search' to also include the module registry in results.`,
		Args: cobra.MinimumNArgs(0),
		RunE: runConfigSearch,
	}
	searchCmd.Flags().StringSlice("tag", nil, "Filter by tags (comma-separated)")
	searchCmd.Flags().Bool("json", false, "Output as JSON")

	parent.AddCommand(searchCmd)
}

// runConfigSearch searches local and global config, excluding the registry.
func runConfigSearch(cmd *cobra.Command, args []string) error {
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
		terms = modules.ParseSearchPatterns(query)
	}

	if len(terms) > 0 {
		if _, err := modules.CompileSearchTerms(terms); err != nil {
			return err
		}
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	flags := getFlags(cmd)
	loader := internalcue.NewLoader()
	categories := []struct {
		cueKey   string
		category string
	}{
		{internalcue.KeyAgents, "agents"},
		{internalcue.KeyRoles, "roles"},
		{internalcue.KeyContexts, "contexts"},
		{internalcue.KeyTasks, "tasks"},
	}

	var sections []searchSection
	stderr := cmd.ErrOrStderr()

	// Search local config (.start/)
	if paths.LocalExists {
		cfg, err := loader.LoadSingle(paths.Local)
		if err != nil && !errors.Is(err, internalcue.ErrNoCUEFiles) {
			printWarning(stderr, "failed to load local config: %s", err)
		} else if err == nil {
			var results []modules.SearchResult
			for _, cat := range categories {
				catResults, err := modules.SearchInstalledConfig(cfg, cat.cueKey, cat.category, query, tags)
				if err != nil {
					return err
				}
				results = append(results, catResults...)
			}
			if len(results) > 0 {
				sections = append(sections, searchSection{
					Label:   "local",
					Path:    "./.start",
					Results: results,
				})
			}
		}
	}

	// Search global config unless --local flag is set
	if !flags.Local && paths.GlobalExists {
		cfg, err := loader.LoadSingle(paths.Global)
		if err != nil && !errors.Is(err, internalcue.ErrNoCUEFiles) {
			printWarning(stderr, "failed to load global config: %s", err)
		} else if err == nil {
			var results []modules.SearchResult
			for _, cat := range categories {
				catResults, err := modules.SearchInstalledConfig(cfg, cat.cueKey, cat.category, query, tags)
				if err != nil {
					return err
				}
				results = append(results, catResults...)
			}
			if len(results) > 0 {
				sections = append(sections, searchSection{
					Label:   "global",
					Path:    shortenHome(paths.Global),
					Results: results,
				})
			}
		}
	}

	if jsonFlag {
		if sections == nil {
			sections = []searchSection{}
		}
		if err := writeJSON(cmd.OutOrStdout(), sections); err != nil {
			return fmt.Errorf("marshalling search results: %w", err)
		}
		return nil
	}

	displayQuery := query
	if displayQuery == "" && len(tags) > 0 {
		displayQuery = "--tag " + strings.Join(tags, ",")
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	if len(sections) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No matches found for %q\n", displayQuery)
		return nil
	}

	printSearchSections(cmd.OutOrStdout(), sections, flags.Verbose, nil)
	return nil
}
