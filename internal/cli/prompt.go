package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/orchestration"
)

// addPromptCommand adds the prompt command to the parent command.
func addPromptCommand(parent *cobra.Command) {
	promptCmd := &cobra.Command{
		Use:     "prompt [text]",
		GroupID: "workflow",
		Short:   "Launch AI agent with a custom prompt",
		Long: `Launch AI agent with a custom prompt and only required contexts.

The argument can be inline text or a file path (starting with ./, /, or ~).
If no argument is given and stdin is piped, the piped content is used as the
prompt text. Default contexts are excluded to keep the prompt focused.
Use -c default to include contexts configured with default: true.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runPrompt,
	}
	parent.AddCommand(promptCmd)
}

// runPrompt executes the prompt command.
func runPrompt(cmd *cobra.Command, args []string) error {
	customText := ""
	if len(args) > 0 {
		arg := args[0]
		if orchestration.IsFilePath(arg) {
			content, err := orchestration.ReadFilePath(arg)
			if err != nil {
				return fmt.Errorf("reading prompt file %q: %w", arg, err)
			}
			customText = content
		} else {
			customText = arg
		}
	} else {
		stdin := cmd.InOrStdin()
		pipedText, piped, err := readPipedStdin(stdin)
		if err != nil {
			return err
		}
		if piped {
			// `start prompt` accepts empty piped stdin: execution proceeds
			// with role + required contexts only, mirroring `start prompt`
			// invoked with no argument and no pipe.
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
