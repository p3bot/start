# Fold modules info into start describe; enrich describe metadata

## Goal

Make `start describe <name>` the single command for inspecting modules — installed or registry-only — by reusing the same `resolveCrossCategory` resolution that `start get` uses, with AND-word fuzzy added as a fourth resolver tier, formatted metadata added to the verbose dump, and a `--json` mode. Delete `start modules info`. `start config info` is unchanged; a follow-up project will rename it to `start config get`.

## Scope

In scope:

- Add a fourth tier to `resolveCrossCategory`: AND-word fuzzy match across the registry index using `modules.SearchIndex`. Benefits `start get` as well.
- Augment `printVerboseDump` with a formatted metadata block between Cache and the CUE Definition section. Surfaces the structured fields that `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` emit today.
- Wire `start describe <name>` through `resolveCrossCategory` (already done post-02; confirm).
- Add `--json` to `start describe`, emitting a raw unwrapped JSON value via `writeJSON`. No envelope.
- Specify the `--json` interaction with auto-install: JSON mode does not auto-install. A registry-only match (tier 2 or tier 4) under `--json` errors with an explicit install hint and exits 1.
- Delete `start modules info` (entire subcommand at `internal/cli/modules_info.go`).
- Remove registration calls, tests, help-table entries, and stale comments referencing `modules info`.
- Update documentation (`README.md`, `AGENTS.md`, `internal/cli/help/agents.md`) and scripts to point at `start describe` where they reference `modules info`.

Out of scope:

- `start config info`. Left in place for a follow-up project that renames it to `start config get`. No changes to `internal/cli/config_info.go`, `runConfigInfo`, the `print*Info` formatters, or related tests in this project.
- The no-argument listing form of `start describe`. Behaviour is unchanged; the fold applies only to the named-argument form.
- Renaming or relocating `start describe`.
- Cross-cutting JSON envelope adoption. start's current `--json` convention is raw unwrapped values across all twelve existing data-returning commands; describe matches that precedent rather than the spec's wrapped envelope. Envelope adoption, if ever, is a separate cross-cutting project.
- The `start search` / `start config search` overlap.
- Any change under `library/` or `homebrew-tap/`.
- Releases or tags.

## Current State

This project assumes project `02-show-to-describe.md` has completed. `internal/cli/show.go` does not exist; `internal/cli/describe.go` is in place. `runDescribe` and `runDescribeSearch` are the post-02 handlers.

Two commands currently overlap with describe for inspection:

`start describe <name>` (post-project-02):

- Resolution: `resolveCrossCategory` over installed config (exact, then substring), with exact registry match auto-installing on miss via the shared resolver (writes to global config; `notifyScopeWidenedIfLocal` warns under `--local`).
- Multi-match: interactive menu via `promptModuleSelection`, error in non-interactive.
- Output: verbose dump (`printVerboseDump`) — header, Config, Origin, Cache, CUE Definition, File path, Path, file contents, Command.
- JSON: no support.
- Scope: `--global` / `--local` (mutually exclusive).

`start modules info <name>` (file: `internal/cli/modules_info.go`):

- Resolution: AND-word fuzzy match across the registry index; no auto-install; works on uninstalled modules.
- Multi-match: numbered menu via `promptModuleInfoSelection` in interactive mode; first of N in non-interactive.
- Output: `printModuleInfo` — Type, Module, Description, Tags, install status, Version, install hint when not installed.
- JSON: `--json` returns `[]ModuleInfoResult` (exported struct in `modules_info.go`).
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
- `internal/cli/resolve.go`: `resolveCrossCategory`, `newResolver`, `autoInstall`, `selectSingleMatch`, `promptModuleSelection`, `reloadConfig`
- `internal/cli/describe.go`: `notifyScopeWidenedIfLocal` (still called from `runGet`), `printVerboseDump`, `prepareDescribe`, `findConfigSource`, `formatCUEDefinition`, `deriveCacheDir`, `partialFillAgentCommand`, `describeScopeFromCmd`, `loadConfig`
- `internal/modules/`: `SearchIndex` (reused by the new resolver tier).

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

- JSON output convention is raw values. `internal/cli/output.go:17` defines `writeJSON` as a thin wrapper around `json.NewEncoder`. All twelve current `--json` commands emit unwrapped values. No `{"status":"ok","data":…}` envelope exists anywhere. Describe `--json` follows this convention.
- Exit codes are binary. `cmd/start/main.go:18` is the only `os.Exit` call in the repository; it hardcodes exit 1 on any non-nil error from `Execute()`. Describe preserves this.
- `start describe` and `start get` already share the resolver. The new tier 4 benefits both.

## References

- CLI Design for Agents spec at https://github.com/start-cli/library/issues/2 — Rule 9 puts `info` and `show` in the never-canonical column for `get`. This project serves Rule 9 by folding `modules info` into `start describe`. Rule 3's wrapped JSON envelope is rejected in favour of the codebase's established raw-value `--json` convention.

## Requirements

### 1. Shared resolution with `start get`, with new AND-word fuzzy tier

`start describe <name>` resolves through `resolveCrossCategory(name, r)` exactly as `start get` does. The resolver's tiers, in order:

1. Exact match against installed config (agents, roles, contexts, tasks).
2. Exact match against the registry index. Match triggers auto-install (text mode) or registry-only error (JSON mode).
3. Substring match across installed config.
4. AND-word fuzzy match across the registry index using `modules.SearchIndex` (new). Match in text mode triggers auto-install; in JSON mode errors with the install hint. Multi-match enters the standard menu/non-interactive-error path. Minimum query length of 3 characters applies (same guard as `modules info` today).

Multi-match handling, auto-install behaviour, `r.didInstall` reload, and `notifyScopeWidenedIfLocal` are unchanged across all tiers.

Installs land in global config. After `r.didInstall` fires, lookup widens to merged scope and `notifyScopeWidenedIfLocal` emits a stderr warning under `--local`.

`--global` / `--local` mutually-exclusive flags scope the installed lookup.

The implementer adds a `disableAutoInstall bool` to `resolver` so JSON mode can suppress auto-install for both tier 2 and tier 4 hits; the resolver returns a "registry-only match found, not installed" signal that the JSON renderer converts to the install-hint error.

### 2. Text output: formatted metadata block

`start describe <name>` extends `printVerboseDump` with a formatted metadata block placed between the Cache line and the CUE Definition section. The block emits the human-readable fields from the legacy `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` formatters, scoped to the item's category.

Agents:

- `Bin: <bin>` (when set)
- `Default Model: <model>` (when set)
- `Tags: <comma-joined>` (when set)
- `Models:` followed by indented `<alias> -> <id>` lines (when set, sorted by alias)
- `Description: <text>` (when set)

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

Updated dump structure:

```
<ItemType>: <Name>
─────────────────────────────────────
Config: <config file path>
Origin: <registry origin coords>            (when set)
Cache:  <cache directory>                   (when origin set)

<formatted metadata block — category-specific fields>

<CUE Definition>                            (pretty-printed CUE source)

File: <file path>                           (when File field set)
Path: <resolved file path>                  (when File resolves)
<rendered file content>                     (when File resolves)

Command: <command template>                 (when Command field set)
─────────────────────────────────────
```

The `Config:` label stays as-is.

### 3. JSON output

`start describe --json <name>` emits the describe payload as a raw, unwrapped JSON value via `writeJSON`. The shape matches the post-resolution installed state.

Payload (object):

- Identity: `category`, `name`, `source` (config file path), `scope` (`"global"` or `"local"` — which config file `source` belongs to)
- Registry coords (when set): `origin`, `cache`. The `origin` value encodes module path and version (e.g. `github.com/org/mod@v1.2.3`); no separate `module` / `version` fields.
- Metadata: `description`, `tags`
- Category-specific fields per the existing `ConfigListItem` shape in `config_list.go`:
  - agents: `bin`, `command`, `defaultModel`, `models`
  - roles: `file`, `prompt`, `optional`
  - contexts: `file`, `command`, `prompt`, `required`, `default`
  - tasks: `file`, `command`, `prompt`, `role`
- `file`: `{path, resolvedPath}` object when the item has a file field. No embedded file contents — callers wanting content use `start get`.
- `command`: string when set at the top level (agents).

Sub-behaviours:

- Name required with `--json`. `start describe --json` (no argument) errors with `name required with --json` and exits 1. Matches the `modules info --json` precedent.
- Auto-install disabled in JSON mode. If the resolver finds a registry-only match (tier 2 or tier 4), describe errors with `module <category:name> not installed; run 'start modules install <category:name>' first` and exits 1. Implementation: set `r.disableAutoInstall = true` on the resolver under `--json`.
- Multi-match in JSON mode is non-interactive. The resolver's interactive menu does not run; the candidate list is written to stderr (same format as the non-terminal multi-match path), nothing is written to stdout, exit is 1.

Legacy JSON shapes (`[]ConfigListItem` from `config info`, `[]ModuleInfoResult` from `modules info`) are not reproduced. The describe payload's category-specific fields mirror `ConfigListItem` so callers can migrate fields shallowly.

### 4. Removal of modules info

The cobra subcommand `start modules info` is removed:

- File `internal/cli/modules_info.go` is deleted in full.
- Registration call at `internal/cli/modules.go:71` is removed.
- Exported type `ModuleInfoResult` is removed (no replacement).
- `TestModuleInfoResultJSON` is deleted.
- Subcommand-set assertions in `internal/cli/modules_test.go:215` and `test/integration/modules_test.go:460` drop the `"info"` entry.
- Help-table entry in `internal/cli/root_test.go` at lines 191-194 is deleted.

`start config info` is left as-is. The forthcoming rename project renames it to `start config get`.

No backward-compatibility aliases. No deprecation shims.

### 5. Documentation and scripts

- `README.md`: rewrite line 276 to use `start describe`. The `config info` line at 193 and the `config info` block at 317-318 are not touched in this project.
- `AGENTS.md` lines 40-43: replace the misleading per-category form with one or two lines describing `start describe` (no-arg lists, with-arg searches and may auto-install).
- `internal/cli/help/agents.md`: confirm with `rg -n 'modules info' internal/cli/help/` returns nothing before declaring done.
- `scripts/test-supporting-commands.sh`: rewrite the `modules info` cases at lines 187-191 as `start describe` cases or delete if equivalent describe cases already exist.
- `scripts/manual-test`: delete the `modules info` cases at lines 163-164 and 622-629. Extend the existing `describe` cases to cover the tier-2 registry-only auto-install path and at least one tier-4 (AND-word fuzzy) path.
- `scripts/show-help`: delete line 56 (`${BIN} modules info`).

`config info` references in docs and scripts (`README.md` lines 193 and 317-318, `scripts/manual-test` lines 115-116 and 369-376) remain pending the follow-up rename project.

### 6. Verification

- `start --help`, `start modules --help` show no `info` subcommand. `start config --help` still shows `info` (handled by the follow-up project).
- `start describe <installed-name>` produces the verbose dump with the new formatted metadata block.
- `start describe <registry-only-name>` (tier 2 exact registry hit) auto-installs then renders the installed view, emitting the `--local`-widening warning on stderr when `--local` was set.
- `start describe <fuzzy-words>` (tier 4 AND-word fuzzy registry hit) resolves and auto-installs the single match, or surfaces the standard multi-match menu.
- `start get <fuzzy-words>` also resolves via tier 4 (shared resolver).
- `start describe --json <installed-name>` emits the raw JSON payload defined in requirement 3, including `scope`.
- `start describe --json <registry-only-name>` writes nothing to stdout, errors with the install hint on stderr, exits 1, and does not write to config.
- `start describe --json` (no argument) errors with `name required with --json` and exits 1.
- `start describe <ambiguous>` shows the menu in a terminal and errors with the candidate list otherwise.
- `start describe --json <ambiguous>` errors with the candidate list on stderr, writes nothing to stdout, exits 1.
- `rg -n 'addModulesInfoCommand|runModulesInfo|ModuleInfoResult|checkIfInstalled|printModuleInfo|promptModuleInfoSelection' .` returns no matches outside this document.
- `rg -n 'modules info\b' .` returns no matches outside this document.
- `start config info <name>` continues to work unchanged.
- `scripts/invoke-tests` passes.

## Implementation Plan

1. Add tier 4 (AND-word fuzzy registry match) to `resolveCrossCategory` in `internal/cli/resolve.go`. Reuse `modules.SearchIndex`. Place after the existing substring-installed tier. Enforce minimum 3-character query (consistent with current `modules info`). Multi-match passes through the standard selection/error path; single match auto-installs unless `r.disableAutoInstall` is set. If the registry index has already been fetched earlier in the resolver call, do not re-fetch.
2. Add `disableAutoInstall bool` to the `resolver` struct in `internal/cli/resolve.go`. When set, `autoInstall` is skipped and `resolveCrossCategory` returns a structured signal (existing match data plus a flag) that callers convert to the install-hint error.
3. Verify existing `runDescribeSearch` (post-02) routes through `resolveCrossCategory` and emits `notifyScopeWidenedIfLocal` on `r.didInstall`. No change needed unless gaps surface.
4. Add the formatted metadata block to `printVerboseDump` in `internal/cli/describe.go`. New helper `printMetadataBlock(w io.Writer, r DescribeResult)` switches on `r.ItemType` and emits the category-specific fields per requirement 2. Reuse `truncatePrompt` from `config_helpers.go`. Field-selection logic mirrors the `print*Info` functions in `config_info.go` but does not call them (they emit headers/separators that don't fit the new placement). Place the block between the Cache line and the CUE Definition section.
5. Add `--json` flag to the describe command. Implement a renderer that constructs the JSON payload from the resolved item and post-resolution config, then emits via `writeJSON`. Reuse `buildConfigListItem` (already exported in `config_list.go`) for category-specific fields. Augment with `origin`, `cache`, `scope`, `file: {path, resolvedPath}`.
6. Wire `--json` into the resolver path: set `r.disableAutoInstall = true` and translate the resolver's registry-only signal into the install-hint error. Suppress the multi-match interactive menu when `--json` is set; emit the candidate list to stderr and exit 1 instead.
7. Implement the `--json` no-listing-form behaviour. `runDescribe` checks the flag before the listing branch and errors `name required with --json` if no argument was given.
8. Delete `internal/cli/modules_info.go` in full. Remove the registration call at `internal/cli/modules.go:71`. Remove the `ModuleInfoResult` type. Delete `TestModuleInfoResultJSON`. Delete the `modules info help` table entry at `internal/cli/root_test.go:191-194`. Update the subcommand-set assertions in `internal/cli/modules_test.go:215` and `test/integration/modules_test.go:460` to drop `"info"`.
9. Add tests: installed single match including the new metadata block, registry-only auto-install (tier 2, text mode), registry-only no-install (JSON mode), tier-4 single match auto-install (text mode), tier-4 multi-match interactive menu, tier-4 JSON error, JSON payload shape including `scope`, ambiguous-match non-interactive error, ambiguous-match JSON error, `--json` no-argument error.
10. Update `README.md` (line 276 only), `AGENTS.md` (lines 40-43), and confirm `internal/cli/help/agents.md` has no `modules info` references.
11. Update `scripts/test-supporting-commands.sh`, `scripts/manual-test`, `scripts/show-help` per requirement 5.
12. Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

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
- For the JSON payload, reuse `buildConfigListItem` rather than re-implementing per-category extraction. The augmentation describe adds on top is `origin`, `cache`, `scope`, and `file: {path, resolvedPath}`.
- `notifyScopeWidenedIfLocal` remains a shared helper called by both `runGet` and the describe handler. Do not delete it.
- Current `start describe` is verbose by default with no `--verbose` flag. Preserve that — the unified output is "the dump."
- Auto-install on inspect: existing behaviour for `start describe`. The fold does not introduce or remove the side effect; it just makes describe's resolution explicitly shared with get and adds tier 4. The only place auto-install is deliberately suppressed is `--json` mode (per requirement 3 sub-behaviours), to keep the machine-consumer surface read-only.

## Acceptance Criteria

- `start describe <installed-name>` produces the verbose dump including the new formatted metadata block.
- `start describe <registry-only-name>` (tier 2) auto-installs into global config and renders the installed view; `--local` invocations emit the `notifyScopeWidenedIfLocal` warning on stderr.
- `start describe <fuzzy-words>` (tier 4) AND-word fuzzy-matches the registry index; single match auto-installs and renders; multi-match enters the standard menu (TTY) or candidate-list error (non-TTY).
- `start get <fuzzy-words>` resolves via tier 4 identically (shared resolver).
- `start describe --json <installed-name>` emits the raw JSON payload defined in requirement 3 (no envelope) on stdout, including the `scope` field.
- `start describe --json <registry-only-name>` writes nothing to stdout, writes `module <addr> not installed; run 'start modules install <addr>' first` to stderr, exits 1, and does not modify config.
- `start describe --json` with no argument writes `name required with --json` to stderr and exits 1.
- `start describe <ambiguous>` opens the interactive menu in a TTY and errors with the candidate list otherwise.
- `start describe --json <ambiguous>` writes nothing to stdout, writes the candidate list to stderr, and exits 1.
- `start --help`, `start modules --help` show no `info` subcommand.
- `start config info` continues to work unchanged.
- `internal/cli/modules_info.go` does not exist.
- `rg -n 'addModulesInfoCommand|runModulesInfo|ModuleInfoResult|checkIfInstalled|printModuleInfo|promptModuleInfoSelection' .` returns no matches outside this project document.
- `rg -n 'modules info\b' .` returns no matches outside this project document.
- `README.md` line 276 and `AGENTS.md` lines 40-43 reference `start describe` for the inspection use case. `internal/cli/help/agents.md` contains no `modules info` references.
- `scripts/invoke-tests` passes.
