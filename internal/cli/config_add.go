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

// addConfigAddCommand adds the "config add [category]" command.
func addConfigAddCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "add [category]",
		Short: "Add a new config item",
		Long: `Add a new agent, role, context, or task interactively.

Provide a category (agent, role, context, task) to skip the category prompt.
Plural aliases (agents, roles, contexts, tasks) are accepted.

Always interactive — use 'start config open' to edit CUE files directly.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigAdd,
	}
	parent.AddCommand(cmd)
}

// runConfigAdd is the handler for "config add [category]".
func runConfigAdd(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	local := getFlags(cmd).Local

	// Always interactive
	if !isTerminal(stdin) {
		return fmt.Errorf("interactive add requires a terminal")
	}

	category := ""
	if len(args) > 0 {
		category = normalizeCategoryArg(args[0])
		if category == "" {
			return fmt.Errorf("unknown category %q: expected agent, role, context, or task", args[0])
		}
	}

	if category == "" {
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprintln(stdout, "Add:")
		var err error
		category, err = promptSelectCategory(stdout, stdin, allConfigCategories)
		if err != nil || category == "" {
			return err
		}
		// promptSelectCategory returns plural; normalize to singular
		category = strings.TrimSuffix(category, "s")
	} else {
		_, _ = fmt.Fprintln(stdout)
	}

	switch category {
	case "agent":
		return configAgentAdd(stdin, stdout, local)
	case "role":
		return configRoleAdd(stdin, stdout, local)
	case "context":
		return configContextAdd(stdin, stdout, local)
	case "task":
		return configTaskAdd(stdin, stdout, local)
	}
	return nil
}

// configAgentAdd is the inner add logic for agents.
func configAgentAdd(stdin io.Reader, stdout io.Writer, local bool) error {
	name, err := promptString(stdout, stdin, "Agent name", "")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("agent name is required")
	}

	bin, err := promptString(stdout, stdin, "Binary (optional)", "")
	if err != nil {
		return err
	}

	defaultCmd := `"{{.prompt}}"`
	if bin != "" {
		defaultCmd = fmt.Sprintf(`%s "{{.prompt}}"`, bin)
	}
	command, err := promptString(stdout, stdin, "Command template", defaultCmd)
	if err != nil {
		return err
	}
	if command == "" {
		return fmt.Errorf("command template is required")
	}

	var models map[string]string
	models, err = promptModelsAdd(stdout, stdin)
	if err != nil {
		return err
	}

	defaultModel, err := promptDefaultModel(stdout, stdin, "", models)
	if err != nil {
		return err
	}

	description, err := promptString(stdout, stdin, "Description (optional)", "")
	if err != nil {
		return err
	}

	tags, err := promptTags(stdout, stdin, nil, false)
	if err != nil {
		return err
	}

	agent := AgentConfig{
		Name:         name,
		Bin:          bin,
		Command:      command,
		DefaultModel: defaultModel,
		Description:  description,
		Models:       models,
		Tags:         tags,
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(local)
	scopeName := scopeString(local)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	existingAgents, _, err := loadAgentsFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading existing agents: %w", err)
	}

	if _, exists := existingAgents[name]; exists {
		return fmt.Errorf("agent %q already exists in %s config", name, scopeName)
	}

	existingAgents[name] = agent

	agentPath := filepath.Join(configDir, "agents.cue")
	if err := writeAgentsFile(agentPath, existingAgents); err != nil {
		return fmt.Errorf("writing agents file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Added agent %q to %s config\n", name, scopeName)
	_, _ = fmt.Fprintf(stdout, "Config: %s\n", agentPath)
	return nil
}

// configRoleAdd is the inner add logic for roles.
func configRoleAdd(stdin io.Reader, stdout io.Writer, local bool) error {
	name, err := promptString(stdout, stdin, "Role name", "")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("role name is required")
	}

	description, err := promptString(stdout, stdin, "Description (optional)", "")
	if err != nil {
		return err
	}

	file, command, prompt, err := promptContentSource(stdout, stdin, "1", "")
	if err != nil {
		return err
	}

	sourceCount := 0
	if file != "" {
		sourceCount++
	}
	if command != "" {
		sourceCount++
	}
	if prompt != "" {
		sourceCount++
	}

	if sourceCount == 0 {
		return fmt.Errorf("must specify one of: file, command, or prompt")
	}
	if sourceCount > 1 {
		return fmt.Errorf("specify only one of: file, command, or prompt")
	}

	var optional bool
	if file != "" {
		_, _ = fmt.Fprintf(stdout, "Optional %s? %s ", tui.Annotate("skip if file missing"), tui.Bracket("y/N"))
		reader := bufio.NewReader(stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		input = strings.TrimSpace(strings.ToLower(input))
		optional = input == "y" || input == "yes"
	}

	tags, err := promptTags(stdout, stdin, nil, false)
	if err != nil {
		return err
	}

	role := RoleConfig{
		Name:        name,
		Description: description,
		File:        file,
		Command:     command,
		Prompt:      prompt,
		Optional:    optional,
		Tags:        tags,
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(local)
	scopeName := scopeString(local)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	existingRoles, existingOrder, err := loadRolesFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading existing roles: %w", err)
	}

	if _, exists := existingRoles[name]; exists {
		return fmt.Errorf("role %q already exists in %s config", name, scopeName)
	}

	existingRoles[name] = role

	rolePath := filepath.Join(configDir, "roles.cue")
	if err := writeRolesFile(rolePath, existingRoles, append(existingOrder, name)); err != nil {
		return fmt.Errorf("writing roles file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Added role %q to %s config\n", name, scopeName)
	_, _ = fmt.Fprintf(stdout, "Config: %s\n", rolePath)
	return nil
}

// configContextAdd is the inner add logic for contexts.
func configContextAdd(stdin io.Reader, stdout io.Writer, local bool) error {
	name, err := promptString(stdout, stdin, "Context name", "")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("context name is required")
	}

	description, err := promptString(stdout, stdin, "Description (optional)", "")
	if err != nil {
		return err
	}

	file, command, prompt, err := promptContentSource(stdout, stdin, "1", "")
	if err != nil {
		return err
	}

	sourceCount := 0
	if file != "" {
		sourceCount++
	}
	if command != "" {
		sourceCount++
	}
	if prompt != "" {
		sourceCount++
	}

	if sourceCount == 0 {
		return fmt.Errorf("must specify one of: file, command, or prompt")
	}
	if sourceCount > 1 {
		return fmt.Errorf("specify only one of: file, command, or prompt")
	}

	var required, isDefault bool
	{
		_, _ = fmt.Fprintf(stdout, "Required %s? %s ", tui.Annotate("always include"), tui.Bracket("y/N"))
		reader := bufio.NewReader(stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		input = strings.TrimSpace(strings.ToLower(input))
		required = input == "y" || input == "yes"

		if !required {
			_, _ = fmt.Fprintf(stdout, "Default %s? %s ", tui.Annotate("include by default"), tui.Bracket("y/N"))
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("reading input: %w", err)
			}
			input = strings.TrimSpace(strings.ToLower(input))
			isDefault = input == "y" || input == "yes"
		}
	}

	tags, err := promptTags(stdout, stdin, nil, false)
	if err != nil {
		return err
	}

	ctx := ContextConfig{
		Name:        name,
		Description: description,
		File:        file,
		Command:     command,
		Prompt:      prompt,
		Required:    required,
		Default:     isDefault,
		Tags:        tags,
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(local)
	scopeName := scopeString(local)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	existingContexts, existingOrder, err := loadContextsFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading existing contexts: %w", err)
	}

	if _, exists := existingContexts[name]; exists {
		return fmt.Errorf("context %q already exists in %s config", name, scopeName)
	}

	existingContexts[name] = ctx

	contextPath := filepath.Join(configDir, "contexts.cue")
	if err := writeContextsFile(contextPath, existingContexts, append(existingOrder, name)); err != nil {
		return fmt.Errorf("writing contexts file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Added context %q to %s config\n", name, scopeName)
	_, _ = fmt.Fprintf(stdout, "Config: %s\n", contextPath)
	return nil
}

// configTaskAdd is the inner add logic for tasks.
func configTaskAdd(stdin io.Reader, stdout io.Writer, local bool) error {
	name, err := promptString(stdout, stdin, "Task name", "")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("task name is required")
	}

	description, err := promptString(stdout, stdin, "Description (optional)", "")
	if err != nil {
		return err
	}

	file, command, prompt, err := promptContentSource(stdout, stdin, "3", "")
	if err != nil {
		return err
	}

	sourceCount := 0
	if file != "" {
		sourceCount++
	}
	if command != "" {
		sourceCount++
	}
	if prompt != "" {
		sourceCount++
	}

	if sourceCount == 0 {
		return fmt.Errorf("must specify one of: file, command, or prompt")
	}
	if sourceCount > 1 {
		return fmt.Errorf("specify only one of: file, command, or prompt")
	}

	role, err := promptString(stdout, stdin, "Role (optional)", "")
	if err != nil {
		return err
	}

	tags, err := promptTags(stdout, stdin, nil, false)
	if err != nil {
		return err
	}

	task := TaskConfig{
		Name:        name,
		Description: description,
		File:        file,
		Command:     command,
		Prompt:      prompt,
		Role:        role,
		Tags:        tags,
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(local)
	scopeName := scopeString(local)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	existingTasks, _, err := loadTasksFromDir(configDir)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading existing tasks: %w", err)
	}

	if _, exists := existingTasks[name]; exists {
		return fmt.Errorf("task %q already exists in %s config", name, scopeName)
	}

	existingTasks[name] = task

	taskPath := filepath.Join(configDir, "tasks.cue")
	if err := writeTasksFile(taskPath, existingTasks); err != nil {
		return fmt.Errorf("writing tasks file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Added task %q to %s config\n", name, scopeName)
	_, _ = fmt.Fprintf(stdout, "Config: %s\n", taskPath)
	return nil
}
