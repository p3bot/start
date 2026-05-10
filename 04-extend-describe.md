# Extend describe to cover registry metadata; drop config info and modules info

## Goal

Make `start describe <name>` the single command for "tell me about this module," covering both installed modules and registry-only modules in one consistent output. Delete `start config info` and `start modules info`. The CLI's three overlapping inspection commands collapse into one verb that adapts its output to what is known about the named module.

## Scope

In scope:

- Extend `start describe <name>` to look up the name across installed config AND the registry index, with one adaptive output shape that populates fields based on what is known
- Add `--json` support to `start describe` per the project's CLI Design for Agents spec
- Remove the auto-install-on-miss behaviour that `start describe` inherited from `start show`. Inspection becomes read-only; a not-installed match emits a `Use 'start modules install …' to install` hint instead of fetching
- Delete `start config info` (entire subcommand at `internal/cli/config_info.go`)
- Delete `start modules info` (entire subcommand at `internal/cli/modules_info.go`)
- Remove the corresponding registration calls and tests
- Update documentation (`README.md`, `AGENTS.md`, `internal/cli/help/agents.md`) and shell scripts to reference the unified `describe` and remove references to the deleted commands

Out of scope:

- The no-argument listing form of `start describe` (which lists all installed items by category). Listing behaviour is unchanged. The fold applies only to the named-argument form.
- Renaming `start describe` further or relocating it
- Splitting the overloaded listing-vs-describe behaviour. That is a separate flattening project.
- The `start search` and `start config search` overlap (project 05)
- Any change to the registry index format or the CUE schemas under `library/`
- Releases or tags

## Current State

This project assumes project `02-show-to-describe.md` has completed. Throughout this document, `describe.go` and `runDescribe` refer to the post-02 file and handler; if `02` has not yet run, the implementer must run it first.

Three commands today inspect modules from different angles:

`start describe <name>` (renamed from `show` in project 02):

- Resolution: cross-category search across INSTALLED config; auto-installs from the registry on miss (writes to global config permanently)
- Multi-match: errors with "ambiguous name" + match list
- Output: multi-section verbose dump — header (category:name with reason), Config (filesystem path), Origin (registry coords), Cache (cache directory), CUE Definition (pretty-printed structured CUE), File path + Path + rendered file content, Command (agent template)
- Source data: installed config + filesystem cache + rendered file content
- JSON: NO (text mode only)
- Scope: `--global` / `--local` (mutually exclusive) on the installed lookup
- Auto-install side effect: yes

`start config info <name>` (file: `internal/cli/config_info.go`):

- Resolution: substring match across INSTALLED config categories; no auto-install
- Multi-match: numbered menu in interactive mode; error in non-interactive
- Output: single-section dump per category — Source (config file), Origin (registry coords if any), category-specific fields (Bin, Command, Default Model for agents; File, Command, Prompt, Optional, Tags for roles; etc.), Description, Tags
- Source data: installed config only
- JSON: YES (`--json`), shape is `[]ConfigListItem` (defined in `config_list.go`)
- Scope: `--local` only

`start modules info <name>` (file: `internal/cli/modules_info.go`):

- Resolution: AND-word fuzzy search across the REGISTRY INDEX (multi-word queries supported); no auto-install; works on uninstalled modules
- Multi-match: numbered menu in interactive mode; shows first of N (with hint) in non-interactive
- Output: single-section dump — Type, Module (registry path), Description, Tags, install status (`✓ Installed [scope]` or `Not installed`), Version, then a `Use 'start modules add …' to install` footer when not installed
- Source data: registry index + installation cross-check via `checkIfInstalled`
- JSON: YES (`--json`), shape is `[]ModuleInfoResult` (defined in `modules_info.go`)
- Scope: not applicable (registry is unscoped)

Helper inventory:

Helpers in `internal/cli/config_info.go` (all to be removed unless reused):

- `addConfigInfoCommand`, `runConfigInfo`, `runConfigInfoInteractive`
- `printConfigInfo`, `printAgentInfo`, `printRoleInfo`, `printContextInfo`, `printTaskInfo`

Helpers in `internal/cli/modules_info.go` (all to be removed unless reused):

- `addModulesInfoCommand`, `runModulesInfo`
- `ModuleInfoResult` (exported struct)
- `checkIfInstalled`, `printModuleInfo`, `promptModuleInfoSelection`

Shared helpers in `internal/cli/config_helpers.go` and `internal/cli/modules_list.go` that are used by other commands and MUST be retained:

- `searchAllConfigCategories`, `configMatch`, `promptSelectConfigMatch`, `promptSelectCategory`, `promptSelectOneFromList`, `loadNamesForCategory`, `resolveInstalledName`, `loadAgentsForScope` / `loadRolesForScope` / `loadContextsForScope` / `loadTasksForScope`, `truncatePrompt`, `buildConfigListItem`, `collectInstalledScopes`, `collectInstalledModules`, `allConfigCategories`

Cross-file registration calls to remove:

- `internal/cli/config.go` line 28 — `addConfigInfoCommand(configCmd)`
- `internal/cli/modules.go` line 71 — `addModulesInfoCommand(modulesCmd)`

Test files affected:

- `internal/cli/config_test.go` — `TestConfigInfo_*` functions (TestConfigInfo_Agent at line 85, TestConfigInfo_NotFound at line 139, TestConfigInfoJSON_MultipleMatches at line 2259, TestConfigInfoJSON_WithMatch at line 2320, TestConfigInfoJSON_NoArgs at line 2369, TestConfigInfoJSON_NotFound at line 2390)
- `internal/cli/config_integration_test.go` — `TestConfigInfo_ZeroMatch` at line 1222
- `internal/cli/modules_test.go` — `TestModuleInfoResultJSON` at line 925; the subcommand-set assertion at line 215 (`{"browse", "index", "search", "add", "list", "info", "update"}`) and the analogous assertion in `test/integration/modules_test.go` line 460 must drop `"info"`
- `test/integration/modules_test.go` — any test cases that invoke `modules info`

Documentation references to remove or rewrite:

- `README.md` lines 192-193 (`start config info` mention and example), 276 (`start modules info` example), 316-318 (`start config info` example block)
- `AGENTS.md` — the existing `start show <category>` lines (40-43) become irrelevant in the unified model; rewrite to reflect the new world
- `internal/cli/help/agents.md` — any `config info` or `modules info` references
- `scripts/test-supporting-commands.sh` lines 187-191 (`modules info` test cases)
- `scripts/manual-test` lines 115-116, 163-164, 369-376, 625-632 (`config info` and `modules info` test cases — must be removed; equivalent `describe` cases already exist or are added in scope)
- `scripts/show-help` line 56 (`${BIN} modules info`)

## References

- CLI Design for Agents spec at https://github.com/start-cli/library/issues/2 — Rule 9 puts both `info` and `show` in the never-canonical column for `get`. Rule 3 mandates a wrapped JSON envelope on every data-returning command. The fold serves both rules: one canonical inspection verb (`describe` is the documented domain exception per project 02), one consistent envelope.

## Requirements

### 1. Unified resolution

`start describe <name>` resolves the name by querying both installed config and the registry index in a single pass. The lookup proceeds in this priority:

1. Exact match in installed config (across all four categories)
2. Exact match in the registry index
3. Substring match in installed config; AND-word match in the registry index. Results from both sources are merged; matches that appear in both sources count as one match (installed-side data wins for shared fields)

Resolution outcomes:

- Single match: render the unified output
- Multiple matches in non-interactive mode: error with the candidate list (category:name lines) and exit. No "first of N" silent pick.
- Multiple matches in interactive mode: numbered menu (reuse `promptModuleInfoSelection`-style or `promptSelectConfigMatch`-style prompt; the implementer picks the appropriate helper)
- Zero matches: exit with `not found` error and a hint suggesting `start search <name>`

### 2. No auto-install

`start describe` MUST NOT fetch from the registry as a side effect of a name miss. The current `show` auto-install behaviour is removed. When the name resolves only against the registry index (not installed locally), the output includes a `Use 'start modules install <category:name>' to install` hint at the end. If the name is not in the registry either, the resolution falls into the zero-match path above.

The pre-existing `notifyScopeWidenedIfLocal` helper in `describe.go` becomes unused by `describe` once auto-install is removed; the helper is still called from the read/get command file (project 01) and remains in place for that caller.

### 3. Adaptive text output shape

One output shape, fields populate based on what is known. Mandatory ordering:

```
<category>:<name>
─────────────────────────────────────
Source: <config file path>                  (when installed)
Origin: <registry origin coords>            (when registry-known)
Cache:  <cache directory>                   (when fetched)
Module: <registry module path>              (when registry-known)
Version: <version>                          (when registry-known)

Description: <description>                  (when known from either source)
Tags: <comma-separated tags>                (when present)

<category-specific fields>                  (when installed: Bin, Command,
                                             Default Model, Models for agents;
                                             File, Prompt, Optional for roles;
                                             File, Command, Prompt, Required,
                                             Default for contexts;
                                             File, Command, Prompt, Role for tasks)

<CUE Definition>                            (when installed)

File: <file path>                           (when installed and File field set)
Path: <resolved file path>                  (when installed and File resolves)
<rendered file content>                     (when installed and File resolves)

Command: <command template>                 (when installed and Command field set)

✓ Installed in <scope>                      (when installed)
  Not installed                             (when registry-only)

Use 'start modules install <addr>' to install   (when registry-only and not installed)
─────────────────────────────────────
```

Field rules:

- Header always present
- "Source" replaces what current `show` calls "Config" (the change is wording only)
- Empty fields are omitted entirely (no `(none)`, no blank values)
- One blank line maximum between sections

Output for installed modules MUST be a strict superset of what current `start describe` (renamed `show`) produces today, with the `Source:` label substitution being the only structural change.

Output for registry-only modules MUST contain the same identifying and metadata fields that `start modules info` produces today (Type/Module/Description/Tags/Version/install status), rendered under the unified shape above.

### 4. JSON envelope

`start describe --json` emits a wrapped envelope per the project's CLI Design for Agents spec, Rule 3:

```json
{"status":"ok","data":{ /* describe payload */ }}
```

The `data` payload includes:

- `category`, `name` — identity
- `installed` (bool), `installedScope` (string, omitempty)
- `source` — config file path when installed; omitted otherwise
- `origin`, `module`, `version` — when registry-known
- `cache` — when fetched
- `description`, `tags` — when known
- Category-specific fields under their existing names (matching the current `ConfigListItem` shape for installed-only fields where reasonable)
- `cueDefinition` — pretty-printed CUE source, when installed
- `file` — `{path, resolvedPath, contents}` object, when installed and file is set
- `command` — string, when installed and command is set
- `nextAction` — `{message, command}` object when registry-only

Multi-match in JSON mode returns the candidate list under `data` as `{matches: [...]}` with exit code 5 (`CONFLICT`) and error code `AMBIGUOUS_NAME`. No silent first-of-N pick.

The legacy JSON shapes `[]ConfigListItem` (from `config info`) and `[]ModuleInfoResult` (from `modules info`) are NOT preserved. Callers depending on those shapes must migrate.

### 5. Removal of config info and modules info

The cobra subcommands `start config info` and `start modules info` are removed. Specifically:

- Files `internal/cli/config_info.go` and `internal/cli/modules_info.go` are deleted in full
- Registration calls in `internal/cli/config.go` and `internal/cli/modules.go` are removed
- The `ModuleInfoResult` exported type is removed (no replacement; its JSON role is taken over by the unified envelope)
- Test functions enumerated under Current State are deleted
- Subcommand-list assertions in `internal/cli/modules_test.go` line 215 and `test/integration/modules_test.go` line 460 drop the `"info"` entry
- Help text in `internal/cli/help/agents.md` and any prose in `README.md` describing the removed commands is rewritten to reference `start describe` instead

The `--json` flag previously on `config info` and `modules info` migrates to `start describe`. No standalone JSON contract survives.

### 6. Documentation and scripts

- `README.md` — every `start config info` and `start modules info` example is rewritten as `start describe`. The describe section reflects the new "covers installed AND registry" behaviour.
- `AGENTS.md` lines 40-43 — rewrite to reflect the unified verb. Substitution-only (project 02) preserved misleading text; this project corrects it. Suggested replacement is one line per common operation, not the misleading per-category form.
- `internal/cli/help/agents.md` — `config info` and `modules info` references rewritten as `start describe`.
- `scripts/test-supporting-commands.sh` — the `modules info` test cases at lines 187-191 are rewritten as `describe` cases (or deleted if `describe` cases already cover the same ground after project 02).
- `scripts/manual-test` — `config info` test cases (lines 115-116, 369-376) and `modules info` test cases (lines 163-164, 625-632) are deleted; the existing `describe` cases (added or already present after project 02) are extended to cover the new "registry-only" path with at least one positive case.
- `scripts/show-help` — line 56 (`${BIN} modules info`) is removed.

### 7. Verification

- `start --help` and `start config --help` and `start modules --help` show no `info` subcommand.
- `start describe <installed-name>` produces output matching the post-project-02 behaviour (Source: label substitution aside).
- `start describe <registry-only-name>` produces the registry-only output and the install hint without auto-installing.
- `start describe <ambiguous-name>` exits non-zero in non-interactive mode with a candidate list; prompts in interactive mode.
- `start describe --json <name>` emits the wrapped envelope.
- `rg -n 'addConfigInfoCommand|runConfigInfo|printConfigInfo|printAgentInfo|printRoleInfo|printContextInfo|printTaskInfo|addModulesInfoCommand|runModulesInfo|ModuleInfoResult|checkIfInstalled|printModuleInfo|promptModuleInfoSelection' .` returns no matches outside this project document.
- `rg -n 'config info\b|modules info\b' .` returns no matches outside this project document.

## Implementation Plan

1. Lift the registry-index lookup helpers
   The describe handler needs to query the registry index. The current `runModulesInfo` has the index-fetch and search logic; lift the relevant parts (registry client, `fetchIndex`, `modules.SearchIndex`, `checkIfInstalled`-style cross-check) into reusable form callable from `describe.go`. The implementer chooses whether to inline, extract to a helper file, or move into `describe.go` directly.

2. Rewrite the describe handler
   Replace `runDescribeSearch` with a new resolver that queries both installed config and the registry index in parallel (or sequentially, implementer's call), merges results per requirement 1, and dispatches to the unified renderer. Remove the auto-install branch and the `notifyScopeWidenedIfLocal` call inside `describe`. Wire `--json` through.

3. Implement the unified renderer
   One renderer that takes a populated payload struct and emits text or JSON per the flag. Field ordering and presence rules per requirement 3 (text) and requirement 4 (JSON). The payload struct combines installed-side fields (lifted from the four `print*Info` functions in `config_info.go`) and registry-side fields (lifted from `printModuleInfo` in `modules_info.go`). The implementer chooses whether to share code with the existing `printVerboseDump` in `describe.go` or replace it.

4. Delete `config info`
   Remove `internal/cli/config_info.go` in full. Remove the registration call in `internal/cli/config.go`. Delete the `TestConfigInfo_*` tests enumerated in Current State.

5. Delete `modules info`
   Remove `internal/cli/modules_info.go` in full. Remove the registration call in `internal/cli/modules.go`. Remove the `ModuleInfoResult` type. Delete `TestModuleInfoResultJSON`. Update the subcommand-set assertions in `internal/cli/modules_test.go` (line 215) and `test/integration/modules_test.go` (line 460) to drop the `"info"` entry.

6. Add tests for the unified describe
   Cover: installed-only resolution (regression test for current behaviour), registry-only resolution + install hint, unified output ordering, ambiguous name in non-interactive, ambiguous name in interactive, `--json` envelope shape (success and ambiguous), no auto-install side effect.

7. Documentation
   Edit `README.md`, `AGENTS.md`, and `internal/cli/help/agents.md` per requirement 6.

8. Scripts
   Edit `scripts/test-supporting-commands.sh`, `scripts/manual-test`, and `scripts/show-help` per requirement 6.

9. Verify
   Run the verification queries from requirement 7. Manually invoke `start describe <installed>`, `start describe <registry-only>`, `start describe <ambiguous>`, and `start describe --json <name>` and confirm output shape.

10. Build and test
    Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Hard cutover. No `config info` or `modules info` aliases. No backward-compatibility shims for the legacy JSON shapes.
- Removing auto-install IS a behaviour change for users who relied on `start show <not-installed-name>` silently fetching the module. The deliberate trade is recorded in Issues Discovered.
- The shared helpers in `internal/cli/config_helpers.go` and `internal/cli/modules_list.go` are used by other commands and must not be deleted.
- Project `02-show-to-describe.md` MUST complete before this project starts. The implementer of project 04 expects `describe.go` to exist.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- The four `print*Info` functions in `config_info.go` and the single `printModuleInfo` in `modules_info.go` are the canonical formatters for the data this project unifies. Reuse their formatting logic rather than rewriting from scratch — the unified renderer largely composes their output sections under one header. Whether to lift the functions verbatim, extract their bodies into helpers, or fold them into a new payload-driven renderer is the implementer's call; the user-facing output is what matters.
- The registry-index fetch is network-bound. The current `runModulesInfo` shows progress to stderr via `tui.NewProgress`; preserve this in the unified describe so users on slow links see the fetch happening. Stdout stays reserved for the response per spec Rule 3.
- Multi-match resolution merges installed and registry results. A name that exists in both sources counts as one match — the merge happens before counting. Test cases must cover: installed-only single match, registry-only single match, both-source single match (same name), installed-only ambiguous, registry-only ambiguous, mixed-source ambiguous.
- The `--global` and `--local` scope flags only affect the installed-config lookup. They do not gate or filter the registry search. A `--local` invocation that finds the name only in the registry returns the registry-only output unchanged — no `--local` widening happens because there is no install.
- The `notifyScopeWidenedIfLocal` helper in `describe.go` is unused by `describe` once auto-install is gone. Do not delete it — it is still called from the read/get command file (project 01). The unused-import / unused-call lint will be quiet because the helper is still referenced by the other caller.
- The current `start show` (project 02's `describe`) is verbose by default with no `--verbose` flag. Preserve that — the unified output is "the dump." If a future compact form is wanted, that is a separate project.

## Issues Discovered

1. Auto-install removal is a user-visible behaviour change (decision) — Resolved: remove auto-install; emit install hint instead.

   `start show` (and therefore the post-project-02 `start describe`) currently auto-installs a registry module when the user invokes it with a name that matches a registry module but is not yet installed. The install lands in global config and the verbose dump renders the freshly fetched module. Users have come to expect this convenience.

   Removing auto-install changes the contract: the same invocation now prints the registry metadata and a `Use 'start modules install …'` hint, with no side effect. The trade favours predictability (inspection commands should be read-only — a user asking "what is this?" should not get permanent state mutation as a side effect) over convenience (one fewer command for the install-and-inspect workflow).

   Resolution: remove auto-install. Surface the change clearly in `README.md` and `AGENTS.md` so existing users notice on first miss. The install workflow is now `start modules install <name>` then `start describe <name>`, two explicit steps.

2. Legacy JSON shapes (`ConfigListItem`, `ModuleInfoResult`) are not preserved (decision) — Resolved: replace with the unified envelope.

   `start config info --json` returns `[]ConfigListItem`. `start modules info --json` returns `[]ModuleInfoResult`. `start describe --json` (new) returns the wrapped envelope per spec Rule 3. Callers parsing the legacy shapes will break.

   The legacy shapes are internal types not part of any documented API contract. The CLI's pre-release status (no tagged release per `start/AGENTS.md`) means there is no stability obligation. Keeping the legacy shapes available under deprecated subcommands would defeat the fold.

   Resolution: drop the legacy shapes. Document the new envelope in the help text and in `internal/cli/help/agents.md` so callers can migrate.

3. AGENTS.md lines 40-43 misrepresent describe's category-listing behaviour (gap from project 02) — Resolved: rewrite in this project.

   Project 02 explicitly preserved the misleading lines (`start show agents` etc.) because a pure rename was its scope. Those lines now read `start describe agents` and remain misleading — `start describe agents` does a name search for "agents," it does not list installed agents.

   Resolution: rewrite the four lines in this project to reflect the actual unified verb. Suggested replacement is one or two lines describing `start describe` and `start describe <name>` (no per-category form), since the no-arg form already covers the listing use case.

## Acceptance Criteria

- `start describe <installed-name>` produces output matching the post-project-02 behaviour, with `Source:` replacing the `Config:` label and no other structural change.
- `start describe <registry-only-name>` produces the registry-only output (Type, Module, Description, Tags, Version, install status) under the unified shape, ending with the `Use 'start modules install …'` hint, and does not write to config.
- `start describe <ambiguous-name>` exits non-zero in non-interactive mode with the candidate list on stderr; prompts a numbered menu in interactive mode.
- `start describe --json <name>` emits the wrapped envelope per requirement 4. `start describe --json <ambiguous>` emits the ambiguous-name envelope with exit code 5.
- `start --help` and `start config --help` and `start modules --help` show no `info` subcommand.
- `internal/cli/config_info.go` and `internal/cli/modules_info.go` do not exist.
- `rg -n 'addConfigInfoCommand|runConfigInfo|printConfigInfo|printAgentInfo|printRoleInfo|printContextInfo|printTaskInfo|addModulesInfoCommand|runModulesInfo|ModuleInfoResult|checkIfInstalled|printModuleInfo|promptModuleInfoSelection' .` returns no matches outside this project document.
- `rg -n 'config info\b|modules info\b' .` returns no matches outside this project document.
- README.md, AGENTS.md, and `internal/cli/help/agents.md` reference `start describe` for the inspection use case and contain no mention of `config info` or `modules info`.
- `scripts/invoke-tests` passes.
