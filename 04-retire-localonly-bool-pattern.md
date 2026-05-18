# Retire localOnly bool pattern codebase-wide (optional)

## Goal

Two scope-selection styles coexist in the codebase: `config.Scope`
(three-state enum, used by `describe`, `get`, and `config get` after the
preceding projects land) and `localOnly bool` (two-state, used by every
other config-related helper). The split is cosmetic — both produce the
same merged/local behaviour — but it forces translation at every
boundary between the two. Replace the bool pattern with `config.Scope`
everywhere so a single scope type flows through the codebase.

This project is optional. It delivers no user-visible behaviour change
and exists solely to retire the older pattern. Skip it indefinitely if
codebase-wide scope-type uniformity is not a goal.

## Scope

In scope:

- Replace `localOnly bool` (or equivalent `local bool`) with
  `config.Scope` on every read-side helper that selects scope. The
  helpers in question are listed in Current State.
- Update every caller of those helpers — production code and tests — to
  pass `config.Scope` values instead of `bool`.
- Translate `false → config.ScopeMerged` and `true → config.ScopeLocal`
  literally. No call site outside the three commands that own
  `--global` constructs `config.ScopeGlobal`.

Out of scope:

- Write-side helpers that take `local bool` for a binary write-target
  decision (e.g. `removeAgent`, `configAgentEdit`, `unsetSetting`,
  `setSetting`, `editSettings`, `resolveConfigOpenPath`,
  `exportSingleCategory`). These are not scope selectors — they
  choose a target directory for a write. They keep their `bool`
  signature.
- Any change to user-visible behaviour. Output, JSON shape, error
  messages, and side effects of every command are preserved exactly.
- Adding any new flag, command, or behaviour.
- Any change under `library/` or `homebrew-tap/`.

## Prerequisites

This project assumes projects 01, 02, and 03 have landed. After 03, the
`config get` call chain already uses either parallel global-only helpers
or partially-refactored wrappers. This project completes the refactor
and unifies the read-side scope type across the codebase.

If 03 took the parallel-helper approach, this project also retires those
parallel helpers and folds their behaviour into the unified
`config.Scope`-based helpers.

## Current State

Read-side helpers that take `localOnly bool` (or `local bool`) for scope
selection and are in scope for this refactor:

- `internal/cli/config_helpers.go` — `loadForScope[T]`,
  `searchAllConfigCategories`.
- `internal/cli/config_interactive.go` — `loadNamesForCategory`.
- `internal/cli/config_types.go` — `loadAgentsForScope`,
  `loadRolesForScope`, `loadContextsForScope`, `loadTasksForScope`,
  `loadConfigForScope`.
- `internal/cli/config_settings.go` — `loadSettingsForScope`,
  `listSettings`, `showSetting`, `listSettingsJSON`,
  `showSettingJSON`.
- `internal/cli/config_list.go` — `buildConfigListItem`,
  `collectConfigListItems`, `listAgents`, `listRoles`, `listContexts`,
  `listTasks`.
- `internal/cli/config_get.go` — `printAgentGet`, `printRoleGet`,
  `printContextGet`, `printTaskGet`, `printConfigGet`,
  `runConfigGetInteractive`. (Already updated by project 03 if 03 took
  the refactor approach; otherwise updated here.)
- `internal/config/settings.go` — `ResolveAllSettings`.

Write-side helpers that take `local bool` and stay as bool:

- `internal/cli/config_settings.go` — `setSetting`, `unsetSetting`,
  `editSettings` (use `paths.Dir(localOnly)` to pick a write target).
- `internal/cli/config_edit.go` — `runConfigEditInteractive`,
  `configEditByCategory`, `configAgentEdit`, `configRoleEdit`,
  `configContextEdit`, `configTaskEdit`.
- `internal/cli/config_remove.go` — `runConfigRemoveInteractive`,
  `confirmConfigRemoval`, `removeConfigItem`, `removeAgent`,
  `removeRole`, `removeContext`, `removeTask`.
- `internal/cli/config_add.go` — `configAgentAdd`, `configRoleAdd`,
  `configContextAdd`, `configTaskAdd`.
- `internal/cli/config_order.go` — `reorderRoles`, `reorderContexts`.
- `internal/cli/config_open.go` — `resolveConfigOpenPath`.
- `internal/cli/config_export.go` — `exportSingleCategory`.
- `internal/cli/config_helpers.go` — `confirmMultiRemoval`,
  `scopeString` (display string helper).

Call sites that read `flags.Local` directly and pass it into refactored
helpers — these translate `flags.Local` to `config.Scope` at the entry
point:

- `internal/cli/config_settings.go` — `executeConfigSettings`.
- `internal/cli/config.go` — `runConfigList`.
- `internal/cli/config_list.go` — `runConfigListCmd`,
  `runConfigTaskList`.
- `internal/cli/modules_install.go` — `runModulesInstall`.
- `internal/cli/resolve.go:748` — `resolveLibraryIndexPath`.
- `internal/doctor/checks.go:491` — `CheckSettings`.

Where the write side calls a refactored read helper (e.g.
`configAgentEdit` calling `loadAgentsForScope` to fetch existing
agents before editing), the write helper translates its `local bool`
to `config.Scope` inline at the call site.

`config.Scope` already exists at `internal/config/paths.go:9` with
three values and a `String()` method.

External tests that pass `false`/`true` literals to refactored helpers
must be updated mechanically:

- `internal/config/settings_test.go` — four tests at lines 77, 121,
  162, 203.

## Requirements

1. Every read-side helper listed in Current State accepts
   `config.Scope` instead of `bool`.

2. Every call site that passes `flags.Local` to a refactored helper
   translates the bool to `config.Scope` at the entry point (`true →
   config.ScopeLocal`, `false → config.ScopeMerged`). The translated
   scope is passed through the rest of the chain.

3. Write-side helpers keep their `local bool` signatures. Where they
   call a refactored read helper, they translate inline at the call
   site.

4. User-visible behaviour is unchanged for every command. Output,
   JSON shape, error messages, and side effects of every existing
   command match the behaviour produced by projects 01 + 02 + 03.

5. No call site outside the three commands that own `--global`
   (`describe`, `get`, `config get`) constructs `config.ScopeGlobal`.

6. External tests in `internal/config/settings_test.go` pass after a
   literal-only update (`false → config.ScopeMerged`, `true →
   config.ScopeLocal`). Assertions are unchanged.

7. `internal/doctor/checks.go:491` passes `config.ScopeMerged` after
   the signature swap. Existing `TestCheckSettings_*` assertions are
   unchanged.

8. If project 03 introduced parallel global-only helpers
   (`loadXxxForGlobalScope` or similar), they are retired here and
   their behaviour is folded into the unified `config.Scope`-based
   helpers.

## Implementation Plan

1. Refactor `loadForScope[T]` and the four typed wrappers
   (`loadAgentsForScope`, `loadRolesForScope`, `loadContextsForScope`,
   `loadTasksForScope`) plus `loadConfigForScope`. Compile errors at
   every call site guide the rest of the refactor.

2. Walk every compile error. For each call site:
   - If the site reads `flags.Local` directly, translate it to
     `config.Scope` at that site.
   - If the site is a read-side helper listed in Current State,
     refactor its signature to take `config.Scope` and propagate the
     change to its callers.
   - If the site is a write-side helper, keep its `bool` signature and
     translate inline at the refactored-helper call.

3. Refactor `ResolveAllSettings` last. Its cascade is the smallest set
   (the four `settings_test.go` cases, `doctor/checks.go:491`, and the
   sites inside `config_settings.go`).

4. If project 03 introduced parallel global-only helpers, retire them
   in this step by replacing call sites with the unified helper.

5. Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- No user-visible behaviour change. Every existing test must pass with
  no assertion changes — only signature-driven mechanical updates to
  test fixtures and call sites.
- No new flag, command, or behaviour.
- Do not refactor write-side helpers' signatures.
- Do not modify any file under `library/` or `homebrew-tap/`.

## Implementation Guidance

- The refactor is mechanical. Resist the temptation to fold in
  unrelated cleanups (renames, doc updates, dead code removal). Keep
  the diff narrow so review is straightforward.
- A small inline expression at translation sites is fine
  (`scope := config.ScopeMerged; if local { scope = config.ScopeLocal
  }`). Introducing a helper is acceptable only if it removes obvious
  duplication.
- The `Source` field tagging in `loadForScope[T]` continues to mirror
  the scope: `"global"` for entries from the global dir under
  `ScopeMerged` or `ScopeGlobal`, `"local"` for entries from the local
  dir.

## Acceptance Criteria

- Every read-side helper listed in Current State takes `config.Scope`.
- No `localOnly bool` (or `local bool` used for read-side scope
  selection) remains in the helpers listed in Current State.
- Write-side helpers retain their `bool` signatures.
- All existing tests pass with no assertion changes — only mechanical
  updates to literals.
- `scripts/invoke-tests` passes.
- Parallel global-only helpers introduced by project 03 (if any) are
  retired.
