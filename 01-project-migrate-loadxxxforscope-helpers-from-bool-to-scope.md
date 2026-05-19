# Migrate loadXxxForScope helpers from bool to Scope

Source: pre-commit review on 2026-05-19
Severity: info
Category: Architecture
Location: `internal/cli/config.go:97-179`, `internal/cli/config_types.go`, `internal/cli/config_settings.go:324`

## Goal

Unify the scope-passing convention across the CLI's config-loading helpers so the entire layer speaks `config.Scope` instead of half-and-half. Today `config.ResolveAllSettings` accepts the three-state `config.Scope` while every sibling helper (`loadAgentsForScope`, `loadRolesForScope`, `loadContextsForScope`, `loadTasksForScope`, `loadSettingsForScope`) still accepts a two-state `localOnly bool`. Functions that load several categories — most visibly `runConfigList` — alternate between the two shapes back-to-back, which obscures intent and blocks future `--global` support on `start config`.

## Scope

In scope:

- Change every `loadXxxForScope(localOnly bool)` helper in `internal/cli/` to accept `config.Scope`.
- Replace each helper's two-arm `if localOnly { ... } else { ... }` with `paths.ForScope(scope)` and the standard loader, matching the existing pattern in `loadConfig` (`internal/cli/describe.go:442`).
- Update every caller to pass a `config.Scope` value. Where the caller still receives `--local` only, use `config.ScopeFromLocal(local)` at the call site.
- Update tests that call the helpers directly.

Out of scope:

- Adding `--global` to any command. (`start config` may later want it; this project only prepares the helpers.)
- Changing merge semantics or precedence rules.
- Replacing the `flags.Local` bool on the `Flags` struct or rewiring cobra flag registration.
- Removing `config.ScopeFromLocal` — it remains the bridge for call sites whose CLI surface accepts only `--local`.
- Any changes under `library/` or `homebrew-tap/`.

## Current State

`config.Scope` is defined at `internal/config/paths.go:9-19` with three values (`ScopeMerged`, `ScopeGlobal`, `ScopeLocal`). `Paths.ForScope(scope)` at `internal/config/paths.go:115-138` already returns the correct directory list for all three. `config.ScopeFromLocal(local)` at `internal/config/paths.go:36-41` maps the legacy bool to a Scope.

The five helpers still on the bool API:

| Helper | File | Signature |
| --- | --- | --- |
| `loadAgentsForScope` | `internal/cli/config_types.go:79` | `(localOnly bool) (map[string]AgentConfig, []string, error)` |
| `loadRolesForScope` | `internal/cli/config_types.go` (same file) | `(localOnly bool) (..., error)` |
| `loadContextsForScope` | `internal/cli/config_types.go` (same file) | `(localOnly bool) (..., error)` |
| `loadTasksForScope` | `internal/cli/config_types.go` (same file) | `(localOnly bool) (..., error)` |
| `loadSettingsForScope` | `internal/cli/config_settings.go:324` | `(localOnly bool) (map[string]string, error)` |

Each helper today has the same two-branch shape:

```go
if localOnly {
    // load from paths.Local only
} else {
    // load from paths.Global then paths.Local (local wins)
}
```

This is exactly what `paths.ForScope` already encodes for all three scopes, including the missing `ScopeGlobal` arm.

The internal helper `loadConfigForScope` at `internal/cli/config_types.go:180` (used by `runConfigList` for the default-agent lookup at `internal/cli/config.go:109` and again at `internal/cli/config_list.go:292`) is also on the bool API and should migrate with the rest.

Call-site totals per helper (verified via `rg "<helper>\\(" internal/cli/`):

| Helper | Count | Files |
| --- | --- | --- |
| `loadAgentsForScope` | 8 | `config.go`, `config_get.go`, `config_edit.go`, `config_interactive.go`, `config_helpers.go`, `config_list.go` (×3), `config_types_test.go` |
| `loadRolesForScope` | 8 | `config.go`, `config_get.go`, `config_edit.go`, `config_helpers.go`, `config_interactive.go`, `config_list.go` (×3) |
| `loadContextsForScope` | 8 | same files as `loadRolesForScope` |
| `loadTasksForScope` | 8 | same files as `loadRolesForScope` |
| `loadConfigForScope` | 2 | `config.go`, `config_list.go` |
| `loadSettingsForScope` | 1 | `resolve.go` |

Total ≈ 35 call sites. The four `roles/contexts/tasks` helpers cluster together at each caller — `config_list.go` alone hosts nine of them across three functions. Re-run `rg` at the start of the work to get current line numbers (they drift).

`config.ResolveAllSettings` was migrated to `Scope` in commit on 2026-05-19 (see `internal/config/settings.go:67`). The CLI-layer helpers are the remaining holdouts.

## Requirements

1. Every `loadXxxForScope` helper in `internal/cli/` (the five listed above plus `loadConfigForScope`) accepts `config.Scope` and routes via `paths.ForScope(scope)`.
2. Every caller passes a `config.Scope`. Where a caller's own contract is still a bool (because its CLI surface only exposes `--local`), the conversion happens at the call site via `config.ScopeFromLocal(local)`.
3. Behaviour is identical before and after for every existing command and test. No new scope handling is introduced (the helpers will support `ScopeGlobal` after this change, but no caller uses it yet — that's left for a subsequent `--global` rollout).
4. `runConfigList` in `internal/cli/config.go` reads as a single uniform sequence — one scope variable, one API style across all five category loads.
5. Existing tests pass unchanged. `internal/cli/config_types_test.go:336` updates mechanically (`false` → `config.ScopeMerged`).
6. `go build ./...`, `go vet ./...`, and `scripts/invoke-tests` all pass clean.

## Implementation Plan

1. Migrate `loadAgentsForScope` first as the reference implementation: change signature, replace branching with `paths.ForScope(scope)`, run the affected tests. This is the most-called helper, so getting it right anchors the pattern.
2. Apply the same shape to `loadRolesForScope`, `loadContextsForScope`, `loadTasksForScope`, `loadSettingsForScope`, and `loadConfigForScope` in one sweep.
3. Update every call site in the table above. At each, the change is mechanical: wrap the existing `local` argument in `config.ScopeFromLocal(...)`. Do not change the cobra flag layer or the `Flags` struct.
4. In `runConfigList` specifically, hoist the `scope := config.ScopeFromLocal(local)` line so it appears once at the top of the function, then thread the `scope` variable into all five loads instead of repeating the conversion.
5. Run `gofmt -l` over touched files and confirm zero output.
6. Run `go test ./internal/...` and `scripts/invoke-tests`.

Snippet — target shape of each migrated helper (illustrative, not a literal copy-paste):

```go
func loadAgentsForScope(scope config.Scope) (map[string]AgentConfig, []string, error) {
    paths, err := config.ResolvePaths("")
    if err != nil {
        return nil, nil, fmt.Errorf("resolving config paths: %w", err)
    }
    dirs := paths.ForScope(scope)
    // ...existing load + extract logic, unchanged
}
```

## Constraints

- Pure refactor. No behaviour change for any command. No new flags. No new CLI surface.
- Do not remove `config.ScopeFromLocal`; it stays as the bridge for `--local`-only commands.
- Do not change the `Flags.Local` bool or how cobra populates it.
- Do not extend `--global` to `start config` or any other command in this project — that is a separate piece of work that will use these migrated helpers.
- Preserve all existing function exports and names. Only the parameter type changes.
- Do not touch `library/` or `homebrew-tap/`.

## Acceptance Criteria

- Every `loadXxxForScope` helper in `internal/cli/` takes `config.Scope`; `rg "loadXxxForScope|localOnly bool" internal/cli/` returns no bool-typed scope parameters on these helpers.
- `runConfigList` declares `scope` once and passes it to all five category loads with no `local` bool reappearing inside the function body after the conversion line.
- `git grep -n 'For Scope(local)' internal/cli/` (or equivalent) shows every former bool-typed call now wrapped in `config.ScopeFromLocal(...)`.
- `go test ./internal/...` and `scripts/invoke-tests` both pass on the resulting branch.
