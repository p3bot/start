package cli

import (
	"github.com/spf13/cobra"
)

// Asset repository constants
const (
	// DefaultAssetRepoURL is the default GitHub repository for browsing assets.
	DefaultAssetRepoURL = "https://github.com/grantcarthew/start-assets"
)

// addAssetsCommand adds the assets command group and its subcommands to the parent.
func addAssetsCommand(parent *cobra.Command) {
	assetsCmd := &cobra.Command{
		Use:     "assets",
		Aliases: []string{"asset"},
		GroupID: "commands",
		Short:   "Manage assets from the CUE registry",
		Long: `Manage assets (agents, roles, contexts, tasks) from the CUE Central Registry.

Assets are CUE modules that define reusable AI agent configurations.
Use these commands to discover, install, and update assets.`,
		RunE: runAssets,
	}

	assetsCmd.Flags().Bool("json", false, "Output as JSON")

	// Add subcommands
	addAssetsBrowseCommand(assetsCmd)
	addAssetsIndexCommand(assetsCmd)
	addAssetsSearchCommand(assetsCmd)
	addAssetsAddCommand(assetsCmd)
	addAssetsListCommand(assetsCmd)
	addAssetsInfoCommand(assetsCmd)
	addAssetsUpdateCommand(assetsCmd)
	addAssetsValidateCommand(assetsCmd)

	parent.AddCommand(assetsCmd)
}

// runAssets runs list by default, handles help subcommand.
func runAssets(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) > 0 {
		return unknownCommandError("start assets", args[0])
	}
	return runAssetsList(cmd, args)
}
