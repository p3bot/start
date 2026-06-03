# Project: Local User Aliases for Module Names

## Goal

Add an isolated user-convenience alias layer to the `start` CLI so a short top-level token expands into a full invocation. `start pc` runs what `pc` points at (for example `start task review/pre-commit`), saving keystrokes on commands run many times a day. Aliases are a personal shortcut system only — not a launch feature, not distributable, and not a config category.

## Scope

In scope:

- A global-only alias store (`_aliases.cue`) and an embedded schema for it.
- A top-level resolver that rewrites `start <alias> ...` into a full `start` invocation before command dispatch.
- Composition: one alias may set a task plus role, agent, and contexts in a single invocation.
- A self-contained `start alias` command (list, set, get, delete, open, export) that owns every alias operation.
- A `start doctor` health check for the alias store.
- One change to core config: `config export` excludes underscore-prefixed files.

Out of scope:

- Per-project (`--local`) aliases. Aliases are global only.
- Any other config-command integration. `config open`, `config list`, `config add`, and `config remove` are not extended for aliases; all alias operations live under `start alias`.
- Unqualified alias values. Every target is a qualified `category:path` address.
- In-command expansion. `start task pc` does not consult aliases; only the top-level `start pc` form does.
- Arbitrary command strings or shell fragments as values. A value is only typed module addresses.
- A `models:` category. Models are per-agent; the `--model` flag still works as a trailing flag on an aliased command.
- Any change to the published library or registry. This is client-only; the schema is embedded in the binary.

## Current State

`start` is a cobra-based Go CLI built on CUE. Global config lives in `~/.config/start/` (or `$XDG_CONFIG_HOME/start`); local config in `./.start/`. Each category has its own file (`agents.cue`, `roles.cue`, `contexts.cue`, `tasks.cue`, `settings.cue`). `config.ResolvePaths` returns a `config.Paths` with `Global`/`Local` and existence flags; `Paths.Dir(local)` selects a scope directory.

Config loading and merging is in `internal/cue/loader.go`. The directory loader builds each scope with CUE's package loader (`load.Instances` with `Package: "*"`), merging global then local. CUE key constants and the category-to-filename map are in `internal/cue/keys.go`. Runtime config loading does not validate against schemas; only the maintainer-facing `doctor validate` does.

Verified CUE behaviour (`cuelang.org/go` v0.16.1): CUE's loader ignores files whose names begin with `_`, so a package-less `_aliases.cue` is excluded from every directory package build. A directory whose only `.cue` file is `_aliases.cue` builds as an empty instance with no error. A malformed `_aliases.cue` beside valid config files does not fail the build, and the excluded file's fields never appear in the merged value. `HasCUEFiles` in `loader.go` checks for a `.cue` suffix, so it reports true for a directory containing only `_aliases.cue`; this is not load-bearing for the resilience guarantee, since the build excludes the file regardless.

Address parsing lives in `internal/cli/describe.go`. `parseAddress` splits `category:name` on the first colon and validates the left segment against the four known categories (`describeCategories`); a colon-less token returns success as a bare name, so qualification must be enforced separately from the category check. `formatAddress` produces the canonical `category:name`.

Command wiring is in `internal/cli/root.go`. `NewRootCmd` builds the root command, registers subcommands, and sets persistent flags (`--agent`/`-a`, `--role`/`-r`, `--model`/`-m`, `--context`/`-c`, `--dry-run`, `--quiet`/`-q`, `--verbose`, `--debug`, `--color`, `--local`/`-l`). `Execute()` is `NewRootCmd().Execute()` with no pre-dispatch interception. An unknown first token currently yields cobra's `unknown command` error. cobra's flag-aware lookup (`ParseFlags` followed by `Find` / `Flags().Args()`) correctly treats a token consumed as a flag value (for example `pc` in `start --role pc`) as not a positional argument.

The three-tier resolver used by execution and module commands is in `internal/cli/resolve.go` and `cross_resolve.go`. After an alias rewrite, the resolved command flows through this unchanged, including search, interactive selection, and not-found handling.

`internal/cli/config_settings.go` manages a key-value store with load, show, set, `--unset`, and scope handling; its `writeSettingsFile` builds CUE by hand-rolled string construction, which this project must not copy. The verb-subcommand structure of the `config` command family (`config add`, `config remove`, `config list`, `config open`, `config export`) is the structural model for the `alias` subcommands.

`internal/cli/config_export.go` holds `printCueFiles`, which walks the scope directory and streams every `.cue` file's raw bytes, filtering only on the `.cue` extension.

Doctor lives in `internal/doctor/` with command wiring in `internal/cli/doctor.go`; `prepareDoctor` appends report sections produced by `doctor.Check*` functions.

Tests use `setupStartTestConfig(t)` (temp `.start/`, `os.Chdir`, `$HOME` isolation), table-driven cases, real CUE validation via `t.TempDir()`, and `captureJSON(t, stub, args...)` for `--json` coverage.

## References

- GitHub issue #5 ("feat(alias): add local user aliases to module names"), `git@github.com:start-cli/start.git` — the originating feature request.

## Requirements

1. Alias store. Aliases are stored only in global config, in a managed file `_aliases.cue` holding a single top-level `aliases:` field that maps an alias name to a list of qualified addresses. The leading underscore is load-bearing: CUE's loader ignores `_`-prefixed files, so `_aliases.cue` is excluded from every package build automatically, with no change to the shared directory loader. The store is never read through the directory loader; every alias-consuming surface compiles the single file in isolation. The user-facing category is `aliases` and the command is `alias`; only the on-disk filename carries the underscore. The file lives in the global config directory because that is where global state lives and where `start alias open` reads and writes it.

2. Value format. Each address is `category:path` where category is one of `agents`, `roles`, `contexts`, `tasks`. `path` matches the module-name shape: lowercase, kebab-case segments, slash-separated. Every address must carry an explicit category prefix; a colon-less value is rejected on `set` (note `parseAddress` accepts a bare name with no error, so the qualification check is separate from its category validation). Names and values are case-insensitive and normalised to lowercase on write; an uppercase input resolves identically to its lowercase form. Path existence is not validated — a well-formed alias whose target does not exist resolves exactly as if the user had typed that command, driving the normal search/selection/not-found behaviour.

3. Composition and cardinality. A value may compose multiple targets. Cardinality limits are enforced in Go, because the embedded `#Aliases` schema covers address shape only and CUE cannot express per-category list cardinality cleanly (see Requirement 8): at most one `tasks:`, at most one `roles:`, at most one `agents:`, and any number of `contexts:`. A value breaching a limit is rejected on `set`. The same Go check runs on the resolve path and in `doctor`, so a hand-edited store that breaches a limit is caught — reported by `doctor`, surfaced as an entry error by the resolver per Requirement 6 — rather than silently expanded.

4. Dispatch. An alias value maps to a single `start` invocation:

   | Category | Contributes |
   | -------- | ----------- |
   | `tasks:X` | `task X` (the subcommand) |
   | `roles:X` | `--role X` |
   | `agents:X` | `--agent X` |
   | `contexts:X` | `--context X` (one per context) |

   Token order is deterministic: the `task` subcommand first when present, then flags in a fixed order. The expanded token sequence used for display and for execution must be identical; one builder is the single source of truth.

5. Top-level resolution. Before cobra command dispatch, identify the first positional argument with persistent flags stripped (persistent flags may appear in any position and are passed through). When that token is not a known subcommand or subcommand alias and matches an alias name (case-insensitive), replace the token in place with the alias's expansion tokens and let the rewritten invocation run through cobra normally. All other arguments retain their position, so trailing arguments and flags are preserved:

   ```
   start pc                 → start task review/pre-commit
   start pc "fix the lint"  → start task review/pre-commit "fix the lint"
   start pc --model opus    → start task review/pre-commit --model opus
   start --debug pc         → start --debug task review/pre-commit
   start dev                → start --role go-expert --context cwd/agents-md
   ```

   The rewrite adds no flag-conflict resolution; standard pflag last-wins precedence applies to the merged argv. When an alias sets a flag the user also passes, position decides: a user flag after the alias token wins, one before it is overridden (`start dev --role x` resolves the role to `x`; `start --role x dev` resolves it to the alias's role). When the first token matches no subcommand and no alias, behaviour is unchanged (cobra's `unknown command` error). Bare `start` with no positional token is unchanged. There is no recursion: values reference categories and paths, never other aliases.

6. Lazy, resilient loading. Because `_aliases.cue` is excluded from every package build (Requirement 1), a malformed store never fails the main config load: `start task ...`, bare `start`, and every other subcommand load the merged config without parsing it, regardless of its contents. A global directory containing only `_aliases.cue` loads as an empty instance, not an error. The top-level resolver is lazy and match-scoped: it reads the store only when the first positional token is not a known subcommand, enumerates the alias names, and — only when the token matches a defined name — validates that single entry through the combined validator of Requirement 8 and surfaces a clear error if it is invalid. An absent or empty store, a token matching no name, or a store that cannot be parsed far enough to enumerate names all fall through unchanged to cobra's `unknown command`, so an ordinary typo never surfaces store corruption while an invalid alias still fails loudly the moment it is invoked. The alias-displaying surfaces (`start alias list`/`get` and `start doctor`) report an absent store as zero aliases and surface a clear parse/validation error only for a present-but-invalid store.

7. Management command. A top-level command `alias` (with cobra alias `aliases`), global config only:

   ```
   start alias                       list all aliases
   start alias list                  list all aliases (cobra alias: ls)
   start alias set <name> <value>    create or update one alias
   start alias get <name>            show one alias as its full expanded command
   start alias delete <name>...      delete one or more aliases (cobra alias: rm)
   start alias open                  open _aliases.cue in $EDITOR
   start alias export                print _aliases.cue to stdout
   ```

   - No-argument `start alias` dispatches the same listing as `start alias list`.
   - List and get render each alias as its name plus its full expanded `start ...` command — the same expansion used at resolution — not the raw stored value.
   - Set takes two positional arguments, the name and the value; the value's composed targets are comma-separated. The value is validated before any write; an invalid value writes nothing. Set is an upsert and preserves all other stored aliases.
   - Get on an unset name reports "not set".
   - Delete is variadic; an absent name reports "not set" rather than erroring.
   - `--json` is supported on `list` and `get`. JSON output includes both the raw value and the expanded command so scripts can consume either.
   - `--quiet` suppresses confirmation lines on `set` and `delete`.
   - An alias name is any non-empty single token that does not equal a registered top-level subcommand name or subcommand alias; a colliding name is rejected on `set` (cobra would shadow it at the top level, so it could never fire). Derive the forbidden set from the live command tree at runtime so it stays correct as commands change. Verb-first subcommands mean alias names are otherwise unrestricted — `start alias get open` shows an alias named `open`.

8. Embedded schema and validation. Embed a `#Aliases` CUE schema in the binary for address shape, paired with a Go validator for the per-category cardinality limits CUE cannot express cleanly. Alias-entry validity means both together. Validate through this one combined validator at three points: on write (reject a bad `set`), on the resolve path (validate the matched entry per the match-scoped rule in Requirement 6), and in `start doctor` (validate the file and report alias count and any problems). Sharing one validator across write, resolve, and doctor guarantees a hand-edited breach is caught on the dispatch path the same way doctor catches it, so a breaching entry is never silently expanded.

9. Config export isolation. `config export` excludes underscore-prefixed files from its output. `printCueFiles` currently streams every `.cue` file; it must skip files whose names begin with `_`, so the export reflects exactly the files that form the config package and aligns with CUE's own discovery rule. This keeps the global `_aliases.cue` out of `config export`. No other config command is extended for aliases: `config open`, `config list`, `config add`, and `config remove` gain no alias handling.

10. Tests. Cover value parsing and validation (valid forms, invalid category, unqualified address, cardinality breaches, case-insensitivity), the dispatch/expansion builder, the top-level resolver rewrite (flags before the token; a value-bearing persistent flag whose value equals an alias name — `start --role pc` / `start -r pc` with `pc` defined — which must not rewrite because the token is consumed as the flag's value; trailing args; no-match passthrough; subcommand-name collisions; and the match-scoped failure mode — a token matching a malformed alias entry surfaces that entry's error, while an unrelated unknown token against the same broken store still yields cobra's `unknown command`), the `alias` command (list, get-expanded, set, delete present and absent, `--json`), the `config export` underscore-exclusion filter, and the `doctor` alias check. Follow existing test conventions (`setupStartTestConfig`, table-driven, real CUE, `captureJSON` for `--json`).

## Constraints

- Go, cobra, and CUE as already used in the repository. No new heavyweight dependencies for this feature.
- Read and write the alias store via the CUE Go API. Decode by compiling the single `_aliases.cue` file in isolation (the `ctx.CompileBytes(data, cue.Filename(path))` pattern already used in `internal/cue/loader.go` and `internal/doctor/schema.go`) and looking up the `aliases` field through the CUE value API. Encode by constructing a CUE value or syntax tree and formatting it with CUE's formatter so output is canonical. Do not hand-roll CUE serialisation with string building, and do not copy `writeSettingsFile`'s string-builder approach.
- The managed `_aliases.cue` holds only the `aliases` field. Before overwriting, guard against clobbering content the tool cannot safely round-trip: if the existing file fails to parse, or parses but contains non-alias top-level keys, refuse to write and direct the user to fix or edit the file manually. A malformed `_aliases.cue` is tolerated everywhere else in the design, so `set` must fail closed rather than silently discard it.
- Aliases are global only. The `alias` command and the resolver never read or write local config for aliases; the `alias` command has no `--local` flag.
- The feature is client-only. Do not add anything to the library or registry; the schema is embedded in the binary.
- Resolution after an alias rewrite must reuse the existing resolver and command paths unchanged. The alias layer only rewrites argv; it does not reimplement search, selection, or installation.

## Implementation Plan

1. Alias store and schema. Add a config-layer component that owns the global `_aliases.cue` path, the embedded `#Aliases` schema, isolated loading, and writing. Read the store by compiling the single file's bytes (the `ctx.CompileBytes` pattern) and looking up the `aliases` field — do not load it through the directory loader. The underscore prefix excludes the file from every package build natively, so the directory loader keeps its existing glob untouched. Write via the CUE API with the non-alias-content guard. Normalise names and values to lowercase. Storage shape is a map of name to a list of qualified-address strings.

2. Value parsing, validation, and dispatch. Parse a comma-separated value into addresses (reuse `parseAddress` for the `category:path` split and category validation, plus a qualification check that rejects any colon-less address), validate cardinality (Requirement 3), and build the deterministic expansion token sequence (Requirement 4). Provide a helper that renders the expansion as a displayable `start ...` command string. This builder is the single source of truth for both execution and display.

3. Top-level resolver. Add interception ahead of cobra command dispatch (in or alongside `Execute`). Determine the first positional token with persistent flags stripped using cobra's flag-aware lookup, decide whether it is a known subcommand, and when it is not, load the alias store and attempt a case-insensitive match. On a match against a valid entry, rewrite argv in place and continue to cobra. The failure mode is match-scoped per Requirement 6: enumerate the alias names, and only when the first token matches a defined name validate that single entry through the combined validator of Requirement 8 and surface its error if it is invalid; an absent or empty store, a token matching no name, or a store that cannot be parsed far enough to enumerate names all fall through unchanged. Keep the load lazy.

4. Management command. Add the top-level `alias` command (cobra alias `aliases`) with `list`, `set`, `get`, `delete`, `open`, and `export` subcommands, no-argument invocation defaulting to list, `--json` on list and get, `--quiet` on set and delete, and the dynamically-derived subcommand-collision check on names. Reuse the validation and dispatch helpers from step 2.

5. Config export and doctor. Filter underscore-prefixed files in `printCueFiles` so `config export` omits them. Add an alias-store validation section to `start doctor` that reports alias count and any problems. Make no other config-command change.

6. Tests and documentation. Add the test coverage in Requirement 10. Update `AGENTS.md` command listings and any user-facing help to document `start alias` and the `start <alias>` shortcut.

## Implementation Guidance

- The dispatch builder is the single source of truth for the expansion token sequence. Both the argv rewrite (execution) and the `alias` list/get output (display) must call it, so an alias always shows exactly what it runs.
- Derive the forbidden-name set and the known-subcommand check from the live cobra command tree (`Commands()` and their `Aliases`) so new commands are covered automatically.
- For first-positional detection, leverage cobra's flag-aware command lookup (`ParseFlags` then `Find` / `Flags().Args()`) rather than re-implementing flag parsing by hand. Persistent flags can legitimately precede the alias token, and a flag value that equals an alias name must not be treated as the positional.
- Keep the resolver's failure mode narrow: only an invocation whose first token is a non-subcommand reaches the alias load, so its errors never affect ordinary subcommands.

## Acceptance Criteria

- `start alias set pc tasks:review/pre-commit` then `start pc` runs the same command as `start task review/pre-commit`, and `start pc "msg"` passes `msg` through as task instructions.
- A composed alias `start alias set dev roles:go-expert,contexts:cwd/agents-md` makes `start dev` expand to `start --role go-expert --context cwd/agents-md`, and a task-plus-role alias expands to `start task <path> --role <role>`.
- `start alias get pc` and `start alias list` display aliases as their full expanded `start ...` commands, not the raw stored values.
- Setting a value with an invalid category, an unqualified address (no category prefix), with two tasks/roles/agents, or with an empty value is rejected and writes nothing.
- An alias name equal to a subcommand name or alias is rejected on `set`.
- `start alias set PC TASKS:REVIEW/PRE-COMMIT` and the lowercase form produce identical stored aliases and identical resolution.
- `_aliases.cue` is written and read via the CUE API and contains only the `aliases` field; a file containing other top-level keys is not overwritten.
- Aliases are read and written only in global config; the `alias` command has no `--local` flag.
- A first token matching neither a subcommand nor an alias yields cobra's `unknown command` error; bare `start` is unchanged; `start task <name>` does not consult aliases.
- A malformed `_aliases.cue` does not break `start task ...`, bare `start`, or any other subcommand, and a global directory containing only `_aliases.cue` loads as empty. A present-but-invalid store surfaces a resolver error only when the typed first token matches the malformed alias's name; an unrelated unknown token still yields cobra's `unknown command`. A hand-edited cardinality breach (for example two `tasks:`) is rejected on resolution rather than silently expanded — the same breach `start doctor` reports.
- `start config export` does not include `_aliases.cue`, nor any other `_`-prefixed file, in its output.
- `start alias open` opens the global `_aliases.cue`; `start alias export` prints it to stdout.
