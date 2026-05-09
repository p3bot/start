package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/assets"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
	"golang.org/x/mod/semver"
)

// NOTE(design): This file shares registry client creation, index fetching, and config
// loading patterns with assets_add.go, assets_search.go, assets_update.go, and
// assets_index.go. This duplication is accepted - each command uses the results
// differently and a shared helper would couple them for modest line savings.

// InstalledAsset represents an installed asset with version info.
type InstalledAsset struct {
	Category     string   `json:"category"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Models       []string `json:"models,omitempty"`
	InstalledVer string   `json:"version,omitempty"`
	LatestVer    string   `json:"latestVersion,omitempty"`
	UpdateAvail  bool     `json:"updateAvailable,omitempty"`
	Scope        string   `json:"scope"`
	Origin       string   `json:"origin"`
	ConfigFile   string   `json:"configFile"`
}

// addAssetsListCommand adds the list subcommand to the assets command.
func addAssetsListCommand(parent *cobra.Command) {
	listCmd := &cobra.Command{
		Use:     "list [category]",
		Aliases: []string{"ls"},
		Short:   "List installed assets",
		Long: `List installed registry assets with update status.

Shows all assets installed via the registry with their current version
and whether updates are available.

Optionally filter by category: agents, roles, contexts, or tasks.

Use --json to output machine-readable JSON.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAssetsList,
	}

	listCmd.Flags().Bool("json", false, "Output as JSON")

	parent.AddCommand(listCmd)
}

// runAssetsList lists installed assets with update status.
func runAssetsList(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	// Validate category arg before any I/O
	var category string
	if len(args) > 0 {
		singular := normalizeCategoryArg(args[0])
		if singular == "" {
			return fmt.Errorf("unknown category %q: expected agents, roles, contexts, or tasks", args[0])
		}
		category = singular + "s"
	}

	jsonFlag, _ := cmd.Flags().GetBool("json")
	ctx := context.Background()

	// Load configuration
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	if !paths.AnyExists() {
		if jsonFlag {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[]")
			return nil
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No configuration found. Run 'start' to set up.")
		return nil
	}

	// Load merged config
	dirs := paths.ForScope(config.ScopeMerged)
	loader := internalcue.NewLoader()
	cfg, err := loader.Load(dirs)
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	// Load local config separately for scope detection
	var localCfg cue.Value
	if paths.LocalExists {
		if v, loadErr := loader.LoadSingle(paths.Local); loadErr == nil {
			localCfg = v
		}
	}

	// Collect installed assets from config
	installed := collectInstalledAssets(cfg.Value, paths, localCfg)

	if len(installed) == 0 {
		if jsonFlag {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[]")
			return nil
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No assets installed from registry.")
		return nil
	}

	// Filter by category if specified
	if category != "" {
		var filtered []InstalledAsset
		for _, a := range installed {
			if a.Category == category {
				filtered = append(filtered, a)
			}
		}
		installed = filtered

		if len(installed) == 0 {
			if jsonFlag {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[]")
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No %s installed from registry.\n", category)
			return nil
		}
	}

	// Check for updates if verbose
	flags := getFlags(cmd)
	if flags.Verbose {
		client, err := registry.NewClient()
		if err == nil {
			prog := tui.NewProgress(cmd.ErrOrStderr(), flags.Quiet)
			prog.Update("Checking for updates...")
			checkForUpdates(ctx, client, installed, resolveAssetsIndexPath())
			prog.Done()
		}
	}

	if jsonFlag {
		if err := writeJSON(cmd.OutOrStdout(), installed); err != nil {
			return fmt.Errorf("marshalling assets: %w", err)
		}
		return nil
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	printInstalledAssets(cmd.OutOrStdout(), installed, flags.Verbose)

	return nil
}

// collectInstalledAssets extracts installed assets from the config.
func collectInstalledAssets(v cue.Value, paths config.Paths, localCfg cue.Value) []InstalledAsset {
	var installed []InstalledAsset

	categories := []string{"agents", "roles", "contexts", "tasks"}
	for _, cat := range categories {
		catVal := v.LookupPath(cue.ParsePath(cat))
		if !catVal.Exists() {
			continue
		}

		iter, err := catVal.Fields()
		if err != nil {
			continue
		}

		for iter.Next() {
			name := iter.Selector().Unquoted()
			assetVal := iter.Value()

			// Extract origin field (registry provenance)
			var origin string
			if originVal := assetVal.LookupPath(cue.ParsePath("origin")); originVal.Exists() {
				origin, _ = originVal.String()
			}

			// Only include assets with origin (from registry)
			if origin == "" {
				continue
			}

			installedVer := assets.VersionFromOrigin(origin)

			// Extract description
			var description string
			if descVal := assetVal.LookupPath(cue.ParsePath("description")); descVal.Exists() {
				description, _ = descVal.String()
			}

			// Extract tags
			var tags []string
			if tagsVal := assetVal.LookupPath(cue.ParsePath("tags")); tagsVal.Exists() {
				tagIter, tagErr := tagsVal.List()
				if tagErr == nil {
					for tagIter.Next() {
						if s, sErr := tagIter.Value().String(); sErr == nil {
							tags = append(tags, s)
						}
					}
				}
			}

			// Extract models (agents only)
			var models []string
			if cat == "agents" {
				if modelsVal := assetVal.LookupPath(cue.ParsePath("models")); modelsVal.Exists() {
					modIter, modErr := modelsVal.List()
					if modErr == nil {
						for modIter.Next() {
							if s, sErr := modIter.Value().String(); sErr == nil {
								models = append(models, s)
							}
						}
					}
				}
			}

			scope, configFile := determineScopeAndFile(localCfg, paths, cat, name)
			asset := InstalledAsset{
				Category:     cat,
				Name:         name,
				Description:  description,
				Tags:         tags,
				Models:       models,
				InstalledVer: installedVer,
				Scope:        scope,
				Origin:       origin,
				ConfigFile:   configFile,
			}
			installed = append(installed, asset)
		}
	}

	// Sort by category then name
	sort.Slice(installed, func(i, j int) bool {
		if installed[i].Category != installed[j].Category {
			return assets.CategoryOrder(installed[i].Category) < assets.CategoryOrder(installed[j].Category)
		}
		return installed[i].Name < installed[j].Name
	})

	return installed
}

// determineScopeAndFile determines whether an asset is from global or local config
// and returns the path to the config file.
func determineScopeAndFile(localCfg cue.Value, paths config.Paths, category, name string) (scope, configFile string) {
	configFileName, ok := internalcue.ConfigFiles[category]
	if !ok {
		configFileName = internalcue.ConfigFiles[internalcue.KeySettings]
	}

	// Check local first (takes precedence)
	if paths.LocalExists && assets.AssetExists(localCfg, category, name) {
		return "local", filepath.Join(paths.Local, configFileName)
	}

	// Default to global. Assets from collectInstalledAssets came from CUE evaluation
	// of these same files, so this fallback is for informational display purposes.
	return "global", filepath.Join(paths.Global, configFileName)
}

// checkForUpdates checks registry for available updates.
func checkForUpdates(ctx context.Context, client *registry.Client, installed []InstalledAsset, indexPath string) {
	// Fetch index for version info
	index, indexVersion, err := client.FetchIndex(ctx, indexPath)
	if err != nil {
		return
	}
	_ = cache.WriteIndex(indexVersion)

	for i := range installed {
		entry := findInIndex(index, installed[i].Category, installed[i].Name)
		if entry != nil && entry.Version != "" {
			installed[i].LatestVer = entry.Version
			installed[i].UpdateAvail = semver.Compare(entry.Version, installed[i].InstalledVer) > 0
		}
	}
}

// findInIndex looks up an asset in the index.
func findInIndex(index *registry.Index, category, name string) *registry.IndexEntry {
	var entries map[string]registry.IndexEntry

	switch category {
	case "agents":
		entries = index.Agents
	case "roles":
		entries = index.Roles
	case "contexts":
		entries = index.Contexts
	case "tasks":
		entries = index.Tasks
	}

	if entry, ok := entries[name]; ok {
		return &entry
	}
	return nil
}

// printInstalledAssets prints the list of installed assets.
func printInstalledAssets(w io.Writer, installed []InstalledAsset, verbose bool) {
	_, _ = fmt.Fprintln(w, "Installed assets:")
	_, _ = fmt.Fprintln(w)

	// Group by category
	grouped := make(map[string][]InstalledAsset)
	for _, a := range installed {
		grouped[a.Category] = append(grouped[a.Category], a)
	}

	categories := []string{"agents", "roles", "contexts", "tasks"}
	for _, cat := range categories {
		assets := grouped[cat]
		if len(assets) == 0 {
			continue
		}

		_, _ = tui.CategoryColor(cat).Fprint(w, cat)
		_, _ = fmt.Fprintln(w, "/")
		for _, a := range assets {
			if verbose && a.LatestVer != "" {
				_, _ = fmt.Fprintf(w, "  %-25s ", a.Name)
				if a.UpdateAvail {
					_, _ = fmt.Fprint(w, tui.Annotate("update available: %s", a.LatestVer))
				} else {
					_, _ = fmt.Fprint(w, tui.Annotate("latest"))
				}
				_, _ = fmt.Fprintln(w)
			} else {
				scopeIndicator := ""
				if verbose {
					scopeIndicator = fmt.Sprintf(" [%s]", a.Scope)
				}
				if a.InstalledVer != "" {
					_, _ = fmt.Fprintf(w, "  %-25s %s%s\n", a.Name, tui.Annotate("%s", a.InstalledVer), scopeIndicator)
				} else {
					_, _ = fmt.Fprintf(w, "  %s%s\n", a.Name, scopeIndicator)
				}
			}
		}
		_, _ = fmt.Fprintln(w)
	}
}
