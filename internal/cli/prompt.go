package cli

import (
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/orchestration"
)

func addPromptCommand(parent *cobra.Command) {
	promptCmd := &cobra.Command{
		Use:     "prompt [text|file ...]",
		GroupID: "workflow",
		Short:   "Launch AI agent with a custom prompt",
		Long: `Launch AI agent with a custom prompt and only required contexts.

Accepts any number of arguments. Each argument is independently treated as
inline text or a file path (starting with ./, /, ~, or ~/); file paths are read
and inline text is used verbatim. Resolved segments are joined with exactly one
blank line between them.

If no argument is given and stdin is piped, the piped content is used as the
prompt text. Default contexts are excluded to keep the prompt focused.
Use -c default to include contexts configured with default: true.`,
		Args: cobra.ArbitraryArgs,
		RunE: runPrompt,
	}
	parent.AddCommand(promptCmd)
}

func runPrompt(cmd *cobra.Command, args []string) error {
	customText := ""
	if len(args) > 0 {
		composed, err := orchestration.ComposeSegments(args, "prompt file")
		if err != nil {
			return err
		}
		customText = composed
	} else {
		stdin := cmd.InOrStdin()
		pipedText, piped, err := readPipedStdin(stdin)
		if err != nil {
			return err
		}
		if piped {
			// Empty piped stdin is accepted: proceeds with role + required
			// contexts only, like `start prompt` with no argument and no pipe.
			customText = pipedText
		} else {
			interactive, err := promptText(cmd.OutOrStdout(), stdin, "Prompt text", "")
			if err != nil {
				return err
			}
			if interactive == "" {
				return nil
			}
			customText = interactive
		}
	}

	flags := getFlags(cmd)

	selection := orchestration.ContextSelection{
		IncludeRequired: true,
		IncludeDefaults: false,
		Tags:            flags.Context,
	}

	return executeStart(cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin(), flags, selection, customText)
}
