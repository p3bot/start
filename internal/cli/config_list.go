package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/tui"
)

// ConfigListItem represents a single configured item for JSON output.
// All optional fields use omitempty so absent fields are not emitted.
type ConfigListItem struct {
	Category     string            `json:"category"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Bin          string            `json:"bin,omitempty"`
	Command      string            `json:"command,omitempty"`
	DefaultModel string            `json:"defaultModel,omitempty"`
	File         string            `json:"file,omitempty"`
	Prompt       string            `json:"prompt,omitempty"`
	Role         string            `json:"role,omitempty"`
	Required     bool              `json:"required,omitempty"`
	Default      bool              `json:"default,omitempty"`
	Optional     bool              `json:"optional,omitempty"`
	Models       map[string]string `json:"models,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Source       string            `json:"source"`
	Origin       string            `json:"origin,omitempty"`
}

// buildConfigListItem loads the full config data for a match and maps it to ConfigListItem.
// Used by config get and config list JSON paths.
func buildConfigListItem(m configMatch, local bool) (ConfigListItem, error) {
	item := ConfigListItem{Category: m.Category, Name: m.Name}
	switch m.Category {
	case "agent":
		agents, _, err := loadAgentsForScope(local)
		if err != nil {
			return item, err
		}
		_, agent, err := resolveInstalledName(agents, "agent", m.Name)
		if err != nil {
			return item, err
		}
		item.Bin = agent.Bin
		item.Command = agent.Command
		item.DefaultModel = agent.DefaultModel
		item.Description = agent.Description
		item.Models = agent.Models
		item.Tags = agent.Tags
		item.Source = agent.Source
		item.Origin = agent.Origin
	case "role":
		roles, _, err := loadRolesForScope(local)
		if err != nil {
			return item, err
		}
		_, role, err := resolveInstalledName(roles, "role", m.Name)
		if err != nil {
			return item, err
		}
		item.Command = role.Command
		item.Description = role.Description
		item.File = role.File
		item.Optional = role.Optional
		item.Prompt = role.Prompt
		item.Tags = role.Tags
		item.Source = role.Source
		item.Origin = role.Origin
	case "context":
		contexts, _, err := loadContextsForScope(local)
		if err != nil {
			return item, err
		}
		_, ctx, err := resolveInstalledName(contexts, "context", m.Name)
		if err != nil {
			return item, err
		}
		item.Command = ctx.Command
		item.Default = ctx.Default
		item.Description = ctx.Description
		item.File = ctx.File
		item.Prompt = ctx.Prompt
		item.Required = ctx.Required
		item.Tags = ctx.Tags
		item.Source = ctx.Source
		item.Origin = ctx.Origin
	case "task":
		tasks, _, err := loadTasksForScope(local)
		if err != nil {
			return item, err
		}
		_, task, err := resolveInstalledName(tasks, "task", m.Name)
		if err != nil {
			return item, err
		}
		item.Command = task.Command
		item.Description = task.Description
		item.File = task.File
		item.Prompt = task.Prompt
		item.Role = task.Role
		item.Tags = task.Tags
		item.Source = task.Source
		item.Origin = task.Origin
	default:
		return item, fmt.Errorf("unknown category %q", m.Category)
	}
	return item, nil
}

// collectConfigListItems loads all configured items for the given category (or all if "").
// All categories are sorted alphabetically for consistent, analysis-friendly JSON output.
// The human-readable display preserves injection order for roles and contexts.
func collectConfigListItems(local bool, category string) ([]ConfigListItem, error) {
	var items []ConfigListItem

	if category == "" || category == "agent" {
		agents, order, err := loadAgentsForScope(local)
		if err != nil {
			return nil, err
		}
		sort.Strings(order)
		for _, name := range order {
			a := agents[name]
			items = append(items, ConfigListItem{
				Category: "agent", Name: name, Bin: a.Bin, Command: a.Command,
				DefaultModel: a.DefaultModel, Description: a.Description,
				Models: a.Models, Tags: a.Tags, Source: a.Source, Origin: a.Origin,
			})
		}
	}

	if category == "" || category == "role" {
		roles, order, err := loadRolesForScope(local)
		if err != nil {
			return nil, err
		}
		sort.Strings(order)
		for _, name := range order {
			r := roles[name]
			items = append(items, ConfigListItem{
				Category: "role", Name: name, Command: r.Command, Description: r.Description,
				File: r.File, Optional: r.Optional, Prompt: r.Prompt,
				Tags: r.Tags, Source: r.Source, Origin: r.Origin,
			})
		}
	}

	if category == "" || category == "context" {
		contexts, order, err := loadContextsForScope(local)
		if err != nil {
			return nil, err
		}
		sort.Strings(order)
		for _, name := range order {
			c := contexts[name]
			items = append(items, ConfigListItem{
				Category: "context", Name: name, Command: c.Command, Default: c.Default,
				Description: c.Description, File: c.File, Prompt: c.Prompt,
				Required: c.Required, Tags: c.Tags, Source: c.Source, Origin: c.Origin,
			})
		}
	}

	if category == "" || category == "task" {
		tasks, order, err := loadTasksForScope(local)
		if err != nil {
			return nil, err
		}
		sort.Strings(order)
		for _, name := range order {
			t := tasks[name]
			items = append(items, ConfigListItem{
				Category: "task", Name: name, Command: t.Command, Description: t.Description,
				File: t.File, Prompt: t.Prompt, Role: t.Role,
				Tags: t.Tags, Source: t.Source, Origin: t.Origin,
			})
		}
	}

	return items, nil
}

// addConfigListCommand adds the "config list [category]" command.
func addConfigListCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "list [category]",
		Aliases: []string{"ls"},
		Short:   "List configuration items",
		Long: `List configured agents, roles, contexts, and tasks.

Without a category, lists all items grouped by category.
With a category (agent, role, context, task), lists only that category.

Plural aliases (agents, roles, contexts, tasks) are accepted.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigListCmd,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	parent.AddCommand(cmd)
}

// runConfigListCmd is the handler for "config list [category]".
func runConfigListCmd(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	local := getFlags(cmd).Local
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if jsonFlag {
		category := ""
		if len(args) > 0 {
			category = normalizeCategoryArg(args[0])
			if category == "" {
				return fmt.Errorf("unknown category %q: expected agent, role, context, or task", args[0])
			}
		}
		items, err := collectConfigListItems(local, category)
		if err != nil {
			return err
		}
		if items == nil {
			items = []ConfigListItem{}
		}
		if err := writeJSON(cmd.OutOrStdout(), items); err != nil {
			return fmt.Errorf("marshalling config list: %w", err)
		}
		return nil
	}

	w := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	if len(args) == 0 {
		// List all categories
		if err := listAgents(w, stderr, local); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(w)
		if err := listRoles(w, stderr, local); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(w)
		if err := listContexts(w, stderr, local); err != nil {
			return err
		}
		_, _ = fmt.Fprintln(w)
		return listTasks(w, stderr, local)
	}

	category := normalizeCategoryArg(args[0])
	if category == "" {
		return fmt.Errorf("unknown category %q: expected agent, role, context, or task", args[0])
	}

	switch category {
	case "agent":
		return listAgents(w, stderr, local)
	case "role":
		return listRoles(w, stderr, local)
	case "context":
		return listContexts(w, stderr, local)
	case "task":
		return listTasks(w, stderr, local)
	}
	return nil
}

// listAgents prints the agents section to w.
func listAgents(w io.Writer, stderr io.Writer, local bool) error {
	agents, order, err := loadAgentsForScope(local)
	if err != nil {
		printWarning(stderr, "failed to load agents: %s", err)
	}
	sort.Strings(order)

	_, _ = tui.ColorAgents.Fprint(w, "agents")
	_, _ = fmt.Fprintln(w, "/")

	if len(agents) == 0 {
		_, _ = tui.ColorDim.Fprintln(w, "  none")
		return nil
	}

	defaultAgent := ""
	if cfg, err := loadConfigForScope(local); err == nil {
		defaultAgent = getDefaultAgentFromConfig(cfg)
	}

	for _, name := range order {
		agent := agents[name]
		marker := "  "
		if name == defaultAgent {
			marker = tui.ColorInstalled.Sprint("→") + " "
		}
		source := agent.Source
		if agent.Origin != "" {
			source += ", registry"
		}
		if agent.Description != "" {
			_, _ = fmt.Fprintf(w, "%s%s ", marker, name)
			_, _ = tui.ColorDim.Fprint(w, "- "+agent.Description+" ")
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", source))
		} else {
			_, _ = fmt.Fprintf(w, "%s%s ", marker, name)
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", source))
		}
	}
	return nil
}

// listRoles prints the roles section to w.
func listRoles(w io.Writer, stderr io.Writer, local bool) error {
	roles, order, err := loadRolesForScope(local)
	if err != nil {
		printWarning(stderr, "failed to load roles: %s", err)
	}

	_, _ = tui.ColorRoles.Fprint(w, "roles")
	_, _ = fmt.Fprint(w, "/ ")
	_, _ = fmt.Fprintln(w, tui.Annotate("injection order"))

	if len(roles) == 0 {
		_, _ = tui.ColorDim.Fprintln(w, "  none")
		return nil
	}

	for _, name := range order {
		role := roles[name]
		source := role.Source
		if role.Origin != "" {
			source += ", registry"
		}
		if role.Description != "" {
			_, _ = fmt.Fprintf(w, "  %s ", name)
			_, _ = tui.ColorDim.Fprint(w, "- "+role.Description+" ")
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", source))
		} else {
			_, _ = fmt.Fprintf(w, "  %s ", name)
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", source))
		}
	}
	return nil
}

// listContexts prints the contexts section to w.
func listContexts(w io.Writer, stderr io.Writer, local bool) error {
	contexts, order, err := loadContextsForScope(local)
	if err != nil {
		printWarning(stderr, "failed to load contexts: %s", err)
	}

	_, _ = tui.ColorContexts.Fprint(w, "contexts")
	_, _ = fmt.Fprint(w, "/ ")
	_, _ = fmt.Fprintln(w, tui.Annotate("injection order"))

	if len(contexts) == 0 {
		_, _ = tui.ColorDim.Fprintln(w, "  none")
		return nil
	}

	for _, name := range order {
		ctx := contexts[name]
		source := ctx.Source
		if ctx.Origin != "" {
			source += ", registry"
		}
		if ctx.Description != "" {
			_, _ = fmt.Fprintf(w, "  %s ", name)
			_, _ = tui.ColorDim.Fprint(w, "- "+ctx.Description+" ")
			_, _ = fmt.Fprint(w, tui.Annotate("%s", source))
		} else {
			_, _ = fmt.Fprintf(w, "  %s ", name)
			_, _ = fmt.Fprint(w, tui.Annotate("%s", source))
		}
		if ctx.Required {
			_, _ = fmt.Fprintf(w, " %s", tui.Bracket("required"))
		}
		if ctx.Default {
			_, _ = fmt.Fprintf(w, " %s", tui.Bracket("default"))
		}
		if len(ctx.Tags) > 0 {
			_, _ = fmt.Fprint(w, " ")
			_, _ = tui.ColorDim.Fprint(w, "tags:")
			_, _ = fmt.Fprint(w, tui.Bracket("%s", strings.Join(ctx.Tags, ", ")))
		}
		_, _ = fmt.Fprintln(w)
	}
	return nil
}

// runConfigTaskList is a Cobra-compatible wrapper used by task.go.
func runConfigTaskList(cmd *cobra.Command, _ []string) error {
	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	return listTasks(cmd.OutOrStdout(), cmd.ErrOrStderr(), getFlags(cmd).Local)
}

// listTasks prints the tasks section to w.
func listTasks(w io.Writer, stderr io.Writer, local bool) error {
	tasks, order, err := loadTasksForScope(local)
	if err != nil {
		printWarning(stderr, "failed to load tasks: %s", err)
	}
	sort.Strings(order)

	_, _ = tui.ColorTasks.Fprint(w, "tasks")
	_, _ = fmt.Fprintln(w, "/")

	if len(tasks) == 0 {
		_, _ = tui.ColorDim.Fprintln(w, "  none")
		return nil
	}

	for _, name := range order {
		task := tasks[name]
		source := task.Source
		if task.Origin != "" {
			source += ", registry"
		}
		if task.Description != "" {
			_, _ = fmt.Fprintf(w, "  %s ", name)
			_, _ = tui.ColorDim.Fprint(w, "- "+task.Description+" ")
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", source))
		} else {
			_, _ = fmt.Fprintf(w, "  %s ", name)
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", source))
		}
	}
	return nil
}
