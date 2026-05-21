package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

// addConfigEditCommand adds the "config edit [query]" command.
func addConfigEditCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "edit [query]",
		Short: "Edit a config item interactively",
		Long: `Edit an agent, role, context, or task interactively.

Search by name across all categories. If multiple items match, a numbered
menu is presented. With no argument, prompts interactively for category and item.

Always interactive — use 'start config open' to edit CUE files directly.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigEdit,
	}
	parent.AddCommand(cmd)
}

// runConfigEdit is the handler for "config edit [query]".
func runConfigEdit(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	stdin := cmd.InOrStdin()
	stdout := cmd.OutOrStdout()
	local := getFlags(cmd).Local

	if len(args) == 0 {
		if !isTerminal(stdin) {
			return fmt.Errorf("interactive edit requires a terminal")
		}
		return runConfigEditInteractive(stdin, stdout, local)
	}

	_, _ = fmt.Fprintln(stdout)
	query := args[0]
	matches, err := searchAllConfigCategories(query, config.ScopeFromLocal(local))
	if err != nil {
		return err
	}

	if len(matches) == 0 {
		return fmt.Errorf("%q not found", query)
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

	if !isTerminal(stdin) {
		return fmt.Errorf("editing %q requires a terminal — use 'start config open' to edit the CUE file directly", selected.Name)
	}

	return configEditByCategory(stdin, stdout, local, selected.Category, selected.Name)
}

// runConfigEditInteractive prompts for category then item, then edits.
func runConfigEditInteractive(stdin io.Reader, stdout io.Writer, local bool) error {
	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintln(stdout, "Edit:")
	category, err := promptSelectCategory(stdout, stdin, allConfigCategories)
	if err != nil || category == "" {
		return err
	}

	names, err := loadNamesForCategory(category, config.ScopeFromLocal(local))
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

	return configEditByCategory(stdin, stdout, local, singular, selected)
}

// configEditByCategory dispatches to the appropriate category edit function.
func configEditByCategory(stdin io.Reader, stdout io.Writer, local bool, category, name string) error {
	switch category {
	case "agent":
		return configAgentEdit(stdin, stdout, local, name)
	case "role":
		return configRoleEdit(stdin, stdout, local, name)
	case "context":
		return configContextEdit(stdin, stdout, local, name)
	case "task":
		return configTaskEdit(stdin, stdout, local, name)
	}
	return fmt.Errorf("unknown category %q", category)
}

// configAgentEdit is the inner edit logic for agents.
func configAgentEdit(stdin io.Reader, stdout io.Writer, local bool, name string) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	// Resolve from all scopes so we find the agent regardless of which dir it lives in.
	allAgents, _, err := loadAgentsForScope(config.ScopeFromLocal(local))
	if err != nil {
		return fmt.Errorf("loading agents: %w", err)
	}
	resolvedName, agent, err := resolveInstalledName(allAgents, "agent", name)
	if err != nil {
		return err
	}

	// Edit in the dir where the agent actually lives.
	configDir := paths.Dir(agent.Source == "local")
	agentPath := filepath.Join(configDir, "agents.cue")
	dirAgents, _, err := loadAgentsFromDir(configDir)
	if err != nil {
		return fmt.Errorf("loading agents: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Editing agent %q %s\n\n", resolvedName, tui.Annotate("press Enter to keep current value"))

	newBin, err := promptString(stdout, stdin, "Binary", agent.Bin)
	if err != nil {
		return err
	}
	if newBin == "" {
		newBin = agent.Bin
	}

	newCommand, err := promptString(stdout, stdin, "Command template", agent.Command)
	if err != nil {
		return err
	}
	if newCommand == "" {
		newCommand = agent.Command
	}

	newDescription, err := promptString(stdout, stdin, "Description", agent.Description)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout)
	newModels, err := promptModels(stdout, stdin, agent.Models)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout)
	newDefaultModel, err := promptDefaultModel(stdout, stdin, agent.DefaultModel, newModels)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout)
	newTags, err := promptTags(stdout, stdin, agent.Tags, true)
	if err != nil {
		return err
	}

	agent.Bin = newBin
	agent.Command = newCommand
	agent.DefaultModel = newDefaultModel
	agent.Description = newDescription
	agent.Models = newModels
	agent.Tags = newTags
	dirAgents[resolvedName] = agent

	if err := writeAgentsFile(agentPath, dirAgents); err != nil {
		return fmt.Errorf("writing agents file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "\nUpdated agent %q\n", resolvedName)
	return nil
}

// configRoleEdit is the inner edit logic for roles.
func configRoleEdit(stdin io.Reader, stdout io.Writer, local bool, name string) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	allRoles, _, err := loadRolesForScope(config.ScopeFromLocal(local))
	if err != nil {
		return fmt.Errorf("loading roles: %w", err)
	}
	resolvedName, role, err := resolveInstalledName(allRoles, "role", name)
	if err != nil {
		return err
	}

	configDir := paths.Dir(role.Source == "local")
	rolePath := filepath.Join(configDir, "roles.cue")
	dirRoles, order, err := loadRolesFromDir(configDir)
	if err != nil {
		return fmt.Errorf("loading roles: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Editing role %q %s\n\n", resolvedName, tui.Annotate("press Enter to keep current value"))

	newDescription, err := promptString(stdout, stdin, "Description", role.Description)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout, "\nCurrent content source:")
	if role.File != "" {
		_, _ = fmt.Fprintf(stdout, "  File: %s\n", role.File)
	}
	if role.Command != "" {
		_, _ = fmt.Fprintf(stdout, "  Command: %s\n", role.Command)
	}
	if role.Prompt != "" {
		_, _ = fmt.Fprintf(stdout, "  Prompt: %s\n", truncatePrompt(role.Prompt, 50))
	}

	_, _ = fmt.Fprintf(stdout, "Keep current? %s ", tui.Bracket("Y/n"))
	reader := bufio.NewReader(stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))

	newFile := role.File
	newCommand := role.Command
	newPrompt := role.Prompt

	if input == "n" || input == "no" {
		newFile, newCommand, newPrompt, err = promptContentSource(stdout, stdin, "1", role.Prompt)
		if err != nil {
			return err
		}
	}

	newOptional := role.Optional
	if newFile != "" {
		optBracket := "y/N"
		if role.Optional {
			optBracket = "Y/n"
		}
		_, _ = fmt.Fprintf(stdout, "\nOptional %s? %s ", tui.Annotate("currently %t", role.Optional), tui.Bracket("%s", optBracket))
		optInput, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		optInput = strings.TrimSpace(strings.ToLower(optInput))
		switch optInput {
		case "y", "yes":
			newOptional = true
		case "n", "no":
			newOptional = false
		}
	} else {
		newOptional = false
	}

	_, _ = fmt.Fprintln(stdout)
	newTags, err := promptTags(stdout, stdin, role.Tags, true)
	if err != nil {
		return err
	}

	role.Description = newDescription
	role.File = newFile
	role.Command = newCommand
	role.Prompt = newPrompt
	role.Tags = newTags
	role.Optional = newOptional
	dirRoles[resolvedName] = role

	if err := writeRolesFile(rolePath, dirRoles, order); err != nil {
		return fmt.Errorf("writing roles file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "\nUpdated role %q\n", resolvedName)
	return nil
}

// configContextEdit is the inner edit logic for contexts.
func configContextEdit(stdin io.Reader, stdout io.Writer, local bool, name string) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	allContexts, _, err := loadContextsForScope(config.ScopeFromLocal(local))
	if err != nil {
		return fmt.Errorf("loading contexts: %w", err)
	}
	resolvedName, ctx, err := resolveInstalledName(allContexts, "context", name)
	if err != nil {
		return err
	}

	configDir := paths.Dir(ctx.Source == "local")
	contextPath := filepath.Join(configDir, "contexts.cue")
	dirContexts, order, err := loadContextsFromDir(configDir)
	if err != nil {
		return fmt.Errorf("loading contexts: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Editing context %q %s\n\n", resolvedName, tui.Annotate("press Enter to keep current value"))

	newDescription, err := promptString(stdout, stdin, "Description", ctx.Description)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout, "\nCurrent content source:")
	if ctx.File != "" {
		_, _ = fmt.Fprintf(stdout, "  File: %s\n", ctx.File)
	}
	if ctx.Command != "" {
		_, _ = fmt.Fprintf(stdout, "  Command: %s\n", ctx.Command)
	}
	if ctx.Prompt != "" {
		_, _ = fmt.Fprintf(stdout, "  Prompt: %s\n", truncatePrompt(ctx.Prompt, 50))
	}

	_, _ = fmt.Fprintf(stdout, "Keep current? %s ", tui.Bracket("Y/n"))
	reader := bufio.NewReader(stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))

	newFile := ctx.File
	newCommand := ctx.Command
	newPrompt := ctx.Prompt

	if input == "n" || input == "no" {
		newFile, newCommand, newPrompt, err = promptContentSource(stdout, stdin, "1", ctx.Prompt)
		if err != nil {
			return err
		}
	}

	_, _ = fmt.Fprintf(stdout, "\nRequired %s? %s ", tui.Annotate("currently %t", ctx.Required), tui.Bracket("y/N"))
	input, err = reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))
	newRequired := ctx.Required
	switch input {
	case "y", "yes":
		newRequired = true
	case "n", "no":
		newRequired = false
	}

	newDefault := ctx.Default
	if !newRequired {
		_, _ = fmt.Fprintf(stdout, "Default %s? %s ", tui.Annotate("currently %t", ctx.Default), tui.Bracket("y/N"))
		input, err = reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading input: %w", err)
		}
		input = strings.TrimSpace(strings.ToLower(input))
		switch input {
		case "y", "yes":
			newDefault = true
		case "n", "no":
			newDefault = false
		}
	}

	_, _ = fmt.Fprintln(stdout)
	newTags, err := promptTags(stdout, stdin, ctx.Tags, true)
	if err != nil {
		return err
	}

	ctx.Description = newDescription
	ctx.File = newFile
	ctx.Command = newCommand
	ctx.Prompt = newPrompt
	ctx.Required = newRequired
	ctx.Default = newDefault
	ctx.Tags = newTags
	dirContexts[resolvedName] = ctx

	if err := writeContextsFile(contextPath, dirContexts, order); err != nil {
		return fmt.Errorf("writing contexts file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "\nUpdated context %q\n", resolvedName)
	return nil
}

// configTaskEdit is the inner edit logic for tasks.
func configTaskEdit(stdin io.Reader, stdout io.Writer, local bool, name string) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	allTasks, _, err := loadTasksForScope(config.ScopeFromLocal(local))
	if err != nil {
		return fmt.Errorf("loading tasks: %w", err)
	}
	resolvedName, task, err := resolveInstalledName(allTasks, "task", name)
	if err != nil {
		return err
	}

	configDir := paths.Dir(task.Source == "local")
	taskPath := filepath.Join(configDir, "tasks.cue")
	dirTasks, _, err := loadTasksFromDir(configDir)
	if err != nil {
		return fmt.Errorf("loading tasks: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "Editing task %q %s\n\n", resolvedName, tui.Annotate("press Enter to keep current value"))

	newDescription, err := promptString(stdout, stdin, "Description", task.Description)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout, "\nCurrent content source:")
	if task.File != "" {
		_, _ = fmt.Fprintf(stdout, "  File: %s\n", task.File)
	}
	if task.Command != "" {
		_, _ = fmt.Fprintf(stdout, "  Command: %s\n", task.Command)
	}
	if task.Prompt != "" {
		_, _ = fmt.Fprintf(stdout, "  Prompt: %s\n", truncatePrompt(task.Prompt, 50))
	}

	_, _ = fmt.Fprintf(stdout, "Keep current? %s ", tui.Bracket("Y/n"))
	reader := bufio.NewReader(stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(strings.ToLower(input))

	newFile := task.File
	newCommand := task.Command
	newPrompt := task.Prompt

	if input == "n" || input == "no" {
		newFile, newCommand, newPrompt, err = promptContentSource(stdout, stdin, "3", task.Prompt)
		if err != nil {
			return err
		}
	}

	newRole, err := promptString(stdout, stdin, "Role", task.Role)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(stdout)
	newTags, err := promptTags(stdout, stdin, task.Tags, true)
	if err != nil {
		return err
	}

	task.Description = newDescription
	task.File = newFile
	task.Command = newCommand
	task.Prompt = newPrompt
	task.Role = newRole
	task.Tags = newTags
	dirTasks[resolvedName] = task

	if err := writeTasksFile(taskPath, dirTasks); err != nil {
		return fmt.Errorf("writing tasks file: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "\nUpdated task %q\n", resolvedName)
	return nil
}
