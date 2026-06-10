package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

// flags is forwarded only to addConfigGetCommand, which binds --global to flags.Global.
func addConfigCommand(parent *cobra.Command, flags *Flags) {
	configCmd := &cobra.Command{
		Use:     "config",
		GroupID: "workflow",
		Short:   "Manage start configuration",
		Long: `Manage configuration for agents, roles, contexts, and tasks.

Configuration can be stored globally (~/.config/start/) or locally (./.start/).
Use --local to target project-specific configuration.`,
		RunE: runConfigList,
	}

	parent.AddCommand(configCmd)

	addConfigListCommand(configCmd)
	addConfigGetCommand(configCmd, flags)
	addConfigAddCommand(configCmd)
	addConfigEditCommand(configCmd)
	addConfigRemoveCommand(configCmd)
	addConfigOpenCommand(configCmd)
	addConfigOrderCommand(configCmd)
	addConfigSettingsCommand(configCmd)
	addConfigExportCommand(configCmd)
}

func runConfigList(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) > 0 {
		return unknownCommandError("start config", args[0])
	}

	w := cmd.OutOrStdout()
	flags := getFlags(cmd)

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	fmt.Fprintln(w)
	tui.ColorPaths.Fprintln(w, "Configuration Paths:")
	globalStatus := "not found"
	if paths.GlobalExists {
		globalStatus = "exists"
	}
	localStatus := "not found"
	if paths.LocalExists {
		localStatus = "exists"
	}
	tui.ColorDim.Fprintf(w, "  Global: ")
	fmt.Fprintf(w, "%s ", paths.Global)
	fmt.Fprintln(w, tui.Annotate("%s", globalStatus))
	tui.ColorDim.Fprintf(w, "  Local:  ")
	fmt.Fprintf(w, "%s ", paths.Local)
	fmt.Fprintln(w, tui.Annotate("%s", localStatus))

	scope := config.ScopeFromLocal(flags.Local)
	scopeLabel := scope.String()

	stderr := cmd.ErrOrStderr()

	entries, err := config.ResolveAllSettings(paths, scope)
	if err != nil {
		printWarning(stderr, "failed to load settings: %s", err)
	}
	fmt.Fprintln(w)
	tui.ColorSettings.Fprint(w, "settings")
	fmt.Fprint(w, ": ")
	fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	tui.ColorDim.Fprintf(w, ": %d\n", len(entries))
	if len(entries) > 0 {
		printSettingsEntries(w, entries)
	}

	agents, agentOrder, err := loadAgentsForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load agents: %s", err)
	}
	sort.Strings(agentOrder)
	fmt.Fprintln(w)
	tui.ColorAgents.Fprint(w, "agents")
	fmt.Fprint(w, ": ")
	fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	tui.ColorDim.Fprintf(w, ": %d\n", len(agents))
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
			fmt.Fprintf(w, "  %s%s ", marker, name)
			fmt.Fprintln(w, tui.Annotate("%s", agent.Source))
		}
	}

	roles, roleOrder, err := loadRolesForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load roles: %s", err)
	}
	fmt.Fprintln(w)
	tui.ColorRoles.Fprint(w, "roles")
	fmt.Fprint(w, ": ")
	fmt.Fprint(w, tui.Annotate("injection order"))
	fmt.Fprint(w, " ")
	fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	tui.ColorDim.Fprintf(w, ": %d\n", len(roles))
	if len(roles) > 0 {
		for _, name := range roleOrder {
			role := roles[name]
			fmt.Fprintf(w, "    %s ", name)
			fmt.Fprintln(w, tui.Annotate("%s", role.Source))
		}
	}

	contexts, contextOrder, err := loadContextsForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load contexts: %s", err)
	}
	fmt.Fprintln(w)
	tui.ColorContexts.Fprint(w, "contexts")
	fmt.Fprint(w, ": ")
	fmt.Fprint(w, tui.Annotate("injection order"))
	fmt.Fprint(w, " ")
	fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	tui.ColorDim.Fprintf(w, ": %d\n", len(contexts))
	if len(contexts) > 0 {
		for _, name := range contextOrder {
			ctx := contexts[name]
			fmt.Fprintf(w, "    %s ", name)
			fmt.Fprint(w, tui.Annotate("%s", ctx.Source))
			if ctx.Required {
				fmt.Fprintf(w, " %s", tui.Bracket("required"))
			}
			if ctx.Default {
				fmt.Fprintf(w, " %s", tui.Bracket("default"))
			}
			if len(ctx.Tags) > 0 {
				fmt.Fprint(w, " ")
				tui.ColorDim.Fprint(w, "tags:")
				fmt.Fprint(w, tui.Bracket("%s", strings.Join(ctx.Tags, ", ")))
			}
			fmt.Fprintln(w)
		}
	}

	tasks, taskOrder, err := loadTasksForScope(scope)
	if err != nil {
		printWarning(stderr, "failed to load tasks: %s", err)
	}
	sort.Strings(taskOrder)
	fmt.Fprintln(w)
	tui.ColorTasks.Fprint(w, "tasks")
	fmt.Fprint(w, ": ")
	fmt.Fprint(w, tui.Annotate("%s", scopeLabel))
	tui.ColorDim.Fprintf(w, ": %d\n", len(tasks))
	if len(tasks) > 0 {
		for _, name := range taskOrder {
			task := tasks[name]
			fmt.Fprintf(w, "    %s ", name)
			fmt.Fprintln(w, tui.Annotate("%s", task.Source))
		}
	}

	return nil
}
