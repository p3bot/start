# Rename start config info to start config get

## Goal

Rename `start config info` to `start config get` so the config subcommand surface uses the canonical `get` verb. This is a pure rename: the command's behaviour, flags, output, and JSON shape are preserved; only the command name, internal Go symbols, file name, test names, and documentation references change.

## Scope

In scope:

- Rename the cobra subcommand from `info` to `get` on the `config` command (`internal/cli/config.go`).
- Rename `internal/cli/config_info.go` to `internal/cli/config_get.go`.
- Rename the internal Go symbols defined in that file: `addConfigInfoCommand` → `addConfigGetCommand`, `runConfigInfo` → `runConfigGet`, `runConfigInfoInteractive` → `runConfigGetInteractive`, `printConfigInfo` → `printConfigGet`. The four per-category renderers (`printAgentInfo`, `printRoleInfo`, `printContextInfo`, `printTaskInfo`) may be renamed for consistency or kept; renderer naming is an implementation choice.
- Update the cobra `Use:`, `Short:`, and `Long:` fields to use `get` and to clarify how this command differs from the top-level `start get` and `start describe`.
- Rename tests in `internal/cli/config_test.go`, `internal/cli/config_integration_test.go`, and `internal/cli/root_test.go` to reflect the new command name.
- Update the stale comment at `internal/cli/config_list.go:35` to reference `config get` instead of `config info`.
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

- `internal/cli/config_test.go` — `TestConfigInfo_Agent` (line 85), `TestConfigInfo_NotFound` (line 139), `TestConfigInfoJSON_MultipleMatches` (line 2259), `TestConfigInfoJSON_WithMatch` (line 2320), `TestConfigInfoJSON_NoArgs` (line 2369), `TestConfigInfoJSON_NotFound` (line 2390).
- `internal/cli/config_integration_test.go` — `TestConfigInfo_ZeroMatch` (line 1222).
- `internal/cli/root_test.go` — help-table entry at lines 133-136 (`config info help`).

Stale comment to update:

- `internal/cli/config_list.go:35` reads `// Used by config info and config list JSON paths.`. Update the `config info` reference.

Documentation references:

- `README.md` — line 193 (`start config info claude`), line 317 (`start config info`), line 318 (`start config info claude`). All three become `start config get`.
- `scripts/manual-test` — lines 115-116 (`config info --help`), lines 369-376 (`config info` test cases). Update command names and the `"config info"` section labels.

Files NOT affected (verified): `AGENTS.md`, `internal/cli/help/agents.md` have no current `config info` references.

## Requirements

1. The cobra subcommand `start config info` is renamed to `start config get`. Invoking `start config info` returns cobra's unknown-command error. `start config get` produces the output that `start config info` produced before this project.
2. `internal/cli/config_info.go` is renamed to `internal/cli/config_get.go`. The symbols `addConfigInfoCommand`, `runConfigInfo`, `runConfigInfoInteractive`, and `printConfigInfo` are renamed to `addConfigGetCommand`, `runConfigGet`, `runConfigGetInteractive`, and `printConfigGet`.
3. The cobra registration call at `internal/cli/config.go:28` calls `addConfigGetCommand(configCmd)`.
4. The cobra `Use:` field changes from `info [query]` to `get [query]`. `Short:` and `Long:` are updated to use `get` consistently and to note the contrast with top-level `start get` (which outputs module content) and `start describe` (which shows the verbose resolved dump).
5. Inline production strings inside the renamed file are updated: the doc comments at lines 13, 32, 100, 127 reflect the new symbol/command names; the error context at lines 62 and 79 (`marshalling config info`) becomes `marshalling config get`; the interactive header at line 102 (`Info:`) is updated so it does not mislabel the renamed command.
6. All `TestConfigInfo*` and `TestConfigInfoJSON*` test functions are renamed to `TestConfigGet*` and `TestConfigGetJSON*`. Inline command-name strings inside test bodies (e.g. `"info"`, `"config info"`, error messages mentioning the command name) are updated.
7. The help-table entry at `internal/cli/root_test.go:133-136` is updated from `config info help` to `config get help`, with the expected output reflecting the renamed cobra command.
8. The comment at `internal/cli/config_list.go:35` is updated to read `config get` in place of `config info`.
9. README.md and `scripts/manual-test` are updated to reference `start config get` wherever they currently reference `start config info`. Section labels in `scripts/manual-test` (`"config info"`) become `"config get"`.
10. The command's flag set (`--local`, `--json`), JSON output shape (`[]ConfigListItem`), resolution behaviour, multi-match handling, no-arg interactive flow, and per-category output formatting are unchanged.
11. No backward-compatibility alias `info` is added. Hard cutover.
12. Final verification: `rg -n 'addConfigInfoCommand|runConfigInfo|printConfigInfo|TestConfigInfo' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches. `rg -n 'config info\b' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches. (Other project documents under the repo root may continue to mention `config info` for historical context and are not scanned.)

## Implementation Plan

1. Rename the file `internal/cli/config_info.go` to `internal/cli/config_get.go`. Inside the new file, rename `addConfigInfoCommand`, `runConfigInfo`, `runConfigInfoInteractive`, and `printConfigInfo` to their `Get` counterparts. Change the cobra `Use:` field to `get [query]`. Rewrite the `Long:` description: it currently ends with `This shows raw stored fields, not resolved content. Use 'start describe' to view resolved content after global/local merging.` Update to clarify the three-command landscape — `start config get` shows raw installed config fields; top-level `start get` outputs resolved module content; `start describe` shows the verbose resolved dump. Also update the inline production strings flagged in Current State: the two `marshalling config info` error contexts (lines 62, 79) and the `Info:` interactive header (line 102).
2. Update `internal/cli/config.go:28` to call `addConfigGetCommand(configCmd)`.
3. Update the stale comment at `internal/cli/config_list.go:35` to reference `config get` instead of `config info`.
4. Rename the test functions in `internal/cli/config_test.go` and `internal/cli/config_integration_test.go` to their `Get` counterparts. Within each test body, update any string literals referencing the old command (e.g. `"info"`, `"config info"`, error messages mentioning the command name) to use `get` / `config get`.
5. Update the help-table entry at `internal/cli/root_test.go:133-136` from `config info help` to `config get help`, including the expected `Short:` text from the renamed cobra command.
6. Update documentation: rewrite the three `start config info` lines in `README.md` (193, 317, 318) to `start config get`. In `scripts/manual-test`, update lines 115-116 and 369-376 to use the new command name and update the `"config info"` section labels.
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

## Acceptance Criteria

- `start config --help` lists `get` as a subcommand and does not list `info`.
- `start config get <installed-name>` produces output equivalent to `start config info <installed-name>` produced before this project, for all four categories.
- `start config get --json <name>` produces the same `[]ConfigListItem` JSON shape as `start config info --json` did before.
- `start config info` exits with cobra's unknown-command error.
- `internal/cli/config_info.go` does not exist; `internal/cli/config_get.go` does.
- `rg -n 'addConfigInfoCommand|runConfigInfo|printConfigInfo|TestConfigInfo' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches.
- `rg -n 'config info\b' internal/ cmd/ test/ scripts/ README.md AGENTS.md` returns no matches.
- `README.md` and `scripts/manual-test` reference `start config get` in all locations that previously referenced `start config info`.
- `scripts/invoke-tests` passes.
