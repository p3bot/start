package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

func addConfigGetCommand(parent *cobra.Command, flags *Flags) {
	cmd := &cobra.Command{
		Use:   "get [query]",
		Short: "Show raw config fields for an item",
		Long: `Show raw stored configuration fields for an agent, role, context, or task.

Search by name across all categories. If multiple items match, a numbered
menu is presented. With no argument, prompts interactively for category and item.

Use --global to restrict the lookup to the global config (~/.config/start/) or
--local to restrict to the local config (./.start/). These flags are mutually
exclusive; omitting both reads the merged configuration where local entries
override global entries with the same name.

This command shows the raw stored fields for an installed config entry. Related
commands operate on different data:
  - 'start get <name>'      outputs a module's rendered content (file body,
                            rendered prompt, command output, or agent command
                            template) to stdout for piping.
  - 'start describe <name>' shows the verbose resolved dump for a module,
                            after global/local merging.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigGet,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")

	cmd.Flags().BoolVar(&flags.Global, "global", false, "Restrict to global config only")
	parent.AddCommand(cmd)
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	if err := validateScopeFlags(getFlags(cmd)); err != nil {
		return err
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	scope := scopeFromFlags(getFlags(cmd))
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if len(args) == 0 {
		if jsonFlag {
			return usageError(fmt.Errorf("query required with --json"))
		}
		if !isTerminal(stdin) {
			return usageError(fmt.Errorf("interactive get requires a terminal"))
		}
		return runConfigGetInteractive(stdin, stdout, scope)
	}

	query := args[0]
	matches, err := searchAllConfigCategories(query, scope)
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		if jsonFlag {
			if err := writeJSON(stdout, []ConfigListItem{}); err != nil {
				return fmt.Errorf("marshalling config get: %w", err)
			}
			return nil
		}
		return notFoundError(fmt.Errorf("%q not found", query))
	}

	if jsonFlag {
		var results []ConfigListItem
		for _, m := range matches {
			item, err := buildConfigListItem(m, scope)
			if err != nil {
				return err
			}
			results = append(results, item)
		}
		if err := writeJSON(stdout, results); err != nil {
			return fmt.Errorf("marshalling config get: %w", err)
		}
		return nil
	}

	var selected configMatch
	if len(matches) == 1 {
		selected = matches[0]
	} else {
		if !isTerminal(stdin) {
			return usageError(fmt.Errorf("ambiguous query %q matches multiple items — use an exact name", query))
		}
		selected, err = promptSelectConfigMatch(stdout, stdin, query, matches)
		if err != nil || selected.Category == "" {
			return err
		}
	}

	return printConfigGet(stdout, scope, selected)
}

func runConfigGetInteractive(stdin io.Reader, stdout io.Writer, scope config.Scope) error {
	fmt.Fprintln(stdout, "Get:")
	category, err := promptSelectCategory(stdout, stdin, allConfigCategories)
	if err != nil || category == "" {
		return err
	}

	names, err := loadNamesForCategory(category, scope)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintf(stdout, "No %s configured.\n", category)
		return nil
	}

	singular := strings.TrimSuffix(category, "s")
	fmt.Fprintln(stdout)
	selected, err := promptSelectOneFromList(stdout, stdin, singular, names)
	if err != nil || selected == "" {
		return err
	}

	return printConfigGet(stdout, scope, configMatch{Name: selected, Category: singular})
}

func printConfigGet(w io.Writer, scope config.Scope, m configMatch) error {
	switch m.Category {
	case "agent":
		return printAgentGet(w, scope, m.Name)
	case "role":
		return printRoleGet(w, scope, m.Name)
	case "context":
		return printContextGet(w, scope, m.Name)
	case "task":
		return printTaskGet(w, scope, m.Name)
	}
	return fmt.Errorf("unknown category %q", m.Category)
}

// Command is agent-only and not emitted by the shared metadata writer, so the
// header section prints it inline here; the writer owns its own leading blank line.
func printAgentGet(w io.Writer, scope config.Scope, name string) error {
	agents, _, err := loadAgentsForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, agent, err := resolveInstalledName(agents, "agent", name)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	tui.ColorAgents.Fprint(w, "agents")
	fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	tui.ColorDim.Fprint(w, "Source:")
	fmt.Fprintf(w, " %s\n", agent.Source)
	if agent.Origin != "" {
		tui.ColorDim.Fprint(w, "Origin:")
		fmt.Fprintf(w, " %s\n", agent.Origin)
	}
	tui.ColorDim.Fprint(w, "Command:")
	fmt.Fprintf(w, " %s\n", agent.Command)

	writeAgentMetadata(w, agent)

	printSeparator(w)
	return nil
}

func printRoleGet(w io.Writer, scope config.Scope, name string) error {
	roles, _, err := loadRolesForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, role, err := resolveInstalledName(roles, "role", name)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	tui.ColorRoles.Fprint(w, "roles")
	fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	tui.ColorDim.Fprint(w, "Source:")
	fmt.Fprintf(w, " %s\n", role.Source)
	if role.Origin != "" {
		tui.ColorDim.Fprint(w, "Origin:")
		fmt.Fprintf(w, " %s\n", role.Origin)
	}

	writeRoleMetadata(w, role)

	printSeparator(w)
	return nil
}

func printContextGet(w io.Writer, scope config.Scope, name string) error {
	contexts, _, err := loadContextsForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, ctx, err := resolveInstalledName(contexts, "context", name)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	tui.ColorContexts.Fprint(w, "contexts")
	fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	tui.ColorDim.Fprint(w, "Source:")
	fmt.Fprintf(w, " %s\n", ctx.Source)
	if ctx.Origin != "" {
		tui.ColorDim.Fprint(w, "Origin:")
		fmt.Fprintf(w, " %s\n", ctx.Origin)
	}

	writeContextMetadata(w, ctx)

	printSeparator(w)
	return nil
}

func printTaskGet(w io.Writer, scope config.Scope, name string) error {
	tasks, _, err := loadTasksForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, task, err := resolveInstalledName(tasks, "task", name)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	tui.ColorTasks.Fprint(w, "tasks")
	fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	tui.ColorDim.Fprint(w, "Source:")
	fmt.Fprintf(w, " %s\n", task.Source)
	if task.Origin != "" {
		tui.ColorDim.Fprint(w, "Origin:")
		fmt.Fprintf(w, " %s\n", task.Origin)
	}

	writeTaskMetadata(w, task)

	printSeparator(w)
	return nil
}
