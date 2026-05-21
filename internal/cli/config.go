package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

// addConfigCommand adds the config command group and its subcommands to the parent.
func addConfigCommand(parent *cobra.Command) {
	configCmd := &cobra.Command{
		Use:     "config",
		GroupID: "commands",
		Short:   "Manage start configuration",
		Long: `Manage configuration for agents, roles, contexts, and tasks.

Configuration can be stored globally (~/.config/start/) or locally (./.start/).
Use --local to target project-specific configuration.`,
		RunE: runConfigList,
	}

	// Verb-first subcommands
	addConfigListCommand(configCmd)
	addConfigGetCommand(configCmd)
	addConfigAddCommand(configCmd)
	addConfigEditCommand(configCmd)
	addConfigRemoveCommand(configCmd)
	addConfigOpenCommand(configCmd)
	addConfigOrderCommand(configCmd)
	addConfigSettingsCommand(configCmd)
	addConfigExportCommand(configCmd)

	parent.AddCommand(configCmd)
}

// runConfigList displays an overview of all configuration.
func runConfigList(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) > 0 {
		return unknownCommandError("start config", args[0])
	}

	w := cmd.OutOrStdout()
	flags := getFlags(cmd)

	// Show config paths
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorPaths.Fprintln(w, "Configuration Paths:")
	globalStatus := "not found"
	if paths.GlobalExists {
		globalStatus = "exists"
	}
	localStatus := "not found"
	if paths.LocalExists {
		localStatus = "exists"
	}
	_, _ = tui.ColorDim.Fprintf(w, "  Global: ")
	_, _ = fmt.Fprintf(w, "%s ", paths.Global)
	_, _ = fmt.Fprintln(w, tui.Annotate("%s", globalStatus))
	_, _ = tui.ColorDim.Fprintf(w, "  Local:  ")
	_, _ = fmt.Fprintf(w, "%s ", paths.Local)
	_, _ = fmt.Fprintln(w, tui.Annotate("%s", localStatus))

	// Determine scope for listing
	scope := config.ScopeFromLocal(flags.Local)
	scopeLabel := scope.String()

	stderr := cmd.ErrOrStderr()

	// Settings
	entries, err := config.ResolveAllSettings(paths, scope)
	if err != nil {
		printWarning(stderr, "failed to load settings: %s", err)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorSettings.Fprint(w, "settings")
	_, _ = fmt.Fprint(w, "/ ")
	_, _ = fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	_, _ = tui.ColorDim.Fprintf(w, ": %d\n", len(entries))
	if len(entries) > 0 {
		printSettingsEntries(w, entries)
	}

	// Agents
	agents, agentOrder, err := loadAgentsForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load agents: %s", err)
	}
	sort.Strings(agentOrder)
	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorAgents.Fprint(w, "agents")
	_, _ = fmt.Fprint(w, "/ ")
	_, _ = fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	_, _ = tui.ColorDim.Fprintf(w, ": %d\n", len(agents))
	if len(agents) > 0 {
		defaultAgent := ""
		if cfg, err := loadConfigForScope(scope); err == nil {
			defaultAgent = getDefaultAgentFromConfig(cfg)
		}
		for _, name := range agentOrder {
			agent := agents[name]
			marker := "  "
			if name == defaultAgent {
				marker = tui.ColorInstalled.Sprint("→") + " "
			}
			_, _ = fmt.Fprintf(w, "  %s%s ", marker, name)
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", agent.Source))
		}
	}

	// Roles
	roles, roleOrder, err := loadRolesForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load roles: %s", err)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorRoles.Fprint(w, "roles")
	_, _ = fmt.Fprint(w, "/ ")
	_, _ = fmt.Fprint(w, tui.Annotate("injection order"))
	_, _ = fmt.Fprint(w, " ")
	_, _ = fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	_, _ = tui.ColorDim.Fprintf(w, ": %d\n", len(roles))
	if len(roles) > 0 {
		for _, name := range roleOrder {
			role := roles[name]
			_, _ = fmt.Fprintf(w, "    %s ", name)
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", role.Source))
		}
	}

	// Contexts
	contexts, contextOrder, err := loadContextsForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load contexts: %s", err)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorContexts.Fprint(w, "contexts")
	_, _ = fmt.Fprint(w, "/ ")
	_, _ = fmt.Fprint(w, tui.Annotate("injection order"))
	_, _ = fmt.Fprint(w, " ")
	_, _ = fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	_, _ = tui.ColorDim.Fprintf(w, ": %d\n", len(contexts))
	if len(contexts) > 0 {
		for _, name := range contextOrder {
			ctx := contexts[name]
			_, _ = fmt.Fprintf(w, "    %s ", name)
			_, _ = fmt.Fprint(w, tui.Annotate("%s", ctx.Source))
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
	}

	// Tasks
	tasks, taskOrder, err := loadTasksForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load tasks: %s", err)
	}
	sort.Strings(taskOrder)
	_, _ = fmt.Fprintln(w)
	_, _ = tui.ColorTasks.Fprint(w, "tasks")
	_, _ = fmt.Fprint(w, "/ ")
	_, _ = fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	_, _ = tui.ColorDim.Fprintf(w, ": %d\n", len(tasks))
	if len(tasks) > 0 {
		for _, name := range taskOrder {
			task := tasks[name]
			_, _ = fmt.Fprintf(w, "    %s ", name)
			_, _ = fmt.Fprintln(w, tui.Annotate("%s", task.Source))
		}
	}

	return nil
}
