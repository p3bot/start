# Project: Local User Aliases (Saved Commands)

## Goal

Add a personal alias layer to the `start` CLI so a short top-level token expands into a saved `start` command. `start pc` runs what `pc` points at (for example `start task review/pre-commit`), saving keystrokes on commands run many times a day. An alias may also expand to a fixed prompt — `start foo` runs `start prompt "this is the prompt"`.

An alias value is simply the saved argument tokens of a `start` command minus the leading `start`. There is no alias mini-language: the value is whatever you would have typed, captured verbatim and spliced back in at resolution. A value may be a subcommand with flags (`task review/pre-commit --role go-expert --model opus`), flags only (`--role go-expert --context cwd/agents-md`), or a prompt (`prompt "this is the prompt"`). Every flag and subcommand is expressible because the value is literal argv, never a parsed DSL and never re-run through a shell.

Aliases are a personal shortcut system only — global, client-only, not distributable, and not a config category.

## Scope

In scope:

- A global-only alias store (`_aliases.cue`) and a trivial embedded structural schema for it.
- A top-level resolver that rewrites `start <alias> ...` into the saved argv before cobra command dispatch.
- A self-contained `start alias` command (list, get, set, delete, open, export, import) that owns every alias operation.
- A `start doctor` health check for the alias store.
- Underscore-prefixed files are excluded from every direct `.cue` file enumeration that runs outside CUE's package loader: `config export`, doctor's Configuration-section file listing, doctor's schema-validation sweep, and the `autosetup`/`install` broken-file diagnostics.

Out of scope:

- Per-project (`--local`) aliases. Aliases are global only; the `alias` command has no `--local` flag.
- Any other config-command integration. `config open`, `config list`, `config add`, and `config remove` are not extended for aliases; all alias operations live under `start alias`.
- A value DSL. There are no category prefixes, no anchor tokeniser, no cardinality rules, and no mutual-exclusion checks. The value is literal argv tokens.
- Shell fragments or command strings. The value is argv tokens spliced directly into the invocation; it is never interpreted or re-parsed by a shell.
- `--json` on any alias subcommand.
- Recursion. An alias value never references another alias; resolution is single-pass.
- Any change to the published library or registry. This is client-only; the schema is embedded in the binary.

## Current State

`start` is a cobra-based Go CLI built on CUE. Global config lives in `~/.config/start/` (or `$XDG_CONFIG_HOME/start`); local config in `./.start/`. Each category has its own file (`agents.cue`, `roles.cue`, `contexts.cue`, `tasks.cue`, `settings.cue`). `config.ResolvePaths` returns a `config.Paths` with `Global`/`Local` and existence flags; `Paths.Dir(local)` selects a scope directory.

Config loading and merging is in `internal/cue/loader.go`. The directory loader builds each scope with CUE's package loader (`load.Instances` with `Package: "*"`), merging global then local.

VERIFIED, DO NOT RE-TEST (cuelang.org/go v0.16.1, confirmed three times): CUE's loader ignores files whose names begin with `_`. A package-less `_aliases.cue` is therefore excluded from every directory package build automatically, with no change to the shared directory loader. A directory whose only `.cue` file is `_aliases.cue` builds as an empty instance with no error; a malformed `_aliases.cue` beside valid config files does not fail the build, and the excluded file's fields never appear in the merged value. This is the load-bearing resilience guarantee; it is settled and needs no further investigation or test.

Command wiring is in `internal/cli/root.go`. `NewRootCmd` builds the root command, registers subcommands, and sets persistent flags (`--agent`/`-a`, `--role`/`-r`, `--model`/`-m`, `--context`/`-c`, `--dry-run`, `--quiet`/`-q`, `--verbose`, `--debug`, `--color`, `--local`/`-l`). `Execute()` is `NewRootCmd().Execute()` with no pre-dispatch interception. An unknown first token currently yields cobra's `unknown command` error. cobra's flag-aware lookup (`ParseFlags` followed by `Find` / `Flags().Args()`) correctly treats a token consumed as a flag value (for example `pc` in `start --role pc`) as not a positional argument.

`checkHelpArg(cmd, args)` (in `root.go`, used at `task.go:67`) is the established pattern for surfacing help on a command that may receive an arbitrary first argument. As written it matches only the literal token `help` (`args[0] == "help"`); it does NOT detect `-h`/`--help`. For commands with normal flag parsing (`task`, `config settings`) cobra intercepts `--help` before `RunE`, so the gap is invisible there. `alias set` uses `DisableFlagParsing`, so cobra never processes `--help`: the token arrives as `args[0] == "--help"` and the unextended helper lets it fall through to be treated as the alias name. Extend `checkHelpArg` so a leading `-h`/`--help` is also treated as a help request (`args[0]` equal to `help`, `-h`, or `--help`); this is the shared mechanism `alias set` reuses so help works despite `DisableFlagParsing`, and it is a no-op for the flag-parsing commands because cobra catches `--help` ahead of `RunE` there.

The three-tier resolver used by execution and module commands is in `internal/cli/resolve.go` and `cross_resolve.go`. After an alias rewrite, the resolved command flows through this unchanged, including search, interactive selection, and not-found handling.

`internal/cli/config_settings.go` manages a key-value store; its `writeSettingsFile` builds CUE by hand-rolled string construction, which this project must not copy. The verb-subcommand structure of the `config` command family is the structural model for the `alias` subcommands.

`internal/cli/config_export.go` holds `printCueFiles`, which walks the scope directory and streams every `.cue` file's raw bytes, filtering only on the `.cue` extension.

Doctor lives in `internal/doctor/` with command wiring in `internal/cli/doctor.go`; `prepareDoctor` appends report sections produced by `doctor.Check*` functions. The `ctx.CompileBytes(data, cue.Filename(path))` pattern in `internal/cue/loader.go` and `internal/doctor/schema.go` is the way to compile a single CUE file in isolation.

Tests use `setupStartTestConfig(t)` (temp `.start/`, `os.Chdir`, `$HOME` isolation), table-driven cases, and real CUE validation via `t.TempDir()`.

## References

- GitHub issue #5 ("feat(alias): add local user aliases to module names"), `git@github.com:start-cli/start.git` — the originating feature request.
- `01-prompt-arg-feature.md` — prerequisite. Prompt-valued aliases rely on the multi-segment `prompt` command (a trailing positional after the alias composes into the prompt body) and on `prompt` accepting file paths. Land 01 before the prompt-alias paths here.
- `02-task-arg-feature.md` — enriches trailing-argument pass-through into task instructions; not strictly required for a basic task alias.

## Requirements

1. Alias store. Aliases are stored only in global config, in a managed file `_aliases.cue` holding a single top-level `aliases:` field that maps an alias name to a list of token strings. The leading underscore excludes the file from every package build automatically (see Current State, VERIFIED). The store is never read through the directory loader; every alias-consuming surface compiles the single file in isolation via `ctx.CompileBytes` and looks up the `aliases` field. The user-facing category is `aliases` and the command is `alias`; only the on-disk filename carries the underscore. The file lives in the global config directory, where `start alias open` reads and writes it. Read and write go through the CUE Go API; write formats with CUE's formatter so output is canonical and token strings (including embedded quotes, commas, and colons) round-trip losslessly. Do not hand-roll CUE serialisation or copy `writeSettingsFile`'s string-builder approach.

2. Value model. An alias value is an ordered list of literal argument tokens — the `start` command minus the leading `start`. Tokens are stored verbatim: case, file paths, prompt text, and embedded quotes are preserved exactly. There are no category prefixes, no DSL, no cardinality rules, and no mutual-exclusion checks. Any subcommand and any flag is expressible because the value is literal argv, governed by normal cobra/pflag rules at run time. The stored token list is the expansion: execution splices it and display shell-quotes the same tokens; there is no separate builder. Alias names are case-insensitive and lowercased on write; an uppercase name resolves identically to its lowercase form. Target existence is not validated — a well-formed alias whose command refers to a missing module resolves exactly as if the user had typed that command, driving the normal search/selection/not-found behaviour.

3. Set command. `start alias set <name> <token>...` uses `DisableFlagParsing`, so everything after `<name>` is captured verbatim as the value; `set` has no own flags, and global flags do not apply to it. Help is available via `start help alias set` and via `start alias set --help` (a leading `-h`/`--help` is caught by the extended `checkHelpArg` — see Current State — before the args are treated as data). Set is an upsert: it creates or replaces one alias and preserves all others. All rejections write nothing and print a specific hint with a valid example:

   - The name must be a single non-empty token that does not start with `-` (so a stray flag is not taken as a name) and does not equal a registered top-level subcommand or subcommand alias (derive the forbidden set live from the command tree). A collision reports, for example, `config is a built-in command; choose another name, e.g. start alias set cfg ...`.
   - The value must be non-empty.
   - The value must not begin with the literal token `start`; report `drop the leading start; the value is the command without it, e.g. start alias set pc task review/pre-commit`.

   Before overwriting `_aliases.cue`, guard against clobbering content the tool cannot safely round-trip: if the existing file fails to parse, or parses but contains non-`aliases` top-level keys, refuse to write and direct the user to fix or edit it manually. Fail closed.

4. Top-level resolution. Before cobra command dispatch (in or alongside `Execute`), identify the first positional argument with persistent flags stripped, using cobra's flag-aware lookup (`ParseFlags` then `Find` / `Flags().Args()`). A flag value that equals an alias name (`start --role pc`, `start -r pc`) is not the positional and must not trigger expansion. When the first token is not a known subcommand or subcommand alias and matches an alias name (case-insensitive), replace that token in place with the alias's stored tokens and let the rewritten invocation run through cobra normally. All other arguments and flags keep their positions, so trailing arguments and flags are preserved:

   ```
   start pc                  → start task review/pre-commit
   start pc "fix the lint"   → start task review/pre-commit "fix the lint"
   start pc --model opus      → start task review/pre-commit --model opus
   start --debug pc          → start --debug task review/pre-commit
   start dev                 → start --role go-expert --context cwd/agents-md
   start foo                 → start prompt "this is the prompt"
   start foo "extra text"    → start prompt "this is the prompt" "extra text"
   start foo --role go-expert → start prompt "this is the prompt" --role go-expert
   ```

   Trailing arguments after the alias token flow through unchanged. With projects 01 and 02, a trailing literal-or-file argument becomes an additional prompt or task instruction segment: `start pc "extra" ./foo` resolves to `start task review/pre-commit "extra" ./foo`, whose instructions are `extra`, one blank line, then the contents of `./foo`. The alias layer does no segment work; it only splices argv.

   The rewrite adds no flag-conflict resolution; standard pflag last-wins precedence applies to the merged argv. When an alias sets a flag the user also passes, position decides: a user flag after the alias token wins, one before it is overridden (`start dev --role x` resolves the role to `x`; `start --role x dev` resolves it to the alias's role). When the first token matches no subcommand and no alias, behaviour is unchanged (cobra's `unknown command` error). Bare `start` with no positional token is unchanged. Resolution is single-pass: a spliced first token is never re-resolved as another alias. `start task pc` does not consult aliases; only the top-level `start pc` form does.

5. Lazy, resilient loading. Because `_aliases.cue` is excluded from every package build (Requirement 1), a malformed store never fails the main config load: `start task ...`, bare `start`, and every other subcommand load the merged config without parsing it, regardless of its contents. A global directory containing only `_aliases.cue` loads as an empty instance, not an error. The top-level resolver is lazy and match-scoped: it reads the store only when the first positional token is not a known subcommand, enumerates the alias names, and — only when the token matches a defined name — validates that single entry and surfaces a clear error if it is invalid. An absent or empty store, a token matching no name, or a store that cannot be parsed far enough to enumerate names all fall through unchanged to cobra's `unknown command`, so an ordinary typo never surfaces store corruption while an invalid alias still fails loudly the moment it is invoked.

6. Management command. A top-level command `alias` (with cobra alias `aliases`), global config only:

   ```
   start alias                       list all aliases
   start alias list                  list all aliases (cobra alias: ls)
   start alias set <name> <token>... create or update one alias (DisableFlagParsing)
   start alias get <name>            show one alias as its full expanded command
   start alias delete <name>...      delete one or more aliases (cobra alias: rm)
   start alias open                  open _aliases.cue in $EDITOR
   start alias export                print _aliases.cue to stdout
   start alias import [file]         merge aliases from stdin or a file (--replace to overwrite)
   ```

   - No-argument `start alias` dispatches the same listing as `start alias list`.
   - List and get render each alias as its name plus its full expanded `start ...` command — the stored tokens shell-quoted so the shown command is copy-pasteable — not a raw token dump. This is the same token sequence used at resolution.
   - Get on an unset name reports "not set".
   - Delete is variadic; an absent name reports "not set" rather than erroring.
   - No alias subcommand supports `--json`. `--quiet` is a global persistent flag and is not honoured by the alias subcommands; `set` and `delete` confirmations always print. (Because `set` disables flag parsing, `--quiet` would otherwise be captured as data anyway.)
   - An alias name is any non-empty single token that does not start with `-` and does not equal a registered top-level subcommand or subcommand alias; a colliding name is rejected on `set` (cobra would shadow it). Verb-first subcommands mean alias names are otherwise unrestricted — `start alias get open` shows an alias named `open`.

7. Import. `start alias import` reads a CUE document from stdin; `start alias import <file>` reads it from a path. The document has the same shape as `export` output: a top-level `aliases:` map of name to token list. Default behaviour is merge (upsert): each incoming alias is added or overwrites the same-named entry, and existing aliases not present in the import are left untouched, so `start alias export | start alias import` is a no-op. `--replace` instead replaces the entire store with the imported set. Import is atomic and fails closed: every incoming entry runs the full `set` validation (name shape, no `-` prefix, no subcommand collision, non-empty value, no leading `start` token), and if any entry is invalid, the document fails to parse, or it carries non-`aliases` top-level keys, the whole import is rejected and nothing is written. Both modes also honour the existing-store guard (Requirement 3): if the current `_aliases.cue` fails to parse or carries non-`aliases` top-level keys, the import — merge or `--replace` alike — is refused and nothing is written, so `--replace` is not an escape hatch for a hand-mangled store. Recovery from such a store is `start alias open` (manual edit) or deleting the file; the next write then starts clean. The write goes through the CUE API and formatter.

8. Embedded schema and validation. Embed a trivial structural `#Aliases` schema: `aliases` is a map of name to a list of strings. Its sole purpose is letting surfaces reject a hand-edited file whose types are wrong; there is no address-shape, cardinality, or mutual-exclusion logic. The same lightweight structural-plus-name validation is shared by `set` (write), `import` (each entry), the resolver (the matched entry), and `doctor`, so a hand-edited breach is caught on the dispatch path the same way doctor catches it rather than silently expanded.

9. Doctor. `start doctor` adds an alias-store section reporting the alias count and any problems: an unparseable file, wrong value types, an empty value, a name that collides with a subcommand, or non-`aliases` top-level keys. An absent store reports zero aliases, not an error.

10. Underscore-file isolation. Every surface that enumerates `.cue` files directly — outside CUE's package loader, which already ignores `_`-prefixed files — must skip files whose names begin with `_`, aligning these surfaces with CUE's own discovery rule and keeping the global `_aliases.cue` out of generic config tooling. The following surfaces need the filter:

    - `config export`: `printCueFiles` currently streams every `.cue` file; skip `_`-prefixed names so `_aliases.cue` is not exported.
    - Doctor Configuration section: `checkConfigDir` in `internal/doctor/checks.go` builds its `cueFiles` listing with a raw `os.ReadDir` and prints each as a PASS row. Without the filter a malformed `_aliases.cue` is shown as a valid file (its `LoadSingle` validation passes because the package loader ignores it) while the dedicated alias section flags it as broken — contradictory output. Exclude `_`-prefixed names from this listing.
    - Doctor schema validation: `config.CUEFilesInDir` (`internal/config/validation.go`), consumed by `internal/doctor/schema.go`, returns every `.cue` file; it must skip `_`-prefixed names so the managed store is not compiled and walked on the schema-validation path. (`internal/doctor/schema.go` is its only caller, so the filter is safe to apply in `CUEFilesInDir` itself.)
    - Broken-file diagnostics: `autosetup.go` and `install.go` glob `configDir/*.cue` and pass the matches to `internalcue.IdentifyBrokenFiles`, which compiles each file individually, so a malformed `_aliases.cue` would be listed as a broken config file. These globs run only when `LoadSingle(configDir)` has already failed on other config — `LoadSingle` ignores `_aliases.cue`, so the store alone never triggers them — but when they run they still report on the store, contradicting the single-authority guarantee. Skip `_`-prefixed names from these config-dir globs.

    The dedicated alias-store section (Requirement 9) is the single authority on alias-store health; no generic config or doctor surface reports on `_aliases.cue`. Beyond these enumerations, no config command is extended for aliases.

11. Tests. Cover:

    - Store read/write round-trip via the CUE API, including a token containing spaces, commas, colons, and embedded quotes preserved verbatim; name lowercasing with value case preserved.
    - The `set` command: capture-verbatim under `DisableFlagParsing` (a value containing flags like `--role` is stored intact), upsert preserving other aliases, and each rejection with its hint (empty value, leading `start` token, name starting with `-`, name colliding with a subcommand or its cobra alias).
    - `start alias set --help` and `start help alias set` both surface help without writing.
    - The resolver rewrite: bare alias, trailing args, trailing flags, flags before the token, a value-bearing persistent flag whose value equals an alias name (`start --role pc` / `start -r pc` must not rewrite), no-match passthrough to `unknown command`, subcommand-name non-collision, single-pass non-recursion, and the match-scoped failure mode (a token matching a malformed entry surfaces that entry's error while an unrelated unknown token against the same broken store still yields `unknown command`).
    - The `alias` command surface: list and get rendering the expanded shell-quoted command (not a raw dump), get/delete on present and absent names, delete variadic.
    - Import: merge upsert (export→import round-trip is a no-op), `--replace` overwriting the store, stdin and file sources, and atomic fail-closed rejection on an invalid entry, a parse failure, or non-`aliases` keys. Both modes also refuse — writing nothing — when the existing `_aliases.cue` is unparseable or carries non-`aliases` top-level keys, including the `--replace`-against-a-non-`aliases`-existing-store case.
    - The underscore-exclusion filter on every direct `.cue` enumeration: `config export` omits `_aliases.cue`; doctor's Configuration listing does not show it (including when it is malformed); doctor's schema-validation sweep does not compile it; and the `autosetup`/`install` broken-file diagnostics do not list it even when other config is broken and the store is malformed. The `doctor` alias check (count, absent store, malformed store).

    Follow existing test conventions (`setupStartTestConfig`, table-driven, real CUE via `t.TempDir()`).

## Constraints

- Go, cobra, and CUE as already used in the repository. No new heavyweight dependencies.
- Read the store by compiling the single `_aliases.cue` file in isolation (`ctx.CompileBytes(data, cue.Filename(path))`) and looking up the `aliases` field; never load it through the directory loader. Write by constructing a CUE value or syntax tree and formatting with CUE's formatter. Do not hand-roll CUE serialisation.
- The managed `_aliases.cue` holds only the `aliases` field. Before overwriting, refuse to write if the existing file fails to parse or contains non-`aliases` top-level keys, directing the user to fix or edit it manually. Fail closed; never silently discard unrecognised content.
- Aliases are global only. The `alias` command and the resolver never read or write local config; the `alias` command has no `--local` flag.
- Client-only. Do not add anything to the library or registry; the schema is embedded in the binary.
- Resolution after an alias rewrite reuses the existing resolver and command paths unchanged. The alias layer only rewrites argv; it does not reimplement search, selection, or installation.

## Implementation Plan

1. Alias store and schema. Add a config-layer component that owns the global `_aliases.cue` path, the trivial embedded structural `#Aliases` schema, isolated loading via `ctx.CompileBytes`, and writing via the CUE API and formatter with the non-alias-content guard. Normalise names to lowercase; store values as verbatim token lists. Storage shape is a map of name to a list of strings.

2. Top-level resolver. Add interception ahead of cobra command dispatch (in or alongside `Execute`). Determine the first positional token with persistent flags stripped using cobra's flag-aware lookup, decide whether it is a known subcommand, and when it is not, load the store and attempt a case-insensitive match. On a match against a valid entry, splice its tokens into argv in place and continue to cobra. The failure mode is match-scoped per Requirement 5: enumerate names, validate only the matched entry, surface its error if invalid, and otherwise fall through unchanged. Keep the load lazy and single-pass.

3. Management command. First extend the shared `checkHelpArg` in `root.go` to also recognise a leading `-h`/`--help` (not only the literal `help`), so flag-disabled subcommands can surface help; this is a no-op for the existing flag-parsing callers. Then add the top-level `alias` command (cobra alias `aliases`) with `list`, `set` (DisableFlagParsing, `checkHelpArg`), `get`, `delete`, `open`, `export`, and `import` (`--replace`) subcommands, no-argument invocation defaulting to list, and the dynamically-derived subcommand-collision and `-`-prefix name checks. Provide a display helper that shell-quotes the stored tokens into a copy-pasteable `start ...` command for list/get. Reuse the shared name/structural validation across set, import, resolver, and doctor.

4. Config export and doctor. Filter underscore-prefixed files from every direct `.cue` enumeration outside the package loader: `printCueFiles` (`config export`), the `checkConfigDir` file listing in `internal/doctor/checks.go`, `config.CUEFilesInDir` (feeding doctor's schema validation), and the `configDir/*.cue` globs in `autosetup.go` and `install.go` that feed `IdentifyBrokenFiles`. Add an alias-store validation section to `start doctor` reporting count and problems. Make no other config-command change.

5. Tests and documentation. Add the coverage in Requirement 11. Update `AGENTS.md` command listings and any user-facing help to document `start alias` and the `start <alias>` shortcut.

## Implementation Guidance

- The stored token list is the single source of truth. The argv rewrite (execution) and the `list`/`get` output (display) both work from it, so an alias always shows exactly what it runs; display only adds shell-quoting.
- The value is captured verbatim under `DisableFlagParsing`; there is no parsing of the value at set time beyond the name and value guards. It is never passed through a shell, so spaces, commas, colons, and embedded quotes in a token survive untouched on execution.
- Derive the forbidden-name set and the known-subcommand check from the live cobra command tree (`Commands()` and their `Aliases`) so new commands are covered automatically.
- For first-positional detection, leverage cobra's flag-aware lookup (`ParseFlags` then `Find` / `Flags().Args()`) rather than re-implementing flag parsing. Persistent flags can precede the alias token, and a flag value that equals an alias name must not be treated as the positional.
- Keep the resolver's failure mode narrow: only an invocation whose first token is a non-subcommand reaches the alias load, so its errors never affect ordinary subcommands.

## Acceptance Criteria

- `start alias set pc task review/pre-commit` then `start pc` runs the same command as `start task review/pre-commit`, and `start pc "msg"` passes `msg` through as task instructions.
- `start alias set dev --role go-expert --context cwd/agents-md` makes `start dev` expand to `start --role go-expert --context cwd/agents-md`, and `start alias set rev task review/pre-commit --role go-expert --model opus` expands to that full command.
- `start alias set foo prompt "this is the prompt"` then `start foo` runs the same command as `start prompt "this is the prompt"`, and `start foo --role go-expert` expands to `start prompt "this is the prompt" --role go-expert`.
- A value token containing commas, colons, spaces, and embedded quotes is stored verbatim, round-trips through `_aliases.cue`, and reaches the target argument unchanged on resolution; token case is preserved while the alias name is lowercased.
- `start alias get pc` and `start alias list` display aliases as their full expanded, shell-quoted `start ...` commands, not raw token dumps.
- `set` rejects, writing nothing and printing a hint: an empty value; a value beginning with `start`; a name beginning with `-`; a name equal to a subcommand or its cobra alias.
- `start alias set --help` and `start help alias set` print help without writing.
- `start alias set PC TASK REVIEW/PRE-COMMIT` stores the name lowercased and the tokens verbatim; resolving `start pc` runs the stored command.
- `start alias export | start alias import` is a no-op; `start alias import other.cue` merges its aliases into the store; `start alias import --replace other.cue` replaces the store with that file; an import containing an invalid entry, a parse error, or non-`aliases` keys writes nothing.
- `_aliases.cue` is written and read via the CUE API and contains only the `aliases` field; a file containing other top-level keys is not overwritten.
- Aliases are read and written only in global config; the `alias` command has no `--local` flag.
- A first token matching neither a subcommand nor an alias yields cobra's `unknown command` error; bare `start` is unchanged; `start task <name>` does not consult aliases.
- A malformed `_aliases.cue` does not break `start task ...`, bare `start`, or any other subcommand, and a global directory containing only `_aliases.cue` loads as empty. A present-but-invalid store surfaces a resolver error only when the typed first token matches the malformed alias's name; an unrelated unknown token still yields `unknown command`.
- `start config export` does not include `_aliases.cue`, nor any other `_`-prefixed file, in its output.
- `start doctor` does not list `_aliases.cue` in its Configuration section (even when the store is malformed) and does not compile it on the schema-validation path; the alias-store section is the only place the store's health is reported.
- `start alias open` opens the global `_aliases.cue`; `start alias export` prints it to stdout.
