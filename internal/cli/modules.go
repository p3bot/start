package cli

import (
	"github.com/spf13/cobra"
)

// Library repository constants
const (
	// DefaultLibraryRepoURL is the default GitHub repository for browsing the library.
	DefaultLibraryRepoURL = "https://github.com/start-cli/library"
)

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
	addModulesAddCommand(modulesCmd)
	addModulesListCommand(modulesCmd)
	addModulesInfoCommand(modulesCmd)
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
