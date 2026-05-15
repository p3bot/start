package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/tui"
)

// addConfigInfoCommand adds the "config info [query]" command.
func addConfigInfoCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "info [query]",
		Short: "Show raw config fields for an item",
		Long: `Show raw stored configuration fields for an agent, role, context, or task.

Search by name across all categories. If multiple items match, a numbered
menu is presented. With no argument, prompts interactively for category and item.

This shows raw stored fields, not resolved content. Use 'start describe' to view
resolved content after global/local merging.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigInfo,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	parent.AddCommand(cmd)
}

// runConfigInfo is the handler for "config info [query]".
func runConfigInfo(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	local := getFlags(cmd).Local
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if len(args) == 0 {
		if jsonFlag {
			return fmt.Errorf("query required with --json")
		}
		if !isTerminal(stdin) {
			return fmt.Errorf("interactive info requires a terminal")
		}
		return runConfigInfoInteractive(stdin, stdout, local)
	}

	query := args[0]
	matches, err := searchAllConfigCategories(query, local)
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		if jsonFlag {
			if err := writeJSON(stdout, []ConfigListItem{}); err != nil {
				return fmt.Errorf("marshalling config info: %w", err)
			}
			return nil
		}
		return fmt.Errorf("%q not found", query)
	}

	if jsonFlag {
		var results []ConfigListItem
		for _, m := range matches {
			item, err := buildConfigListItem(m, local)
			if err != nil {
				return err
			}
			results = append(results, item)
		}
		if err := writeJSON(stdout, results); err != nil {
			return fmt.Errorf("marshalling config info: %w", err)
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

	return printConfigInfo(stdout, local, selected)
}

// runConfigInfoInteractive prompts for category then item, then shows info.
func runConfigInfoInteractive(stdin io.Reader, stdout io.Writer, local bool) error {
	_, _ = fmt.Fprintln(stdout, "Info:")
	category, err := promptSelectCategory(stdout, stdin, allConfigCategories)
	if err != nil || category == "" {
		return err
	}

	names, err := loadNamesForCategory(category, local)
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

	return printConfigInfo(stdout, local, configMatch{Name: selected, Category: singular})
}

// printConfigInfo displays the raw config fields for a single matched item.
func printConfigInfo(w io.Writer, local bool, m configMatch) error {
	switch m.Category {
	case "agent":
		return printAgentInfo(w, local, m.Name)
	case "role":
		return printRoleInfo(w, local, m.Name)
	case "context":
		return printContextInfo(w, local, m.Name)
	case "task":
		return printTaskInfo(w, local, m.Name)
	}
	return fmt.Errorf("unknown category %q", m.Category)
}

// printAgentInfo displays raw fields for an agent. The header section emits
// Source / Origin / Command (Command is agent-only and not owned by the
// shared writer); the writer owns its own leading blank line and renders
// the rest.
func printAgentInfo(w io.Writer, local bool, name string) error {
	agents, _, err := loadAgentsForScope(local)
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

// printRoleInfo displays raw fields for a role. The header section emits
// Source / Origin; everything below — including the leading blank line — is
// owned by the shared writer (Description -> File -> Command -> Prompt ->
// Optional -> Tags).
func printRoleInfo(w io.Writer, local bool, name string) error {
	roles, _, err := loadRolesForScope(local)
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

// printContextInfo displays raw fields for a context. The header section
// emits Source / Origin; everything below — including the leading blank
// line — is owned by the shared writer.
func printContextInfo(w io.Writer, local bool, name string) error {
	contexts, _, err := loadContextsForScope(local)
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

// printTaskInfo displays raw fields for a task. The header section emits
// Source / Origin; everything below — including the leading blank line —
// is owned by the shared writer.
func printTaskInfo(w io.Writer, local bool, name string) error {
	tasks, _, err := loadTasksForScope(local)
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
