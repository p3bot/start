package cli

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed help/agents.md
var agentsHelp string

//go:embed help/templates.md
var templatesHelp string

//go:embed help/config.md
var configHelp string

// addHelpCommand replaces Cobra's default help command with a custom one that
// adds agent-focused topic subcommands (agents, templates, config).
func addHelpCommand(root *cobra.Command) {
	helpCmd := &cobra.Command{
		Use:     "help [command]",
		Short:   "Help about any command or topic",
		GroupID: "utilities",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				_ = root.Help()
				return
			}
			target, _, err := root.Find(args)
			if err != nil || target == nil || target == root {
				fmt.Fprintf(cmd.OutOrStdout(), "Unknown help topic: %s\n", args[0])
				return
			}
			_ = target.Help()
		},
	}

	helpCmd.AddCommand(&cobra.Command{
		Use:   "agents",
		Short: "AI agent quick reference",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(agentsHelp)
		},
	})

	helpCmd.AddCommand(&cobra.Command{
		Use:   "templates",
		Short: "Template placeholder reference",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(templatesHelp)
		},
	})

	helpCmd.AddCommand(&cobra.Command{
		Use:   "config",
		Short: "Configuration structure reference",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Print(configHelp)
		},
	})

	root.SetHelpCommand(helpCmd)
	root.InitDefaultHelpCmd()
}
