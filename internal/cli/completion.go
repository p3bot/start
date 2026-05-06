package cli

import (
	"github.com/spf13/cobra"
)

// addCompletionCommand adds the completion command to the parent command.
func addCompletionCommand(parent *cobra.Command) {
	completionCmd := &cobra.Command{
		Use:     "completion",
		Aliases: []string{"completions"},
		GroupID: "utilities",
		Short:   "Generate shell completion scripts",
		Long: `Generate shell completion scripts for bash, zsh, or fish.

Shell completion enables tab-completion for commands, subcommands, and flags.
Each subcommand outputs a completion script to stdout for the specified shell.`,
	}

	completionCmd.AddCommand(
		newBashCompletionCmd(),
		newZshCompletionCmd(),
		newFishCompletionCmd(),
	)

	parent.AddCommand(completionCmd)
}

func newBashCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		Long: `Generate the autocompletion script for bash.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

    source <(start completion bash)

To load completions for every new session, execute once:

Linux:

    start completion bash > /etc/bash_completion.d/start

macOS:

    start completion bash > $(brew --prefix)/etc/bash_completion.d/start

You will need to start a new shell for this setup to take effect.`,
		Args:              noArgsOrHelp,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shown, err := checkHelpArg(cmd, args); shown || err != nil {
				return err
			}
			return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
		},
	}
}

func newZshCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		Long: `Generate the autocompletion script for zsh.

If shell completion is not already enabled in your environment you will need
to enable it. You can execute the following once:

    echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

    source <(start completion zsh)

To load completions for every new session, execute once:

Linux:

    start completion zsh > "${fpath[1]}/_start"

macOS:

    start completion zsh > $(brew --prefix)/share/zsh/site-functions/_start

You will need to start a new shell for this setup to take effect.`,
		Args:              noArgsOrHelp,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shown, err := checkHelpArg(cmd, args); shown || err != nil {
				return err
			}
			return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
		},
	}
}

func newFishCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		Long: `Generate the autocompletion script for fish.

To load completions in your current shell session:

    start completion fish | source

To load completions for every new session, execute once:

    start completion fish > ~/.config/fish/completions/start.fish

You will need to start a new shell for this setup to take effect.`,
		Args:              noArgsOrHelp,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shown, err := checkHelpArg(cmd, args); shown || err != nil {
				return err
			}
			return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
		},
	}
}
