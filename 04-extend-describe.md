# Fold modules info into start describe; enrich describe metadata

## Goal

Make `start describe <name>` the single command for inspecting modules — installed or registry-only — by reusing the same `resolveCrossCategory` resolution that `start get` already uses (including registry-fuzzy match and the 3-char query guard), and by adding a formatted metadata block to the verbose dump so common fields are scannable without parsing CUE. Delete `start modules info`. `start config info` is unchanged; a follow-up project will rename it to `start config get`.

## Scope

In scope:

- Augment `printVerboseDump` with a formatted metadata block between Cache and the CUE Definition section. Surfaces the structured fields that `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` emit today.
- Wire `start describe <name>` through `resolveCrossCategory` (already done post-02; confirm).
- Delete `start modules info` (entire subcommand at `internal/cli/modules_info.go`).
- Remove registration calls, tests, help-table entries, and stale comments referencing `modules info`.
- Update documentation (`README.md`, `AGENTS.md`, `internal/cli/help/agents.md`) and scripts to point at `start describe` where they reference `modules info`.

Out of scope:

- `--json` for `start describe`. The existing `--json` surface (`config info`, `config list`, `config search`, `config settings`, `modules list`, `modules search`, `modules index`, `modules update`, `modules validate`, `search`, `doctor`) covers the machine-readable inspection cases today; a structured resolved-merged view via `start describe --json` is deferred to a separate project if a concrete consumer emerges.
- `start config info`. Left in place for a follow-up project that renames it to `start config get`. No changes to `internal/cli/config_info.go`, `runConfigInfo`, the `print*Info` formatters, or related tests in this project.
- The no-argument listing form of `start describe`. Behaviour is unchanged; the fold applies only to the named-argument form.
- Renaming or relocating `start describe`.
- The `start search` / `start config search` overlap.
- Any change under `library/` or `homebrew-tap/`.
- Releases or tags.

## Current State

This project assumes project `02-show-to-describe.md` has completed. `internal/cli/show.go` does not exist; `internal/cli/describe.go` is in place. `runDescribe` and `runDescribeSearch` are the post-02 handlers.

Two commands currently overlap with describe for inspection:

`start describe <name>` (post-project-02):

- Resolution: `resolveCrossCategory` — exact installed → substring installed → exact registry (auto-install) → combined installed-substring + registry-fuzzy (auto-install on single, menu on multi). 3-char query guard at `describe.go:154`. Auto-install writes to global config; `notifyScopeWidenedIfLocal` warns under `--local`.
- Multi-match: interactive menu via `promptModuleSelection`, error in non-interactive.
- Output: verbose dump (`printVerboseDump`) — header, Config, Origin, Cache, CUE Definition, File path, Path, file contents, Command.
- JSON: no support.
- Scope: `--global` / `--local` (mutually exclusive).

`start modules info <name>` (file: `internal/cli/modules_info.go`):

- Resolution: AND-word fuzzy match across the registry index; no auto-install; works on uninstalled modules.
- Multi-match: numbered menu via `promptModuleInfoSelection` in interactive mode; first of N in non-interactive.
- Output: `printModuleInfo` — Type, Module, Description, Tags, install status, Version, install hint when not installed.
- JSON: `--json` returns `[]ModuleInfoResult` (exported struct in `modules_info.go`). Deleted with the file; no replacement in this project.
- Scope: not applicable.

`start config info <name>` is not addressed by this project. Its file, helpers, tests, and registration remain unchanged.

### Helper inventory

Helpers in `internal/cli/modules_info.go` (delete with the file):

- `addModulesInfoCommand`, `runModulesInfo`
- `ModuleInfoResult` (exported struct)
- `checkIfInstalled`, `printModuleInfo`, `promptModuleInfoSelection`

Helpers in `internal/cli/config_info.go` — unchanged in this project.

Shared helpers used by other commands — retain:

- `internal/cli/config_helpers.go`: `searchAllConfigCategories`, `configMatch`, `promptSelectConfigMatch`, `promptSelectCategory`, `promptSelectOneFromList`, `loadNamesForCategory`, `resolveInstalledName`, `loadAgentsForScope`, `loadRolesForScope`, `loadContextsForScope`, `loadTasksForScope`, `truncatePrompt`, `allConfigCategories`
- `internal/cli/config_list.go`: `buildConfigListItem`, `ConfigListItem`
- `internal/cli/modules_list.go`: `collectInstalledScopes`, `collectInstalledModules`
- `internal/cli/resolve.go`: `resolver` struct, `newResolver`, `autoInstall`, `selectSingleMatch`, `promptModuleSelection`, `ensureIndex`, `reloadConfig`
- `internal/cli/cross_resolve.go`: `resolveCrossCategory`, `ModuleMatch`, `installIfRegistry`, `promptCrossCategorySelection`
- `internal/cli/describe.go`: `notifyScopeWidenedIfLocal` (still called from `runGet`), `printVerboseDump`, `prepareDescribe`, `findConfigSource`, `formatCUEDefinition`, `deriveCacheDir`, `partialFillAgentCommand`, `describeScopeFromCmd`, `loadConfig`
- `internal/modules/`: `SearchIndex`, `SearchCategoryEntries`, `ValidateSearchQuery`, `ParseSearchPatterns`, `CompileSearchTerms` (existing registry-fuzzy engine; used by `modules search`, `config search`, `search`, and `cross_resolve.go`).

### Cross-file registration calls to remove

- `internal/cli/modules.go:71` — `addModulesInfoCommand(modulesCmd)`

### Test files affected

- `internal/cli/modules_test.go`: `TestModuleInfoResultJSON` (line 925); subcommand-set assertion at line 215 — current literal is `{"browse", "index", "search", "install", "list", "info", "update"}`; drop `"info"`.
- `test/integration/modules_test.go`: subcommand-set assertion at line 460 — current literal is `{"Manage modules", "browse", "search", "install", "list", "info", "update"}`; drop `"info"`. Any test cases that invoke `modules info`.
- `internal/cli/root_test.go`: help-table entry at lines 191-194 (`modules info help`); delete.

`config_test.go`, `config_integration_test.go`, and the `config_info` help-table entries in `root_test.go` (lines 133-136) are not touched in this project.

### Documentation references

- `README.md`: line 276 (`start modules info golang/assistant`). The `config info` references at line 193 and lines 317-318 are not touched here.
- `AGENTS.md` lines 40-43: the post-project-02 `start describe agents` / `roles` / `tasks` / `contexts` lines are misleading — `start describe <category>` searches for a name matching `<category>`, it does not list. Rewrite to describe `start describe` correctly (no-arg lists; with-arg searches and may auto-install).
- `internal/cli/help/agents.md`: no `modules info` references at present (verified). Vacuously clean for this project.
- `scripts/test-supporting-commands.sh` lines 187-191 (`modules info` test cases).
- `scripts/manual-test` lines 163-164 (`modules info --help`), 622-629 (`modules info` cases).
- `scripts/show-help` line 56 (`${BIN} modules info`).

### Codebase conventions confirmed

- Exit codes are binary. `cmd/start/main.go:18` is the only `os.Exit` call in the repository; it hardcodes exit 1 on any non-nil error from `Execute()`. Describe preserves this.
- `start describe` and `start get` already share the resolver, including the registry-fuzzy match path (`searchRegistryCategory` in `resolve.go:481`, same `searchCategory`/`matchScorePatterns` engine as `modules.SearchIndex`) and the 3-char query guard (`get.go:156`, `describe.go:154`). Verified runtime: `start get "gitlab pipeline teacher"` against an uninstalled registry module auto-installs `gitlab/pipeline/teacher@v1.0.0` and emits content; `start get "go"` errors `query must be at least 3 characters`.

## References

- CLI Design for Agents spec at https://github.com/start-cli/library/issues/2 — Rule 9 puts `info` and `show` in the never-canonical column for `get`. This project serves Rule 9 by folding `modules info` into `start describe`.

## Requirements

### 1. Shared resolution with `start get`

`start describe <name>` resolves through `resolveCrossCategory(name, r)` exactly as `start get` does. The resolver's existing search path (in `cross_resolve.go:34-195`):

1. Exact match against installed config across in-scope categories.
2. Substring match across installed config (combined; gates the single-exact-match disambiguation).
3. Exact registry match (only when no installed substring matches). Triggers `installIfRegistry` → auto-install.
4. Combined search: installed-substring + registry-fuzzy via `searchRegistryCategory` (same AND-word/regex engine as `modules.SearchIndex`). Single match auto-installs; multi-match enters selection.

The 3-char minimum query length is already enforced at the describe and get call sites (`describe.go:154`, `get.go:156`). No resolver search-behaviour changes are required by this project.

Multi-match handling, auto-install behaviour, `r.didInstall` reload, and `notifyScopeWidenedIfLocal` are unchanged.

Installs land in global config. After `r.didInstall` fires, lookup widens to merged scope and `notifyScopeWidenedIfLocal` emits a stderr warning under `--local`.

`--global` / `--local` mutually-exclusive flags scope the installed lookup.

### 2. Text output: formatted metadata block

`start describe <name>` extends `printVerboseDump` with a formatted metadata block placed between the Cache line and the CUE Definition section. The block emits the human-readable fields from the legacy `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` formatters, scoped to the item's category.

Agents:

- `Description: <text>` (when set)
- `Bin: <bin>` (when set)
- `Default Model: <model>` (when set)
- `Tags: <comma-joined>` (when set)
- `Models:` followed by indented `<alias> -> <id>` lines (when set, sorted by alias)

Roles:

- `Description: <text>` (when set)
- `Prompt: <truncated to 100 chars>` (when set)
- `Optional: true` (when true)
- `Tags: <comma-joined>` (when set)

Contexts:

- `Description: <text>` (when set)
- `Prompt: <truncated to 100 chars>` (when set)
- `Required: <bool>`
- `Default: <bool>`
- `Tags: <comma-joined>` (when set)

Tasks:

- `Description: <text>` (when set)
- `Prompt: <truncated to 100 chars>` (when set)
- `Role: <role>` (when set)
- `Tags: <comma-joined>` (when set)

The block is additive. The CUE Definition section, File path/contents, and Command sections remain. The CUE definition is still the source of truth; the formatted block makes common fields scannable.

`printMetadataBlock` writes its own leading blank line and emits nothing (no blank line either) when all category-specific fields are absent. The CUE Definition section continues to manage its own leading blank line.

Updated dump structure:

```
<ItemType>: <Name>
─────────────────────────────────────
Config: <config file path>
Origin: <registry origin coords>            (when set)
Cache: <cache directory>                    (when origin set)

<formatted metadata block — category-specific fields>

<CUE Definition>                            (pretty-printed CUE source)

File: <file path>                           (when File field set)
Path: <resolved file path>                  (when File resolves)
<rendered file content>                     (when File resolves)

Command: <command template>                 (when Command field set)
─────────────────────────────────────
```

The `Config:` label stays as-is.

### 3. Removal of modules info

The cobra subcommand `start modules info` is removed:

- File `internal/cli/modules_info.go` is deleted in full.
- Registration call at `internal/cli/modules.go:71` is removed.
- Exported type `ModuleInfoResult` is removed (no replacement).
- `TestModuleInfoResultJSON` is deleted.
- Subcommand-set assertions in `internal/cli/modules_test.go:215` and `test/integration/modules_test.go:460` drop the `"info"` entry.
- Help-table entry in `internal/cli/root_test.go` at lines 191-194 is deleted.

`start config info` is left as-is. The forthcoming rename project renames it to `start config get`.

No backward-compatibility aliases. No deprecation shims.

### 4. Documentation and scripts

- `README.md`: rewrite line 276 to use `start describe`. The `config info` line at 193 and the `config info` block at 317-318 are not touched in this project.
- `AGENTS.md` lines 40-43: replace the misleading per-category form with one or two lines describing `start describe` (no-arg lists, with-arg searches and may auto-install).
- `internal/cli/help/agents.md`: confirm with `rg -n 'modules info' internal/cli/help/` returns nothing before declaring done.
- `scripts/test-supporting-commands.sh`: rewrite the `modules info` cases at lines 187-191 as `start describe` cases or delete if equivalent describe cases already exist.
- `scripts/manual-test`: delete the `modules info` cases at lines 163-164 and 622-629. Extend the existing `describe` cases to cover the tier-3 (exact registry) auto-install path and at least one tier-4 (registry-fuzzy) path.
- `scripts/show-help`: delete line 56 (`${BIN} modules info`).

`config info` references in docs and scripts (`README.md` lines 193 and 317-318, `scripts/manual-test` lines 115-116 and 369-376) remain pending the follow-up rename project.

### 5. Verification

- `start --help`, `start modules --help` show no `info` subcommand. `start config --help` still shows `info` (handled by the follow-up project).
- `start describe <installed-name>` produces the verbose dump with the new formatted metadata block.
- `start describe <registry-only-name>` (tier 3 exact registry hit) auto-installs then renders the installed view, emitting the `--local`-widening warning on stderr when `--local` was set.
- `start describe <fuzzy-words>` (tier 4 registry-fuzzy hit) resolves and auto-installs the single match, or surfaces the standard multi-match menu.
- `start get <fuzzy-words>` resolves via the same tier 4 (shared resolver) — existing behaviour.
- `start describe <ambiguous>` shows the menu in a terminal and errors with the candidate list otherwise.
- `rg -n 'addModulesInfoCommand|runModulesInfo|ModuleInfoResult|checkIfInstalled|printModuleInfo|promptModuleInfoSelection' .` returns no matches outside this document.
- `rg -n 'modules info\b' .` returns no matches outside this document.
- `start config info <name>` continues to work unchanged.
- `scripts/invoke-tests` passes.

## Issues Discovered

1. Mechanism for suppressing the multi-match interactive menu under `--json` (design) — Resolved: dropped `--json` from describe.

   Original concern: `promptCrossCategorySelection` (`cross_resolve.go:223`) and `promptModuleSelection` (`resolve.go:542`) gate their interactive branch only on `isTerminal(r.stdin)`. With `--json` and a TTY stdin, the menu would have written to `r.stdout` and corrupted JSON output.
   Resolution: `--json` removed from describe scope entirely. The resolver is reused as-is — no `disableAutoInstall` field, no `nonInteractive` field, no menu-suppression mechanism needed. If a future project reintroduces JSON for describe, both flags are added together at the resolver call site (`r.disableAutoInstall = jsonFlag; r.nonInteractive = jsonFlag`) and the prompt sites gate on `isTerminal(r.stdin) && !r.nonInteractive`.

2. JSON `category` field — singular or plural (decision) — Resolved: dropped `--json` from describe.

   Original concern: codebase has both forms (`ConfigListItem.Category` singular; `ModuleMatch.Category` plural). The describe JSON payload would have had to pick one.
   Resolution: moot. No JSON output from describe in this project. When/if a future project reintroduces JSON, the decision returns at that point.

## Implementation Plan

1. Verify existing `runDescribeSearch` (post-02) routes through `resolveCrossCategory` and emits `notifyScopeWidenedIfLocal` on `r.didInstall`. No change needed unless gaps surface.
2. Add the formatted metadata block to `printVerboseDump` in `internal/cli/describe.go`. New helper `printMetadataBlock(w io.Writer, r DescribeResult)` switches on `r.ItemType` and emits the category-specific fields per requirement 2. Reuse `truncatePrompt` from `config_helpers.go`. Field-selection logic mirrors the `print*Info` functions in `config_info.go` but does not call them (they emit headers/separators that don't fit the new placement). Place the block between the Cache line and the CUE Definition section.
3. Delete `internal/cli/modules_info.go` in full. Remove the registration call at `internal/cli/modules.go:71`. Remove the `ModuleInfoResult` type. Delete `TestModuleInfoResultJSON`. Delete the `modules info help` table entry at `internal/cli/root_test.go:191-194`. Update the subcommand-set assertions in `internal/cli/modules_test.go:215` and `test/integration/modules_test.go:460` to drop `"info"`.
4. Add tests: per category (agent, role, context, task), installed single match with all metadata fields set, and installed single match with no metadata fields set; tier-3 exact registry auto-install; tier-4 registry-fuzzy single match auto-install; tier-4 multi-match non-TTY candidate-list error. TTY multi-match menu is covered by `scripts/manual-test`, not Go tests.
5. Update `README.md` (line 276 only), `AGENTS.md` (lines 40-43), and confirm `internal/cli/help/agents.md` has no `modules info` references.
6. Update `scripts/test-supporting-commands.sh`, `scripts/manual-test`, `scripts/show-help` per requirement 4.
7. Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Hard cutover for `modules info`. No alias.
- `config info` is left untouched and is the subject of a follow-up rename project.
- Shared helpers in `config_helpers.go`, `config_list.go`, `modules_list.go`, and `resolve.go` are used by other commands and must not be deleted.
- The `print*Info` functions in `config_info.go` remain — they continue to back `start config info`. The new `printMetadataBlock` is a parallel implementation, not a refactor of those.
- Project `02-show-to-describe.md` MUST be complete. Verified: `internal/cli/describe.go` exists and `internal/cli/show.go` does not.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- Resolution is fully shared with `start get`. The describe handler should look almost identical to `runGet` up to and including the `r.didInstall` reload block — only the post-resolution rendering differs (verbose dump for describe; content render for get).
- Tier 4's network access reuses the resolver's existing registry index fetch and `tui.NewProgress` wiring. Do not introduce a second progress instance.
- `notifyScopeWidenedIfLocal` remains a shared helper called by both `runGet` and the describe handler. Do not delete it.
- Current `start describe` is verbose by default with no `--verbose` flag. Preserve that — the unified output is "the dump."
- Auto-install on inspect: existing behaviour for `start describe`. The fold does not introduce or remove the side effect.

## Acceptance Criteria

- `start describe <installed-name>` produces the verbose dump including the new formatted metadata block.
- `start describe <registry-only-name>` (tier 3) auto-installs into global config and renders the installed view; `--local` invocations emit the `notifyScopeWidenedIfLocal` warning on stderr.
- `start describe <fuzzy-words>` (tier 4) registry-fuzzy-matches the index; single match auto-installs and renders; multi-match enters the standard menu (TTY) or candidate-list error (non-TTY).
- `start get <fuzzy-words>` resolves via the same tier 4 (shared resolver) — existing behaviour, listed here for completeness.
- `start describe <ambiguous>` opens the interactive menu in a TTY and errors with the candidate list otherwise.
- `start --help`, `start modules --help` show no `info` subcommand.
- `start config info` continues to work unchanged.
- `internal/cli/modules_info.go` does not exist.
- `rg -n 'addModulesInfoCommand|runModulesInfo|ModuleInfoResult|checkIfInstalled|printModuleInfo|promptModuleInfoSelection' .` returns no matches outside this project document.
- `rg -n 'modules info\b' .` returns no matches outside this project document.
- `README.md` line 276 and `AGENTS.md` lines 40-43 reference `start describe` for the inspection use case. `internal/cli/help/agents.md` contains no `modules info` references.
- `scripts/invoke-tests` passes.
