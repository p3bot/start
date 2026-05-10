# Rename show command to describe

## Goal

Rename the `start show` command to `start describe`. The verb `describe` is the conventional name (kubectl, AWS CLI) for the rich human-readable detail dump that `show` produces, and `show` is in the never-canonical column for `get` in the project's CLI Design for Agents spec. The command's behaviour does not change.

## Scope

In scope:

- Rename the cobra command from `show` to `describe` and drop the `view` alias
- Rename `internal/cli/show.go` and `internal/cli/show_test.go` to `describe.go` and `describe_test.go`
- Rename all `show`-rooted Go identifiers (commands, helpers, exported types, fields, test names) declared in those files
- Update all comments, doc-comments, and help text that name the command or its identifiers
- Update cross-file callers and comment references in `internal/cli/root.go`, `internal/cli/cross_resolve.go`, the read/get command files (see Current State), and their tests
- Update the help-text reference to `start show` inside the read/get command's `Long:` text
- Update documentation in `README.md`, `AGENTS.md`, and `internal/cli/help/agents.md`
- Update shell scripts under `scripts/` that invoke `start show`
- Hard cutover with no `show` or `view` alias preserved

Out of scope:

- Splitting the command's overloaded behaviour (no-arg listing vs single-name describe). The overloading is preserved; the rename does not address it.
- Folding `start config info` or `start modules info` into the renamed command.
- Behaviour changes (output format, flags, resolution logic, error messages other than command-name wording).
- The `read` → `get` rename in project `01-read-to-get.md`. The two projects are independent and may be implemented in either order; cross-references between them are noted in Current State.
- Releases or tags.

## Current State

The command is defined at `internal/cli/show.go` and registered from `internal/cli/root.go` via `addShowCommand(cmd)`. It has alias `view`. The command operates in two modes dispatched on argument count: with no argument it lists all configured items grouped by category with descriptions, and with an argument it cross-category resolves the name and prints a verbose dump (config source path, registry origin, cache path, structured CUE, and rendered file content).

Identifier inventory (from `rg -n` against `internal/cli/show.go` and `internal/cli/show_test.go`):

Command and handlers:

- `addShowCommand` — registration entry point
- `runShow` — RunE handler
- `runShowListing` — no-arg listing branch
- `runShowSearch` — single-name describe branch

Result type and field:

- `ShowResult` — exported struct returned by `prepareShow`
- `ShowReason` — field on `ShowResult`

Helpers:

- `prepareShow` — assembles a `ShowResult` for a given name and scope
- `showVerboseItem` — calls `prepareShow` and prints the dump
- `showScopeFromCmd` — derives `config.Scope` from `--global` / `--local` flags (also called from `read.go`)
- `resolveShowFile` — resolves `@module/` and other file references for the verbose dump

Category metadata (used by multiple files in the package):

- `showCategory` — struct
- `showCategories` — package-level variable
- `showCategoryFor` — lookup helper

Tests:

- `TestShow*` — test functions in `show_test.go`
- `TestPrepareShow*` — test functions in `show_test.go`

Identifiers in `show.go` that are NOT show-rooted and must NOT be renamed:

- `parseAddress`, `formatAddress`, `parsedAddress`, `knownCategoriesList`
- `notifyScopeWidenedIfLocal`
- `printVerboseDump`, `findConfigSource`, `formatCUEDefinition`, `loadConfig`

Cross-file Go references to the renamed identifiers:

- `internal/cli/root.go` — call to `addShowCommand`
- `internal/cli/cross_resolve.go` — uses `showCategories` and `showCategoryFor`; one comment paragraph names `show` and `showVerboseItem` / `prepareShow`
- `internal/cli/read.go` — calls `showScopeFromCmd`, `showCategoryFor`, and `notifyScopeWidenedIfLocal`; one help-text sentence references `start show`; comments reference `runShowSearch` and `show.go`
- `internal/cli/read_test.go` — comments reference `notifyScopeWidenedIfLocal (show.go)`, `TestShowGlobalFlag in show_test.go`, and `showScopeFromCmd`

If project `01-read-to-get.md` has completed before this project starts, `internal/cli/read.go` and `internal/cli/read_test.go` are named `get.go` and `get_test.go`. The implementer should locate the cross-file callers by searching for the renamed identifiers rather than relying on the original filenames; the verification commands below use `rg` for this reason.

Documentation references:

- `README.md` lines 207, 211, 214, 257, 260 — `start show` invocations in user-facing prose and code blocks. Line 217 mentions `--global` and `--local` without naming the command and does not require an edit. The phrases "Show raw config fields" (line 192), "Show the full registry catalog" (line 269), and "Show details for a specific module" (line 275) use "Show" as an English verb describing different commands and must not be changed.
- `AGENTS.md` lines 40-43 — four lines naming `start show <category>` (see Issues Discovered)
- `internal/cli/help/agents.md` lines 26-29 — `start show` invocations in the agent help reference

Shell scripts:

- `scripts/manual-test` — `start show` invocations around lines 143 and 537-558, including a "view alias" test case that must be removed when the alias is dropped
- `scripts/test-config-commands.sh` — `start show` invocations around lines 347-439. The argument `test-show` (line 359) is a module name that contains the substring "show" and must not be renamed. The `--scope global` and `--scope local` arguments at lines 374-439 are pre-existing bugs (the actual flag is `--global` / `--local`) and are out of scope.

## References

- CLI Design for Agents spec at https://github.com/start-cli/library/issues/2 — the canonical vocabulary that motivates this rename. Rule 9 places `show` in the never-canonical column for `get`. `describe` is widely established as the human-readable detail-dump verb (kubectl, AWS CLI) and is the deliberate non-canonical exception this project documents.

## Requirements

1. The cobra command `Use` is `describe [name]`. There is no `Aliases:` field. Invoking `start show` or `start view` returns the standard cobra unknown-command error.
2. `internal/cli/show.go` is renamed to `internal/cli/describe.go` and `internal/cli/show_test.go` to `internal/cli/describe_test.go`. Package declarations are unchanged.
3. All `show`-rooted identifiers from the inventory above are renamed to their `describe`-rooted equivalents. The implementer chooses the natural mapping (`addShowCommand` → `addDescribeCommand`, `runShow` → `runDescribe`, `ShowResult` → `DescribeResult`, `prepareShow` → `prepareDescribe`, etc.). No `show`-rooted identifier from the inventory remains in the codebase.
4. The category metadata identifiers (`showCategory`, `showCategories`, `showCategoryFor`) are renamed to their `describe`-rooted equivalents and every caller in the package is updated.
5. The exported `ShowResult` struct is renamed to `DescribeResult` and its `ShowReason` field is renamed to `DescribeReason`. Any external users of these names within the workspace are updated.
6. The cobra `Long:` help text and inline doc-comments are updated so wording reflects the new command name.
7. The help-text sentence in the read/get command that reads "use 'start show' to inspect the prompt" is updated to "use 'start describe' to inspect the prompt".
8. All `TestShow*` and `TestPrepareShow*` function names are renamed to their `Describe` equivalents. All `cmd.SetArgs` invocations using `"show"` are updated to `"describe"`. Test case `name:` strings that use lowercase English "show"/"shows" as a verb describing behaviour may be left as-is at the implementer's discretion; only references to the literal command must change.
9. Cross-file references in `internal/cli/root.go`, `internal/cli/cross_resolve.go`, the read/get command file, and the read/get test file are updated to use the renamed identifiers and the new command name.
10. Documentation is updated:
    - `README.md` lines 207, 211, 214, 257, 260 use `start describe`
    - `AGENTS.md` lines 40-43 are substituted mechanically (`start show` → `start describe`); the per-line comment text is preserved unchanged (see Issues Discovered)
    - `internal/cli/help/agents.md` lines 26-29 use `start describe`
11. Shell scripts under `scripts/` that invoke `start show` are updated to `start describe`. The "view alias" test case in `scripts/manual-test` is removed. Module names containing the substring "show" (e.g., `test-show`) are not renamed.
12. Final verification: `rg -n 'addShowCommand|runShowListing|runShowSearch|runShow\b|showVerboseItem|prepareShow|showScopeFromCmd|resolveShowFile|showCategory|showCategories|showCategoryFor|ShowResult|ShowReason|TestShow|TestPrepareShow' .` returns no matches outside this project document. `rg -n 'start show\b|start view\b' .` returns no matches outside this project document.

## Implementation Plan

1. File rename
   Move `internal/cli/show.go` to `internal/cli/describe.go` and `internal/cli/show_test.go` to `internal/cli/describe_test.go`. Package declaration stays `cli`.

2. Identifier rename in renamed files
   Apply the mapping in requirements 3-5 to `describe.go` and `describe_test.go`. Drop the `Aliases: []string{"view"}` field on the cobra command. Apply the `TestShow*` and `TestPrepareShow*` mappings. Update `cmd.SetArgs([]string{"show", ...})` to `"describe"`.

3. Cobra command surface
   Set `Use: "describe [name]"`. Confirm no `Aliases:` field is present. Update the `Short:` and `Long:` strings so wording reflects the new command name.

4. Cross-file Go references
   Use `rg -n 'addShowCommand|showScopeFromCmd|showCategoryFor|showCategories|runShowSearch|ShowResult|ShowReason'` to locate every caller. Update each. Specifically:
   - `internal/cli/root.go` — registration call
   - `internal/cli/cross_resolve.go` — variable use, lookup call, and the comment paragraph that names `show`
   - The read/get command file (`read.go` or `get.go` after project 01) — three call sites and the comment references
   - The read/get test file — three comment references
   - The read/get command's `Long:` help text — the sentence referencing `start show`

5. Documentation
   Edit `README.md`, `AGENTS.md`, and `internal/cli/help/agents.md` per requirement 10. For `AGENTS.md` lines 40-43, perform the mechanical substitution only; do not rewrite the misleading per-line comments.

6. Scripts
   Update `scripts/manual-test` and `scripts/test-config-commands.sh` per requirement 11. Remove the `view alias` test case in `manual-test`. Leave the `test-show` module-name argument and the pre-existing `--scope` bugs untouched.

7. Verify
   Run the two `rg` queries from requirement 12 and confirm zero matches outside this project document. Audit any remaining English uses of "show" in the touched files — leave incidental prose verbs (e.g., "showing", "shows that") alone; resolve anything that names the command or a renamed identifier.

8. Build and test
   Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Hard cutover. No `show` or `view` alias on the cobra command. No backward-compatibility shim.
- Pure rename. The command's behaviour, flags, output format, error handling, and help-text content (other than the command name itself) are unchanged.
- The `notifyScopeWidenedIfLocal`, `printVerboseDump`, `findConfigSource`, `formatCUEDefinition`, `loadConfig`, `parseAddress`, `formatAddress`, `parsedAddress`, and `knownCategoriesList` identifiers are general-purpose and are NOT renamed despite living in the renamed file.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- "Show" appears in three distinct senses in the touched files: naming the CLI command ("start show", "the show command"), as an English verb in prose comments and test-case name strings ("listing shows only global items"), and as part of module names like `test-show` in scripts. Only the first sense must be substituted. Audit each occurrence rather than blanket-replacing.
- The category metadata identifiers (`showCategory`, `showCategories`, `showCategoryFor`) are not specific to the show/describe command — they describe the four module categories and are used from `cross_resolve.go` and the read/get command. The `describe`-rooted rename keeps the identifiers coupled to their original file location, which is the simplest mapping. A semantically cleaner name (e.g., dropping the prefix entirely) is out of scope for this project; raise it as a follow-up if it seems worth doing.
- Project `01-read-to-get.md` may run before, after, or never. If `01` has completed, `read.go` is `get.go` and the call sites this project must update have moved with the file but kept the same identifier names (`showScopeFromCmd`, `showCategoryFor`, etc., which are this project's responsibility to rename). Locating the call sites via `rg` is reliable in either ordering.
- The cobra `--global` flag on the renamed command keeps the description "Show from global scope only" today. Update it to "Describe from global scope only" or similar so the verb form matches the command name.

## Issues Discovered

1. AGENTS.md describes `start show <category>` as a category listing (gap) — Resolved: substitute mechanically; do not rewrite.

   Lines 40-43 of `AGENTS.md` claim that `start show agents` lists installed agents, `start show roles` lists installed roles, and so on for `tasks` and `contexts`. The actual behaviour, confirmed by `show_test.go` (`TestShowCommandIntegration` test cases at lines 998-1018) and by direct invocation, is that `start show <category>` performs a cross-category name search for a module literally named `agents` (or `roles`, etc.) and either errors as ambiguous or returns not-found. There is no per-category listing operation under `show`; the no-argument form lists all categories together.

   The misleading documentation predates this project and is not introduced by the rename. Fixing it requires either splitting the overloaded command (so a true `list <category>` form exists) or correcting the documentation to describe the actual behaviour. Both options are out of scope for a pure rename.

   Resolution: substitute `start show` → `start describe` mechanically on these four lines and leave the `# Show installed …` comments untouched. Flag the underlying overload for a follow-up project that addresses the listing-vs-describe split.

## Acceptance Criteria

- `start --help` lists `describe` and does not list `show` or `view`.
- `start describe` (no arg) and `start describe <name>` produce the same output as the previous `start show` and `start show <name>` did before this project.
- `start show` and `start view` exit with cobra's unknown-command error.
- `internal/cli/describe.go` and `internal/cli/describe_test.go` exist; `internal/cli/show.go` and `internal/cli/show_test.go` do not.
- `rg -n 'addShowCommand|runShowListing|runShowSearch|runShow\b|showVerboseItem|prepareShow|showScopeFromCmd|resolveShowFile|showCategory|showCategories|showCategoryFor|ShowResult|ShowReason|TestShow|TestPrepareShow' .` returns no matches outside this project document.
- `rg -n 'start show\b|start view\b' .` returns no matches outside this project document.
- `README.md`, `AGENTS.md`, and `internal/cli/help/agents.md` reference `start describe` (with `AGENTS.md` lines 40-43 substituted mechanically per Issues Discovered).
- `scripts/manual-test` and `scripts/test-config-commands.sh` invoke `start describe` and no longer test the `view` alias.
- `scripts/invoke-tests` passes.
