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
  presents a selection menu on ambiguity, reusing the existing config-removal search.

Out of scope:

- The skills category. Skills are a separate project and require materialised-bundle
  removal rather than config-entry removal. Factor the removal logic so a
  category-specific removal path can be added later without reworking the command.
- Cascading dependency removal. Uninstalling a task must not remove a role it depends
  on; other modules may use it.
- Dangling-reference repair beyond a single warning when the removed module is the
  configured default agent.
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
  build on. Two weaknesses must be fixed during unification: its writers
  (`writeAgentsFile`, `writeRolesFile`, `writeContextsFile`, `writeTasksFile` in
  `internal/cli/config_types.go`) rewrite each file from a struct via string building,
  losing the install-managed comment header and leaving an emptied category as
  `agents: {}`; and its search runs at merged scope (`config.ScopeFromLocal`) while
  removal targets a single file. Note `configMatch.Category` uses singular labels
  (`agent`, `role`, ...) whereas `ConfigFiles` and the CUE keys are plural.
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
   `remove` and `rm`, and help text describing it as the inverse of install.
2. Resolution considers only installed modules in the selected scope. It must not query
   the registry and must not auto-install. A query matching nothing installed returns a
   not-found error.
3. Resolution is cross-category for a bare query and scoped to one category when the
   query is `category:name`. An ambiguous bare query presents the shared removal core's
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
    `--local` for local) instead of merged, and its writers are replaced by the
    comment-preserving AST removal in requirement 5. `config remove` keeps its place and
    aliases in the config command set.

## Constraints

- Go 1.25, cobra, cuelang.org/go, matching the existing module.
- All CUE file reads, mutations, and writes use the official `cuelang.org/go` packages
  (`cue/parser`, `cue/ast`, `cue/format`). Do not edit config files by string
  manipulation.
- Reuse the existing config-removal core (`searchAllConfigCategories`, the selection
  menu, `confirmConfigRemoval`, and the `removeConfigItem` dispatch) rather than the
  registry-aware resolver engine or any new parallel resolution path. Removal must never
  query the registry, so the resolver engine in `engine.go` is not used here.
- Reuse the existing scope and config-path helpers; do not reimplement path discovery.
- Preserve config file formatting conventions produced by install
  (`format.Simplify()`).

## Implementation Plan

1. Add a removal function in `internal/modules` that is the inverse of
   `writeModuleToConfig`: parse the scope's category file with `cue/parser` (comments
   preserved), locate the category and module fields with the existing
   `findCategoryField`/`findModuleField`, delete the module field, drop the category
   field if it is now empty, reformat with `format.Simplify()`, and write. Return a clear
   not-found error when the field is absent.
2. Route `removeConfigItem` through step 1, replacing the four string-rewrite writers
   (`writeAgentsFile` and siblings). Keep `removeConfigItem` as the per-category dispatch
   seam for the future skills path, mapping its singular category labels to the plural
   CUE keys and config filenames. `config remove` inherits the comment-preserving,
   empty-category-pruning behaviour for free.
3. Tighten the config-removal search to be scope-bound: search and removal both operate
   in the selected scope only — default global, `--local` for local — replacing the
   current merged-scope search. A module present only in the other scope is reported as
   not found. Make non-TTY ambiguity an error in all cases, dropping the current
   `--force`-removes-all-matches behaviour.
4. Factor the shared removal core that both `runConfigRemove` and `runUninstall` call. It
   selects the scope, resolves each query against installed modules in that scope
   (cross-category, `category:name` qualifier, menu and ambiguity behaviours), gathers
   the set to remove, shows the confirmation naming the modules and scope unless
   `--force` (erroring on a non-interactive stream without `--force`), and removes each
   via step 1. Process multiple queries independently; report per-query success and
   failure without aborting the rest.
5. Add `addUninstallCommand` in a new `internal/cli/uninstall.go` and register it in
   `root.go` in the modules group. Define the `--local` and `--force` flags and the
   `remove`/`rm` aliases, accept multiple positional queries, and route `runUninstall`
   through the shared core — a front-end, not a new pipeline.
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
- `start config remove` defaults to global scope: a module present only in local config
  is reported as not found without `--local`, matching `uninstall`.
