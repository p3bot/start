package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/start-cli/start/internal/tui"
	"github.com/spf13/cobra"
)

// addConfigInfoCommand adds the "config info [query]" command.
func addConfigInfoCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "info [query]",
		Short: "Show raw config fields for an item",
		Long: `Show raw stored configuration fields for an agent, role, context, or task.

Search by name across all categories. If multiple items match, a numbered
menu is presented. With no argument, prompts interactively for category and item.

This shows raw stored fields, not resolved content. Use 'start show' to view
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

// printAgentInfo displays raw fields for an agent.
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
	_, _ = fmt.Fprintf(w, "/%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", agent.Source)
	if agent.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", agent.Origin)
	}
	if agent.Bin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Bin:")
		_, _ = fmt.Fprintf(w, " %s\n", agent.Bin)
	}
	_, _ = tui.ColorDim.Fprint(w, "Command:")
	_, _ = fmt.Fprintf(w, " %s\n", agent.Command)
	if agent.DefaultModel != "" {
		_, _ = tui.ColorDim.Fprint(w, "Default Model:")
		_, _ = fmt.Fprintf(w, " %s\n", agent.DefaultModel)
	}
	if agent.Description != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = tui.ColorDim.Fprint(w, "Description:")
		_, _ = fmt.Fprintf(w, " %s\n", agent.Description)
	}
	if len(agent.Tags) > 0 {
		_, _ = tui.ColorDim.Fprint(w, "Tags:")
		_, _ = fmt.Fprintf(w, " %s\n", strings.Join(agent.Tags, ", "))
	}
	if len(agent.Models) > 0 {
		_, _ = fmt.Fprintln(w)
		_, _ = tui.ColorDim.Fprintln(w, "Models:")
		var aliases []string
		for alias := range agent.Models {
			aliases = append(aliases, alias)
		}
		sort.Strings(aliases)
		for _, alias := range aliases {
			_, _ = fmt.Fprintf(w, "  %s ", alias)
			_, _ = tui.ColorBlue.Fprint(w, "->")
			_, _ = fmt.Fprint(w, " ")
			_, _ = tui.ColorDim.Fprintf(w, "%s\n", agent.Models[alias])
		}
	}
	printSeparator(w)
	return nil
}

// printRoleInfo displays raw fields for a role.
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
	_, _ = fmt.Fprintf(w, "/%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", role.Source)
	if role.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", role.Origin)
	}
	if role.Description != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = tui.ColorDim.Fprint(w, "Description:")
		_, _ = fmt.Fprintf(w, " %s\n", role.Description)
	}
	if role.File != "" {
		_, _ = tui.ColorDim.Fprint(w, "File:")
		_, _ = fmt.Fprintf(w, " %s\n", role.File)
	}
	if role.Command != "" {
		_, _ = tui.ColorDim.Fprint(w, "Command:")
		_, _ = fmt.Fprintf(w, " %s\n", role.Command)
	}
	if role.Prompt != "" {
		_, _ = tui.ColorDim.Fprint(w, "Prompt:")
		_, _ = fmt.Fprintf(w, " %s\n", truncatePrompt(role.Prompt, 100))
	}
	if role.Optional {
		_, _ = tui.ColorDim.Fprint(w, "Optional:")
		_, _ = fmt.Fprintln(w, " true")
	}
	if len(role.Tags) > 0 {
		_, _ = tui.ColorDim.Fprint(w, "Tags:")
		_, _ = fmt.Fprintf(w, " %s\n", strings.Join(role.Tags, ", "))
	}
	printSeparator(w)
	return nil
}

// printContextInfo displays raw fields for a context.
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
	_, _ = fmt.Fprintf(w, "/%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", ctx.Source)
	if ctx.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", ctx.Origin)
	}
	if ctx.Description != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = tui.ColorDim.Fprint(w, "Description:")
		_, _ = fmt.Fprintf(w, " %s\n", ctx.Description)
	}
	if ctx.File != "" {
		_, _ = tui.ColorDim.Fprint(w, "File:")
		_, _ = fmt.Fprintf(w, " %s\n", ctx.File)
	}
	if ctx.Command != "" {
		_, _ = tui.ColorDim.Fprint(w, "Command:")
		_, _ = fmt.Fprintf(w, " %s\n", ctx.Command)
	}
	if ctx.Prompt != "" {
		_, _ = tui.ColorDim.Fprint(w, "Prompt:")
		_, _ = fmt.Fprintf(w, " %s\n", truncatePrompt(ctx.Prompt, 100))
	}
	_, _ = tui.ColorDim.Fprint(w, "Required:")
	_, _ = fmt.Fprintf(w, " %t\n", ctx.Required)
	_, _ = tui.ColorDim.Fprint(w, "Default:")
	_, _ = fmt.Fprintf(w, " %t\n", ctx.Default)
	if len(ctx.Tags) > 0 {
		_, _ = tui.ColorDim.Fprint(w, "Tags:")
		_, _ = fmt.Fprintf(w, " %s\n", strings.Join(ctx.Tags, ", "))
	}
	printSeparator(w)
	return nil
}

// printTaskInfo displays raw fields for a task.
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
	_, _ = fmt.Fprintf(w, "/%s\n", resolvedName)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Source:")
	_, _ = fmt.Fprintf(w, " %s\n", task.Source)
	if task.Origin != "" {
		_, _ = tui.ColorDim.Fprint(w, "Origin:")
		_, _ = fmt.Fprintf(w, " %s\n", task.Origin)
	}
	if task.Description != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = tui.ColorDim.Fprint(w, "Description:")
		_, _ = fmt.Fprintf(w, " %s\n", task.Description)
	}
	if task.File != "" {
		_, _ = tui.ColorDim.Fprint(w, "File:")
		_, _ = fmt.Fprintf(w, " %s\n", task.File)
	}
	if task.Command != "" {
		_, _ = tui.ColorDim.Fprint(w, "Command:")
		_, _ = fmt.Fprintf(w, " %s\n", task.Command)
	}
	if task.Prompt != "" {
		_, _ = tui.ColorDim.Fprint(w, "Prompt:")
		_, _ = fmt.Fprintf(w, " %s\n", truncatePrompt(task.Prompt, 100))
	}
	if task.Role != "" {
		_, _ = tui.ColorDim.Fprint(w, "Role:")
		_, _ = fmt.Fprintf(w, " %s\n", task.Role)
	}
	if len(task.Tags) > 0 {
		_, _ = tui.ColorDim.Fprint(w, "Tags:")
		_, _ = fmt.Fprintf(w, " %s\n", strings.Join(task.Tags, ", "))
	}
	printSeparator(w)
	return nil
}
