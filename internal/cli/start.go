package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/shell"
	"github.com/start-cli/start/internal/temp"
	"github.com/start-cli/start/internal/tui"
)

// flagsKey is the context key for storing Flags.
type flagsKey struct{}

// Flags holds all CLI flag values. Each command instance gets its own Flags,
// enabling parallel test execution without shared state.
type Flags struct {
	Agent   string
	Role    string
	Model   string
	Context []string
	DryRun  bool
	Quiet   bool
	Verbose bool
	Debug   bool
	NoColor bool
	Local   bool
	NoRole  bool
}

// getFlags retrieves Flags from the command context.
func getFlags(cmd *cobra.Command) *Flags {
	if f, ok := cmd.Context().Value(flagsKey{}).(*Flags); ok {
		return f
	}
	// Fallback for commands without context (shouldn't happen in normal use)
	return &Flags{}
}

// Debug log categories.
const (
	dbgConfig  = "config"
	dbgAgent   = "agent"
	dbgRole    = "role"
	dbgContext = "context"
	dbgTask    = "task"
	dbgCompose = "compose"
	dbgExec    = "exec"
	dbgResolve = "resolve"
	dbgCache   = "cache"
)

// debugf prints debug output if debug mode is enabled.
// Format: [DEBUG HH:MM:SS.mmm] <category>: <message>
func debugf(stderr io.Writer, flags *Flags, category, format string, args ...any) {
	if flags.Debug {
		ts := time.Now().Format("15:04:05.000")
		_, _ = fmt.Fprintf(stderr, "[DEBUG %s] %s: "+format+"\n", append([]any{ts, category}, args...)...)
	}
}

// ExecutionEnv holds the common execution environment for start and task commands.
type ExecutionEnv struct {
	Cfg        internalcue.LoadResult
	WorkingDir string
	Agent      orchestration.Agent
	Composer   *orchestration.Composer
	Executor   *orchestration.Executor
}

// loadExecutionConfig loads configuration and resolves the working directory.
// This is the first phase of execution environment setup, separated so the
// resolver can search installed config before building the full environment.
func loadExecutionConfig(stdout, stderr io.Writer, stdin io.Reader, flags *Flags) (internalcue.LoadResult, string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return internalcue.LoadResult{}, "", fmt.Errorf("getting working directory: %w", err)
	}
	debugf(stderr, flags, dbgConfig, "Working directory: %s", workingDir)

	cfg, err := loadMergedConfigFromDirWithDebug(stdout, stderr, stdin, workingDir, flags)
	if err != nil {
		return internalcue.LoadResult{}, "", fmt.Errorf("loading configuration: %w", err)
	}

	return cfg, workingDir, nil
}

// resolveAgentName determines which agent to use when no --agent flag was provided.
// It checks settings.default_agent, falls back to the only configured agent,
// or prompts interactively when multiple agents exist and stdin is a TTY.
func resolveAgentName(cfg internalcue.LoadResult, flags *Flags, stdout, stderr io.Writer, stdin io.Reader) (string, error) {
	// Try settings.default_agent
	if def := cfg.Value.LookupPath(cue.ParsePath(internalcue.KeySettings + ".default_agent")); def.Exists() {
		if s, err := def.String(); err == nil && s != "" {
			debugf(stderr, flags, dbgAgent, "Selected %q (config default)", s)
			return s, nil
		}
	}

	// No default - check configured agents
	choices, err := getConfiguredAgents(cfg.Value)
	if err != nil {
		return "", err
	}
	switch len(choices) {
	case 0:
		return "", fmt.Errorf("no agent configured")
	case 1:
		debugf(stderr, flags, dbgAgent, "Selected %q (only agent)", choices[0].Name)
		return choices[0].Name, nil
	}

	// Multiple agents - check if interactive selection is possible
	isTTY := isTerminal(stdin)
	if !isTTY {
		// Non-TTY fallback: use first agent
		name := choices[0].Name
		debugf(stderr, flags, dbgAgent, "Selected %q (first agent, non-TTY)", name)
		if !flags.Quiet {
			_, _ = fmt.Fprintf(stdout, "Using agent %q %s\n", name, tui.Annotate("set default_agent or use --agent to specify"))
		}
		return name, nil
	}

	// Note: bufio.NewReader may buffer ahead from stdin. This is safe because
	// nothing reads from stdin after agent resolution (the process is replaced by syscall.Exec).
	reader := bufio.NewReader(stdin)
	selected, err := promptAgentSelection(stdout, reader, choices)
	if err != nil {
		return "", err
	}
	debugf(stderr, flags, dbgAgent, "Selected %q (interactive)", selected)
	if promptSetDefault(stdout, reader, selected) {
		if err := setSetting(stdout, flags, "default_agent", selected, false); err != nil {
			printWarning(stdout, "could not save default: %v", err)
		}
	}
	return selected, nil
}

// buildExecutionEnv builds the execution environment from a loaded config and agent name.
// This is the second phase, called after the resolver has resolved flag values.
func buildExecutionEnv(cfg internalcue.LoadResult, workingDir string, agentName string, flags *Flags, stdout, stderr io.Writer, stdin io.Reader) (*ExecutionEnv, error) {
	if agentName == "" {
		resolved, err := resolveAgentName(cfg, flags, stdout, stderr, stdin)
		if err != nil {
			return nil, err
		}
		agentName = resolved
	} else {
		debugf(stderr, flags, dbgAgent, "Selected %q (--agent flag)", agentName)
	}

	agent, err := orchestration.ExtractAgent(cfg.Value, agentName)
	if err != nil {
		return nil, fmt.Errorf("loading agent: %w", err)
	}
	debugf(stderr, flags, dbgAgent, "Binary: %s", agent.Bin)
	debugf(stderr, flags, dbgAgent, "Command template: %s", agent.Command)

	shellRunner := shell.NewRunner()
	processor := orchestration.NewTemplateProcessor(nil, shellRunner, workingDir)
	composer := orchestration.NewComposer(processor, workingDir)
	executor := orchestration.NewExecutor(workingDir)

	return &ExecutionEnv{
		Cfg:        cfg,
		WorkingDir: workingDir,
		Agent:      agent,
		Composer:   composer,
		Executor:   executor,
	}, nil
}

// agentChoice represents an agent available for interactive selection.
type agentChoice struct {
	Name        string
	Description string
}

// getConfiguredAgents returns the list of configured agents in definition order.
func getConfiguredAgents(cfg cue.Value) ([]agentChoice, error) {
	agents := cfg.LookupPath(cue.ParsePath(internalcue.KeyAgents))
	if !agents.Exists() {
		return nil, nil
	}
	iter, err := agents.Fields()
	if err != nil {
		return nil, fmt.Errorf("reading agents configuration: %w", err)
	}
	var choices []agentChoice
	for iter.Next() {
		name := iter.Selector().Unquoted()
		val := iter.Value()
		var desc string
		if v := val.LookupPath(cue.ParsePath("description")); v.Exists() {
			desc, _ = v.String()
		}
		choices = append(choices, agentChoice{Name: name, Description: desc})
	}
	return choices, nil
}

// promptAgentSelection prompts the user to select an agent from multiple choices.
// The caller is responsible for TTY detection; this function assumes interactive input.
func promptAgentSelection(w io.Writer, reader *bufio.Reader, choices []agentChoice) (string, error) {
	_, _ = fmt.Fprintf(w, "Multiple agents configured. Select an agent:\n\n")

	// Find longest name for alignment
	maxNameLen := 0
	for _, c := range choices {
		if len(c.Name) > maxNameLen {
			maxNameLen = len(c.Name)
		}
	}

	for i, c := range choices {
		if c.Description != "" {
			padding := strings.Repeat(" ", maxNameLen-len(c.Name)+2)
			_, _ = fmt.Fprintf(w, "  %2d. %s%s%s\n", i+1, c.Name, padding, c.Description)
		} else {
			_, _ = fmt.Fprintf(w, "  %2d. %s\n", i+1, c.Name)
		}
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(choices)))

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("no selection provided")
	}

	// Try number
	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= len(choices) {
			return choices[choice-1].Name, nil
		}
		return "", fmt.Errorf("invalid selection: %s (choose 1-%d)", input, len(choices))
	}

	// Try exact name match
	inputLower := strings.ToLower(input)
	for _, c := range choices {
		if strings.ToLower(c.Name) == inputLower {
			return c.Name, nil
		}
	}

	// Try substring match
	var subMatches []agentChoice
	for _, c := range choices {
		if strings.Contains(strings.ToLower(c.Name), inputLower) {
			subMatches = append(subMatches, c)
		}
	}
	if len(subMatches) == 1 {
		return subMatches[0].Name, nil
	}

	return "", fmt.Errorf("invalid selection: %s", input)
}

// promptSetDefault asks the user whether to set the selected agent as default.
// The caller is responsible for TTY detection; this function assumes interactive input.
func promptSetDefault(w io.Writer, reader *bufio.Reader, agentName string) bool {
	_, _ = fmt.Fprintf(w, "Set %q as default agent? %s: ", agentName, tui.Bracket("y/N"))

	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// runStart executes the start command (root command with no subcommand).
func runStart(cmd *cobra.Command, args []string) error {
	flags := getFlags(cmd)
	return executeStart(cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin(), flags, orchestration.ContextSelection{
		IncludeRequired: true,
		IncludeDefaults: true,
		Tags:            flags.Context,
	}, "")
}

// executeStart is the shared execution logic for start commands.
func executeStart(stdout, stderr io.Writer, stdin io.Reader, flags *Flags, selection orchestration.ContextSelection, customText string) error {
	// Phase 1: Load config
	cfg, workingDir, err := loadExecutionConfig(stdout, stderr, stdin, flags)
	if err != nil {
		return err
	}

	// Phase 2: Resolve asset flags
	r := newResolver(cfg, flags, stdout, stderr, stdin)

	// Resolve --agent flag
	agentName := flags.Agent
	if agentName != "" {
		agentName, err = r.resolveAgent(agentName)
		if err != nil {
			return err
		}
	}

	// Resolve --role flag
	roleName := flags.Role
	if roleName != "" && !flags.NoRole {
		roleName, err = r.resolveRole(roleName)
		if err != nil {
			return err
		}
	}

	// Resolve --context flags
	if len(selection.Tags) > 0 {
		selection.Tags, err = r.resolveContexts(selection.Tags)
		if err != nil {
			return err
		}
	}

	// If any registry installs happened, reload config
	if r.didInstall {
		debugf(stderr, flags, dbgConfig, "Reloading config after registry installs")
		if err := r.reloadConfig(workingDir); err != nil {
			return err
		}
		cfg = r.cfg
	}

	// Phase 3: Build execution environment with resolved agent
	env, err := buildExecutionEnv(cfg, workingDir, agentName, flags, stdout, stderr, stdin)
	if err != nil {
		return err
	}

	// Resolve --model flag against agent's models map
	resolvedModel := flags.Model
	if resolvedModel != "" {
		resolvedModel = r.resolveModelName(resolvedModel, env.Agent)
	}

	debugf(stderr, flags, dbgContext, "Selection: required=%t, defaults=%t, tags=%v",
		selection.IncludeRequired, selection.IncludeDefaults, selection.Tags)

	// Compose prompt with or without role
	var result orchestration.ComposeResult
	var composeErr error
	if flags.NoRole {
		debugf(stderr, flags, dbgRole, "Skipping role (--no-role)")
		result, composeErr = env.Composer.Compose(env.Cfg.Value, selection, customText)
	} else {
		result, composeErr = env.Composer.ComposeWithRole(env.Cfg.Value, selection, roleName, customText)
	}
	if composeErr != nil {
		// Show UI with role resolutions before returning error
		if !flags.Quiet && len(result.RoleResolutions) > 0 {
			printComposeError(stdout, env.Agent, result)
		}
		return fmt.Errorf("composing prompt: %w", composeErr)
	}

	debugf(stderr, flags, dbgRole, "Selected %q", result.RoleName)
	for _, ctx := range result.Contexts {
		debugf(stderr, flags, dbgContext, "Including %q", ctx.Name)
	}
	debugf(stderr, flags, dbgCompose, "Role: %d bytes", len(result.Role))
	debugf(stderr, flags, dbgCompose, "Prompt: %d bytes (%d contexts)", len(result.Prompt), len(result.Contexts))

	// Print warnings
	printWarnings(flags, stderr, result.Warnings)

	// Determine effective model and its source
	model, modelSource := resolveModel(resolvedModel, env.Agent.DefaultModel)
	if model != "" {
		debugf(stderr, flags, dbgAgent, "Model: %s (%s)", model, modelSource)
	} else {
		debugf(stderr, flags, dbgAgent, "Model: agent default (none specified)")
	}

	// Build execution config
	execConfig := orchestration.ExecuteConfig{
		Agent:      env.Agent,
		Model:      resolvedModel,
		Role:       result.Role,
		RoleFile:   result.RoleFile,
		Prompt:     result.Prompt,
		WorkingDir: env.WorkingDir,
		DryRun:     flags.DryRun,
	}

	// Build command and validate before proceeding
	cmdStr, err := env.Executor.BuildCommand(execConfig)
	if err != nil {
		return err
	}
	debugf(stderr, flags, dbgExec, "Final command: %s", cmdStr)

	if flags.DryRun {
		debugf(stderr, flags, dbgExec, "Dry-run mode, skipping execution")
		return executeDryRun(stdout, cmdStr, execConfig, result, env.Agent, model, modelSource)
	}

	// Print execution info
	if !flags.Quiet {
		printExecutionInfo(stdout, env.Agent, model, modelSource, result)
	}

	debugf(stderr, flags, dbgExec, "Executing agent (process replacement)")
	// Execute agent (replaces current process) - command already validated
	return env.Executor.ExecuteCommand(cmdStr, execConfig)
}

// resolveModel determines the effective model and its source.
// Returns the model name and source ("--model" or "config").
// If both are empty, returns empty strings.
func resolveModel(flagModel, configModel string) (model, source string) {
	if flagModel != "" {
		return flagModel, "--model"
	}
	if configModel != "" {
		return configModel, "config"
	}
	return "", ""
}

// printWarnings prints warnings to stderr if not in quiet mode.
func printWarnings(flags *Flags, stderr io.Writer, warnings []string) {
	if flags.Quiet {
		return
	}
	for _, w := range warnings {
		printWarning(stderr, "%s", w)
	}
}

// executeDryRun handles --dry-run mode.
// cmdStr is the pre-built, pre-validated command string from the caller.
func executeDryRun(w io.Writer, cmdStr string, cfg orchestration.ExecuteConfig, result orchestration.ComposeResult, agent orchestration.Agent, model, modelSource string) error {
	// Create temp directory
	tempMgr := temp.NewDryRunManager()
	dir, err := tempMgr.DryRunDir()
	if err != nil {
		return fmt.Errorf("creating dry-run directory: %w", err)
	}

	// Get context names
	var contextNames []string
	for _, ctx := range result.Contexts {
		contextNames = append(contextNames, ctx.Name)
	}

	// Generate command file content
	cmdContent := orchestration.GenerateDryRunCommand(agent, cfg.Model, result.RoleName, contextNames, cfg.WorkingDir, cmdStr)

	// Write files
	if err := tempMgr.WriteDryRunFiles(dir, result.Role, result.Prompt, cmdContent); err != nil {
		return fmt.Errorf("writing dry-run files: %w", err)
	}

	// Print summary
	printDryRunSummary(w, agent, model, modelSource, result, dir)

	return nil
}

// printExecutionInfo prints the execution summary.
func printExecutionInfo(w io.Writer, agent orchestration.Agent, model, modelSource string, result orchestration.ComposeResult) {
	printHeader(w, "Starting AI Agent")
	printSeparator(w)

	printAgentModel(w, agent, model, modelSource)
	printContextTable(w, result.Contexts, result.Selection)
	printRoleTable(w, result.RoleResolutions)

	_, _ = fmt.Fprintf(w, "Starting %s - awaiting response...\n", agent.Name)
}

// printDryRunSummary prints the dry-run summary.
func printDryRunSummary(w io.Writer, agent orchestration.Agent, model, modelSource string, result orchestration.ComposeResult, dir string) {
	printHeader(w, "Dry Run - Agent Not Executed")
	printSeparator(w)

	printAgentModel(w, agent, model, modelSource)
	printContextTable(w, result.Contexts, result.Selection)
	printRoleTable(w, result.RoleResolutions)

	// Show role preview
	if result.Role != "" {
		printContentPreview(w, "Role", tui.ColorRoles, result.Role, 5)
		_, _ = fmt.Fprintln(w)
	}

	// Show prompt preview
	if result.Prompt != "" {
		printContentPreview(w, "Prompt", tui.ColorPrompts, result.Prompt, 5)
		_, _ = fmt.Fprintln(w)
	}

	_, _ = tui.ColorDim.Fprint(w, "Files:")
	_, _ = fmt.Fprintf(w, " %s\n", dir)
	_, _ = fmt.Fprintln(w, "  role.md")
	_, _ = fmt.Fprintln(w, "  prompt.md")
	_, _ = fmt.Fprintln(w, "  command.txt")
}

// printComposeError prints UI before a composition error.
// Shows agent, contexts, and role resolutions so user understands what failed.
func printComposeError(w io.Writer, agent orchestration.Agent, result orchestration.ComposeResult) {
	printHeader(w, "Starting AI Agent")
	printSeparator(w)

	_, _ = tui.ColorAgents.Fprint(w, "Agent:")
	_, _ = fmt.Fprintf(w, " %s\n", agent.Name)
	_, _ = fmt.Fprintln(w)

	printContextTable(w, result.Contexts, result.Selection)

	printRoleTable(w, result.RoleResolutions)
}

// printContentPreview prints content with a header showing line count only when truncated.
// Shows all content if total lines <= 2*maxLines, otherwise truncates to maxLines.
func printContentPreview(w io.Writer, label string, labelColor *color.Color, text string, maxLines int) {
	lines := strings.Split(text, "\n")
	threshold := maxLines * 2
	truncated := len(lines) > threshold

	if truncated {
		_, _ = labelColor.Fprint(w, label)
		_, _ = fmt.Fprintf(w, " %s:\n", tui.Annotate("%d lines", maxLines))
	} else {
		_, _ = labelColor.Fprint(w, label)
		_, _ = fmt.Fprintln(w, ":")
	}

	if truncated {
		for i := range maxLines {
			_, _ = fmt.Fprintf(w, "  %s\n", lines[i])
		}
		_, _ = fmt.Fprintf(w, "  ... %s\n", tui.Annotate("%d more lines", len(lines)-maxLines))
	} else {
		for _, line := range lines {
			_, _ = fmt.Fprintf(w, "  %s\n", line)
		}
	}
}

// loadMergedConfigFromDir loads configuration using the specified working directory
// for local config resolution. If workingDir is empty, uses current directory.
func loadMergedConfigFromDir(workingDir string) (internalcue.LoadResult, error) {
	return loadMergedConfigWithIO(os.Stdout, os.Stderr, os.Stdin, workingDir)
}

// loadMergedConfigFromDirWithDebug loads configuration with debug logging.
func loadMergedConfigFromDirWithDebug(stdout, stderr io.Writer, stdin io.Reader, workingDir string, flags *Flags) (internalcue.LoadResult, error) {
	// Resolve paths first for debug output
	paths, err := config.ResolvePaths(workingDir)
	if err != nil {
		return internalcue.LoadResult{}, fmt.Errorf("resolving config paths: %w", err)
	}

	debugf(stderr, flags, dbgConfig, "Global: %s (exists: %t)", paths.Global, paths.GlobalExists)
	debugf(stderr, flags, dbgConfig, "Local: %s (exists: %t)", paths.Local, paths.LocalExists)

	// Load using the standard function
	result, err := loadMergedConfigWithIO(stdout, stderr, stdin, workingDir)
	if err != nil {
		return result, err
	}

	// Log what was loaded
	var loaded []string
	if result.GlobalLoaded {
		loaded = append(loaded, "global")
	}
	if result.LocalLoaded {
		loaded = append(loaded, "local")
	}
	if len(loaded) > 0 {
		debugf(stderr, flags, dbgConfig, "Loaded from: %s", strings.Join(loaded, ", "))
	}

	return result, nil
}

// loadMergedConfigWithIO loads configuration with custom I/O streams.
func loadMergedConfigWithIO(stdout, stderr io.Writer, stdin io.Reader, workingDir string) (internalcue.LoadResult, error) {
	paths, err := config.ResolvePaths(workingDir)
	if err != nil {
		return internalcue.LoadResult{}, fmt.Errorf("resolving config paths: %w", err)
	}

	// Validate existing configuration
	validation := config.ValidateConfig(paths)

	// If no valid config exists, trigger auto-setup
	if !validation.AnyValid() {
		// Check if there are validation errors (config exists but is invalid)
		if validation.HasErrors() {
			// Report the first error with full details
			if validation.GlobalError != nil {
				return internalcue.LoadResult{}, fmt.Errorf("%s", validation.GlobalError.DetailedError())
			}
			if validation.LocalError != nil {
				return internalcue.LoadResult{}, fmt.Errorf("%s", validation.LocalError.DetailedError())
			}
		}

		// No config at all - trigger auto-setup
		if err := runAutoSetup(stdout, stderr, stdin); err != nil {
			return internalcue.LoadResult{}, err
		}
		// Re-resolve and validate after auto-setup
		paths, err = config.ResolvePaths(workingDir)
		if err != nil {
			return internalcue.LoadResult{}, fmt.Errorf("resolving config paths: %w", err)
		}
		validation = config.ValidateConfig(paths)
		if !validation.AnyValid() {
			return internalcue.LoadResult{}, fmt.Errorf("auto-setup did not create valid configuration")
		}
	}

	dirs := paths.ForScope(config.ScopeMerged)
	loader := internalcue.NewLoader()
	return loader.Load(dirs)
}

// runAutoSetup runs the auto-setup flow.
func runAutoSetup(stdout, stderr io.Writer, stdin io.Reader) error {
	// Check if stdin is a TTY
	isTTY := isTerminal(stdin)

	autoSetup := orchestration.NewAutoSetup(stdout, stderr, stdin, isTTY)
	ctx := context.Background()

	_, err := autoSetup.Run(ctx)
	if err != nil {
		return fmt.Errorf("auto-setup failed: %w", err)
	}

	_, _ = fmt.Fprintln(stdout)
	return nil
}
