package cli

import (
	"bufio"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

// addConfigOrderCommand adds the bare order command to the config command.
func addConfigOrderCommand(parent *cobra.Command) {
	orderCmd := &cobra.Command{
		Use:     "order [category]",
		Aliases: []string{"reorder"},
		Short:   "Reorder configuration items",
		Long: `Reorder roles or contexts interactively.

Provide a category (role, context) or omit it to be prompted
interactively. Non-orderable categories (agent, task) fall back
to the interactive menu.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigOrder,
	}

	parent.AddCommand(orderCmd)
}

// resolveOrderCategory maps a category argument to the canonical reorder target.
// Returns ("roles"/"contexts", true) for orderable categories, ("", true) for known
// non-orderable categories (agent, task), and ("", false) for unknown categories.
func resolveOrderCategory(arg string) (string, bool) {
	singular := strings.TrimSuffix(strings.ToLower(arg), "s")
	switch singular {
	case "role":
		return "roles", true
	case "context":
		return "contexts", true
	case "agent", "task":
		return "", true
	}
	return "", false
}

// runConfigOrder prompts the user to select roles or contexts, then runs reorder.
func runConfigOrder(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	stdin := cmd.InOrStdin()
	if !isTerminal(stdin) {
		return fmt.Errorf("interactive reordering requires a terminal")
	}

	stdout := cmd.OutOrStdout()
	local := getFlags(cmd).Local

	category := ""
	if len(args) > 0 {
		// Known non-orderable categories (agent, task) fall back silently; unknown categories get an error.
		var known bool
		category, known = resolveOrderCategory(args[0])
		if category == "" && !known {
			_, _ = fmt.Fprintf(stdout, "unknown category %q\n", args[0])
		}
	}

	if category == "" {
		_, _ = fmt.Fprintln(stdout)
		_, _ = fmt.Fprintln(stdout, "Reorder:")
		var err error
		category, err = promptSelectCategory(stdout, stdin, []string{"roles", "contexts"})
		if err != nil || category == "" {
			return err
		}
	} else {
		_, _ = fmt.Fprintln(stdout)
	}

	switch category {
	case "roles":
		return reorderRoles(stdout, stdin, local)
	case "contexts":
		return reorderContexts(stdout, stdin, local)
	}
	return nil
}

// reorderContexts performs the interactive context reorder.
func reorderContexts(stdout io.Writer, stdin io.Reader, local bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(local)

	contexts, order, err := loadContextsFromDir(configDir)
	if err != nil {
		return fmt.Errorf("loading contexts: %w", err)
	}

	if len(order) == 0 {
		_, _ = fmt.Fprintln(stdout, "No contexts configured.")
		return nil
	}

	contextPath := filepath.Join(configDir, "contexts.cue")
	heading := fmt.Sprintf("Reorder Contexts %s:", tui.Annotate("%s - %s", scopeString(local), contextPath))

	formatItem := func(i int, name string) string {
		ctx := contexts[name]
		markers := ""
		if ctx.Required {
			markers += " " + tui.Bracket("required")
		}
		if ctx.Default {
			markers += " " + tui.Bracket("default")
		}
		return fmt.Sprintf("  %d. %s%s", i+1, name, markers)
	}

	newOrder, saved, err := runReorderLoop(stdout, stdin, heading, order, formatItem)
	if err != nil {
		return err
	}

	if !saved {
		_, _ = fmt.Fprintln(stdout, "Cancelled.")
		return nil
	}

	if err := writeContextsFile(contextPath, contexts, newOrder); err != nil {
		return fmt.Errorf("writing contexts file: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "Order saved.")
	return nil
}

// reorderRoles performs the interactive role reorder.
func reorderRoles(stdout io.Writer, stdin io.Reader, local bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(local)

	roles, order, err := loadRolesFromDir(configDir)
	if err != nil {
		return fmt.Errorf("loading roles: %w", err)
	}

	if len(order) == 0 {
		_, _ = fmt.Fprintln(stdout, "No roles configured.")
		return nil
	}

	rolePath := filepath.Join(configDir, "roles.cue")
	heading := fmt.Sprintf("Reorder Roles %s:", tui.Annotate("%s - %s", scopeString(local), rolePath))

	formatItem := func(i int, name string) string {
		role := roles[name]
		markers := ""
		if role.Optional {
			markers += " " + tui.Bracket("optional")
		}
		return fmt.Sprintf("  %d. %s%s", i+1, name, markers)
	}

	newOrder, saved, err := runReorderLoop(stdout, stdin, heading, order, formatItem)
	if err != nil {
		return err
	}

	if !saved {
		_, _ = fmt.Fprintln(stdout, "Cancelled.")
		return nil
	}

	if err := writeRolesFile(rolePath, roles, newOrder); err != nil {
		return fmt.Errorf("writing roles file: %w", err)
	}

	_, _ = fmt.Fprintln(stdout, "Order saved.")
	return nil
}

// runReorderLoop runs the interactive move-up reorder loop.
// Returns the final order, whether the user saved (true) or cancelled (false), and any error.
func runReorderLoop(w io.Writer, r io.Reader, heading string, order []string, formatItem func(int, string) string) ([]string, bool, error) {
	// Make a copy to avoid mutating the caller's slice
	current := make([]string, len(order))
	copy(current, order)

	reader := bufio.NewReader(r)

	// Display initial list
	_, _ = fmt.Fprintln(w, heading)
	_, _ = fmt.Fprintln(w)
	for i, name := range current {
		_, _ = fmt.Fprintln(w, formatItem(i, name))
	}

	for {
		_, _ = fmt.Fprintln(w)
		_, _ = fmt.Fprintf(w, "Move up %s, Enter to save, q to cancel: ", tui.Annotate("number"))

		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, false, fmt.Errorf("reading input: %w", err)
		}
		input = strings.TrimSpace(input)

		// Empty input: save
		if input == "" {
			return current, true, nil
		}

		// Cancel
		lower := strings.ToLower(input)
		if lower == "q" || lower == "quit" || lower == "exit" {
			return nil, false, nil
		}

		// Try to parse as number
		num, err := strconv.Atoi(input)
		if err != nil {
			_, _ = fmt.Fprintf(w, "Invalid input: %s\n", input)
			continue
		}

		if num < 1 || num > len(current) {
			_, _ = fmt.Fprintf(w, "Invalid number: %d %s\n", num, tui.Annotate("must be 1-%d", len(current)))
			continue
		}

		if num == 1 {
			_, _ = fmt.Fprintln(w, "Already at top.")
			continue
		}

		// Swap item at position num with the one above it (num-1)
		idx := num - 1
		current[idx], current[idx-1] = current[idx-1], current[idx]

		// Re-display
		_, _ = fmt.Fprintln(w)
		for i, name := range current {
			_, _ = fmt.Fprintln(w, formatItem(i, name))
		}
	}
}
