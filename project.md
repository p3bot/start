# Project: Local User Aliases for Module Names

## Goal

Add a user-defined alias mechanism to the `start` CLI so heavily-used commands can be invoked by a short top-level token. Typing `start pc` runs the full command the alias points at (for example `start task review/pre-commit`), reducing keystrokes for commands run many times a day.

## Scope

In scope:

- A global-only alias store and an embedded schema for it.
- A top-level resolver that rewrites `start <alias> ...` into a full `start` invocation before command dispatch.
- Composition: one alias may set a task plus role, agent, and contexts in a single invocation.
- A management command (`start alias`) to list, show, set, and delete aliases.
- Integration with existing `config` surfaces (list view, `config open`, `config export`) and `doctor`.

Out of scope:

- Per-project (`--local`) aliases. Aliases are global only.
- Unqualified alias values. Every target is a qualified `category:path` address.
- In-command expansion. `start task pc` does not consult aliases; only the top-level `start pc` form does.
- Arbitrary command strings or shell fragments as values. A value is only typed module addresses.
- A `models:` category. Models are per-agent; the `--model` flag still works as a trailing flag on an aliased command.
- Any change to the published library or registry. This is a client-only feature.

## Current State

`start` is a cobra-based Go CLI built on CUE. Configuration lives in two scopes: global (`~/.config/start/`, or `$XDG_CONFIG_HOME/start`) and local (`./.start/`). Each category has its own file: `agents.cue`, `roles.cue`, `contexts.cue`, `tasks.cue`, `settings.cue`. Paths are resolved by `config.ResolvePaths` returning a `config.Paths` with `Global`/`Local` and existence flags; `Paths.Dir(local)` selects a scope directory.

Configuration loading and merging is in `internal/cue/loader.go`. The loader merges directories (global then local, local wins). Category collections (`agents`/`roles`/`contexts`/`tasks`) merge additively by item name; other top-level keys (such as `settings`) merge field-by-field. CUE key constants and the category-to-filename map are in `internal/cue/keys.go` (`KeyAgents`, `KeySettings`, `ConfigFiles`). Runtime config loading does not validate against schemas; only the maintainer-facing `doctor validate` does.

The settings feature is the closest existing analogue and the template for the management command. Relevant files:

- `internal/config/settings.go` — `LoadSettingsFromDir`, `ResolveAllSettings`, `SettingsRegistry`, scope handling.
- `internal/cli/config_settings.go` — the `config settings` command: list, show, set, `--unset`, `edit`, `--json`. Note `writeSettingsFile` builds the CUE file by hand-rolled string construction; this project must not copy that approach (see Constraints).

Name resolution and address parsing:

- `internal/cli/describe.go` — `parseAddress` splits `category:name` on the first colon and validates the category against the four known categories; `formatAddress` produces `category:name`; `describeCategories` lists the four categories with their CUE keys and display types.
- `internal/cli/resolve.go` and `internal/cli/cross_resolve.go` — the three-tier resolver used by execution and module commands. After an alias rewrite, the resolved command flows through this unchanged, including search, interactive selection, and not-found handling.

Command wiring:

- `internal/cli/root.go` — `NewRootCmd` builds the root command, registers every subcommand (`addTaskCommand`, `addConfigCommand`, etc.), sets persistent flags, and `Execute()` constructs and runs the root command. `PersistentPreRunE` stores `Flags` on the context and normalises the `none` sentinel.
- Persistent flags (root.go): `--agent`/`-a`, `--role`/`-r`, `--model`/`-m`, `--context`/`-c` (string slice, repeatable), `--dry-run`, `--quiet`/`-q`, `--verbose`, `--debug`, `--color`, `--local`/`-l`.
- `internal/cli/start.go` — `runStart` is the root `RunE`; it reads piped stdin as prompt text and otherwise runs the default flow. It ignores positional arguments.
- Verified behaviour: an unknown first token (e.g. `start zzdoesnotexist`) currently produces cobra's `unknown command "..."` error. The alias resolver must intercept before this error is produced.

Config command family:

- `internal/cli/config.go` — `addConfigCommand` registers subcommands; `runConfigList` renders the config list view with a section per category.
- `internal/cli/config_open.go` — `openCategories` and `resolveConfigOpenPath` map a category to its `.cue` filename for `$EDITOR`.
- `internal/cli/config_export.go` — exports merged configuration.

Doctor lives in `internal/doctor/` with command wiring in `internal/cli/doctor.go`.

Tests use `setupStartTestConfig(t)` (temp `.start/`, `os.Chdir`, `$HOME` isolation), table-driven cases, real CUE validation via `t.TempDir()`, and registry stubbing through the per-command provider. Offline `--json` coverage uses `captureJSON(t, stub, args...)`.

## References

- GitHub issue #5 ("feat(alias): add local user aliases to module names"), `git@github.com:start-cli/start.git` — the originating feature request.

## Requirements

1. Alias store. Aliases are stored in the global config only, in a dedicated managed file `aliases.cue` containing a single top-level `aliases:` field. The field maps an alias name to a list of qualified addresses.

2. Value format. Each address is `category:path` where category is one of `agents`, `roles`, `contexts`, `tasks` (plural). `path` matches the module-name shape: lowercase, kebab-case segments, slash-separated. Names and values are case-insensitive and normalised to lowercase on write; an uppercase input resolves identically to its lowercase form. Path existence is not validated — a well-formed alias whose target does not exist resolves exactly as if the user had typed that command, driving the normal search/selection/not-found behaviour.

3. Composition and cardinality. A value may compose multiple targets. Cardinality limits, enforced on `set` and by the schema: at most one `tasks:`, at most one `roles:`, at most one `agents:`, and any number of `contexts:`. A value with two tasks, two roles, or two agents is rejected.

4. Dispatch. An alias value maps to a single `start` invocation:

   | Category | Contributes |
   | -------- | ----------- |
   | `tasks:X` | `task X` (the subcommand) |
   | `roles:X` | `--role X` |
   | `agents:X` | `--agent X` |
   | `contexts:X` | `--context X` (one per context) |

   Token order is deterministic: the `task` subcommand first when present, then flags in a fixed order. The expanded token sequence used for display and for execution must be identical.

5. Top-level resolution. Before cobra command dispatch, identify the first positional argument (persistent flags may appear in any position and are passed through). When that token is not a known subcommand or subcommand alias and exactly matches an alias name (case-insensitive), replace the token in place with the alias's expansion tokens and let the rewritten invocation run through cobra normally. All other arguments retain their position, so trailing arguments and flags are preserved:

   ```
   start pc                 → start task review/pre-commit
   start pc "fix the lint"  → start task review/pre-commit "fix the lint"
   start pc --model opus    → start task review/pre-commit --model opus
   start --debug pc         → start --debug task review/pre-commit
   start dev                → start --role go-expert --context cwd/agents-md
   ```

   When the first token matches no subcommand and no alias, behaviour is unchanged (cobra's `unknown command` error). Bare `start` with no positional token is unchanged. There is no recursion: values reference categories and paths, never other aliases.

6. Lazy, resilient loading. Aliases are consulted only when the first positional token is not a known subcommand. The common cases (`start task ...`, `start config ...`, bare `start`) never load aliases, so a malformed `aliases.cue` cannot break them. When aliases are consulted and the store fails to load or validate, surface a clear error at that point.

7. Management command. A top-level command `alias` (with cobra alias `aliases`), global config only:

   ```
   start alias                                    list all aliases
   start alias pc                                 show one alias as its full expanded command
   start alias pc=tasks:review/pre-commit pr=...  set one or many
   start alias --unset pc pr                       delete one or many
   ```

   - List: render each alias as name plus its full expanded command (the same expansion used at resolution, prefixed so it reads as a runnable `start ...` command).
   - Show: render the single alias as its full expanded command, not the raw stored value. An unset name reports "not set".
   - Set: space separates multiple aliases; comma separates composed targets within one value. Each value is validated before any write; an invalid value fails the whole `set` without writing.
   - Unset: variadic; an absent key reports "not set" rather than erroring (matching `config settings --unset`).
   - `--json` is supported on the no-argument list and the single-name show. JSON output is structured and includes both the raw value and the expanded command so scripts can consume either.
   - `--quiet` suppresses confirmation lines.
   - An alias name is any non-empty string except: it may not contain `=`, and it may not equal any registered subcommand name or subcommand alias. A colliding name is rejected on `set` (cobra would shadow it, so it could never fire). Derive the forbidden set from the command tree at runtime rather than hardcoding it, so it stays correct as commands change.

8. Embedded schema and validation. Embed a `#Aliases` CUE schema in the binary. Validate the alias store at three points: on write (reject a bad `set`), on load when consulted (surface a clear error), and in `start doctor` (validate the file and report alias count and any problems). The schema enforces address shape; per-category cardinality is enforced in Go where CUE cannot express it cleanly.

9. Config integration.

   - `config open aliases` (and `alias`) opens `aliases.cue` in `$EDITOR`.
   - The `start config` list view gains an `aliases/` section alongside the existing category sections.
   - `config export` includes aliases in exported configuration.
   - `config remove` is not extended; alias deletion is `start alias --unset`.

10. Tests. Cover value parsing and validation (valid forms, invalid category, cardinality breaches, case-insensitivity), the dispatch/expansion builder, the top-level resolver rewrite (including flags before the token, trailing args, no-match passthrough, and subcommand-name collisions), the management command (list, show-expanded, set single and multiple, unset present and absent, `--json`), and the `doctor` and `config` integrations. Follow existing test conventions (`setupStartTestConfig`, table-driven, real CUE, `captureJSON` for `--json`).

## Constraints

- Go, cobra, and CUE as already used in the repository. No new heavyweight dependencies for this feature.
- Read and write the alias store via the CUE Go API. Decode by loading and looking up the `aliases` field through the CUE value API; encode by constructing a CUE value or syntax tree and formatting it with CUE's formatter so output is canonical. Do not hand-roll CUE serialisation with string building, and do not copy `writeSettingsFile`'s string-builder approach.
- The managed `aliases.cue` holds only the `aliases` field. Before overwriting, guard against clobbering unrelated top-level content: if the existing file contains non-alias top-level keys, refuse to write and direct the user to edit the file manually.
- Aliases are global only. The `alias` command and the resolver never read or write local config for aliases.
- The feature is client-only. Do not add anything to the library or registry; the schema is embedded in the binary.
- Resolution after an alias rewrite must reuse the existing resolver and command paths unchanged. The alias layer only rewrites argv; it does not reimplement search, selection, or installation.

## Implementation Plan

1. Alias store and schema. Add a config-layer component that owns the global `aliases.cue` path, an embedded `#Aliases` schema, loading (decode the `aliases` field from the global config via the CUE API), and writing (encode via the CUE API with the non-alias-content guard). Normalise names and values to lowercase. Storage shape is a map of name to a list of qualified-address strings.

2. Value parsing, validation, and dispatch. Implement parsing of a comma-separated value into addresses (reusing `parseAddress` for the `category:path` split and category validation), cardinality validation (the limits in Requirement 3), and a dispatch builder that turns a validated value into the deterministic expansion token sequence (Requirement 4). Provide a helper that renders the expansion as a displayable `start ...` command string for the management command.

3. Top-level resolver. Add interception ahead of cobra command dispatch (in or alongside `Execute`). Determine the first positional token with persistent flags stripped, decide whether it is a known subcommand, and when it is not, load the alias store and attempt a case-insensitive match. On a match, rewrite argv in place and continue to cobra; on load/validation failure surface the error; otherwise fall through unchanged. Keep the load lazy per Requirement 6.

4. Management command. Add the top-level `alias` command (cobra alias `aliases`) with list, show (expanded), set (validated, space-separated multiples, comma-separated composition), `--unset` (variadic), `--json`, and `--quiet`. Enforce the name rules including the dynamically-derived subcommand-collision check. Reuse the validation and dispatch helpers from step 2.

5. Config and doctor integration. Wire `aliases`/`alias` into `config open`, add the `aliases/` section to the `config` list view, include aliases in `config export`, and add an alias-store validation check to `start doctor`.

6. Tests and documentation. Add the test coverage in Requirement 10. Update `AGENTS.md` command listings and any user-facing help to document `start alias` and the `start <alias>` shortcut.

## Implementation Guidance

- The dispatch builder is the single source of truth for the expansion token sequence. Both the argv rewrite (execution) and the management command's show/list (display) must call it, so an alias always shows exactly what it runs.
- Prefer deriving the forbidden-name set and the known-subcommand check from the live cobra command tree (`Commands()` and their `Aliases`) so new commands are covered automatically.
- For the first-positional detection in the resolver, prefer leveraging cobra's own flag-aware command lookup rather than re-implementing flag parsing by hand; persistent flags can legitimately appear before the alias token.
- Keep the resolver's failure mode narrow: only invocations whose first token is a non-subcommand reach the alias load, so its errors never affect ordinary subcommands.

## Acceptance Criteria

- `start alias pc=tasks:review/pre-commit` then `start pc` runs the same command as `start task review/pre-commit`, and `start pc "msg"` passes `msg` through as task instructions.
- A composed alias such as `dev=roles:go-expert,contexts:cwd/agents-md` expands to `start --role go-expert --context cwd/agents-md`, and a task-plus-role alias expands to `start task <path> --role <role>`.
- `start alias pc` and `start alias` display aliases as their full expanded `start ...` commands, not the raw stored values.
- Setting a value with an invalid category, with two tasks/roles/agents, or with an empty value is rejected and writes nothing.
- An alias name equal to a subcommand name or alias is rejected on `set`.
- `TASKS:REVIEW/PRE-COMMIT` and `tasks:review/pre-commit` produce identical stored aliases and identical resolution.
- `aliases.cue` is written and read via the CUE API and contains only the `aliases` field; a file containing other top-level keys is not overwritten.
- Aliases are read and written only in global config; `--local` has no effect on aliases.
- A first token matching neither a subcommand nor an alias still yields cobra's `unknown command` error; bare `start` is unchanged; `start task <name>` does not consult aliases.
- A malformed `aliases.cue` does not break `start task ...`, `start config ...`, or bare `start`; `start doctor` reports it.
- `config open aliases` opens the file, the `config` list view shows an `aliases/` section, and `config export` includes aliases.
