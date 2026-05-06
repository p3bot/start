package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"cuelang.org/go/cue"
	"github.com/start-cli/start/internal/assets"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

// errNoAssets is returned by installAsset when no matching assets are found.
var errNoAssets = errors.New("no assets found")

// NOTE(design): This file shares registry client creation, index fetching, and config
// loading patterns with assets_list.go, assets_search.go, assets_update.go, and
// assets_index.go. This duplication is accepted - each command uses the results
// differently and a shared helper would couple them for modest line savings.

// addAssetsAddCommand adds the add subcommand to the assets command.
func addAssetsAddCommand(parent *cobra.Command) {
	addCmd := &cobra.Command{
		Use:     "add [query]...",
		Aliases: []string{"install"},
		Short:   "Install assets from registry",
		Long: `Install one or more assets from the CUE registry to your configuration.

Searches the registry index for matching assets. If multiple matches are found,
prompts for selection. Use a direct path (e.g., "golang/code-review") for exact match.

Multiple queries can be provided to install several assets at once.

By default, installs to global config (~/.config/start/).
Use --local to install to project config (./.start/).`,
		Args: cobra.MinimumNArgs(0),
		RunE: runAssetsAdd,
	}

	parent.AddCommand(addCmd)
}

// runAssetsAdd searches for and installs one or more assets.
func runAssetsAdd(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	prompted := false
	if len(args) == 0 {
		query, err := promptSearchQuery(cmd.OutOrStdout(), cmd.InOrStdin())
		if err != nil {
			return err
		}
		if query == "" {
			return nil
		}
		args = []string{query}
		prompted = true
	}

	// Validate all queries are at least 3 characters
	w := cmd.OutOrStdout()
	stdin := cmd.InOrStdin()
	var validated []string
	for _, q := range args {
		if len(q) < 3 {
			if !isTerminal(stdin) {
				return fmt.Errorf("query %q must be at least 3 characters", q)
			}
			_, _ = fmt.Fprintf(w, "Query %q must be at least 3 characters\n", q)
			input, err := promptSearchQuery(w, stdin)
			if err != nil {
				return err
			}
			if input == "" {
				continue
			}
			q = input
			prompted = true
		}
		validated = append(validated, q)
	}
	if len(validated) == 0 {
		return nil
	}
	args = validated

	ctx := context.Background()
	flags := getFlags(cmd)
	prog := tui.NewProgress(cmd.ErrOrStderr(), flags.Quiet)
	defer prog.Done()

	// Create registry client
	client, err := registry.NewClient()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	// Fetch index
	prog.Update("Fetching index...")
	index, indexVersion, err := client.FetchIndex(ctx, resolveAssetsIndexPath())
	if err != nil {
		return fmt.Errorf("fetching index: %w", err)
	}
	if err := cache.WriteIndex(indexVersion); err != nil {
		debugf(cmd.ErrOrStderr(), getFlags(cmd), dbgCache, "cache write failed: %v", err)
	}
	prog.Done()

	// Determine config path
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(flags.Local)
	scopeName := scopeString(flags.Local)

	// Load CUE config once for existence checks across all queries.
	// On error with no CUE files (fresh install), cfg is a zero-value cue.Value;
	// LookupPath on it returns non-existent, so AssetExists correctly returns false.
	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(configDir)
	if err != nil {
		if matches, _ := filepath.Glob(filepath.Join(configDir, "*.cue")); len(matches) > 0 {
			return fmt.Errorf("invalid config in %s:\n%s\nRun 'start doctor' to diagnose",
				configDir, internalcue.IdentifyBrokenFiles(matches))
		}
	}

	// Install each queried asset
	var errs []error
	for _, query := range args {
		if err := installAsset(ctx, cmd, prog, client, index, query, configDir, scopeName, flags, cfg); err != nil {
			if prompted && len(args) == 1 && errors.Is(err, errNoAssets) {
				_, _ = fmt.Fprintf(w, "No assets found matching %q\n", query)
				return nil
			}
			errs = append(errs, fmt.Errorf("%s: %w", query, err))
			if len(args) > 1 {
				_, _ = fmt.Fprintf(w, "Error installing %q: %v\n", query, err)
			}
		}
	}

	return errors.Join(errs...)
}

// installAsset searches for, selects, and installs a single asset.
func installAsset(ctx context.Context, cmd *cobra.Command, prog *tui.Progress, client *registry.Client, index *registry.Index, query, configDir, scopeName string, flags *Flags, cfg cue.Value) error {
	w := cmd.OutOrStdout()

	// Search for matching assets
	results, err := assets.SearchIndex(index, query, nil)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("%w matching %q", errNoAssets, query)
	}

	// Select asset(s)
	var selections []assets.SearchResult
	if len(results) == 1 {
		selections = results
	} else {
		var err error
		selections, err = promptAssetSelection(w, cmd.InOrStdin(), results, cfg)
		if err != nil {
			return err
		}
		if len(selections) == 0 {
			return nil
		}
	}

	// Install each selected asset
	var errs []error
	for _, selected := range selections {
		if err := installSingleAsset(ctx, w, prog, client, index, selected, configDir, scopeName, flags, cfg); err != nil {
			errs = append(errs, fmt.Errorf("%s/%s: %w", selected.Category, selected.Name, err))
			_, _ = fmt.Fprintf(w, "Error installing %s/%s: %v\n", selected.Category, selected.Name, err)
		}
	}

	return errors.Join(errs...)
}

// installSingleAsset checks and installs a single selected asset.
func installSingleAsset(ctx context.Context, w io.Writer, prog *tui.Progress, client *registry.Client, index *registry.Index, selected assets.SearchResult, configDir, scopeName string, flags *Flags, cfg cue.Value) error {
	// Check if already installed
	if assets.AssetExists(cfg, selected.Category, selected.Name) {
		origin := assets.GetInstalledOrigin(cfg, selected.Category, selected.Name)

		// Manually-added asset (no origin) — warn and proceed with install
		if origin == "" {
			if !flags.Quiet {
				printWarning(w, "replacing manually-added %s/%s with registry version",
					selected.Category, selected.Name)
			}
		} else {
			if !flags.Quiet {
				installedVer := assets.VersionFromOrigin(origin)
				latestVer := selected.Entry.Version
				outdated := latestVer != "" && installedVer != "" && semver.Compare(latestVer, installedVer) > 0

				if outdated {
					_, _ = fmt.Fprint(w, "○ ")
				} else {
					_, _ = tui.ColorSuccess.Fprint(w, "✓ ")
				}
				_, _ = tui.ColorDim.Fprint(w, "Already installed: ")
				_, _ = tui.CategoryColor(selected.Category).Fprint(w, selected.Category)
				_, _ = fmt.Fprintf(w, "/%s ", selected.Name)
				_, _ = tui.ColorCyan.Fprint(w, "(")
				if installedVer != "" {
					_, _ = tui.ColorDim.Fprint(w, installedVer)
				}
				if outdated {
					_, _ = fmt.Fprint(w, " ")
					_, _ = tui.ColorBlue.Fprint(w, "->")
					_, _ = fmt.Fprint(w, " ")
					_, _ = tui.ColorWarning.Fprint(w, latestVer)
				} else {
					_, _ = fmt.Fprint(w, " ")
					_, _ = tui.ColorBlue.Fprint(w, "->")
					_, _ = fmt.Fprint(w, " ")
					_, _ = tui.ColorDim.Fprint(w, "current")
				}
				_, _ = tui.ColorCyan.Fprintln(w, ")")
			}
			return nil
		}
	}

	// Install the asset
	prog.Update("Fetching asset...")
	version, err := assets.InstallAsset(ctx, client, index, selected, configDir)
	if err != nil {
		return err
	}
	prog.Done()

	if !flags.Quiet {
		configFile := map[string]string{
			"agents":   "agents.cue",
			"roles":    "roles.cue",
			"tasks":    "tasks.cue",
			"contexts": "contexts.cue",
		}[selected.Category]
		if configFile == "" {
			configFile = "settings.cue"
		}
		if version != "" {
			_, _ = fmt.Fprintf(w, "\nInstalled %s/%s@%s to %s config\n", selected.Category, selected.Name, version, scopeName)
		} else {
			_, _ = fmt.Fprintf(w, "\nInstalled %s/%s to %s config\n", selected.Category, selected.Name, scopeName)
		}
		_, _ = fmt.Fprintf(w, "Config: %s/%s\n", configDir, configFile)
	}

	return nil
}

// promptAssetSelection prompts the user to select one or more assets from multiple matches.
// Supports single numbers, CSV (1,3,5), ranges (1-3), "all", or name matching.
// Returns nil and nil if the user cancels (empty input).
func promptAssetSelection(w io.Writer, r io.Reader, results []assets.SearchResult, cfg cue.Value) ([]assets.SearchResult, error) {
	// Check if stdin is a TTY
	isTTY := isTerminal(r)

	if !isTTY {
		var names []string
		for _, res := range results {
			names = append(names, fmt.Sprintf("%s/%s", res.Category, res.Name))
		}
		return nil, fmt.Errorf(
			"multiple assets found: %s\nSpecify exact path or run interactively",
			strings.Join(names, ", "),
		)
	}

	_, _ = fmt.Fprintf(w, "\nFound %d matches:\n\n", len(results))

	for i, res := range results {
		marker := "  "
		if assets.AssetExists(cfg, res.Category, res.Name) {
			marker = tui.ColorInstalled.Sprint("★") + " "
		}
		_, _ = fmt.Fprintf(w, "  %s%d. ", marker, i+1)
		_, _ = tui.CategoryColor(res.Category).Fprint(w, res.Category)
		_, _ = fmt.Fprintf(w, "/%s ", res.Name)
		_, _ = tui.ColorDim.Fprintf(w, "- %s", res.Entry.Description)
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "CSV %s, range %s, or \"all\" supported\n",
		tui.Annotate("1,2,3"), tui.Annotate("1-3"))
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(results)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)

	if input == "" {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return nil, nil
	}
	if strings.ToLower(input) == "all" {
		return results, nil
	}

	// Try matching by name (single result)
	inputLower := strings.ToLower(input)
	for _, res := range results {
		fullPath := fmt.Sprintf("%s/%s", res.Category, res.Name)
		if strings.ToLower(res.Name) == inputLower || strings.ToLower(fullPath) == inputLower {
			return []assets.SearchResult{res}, nil
		}
	}

	// Parse as numbers, CSV, and/or ranges
	indices, err := parseSelectionInput(input, len(results))
	if err != nil {
		return nil, err
	}
	if len(indices) == 0 {
		_, _ = fmt.Fprintln(w, "Cancelled.")
		return nil, nil
	}
	selected := make([]assets.SearchResult, len(indices))
	for i, idx := range indices {
		selected[i] = results[idx]
	}
	return selected, nil
}
