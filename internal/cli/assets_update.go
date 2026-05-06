package cli

import (
	"context"
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

// NOTE(design): This file shares registry client creation, index fetching, and config
// loading patterns with assets_add.go, assets_list.go, assets_search.go, and
// assets_index.go. This duplication is accepted - each command uses the results
// differently and a shared helper would couple them for modest line savings.

// UpdateResult tracks the result of an update operation.
type UpdateResult struct {
	Asset        InstalledAsset `json:"asset"`
	OldVersion   string         `json:"oldVersion,omitempty"`
	NewVersion   string         `json:"newVersion,omitempty"`
	Updated      bool           `json:"updated"`
	Error        error          `json:"-"`
	ErrorMessage string         `json:"error,omitempty"`
}

// addAssetsUpdateCommand adds the update subcommand to the assets command.
func addAssetsUpdateCommand(parent *cobra.Command) {
	updateCmd := &cobra.Command{
		Use:     "update [query]",
		Aliases: []string{"upgrade"},
		Short:   "Update installed assets",
		Long: `Update installed assets to their latest versions.

Without arguments, updates all installed assets.
With a query, updates only matching assets.

Use --dry-run to preview what would be updated without applying changes.
Use --force to re-fetch and update assets even when already at the latest version.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAssetsUpdate,
	}

	updateCmd.Flags().Bool("force", false, "Re-fetch even if already at latest version")
	updateCmd.Flags().Bool("json", false, "Output as JSON")

	parent.AddCommand(updateCmd)
}

// runAssetsUpdate updates installed assets.
func runAssetsUpdate(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	query := ""
	if len(args) > 0 {
		query = args[0]
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

	// Collect installed assets
	installed := collectInstalledAssets(cfg.Value, paths, localCfg)

	if len(installed) == 0 {
		if jsonFlag {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[]")
			return nil
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No assets installed from registry.")
		return nil
	}

	// Filter by query if provided
	if query != "" {
		var filtered []InstalledAsset
		queryLower := strings.ToLower(query)
		for _, a := range installed {
			if strings.Contains(strings.ToLower(a.Name), queryLower) ||
				strings.Contains(strings.ToLower(a.Category), queryLower) {
				filtered = append(filtered, a)
			}
		}
		installed = filtered

		if len(installed) == 0 {
			if jsonFlag {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "[]")
				return nil
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No installed assets matching %q\n", query)
			return nil
		}
	}

	// Create registry client
	client, err := registry.NewClient()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	// Fetch index for version comparison
	flags := getFlags(cmd)
	prog := tui.NewProgress(cmd.ErrOrStderr(), flags.Quiet)
	defer prog.Done()

	prog.Update("Checking for updates...")
	index, indexVersion, err := client.FetchIndex(ctx, resolveAssetsIndexPath())
	if err != nil {
		return fmt.Errorf("fetching index: %w", err)
	}
	if err := cache.WriteIndex(indexVersion); err != nil {
		debugf(cmd.ErrOrStderr(), getFlags(cmd), dbgCache, "cache write failed: %v", err)
	}
	prog.Done()

	// Check each asset for updates
	dryRun := getFlags(cmd).DryRun
	force, _ := cmd.Flags().GetBool("force")
	total := len(installed)
	var results []UpdateResult
	for i, asset := range installed {
		prog.Update("Updating %d/%d %s/%s...", i+1, total, asset.Category, asset.Name)
		result := checkAndUpdate(ctx, client, paths, index, asset, dryRun, force)
		results = append(results, result)
	}
	prog.Done()

	if jsonFlag {
		// Populate ErrorMessage for JSON serialisation
		for i := range results {
			if results[i].Error != nil {
				results[i].ErrorMessage = results[i].Error.Error()
			}
		}
		if err := writeJSON(cmd.OutOrStdout(), results); err != nil {
			return fmt.Errorf("marshalling update results: %w", err)
		}
		return nil
	}

	// Print results
	printUpdateResults(cmd.OutOrStdout(), results, dryRun)

	return nil
}

// checkAndUpdate checks for updates and optionally applies them.
func checkAndUpdate(ctx context.Context, client *registry.Client, paths config.Paths, index *registry.Index, asset InstalledAsset, dryRun, force bool) UpdateResult {
	result := UpdateResult{Asset: asset}

	// Find asset in index
	entry := findInIndex(index, asset.Category, asset.Name)
	if entry == nil {
		// Not in index, can't update
		return result
	}

	// Get current and latest versions
	result.OldVersion = asset.InstalledVer
	result.NewVersion = entry.Version

	// Check if update is needed (semver comparison detects upgrades only)
	needsUpdate := force || (entry.Version != "" && (asset.InstalledVer == "" || semver.Compare(entry.Version, asset.InstalledVer) > 0))

	if !needsUpdate {
		return result
	}

	if dryRun {
		result.Updated = true
		return result
	}

	if asset.ConfigFile == "" {
		result.Error = fmt.Errorf("no config file path for asset")
		return result
	}

	// Re-fetch the module to get latest version
	// First resolve @v0 to canonical version (e.g., @v0.0.1)
	modulePath := entry.Module
	if !strings.Contains(modulePath, "@") {
		modulePath += "@v0"
	}

	// Resolve to canonical version before fetching
	resolvedPath, err := client.ResolveLatestVersion(ctx, modulePath)
	if err != nil {
		result.Error = err
		return result
	}

	fetchResult, err := client.Fetch(ctx, resolvedPath)
	if err != nil {
		result.Error = err
		return result
	}

	// Extract the new content from fetched module
	searchResult := assets.SearchResult{
		Category: asset.Category,
		Name:     asset.Name,
		Entry:    *entry,
	}

	// For tasks, detect and install role dependencies before extracting content
	var roleName string
	if asset.Category == "tasks" && index != nil {
		configDir := filepath.Dir(asset.ConfigFile)
		roleName, err = assets.InstallRoleDependency(ctx, client, index, fetchResult.SourceDir, configDir)
		if err != nil {
			result.Error = fmt.Errorf("installing role dependency: %w", err)
			return result
		}
	}

	// Use resolved path with version for origin field (e.g., "github.com/.../task@v0.1.1")
	// This preserves full provenance information for future updates.
	assetContent, err := assets.ExtractAssetContent(fetchResult.SourceDir, searchResult, client.Registry(), resolvedPath, roleName)
	if err != nil {
		result.Error = fmt.Errorf("extracting asset content: %w", err)
		return result
	}

	// Update the config file with new content
	if err := assets.UpdateAssetInConfig(asset.ConfigFile, asset.Category, asset.Name, assetContent); err != nil {
		result.Error = fmt.Errorf("updating config: %w", err)
		return result
	}

	result.Updated = true
	return result
}

// printUpdateResults prints the results of the update operation.
func printUpdateResults(w io.Writer, results []UpdateResult, dryRun bool) {
	if dryRun {
		_, _ = fmt.Fprintln(w, "\nDry run - no changes applied:")
	} else {
		_, _ = fmt.Fprintln(w)
	}

	var updated, current, failed int

	for _, r := range results {
		name := r.Asset.Category + "/" + r.Asset.Name
		_, _ = fmt.Fprintf(w, "  %s ", name)

		if r.Error != nil {
			_, _ = tui.ColorCyan.Fprint(w, "(")
			_, _ = tui.ColorError.Fprintf(w, "error: %v", r.Error)
			_, _ = tui.ColorCyan.Fprintln(w, ")")
			failed++
		} else if r.Updated {
			_, _ = tui.ColorCyan.Fprint(w, "(")
			if r.OldVersion != "" {
				_, _ = tui.ColorDim.Fprint(w, r.OldVersion)
				_, _ = fmt.Fprint(w, " ")
			}
			_, _ = tui.ColorBlue.Fprint(w, "->")
			if r.NewVersion != "" {
				_, _ = fmt.Fprint(w, " ")
				_, _ = tui.ColorSuccess.Fprint(w, r.NewVersion)
			}
			_, _ = tui.ColorCyan.Fprintln(w, ")")
			updated++
		} else {
			_, _ = tui.ColorCyan.Fprint(w, "(")
			if r.OldVersion != "" {
				_, _ = tui.ColorDim.Fprint(w, r.OldVersion)
				_, _ = fmt.Fprint(w, " ")
				_, _ = tui.ColorBlue.Fprint(w, "->")
				_, _ = fmt.Fprint(w, " ")
			}
			_, _ = tui.ColorDim.Fprint(w, "current")
			_, _ = tui.ColorCyan.Fprintln(w, ")")
			current++
		}
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Updated: %d, Current: %d", updated, current)
	if failed > 0 {
		_, _ = fmt.Fprintf(w, ", Failed: %d", failed)
	}
	_, _ = fmt.Fprintln(w)
}
