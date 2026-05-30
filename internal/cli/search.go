package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/tui"
)

// searchSection groups search results under a labelled section.
type searchSection struct {
	Label         string                 `json:"label"`
	Path          string                 `json:"path,omitempty"`
	Results       []modules.SearchResult `json:"results"`
	ShowInstalled bool                   `json:"-"` // Only true for registry section; display-only
}

// addSearchCommand adds the top-level search command.
func addSearchCommand(parent *cobra.Command) {
	searchCmd := &cobra.Command{
		Use:     "search [query]...",
		Aliases: []string{"find"},
		GroupID: "modules",
		Short:   "Search configs and registry for modules",
		Long: `Search local config, global config, and the module registry by keyword.

Searches module names, descriptions, and tags. Multiple words are combined
with AND logic - all terms must match. Terms can be space-separated or
comma-separated. Total query must be at least 3 characters.
Terms support regex patterns (e.g. '^home', 'expert$', 'go.*review').
Results are grouped by source (local, global, registry) and category.

Use --tag to filter by tags. Tags can be used alone or combined with a query.`,
		Args: cobra.MinimumNArgs(0),
		RunE: runSearch,
	}
	searchCmd.Flags().StringSlice("tag", nil, "Filter by tags (comma-separated)")
	searchCmd.Flags().Bool("json", false, "Output as JSON")

	parent.AddCommand(searchCmd)
}

// runSearch searches local config, global config, and the registry.
func runSearch(cmd *cobra.Command, args []string) error {
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
			fmt.Fprintln(w, "Query must be at least 3 characters")
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

	// Validate regex patterns before searching
	if len(terms) > 0 {
		if _, err := modules.CompileSearchTerms(terms); err != nil {
			return err
		}
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

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

	// Search local config
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

	// Search global config
	if paths.GlobalExists {
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

	// Search registry (graceful fallback if unavailable)
	var registryErr error
	ctx := context.Background()
	client, err := getProvider(cmd)()
	if err != nil {
		registryErr = err
	} else {
		index, indexVersion, err := client.FetchIndex(ctx, resolveLibraryIndexPath())
		if err != nil {
			registryErr = err
		} else {
			if err := cache.WriteIndex(indexVersion); err != nil {
				debugf(cmd.ErrOrStderr(), getFlags(cmd), dbgCache, "cache write failed: %v", err)
			}
			results, err := modules.SearchIndex(index, query, tags)
			if err != nil {
				return err
			}
			if len(results) > 0 {
				sections = append(sections, searchSection{
					Label:         "registry",
					Results:       results,
					ShowInstalled: true,
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

	if len(sections) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No matches found for %q\n", displayQuery)
		if registryErr != nil {
			printWarning(cmd.ErrOrStderr(), "registry unavailable: %v", registryErr)
		}
		return nil
	}

	fmt.Fprintln(cmd.OutOrStdout())
	installed := collectInstalledNames()
	flags := getFlags(cmd)
	printSearchSections(cmd.OutOrStdout(), sections, flags.Verbose, installed)

	if registryErr != nil {
		fmt.Fprintln(cmd.OutOrStdout())
		printWarning(cmd.ErrOrStderr(), "registry unavailable: %v", registryErr)
	}

	return nil
}

// printSearchSections prints search results grouped by section and category.
func printSearchSections(w io.Writer, sections []searchSection, verbose bool, installed map[string]bool) {
	for i, section := range sections {
		if len(section.Results) == 0 {
			continue
		}

		if i > 0 {
			fmt.Fprintln(w)
		}
		if section.Path != "" {
			fmt.Fprintf(w, "%s %s\n", section.Label, tui.Annotate("%s", section.Path))
		} else {
			fmt.Fprintln(w, section.Label)
		}

		// Group results by category
		grouped := make(map[string][]modules.SearchResult)
		for _, r := range section.Results {
			grouped[r.Category] = append(grouped[r.Category], r)
		}

		// Print in category order
		categories := []string{"agents", "roles", "contexts", "tasks"}
		firstCat := true
		for _, cat := range categories {
			catResults := grouped[cat]
			if len(catResults) == 0 {
				continue
			}

			if !firstCat {
				fmt.Fprintln(w)
			}
			firstCat = false

			fmt.Fprint(w, "  ")
			tui.CategoryColor(cat).Fprint(w, cat)
			fmt.Fprintln(w, "/")

			for _, r := range catResults {
				marker := "  "
				if section.ShowInstalled && installed[r.Category+"/"+r.Name] {
					marker = tui.ColorInstalled.Sprint("★") + " "
				}

				fmt.Fprintf(w, "    %s%-25s %s\n", marker, r.Name, tui.ColorDim.Sprint(r.Entry.Description))
				if verbose {
					if r.Entry.Module != "" {
						fmt.Fprintf(w, "      Module: %s\n", tui.ColorDim.Sprint(r.Entry.Module))
					}
					if len(r.Entry.Tags) > 0 {
						fmt.Fprintf(w, "      Tags: %s\n", tui.ColorDim.Sprint(strings.Join(r.Entry.Tags, ", ")))
					}
				}
			}
		}
	}
}

// collectInstalledNames returns a set of "category/name" keys for installed modules.
func collectInstalledNames() map[string]bool {
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
	names := make(map[string]bool, len(installedModules))
	for _, a := range installedModules {
		names[a.Category+"/"+a.Name] = true
	}
	return names
}

// shortenHome replaces the home directory prefix with ~.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
