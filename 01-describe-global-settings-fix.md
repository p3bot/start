# Fix describe --global settings view

## Goal

`start describe --global` shows merged settings under the settings/ header
today, even though the directory listing in the same output honours
`--global` and shows global-only items. Make the settings block honour the
same scope as the listing so the output is internally consistent.

## Scope

In scope:

- Make the settings block inside `runDescribeListing` honour the scope
  derived from `--local`/`--global` flags.
- Add a test that defines a setting key with different values in global and
  local config and asserts each of `describe`, `describe --local`, and
  `describe --global` shows the right value.

Out of scope:

- Adding `--global` to any command other than where it already exists.
- Changes to merge semantics for any command other than `describe`.
- Refactoring helpers beyond what is strictly required to fix the settings
  call.
- Changes to flag registration, mutual-exclusion enforcement, or the
  `describeScopeFromCmd` helper.
- Any change under `library/` or `homebrew-tap/`.

## Current State

`runDescribeListing` at `internal/cli/describe.go:178` derives the requested
scope via `describeScopeFromCmd(cmd)` at line 182 and passes that scope to
`loadConfig(scope)` at line 207. The directory listing under the four
category headers (agents/, roles/, contexts/, tasks/) reflects the scope
correctly.

The settings block at lines 196-204 passes `flags.Local` directly to
`config.ResolveAllSettings`:

```go
flags := getFlags(cmd)
entries, err := config.ResolveAllSettings(paths, flags.Local)
```

Under `--global`, `flags.Local` is false, so `ResolveAllSettings` returns
merged settings. The settings block is the only part of the listing that
does not honour `--global`.

`config.ResolveAllSettings(paths Paths, localOnly bool)` lives at
`internal/config/settings.go:64`. Two-branch behaviour: `localOnly=true`
returns local-only; `localOnly=false` returns merged (global + local with
local winning). It has no global-only branch today.

`config.Scope` already exists with three values (`ScopeMerged`,
`ScopeGlobal`, `ScopeLocal`) at `internal/config/paths.go:9`, and
`Paths.ForScope` already routes correctly for all three.

## Requirements

1. `start describe --global` shows global-only settings under the
   settings/ header, with `Source` of each entry reported as `"global"`
   when the value comes from the global directory and `"default"` (or
   `"not set"`) otherwise.
2. `start describe --local` continues to show local-only settings
   unchanged.
3. `start describe` with no scope flag continues to show merged settings
   unchanged.
4. A new test in `internal/cli/describe_test.go` covers the three scope
   outcomes against a fixture where the same setting key has different
   values in global and local config.

## Implementation Plan

1. Pick the smallest mechanism that lets the settings block honour the
   scope. The implementer chooses between extending
   `config.ResolveAllSettings` to accept the three-state scope (and
   updating its callers mechanically), adding a parallel global-only
   resolver, or inlining a global-only branch in `runDescribeListing`.
   Whichever is chosen, behaviour of every other caller of
   `ResolveAllSettings` must be preserved.
2. Update the settings call inside `runDescribeListing` so it produces
   the entry set matching the scope already derived for the directory
   listing.
3. Add tests covering the three scope outcomes for the settings block.

## Constraints

- Do not change behaviour for any command other than `start describe`.
- Do not modify flag registration, mutual-exclusion handling, or
  `describeScopeFromCmd`.
- Do not introduce a codebase-wide refactor of the `localOnly bool`
  pattern.
- If `ResolveAllSettings`'s signature changes, every caller updates with
  it (mechanical change only — `false → config.ScopeMerged`, `true →
  config.ScopeLocal`). Callers include
  `internal/cli/config_settings.go`, `internal/cli/config.go`,
  `internal/doctor/checks.go:491`, and
  `internal/config/settings_test.go` (four test sites at lines 77, 121,
  162, 203).

## Acceptance Criteria

- `start describe --global` against a workspace where global and local
  settings differ shows the global value under settings/.
- `start describe --local` and `start describe` show their existing
  values unchanged.
- New test in `describe_test.go` covers the three scope outcomes for the
  settings block.
- `scripts/invoke-tests` passes.
