# Project: start uninstall Command

## Goal

Add a top-level `start uninstall` command that removes installed modules from start
configuration. It is the inverse of `start install` and closes an existing asymmetry:
start can install modules but offers no first-class way to remove them.

## Scope

In scope:

- A new top-level command `uninstall`, in the modules command group, with aliases
  `remove` and `rm`.
- Removal of installed modules across the four existing categories: agents, roles,
  contexts, tasks.
- Scope selection: default to global config; `--local` targets local config.
- Multiple queries in one invocation.
- A confirmation prompt by default, bypassed with `--force`.
- Installed-only resolution that is cross-category, accepts a `category:name`
  qualifier, and presents a selection menu on ambiguity.

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

The install path establishes every pattern the inverse must mirror.

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
- `internal/cli/engine.go` holds the resolver: `interpretSurface`, `resolve`,
  `selectMatch`, the `ModuleMatch`/`resolveScope` types, and the
  `ModuleSourceInstalled` versus registry distinction. The resolver supports
  cross-category lookup, `category:name` qualification, substring and prefix matching, a
  TTY selection menu, and a non-TTY ambiguity error. Install and get use this engine.
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
   query is `category:name`. An ambiguous bare query presents the existing selection
   menu on a TTY and returns the existing ambiguity error on a non-TTY.
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
8. Removing a module whose name equals the configured `default_agent` in settings emits
   a warning. The removal still proceeds; the dangling setting is not auto-repaired.
9. The shared module cache is never modified.

## Constraints

- Go 1.25, cobra, cuelang.org/go, matching the existing module.
- All CUE file reads, mutations, and writes use the official `cuelang.org/go` packages
  (`cue/parser`, `cue/ast`, `cue/format`). Do not edit config files by string
  manipulation.
- Reuse the existing resolver engine rather than introducing a parallel resolution path.
  Restrict it to the installed source; do not duplicate matching, menu, or qualifier
  logic.
- Reuse the existing scope and config-path helpers; do not reimplement path discovery.
- Preserve config file formatting conventions produced by install
  (`format.Simplify()`).

## Implementation Plan

1. Add a removal function in `internal/modules` that is the inverse of
   `writeModuleToConfig`: parse the scope's category file, locate the category and module
   fields with the existing finders, delete the module field, drop the category field if
   it is now empty, reformat, and write. Return a clear not-found error when the field is
   absent.
2. Add an installed-only, scope-bound resolution entry point built on the engine in
   `internal/cli/engine.go`. It resolves a query against installed modules in the chosen
   scope, returning the matched category and name, and surfaces the standard menu and
   ambiguity behaviours. Do not add a registry or auto-install tier.
3. Add `addUninstallCommand` in a new `internal/cli/uninstall.go` and register it in
   `root.go`. Define the `--local` and `--force` flags and the `remove`/`rm` aliases.
   Accept multiple positional queries.
4. Implement `runUninstall`: select the scope from `--local`, load the scope's config,
   resolve each query, gather the set of modules to remove, show the confirmation prompt
   unless `--force` (erroring on a non-interactive stream without `--force`), then remove
   each resolved module from its category file via step 1.
5. Emit the default-agent warning when applicable. Report per-query successes and
   failures so a multi-query run communicates partial outcomes.
6. Add tests covering: single removal in global and in local, multi-query, empty-category
   cleanup, comment and sibling preservation, not-found, ambiguity on TTY and non-TTY,
   the confirmation prompt and `--force`, the non-interactive-without-force error, and the
   default-agent warning.

## Implementation Guidance

- Keep the per-category removal behind a small dispatch point keyed by category, even
  though all four current categories share the same config-entry removal. The skills
  project will register a different removal path (bundle deletion via a manifest) at this
  seam, so the command should not assume config-entry removal is universal.
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
