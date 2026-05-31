package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/temp"
	"github.com/start-cli/start/internal/tui"
)

// TaskSource indicates where a task comes from.
type TaskSource string

const (
	TaskSourceInstalled TaskSource = "installed"
	TaskSourceRegistry  TaskSource = "registry"
)

// TaskMatch represents a task found during resolution.
type TaskMatch struct {
	Name   string
	Source TaskSource
	Entry  registry.IndexEntry // Only set when Source == TaskSourceRegistry.
}

// maxTaskResults is the maximum number of tasks to display in interactive selection.
const maxTaskResults = 20

// addTaskCommand adds the task command to the parent command.
func addTaskCommand(parent *cobra.Command) {
	taskCmd := &cobra.Command{
		Use:     "task [name] [instructions]",
		Aliases: []string{"tasks"},
		GroupID: "workflow",
		Short:   "List or run a predefined task",
		Long: `List configured tasks or run one with optional instructions.

Without arguments, lists all tasks from global and local configuration.
With a name, searches for and runs the matching task.

The name can be a config task name, a "tasks:name" fully-qualified address
(the prefix must be "tasks"), or a file path (starting with ./, /, or ~).
Tasks are reusable workflows defined in configuration.
Instructions are passed to the task template via the {{.instructions}} placeholder.
If no instructions arg is given and stdin is piped, the piped content is used
as the instructions.`,
		Args: cobra.RangeArgs(0, 2),
		RunE: runTask,
	}
	taskCmd.Flags().StringSlice("tag", nil, "Filter task selection by tags (comma-separated)")
	parent.AddCommand(taskCmd)
}

// runTask executes the task command.
func runTask(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) == 0 {
		if err := runConfigTaskList(cmd, args); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "\nRun %s to search and run a task.\n", tui.Annotate("start task <name>"))
		fmt.Fprintf(cmd.OutOrStdout(), "Run %s to search all configuration and modules.\n", tui.Annotate("start search <name>"))
		return nil
	}

	taskName := args[0]
	instructions := ""
	if len(args) > 1 {
		instructions = args[1]
	} else {
		// Accept piped stdin as instructions. The positional arg wins; empty
		// pipes are accepted (templates may not require {{.instructions}}).
		pipedText, piped, err := readPipedStdin(cmd.InOrStdin())
		if err != nil {
			return err
		}
		if piped {
			instructions = pipedText
		}
	}

	tagFlags, _ := cmd.Flags().GetStringSlice("tag")
	tags := modules.ParseSearchTerms(strings.Join(tagFlags, ","))

	flags := getFlags(cmd)
	return executeTask(cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin(), flags, taskName, instructions, tags)
}

// executeTask handles task execution.
func executeTask(stdout, stderr io.Writer, stdin io.Reader, flags *Flags, taskName, instructions string, tags []string) error {
	cfg, workingDir, err := loadExecutionConfig(stdout, stderr, stdin, flags)
	if err != nil {
		return err
	}

	r := newResolver(cfg, flags, stdout, stderr, stdin)

	agentName := flags.Agent
	if agentName != "" {
		agentName, err = r.resolveAgent(agentName)
		if err != nil {
			return err
		}
	}

	var roleName string
	if !flags.NoRole {
		roleName = flags.Role
		if roleName != "" {
			roleName, err = r.resolveRole(roleName)
			if err != nil {
				return err
			}
		}
	}

	contextTags := flags.Context
	if len(contextTags) > 0 {
		contextTags, err = r.resolveContexts(contextTags)
		if err != nil {
			return err
		}
	}

	flagInstalled := r.didInstall

	if flagInstalled {
		debugf(stderr, flags, dbgConfig, "Reloading config after registry installs")
		if err := r.reloadConfig(workingDir); err != nil {
			return err
		}
		cfg = r.cfg
	}

	env, err := buildExecutionEnv(cfg, workingDir, agentName, flags, stdout, stderr, stdin)
	if err != nil {
		return err
	}

	resolvedModel := flags.Model
	if resolvedModel != "" {
		resolvedModel = r.resolveModelName(resolvedModel, env.Agent)
	}

	debugf(stderr, flags, dbgTask, "Searching for task %q", taskName)

	var taskResult orchestration.ProcessResult
	var resolvedName string
	if orchestration.IsFilePath(taskName) {
		debugf(stderr, flags, dbgTask, "Detected file path, reading file")
		content, err := orchestration.ReadFilePath(taskName)
		if err != nil {
			return fmt.Errorf("reading task file %q: %w", taskName, err)
		}
		// Process through the template processor for {{.instructions}} support.
		taskResult, err = env.Composer.ProcessContent(content, instructions)
		if err != nil {
			return fmt.Errorf("processing task file: %w", err)
		}
		taskResult.FileRead = true
		resolvedName = taskName // Display file path as task name.
	} else {
		// Strip the category prefix when present, or error on a mismatch.
		addr, err := parseAddress(taskName)
		if err != nil {
			return err
		}
		if addr.HasPrefix && addr.Category != "tasks" {
			return usageError(fmt.Errorf("task expects category %q, got %q in %q", "tasks", addr.Category, taskName))
		}
		taskName = addr.Name

		// Phase 1: a full exact name in installed config is unambiguous and needs
		// no registry. Skipped when tags are active so Phase 2 applies the filter.
		if len(tags) == 0 && isExactInstalledKey(env.Cfg.Value, internalcue.KeyTasks, taskName) {
			resolvedName = taskName
			debugf(stderr, flags, dbgTask, "Exact full name match: %s", resolvedName)
		}

		if resolvedName == "" {
			// Phase 2: merge candidates from installed config and registry, then select.
			installedMatches, err := findInstalledTasks(env.Cfg, taskName, tags)
			if err != nil {
				return err
			}

			if len(installedMatches) == 0 && !flags.Quiet {
				fmt.Fprintf(stdout, "Task not found in configuration\n")
			}

			// ensureIndex returns nil error; errors are stored in r.indexErr for graceful fallback.
			index, client, _ := r.ensureIndex()
			var registryMatches []TaskMatch
			if index != nil {
				registryMatches, err = findRegistryTasks(index, taskName, tags)
				if err != nil {
					return err
				}
			} else if len(installedMatches) == 0 {
				// Two fault domains: a registry failure is transient (preserve
				// the index error so the retry signal 75 survives, since
				// not-found can't be determined); no index error means the task
				// is genuinely absent (3).
				if r.indexErr != nil {
					return fmt.Errorf("task %q: registry unavailable: %w", taskName, r.indexErr)
				}
				return notFoundError(fmt.Errorf("task %q not found", taskName))
			}

			allMatches := mergeTaskMatches(installedMatches, registryMatches)
			debugf(stderr, flags, dbgTask, "Found %d installed matches, %d registry matches, %d total",
				len(installedMatches), len(registryMatches), len(allMatches))

			switch len(allMatches) {
			case 0:
				return notFoundError(fmt.Errorf("task %q not found", taskName))
			case 1:
				match := allMatches[0]
				if match.Source == TaskSourceRegistry {
					if !flags.Quiet {
						fmt.Fprintf(stdout, "Installing %s from registry...\n", match.Name)
					}
					result := modules.SearchResult{
						Category: "tasks",
						Name:     match.Name,
						Entry:    match.Entry,
					}
					env, err = installTaskAndReloadEnv(stdout, stderr, stdin, flags, client, index, result, workingDir, agentName)
					if err != nil {
						return err
					}
				}
				resolvedName = match.Name
			default:
				debugf(stderr, flags, dbgTask, "Multiple matches, prompting for selection")
				isTTY := isTerminal(stdin)
				if !isTTY {
					var names []string
					for _, m := range allMatches {
						names = append(names, m.Name)
					}
					return usageError(fmt.Errorf("ambiguous task %q matches: %s\nSpecify exact name or run interactively", taskName, strings.Join(names, ", ")))
				}
				reader := bufio.NewReader(stdin)
				selected, err := promptTaskSelection(stdout, reader, allMatches, taskName)
				if err != nil {
					return err
				}
				if selected.Source == TaskSourceRegistry {
					if !flags.Quiet {
						fmt.Fprintf(stdout, "Installing %s from registry...\n", selected.Name)
					}
					result := modules.SearchResult{
						Category: "tasks",
						Name:     selected.Name,
						Entry:    selected.Entry,
					}
					env, err = installTaskAndReloadEnv(stdout, stderr, stdin, flags, client, index, result, workingDir, agentName)
					if err != nil {
						return err
					}
				}
				resolvedName = selected.Name
			}
		}

		if resolvedName != taskName {
			debugf(stderr, flags, dbgTask, "Resolved to %q", resolvedName)
		} else {
			debugf(stderr, flags, dbgTask, "Resolved to %q (exact match)", resolvedName)
		}

		taskResult, err = env.Composer.ResolveTask(env.Cfg.Value, resolvedName, instructions)
		if err != nil {
			return fmt.Errorf("resolving task: %w", err)
		}

		if !flags.NoRole && roleName == "" {
			roleName = orchestration.GetTaskRole(env.Cfg.Value, resolvedName)
			if roleName != "" {
				// An uninstalled task role resolves through the three-tier search,
				// which may auto-install from the registry (like the --role flag).
				if resolved, err := findExactInstalledName(env.Cfg.Value, internalcue.KeyRoles, roleName); err != nil {
					// Ambiguous short name: route through the resolver.
					debugf(stderr, flags, dbgTask, "Task role %q: short name ambiguous, routing to resolver: %v", roleName, err)
					beforeInstall := r.didInstall
					roleName, err = r.resolveRole(roleName)
					if err != nil {
						return err
					}
					if r.didInstall && !beforeInstall {
						env, err = reloadEnv(workingDir, agentName, flags, stdout, stderr, stdin)
						if err != nil {
							return err
						}
					}
				} else if resolved != "" {
					roleName = resolved
				} else {
					beforeInstall := r.didInstall
					roleName, err = r.resolveRole(roleName)
					if err != nil {
						return err
					}
					if r.didInstall && !beforeInstall {
						env, err = reloadEnv(workingDir, agentName, flags, stdout, stderr, stdin)
						if err != nil {
							return err
						}
					}
				}
				debugf(stderr, flags, dbgRole, "Selected %q (from task)", roleName)
			}
		}
	}

	if taskResult.CommandExecuted {
		debugf(stderr, flags, dbgTask, "UTD source: command (executed)")
	} else if taskResult.FileRead {
		debugf(stderr, flags, dbgTask, "UTD source: file")
	} else {
		debugf(stderr, flags, dbgTask, "UTD source: prompt")
	}

	if instructions != "" {
		debugf(stderr, flags, dbgTask, "Instructions: %s", instructions)
	}

	if flags.Role != "" {
		debugf(stderr, flags, dbgRole, "Selected %q (--role flag)", flags.Role)
	}

	// Tasks load required contexts only, unless --context none suppresses them.
	selection := orchestration.ContextSelection{
		IncludeRequired: true,
		IncludeDefaults: false,
		Tags:            contextTags,
	}
	if flags.NoImplicitContexts {
		selection.IncludeRequired = false
	}

	debugf(stderr, flags, dbgContext, "Selection: required=%t, defaults=%t, tags=%v",
		selection.IncludeRequired, selection.IncludeDefaults, selection.Tags)

	var composeResult orchestration.ComposeResult
	var composeErr error
	if flags.NoRole {
		debugf(stderr, flags, dbgRole, "Skipping role (--role none)")
		composeResult, composeErr = env.Composer.Compose(env.Cfg.Value, selection, taskResult.Content)
	} else {
		composeResult, composeErr = env.Composer.ComposeWithRole(env.Cfg.Value, selection, roleName, taskResult.Content)
	}
	if composeErr != nil {
		// Show role resolutions before returning the error.
		if !flags.Quiet && len(composeResult.RoleResolutions) > 0 {
			printComposeError(stdout, env.Agent, composeResult)
		}
		return fmt.Errorf("composing prompt: %w", composeErr)
	}

	for _, ctx := range composeResult.Contexts {
		debugf(stderr, flags, dbgContext, "Including %q", ctx.Name)
	}
	debugf(stderr, flags, dbgCompose, "Role: %d bytes", len(composeResult.Role))
	debugf(stderr, flags, dbgCompose, "Prompt: %d bytes (%d contexts)", len(composeResult.Prompt), len(composeResult.Contexts))

	printWarnings(flags, stderr, taskResult.Warnings)
	printWarnings(flags, stderr, composeResult.Warnings)

	model, modelSource := resolveModel(resolvedModel, env.Agent.DefaultModel)
	if model != "" {
		debugf(stderr, flags, dbgTask, "Model: %s (%s)", model, modelSource)
	} else {
		debugf(stderr, flags, dbgTask, "Model: agent default (none specified)")
	}

	execConfig := orchestration.ExecuteConfig{
		Agent:      env.Agent,
		Model:      resolvedModel,
		Role:       composeResult.Role,
		RoleFile:   composeResult.RoleFile,
		Prompt:     composeResult.Prompt,
		WorkingDir: env.WorkingDir,
		DryRun:     flags.DryRun,
	}

	cmdStr, err := env.Executor.BuildCommand(execConfig)
	if err != nil {
		return err
	}
	debugf(stderr, flags, dbgExec, "Final command: %s", cmdStr)

	if flags.DryRun {
		debugf(stderr, flags, dbgExec, "Dry-run mode, skipping execution")
		return executeTaskDryRun(stdout, cmdStr, execConfig, composeResult, env.Agent, model, modelSource, resolvedName, instructions)
	}

	if !flags.Quiet {
		printTaskExecutionInfo(stdout, env.Agent, model, modelSource, composeResult, resolvedName, instructions, taskResult)
	}

	debugf(stderr, flags, dbgExec, "Executing agent (process replacement)")
	// Replaces the current process; command already validated.
	return env.Executor.ExecuteCommand(cmdStr, execConfig)
}

// executeTaskDryRun handles --dry-run mode for tasks. cmdStr is the pre-built,
// pre-validated command string from the caller.
func executeTaskDryRun(w io.Writer, cmdStr string, cfg orchestration.ExecuteConfig, result orchestration.ComposeResult, agent orchestration.Agent, model, modelSource, taskName, instructions string) error {
	tempMgr := temp.NewDryRunManager()
	dir, err := tempMgr.DryRunDir()
	if err != nil {
		return fmt.Errorf("creating dry-run directory: %w", err)
	}

	var contextNames []string
	for _, ctx := range result.Contexts {
		contextNames = append(contextNames, ctx.Name)
	}

	cmdContent := orchestration.GenerateDryRunCommand(agent, cfg.Model, result.RoleName, contextNames, cfg.WorkingDir, cmdStr)

	if err := tempMgr.WriteDryRunFiles(dir, result.Role, result.Prompt, cmdContent); err != nil {
		return fmt.Errorf("writing dry-run files: %w", err)
	}

	printTaskDryRunSummary(w, agent, model, modelSource, result, dir, taskName, instructions)

	return nil
}

// printTaskExecutionInfo prints the task execution summary.
func printTaskExecutionInfo(w io.Writer, agent orchestration.Agent, model, modelSource string, result orchestration.ComposeResult, taskName, instructions string, taskResult orchestration.ProcessResult) {
	printHeader(w, fmt.Sprintf("Starting Task: %s", taskName))
	printSeparator(w)

	printAgentModel(w, agent, model, modelSource)
	printContextTable(w, result.Contexts, result.Selection)
	printRoleTable(w, result.RoleResolutions)

	if taskResult.CommandExecuted {
		fmt.Fprintln(w, "Command: executed")
	}

	if instructions != "" {
		fmt.Fprintf(w, "Instructions:\n%s\n", instructions)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Starting %s - awaiting response...\n", agent.Name)
}

// printTaskDryRunSummary prints the task dry-run summary.
func printTaskDryRunSummary(w io.Writer, agent orchestration.Agent, model, modelSource string, result orchestration.ComposeResult, dir, taskName, instructions string) {
	printHeader(w, fmt.Sprintf("Dry Run - Task: %s", taskName))
	printSeparator(w)

	printAgentModel(w, agent, model, modelSource)
	printContextTable(w, result.Contexts, result.Selection)
	printRoleTable(w, result.RoleResolutions)

	if instructions != "" {
		fmt.Fprintf(w, "Instructions:\n%s\n", instructions)
		fmt.Fprintln(w)
	}

	if result.Role != "" {
		printContentPreview(w, "Role", tui.ColorRoles, result.Role, 5)
		fmt.Fprintln(w)
	}

	if result.Prompt != "" {
		printContentPreview(w, "Prompt", tui.ColorPrompts, result.Prompt, 5)
		fmt.Fprintln(w)
	}

	tui.ColorDim.Fprint(w, "Files:")
	fmt.Fprintf(w, " %s/\n", dir)
	fmt.Fprintln(w, "  role.md")
	fmt.Fprintln(w, "  prompt.md")
	fmt.Fprintln(w, "  command.txt")
}

// taskInMatches returns true if a task name appears in the match list.
func taskInMatches(name string, matches []TaskMatch) bool {
	for _, m := range matches {
		if m.Name == name {
			return true
		}
	}
	return false
}

// findInstalledTasks finds config tasks matching the search term. Multiple terms
// use AND logic; when tags is non-empty, entries must also match at least one tag.
func findInstalledTasks(cfg internalcue.LoadResult, searchTerm string, tags []string) ([]TaskMatch, error) {
	results, err := modules.SearchInstalledConfig(cfg.Value, internalcue.KeyTasks, "tasks", searchTerm, tags)
	if err != nil {
		return nil, err
	}
	var matches []TaskMatch
	for _, r := range results {
		matches = append(matches, TaskMatch{
			Name:   r.Name,
			Source: TaskSourceInstalled,
		})
	}
	return matches, nil
}

// findRegistryTasks finds registry tasks matching the search term. Multiple terms
// use AND logic; when tags is non-empty, entries must also match at least one tag.
func findRegistryTasks(index *registry.Index, searchTerm string, tags []string) ([]TaskMatch, error) {
	results, err := modules.SearchCategoryEntries("tasks", index.Tasks, searchTerm, tags)
	if err != nil {
		return nil, err
	}
	var matches []TaskMatch
	for _, r := range results {
		matches = append(matches, TaskMatch{
			Name:   r.Name,
			Source: TaskSourceRegistry,
			Entry:  r.Entry,
		})
	}
	return matches, nil
}

// mergeTaskMatches combines installed and registry matches, deduplicating by
// name (installed wins) and sorting alphabetically.
func mergeTaskMatches(installed, registry []TaskMatch) []TaskMatch {
	installedNames := make(map[string]bool)
	for _, m := range installed {
		installedNames[m.Name] = true
	}

	merged := make([]TaskMatch, len(installed))
	copy(merged, installed)

	for _, m := range registry {
		if !installedNames[m.Name] {
			merged = append(merged, m)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Name < merged[j].Name
	})

	return merged
}

// promptTaskSelection prompts the user to select a task from multiple matches.
// The caller is responsible for TTY detection.
func promptTaskSelection(w io.Writer, reader *bufio.Reader, matches []TaskMatch, searchTerm string) (TaskMatch, error) {
	totalCount := len(matches)
	displayCount := totalCount
	truncated := false
	if displayCount > maxTaskResults {
		displayCount = maxTaskResults
		truncated = true
	}

	fmt.Fprintf(w, "Found %d tasks matching %q:\n\n", totalCount, searchTerm)

	maxNameLen := 0
	for i := 0; i < displayCount; i++ {
		if len(matches[i].Name) > maxNameLen {
			maxNameLen = len(matches[i].Name)
		}
	}

	for i := 0; i < displayCount; i++ {
		m := matches[i]
		padding := strings.Repeat(" ", maxNameLen-len(m.Name)+2)
		var sourceLabel string
		if m.Source == TaskSourceInstalled {
			sourceLabel = tui.ColorInstalled.Sprint(m.Source)
		} else {
			sourceLabel = tui.ColorRegistry.Sprint(m.Source)
		}
		fmt.Fprintf(w, "  %2d. %s%s%s\n", i+1, m.Name, padding, sourceLabel)
	}

	if truncated {
		fmt.Fprintf(w, "\nShowing %d of %d matches. Refine search for more specific results.\n", displayCount, totalCount)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", displayCount))

	input, err := reader.ReadString('\n')
	if err != nil {
		return TaskMatch{}, fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)

	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= displayCount {
			return matches[choice-1], nil
		}
		return TaskMatch{}, fmt.Errorf("invalid selection: %s (choose 1-%d)", input, displayCount)
	}

	inputLower := strings.ToLower(input)
	for i := 0; i < displayCount; i++ {
		if strings.ToLower(matches[i].Name) == inputLower {
			return matches[i], nil
		}
	}

	var subMatches []TaskMatch
	for i := 0; i < displayCount; i++ {
		if strings.Contains(strings.ToLower(matches[i].Name), inputLower) {
			subMatches = append(subMatches, matches[i])
		}
	}
	if len(subMatches) == 1 {
		return subMatches[0], nil
	}

	return TaskMatch{}, fmt.Errorf("invalid selection: %s", input)
}

// reloadEnv reloads configuration and rebuilds the execution environment after a module install.
func reloadEnv(workingDir, agentName string, flags *Flags, stdout, stderr io.Writer, stdin io.Reader) (*ExecutionEnv, error) {
	reloadedCfg, err := loadMergedConfigFromDirWithDebug(stdout, stderr, stdin, workingDir, flags)
	if err != nil {
		return nil, fmt.Errorf("reloading configuration: %w", err)
	}
	return buildExecutionEnv(reloadedCfg, workingDir, agentName, flags, stdout, stderr, stdin)
}

// installTaskAndReloadEnv installs a task from the registry and reloads the execution environment.
func installTaskAndReloadEnv(stdout, stderr io.Writer, stdin io.Reader, flags *Flags, client registry.Client, index *registry.Index, result modules.SearchResult, workingDir, agentName string) (*ExecutionEnv, error) {
	if err := installTaskFromRegistry(stdout, flags, client, index, result); err != nil {
		return nil, err
	}
	return reloadEnv(workingDir, agentName, flags, stdout, stderr, stdin)
}

// installTaskFromRegistry installs a task from the registry using a pre-fetched client and result.
func installTaskFromRegistry(stdout io.Writer, flags *Flags, client registry.Client, index *registry.Index, result modules.SearchResult) error {
	ctx := context.Background()

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	// Auto-install always targets global config.
	configDir := paths.Global

	version, err := modules.InstallModule(ctx, client, index, result, configDir)
	if err != nil {
		return err
	}

	if !flags.Quiet {
		if version != "" {
			fmt.Fprintf(stdout, "Installed %s@%s to global config\n\n", result.Name, version)
		} else {
			fmt.Fprintf(stdout, "Installed %s to global config\n\n", result.Name)
		}
	}

	return nil
}
