package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

// addConfigRemoveCommand adds the "config remove [query]" command.
func addConfigRemoveCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "remove [query]",
		Aliases: []string{"rm", "delete"},
		Short:   "Remove a config item",
		Long: `Remove an agent, role, context, or task from configuration.

Search by name across all categories. If multiple items match, a menu is presented.
With no argument, prompts interactively for category and item.

Use --yes / -y to skip the confirmation prompt.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigRemove,
	}
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompt")
	parent.AddCommand(cmd)
}

// runConfigRemove is the handler for "config remove [query]".
func runConfigRemove(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	local := getFlags(cmd).Local
	skipConfirm, _ := cmd.Flags().GetBool("yes")

	if len(args) == 0 {
		if !isTerminal(stdin) {
			return fmt.Errorf("interactive remove requires a terminal")
		}
		return runConfigRemoveInteractive(stdin, stdout, local, skipConfirm, getFlags(cmd).Quiet)
	}

	fmt.Fprintln(stdout)
	query := args[0]
	matches, err := searchAllConfigCategories(query, config.ScopeFromLocal(local))
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return fmt.Errorf("%q not found", query)
	}

	var toRemove []configMatch
	if len(matches) == 1 {
		toRemove = matches
	} else if skipConfirm {
		// --yes with multiple matches: remove all
		toRemove = matches
	} else {
		if !isTerminal(stdin) {
			return fmt.Errorf("--yes flag required in non-interactive mode for ambiguous query %q", query)
		}
		selected, err := promptSelectConfigMatchesFromList(stdout, stdin, query, matches)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			return nil // user cancelled
		}
		toRemove = selected
	}

	if !skipConfirm {
		if !isTerminal(stdin) {
			return fmt.Errorf("--yes flag required in non-interactive mode")
		}
		confirmed, err := confirmConfigRemoval(stdout, stdin, toRemove, local)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	flags := getFlags(cmd)
	for _, m := range toRemove {
		if err := removeConfigItem(m, local); err != nil {
			return fmt.Errorf("removing %s %q: %w", m.Category, m.Name, err)
		}
		if !flags.Quiet {
			fmt.Fprintf(stdout, "Removed %s %q\n", m.Category, m.Name)
		}
	}

	return nil
}

// runConfigRemoveInteractive prompts for category, item(s), confirmation, then removes.
func runConfigRemoveInteractive(stdin io.Reader, stdout io.Writer, local bool, skipConfirm bool, quiet bool) error {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Remove:")
	category, err := promptSelectCategory(stdout, stdin, allConfigCategories)
	if err != nil || category == "" {
		return err
	}

	names, err := loadNamesForCategory(category, config.ScopeFromLocal(local))
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintf(stdout, "No %s configured.\n", category)
		return nil
	}

	singular := strings.TrimSuffix(category, "s")
	fmt.Fprintln(stdout)
	selectedNames, err := promptSelectFromList(stdout, stdin, singular, "", names)
	if err != nil || selectedNames == nil {
		return err
	}

	var toRemove []configMatch
	for _, name := range selectedNames {
		toRemove = append(toRemove, configMatch{Name: name, Category: singular})
	}

	if !skipConfirm {
		confirmed, err := confirmConfigRemoval(stdout, stdin, toRemove, local)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}

	for _, m := range toRemove {
		if err := removeConfigItem(m, local); err != nil {
			return fmt.Errorf("removing %s %q: %w", m.Category, m.Name, err)
		}
		if !quiet {
			fmt.Fprintf(stdout, "Removed %s %q\n", m.Category, m.Name)
		}
	}

	return nil
}

// confirmConfigRemoval prompts the user to confirm removal of one or more items.
// Returns false (without error) when the user declines.
func confirmConfigRemoval(w io.Writer, r io.Reader, items []configMatch, local bool) (bool, error) {
	scope := scopeString(local)
	if len(items) == 1 {
		m := items[0]
		fmt.Fprintf(w, "Remove %s %q from %s config? %s ", m.Category, m.Name, scope, tui.Bracket("y/N"))
	} else {
		fmt.Fprintf(w, "Remove the following items from %s config?\n", scope)
		for _, m := range items {
			fmt.Fprintf(w, "  - %s %s\n", m.Category, m.Name)
		}
		fmt.Fprintf(w, "%s ", tui.Bracket("y/N"))
	}

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))
	if input != "y" && input != "yes" {
		fmt.Fprintln(w, "Cancelled.")
		return false, nil
	}
	return true, nil
}

// removeConfigItem removes a single named item from the appropriate config category file.
func removeConfigItem(m configMatch, local bool) error {
	switch m.Category {
	case "agent":
		return removeAgent(m.Name, local)
	case "role":
		return removeRole(m.Name, local)
	case "context":
		return removeContext(m.Name, local)
	case "task":
		return removeTask(m.Name, local)
	}
	return fmt.Errorf("unknown category %q", m.Category)
}

// removeAgent removes an agent from the config directory.
func removeAgent(name string, local bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}
	configDir := paths.Dir(local)

	agents, _, err := loadAgentsFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading agents: %w", err)
	}

	delete(agents, name)

	agentPath := filepath.Join(configDir, "agents.cue")
	return writeAgentsFile(agentPath, agents)
}

// removeRole removes a role from the config directory (preserving order of remaining roles).
func removeRole(name string, local bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}
	configDir := paths.Dir(local)

	roles, order, err := loadRolesFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading roles: %w", err)
	}

	delete(roles, name)
	newOrder := make([]string, 0, len(order))
	for _, n := range order {
		if n != name {
			newOrder = append(newOrder, n)
		}
	}

	rolePath := filepath.Join(configDir, "roles.cue")
	return writeRolesFile(rolePath, roles, newOrder)
}

// removeContext removes a context from the config directory (preserving order of remaining contexts).
func removeContext(name string, local bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}
	configDir := paths.Dir(local)

	contexts, order, err := loadContextsFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading contexts: %w", err)
	}

	delete(contexts, name)
	newOrder := make([]string, 0, len(order))
	for _, n := range order {
		if n != name {
			newOrder = append(newOrder, n)
		}
	}

	contextPath := filepath.Join(configDir, "contexts.cue")
	return writeContextsFile(contextPath, contexts, newOrder)
}

// removeTask removes a task from the config directory.
func removeTask(name string, local bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}
	configDir := paths.Dir(local)

	tasks, _, err := loadTasksFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading tasks: %w", err)
	}

	delete(tasks, name)

	taskPath := filepath.Join(configDir, "tasks.cue")
	return writeTasksFile(taskPath, tasks)
}
