package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/shell"
)

// addGetCommand registers the `start get` subcommand.
func addGetCommand(parent *cobra.Command, flags *Flags) {
	getCmd := &cobra.Command{
		Use:     "get [name]",
		GroupID: "commands",
		Short:   "Output module content to stdout",
		Long: `Output the resolved content of a module to stdout for piping or preview.

Searches across all categories (agents, roles, contexts, tasks) and writes the
module's content to stdout. Names may be bare (e.g. "agents-md") or fully
qualified as "category:name" (e.g. "contexts:cwd/agents-md"); the category
prefix scopes the search to a single category. UTD modules (roles, contexts,
tasks) are template-resolved: file contents are read, prompts are rendered,
and commands are executed. Agent modules emit the command template with static
placeholders ({{.bin}}, {{.model}}) substituted while runtime placeholders
({{.prompt}}, {{.role}}, {{.role_file}}, {{.datetime}}) are left intact. The
--model flag, when set, overrides the agent's default_model in the
{{.model}} substitution.

Source priority for UTD modules is file > prompt > command. When a UTD module
defines both file and prompt, get outputs the file. During role/task/context
rendering by 'start' or 'start task', behaviour differs: the prompt is rendered
and file contents are injected via {{.file_contents}}, command output via
{{.command_output}}. So for mixed-field modules, get's output will not match
what 'start' renders into the agent prompt — use 'start describe' to inspect the
prompt.

Stdout receives only the module content. Selection menus, registry progress,
auto-install notices, and --verbose metadata are written to stderr so the
output remains pipe-clean.

Use --global to restrict resolution to the global config (~/.config/start/) or
--local to restrict to the local config (./.start/). These flags are mutually
exclusive; omitting both resolves against the merged configuration.

Auto-installed modules always land in global config; the post-install lookup
widens to merged scope so a --local invocation can still see the new module.
To inspect strictly within --local, ensure the module is already installed.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runGet,
	}

	// Bind --global to flags.Global. AddCommand must run before
	// MarkFlagsMutuallyExclusive so cobra's mergePersistentFlags() can see
	// the inherited --local from root via VisitParents.
	getCmd.Flags().BoolVar(&flags.Global, "global", false, "Restrict to global config only")
	parent.AddCommand(getCmd)
	getCmd.MarkFlagsMutuallyExclusive("local", "global")
}

// runGet resolves a module and writes its content to stdout.
func runGet(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()
	stdin := cmd.InOrStdin()

	query, err := getResolveQuery(args, stderr, stdin)
	if err != nil {
		return err
	}
	if query == "" {
		return nil
	}

	flags := getFlags(cmd)
	scope := scopeFromFlags(flags)
	cfg, err := loadConfig(scope)
	if err != nil {
		return err
	}

	// Construct the resolver with stderr in the stdout slot so registry fetch
	// progress, auto-install notices, and selection menus do not corrupt the
	// piped content on stdout. See cross_resolve.go's doc comment.
	r := newResolver(cfg, flags, stderr, stderr, stdin)
	match, err := resolveCrossCategory(query, r)
	if err != nil {
		return err
	}

	// Refresh the in-memory config after an auto-install so the freshly
	// installed module's CUE value is visible. Same pattern as start and task.
	//
	// reloadConfig always reloads in merged scope regardless of the user's
	// original --local/--global flag. This is deliberate: autoInstall always
	// writes to global config (resolve.go's autoInstall), so merged is the
	// smallest scope guaranteed to see the new module for any original scope.
	// Under --global the result is identical (module only exists in global,
	// merged is a superset); under --local the widening is required for the
	// lookup to succeed and is signalled to the user via
	// notifyScopeWidenedIfLocal. A scope-aware reload would be a no-op
	// distinction for --global today.
	if r.didInstall {
		workingDir, wdErr := os.Getwd()
		if wdErr != nil {
			return fmt.Errorf("getting working directory: %w", wdErr)
		}
		if err := r.reloadConfig(workingDir); err != nil {
			return err
		}
		cfg = r.cfg
		notifyScopeWidenedIfLocal(stderr, flags, r.didInstall)
	}

	cat := describeCategoryFor(match.Category)
	if cat == nil {
		return fmt.Errorf("unknown category %q", match.Category)
	}

	items := cfg.Value.LookupPath(cue.ParsePath(cat.key))
	item := items.LookupPath(cue.MakePath(cue.Str(match.Name)))
	if !item.Exists() {
		return fmt.Errorf("%s %q not found", strings.ToLower(cat.itemType), match.Name)
	}

	if cat.itemType == "Agent" {
		return getAgent(stdout, stderr, flags, r, match.Name, item)
	}
	return getUTD(stdout, stderr, flags, match.Name, cat.itemType, item)
}

// getResolveQuery returns the module query, prompting interactively when no
// argument was supplied. All prompts and warnings go to stderr to keep stdout
// reserved for module content (Requirement 5).
func getResolveQuery(args []string, stderr io.Writer, stdin io.Reader) (string, error) {
	if len(args) == 0 {
		if !isTerminal(stdin) {
			return "", fmt.Errorf("name required in non-interactive mode")
		}
		return promptSearchQuery(stderr, stdin)
	}

	query := args[0]
	if len(query) >= 3 {
		return query, nil
	}
	if !isTerminal(stdin) {
		return "", fmt.Errorf("query must be at least 3 characters")
	}
	_, _ = fmt.Fprintln(stderr, "Query must be at least 3 characters")
	return promptSearchQuery(stderr, stdin)
}

// getAgent writes the agent's command template (with {{.bin}} and {{.model}}
// resolved) to stdout. Runtime placeholders are left intact.
//
// When --model is set, it is resolved via resolver.resolveModelName (exact,
// then multi-term substring, then passthrough) to keep `get` consistent with
// `start`'s rendering of the same flag.
func getAgent(stdout, stderr io.Writer, flags *Flags, r *resolver, name string, item cue.Value) error {
	cmdField := item.LookupPath(cue.ParsePath("command"))
	command := ""
	if cmdField.Exists() {
		command, _ = cmdField.String()
	}
	if command == "" {
		return fmt.Errorf("agent %q has no command (or empty command field)", name)
	}

	if flags.Verbose {
		printGetVerbose(stderr, "Agent", name, item, "", "", false)
	}

	modelOverride := ""
	if flags.Model != "" {
		agent, err := orchestration.ExtractAgent(r.cfg.Value, name)
		if err != nil {
			return fmt.Errorf("loading agent %q for --model resolution: %w", name, err)
		}
		modelOverride = r.resolveModelName(flags.Model, agent)
	}

	rendered := partialFillAgentCommand(command, item, modelOverride)
	_, _ = fmt.Fprint(stdout, ensureTrailingNewline(rendered))
	return nil
}

// getUTD resolves a UTD module and writes its content to stdout. Source
// priority is file > prompt > command. The TemplateProcessor's intrinsic
// priority is the inverse (prompt > file > command, see template.go); the
// trim block below flips it by clearing higher-priority sources before Process
// runs. Shell and Timeout are execution config and pass through untouched so a
// command-source module still honours its declared shell and timeout.
func getUTD(stdout, stderr io.Writer, flags *Flags, name, itemType string, item cue.Value) error {
	fields := orchestration.ExtractUTDFields(item)
	if !orchestration.IsUTDValid(fields) {
		return fmt.Errorf("module %q has no content fields (expected one of: file, prompt, command)", name)
	}

	resolvedFile := ""
	fromModuleCache := false
	if fields.File != "" {
		if strings.HasPrefix(fields.File, "@module/") {
			fromModuleCache = true
			origin := orchestration.ExtractOrigin(item)
			if origin == "" {
				return fmt.Errorf("module %q has @module/ file path but no origin field", name)
			}
			resolved, err := orchestration.ResolveModulePath(fields.File, origin)
			if err != nil {
				return fmt.Errorf("resolving module path %s: %w", fields.File, err)
			}
			fields.File = resolved
		}
		// Expand ~/ and relative paths so verbose `Path:`/`Cache:` reports the
		// same location DefaultFileReader will read from. @module/ is already
		// absolute by this point. On expansion failure (rare), keep the
		// literal config string and log the cause under --debug so the
		// misleading verbose Path: line is diagnosable.
		resolvedFile = fields.File
		if expanded, expandErr := orchestration.ExpandFilePath(fields.File); expandErr == nil {
			resolvedFile = expanded
		} else {
			debugf(stderr, flags, dbgResolve, "expanding %s: %v", fields.File, expandErr)
		}
	}

	// Source-priority dependency: see TemplateProcessor.Process in
	// internal/orchestration/template.go. Process picks Prompt before File, so
	// clearing Prompt when File is set is what makes get's file > prompt
	// priority hold. Clearing Command in the file and prompt branches is
	// deliberate side-effect suppression: it disables Process's lazy
	// {{.command_output}} expansion (template.go: needsCommandOutput &&
	// fields.Command != "") so `get` never shells out unless command is the
	// primary source. Do not extend this trim to Shell or Timeout — they
	// configure command execution and apply regardless of which source wins.
	if fields.File != "" {
		fields.Prompt = ""
		fields.Command = ""
	} else if fields.Prompt != "" {
		fields.Command = ""
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Verbose runs after the trim block so fields.Command reflects the
	// chosen source (only set when command is the active source). No I/O
	// happens between the trim and here, so the verbose lines are still
	// emitted before any read or shell-out.
	if flags.Verbose {
		printGetVerbose(stderr, itemType, name, item, resolvedFile, fields.Command, fromModuleCache)
	}

	fr := &orchestration.DefaultFileReader{}
	sr := shell.NewRunner()
	processor := orchestration.NewTemplateProcessor(fr, sr, workingDir)

	result, err := processor.Process(fields, "")
	if err != nil {
		return err
	}

	_, _ = fmt.Fprint(stdout, ensureTrailingNewline(result.Content))
	return nil
}

// printGetVerbose writes module metadata to stderr ahead of the content. Used
// when --verbose is set; stdout remains reserved for the module content itself.
// command is set only when command is the active source — getUTD passes the
// post-trim fields.Command, which is non-empty exactly when command was chosen.
// fromModuleCache labels the file location as `Cache:` (matching `start describe`)
// so users aren't misled into editing the CUE module cache; local-file modules
// keep the `Path:` label so the user knows where the editable source lives.
func printGetVerbose(stderr io.Writer, itemType, name string, item cue.Value, resolvedFile, command string, fromModuleCache bool) {
	_, _ = fmt.Fprintf(stderr, "Type: %s\n", itemType)
	_, _ = fmt.Fprintf(stderr, "Name: %s\n", name)
	if origin := orchestration.ExtractOrigin(item); origin != "" {
		_, _ = fmt.Fprintf(stderr, "Origin: %s\n", origin)
	}
	if resolvedFile != "" {
		label := "Path"
		if fromModuleCache {
			label = "Cache"
		}
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", label, resolvedFile)
	}
	if command != "" {
		_, _ = fmt.Fprintf(stderr, "Command: %s\n", command)
	}
}

// ensureTrailingNewline returns s with exactly one trailing newline. Empty
// strings pass through. Used at every get write site so stdout is line-aligned
// regardless of which module source produced the content.
func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
