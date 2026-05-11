package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
)

// ModuleInfoResult combines a search result with installation status for JSON output.
type ModuleInfoResult struct {
	Category       string              `json:"category"`
	Name           string              `json:"name"`
	Entry          registry.IndexEntry `json:"entry"`
	MatchScore     int                 `json:"matchScore"`
	Installed      bool                `json:"installed"`
	InstalledScope string              `json:"installedScope,omitempty"`
}

// addModulesInfoCommand adds the info subcommand to the modules command.
func addModulesInfoCommand(parent *cobra.Command) {
	infoCmd := &cobra.Command{
		Use:   "info [query]...",
		Short: "Show module details",
		Long: `Show detailed information about a module.

Searches for the module in the registry index and displays full details
including description, module path, tags, and installation status.
Multiple words are combined with AND logic.

Use --json to output machine-readable JSON.`,
		Args: cobra.MinimumNArgs(0),
		RunE: runModulesInfo,
	}

	infoCmd.Flags().Bool("json", false, "Output as JSON")
	parent.AddCommand(infoCmd)
}

// runModulesInfo shows detailed information about a module.
func runModulesInfo(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	jsonFlag, _ := cmd.Flags().GetBool("json")

	prompted := false
	if len(args) == 0 {
		if jsonFlag {
			return fmt.Errorf("query is required with --json")
		}
		input, err := promptSearchQuery(cmd.OutOrStdout(), cmd.InOrStdin())
		if err != nil {
			return err
		}
		if input == "" {
			return nil
		}
		args = []string{input}
		prompted = true
	}

	query := strings.Join(args, " ")
	if len(query) < 3 {
		if jsonFlag {
			return fmt.Errorf("query must be at least 3 characters")
		}
		w := cmd.OutOrStdout()
		stdin := cmd.InOrStdin()
		if !isTerminal(stdin) {
			return fmt.Errorf("query must be at least 3 characters")
		}
		_, _ = fmt.Fprintln(w, "Query must be at least 3 characters")
		input, err := promptSearchQuery(w, stdin)
		if err != nil {
			return err
		}
		if input == "" {
			return nil
		}
		query = input
		prompted = true
	}

	ctx := context.Background()
	flags := getFlags(cmd)

	prog := tui.NewProgress(cmd.ErrOrStderr(), flags.Quiet)
	defer prog.Done()

	index, _, err := fetchIndex(ctx, cmd, prog, "Fetching index...")
	if err != nil {
		return err
	}
	prog.Done()

	results, err := modules.SearchIndex(index, query, nil)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if len(results) == 0 {
		if jsonFlag {
			if err := writeJSON(w, []ModuleInfoResult{}); err != nil {
				return fmt.Errorf("marshalling module info: %w", err)
			}
			return nil
		}
		if prompted {
			_, _ = fmt.Fprintf(w, "No modules found matching %q\n", query)
			return nil
		}
		return fmt.Errorf("no modules found matching %q", query)
	}

	if jsonFlag {
		installedScopes := collectInstalledScopes()
		var infoResults []ModuleInfoResult
		for _, r := range results {
			key := r.Category + "/" + r.Name
			ir := ModuleInfoResult{
				Category:       r.Category,
				Name:           r.Name,
				Entry:          r.Entry,
				MatchScore:     r.MatchScore,
				Installed:      installedScopes[key] != "",
				InstalledScope: installedScopes[key],
			}
			infoResults = append(infoResults, ir)
		}
		if err := writeJSON(w, infoResults); err != nil {
			return fmt.Errorf("marshalling module info: %w", err)
		}
		return nil
	}

	var selected modules.SearchResult
	if len(results) == 1 {
		selected = results[0]
	} else {
		stdin := cmd.InOrStdin()
		if isTerminal(stdin) {
			pick, err := promptModuleInfoSelection(w, stdin, results, query)
			if err != nil {
				return err
			}
			if pick == nil {
				return nil
			}
			selected = *pick
		} else {
			selected = results[0]
			if !flags.Quiet {
				_, _ = fmt.Fprintf(w, "Showing first of %d matches. Use 'start modules search %s' to see all.\n\n", len(results), query)
			}
		}
	}

	installed, installedScope := checkIfInstalled(selected)

	printModuleInfo(w, selected, installed, installedScope, flags.Verbose)

	return nil
}

// checkIfInstalled checks if a module is installed in the config.
func checkIfInstalled(mod modules.SearchResult) (bool, string) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return false, ""
	}

	if !paths.AnyExists() {
		return false, ""
	}

	dirs := paths.ForScope(config.ScopeMerged)
	loader := internalcue.NewLoader()
	cfg, err := loader.Load(dirs)
	if err != nil {
		return false, ""
	}

	var localCfg cue.Value
	if paths.LocalExists {
		if v, loadErr := loader.LoadSingle(paths.Local); loadErr == nil {
			localCfg = v
		}
	}

	installed := collectInstalledModules(cfg.Value, paths, localCfg)
	for _, a := range installed {
		if a.Category == mod.Category && a.Name == mod.Name {
			return true, a.Scope
		}
	}

	return false, ""
}

// printModuleInfo prints detailed information about a module.
func printModuleInfo(w io.Writer, mod modules.SearchResult, installed bool, scope string, verbose bool) {
	_, _ = fmt.Fprintln(w)
	_, _ = tui.CategoryColor(mod.Category).Fprint(w, mod.Category)
	_, _ = fmt.Fprintf(w, ":%s\n", mod.Name)
	printSeparator(w)

	_, _ = tui.ColorDim.Fprint(w, "Type:")
	_, _ = fmt.Fprintf(w, " %s\n", mod.Category)
	_, _ = tui.ColorDim.Fprint(w, "Module:")
	_, _ = fmt.Fprintf(w, " %s\n", mod.Entry.Module)

	if mod.Entry.Description != "" {
		_, _ = fmt.Fprintln(w)
		_, _ = tui.ColorDim.Fprint(w, "Description:")
		_, _ = fmt.Fprintf(w, " %s\n", mod.Entry.Description)
	}

	if len(mod.Entry.Tags) > 0 {
		_, _ = tui.ColorDim.Fprint(w, "Tags:")
		_, _ = fmt.Fprintf(w, " %s\n", strings.Join(mod.Entry.Tags, ", "))
	}

	_, _ = fmt.Fprintln(w)
	if installed {
		_, _ = tui.ColorInstalled.Fprint(w, "✓")
		_, _ = fmt.Fprintf(w, " Installed %s\n", tui.Annotate("%s", scope))
	} else {
		_, _ = fmt.Fprintln(w, "  Not installed")
	}

	if mod.Entry.Version != "" {
		_, _ = tui.ColorDim.Fprint(w, "Version:")
		_, _ = fmt.Fprintf(w, " %s\n", mod.Entry.Version)
	}

	printSeparator(w)

	if !installed {
		_, _ = fmt.Fprintf(w, "\nUse 'start modules install %s' to install.\n", formatAddress(mod.Category, mod.Name))
	}
}

// promptModuleInfoSelection shows a numbered list of module matches and lets the
// user pick one. Returns nil and nil if the user cancels (empty input).
func promptModuleInfoSelection(w io.Writer, r io.Reader, results []modules.SearchResult, query string) (*modules.SearchResult, error) {
	_, _ = fmt.Fprintf(w, "\nFound %d matches for %q:\n\n", len(results), query)

	for i, res := range results {
		_, _ = fmt.Fprintf(w, "  %2d. ", i+1)
		_, _ = tui.CategoryColor(res.Category).Fprint(w, res.Category)
		_, _ = fmt.Fprintf(w, ":%s ", res.Name)
		_, _ = tui.ColorDim.Fprintf(w, "- %s", res.Entry.Description)
		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "Select %s: ", tui.Annotate("1-%d", len(results)))

	reader := bufio.NewReader(r)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}
	input = strings.TrimSpace(input)

	if input == "" {
		return nil, nil
	}

	n, err := strconv.Atoi(input)
	if err != nil || n < 1 || n > len(results) {
		return nil, fmt.Errorf("invalid selection %q: enter a number between 1 and %d", input, len(results))
	}
	return &results[n-1], nil
}
