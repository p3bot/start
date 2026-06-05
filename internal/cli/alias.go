package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/tui"
)

// addAliasCommand wires the alias command family. Aliases are global-only
// personal shortcuts: a leading token that expands into a saved start command.
func addAliasCommand(parent *cobra.Command) {
	aliasCmd := &cobra.Command{
		Use:     "alias",
		Aliases: []string{"aliases"},
		GroupID: "workflow",
		Short:   "Manage personal command aliases",
		Long: `Manage personal aliases: short tokens that expand into saved start commands.

An alias value is the start command minus the leading "start" — saved verbatim
and spliced back in when you run "start <alias> ...". Aliases are global only.

  start alias set pc task review/pre-commit
  start pc                 runs start task review/pre-commit
  start pc "fix the lint"  passes the text through as task instructions
  start alias set dev --role go-expert --context cwd/agents-md
  start dev                runs start --role go-expert --context cwd/agents-md`,
		RunE: runAliasList,
	}

	parent.AddCommand(aliasCmd)

	addAliasListCommand(aliasCmd)
	addAliasSetCommand(aliasCmd)
	addAliasGetCommand(aliasCmd)
	addAliasDeleteCommand(aliasCmd)
	addAliasOpenCommand(aliasCmd)
	addAliasExportCommand(aliasCmd)
	addAliasImportCommand(aliasCmd)
}

func addAliasListCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all aliases",
		Args:    noArgsOrHelp,
		RunE:    runAliasList,
	}
	parent.AddCommand(cmd)
}

func addAliasSetCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "set <name> <token>...",
		Short: "Create or update an alias",
		Long: `Create or update an alias. Everything after the name is captured verbatim
as the value, so flags and subcommands are stored exactly as typed.

  start alias set pc task review/pre-commit
  start alias set rev task review/pre-commit --role go-expert --model opus
  start alias set foo prompt "this is the prompt"`,
		// Capture the value verbatim — flags in the value are data, not flags.
		DisableFlagParsing: true,
		RunE:               runAliasSet,
	}
	parent.AddCommand(cmd)
}

func addAliasGetCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show one alias as its expanded command",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runAliasGet,
	}
	parent.AddCommand(cmd)
}

func addAliasDeleteCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "delete <name>...",
		Aliases: []string{"rm"},
		Short:   "Delete one or more aliases",
		Args:    cobra.ArbitraryArgs,
		RunE:    runAliasDelete,
	}
	parent.AddCommand(cmd)
}

func addAliasOpenCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open the alias store in $EDITOR",
		Args:  noArgsOrHelp,
		RunE:  runAliasOpen,
	}
	parent.AddCommand(cmd)
}

func addAliasExportCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Print the alias store to stdout",
		Args:  noArgsOrHelp,
		RunE:  runAliasExport,
	}
	parent.AddCommand(cmd)
}

func addAliasImportCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "import [file]",
		Short: "Merge aliases from stdin or a file",
		Long: `Merge aliases from a CUE document read from stdin or a file. The document
has the same shape as export output: a top-level aliases map.

Default behaviour upserts each incoming alias, leaving others untouched, so
"start alias export | start alias import" is a no-op. Use --replace to replace
the entire store. Import is atomic: if any entry is invalid the whole import is
rejected and nothing is written.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runAliasImport,
	}
	cmd.Flags().Bool("replace", false, "Replace the entire store instead of merging")
	parent.AddCommand(cmd)
}

func runAliasList(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) > 0 {
		return unknownCommandError("start alias", args[0])
	}

	w := cmd.OutOrStdout()
	aliases, err := loadAliases()
	if err != nil {
		return err
	}

	if len(aliases) == 0 {
		fmt.Fprintln(w, tui.Annotate("no aliases set"))
		fmt.Fprintf(w, "Create one with %s.\n", tui.Annotate("start alias set <name> <command>"))
		return nil
	}

	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}
	for _, name := range names {
		tui.ColorTasks.Fprintf(w, "%-*s", maxLen, name)
		fmt.Fprintf(w, "  %s\n", expandedCommand(aliases[name]))
	}
	return nil
}

func runAliasSet(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) == 0 {
		return usageError(fmt.Errorf("alias set needs a name and a value, e.g. start alias set pc task review/pre-commit"))
	}

	name := config.NormalizeAliasName(args[0])
	tokens := args[1:]

	reserved := reservedCommandNames(cmd.Root())
	if err := validateAliasEntry(reserved, name, tokens); err != nil {
		return err
	}

	storePath, err := aliasStorePath()
	if err != nil {
		return err
	}

	aliases, err := currentAliases(storePath)
	if err != nil {
		return err
	}
	aliases[name] = tokens

	if err := config.WriteAliasStore(storePath, aliases); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Set alias %s\n", tui.ColorTasks.Sprint(name))
	fmt.Fprintf(w, "  %s\n", expandedCommand(tokens))
	return nil
}

func runAliasGet(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) == 0 {
		return usageError(fmt.Errorf("alias get needs a name, e.g. start alias get pc"))
	}

	name := config.NormalizeAliasName(args[0])
	aliases, err := loadAliases()
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	tokens, ok := aliases[name]
	if !ok {
		fmt.Fprintf(w, "%s %s\n", name, tui.Annotate("not set"))
		return nil
	}
	fmt.Fprintf(w, "%s  %s\n", tui.ColorTasks.Sprint(name), expandedCommand(tokens))
	return nil
}

func runAliasDelete(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	if len(args) == 0 {
		return usageError(fmt.Errorf("alias delete needs at least one name, e.g. start alias delete pc"))
	}

	storePath, err := aliasStorePath()
	if err != nil {
		return err
	}
	aliases, err := currentAliases(storePath)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	var deleted []string
	for _, raw := range args {
		name := config.NormalizeAliasName(raw)
		if _, ok := aliases[name]; ok {
			delete(aliases, name)
			deleted = append(deleted, name)
		} else {
			fmt.Fprintf(w, "%s %s\n", name, tui.Annotate("not set"))
		}
	}

	if len(deleted) == 0 {
		return nil
	}

	if err := config.WriteAliasStore(storePath, aliases); err != nil {
		return err
	}
	for _, name := range deleted {
		fmt.Fprintf(w, "Deleted alias %s\n", tui.ColorTasks.Sprint(name))
	}
	return nil
}

func runAliasOpen(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	storePath, err := aliasStorePath()
	if err != nil {
		return err
	}
	// open writes through an external editor, so it must create the directory
	// and seed an empty store itself; otherwise a first save fails.
	if err := config.EnsureAliasStore(storePath); err != nil {
		return err
	}
	return openInEditor(storePath)
}

func runAliasExport(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	storePath, err := aliasStorePath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(storePath)
	if os.IsNotExist(err) {
		// No store yet: emit an empty document so export | import stays a no-op.
		fmt.Fprintf(cmd.OutOrStdout(), "%s: {}\n", config.KeyAliases)
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading alias store: %w", err)
	}
	fmt.Fprint(cmd.OutOrStdout(), string(data))
	return nil
}

func runAliasImport(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	replace, _ := cmd.Flags().GetBool("replace")

	var data []byte
	if len(args) == 1 {
		raw, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading import file: %w", err)
		}
		data = raw
	} else {
		piped, isPiped, err := readPipedStdin(cmd.InOrStdin())
		if err != nil {
			return err
		}
		if !isPiped {
			return usageError(fmt.Errorf("alias import needs a file argument or piped stdin, e.g. start alias import other.cue"))
		}
		data = []byte(piped)
	}

	ctx := cuecontext.New()
	v := ctx.CompileBytes(data, cue.Filename("import.cue"))
	if v.Err() != nil {
		return usageError(fmt.Errorf("import document does not parse as CUE: %w", v.Err()))
	}
	if config.HasNonAliasTopLevelKeys(v) {
		return usageError(fmt.Errorf("import document has non-aliases top-level keys; it must contain only an aliases map"))
	}
	incoming, err := config.DecodeAliases(v)
	if err != nil {
		return usageError(fmt.Errorf("invalid import document: %w", err))
	}

	reserved := reservedCommandNames(cmd.Root())
	normalized := make(map[string][]string, len(incoming))
	seen := make(map[string]string, len(incoming))
	for name, tokens := range incoming {
		norm := config.NormalizeAliasName(name)
		// Names are case-insensitive, so two source keys can fold to one name.
		// The document is then ambiguous; reject it rather than letting Go's
		// map order silently pick a winner. Order the pair for a stable message.
		if prior, ok := seen[norm]; ok {
			first, second := prior, name
			if first > second {
				first, second = second, first
			}
			return usageError(fmt.Errorf("import has two aliases that normalize to %q (%q and %q); names are case-insensitive, so remove one", norm, first, second))
		}
		seen[norm] = name
		if err := validateAliasEntry(reserved, norm, tokens); err != nil {
			return err
		}
		normalized[norm] = tokens
	}

	storePath, err := aliasStorePath()
	if err != nil {
		return err
	}

	final := normalized
	if !replace {
		existing, err := currentAliases(storePath)
		if err != nil {
			return err
		}
		for name, tokens := range normalized {
			existing[name] = tokens
		}
		final = existing
	}

	if err := config.WriteAliasStore(storePath, final); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	verb := "Merged"
	if replace {
		verb = "Imported"
	}
	fmt.Fprintf(w, "%s %d %s\n", verb, len(normalized), pluralize("alias", "aliases", len(normalized)))
	return nil
}

// validateAliasEntry applies the shared name and value rules. Every rejection
// writes nothing and carries a concrete hint with a valid example.
func validateAliasEntry(reserved map[string]bool, name string, tokens []string) error {
	if name == "" {
		return usageError(fmt.Errorf("alias name must be a non-empty token, e.g. start alias set pc task review/pre-commit"))
	}
	if strings.HasPrefix(name, "-") {
		return usageError(fmt.Errorf("alias name %q must not start with '-', e.g. start alias set pc task review/pre-commit", name))
	}
	if strings.ContainsAny(name, " \t\n") {
		return usageError(fmt.Errorf("alias name %q must be a single token, e.g. start alias set pc task review/pre-commit", name))
	}
	if reserved[name] {
		return usageError(fmt.Errorf("%s is a built-in command; choose another name such as %s", name, suggestAliasName(name)))
	}
	if len(tokens) == 0 {
		return usageError(fmt.Errorf("alias %q needs a value, e.g. start alias set pc task review/pre-commit", name))
	}
	if tokens[0] == "start" {
		return usageError(fmt.Errorf("drop the leading start; the value is the command without it, e.g. start alias set pc task review/pre-commit"))
	}
	return nil
}

// suggestAliasName offers a short non-colliding example name for a hint.
func suggestAliasName(name string) string {
	if len(name) >= 3 {
		return name[:3]
	}
	return name + "-alias"
}

// expandedCommand renders the stored tokens as a copy-pasteable start command,
// shell-quoting each token so the displayed command matches what executes.
func expandedCommand(tokens []string) string {
	parts := make([]string, len(tokens)+1)
	parts[0] = "start"
	for i, tok := range tokens {
		parts[i+1] = shellQuote(tok)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	for _, r := range s {
		if !isShellSafe(r) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

func isShellSafe(r rune) bool {
	switch {
	case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return true
	default:
		return strings.ContainsRune("_-./:@%+=,", r)
	}
}

func pluralize(singular, plural string, n int) string {
	if n == 1 {
		return singular
	}
	return plural
}

// aliasStorePath resolves the global alias store path.
func aliasStorePath() (string, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return "", fmt.Errorf("resolving config paths: %w", err)
	}
	return config.AliasStorePath(paths), nil
}

// loadAliases returns the current alias map for display commands.
func loadAliases() (map[string][]string, error) {
	storePath, err := aliasStorePath()
	if err != nil {
		return nil, err
	}
	return currentAliases(storePath)
}

// currentAliases loads the alias map from the store, failing closed when the
// file exists but does not parse so a write never proceeds from a corrupt read.
func currentAliases(storePath string) (map[string][]string, error) {
	ctx := cuecontext.New()
	v, exists, err := config.CompileAliasStore(ctx, storePath)
	if err != nil {
		return nil, err
	}
	if !exists {
		return make(map[string][]string), nil
	}
	if v.Err() != nil {
		return nil, fmt.Errorf("alias store %s does not parse; fix it with start alias open or remove the file: %w", storePath, v.Err())
	}
	return config.DecodeAliases(v)
}
