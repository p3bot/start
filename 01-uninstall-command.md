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
- Cross-category, installed-only resolution that accepts a `category:name` qualifier and
  presents a selection menu on ambiguity, building on the existing config-removal search.
  The qualifier is parsed in the shared search (`searchAllConfigCategories`), so `config
  remove` gains `category:name` support as an additive behaviour alongside `uninstall`; a
  bare name still searches all four categories unchanged.

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

The command is built on the existing config-removal core; the install path supplies the
AST writer pattern its removal function inverts. Both are described below.

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
  `rm`, `delete`): `runConfigRemove` searches all four categories via
  `searchAllConfigCategories`, shows a selection menu, prompts via `confirmConfigRemoval`
  with the scope named, guards non-interactive use behind `--force`, and dispatches per
  category through `removeConfigItem` to
  `removeAgent`/`removeRole`/`removeContext`/`removeTask`. This is the removal core to
  build on. Three weaknesses must be fixed during unification: the removal path's writers
  (`writeAgentsFile`, `writeRolesFile`, `writeContextsFile`, `writeTasksFile` in
  `internal/cli/config_types.go`) rewrite each file from a struct via string building,
  losing the install-managed comment header and leaving an emptied category as
  `agents: {}` — these writers are shared with `config add`, `config edit`, and `config
  order`, so this project migrates only the removal path off them and leaves the writers
  in place for those commands (see Out of scope); its search runs at merged scope
  (`config.ScopeFromLocal`) while removal
  targets a single file; and its search does not parse a `category:name` qualifier —
  `searchAllConfigCategories` passes the raw query to `resolveAllMatchingNames`, which
  hands it to `modules.ParseSearchPatterns`, and neither strips a leading `category:`,
  so `roles:golang/assistant` is treated as one literal term and matches nothing.
  Qualifier parsing must be added to the shared search; the registry-aware resolver in
  `engine.go` already parses `category:name` but is off-limits here. Note
  `configMatch.Category` uses singular labels (`agent`, `role`, ...) whereas
  `ConfigFiles` and the CUE keys are plural.
- `internal/cli/engine.go` holds the resolver: `interpretSurface`, `resolve`,
  `selectMatch`, the `ModuleMatch`/`resolveScope` types, and the
  `ModuleSourceInstalled` versus registry distinction. The resolver supports
  cross-category lookup, `category:name` qualification, substring and prefix matching, a
  TTY selection menu, and a non-TTY ambiguity error. Install and get use this engine.
  Uninstall does not: the resolver is registry-aware (its cross-category exact tier
  always consults the index), so installed-only removal builds on the config-removal
  search instead.
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
   query (including a one- or two-character exact name) must remain removable. To support
   both, `promptSearchQuery` takes a minimum-length parameter: install passes `3`,
   uninstall passes `1` (any non-empty input; empty still cancels).
2. Resolution considers only installed modules in the selected scope. It must not query
   the registry and must not auto-install. A query matching nothing installed returns a
   not-found error.
3. Resolution is cross-category for a bare query and scoped to one category when the
   query is `category:name`. The `category:name` qualifier is parsed in the shared search
   (`searchAllConfigCategories`): split on the first `:`, normalise the category label to
   its canonical form (reuse `normalizeCategoryArg`), restrict the search to that one
   category, and error if the label is not one of the four categories. This is new
   behaviour added to the search, not capability the current search provides; `config
   remove` inherits it. An ambiguous bare query presents the shared removal core's
   selection menu on a TTY and returns an ambiguity error on a non-TTY. The non-TTY
   ambiguity case is always an error, including with `--force`; the current `config
   remove` behaviour of removing all matches on `--force` is not carried forward.
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
   in settings emits a warning. A role, context, or task that shares the name does not
   warn, since `default_agent` can only reference an agent. The removal still proceeds;
   the dangling setting is not auto-repaired.
9. The shared module cache is never modified.
10. `start uninstall` and `start config remove` are both front-ends over one shared
    removal core and produce identical results for the same query and scope. `config
    remove` is migrated onto the core: its search becomes scope-bound (default global,
    `--local` for local) instead of merged, its writers are replaced by the
    comment-preserving AST removal in requirement 5, and it gains `category:name`
    qualifier support from the shared search per requirement 3. `config remove` keeps its
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
- Reuse the existing config-removal core (`searchAllConfigCategories`, the selection
  menu, `confirmConfigRemoval`, and the `removeConfigItem` dispatch) rather than the
  registry-aware resolver engine or any new parallel resolution path. Removal must never
  query the registry, so the resolver engine in `engine.go` is not used here.
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
3. Tighten the config-removal search to be scope-bound: search and removal both operate
   in the selected scope only — default global, `--local` for local — replacing the
   current merged-scope search. Map the scope explicitly as `local → ScopeLocal`,
   `!local → ScopeGlobal`. Do not use `ScopeFromLocal` here: it returns `ScopeMerged`
   for the default case, which would re-create the very search-merged-while-removal-global
   mismatch this step exists to fix (and `scopeFromFlags` also defaults to merged absent a
   `--global` flag, which this command does not have). Introduce a small helper (for
   example `removalScope(local)`) for the mapping if it clarifies the call sites. A module
   present only in the other scope is reported as not found. Make non-TTY ambiguity an error in all cases, dropping the current
   `--force`-removes-all-matches behaviour. In the same search change, add `category:name`
   qualifier parsing to `searchAllConfigCategories` (split on the first `:`, normalise via
   `normalizeCategoryArg`, restrict to that category, error on an unknown label); both
   `config remove` and `uninstall` inherit it.
4. Factor the shared removal core that both `runConfigRemove` and `runUninstall` call. It
   selects the scope, resolves each query against installed modules in that scope
   (cross-category, `category:name` qualifier, menu and ambiguity behaviours), gathers
   the set to remove, shows the confirmation naming the modules and scope unless
   `--force` (erroring on a non-interactive stream without `--force`), and removes each
   via step 1. Process multiple queries independently; report per-query success and
   failure without aborting the rest. `runConfigRemove`'s no-argument interactive path
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
- `start uninstall roles:golang/assistant` resolves only within the roles category, and
  `start config remove` accepts the same `category:name` form.
- `start config remove` with no argument still presents the category-then-name picker,
  now acting on the selected scope only.
- `start config remove` defaults to global scope: a module present only in local config
  is reported as not found without `--local`, matching `uninstall`.
