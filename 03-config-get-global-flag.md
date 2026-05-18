# Add --global flag to config get

## Goal

`start config get` lets users inspect a module's stored configuration but
has no way to ask for the global definition when a local definition
shadows it. Add `--global` to `config get` so users can see what the
global config holds for a name even when local overrides it.

## Scope

In scope:

- Register `--global` on `config get` using the same shape as `describe`
  and `get`: `cmd.Flags().BoolVar(&flags.Global, "global", false, ...)`
  with `cmd.MarkFlagsMutuallyExclusive("local", "global")`.
- Thread `flags *Flags` through `addConfigCommand` and
  `addConfigGetCommand` so the bind site has access to the struct.
- Derive scope in `runConfigGet` via `scopeFromFlags`.
- Add a global-only read path for the four `config get` printers
  (`printAgentGet`, `printRoleGet`, `printContextGet`, `printTaskGet`)
  and the interactive flow (`runConfigGetInteractive`,
  `loadNamesForCategory`) so they can return the global definition of a
  name.
- Add tests covering the four scope outcomes against a fixture where the
  same name has different field values in global and local config.

Out of scope:

- Adding `--global` to any command other than `config get`.
- Refactoring helpers outside the `config get` call chain. Specifically:
  `config_list.go`'s `listAgents`/`listRoles`/`listContexts`/`listTasks`/
  `collectConfigListItems`, `config_settings.go`'s
  `loadSettingsForScope`, and `internal/config/settings.go`'s
  `ResolveAllSettings` are not on the `config get` call chain and must
  not be touched.
- Changes to merge semantics for any other command.
- Promoting `--global` to a root persistent flag.
- Any change under `library/` or `homebrew-tap/`.

## Prerequisites

This project depends on the flag-registration unification project
(`02-unify-global-flag-registration.md`) being complete. That project
provides `Flags.Global`, the `scopeFromFlags` helper, and the binding
pattern this project mirrors. Do not start this project until that one
has landed.

## Current State

`config get` is implemented at `internal/cli/config_get.go`. The relevant
chain:

```
addConfigGetCommand                       config_get.go:13
  runConfigGet                            config_get.go:37
    searchAllConfigCategories             config_helpers.go:863
      loadAgentsForScope / RolesForScope  config_types.go:79, 264, 394, 523
       /ContextsForScope / TasksForScope
        loadForScope[T]                   config_helpers.go:25
    buildConfigListItem                   config_list.go:36
      loadXxxForScope (same set)
    printConfigGet                        config_get.go:132
      printAgentGet / printRoleGet /      config_get.go:150, 185, 217, 249
       printContextGet / printTaskGet
        loadXxxForScope (same set)
    runConfigGetInteractive               config_get.go:105
      loadNamesForCategory                config_interactive.go:15
        loadXxxForScope (same set)
```

Every site in the chain is keyed on `local bool`. `loadForScope[T]` at
`config_helpers.go:25-84` has two branches:

- `localOnly=true`: loads from `paths.Local` only, tags `Source` as
  `"local"`.
- `localOnly=false`: loads global first (tags `"global"`), then local
  overlay (tags `"local"`, replacing on collision).

There is no global-only branch. Adding one is what this project
delivers; how to add it is the implementer's call.

`config.Scope` already exists with three values (`ScopeMerged`,
`ScopeGlobal`, `ScopeLocal`) at `internal/config/paths.go:9`.
`Paths.ForScope` already routes correctly for all three. `describe` and
`get` already use `loadConfig(scope)` at `describe.go:443` and pass
`config.ScopeGlobal` end-to-end through their typed lookups.

`addConfigCommand` at `internal/cli/config.go:14` adds subcommand
factories one at a time without passing `flags`. Only
`addConfigGetCommand` needs the struct; the other subcommand factories
in this file are unaffected.

`runConfigGet` at `internal/cli/config_get.go:37` reads
`local := getFlags(cmd).Local` at line 44 and passes the bool through
every downstream call. The `--json` branch at line 73 calls
`buildConfigListItem(m, local)`; the non-JSON branch at line 101 calls
`printConfigGet(stdout, local, selected)`.

`config get` currently supports two scopes:

- Default (no flag): merged view with local-wins on collision.
- `--local`: local-only.

`config get <name>` with no flag returns the merged-view entry. With
`--local`, returns the local entry or "not found" if defined only in
global. The new `--global` should return the global entry or "not found"
if defined only in local.

`searchAllConfigCategories` and `loadNamesForCategory` are also called
from `config_edit.go`, `config_remove.go`, and `config_add.go`. Any
change to their signatures cascades to those callers — keep the cascade
minimised. The simplest options are to add parallel global-only
helpers, or to refactor the four `loadXxxForScope` wrappers to take
`config.Scope` and translate at every other caller's site. The
implementer picks the approach that touches the fewest files while
satisfying the requirements.

## Requirements

1. `config get` registers `--global` via
   `cmd.Flags().BoolVar(&flags.Global, "global", false, "Restrict to
   global config only")` with `cmd.MarkFlagsMutuallyExclusive("local",
   "global")`. The binding pattern matches `describe` and `get` after
   the prerequisite project.

2. `addConfigCommand` and `addConfigGetCommand` accept `flags *Flags`.
   `addConfigCommand` threads `flags` only to `addConfigGetCommand`;
   the other subcommand factories in `config.go` are unchanged.
   `NewRootCmd` passes `flags` to `addConfigCommand`.

3. `runConfigGet` derives its scope via
   `scopeFromFlags(getFlags(cmd))` and routes the four printers, the
   JSON branch, and the interactive flow to honour the derived scope.

4. `config get <name> --global` outputs the global definition of the
   named item with `Source` reported as `"global"`. When the item is
   defined only in local config, returns the existing "not found"
   error.

5. `config get <name> --local` keeps its existing behaviour.

6. `config get <name>` with no flag keeps its existing merged behaviour
   (local wins on name collision).

7. `config get <name> --local --global` returns the cobra
   mutual-exclusion error at parse time and produces no stdout output.

8. The JSON output shape of `config get --json` is unchanged. `--global`
   selects which entries are returned but does not alter the per-entry
   field set.

9. Behaviour of every command other than `config get` is unchanged.
   This includes `config list`, `config edit`, `config remove`,
   `config add`, `config order`, `config open`, `config export`,
   `config settings`, `modules install`, and any other caller of
   helpers shared with `config get`.

10. Tests cover the four `config get` scope outcomes against a fixture
    that defines the same name in both global and local config with
    different field values:
    - No flag: merged view shows the local-wins entry.
    - `--local`: local entry.
    - `--global`: global entry.
    - `--local --global`: cobra mutual-exclusion error, no output.

## Implementation Plan

1. Register `--global` on `config get`. Thread `flags` through the
   factory functions.

2. Add a global-only read path the `config get` printers can use. The
   implementer chooses the mechanism — likely either adding parallel
   `loadXxxForGlobalScope` helpers in `config_types.go`, or refactoring
   `loadForScope[T]` and the four wrappers to take `config.Scope` while
   keeping their non-`config get` callers behaviour-equivalent. Either
   way, the surface of the change must not extend into files outside
   the `config get` call chain listed in Current State.

3. Update `runConfigGet`, `searchAllConfigCategories`,
   `buildConfigListItem`, the four `printXxxGet` functions,
   `runConfigGetInteractive`, and `loadNamesForCategory` to pass the
   derived scope (or to route to the right helper) so `--global` is
   honoured end to end.

4. Add tests for the four scope outcomes. The fixture defines the same
   item name in both scopes with different field values so the path
   each flag picks is observable in the assertions.

5. Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Do not change behaviour for any command other than `config get`.
- Do not modify the JSON output shape of any command.
- Do not touch `config_list.go`'s list functions,
  `config_settings.go`'s `loadSettingsForScope`, or
  `internal/config/settings.go`'s `ResolveAllSettings`. These are not
  on the `config get` call chain.
- Do not modify any file under `library/` or `homebrew-tap/`.
- `config get <name> --global` where `<name>` exists only in local
  config returns the standard "not found" error, not a "shadowed by
  local" message.

## Implementation Guidance

- The minimal-surface approach is parallel global-only helpers
  alongside the existing `loadXxxForScope` wrappers. The maximal-surface
  approach is refactoring the wrappers to take `config.Scope`, which
  cascades to every caller of those wrappers
  (`config_list.go`'s list functions, `config_edit.go`,
  `config_remove.go`, etc.). Out of scope rules out the cascade.
- `searchAllConfigCategories` and `loadNamesForCategory` are also used
  by write commands (`config edit`, `config remove`). If their
  signatures change, callers outside the `config get` chain must keep
  their existing two-state behaviour (no `--global` semantics on
  writes). A parallel helper is usually less invasive than a signature
  change here.
- `Source` reported by the printers comes from `loadForScope[T]`'s
  per-entry tagging. The global-only path tags as `"global"` so the
  `Source:` line in printer output reflects where the entry came from.

## Acceptance Criteria

- `start config get <name> --global` returns the global definition
  when both scopes define `<name>` with different fields. The output
  contains `Source: global`.
- `start config get <name> --global` returns the standard "not found"
  error when `<name>` is defined only in local config.
- `start config get <name> --local` returns the standard "not found"
  error when `<name>` is defined only in global config.
- `start config get <name>` with no flag returns the merged view with
  local winning on name collisions.
- `start config get <name> --local --global` returns the cobra
  mutual-exclusion error at parse time and produces no stdout output.
- `start config get --json <name>` JSON shape is identical to before
  this project; only the selected entry's content reflects the scope.
- Existing tests for `config list`, `config edit`, `config remove`,
  `config add`, `config order`, `config open`, `config export`,
  `config settings`, and `modules install` pass without modification.
- New tests cover the four `config get` scope outcomes.
- `scripts/invoke-tests` passes.
