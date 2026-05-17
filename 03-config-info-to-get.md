# Rename start config info to start config get

## Goal

Rename `start config info` to `start config get` so the config subcommand surface uses the canonical `get` verb. This is a pure rename: the command's behaviour, flags, output, and JSON shape are preserved; only the command name, internal Go symbols, file name, test names, and documentation references change.

## Scope

In scope:

- Rename the cobra subcommand from `info` to `get` on the `config` command (`internal/cli/config.go`).
- Rename `internal/cli/config_info.go` to `internal/cli/config_get.go`.
- Rename the internal Go symbols defined in that file: `addConfigInfoCommand` → `addConfigGetCommand`, `runConfigInfo` → `runConfigGet`, `runConfigInfoInteractive` → `runConfigGetInteractive`, `printConfigInfo` → `printConfigGet`. The four per-category renderers (`printAgentInfo`, `printRoleInfo`, `printContextInfo`, `printTaskInfo`) may be renamed for consistency or kept; renderer naming is an implementation choice.
- Update the cobra `Use:`, `Short:`, and `Long:` fields to use `get` and to clarify how this command differs from the top-level `start get` and `start describe`.
- Rename tests in `internal/cli/config_test.go`, `internal/cli/config_integration_test.go`, `internal/cli/root_test.go`, and `internal/cli/snapshots_test.go` to reflect the new command name. The six `TestSnapshot_ConfigInfo*` functions in `snapshots_test.go` rename to `TestSnapshot_ConfigGet*`; the file header and per-test docstrings update from `config info` to `config get`.
- Update the stale comment at `internal/cli/config_list.go:35` and the comment at `internal/cli/metadata_writers.go:29` to reference `config get` / `config_get` instead of `config info` / `config_info`.
- Update `README.md` and shell scripts to reference `start config get`.

Out of scope:

- Changing the command's resolution behaviour, output format, or JSON shape.
- Adding auto-install, registry lookup, or fuzzy matching to `start config get`. The command operates only on installed config, exactly as `config info` does today.
- Adding a backward-compatibility alias `info` for `get`. Hard cutover.
- Any change to top-level `start get` or `start describe`.
- Any change under `library/` or `homebrew-tap/`.
- Releases or tags.

## Current State

`start config info <name>` is the installed-config detail view (file: `internal/cli/config_info.go`):

- Resolution: substring match across installed config via `searchAllConfigCategories` (from `internal/cli/config_helpers.go`); no auto-install; no registry lookup.
- Multi-match: numbered menu in interactive mode (`promptSelectConfigMatch`); error in non-interactive.
- No-arg interactive mode: prompts for category then item via `promptSelectCategory` + `promptSelectOneFromList`.
- Output: per-category formatters (`printAgentInfo`, `printRoleInfo`, `printContextInfo`, `printTaskInfo`) emitting Source, Origin, category-specific fields, Description, Tags. Agents render a Models alias map (`<alias> -> <id>`). Roles/contexts/tasks render a Prompt truncated to 100 chars and category-specific flags (Optional, Required, Default, Role).
- JSON: `--json` returns `[]ConfigListItem` built via `buildConfigListItem` in `config_list.go`.
- Scope: `--local` only (no `--global` flag on this command).

Top-level `start get <name>` (file: `internal/cli/get.go`) is conceptually different and remains untouched. It resolves a module via `resolveCrossCategory` (auto-install on registry hit) and writes the module's rendered content (file body, rendered prompt, command output, or agent command template) to stdout for piping. After the rename, the two commands coexist: top-level `start get` is about module content; `start config get` is about the installed config entry.

Registration call to update: `internal/cli/config.go:28` — `addConfigInfoCommand(configCmd)` becomes `addConfigGetCommand(configCmd)`.

Inline production strings inside `internal/cli/config_info.go` that carry the old command name and must be updated during the rename:

- Doc comments at lines 13, 32, 100, 127 — references to `config info [query]`, `config info`, `runConfigInfoInteractive`. These regenerate naturally as the symbols are renamed, but verify after the symbol pass.
- Error context strings at lines 62 and 79: `fmt.Errorf("marshalling config info: %w", err)` — the literal `config info` becomes `config get`.
- Interactive header at line 102: `_, _ = fmt.Fprintln(stdout, "Info:")` — change to `Get:` (or another label that does not mislead the user about the active command).

Test files affected:

- `internal/cli/config_test.go` — `TestConfigInfo_Agent` (line 85), `TestConfigInfo_NotFound` (line 139), `TestConfigInfoJSON_MultipleMatches` (line 2076), `TestConfigInfoJSON_WithMatch` (line 2137), `TestConfigInfoJSON_NoArgs` (line 2186), `TestConfigInfoJSON_NotFound` (line 2207).
- `internal/cli/config_integration_test.go` — `TestConfigInfo_ZeroMatch` (line 1222).
- `internal/cli/root_test.go` — help-table entry at lines 133-136 (`config info help`).
- `internal/cli/snapshots_test.go` — six snapshot tests: `TestSnapshot_ConfigInfoAgent` (line 296), `TestSnapshot_ConfigInfoRole` (line 326), `TestSnapshot_ConfigInfoContext` (line 350), `TestSnapshot_ConfigInfoAgentObjectForm` (line 432), `TestSnapshot_ConfigInfoTask` (line 462), `TestSnapshot_ConfigInfoContextWithoutDescription` (line 493). Plus the file header at lines 3-4 ("End-to-end snapshot tests for the describe and config info rendering surfaces …") and the docstrings on five of the six tests (lines 293, 324, 348, 427, 460) that each reference `start config info <category>`. The sixth test's docstring (line 485, for `TestSnapshot_ConfigInfoContextWithoutDescription`) describes layout regression behaviour and does not reference the command name — only its `func` declaration needs renaming. Each snapshot function also calls `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` directly at lines 300, 330, 354, 436, 466, 497; these calls track the optional renderer rename — if the renderers are renamed, the call sites update with them, otherwise they stay.

Stale comments to update:

- `internal/cli/config_list.go:35` reads `// Used by config info and config list JSON paths.`. Update the `config info` reference.
- `internal/cli/metadata_writers.go:29` reads `// (config_info as a header line; describe via ExtractUTDFields).`. Update the `config_info` reference to `config_get` (underscore preserved since it refers to the file name idiomatically).

Documentation references:

- `README.md` — line 193 (`start config info claude`), line 314 (`start config info`), line 315 (`start config info claude`). All three become `start config get`.
- `scripts/manual-test` — lines 115-116 (`config info --help`), line 357 (section heading `section "9. Config Info"`), lines 360-367 (`config info` test cases at 361, 364, 367; section labels at 360, 363, 366). Update command names, the `"config info"` section labels, and the title-case `Config Info` section heading.

Files NOT affected (verified): `AGENTS.md`, `internal/cli/help/agents.md`, and `cmd/` have no `config info` / `config_info` / `ConfigInfo` references.

## Requirements

1. The cobra subcommand `start config info` is renamed to `start config get`. Invoking `start config info` returns cobra's unknown-command error. `start config get` produces the output that `start config info` produced before this project.
2. `internal/cli/config_info.go` is renamed to `internal/cli/config_get.go`. The symbols `addConfigInfoCommand`, `runConfigInfo`, `runConfigInfoInteractive`, and `printConfigInfo` are renamed to `addConfigGetCommand`, `runConfigGet`, `runConfigGetInteractive`, and `printConfigGet`.
3. The cobra registration call at `internal/cli/config.go:28` calls `addConfigGetCommand(configCmd)`.
4. The cobra `Use:` field changes from `info [query]` to `get [query]`. `Short:` and `Long:` are updated to use `get` consistently and to note the contrast with top-level `start get` (which outputs module content) and `start describe` (which shows the verbose resolved dump).
5. Inline production strings inside the renamed file are updated: the doc comments at lines 13, 32, 100, 127 reflect the new symbol/command names; the error context at lines 62 and 79 (`marshalling config info`) becomes `marshalling config get`; the interactive header at line 102 (`Info:`) is updated so it does not mislabel the renamed command.
6. All `TestConfigInfo*`, `TestConfigInfoJSON*`, and `TestSnapshot_ConfigInfo*` test functions that pertain solely to the `config info` / `config get` command are renamed to `TestConfigGet*`, `TestConfigGetJSON*`, and `TestSnapshot_ConfigGet*`. Exception: `TestConfigInfo_ZeroMatch` at `internal/cli/config_integration_test.go:1222` is a multi-command zero-match contract test (iterating over `edit`, `remove`, `info`) and is renamed to the category-neutral `TestConfig_ZeroMatch`; the `"info"` entry in its loop slice is updated to `"get"`. Inline command-name strings inside the other test bodies (e.g. `"info"` in `cmd.SetArgs([]string{"config", "info", ...})`, `"config info"` in subtest names and error messages) are updated. The `snapshots_test.go` file header (lines 3-4) and the five docstrings that reference `start config info <category>` (lines 293, 324, 348, 427, 460) are updated to `config get`. If the optional renderer rename is taken, the six direct renderer calls in `snapshots_test.go` (lines 300, 330, 354, 436, 466, 497) update to match the new symbol names.
7. The help-table entry at `internal/cli/root_test.go:133-136` is updated from `config info help` to `config get help`, with the expected output reflecting the renamed cobra command.
8. The comment at `internal/cli/config_list.go:35` is updated to read `config get` in place of `config info`. The comment at `internal/cli/metadata_writers.go:29` is updated to read `config_get` in place of `config_info`.
9. README.md and `scripts/manual-test` are updated to reference `start config get` wherever they currently reference `start config info`. Section labels in `scripts/manual-test` (`"config info"`) become `"config get"`, and the title-case section heading at line 357 (`section "9. Config Info"`) becomes `section "9. Config Get"`.
10. The command's flag set (`--local`, `--json`), JSON output shape (`[]ConfigListItem`), resolution behaviour, multi-match handling, no-arg interactive flow, and per-category output formatting are unchanged.
11. No backward-compatibility alias `info` is added. Hard cutover.
12. Final verification: `rg -in 'addConfigInfoCommand|runConfigInfo|printConfigInfo|ConfigInfo' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches. `rg -in 'config info\b|config_info' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches. (Other project documents under the repo root may continue to mention `config info` for historical context and are not scanned.) The `ConfigInfo` portion of the symbol regex deliberately broadens from `TestConfigInfo` to catch `TestSnapshot_ConfigInfo*`; the `config_info` portion of the text regex catches the underscore variant in comments; the `-i` flag catches title-case forms such as `Config Info` in script section headings.

## Implementation Plan

1. Rename the file `internal/cli/config_info.go` to `internal/cli/config_get.go`. Inside the new file, rename `addConfigInfoCommand`, `runConfigInfo`, `runConfigInfoInteractive`, and `printConfigInfo` to their `Get` counterparts. Change the cobra `Use:` field to `get [query]`. Rewrite the `Long:` description: it currently ends with `This shows raw stored fields, not resolved content. Use 'start describe' to view resolved content after global/local merging.` Update to clarify the three-command landscape — `start config get` shows raw installed config fields; top-level `start get` outputs resolved module content; `start describe` shows the verbose resolved dump. Also update the inline production strings flagged in Current State: the two `marshalling config info` error contexts (lines 62, 79) and the `Info:` interactive header (line 102).
2. Update `internal/cli/config.go:28` to call `addConfigGetCommand(configCmd)`.
3. Update the stale comment at `internal/cli/config_list.go:35` to reference `config get` instead of `config info`. Update the comment at `internal/cli/metadata_writers.go:29` to reference `config_get` instead of `config_info`.
4. Rename the test functions in `internal/cli/config_test.go`, `internal/cli/config_integration_test.go`, and `internal/cli/snapshots_test.go` to their `Get` counterparts. Within each test body, update any string literals referencing the old command (e.g. `"info"`, `"config info"`, error messages mentioning the command name) to use `get` / `config get`. For `snapshots_test.go` specifically: rename the six `TestSnapshot_ConfigInfo*` functions to `TestSnapshot_ConfigGet*`, update the file header at lines 3-4 and the per-test docstrings to say `config get`, and — if the optional renderer rename is taken — update the six direct renderer calls (`printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo`) to match the new symbol names.
5. Update the help-table entry at `internal/cli/root_test.go:133-136` from `config info help` to `config get help`, including the expected `Short:` text from the renamed cobra command.
6. Update documentation: rewrite the three `start config info` lines in `README.md` (193, 314, 315) to `start config get`. In `scripts/manual-test`, update lines 115-116 and 360-367 to use the new command name, update the `"config info"` section labels, and update the title-case section heading at line 357 (`section "9. Config Info"` → `section "9. Config Get"`).
7. Run the verification queries from requirement 12. Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Hard cutover. No `info` alias.
- Behaviour, output, flags, and JSON shape are unchanged. This is a rename only.
- Shared helpers in `config_helpers.go`, `config_list.go`, and `config_types.go` retain their current names and signatures. Only the comment update in `config_list.go:35` falls inside this project.
- The per-category renderers (`printAgentInfo`, `printRoleInfo`, `printContextInfo`, `printTaskInfo`) are internal to the renamed file; renaming them is at the implementer's discretion and does not affect external callers.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- This project is independent of project 01 (`01-drop-redundant-search.md`). It can land before, after, or alongside it. There is no file overlap: project 01 touches `config_search.go` and `modules_search.go`; this project touches `config_info.go`.
- `start config get` and top-level `start get` coexist after this project. The pattern matches `kubectl config get-contexts` versus `kubectl get`. The two surfaces are distinct: top-level `get` operates on module content (file body, rendered prompt, command output, or agent command template); `config get` operates on the config entry itself (Source, Origin, Tags, category-specific fields). The updated `Long:` description should make this distinction explicit so users do not assume the two commands are aliases or shorthands of each other.

## Issues Discovered

1. Verb choice for the renamed command (decision) — Resolved: use `get`.

   `start config info` could be renamed to several canonical verbs: `get`, `view`, `show`, or `describe`. The choice affects whether the new name conflicts conceptually with existing commands.

   - `get`: matches the established convention (`kubectl config get-contexts`, `git config --get`, `gh config get`). Conflicts conceptually with top-level `start get`, which operates on module content rather than config entries. The conflict is mitigated by scoping under the `config` subcommand and by clarifying in the help text.
   - `view`: less precedented in start's CLI surface. Avoids the conceptual overlap with top-level `get` but introduces a verb not used elsewhere in the tool.
   - `show`: project 02 renamed `show` to `describe` at the top level. Reintroducing `show` here would undo that direction.
   - `describe`: overlaps with top-level `start describe`, which already covers installed-config inspection via the verbose dump.

   Resolution: use `get`. The conceptual overlap with top-level `start get` is acknowledged and addressed in the renamed command's `Long:` description. The pattern `<group> get <thing>` is well-established in the broader CLI ecosystem.

2. `internal/cli/snapshots_test.go` and `internal/cli/metadata_writers.go` are missing from documented scope (gap) — Resolved: expanded scope to cover both files.

   The Current State claims only `config_test.go`, `config_integration_test.go`, and `root_test.go` need test renames, and asserts "Files NOT affected (verified): `AGENTS.md`, `internal/cli/help/agents.md`". Two additional files carry references that need updating:

   - `internal/cli/snapshots_test.go` contains six test function names (`TestSnapshot_ConfigInfoAgent` at line 296, `TestSnapshot_ConfigInfoRole` at line 326, `TestSnapshot_ConfigInfoContext` at line 350, `TestSnapshot_ConfigInfoAgentObjectForm` at line 432, `TestSnapshot_ConfigInfoTask` at line 462, `TestSnapshot_ConfigInfoContextWithoutDescription` at line 493) and ~10 comment occurrences of `config info` (including the file header at lines 3-4 and per-test docstrings). The tests call `printAgentInfo`, `printRoleInfo`, `printContextInfo`, `printTaskInfo` directly — if the optional renderer rename is taken, those call sites need updating too.
   - `internal/cli/metadata_writers.go:29` carries a comment referencing `config_info as a header line`.

   Consequence: requirement 12's verification regex `rg -n 'config info\b' internal/ cmd/ test/ scripts/ README.md AGENTS.md` will hit the `snapshots_test.go` comment lines and fail acceptance. The snapshot test function names (`TestSnapshot_ConfigInfo*`) survive the symbol regex because they don't match `TestConfigInfo`, and the `metadata_writers.go` reference uses an underscore so it survives both regexes — both would remain as silent stale references unless explicitly addressed.

   Resolution: Scope, Current State, Requirements, Implementation Plan, and Acceptance Criteria extended to cover both files. The six `TestSnapshot_ConfigInfo*` functions are renamed to `TestSnapshot_ConfigGet*`; the file header at lines 3-4 and per-test docstrings in `snapshots_test.go` are updated from `config info` to `config get`; the comment at `metadata_writers.go:29` is updated from `config_info` to `config_get`. The renderer rename remains optional — if taken, the six direct calls to `printAgentInfo`/`printRoleInfo`/`printContextInfo`/`printTaskInfo` in `snapshots_test.go` update accordingly; if not, they stay. Verification regexes are widened to catch the new patterns.

3. `scripts/manual-test` section heading at line 357 is missing from scope (gap) — Resolved: extended scope and widened verification regex to case-insensitive.

   The Current State and Implementation Plan enumerate `scripts/manual-test` updates at lines 115-116 and 360-367, and Requirement 9 says the `"config info"` section labels (the first argument to `cmd` at lines 360, 363, 366) become `"config get"`. They do not enumerate the section heading at line 357, which reads `section "9. Config Info"`. After the rename this heading would be displayed as `9. Config Info` while every command beneath it runs `config get`, which is a confusing stale label in the script's printed output.

   The heading also escapes the documented verification regex `rg -n 'config info\b|config_info'`. That regex is case-sensitive by default, so `Config Info` (title case) does not match. The implementer running the verification commands at the end of step 7 would see clean output even with the stale heading in place.

   Resolution: Scope, Current State, Requirement 9, Implementation Plan step 6, and Acceptance Criteria extended to cover `scripts/manual-test:357` (`section "9. Config Info"` → `section "9. Config Get"`). Requirement 12 and the matching Acceptance Criteria bullets switched from `rg -n` to `rg -in` so future case-mismatch references are caught automatically.

4. `TestConfigInfo_ZeroMatch` is a multi-command test and is misnamed after a blanket rename (design) — Resolved: rename to `TestConfig_ZeroMatch` and update loop slice.

   `TestConfigInfo_ZeroMatch` at `internal/cli/config_integration_test.go:1222` iterates over `[]string{"edit", "remove", "info"}` and asserts not-found behaviour for all three subcommands in a single function. The project's Requirement 6 prescribes renaming all `TestConfigInfo*` functions to `TestConfigGet*`. Applied verbatim, `TestConfigInfo_ZeroMatch` would become `TestConfigGet_ZeroMatch`, which then misrepresents what the test covers — it would still iterate over `edit` and `remove` alongside the renamed `get`.

   Resolution: Requirement 6 carves this test out as a named exception. `TestConfigInfo_ZeroMatch` is renamed to the category-neutral `TestConfig_ZeroMatch` (matching the existing `TestConfig*` naming convention in the same package) and the `"info"` entry in the loop slice becomes `"get"`. The symbol verification regex still passes — it widens from `TestConfigInfo` to `ConfigInfo` and the neutral name does not contain `ConfigInfo`.

## Acceptance Criteria

- `start config --help` lists `get` as a subcommand and does not list `info`.
- `start config get <installed-name>` produces output equivalent to `start config info <installed-name>` produced before this project, for all four categories.
- `start config get --json <name>` produces the same `[]ConfigListItem` JSON shape as `start config info --json` did before.
- `start config info` exits with cobra's unknown-command error.
- `internal/cli/config_info.go` does not exist; `internal/cli/config_get.go` does.
- `rg -in 'addConfigInfoCommand|runConfigInfo|printConfigInfo|ConfigInfo' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches.
- `rg -in 'config info\b|config_info' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches.
- `README.md` and `scripts/manual-test` reference `start config get` in all locations that previously referenced `start config info`, including the title-case section heading at `scripts/manual-test:357`.
- `internal/cli/snapshots_test.go` defines `TestSnapshot_ConfigGet*` functions in place of the previous `TestSnapshot_ConfigInfo*` set, with no remaining `config info` references in the file header, per-test docstrings, or test bodies.
- `internal/cli/metadata_writers.go` contains no `config_info` reference.
- `scripts/invoke-tests` passes.
