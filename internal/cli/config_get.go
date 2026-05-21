package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

// addConfigGetCommand adds the "config get [query]" command.
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

	// Bind --global to flags.Global. AddCommand must run before
	// MarkFlagsMutuallyExclusive so cobra's mergePersistentFlags() can see
	// the inherited --local from root via VisitParents.
	cmd.Flags().BoolVar(&flags.Global, "global", false, "Restrict to global config only")
	parent.AddCommand(cmd)
	cmd.MarkFlagsMutuallyExclusive("local", "global")
}

// runConfigGet is the handler for "config get [query]".
func runConfigGet(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	scope := scopeFromFlags(getFlags(cmd))
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if len(args) == 0 {
		if jsonFlag {
			return fmt.Errorf("query required with --json")
		}
		if !isTerminal(stdin) {
			return fmt.Errorf("interactive get requires a terminal")
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
		return fmt.Errorf("%q not found", query)
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
			return fmt.Errorf("ambiguous query %q matches multiple items — use an exact name", query)
		}
		selected, err = promptSelectConfigMatch(stdout, stdin, query, matches)
		if err != nil || selected.Category == "" {
			return err
		}
	}

	return printConfigGet(stdout, scope, selected)
}

// runConfigGetInteractive prompts for category then item, then shows the entry.
func runConfigGetInteractive(stdin io.Reader, stdout io.Writer, scope config.Scope) error {
	_, _ = fmt.Fprintln(stdout, "Get:")
	category, err := promptSelectCategory(stdout, stdin, allConfigCategories)
	if err != nil || category == "" {
		return err
	}

	names, err := loadNamesForCategory(category, scope)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		_, _ = fmt.Fprintf(stdout, "No %s configured.\n", category)
		return nil
	}

	singular := strings.TrimSuffix(category, "s")
	_, _ = fmt.Fprintln(stdout)
	selected, err := promptSelectOneFromList(stdout, stdin, singular, names)
	if err != nil || selected == "" {
		return err
	}

	return printConfigGet(stdout, scope, configMatch{Name: selected, Category: singular})
}

// printConfigGet displays the raw config fields for a single matched item.
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

// printAgentGet displays raw fields for an agent. The header section emits
// Source / Origin / Command (Command is agent-only and not owned by the
// shared writer); the writer owns its own leading blank line and renders
// the rest.
func printAgentGet(w io.Writer, scope config.Scope, name string) error {
	agents, _, err := loadAgentsForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, agent, err := resolveInstalledName(agents, "agent", name)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorAgents.Fprint(w, "agents")
	_, _ = fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", agent.Source)
	if agent.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", agent.Origin)
	}
	_, _ = tui.ColorDim.Fprint(w, "Command:")
	_, _ = fmt.Fprintf(w, " %s\n", agent.Command)

	writeAgentMetadata(w, agent)

	printSeparator(w)
	return nil
}

// printRoleGet displays raw fields for a role. The header section emits
// Source / Origin; everything below — including the leading blank line — is
// owned by the shared writer (Description -> File -> Command -> Prompt ->
// Optional -> Tags).
func printRoleGet(w io.Writer, scope config.Scope, name string) error {
	roles, _, err := loadRolesForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, role, err := resolveInstalledName(roles, "role", name)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorRoles.Fprint(w, "roles")
	_, _ = fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", role.Source)
	if role.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", role.Origin)
	}

	writeRoleMetadata(w, role)

	printSeparator(w)
	return nil
}

// printContextGet displays raw fields for a context. The header section
// emits Source / Origin; everything below — including the leading blank
// line — is owned by the shared writer.
func printContextGet(w io.Writer, scope config.Scope, name string) error {
	contexts, _, err := loadContextsForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, ctx, err := resolveInstalledName(contexts, "context", name)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorContexts.Fprint(w, "contexts")
	_, _ = fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", ctx.Source)
	if ctx.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", ctx.Origin)
	}

	writeContextMetadata(w, ctx)

	printSeparator(w)
	return nil
}

// printTaskGet displays raw fields for a task. The header section emits
// Source / Origin; everything below — including the leading blank line —
// is owned by the shared writer.
func printTaskGet(w io.Writer, scope config.Scope, name string) error {
	tasks, _, err := loadTasksForScope(scope)
	if err != nil {
		return err
	}

	resolvedName, task, err := resolveInstalledName(tasks, "task", name)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorTasks.Fprint(w, "tasks")
	_, _ = fmt.Fprintf(w, ":%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", task.Source)
	if task.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", task.Origin)
	}

	writeTaskMetadata(w, task)

	printSeparator(w)
	return nil
}
