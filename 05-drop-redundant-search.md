# Drop config search and modules search

## Goal

Delete `start config search` and `start modules search`. Their behaviour is fully covered by the top-level `start search`, which already queries local config, global config, and the registry index in a single invocation and groups results by source. Removing the two redundant subcommands reduces the surface, eliminates per-source verbs that an agent has to remember, and brings the search namespace down to one canonical command.

## Scope

In scope:

- Delete the cobra subcommands `start config search` and `start modules search`
- Delete the corresponding implementation files `internal/cli/config_search.go` and `internal/cli/modules_search.go`
- Relocate the helpers `collectInstalledNames` and `collectInstalledScopes` (currently defined in `modules_search.go`) to `internal/cli/search.go` so the remaining callers continue to compile
- Remove the `printSearchResults` helper (used only by the deleted `modules_search.go`)
- Remove the cobra registration calls in `internal/cli/config.go` and `internal/cli/modules.go`
- Delete the CLI-level tests for the removed subcommands
- Update the modules-subcommand-list test assertions to drop the `"search"` entry
- Update documentation (`README.md`, `AGENTS.md`, `internal/cli/help/agents.md`) and shell scripts to reference only the top-level `start search`

Out of scope:

- Any change to the top-level `start search` behaviour, output, or flags
- Any change to the `modules` package's `SearchIndex` / `SearchInstalledConfig` / `SearchCategoryEntries` functions or their tests in `internal/modules/search_test.go` — these underlying primitives are used by the top-level search and other commands
- Adding `--scope` / `--local` / `--global` / `--registry` filter flags to top-level `start search` to replace the per-source subcommands (see Issues Discovered)
- Any project that this is a prerequisite for or that is a prerequisite for it (this project is independent of 01, 02, 03, and 04)
- Releases or tags

## Current State

Three search commands exist today:

`start search <query>` (top-level, file `internal/cli/search.go`):

- Searches local config, global config, AND the registry index in one pass
- Groups results into sections by source (`local`, `global`, `registry`)
- Supports `--tag` for tag filtering, `--json` for envelope output, `--verbose` for richer per-result detail
- Marks installed modules in the registry section with a star
- Has `find` as an alias

`start config search <query>` (file `internal/cli/config_search.go`):

- Searches local and global config, EXCLUDING the registry
- Groups results by scope (`local`, `global`)
- Honours `--local` to restrict to local-only
- Supports `--tag` and `--json`
- Has `find` as an alias

`start modules search <query>` (file `internal/cli/modules_search.go`):

- Searches the registry index only
- Groups results by category (`agents`, `roles`, `contexts`, `tasks`)
- Supports `--tag` and `--json`
- Has `find` as an alias

Helpers in `modules_search.go` that are used outside the file and must survive deletion:

- `collectInstalledNames` — called from `search.go` line 215 and `modules_index.go` line 114
- `collectInstalledScopes` — called from `collectInstalledNames` and from `modules_info.go` line 128 (the `modules_info.go` caller is removed by project 04)

Helpers in `modules_search.go` that are used only inside the file and are deleted with it:

- `addModulesSearchCommand`, `runModulesSearch`
- `printSearchResults`

Helpers in `config_search.go` that are used only inside the file and are deleted with it:

- `addConfigSearchCommand`, `runConfigSearch`

The shared helpers `printSearchSections`, `searchSection`, and `shortenHome` live in `search.go` and remain in place.

Cross-file registration calls to remove:

- `internal/cli/config.go` line 34 — `addConfigSearchCommand(configCmd)`
- `internal/cli/modules.go` — registration call for `addModulesSearchCommand`

Test files affected:

- `internal/cli/config_test.go` — `TestConfigSearch_NoConfig` (line 1944), `TestConfigSearch_TooShortQuery` (line 1966), `TestConfigSearch_GlobalMatch` (line 1982), `TestConfigSearch_LocalFlag` (line 2023), `TestConfigSearch_TagFilter` (line 2080), `TestConfigSearchJSON_WithResults` (line 2412), `TestConfigSearchJSON_NoResults` (line 2461)
- `internal/cli/modules_test.go` — `TestPrintSearchResults` (line 126; tests a function being deleted), `TestModulesSearchValidation` (line 1023; tests the deleted subcommand), and the subcommand-set assertion at line 215 (drop `"search"`)
- `test/integration/modules_test.go` — the help-output assertion at line 460 (drop `"search"`), and the test case at lines 464 invoking `modules search --help`

Tests in `internal/modules/search_test.go` (the package-level search primitives) are NOT affected — those functions back the top-level `start search` and stay in use.

Documentation references to remove or rewrite:

- `README.md` line 273 (`start modules search go` example), line 333 (`start config search <query>` example), lines 349-350 (the side-by-side comparison block listing both deleted commands)
- `AGENTS.md` line 47 (`start search <term> # Search installed modules`) — the comment understates what top-level search does and is worth updating to reflect "installed config and registry" while this project touches the area
- `internal/cli/help/agents.md` lines 36 (`start config search golang`) and 43 (`start modules search golang`)

Shell scripts:

- `scripts/test-supporting-commands.sh` lines 154-167 — five `modules search` test cases
- `scripts/manual-test` lines 127-128 (`config search --help`), 154-155 (`modules search --help`), 382-386 (config search test cases), and the `modules search role` case around line 571

## References

- CLI Design for Agents spec at https://github.com/start-cli/library/issues/2 — Rule 9 lists `list` and (by extension via `list --filter`) the canonical pattern for filter-style search. Rule 12 (contract stability) treats subcommand removal as a breaking change requiring a major version bump; the project's pre-release status (no tagged release per `start/AGENTS.md`) makes the cutover safe today.

## Requirements

1. The cobra subcommands `start config search` and `start modules search` are removed. Invoking either returns the standard cobra unknown-command error. The top-level `start search` is unchanged in behaviour and remains the sole search command.
2. Files `internal/cli/config_search.go` and `internal/cli/modules_search.go` are deleted in full.
3. Before deleting `modules_search.go`, the helpers `collectInstalledNames` and `collectInstalledScopes` are relocated to `internal/cli/search.go` so the surviving callers (`search.go` itself, `internal/cli/modules_index.go`) continue to compile. The function bodies are unchanged.
4. The `printSearchResults` helper (used only by `modules_search.go`) is deleted along with the file. The `TestPrintSearchResults` test in `internal/cli/modules_test.go` is deleted with it.
5. The cobra registration calls `addConfigSearchCommand(configCmd)` (in `internal/cli/config.go`) and `addModulesSearchCommand(modulesCmd)` (in `internal/cli/modules.go`) are removed.
6. The seven `TestConfigSearch_*` and `TestConfigSearchJSON_*` functions in `internal/cli/config_test.go` are deleted. The `TestModulesSearchValidation` function in `internal/cli/modules_test.go` is deleted.
7. The subcommand-set assertions at `internal/cli/modules_test.go` line 215 and `test/integration/modules_test.go` line 460 drop the `"search"` entry. The `modules search --help` test case at `test/integration/modules_test.go` line 464 is deleted.
8. Documentation is updated:
   - `README.md` — the `start modules search` example, the `start config search` example, and the side-by-side comparison block listing both deleted commands are removed or rewritten as `start search` examples
   - `AGENTS.md` line 47 — wording is refreshed so the comment reflects that top-level search covers installed config AND the registry
   - `internal/cli/help/agents.md` — the two lines invoking the deleted commands are removed; the existing `start search` examples remain
9. Shell scripts are updated:
   - `scripts/test-supporting-commands.sh` — the five `modules search` test cases at lines 154-167 are deleted (or rewritten as top-level `start search` cases if the implementer judges the coverage worth keeping; the existing `start search` cases in `scripts/manual-test` already exercise the same paths)
   - `scripts/manual-test` — the two `--help` cases for the deleted commands, the two `config search` test cases, and the `modules search` case in the modules section are deleted
10. Final verification: `rg -n 'addConfigSearchCommand|runConfigSearch|addModulesSearchCommand|runModulesSearch|printSearchResults|TestConfigSearch|TestModulesSearchValidation|TestPrintSearchResults' .` returns no matches outside this project document. `rg -n 'config search\b|modules search\b' .` returns no matches outside this project document.

## Implementation Plan

1. Relocate the surviving helpers
   Move `collectInstalledNames` and `collectInstalledScopes` from `internal/cli/modules_search.go` into `internal/cli/search.go`. The function bodies, signatures, and doc-comments are unchanged. Confirm that `internal/cli/modules_index.go` (the other caller of `collectInstalledNames`) compiles without imports adjustment — both files are in the same package.

2. Delete `config_search.go`
   Remove `internal/cli/config_search.go` in full. Remove the `addConfigSearchCommand(configCmd)` call from `internal/cli/config.go`.

3. Delete `modules_search.go`
   Remove `internal/cli/modules_search.go` in full (the helpers having already been relocated in step 1). Remove the `addModulesSearchCommand(modulesCmd)` call from `internal/cli/modules.go`.

4. Delete affected tests
   Delete the seven `TestConfigSearch_*` and `TestConfigSearchJSON_*` functions in `internal/cli/config_test.go`. Delete `TestPrintSearchResults` and `TestModulesSearchValidation` in `internal/cli/modules_test.go`. Update the subcommand-set assertion at `internal/cli/modules_test.go` line 215 to drop `"search"`. Update the help-output assertion at `test/integration/modules_test.go` line 460 to drop `"search"` and delete the `modules search --help` test case.

5. Documentation
   Edit `README.md`, `AGENTS.md`, and `internal/cli/help/agents.md` per requirement 8.

6. Scripts
   Edit `scripts/test-supporting-commands.sh` and `scripts/manual-test` per requirement 9.

7. Verify
   Run the two `rg` queries from requirement 10 and confirm zero matches outside this project document. Manually invoke `start config --help` and `start modules --help` and confirm `search` is absent. Manually invoke `start search <query>` and confirm behaviour is unchanged.

8. Build and test
   Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Hard cutover. No `config search` or `modules search` aliases. No backward-compatibility shims.
- The top-level `start search` command, its flags, its output, and its tests are unchanged. This project removes overlapping commands; it does not extend or modify the surviving one.
- The `internal/modules` package's search primitives (`SearchIndex`, `SearchInstalledConfig`, `SearchCategoryEntries`) are unchanged. Their tests in `internal/modules/search_test.go` are unchanged.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- The relocation of `collectInstalledNames` and `collectInstalledScopes` is a copy-paste plus delete-from-original. No logic change. Both functions are package-private in package `cli`; they can live in any file in the package without affecting callers. `search.go` is the natural home because it is the primary remaining caller.
- The `find` alias is currently shared by all three search commands. After this project, the `find` alias survives only on the top-level `start search` (which already declares it). No alias work is needed on the top-level command.
- The README's side-by-side comparison block (lines 349-350) explains when to use `config search` versus `modules search` versus the unified `start search`. After this project the comparison is moot — `start search` is the only option. The cleanest rewrite is to delete the comparison entirely; the README's earlier `start search` example already covers what users need.
- After projects 03, 04, and 05 land, the `modules` subcommand-list assertion at `internal/cli/modules_test.go` line 215 reads `{"browse", "index", "list", "install", "update"}`. Each project owns its own deletion from the list; this project is responsible only for dropping `"search"`.
- The script-level `modules search` test cases in `scripts/test-supporting-commands.sh` exercise short-query validation, no-results handling, verbose output, and tag filtering. Equivalent coverage already exists for the top-level `start search` in `scripts/manual-test` lines 294-312. The implementer may delete the supporting-commands cases outright rather than rewrite them as top-level cases — the coverage is duplicative.

## Issues Discovered

1. Loss of local-only scope filtering on search (decision) — Resolved: accept the loss; do not extend top-level search in this project.

   `start config search --local` is the only invocation today that restricts search to project-local config (`./.start/`), excluding global config and the registry. After this project, no invocation of any search command can produce a local-only result set; the top-level `start search` always queries all three sources and groups them, leaving the user to scan the relevant section.

   Two ways forward exist: (a) accept the loss, since the grouped output makes per-source filtering a visual operation and the local section is at the top of the output; or (b) extend top-level `start search` with a `--scope` (or `--local` / `--global` / `--registry`) filter flag in this project so the local-only use case survives the rename.

   The user has previously accepted option (a) as the working plan: "Drop it [config search]; the unified `start search` covers the use case — its current output already groups by source." This project follows that decision. If the local-only use case proves common enough in practice to justify the flag, a follow-up project can add `--scope` to top-level search; doing it here would expand scope beyond a clean drop.

   Resolution: drop both subcommands without replacement. Note in the README that the top-level search groups by source so users can scan to the local section.

## Acceptance Criteria

- `start config --help` does not list `search` as a subcommand.
- `start modules --help` does not list `search` as a subcommand.
- `start config search` and `start modules search` exit with cobra's unknown-command error.
- `start search <query>` produces the same output as before this project.
- `internal/cli/config_search.go` and `internal/cli/modules_search.go` do not exist.
- `internal/cli/search.go` defines `collectInstalledNames` and `collectInstalledScopes`; `internal/cli/modules_index.go` continues to compile and its tests pass.
- `rg -n 'addConfigSearchCommand|runConfigSearch|addModulesSearchCommand|runModulesSearch|printSearchResults|TestConfigSearch|TestModulesSearchValidation|TestPrintSearchResults' .` returns no matches outside this project document.
- `rg -n 'config search\b|modules search\b' .` returns no matches outside this project document.
- README.md, AGENTS.md, `internal/cli/help/agents.md`, and the shell scripts under `scripts/` reference only `start search`.
- `scripts/invoke-tests` passes.
