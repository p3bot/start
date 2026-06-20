# Project: start uninstall Command

## Goal

Add a top-level `start uninstall` command that removes installed modules from start
configuration, as the inverse of `start install`. Removal already exists as `start
config remove` (aliases `rm`, `delete`); this project does not add a second, independent
removal path. It promotes removal to a top-level `uninstall` for symmetry with
`install`, and unifies both commands onto a single shared removal core so they behave
identically. `config remove` keeps its place in the config command set and is migrated
onto this core, gaining the AST-based, comment-preserving removal specified here.

## Scope

In scope:

- A new top-level command `uninstall`, in the modules command group, with aliases
  `remove` and `rm`, implemented as a thin front-end over a shared removal core.
- A single shared removal core that both `start uninstall` and `start config remove`
  call, so the two commands behave identically. `config remove` stays in the config
  command set and is migrated onto this core.
- Removal of installed modules across the four existing categories: agents, roles,
  contexts, tasks.
- Scope selection: default to global config; `--local` targets local config. The same
  scope model applies to `config remove`, tightening its search from merged to the
  selected scope.
- Multiple queries in one invocation.
- A confirmation prompt by default, bypassed with `--force`.
- Cross-category, installed-only resolution that reuses the engine's matching tier
  (`engine.go`): a bare term is a case-insensitive substring, a `category:name` qualifier
  is a literal prefix scoped to that category, and a whole-name exact match resolves
  ahead of both. Ambiguity presents the engine's selection menu on a TTY and errors on a
  non-TTY. The matcher runs over installed modules only — the registry is never consulted
  — so `config remove` and `uninstall` share one match rule with `install`/`get` without
  inheriting auto-install. A bare name still spans all four categories.

Out of scope:

- The skills category. Skills are a separate project and require materialised-bundle
  removal rather than config-entry removal. Factor the removal logic so a
  category-specific removal path can be added later without reworking the command.
- Cascading dependency removal. Uninstalling a task must not remove a role it depends
  on; other modules may use it.
- Dangling-reference repair beyond a single warning when the removed module is the
  configured default agent.
- Migrating `config add`, `config edit`, and `config order` onto the AST writers. They
  share the four `writeXFile` writers with the removal path; only the removal path is
  migrated here, so those commands keep their string-rebuild writers and their
  comment-loss behaviour. Tracked as a follow-up project
  (`02-migrate-config-writers-onto-ast.md`).
- Any change to `start install` behaviour.

## Current State

The command reuses the engine's matching rule (`engine.go`) for resolution and the
existing config-removal plumbing (confirmation, dispatch, per-category removal) for the
rest; the install path supplies the AST writer pattern its removal function inverts. All
three are described below.

- `internal/cli/install.go` defines `addInstallCommand` and `runInstall`. The command
  lives in the `modules` group and accepts multiple queries. Install always writes to
  global config.
- `internal/modules/install.go` performs the config write. `writeModuleToConfig`,
  `findCategoryField`, `findModuleField`, and `UpdateModuleInConfig` parse the target
  `*.cue` file with `cuelang.org/go/cue/parser` (comments preserved), mutate the AST,
  and reformat with `cuelang.org/go/cue/format` using `format.Simplify()`. Module
  content is inlined into a struct under `<category>.<name>`; for tasks and contexts the
  `file` field stores a `@module/...` reference, and the referenced markdown is never
  copied into the config directory. Removing a module therefore means deleting one field
  from the category struct. No sidecar files or cache entries need cleanup.
- `internal/cue/keys.go` maps each category to its config file via `ConfigFiles`
  (agents.cue, roles.cue, contexts.cue, tasks.cue, settings.cue).
- `internal/cli/config_remove.go` already implements `start config remove` (aliases
  `rm`, `delete`): `runConfigRemove` resolves a query via `searchAllConfigCategories`,
  shows a selection menu, prompts via `confirmConfigRemoval` with the scope named, guards
  non-interactive use behind `--force`, and dispatches per category through
  `removeConfigItem` to `removeAgent`/`removeRole`/`removeContext`/`removeTask`. The
  confirmation, the `removeConfigItem` dispatch, and the per-category remove functions are
  the plumbing to reuse. The resolution step is not: `searchAllConfigCategories` →
  `resolveAllMatchingNames` → `scoreAndSortNames` matches names with a `(?i)` regex —
  substring only, with no exact-whole-name tier, no prefix mode, and no `category:name`
  qualifier (a leading `category:` is never stripped, so `roles:golang/assistant` matches
  nothing). The removal commands move off this search onto the engine's matching tier (see
  the `engine.go` entry); `searchAllConfigCategories` stays in place for `config get` and
  `config edit`, which keep using it. Two further weaknesses are fixed during unification:
  the removal path's writers (`writeAgentsFile`, `writeRolesFile`, `writeContextsFile`,
  `writeTasksFile` in `internal/cli/config_types.go`) rewrite each file from a struct via
  string building, losing the install-managed comment header and leaving an emptied
  category as `agents: {}` — these writers are shared with `config add`, `config edit`, and
  `config order`, so this project migrates only the removal path off them and leaves the
  writers in place for those commands (see Out of scope); and its search runs at merged
  scope (`config.ScopeFromLocal`) while removal targets a single file. Note
  `configMatch.Category` uses singular labels (`agent`, `role`, ...) whereas `ConfigFiles`
  and the CUE keys are plural.
- `internal/cli/engine.go` holds the matching rule that uninstall reuses, cleanly split
  from the registry. `nameMatches` (literal, case-insensitive exact/substring/prefix),
  `interpretSurface` (parses the `category:` prefix and selects `modePrefix` for a
  qualified term, `modeSubstring` for a bare one, validating the category), and
  `collectInstalled` (runs `nameMatches` over installed config) are all registry-free.
  Only the outer tiers are registry-coupled: `resolveExact` and `resolveFallback` call
  `ensureIndex` and merge `collectRegistry` results, and `use`→`installIfRegistry`
  performs auto-install. This project factors the exact→fallback reduction (exact-whole-
  name tier first, then the substring/prefix fallback, with `selectMatch`'s TTY menu and
  non-TTY ambiguity error) into a source-agnostic matcher: install/get keep supplying
  installed + registry candidates and the auto-installing finalize step, while uninstall
  supplies installed candidates only, never constructs the index, and never installs.
  Running the existing live `resolver` in an offline mode is not sufficient — `resolveExact`
  still calls `ensureIndex`, and requirement 2 forbids uninstall from even attempting a
  registry fetch — so the reduction is extracted rather than driven through the resolver
  object.
- `internal/config/paths.go` resolves config directories: global at `~/.config/start/`
  (or `$XDG_CONFIG_HOME/start/`), local at `./.start/`. Scope handling and the
  `scopeFromFlags`/`loadConfig` helpers used by other commands live alongside the CLI.
- `internal/cli/describe.go` defines `describeCategories`, the canonical mapping of
  category to display type used by cross-category surfaces.
- `internal/cli/library.go` has `collectInstalledNames`, an example of enumerating
  installed modules.
- `internal/cli/root.go` registers commands. A new `addUninstallCommand` is wired here.

## Requirements

1. A `start uninstall [query]...` command registered in the modules group with aliases
   `remove` and `rm`, and help text describing it as the inverse of install. With no
   positional query, the command mirrors install: on a TTY it prompts for a query via
   the shared `promptSearchQuery` flow and resolves the entered query; on a
   non-interactive stream it errors with "query required in non-interactive mode".
   Uninstall imposes no minimum query length at all — a single-character query such as
   `a` is accepted. The three-character floor exists only to bound install's
   registry-wide search; uninstall resolves against the installed set, so any non-empty
   query (including a one- or two-character exact name) must remain removable. The
   extracted matcher's exact-whole-name tier is already floor-exempt, and uninstall drops
   the substring/prefix fallback floor entirely (passes a zero floor), so a short
   substring query is honoured too; install keeps the three-character fallback floor for
   its registry-wide search. To support the prompt path, `promptSearchQuery` takes a
   minimum-length parameter: install passes `3`, uninstall passes `1` (any non-empty
   input; empty still cancels).
2. Resolution considers only installed modules in the selected scope. It must not query
   the registry and must not auto-install. A query matching nothing installed returns a
   not-found error.
3. Resolution is cross-category for a bare query and scoped to one category when the
   query is `category:name`, reusing the engine's matching rule: a bare term is a
   case-insensitive substring over names, a `category:name` term a literal prefix within
   that category, and a whole-name exact match wins ahead of both fallback tiers. The
   `category:name` qualifier is parsed by `interpretSurface`, which already validates the
   category and selects prefix matching; uninstall and `config remove` both route through
   the extracted matcher, so both gain the qualifier and the exact/prefix/substring tiers.
   An unknown category label is a usage error. An ambiguous query presents the engine's
   `selectMatch` menu on a TTY and returns an ambiguity error on a non-TTY. The non-TTY
   ambiguity case is always an error, including with `--force`; the current `config
   remove` behaviour of removing all matches on `--force` is not carried forward. The
   query-path menu reduces to a single selection (matching install); `config remove`'s
   no-argument category-then-name picker keeps its existing multi-select.
4. Default scope is global. The `--local` flag switches the target to local config.
   There is no `--global` flag. Resolution and removal both operate within the selected
   scope only; a module present only in the other scope is reported as not found.
5. Removal deletes the module's field from the matching category struct in the scope's
   `*.cue` file, using the official CUE AST, parser, and format packages. Comments and
   unrelated entries are preserved. If the removal empties a category struct, the empty
   category field is removed as well. The file remains valid CUE.
6. Multiple queries are processed in one invocation. Each is resolved and removed
   independently; a failure on one query is reported without aborting the others.
7. A confirmation prompt is shown before removal, listing what will be removed and from
   which scope. `--force` bypasses the prompt. On a non-interactive stream without
   `--force`, the command errors without modifying any config rather than prompting.
8. Removing an agents-category module whose name equals the configured `default_agent`
   emits a warning. Read `default_agent` from the selected scope's settings (global by
   default, local under `--local`) — consistent with the scope-bound removal — not from
   merged config. A role, context, or task that shares the name does not warn, since
   `default_agent` can only reference an agent. The removal still proceeds; the dangling
   setting is not auto-repaired.
9. The shared module cache is never modified.
10. `start uninstall` and `start config remove` are both front-ends over one shared
    removal core and produce identical results for the same query and scope. `config
    remove` is migrated onto the core: its resolution becomes scope-bound (default global,
    `--local` for local) instead of merged, its writers are replaced by the
    comment-preserving AST removal in requirement 5, and it gains `category:name`
    qualifier support from the shared matcher per requirement 3. `config remove` keeps its
    place and aliases in the config command set, and retains its existing no-argument
    interactive path (category picker, then name picker) rather than adopting the
    install-style query prompt — that path is brought under the same scope-binding so its
    no-arg and query behaviours act on the same scope. The two commands stay identical for
    any given query and scope; they differ only in the no-arg affordance, each matching
    its own lineage (uninstall mirrors install, config remove keeps its config-management
    picker).

## Constraints

- Go 1.25, cobra, cuelang.org/go, matching the existing module.
- All CUE file reads, mutations, and writes use the official `cuelang.org/go` packages
  (`cue/parser`, `cue/ast`, `cue/format`). Do not edit config files by string
  manipulation.
- Reuse the engine's matching rule from `engine.go` (`nameMatches`, `interpretSurface`,
  and the extracted exact→substring/prefix reduction with `selectMatch`) for resolution,
  and the existing config-removal plumbing (`confirmConfigRemoval` and the
  `removeConfigItem` dispatch) for confirmation and per-category removal. Do not resolve
  through `searchAllConfigCategories`/`resolveAllMatchingNames` — that path is substring-
  only with no exact or prefix tier — and do not drive the live `resolver` object, which
  consults the index. Resolution must never query the registry: uninstall supplies the
  matcher installed-only candidates and no index, and never auto-installs.
- Reuse the existing config-path helpers (`config.ResolvePaths`, `Paths.Dir`); do not
  reimplement path discovery. For scope selection, map `local → ScopeLocal`,
  `!local → ScopeGlobal` directly — `ScopeFromLocal` is unsuitable here because it
  defaults to merged.
- Preserve config file formatting conventions produced by install
  (`format.Simplify()`).

## Implementation Plan

1. Add a removal function in `internal/modules` that is the inverse of
   `writeModuleToConfig`: parse the scope's category file with `cue/parser` (comments
   preserved), locate the category and module fields with the existing
   `findCategoryField`/`findModuleField`, delete the module field, drop the category
   field if it is now empty, reformat with `format.Simplify()`, and write. Return a clear
   not-found error when the field is absent.
2. Route `removeConfigItem` through step 1 so the removal path no longer calls the four
   string-rewrite writers (`writeAgentsFile` and siblings). Do not delete those writers:
   they remain in `config_types.go` for `config add`, `config edit`, and `config order`,
   which still depend on them and are out of scope here. Keep `removeConfigItem` as the
   per-category dispatch seam for the future skills path, mapping its singular category
   labels to the plural CUE keys and config filenames. `config remove` inherits the
   comment-preserving, empty-category-pruning behaviour for free.
3. Extract the engine's matching tier into a source-agnostic matcher. Factor the
   exact→fallback reduction out of `resolveExact`/`resolveFallback` so it runs over a
   supplied candidate set rather than the resolver's installed-plus-index sources: an
   exact-whole-name tier first (floor-exempt), then the substring/prefix fallback selected
   by `interpretSurface`, reducing to one decision via `selectMatch` (TTY menu, non-TTY
   ambiguity error). install/get keep supplying installed + registry candidates and the
   auto-installing finalize; uninstall and `config remove` supply installed candidates only,
   from the selected scope, and never construct the index or auto-install. The fallback
   floor is a parameter: install passes `3`, the removal commands pass `0` (see
   requirement 1). Leave `searchAllConfigCategories`/`resolveAllMatchingNames` untouched —
   `config get` and `config edit` keep using them.
4. Factor the shared removal core that both `runConfigRemove` and `runUninstall` call. It
   selects the scope (`local → ScopeLocal`, `!local → ScopeGlobal`; do not use
   `ScopeFromLocal`, which returns `ScopeMerged` for the default case and `scopeFromFlags`
   likewise defaults to merged absent a `--global` flag this command lacks — introduce a
   small `removalScope(local)` helper if it clarifies the call sites), loads that scope's
   config, resolves each query through the step-3 matcher against installed modules in that
   scope (cross-category bare, `category:name` prefix, exact tier, single-select menu,
   non-TTY ambiguity error), gathers the set to remove, shows the confirmation naming the
   modules and scope unless `--force` (erroring on a non-interactive stream without
   `--force`), and removes each via step 1. A module present only in the other scope is
   reported as not found. Process multiple queries independently; report per-query success
   and failure without aborting the rest. `runConfigRemove`'s no-argument interactive path
   (`runConfigRemoveInteractive`) stays specific to `config remove` and is not promoted
   into the shared core, but its `loadNamesForCategory` call must move from
   `config.ScopeFromLocal` (merged) to the same scope-bound selection so its no-arg and
   query behaviours act on one scope.
5. Add `addUninstallCommand` in a new `internal/cli/uninstall.go` and register it in
   `root.go` in the modules group. Define the `--force` flag (`--local` is already a
   persistent flag on root) and the `remove`/`rm` aliases, accept multiple positional
   queries, and route `runUninstall` through the shared core — a front-end, not a new
   pipeline. With no positional query, prompt for one via `promptSearchQuery` on a TTY
   and resolve it, erroring on a non-interactive stream. Parameterise `promptSearchQuery`
   with a minimum-length argument and update install's call site to pass `3`; uninstall
   passes `1`, so its prompt accepts any non-empty query (no three-character floor) while
   install's registry search keeps the floor. The positional-query path applies no length
   floor either.
6. Emit the default-agent warning when an agents-category module named by
   `settings.default_agent` is removed; proceed with removal and do not auto-repair.
7. Extend the existing config-remove tests and add uninstall tests covering: single
   removal in global and in local, multi-query, empty-category cleanup, comment and
   sibling preservation under the new AST writer, not-found, ambiguity on TTY and
   non-TTY, the confirmation prompt and `--force`, the non-interactive-without-force
   error, the default-agent warning, and parity of results between `start uninstall`,
   `start remove`, `start rm`, and `start config remove`.

## Implementation Guidance

- Keep the per-category removal behind the existing `removeConfigItem` dispatch point,
  even though all four current categories now share the same AST config-entry removal.
  The skills project will register a different removal path (bundle deletion via a
  manifest) at this seam, so the command must not assume config-entry removal is
  universal.
- The confirmation prompt should make the scope explicit, since the same module name can
  exist in both global and local config and the command only acts on one.
- Model the help text and flag descriptions on the install command for consistency.

## Acceptance Criteria

- `start uninstall <name>` removes the module's entry from the correct category file in
  global config, and the module no longer appears in `start list`.
- `start uninstall <name> --local` removes from `./.start/` and leaves global config
  untouched.
- `start remove <name>` and `start rm <name>` behave identically to `uninstall`.
- Removing the last module in a category leaves the category struct absent from the file,
  and the file remains valid CUE with unrelated entries and comments intact.
- `start uninstall a b c` removes each resolved module and reports the outcome of each.
- On a TTY without `--force`, a confirmation prompt naming the modules and scope appears;
  declining makes no change. With `--force`, no prompt appears.
- On a non-interactive stream without `--force`, the command exits with an error and
  modifies no config.
- An ambiguous bare query shows the selection menu on a TTY and returns an ambiguity
  error on a non-TTY; a query matching nothing installed returns a not-found error.
- Uninstalling the module named by `default_agent` emits a warning and still removes it.
- `start uninstall <name>` and `start config remove <name>` produce identical results for
  the same query and scope, both preserving comments and unrelated entries and both
  removing an emptied category struct rather than leaving `agents: {}`.
- `start uninstall` with no query prompts for one on a TTY (install-style) and resolves
  the entered query; on a non-interactive stream it errors with "query required in
  non-interactive mode" and modifies no config.
- A module whose installed name is one or two characters can be uninstalled by exact
  name, both as a positional query and via the no-arg prompt; uninstall imposes no
  minimum query length, while install retains its three-character floor.
- `start uninstall roles:golang/assistant` resolves by prefix only within the roles
  category, a bare query matches by substring across all four, and a whole-name exact
  match resolves ahead of both; `start config remove` accepts the same `category:name`
  form.
- `start config remove` with no argument still presents the category-then-name picker,
  now acting on the selected scope only.
- `start config remove` defaults to global scope: a module present only in local config
  is reported as not found without `--local`, matching `uninstall`.
