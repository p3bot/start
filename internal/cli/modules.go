package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
)

// Library repository constants
const (
	// DefaultLibraryRepoURL is the default GitHub repository for browsing the library.
	DefaultLibraryRepoURL = "https://github.com/start-cli/library"
)

// fetchIndex creates a registry client, fetches the registry index, and writes
// the resolved version to the cache. The supplied progress reporter shows
// message while the network call is in flight. The cache write is best-effort:
// any error is logged at debug level only.
//
// Centralises the registry-client + fetch + cache-write trio shared by
// modules_install, modules_search, and modules_update. Each
// command's post-fetch logic stays at its call site because the four
// commands use the index differently and consolidating further would
// obscure call-site UX.
//
// modules_index uses Fetch + ResolveLatestVersion directly (it needs the
// source dir, not a parsed index), and modules_list fetches conditionally
// inside checkForUpdates with graceful failure — neither fits this helper.
func fetchIndex(ctx context.Context, cmd *cobra.Command, prog *tui.Progress, message string) (*registry.Index, *registry.Client, error) {
	client, err := registry.NewClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating registry client: %w", err)
	}
	prog.Update("%s", message)
	index, indexVersion, err := client.FetchIndex(ctx, resolveLibraryIndexPath())
	if err != nil {
		return nil, nil, fmt.Errorf("fetching index: %w", err)
	}
	if err := cache.WriteIndex(indexVersion); err != nil {
		debugf(cmd.ErrOrStderr(), getFlags(cmd), dbgCache, "cache write failed: %v", err)
	}
	return index, client, nil
}

// addModulesCommand adds the modules command group and its subcommands to the parent.
func addModulesCommand(parent *cobra.Command) {
	modulesCmd := &cobra.Command{
		Use:     "modules",
		Aliases: []string{"module"},
		GroupID: "commands",
		Short:   "Manage modules from the CUE registry",
		Long: `Manage modules (agents, roles, contexts, tasks) from the CUE Central Registry.

Modules are CUE packages that define reusable AI agent configurations.
Use these commands to discover, install, and update modules from the library.`,
		RunE: runModules,
	}

	modulesCmd.Flags().Bool("json", false, "Output as JSON")

	// Add subcommands
	addModulesBrowseCommand(modulesCmd)
	addModulesIndexCommand(modulesCmd)
	addModulesSearchCommand(modulesCmd)
	addModulesInstallCommand(modulesCmd)
	addModulesListCommand(modulesCmd)
	addModulesUpdateCommand(modulesCmd)
	addModulesValidateCommand(modulesCmd)

	parent.AddCommand(modulesCmd)
}

// runModules runs list by default, handles help subcommand.
func runModules(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) > 0 {
		return unknownCommandError("start modules", args[0])
	}
	return runModulesList(cmd, args)
}
