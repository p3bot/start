# Add --global flag to start config get

## Goal

Add `--global` to `start config get` so users can inspect a module's global definition when a local definition is shadowing it. The supporting work is a refactor of the bool-keyed scope helpers used across the config command surface to take `config.Scope`, gaining a third "global-only" branch. Without that refactor, `--global` cannot be expressed at the load layer.

## Scope

In scope:

- Refactor every helper that currently takes `localOnly bool` for scope selection to take `config.Scope`. New `ScopeGlobal` branch reads the global directory only and tags `Source` as `"global"`.
- Add `Global bool` to the `Flags` struct.
- Register `--global` on the `config get` command as a per-command flag, mirroring the existing pattern on `describe` and `get`.
- Reject `--local --global` on `config get` via `cmd.MarkFlagsMutuallyExclusive`.
- Tests for the four `config get` scope outcomes (no flag, `--local`, `--global`, `--local --global`).

Out of scope:

- Promoting `--global` to a root persistent flag.
- Adding `--global` to commands other than `config get`.
- Introducing a `WriteScope` concept or rejecting `--global` on write commands.
- Rejecting `--local` or `--global` on the run, task, or prompt commands.
- Renaming `ScopeMerged`.
- README updates documenting merge semantics.
- Changes to `mergeWithReplacement` semantics.
- Any change under `library/` or `homebrew-tap/`.
- Releases or tags.

## Current State

This project depends on `03-config-info-to-get.md` having landed: the rename target file is `internal/cli/config_get.go` and the symbol names referenced below assume the post-rename state.

`start config get` supports two scopes today, gated on `flags.Local`:

- Default: merged view (global + local, local wins per name on collision).
- `--local`: local-only.

There is no way to ask for global-only output. The call chain is bool-keyed end to end:

```
runConfigGet
  → searchAllConfigCategories(query, local)        internal/cli/config_helpers.go
      → loadAgentsForScope(local) / loadRolesForScope(local) / ...
          → loadForScope[T](localOnly, ...)
  → buildConfigListItem(m, local)                  internal/cli/config_list.go
      → loadXxxForScope(local)
  → printAgentInfo(w, local, name) / ...           internal/cli/config_get.go
      → loadAgentsForScope(local)
```

Every link gates on a `bool`: true means local-only, false means merged. No branch reads global-only.

Functions with a `localOnly bool` or equivalent scope-selecting bool parameter:

- `internal/cli/config_helpers.go` — `loadForScope[T]`, `loadNamesForCategory`, `searchAllConfigCategories`.
- `internal/cli/config_types.go` — `loadAgentsForScope`, `loadRolesForScope`, `loadContextsForScope`, `loadTasksForScope`, `loadConfigForScope`.
- `internal/cli/config_settings.go` — `loadSettingsForScope`.
- `internal/cli/config_list.go` — `buildConfigListItem`, `collectConfigListItems`, `listAgents`, `listRoles`, `listContexts`, `listTasks`.
- `internal/cli/config_get.go` — `printAgentInfo`, `printRoleInfo`, `printContextInfo`, `printTaskInfo`, `printConfigGet`, `runConfigGetInteractive`.
- `internal/config/settings.go` — `ResolveAllSettings`. This file lives in `internal/config/`, outside `internal/cli/`, and is easy to miss when grepping for `localOnly` from `internal/cli/`.

Call sites that pass `flags.Local` (these are the translation points where the refactor swaps `bool` for `config.Scope`):

- `internal/cli/config_get.go` — multiple sites inside `runConfigGet` and the per-category printers.
- `internal/cli/config_list.go` — `runConfigListCmd`, `collectConfigListItems`, `buildConfigListItem`, `runConfigTaskList`.
- `internal/cli/config.go` — `runConfigList`.
- `internal/cli/config_edit.go` — `runConfigEdit`, `runConfigEditInteractive`, the four `configXxxEdit` functions.
- `internal/cli/config_remove.go` — `runConfigRemove`, `runConfigRemoveInteractive`, `removeConfigItem`, the four `removeXxx` functions, `confirmConfigRemoval`.
- `internal/cli/config_add.go` — `runConfigAdd` and the four `configXxxAdd` functions.
- `internal/cli/config_order.go` — `runConfigOrder`, `reorderRoles`, `reorderContexts`.
- `internal/cli/config_open.go` — `runConfigOpen`, `resolveConfigOpenPath`.
- `internal/cli/config_export.go` — `runConfigExport`, `exportSingleCategory`.
- `internal/cli/config_settings.go` — `executeConfigSettings`, `listSettings`, `showSetting`, `listSettingsJSON`, `showSettingJSON`, `setSetting`, `unsetSetting`, `editSettings`, `loadSettingsForScope`.
- `internal/cli/describe.go:197` — the settings call inside `runDescribeListing` (the directory listing already routes through `loadConfig(scope)`; only the settings section uses the bool helper).
- `internal/cli/modules_install.go` — `runModulesInstall`.
- `internal/cli/resolve.go:748` — `loadSettingsForScope(false)`.

Flag plumbing today:

- `internal/cli/start.go:29` — `Flags` struct has `Local bool` and no `Global` field.
- `internal/cli/root.go:103` — `--local` is registered as a root persistent flag.
- `internal/cli/describe.go:131` and `internal/cli/get.go:57` — `--global` is registered per-command on those two commands only. Both call the shared `describeScopeFromCmd` helper for scope derivation.
- `config get` does not register `--global` today.

Reference pattern for adding `--global` to a single command:

- `internal/cli/describe.go:131` — `describeCmd.PersistentFlags().Bool("global", false, "Restrict to global config only")`.
- `internal/cli/get.go:57` — same shape.

Reference pattern for cobra mutual-exclusion:

- `internal/cli/root.go:105` — `cmd.MarkFlagsMutuallyExclusive("role", "no-role")`.

## Requirements

1. Every helper listed in Current State that takes `localOnly bool` for scope selection accepts `config.Scope` instead. The three branches are `ScopeMerged` (current default behaviour), `ScopeLocal` (current `local=true` behaviour), and `ScopeGlobal` (new — reads global directory only, tags `Source` as `"global"`).

2. Every existing call site that passes `flags.Local` to a refactored helper translates the bool to a `config.Scope` at the entry point: `true` → `config.ScopeLocal`, `false` → `config.ScopeMerged`. The translated scope is passed through the rest of the chain. No call site outside `runConfigGet` (and the helpers it transitively touches) constructs `config.ScopeGlobal` in this project.

3. `Flags.Global bool` is added to the `Flags` struct in `internal/cli/start.go`.

4. `--global` is registered on the `config get` cobra command. Mirror the per-command registration shape used by `describe` and `get`. Do not add `--global` to any other command.

5. `cmd.MarkFlagsMutuallyExclusive("local", "global")` is registered on the `config get` command so cobra rejects `--local --global` at parse time before `runConfigGet` executes.

6. `runConfigGet` derives a `config.Scope` from `flags.Local` and `flags.Global` at the top of the function and passes it through every load and print call: `--local` → `ScopeLocal`, `--global` → `ScopeGlobal`, neither → `ScopeMerged`.

7. `config get <name> --global` outputs the global definition of the named item or returns the existing "not found" error when the item is local-only. `config get <name> --local` keeps its existing behaviour. `config get <name>` with no flag keeps its existing merged behaviour.

8. The JSON output shape of `config get --json` is unchanged. `--global` selects which entries are returned but does not alter the per-entry field set.

9. Every command other than `config get` continues to honour `flags.Local` exactly as it does today. The refactor is observable only inside `config get` (the new branch) and inside the helpers (new signatures).

10. Tests cover the four `config get` scope outcomes against a fixture that defines the same name in both scopes with different fields, so the path picked by each flag is observable in the assertions: no flag (merged view shows local-wins entry), `--local` (local entry), `--global` (global entry), `--local --global` (mutual-exclusion error, no output).

## Implementation Plan

1. Refactor the bool-keyed scope helpers to take `config.Scope`. Pure mechanical change: update signatures, rewrite the internal switch from two branches to three, translate every existing caller's `flags.Local` to `ScopeLocal` or `ScopeMerged`. No caller passes `ScopeGlobal` yet. No user-visible behaviour change. Verify with `scripts/invoke-tests` before moving on.

2. Add `Global bool` to the `Flags` struct.

3. Register `--global` on the `config get` command and add the cobra mutual-exclusion mark for the `local`/`global` pair.

4. In `runConfigGet`, derive the scope from the flags and pass it through to the refactored helpers in place of the existing `local` bool. The new `ScopeGlobal` branch is exercised end to end for the first time at this step.

5. Add tests for the four scope outcomes.

6. Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- The helper refactor must not change behaviour for any command that does not read `flags.Global`. Output, JSON shape, error messages, and side effects are preserved for `describe`, `get`, `config list`, `config edit`, `config remove`, `config add`, `config order`, `config open`, `config export`, `config settings`, `modules install`, and any other caller.
- `--global` is registered only on `config get`. Do not promote it to a root persistent flag and do not add it to other commands.
- Do not introduce a `WriteScope` helper or reject `--global` on write commands. Cobra's "unknown flag" handling is sufficient for commands that do not register `--global`.
- Do not reject `--local` or `--global` on the run, task, or prompt commands.
- Do not change the JSON output shape of any command.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- The helper refactor exists to make `--global` expressible on `config get`. Treat it as scaffolding; the user-visible payoff is `config get --global` working correctly. The refactor lands first so the rest of the work is a one-line scope derivation in `runConfigGet`.
- `describe` and `get` already build a `config.Scope` via `describeScopeFromCmd`. After the refactor, they continue to call into the load layer unchanged in semantics — their scope value just no longer has to be translated to a bool at the boundary.
- The settings call inside `runDescribeListing` at `internal/cli/describe.go:197` currently passes `flags.Local` to `config.ResolveAllSettings`. After the refactor, it should pass the same `config.Scope` value already used for the directory listing in the same function, so `describe --global` (today's existing flag) yields a consistent settings view as a side effect of the refactor. This is the only observable behaviour change outside `config get` and is a strict fix to an existing minor inconsistency.
- When `config get foo --global` is run and `foo` exists only in local, return the normal "not found" path. Do not invent a "shadowed by local" message.

## Acceptance Criteria

- `start config get <name> --global` shows the global definition when both scopes define `<name>`.
- `start config get <name> --local --global` returns a cobra mutual-exclusion error and produces no output.
- `start config get <name> --global` returns the standard "not found" error when `<name>` is defined only in local config.
- `start config get <name> --local` returns the standard "not found" error when `<name>` is defined only in global config.
- `start config get <name>` (no flag) returns the merged view with local winning on name collisions.
- `start config get --json <name>` JSON shape is identical to before this project; only the selected entry's content reflects the scope.
- Every helper listed in Current State accepts `config.Scope`; none retain a `localOnly bool` signature.
- Existing tests for `describe`, `get`, `config list`, `config edit`, `config remove`, `config add`, `config order`, `config open`, `config export`, `config settings`, and `modules install` pass without modification.
- `scripts/invoke-tests` passes.
