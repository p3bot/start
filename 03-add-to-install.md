# Rename modules add subcommand to install

## Goal

Rename the `start modules add` subcommand to `start modules install`. The operation fetches a module from the CUE registry and registers it in the user's configuration so it is immediately usable — that is the textbook "install" semantic shared by apt, brew, pip, gem, npm, cargo install, and helm. The current verb `add` is in the never-canonical column for `create` in the project's CLI Design for Agents spec, and it pairs awkwardly with the existing `start modules update` command. The command's behaviour does not change.

## Scope

In scope:

- Rename the cobra subcommand from `add` to `install` and drop the `install` alias (which currently aliases the canonical `add`)
- Rename `internal/cli/modules_add.go` to `internal/cli/modules_install.go`
- Rename the two `Add`-rooted Go identifiers declared in that file
- Update the cross-file caller in `internal/cli/modules.go`
- Update Go test fixtures and assertions that reference the literal subcommand string `"add"`
- Update documentation in `README.md`, `AGENTS.md`, and `internal/cli/help/agents.md`
- Update shell scripts under `scripts/` that invoke `start modules add`
- Hard cutover with no `add` alias preserved

Out of scope:

- The other subcommand renames in the `modules` group (`info` removal is planned for project 04)
- A symmetric `uninstall` subcommand. None exists today; introducing one is a separate project
- Behaviour changes (output format, flags, resolution logic, error messages other than command-name wording)
- The flattening project that may later promote this command to top-level `start install`
- Releases or tags

## Current State

The subcommand is defined at `internal/cli/modules_add.go` and registered from `internal/cli/modules.go` via `addModulesAddCommand(modulesCmd)`. The cobra command has `Use: "add [query]..."` and `Aliases: []string{"install"}` — so `start modules install` already works today as an alias for the canonical `add`. The `Short:` and `Long:` help text both already use the verb "Install" in prose ("Install modules from registry", "Install one or more modules from the CUE registry…"), so most of the user-facing wording is consistent with the new canonical name.

The operation:

1. Searches the registry index for the supplied query (or prompts for one in interactive mode)
2. Selects a single match, or prompts the user to choose from multiple matches
3. Fetches the module via the CUE registry client, populating the local cache
4. Writes a config entry to the appropriate scope (global by default, local with `--local`) so the module is immediately available to other `start` commands

Identifier inventory (from `rg -n` against `internal/cli/modules_add.go`):

Add-rooted identifiers that must be renamed:

- `addModulesAddCommand` — registration entry point
- `runModulesAdd` — RunE handler

Identifiers declared in the same file that are already correctly named or not Add-rooted, and must NOT be renamed:

- `installModule` — already named for the install operation
- `installSingleModule` — already named for the install operation
- `promptModuleSelection` — generic prompt helper
- `errNoModules` — generic error sentinel; the comment above it ("returned by installModule when no matching modules are found") is already correctly worded

Cross-file Go references:

- `internal/cli/modules.go` line 69 — call to `addModulesAddCommand(modulesCmd)`
- `internal/cli/modules_test.go` line 215 — `subcommands := []string{"browse", "index", "search", "add", "list", "info", "update"}` asserts the registered subcommand set; the literal `"add"` must become `"install"`
- `internal/cli/root_test.go` line 182 — `args: []string{"modules", "add", "help"}` must become `"install"`
- `test/integration/modules_test.go` line 460 — `want: []string{"Manage modules", "browse", "search", "add", "list", "info", "update"}` asserts the help output enumerates these subcommands; the literal `"add"` must become `"install"`
- `test/integration/modules_test.go` lines 468-469 — a test case named `"modules add help"` with args `[]string{"modules", "add", "--help"}` must be updated to `"install"` in both name and args

There is no dedicated `modules_add_test.go` file; coverage lives in the files listed above.

Documentation references:

- `README.md` lines 118, 119, 144, 168, 169, 279, 280 — `start modules add <name>` invocations in user-facing prose and code blocks
- `AGENTS.md` line 45 — `start modules add <pkg>` in the command-list table
- `internal/cli/help/agents.md` line 44 — `start modules add golang/code-review` in the agent help reference

Shell scripts:

- `scripts/test-supporting-commands.sh` lines 169-176 — three `modules add` invocations including a `skip_test` and a `run_test_show` block
- `scripts/show-help` line 53 — `show "${BIN} modules add"`
- `scripts/manual-test` lines 157-158 and 634-638 — `modules add --help` and two manual-test cases (`modules add (direct path)` and `modules add --local …`)

## References

- CLI Design for Agents spec at https://github.com/start-cli/library/issues/2 — Rule 9 places `add` in the never-canonical column for `create`. The `install` verb is not in the never column for any verb and is the textbook domain-specific exception for package-management operations that fetch and register an artifact.

## Requirements

1. The cobra subcommand `Use` is `install [query]...`. There is no `Aliases:` field. Invoking `start modules add` returns the standard cobra unknown-command error.
2. `internal/cli/modules_add.go` is renamed to `internal/cli/modules_install.go`. The package declaration is unchanged.
3. The two Add-rooted identifiers `addModulesAddCommand` and `runModulesAdd` are renamed to `addModulesInstallCommand` and `runModulesInstall`. No Add-rooted identifier from the inventory remains in the codebase.
4. The cross-file call site in `internal/cli/modules.go` uses the renamed registration function.
5. Every test fixture and assertion that contains the literal subcommand string `"add"` (in `internal/cli/modules_test.go`, `internal/cli/root_test.go`, `test/integration/modules_test.go`) is updated to `"install"`. Test names that incorporate `add` are renamed to use `install`.
6. The cobra `Short:` and `Long:` help text are reviewed; the existing prose already uses "Install" and is largely correct. Only wording that names the canonical command form (e.g., "the add subcommand") needs updating.
7. The doc-comment on `addModulesInstallCommand` is updated to reflect the new canonical name.
8. Documentation is updated:
   - `README.md` — every `start modules add <name>` occurrence becomes `start modules install <name>`
   - `AGENTS.md` line 45 — `start modules add <pkg>` becomes `start modules install <pkg>`; the trailing comment text ("Install a module from the library") needs no change
   - `internal/cli/help/agents.md` line 44 — `start modules add golang/code-review` becomes `start modules install golang/code-review`
9. Shell scripts are updated:
   - `scripts/test-supporting-commands.sh` — three `modules add` invocations
   - `scripts/show-help` — the `${BIN} modules add` call
   - `scripts/manual-test` — every `modules add` invocation; test-case labels (the strings in the first argument to `cmd`) updated to use `install`
10. Final verification: `rg -n 'addModulesAddCommand|runModulesAdd' .` returns no matches outside this project document. `rg -n 'modules add\b' .` returns no matches outside this project document. `rg -n '"add"' internal/cli/modules_test.go internal/cli/root_test.go test/integration/modules_test.go` returns no matches.

## Implementation Plan

1. File rename
   Move `internal/cli/modules_add.go` to `internal/cli/modules_install.go`. Package declaration stays `cli`.

2. Identifier rename in renamed file
   Rename `addModulesAddCommand` → `addModulesInstallCommand` and `runModulesAdd` → `runModulesInstall`. Update doc-comments accordingly. Leave `installModule`, `installSingleModule`, `promptModuleSelection`, and `errNoModules` unchanged.

3. Cobra command surface
   Set `Use: "install [query]..."`. Drop the `Aliases:` field entirely. Confirm `Short:` and `Long:` text reads correctly with the new canonical name (no edits expected beyond verifying nothing names the old form).

4. Cross-file Go references
   Use `rg -n 'addModulesAddCommand|"modules", "add"|"add"' internal/cli/ test/` to locate every caller and fixture. Update:
   - The registration call in `internal/cli/modules.go`
   - The subcommand-set assertion in `internal/cli/modules_test.go`
   - The args fixture in `internal/cli/root_test.go`
   - The help-output assertion and the test case in `test/integration/modules_test.go`

5. Documentation
   Edit `README.md`, `AGENTS.md`, and `internal/cli/help/agents.md` per requirement 8.

6. Scripts
   Edit `scripts/test-supporting-commands.sh`, `scripts/show-help`, and `scripts/manual-test` per requirement 9. Test-case labels in `scripts/manual-test` (e.g., `"modules add (direct path)"`) become `"modules install (direct path)"`.

7. Verify
   Run the three `rg` queries from requirement 10 and confirm zero matches outside this project document.

8. Build and test
   Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Hard cutover. No `add` alias on the cobra subcommand. No backward-compatibility shim.
- Pure rename. The subcommand's behaviour, flags, output format, error handling, and prose help text (other than the command name itself) are unchanged.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not introduce a symmetric `uninstall` subcommand. None exists today and introducing one is out of scope.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- The cobra command currently defines `Aliases: []string{"install"}`, which means `start modules install` already works today as an alias of the canonical `add`. The rename swaps which is canonical and drops the alias entirely. Removing the `Aliases:` field is a one-line edit with no semantic surprise — the alias was already pointing at the operation users will now invoke directly.
- The internal helpers (`installModule`, `installSingleModule`) were already named for the install verb. This is not coincidence — the engineer who wrote them knew the operation was an install regardless of what the subcommand was called. Leaving them alone keeps the diff small and focused on the user-facing rename.
- The English word "add" appears in some test-case labels in `scripts/manual-test` (e.g., the descriptive label `"modules add (direct path)"`). These labels are user-facing strings printed by the test runner; update them so the printed output matches the new command name.

## Acceptance Criteria

- `start modules --help` lists `install` as a subcommand and does not list `add`.
- `start modules install <query>` produces the same output as `start modules add <query>` did before this project.
- `start modules add` exits with cobra's unknown-command error.
- `internal/cli/modules_install.go` exists; `internal/cli/modules_add.go` does not.
- `rg -n 'addModulesAddCommand|runModulesAdd' .` returns no matches outside this project document.
- `rg -n 'modules add\b' .` returns no matches outside this project document.
- `README.md`, `AGENTS.md`, `internal/cli/help/agents.md`, and the shell scripts under `scripts/` reference `start modules install`.
- `scripts/invoke-tests` passes.
