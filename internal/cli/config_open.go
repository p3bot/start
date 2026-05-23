package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
)

// openCategories is the ordered list of categories for the config open prompt.
// Plural names are used so tui.CategoryColor returns the correct colour for each.
var openCategories = []string{"agents", "roles", "contexts", "tasks", "settings"}

// addConfigOpenCommand registers the "config open [category]" subcommand.
func addConfigOpenCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "open [category]",
		Short: "Open a config file in $EDITOR",
		Long: `Open a configuration CUE file directly in $EDITOR.

Provide a category (agent, role, context, task, setting) or omit it to be
prompted interactively. Plural aliases (agents, roles, etc.) are accepted.

Use --local to target project-specific configuration (.start/).`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigOpen,
	}
	parent.AddCommand(cmd)
}

// runConfigOpen handles the "config open [category]" command.
func runConfigOpen(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	flags := getFlags(cmd)
	local := flags.Local

	category := ""
	if len(args) > 0 {
		category = args[0]
	}

	if category == "" {
		stdin := cmd.InOrStdin()
		if !isTerminal(stdin) {
			return fmt.Errorf("category required (or run interactively to be prompted)")
		}
		stdout := cmd.OutOrStdout()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Open:")
		var err error
		category, err = promptSelectCategory(stdout, stdin, openCategories)
		if err != nil || category == "" {
			return err
		}
	}

	path, err := resolveConfigOpenPath(local, category)
	if err != nil {
		return err
	}

	return openInEditor(path)
}

// resolveConfigOpenPath returns the absolute path to the CUE config file for
// the given category. Both singular and plural forms are accepted.
func resolveConfigOpenPath(local bool, category string) (string, error) {
	// Normalise plural to singular by stripping a trailing "s".
	singular := strings.TrimSuffix(strings.ToLower(category), "s")

	var filename string
	switch singular {
	case "agent":
		filename = "agents.cue"
	case "role":
		filename = "roles.cue"
	case "context":
		filename = "contexts.cue"
	case "task":
		filename = "tasks.cue"
	case "setting":
		filename = "settings.cue"
	default:
		return "", fmt.Errorf("unknown category %q: expected agent, role, context, task, or setting", category)
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return "", fmt.Errorf("resolving config paths: %w", err)
	}

	return filepath.Join(paths.Dir(local), filename), nil
}
