package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

// addModulesBrowseCommand adds the browse subcommand to the modules command.
func addModulesBrowseCommand(parent *cobra.Command) {
	browseCmd := &cobra.Command{
		Use:     "browse",
		Aliases: []string{"open"},
		Short:   "Open library repository in browser",
		Long:    `Open the GitHub library repository in your default web browser for visual exploration.`,
		Args:    noArgsOrHelp,
		RunE:    runModulesBrowse,
	}

	parent.AddCommand(browseCmd)
}

// runModulesBrowse opens the library repository URL in the default browser.
func runModulesBrowse(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	url := DefaultLibraryRepoURL
	flags := getFlags(cmd)

	if !flags.Quiet {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Opening %s\n", url)
	}

	return openBrowser(url)
}

// openBrowser opens the specified URL in the default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Start()
}
