# Scope flag cleanup and merge-semantics documentation

## Goal

Address the cluster of design inconsistencies in scope-flag handling and merge-semantics documentation that surfaced during the `00-refactor-functions.md` design discussion. None of these are bugs that affect today's runtime behaviour for the primary global-only usage pattern. They are quality issues that complicate maintenance, mislead readers, and undermine the user-facing contract for anyone who exercises local configs or both-scopes-present workflows.

This project is sequenced after `05-drop-redundant-search.md` and before the eventual `config get` work. The `config get` design will inherit the scope-flag surface that lands here, so getting it right before that work starts saves a downstream migration.

## Scope

In scope:

- `--global` flag asymmetry: promote from describe-only to root persistent flag with central plumbing.
- `--local` semantic overload: per-command help-text pass; consider rejecting on `start` (run command).
- `ScopeMerged` naming: rename or document the actual semantics.
- Default merge behaviour: document at the user-facing layer.
- Test coverage for scope paths: add per-command integration tests for both-scopes-present + name collision.

Out of scope:

- Changes to `mergeWithReplacement` semantics. The behaviour matches the design intent; only the labels and surfaces are problematic.
- Replacing `--local` / `--global` with a unified `--scope <global|local|merged>` flag. This was considered and rejected as too disruptive for the migration tax. If the position changes later, a separate project picks it up.
- `config info` itself, which will be replaced by `config get` in a later project. Issues 1-4 should land in a way that `config get` can adopt cleanly.
- Any change under `library/` or `homebrew-tap/`.
- Releases or tags.

## Current State

Three real scopes exist in the codebase, expressed as `config.Scope`:

| Scope | Meaning |
| ----- | ------- |
| `ScopeGlobal` | `~/.config/start/` only |
| `ScopeLocal` | `./.start/` only |
| `ScopeMerged` | union of both, with local replacing global per-name on collision |

The merge for `ScopeMerged` is implemented by `internalcue.Loader.mergeWithReplacement` at `internal/cue/loader.go:140`. For collection keys (`agents`, `roles`, `contexts`, `tasks`), same-named items are replaced wholesale by the later directory (local). There is no field-level merge. This is not CUE's native unification — it's an explicit Go-side merge step that rebuilds the merged CUE source and recompiles it.

### Issue 1: --global flag asymmetry

- `--local` is a root persistent flag (`internal/cli/root.go:95`). Every command in the CLI parses it.
- `--global` is a describe-only persistent flag (`internal/cli/describe.go:133`). No other command parses it.
- The `Flags` struct (`internal/cli/start.go:29`) has `Local bool` but no `Global` field.
- `--global` is read ad-hoc inside `describeScopeFromCmd` via `cmd.Flags().Lookup("global")`.
- Mutual exclusion (`--local` and `--global` both set is an error) is enforced only inside `describeScopeFromCmd`. No other command can express that check.

Consequence: `start config info claude --global` returns "unknown flag: --global". `start describe claude --global` works. Users have no signal in `--help` output that `--global` is describe-specific.

Recommended fix: promote `--global` to a root persistent flag. Add `Global bool` to `Flags`. Add a `Flags.Scope() (config.Scope, error)` helper that:

- Returns `ScopeLocal` if `--local` only.
- Returns `ScopeGlobal` if `--global` only.
- Returns an error if both are set.
- Returns `ScopeMerged` if neither is set.

Every scope-aware command calls one helper. Mutual exclusion lives in one place.

Files affected:

- `internal/cli/root.go` — register `--global` as persistent.
- `internal/cli/start.go` — add `Global bool` to `Flags`; add the `Scope()` helper.
- `internal/cli/describe.go` — remove the describe-specific `--global` registration and the local `describeScopeFromCmd`; call the new helper.
- `internal/cli/config_info.go`, `config_list.go`, `config_edit.go`, `config_remove.go`, `config.go`, `config_helpers.go` — opt into the helper where they currently read `flags.Local`.

### Issue 2: --local semantic overload

The flag carries different meanings across commands:

| Command | What `--local` means |
| ------- | -------------------- |
| `start` (run) | Nothing. Runtime always uses `ScopeMerged` regardless. |
| `start describe` | Filter view to local scope. |
| `start config info` | Filter view to local scope. |
| `start config list` | Filter view to local scope. |
| `start config edit` | Target the local file for modification. |
| `start config remove` | Target the local file for modification. |

The persistent flag's help text — "Target local config (./.start/) instead of global" (`root.go:95`) — papers over this. A user reading `start --help` cannot tell that `--local` does nothing on the run command, or that "target" means two different things across the config subfamily.

Recommended fix:

- For the run command: `--local` and `--global` are persistent root flags, so cobra cannot easily "remove" them from the run surface without affecting subcommands. Two concrete options: (a) keep them persistent and reject either in `runStart` when no subcommand was invoked, surfacing a clear error like "scope flags have no effect on the run command — runtime always uses the merged scope"; or (b) drop the persistent registration on root, re-register `--local` / `--global` non-persistently on each scope-aware subcommand. Option (a) is the smaller change; option (b) is the cleaner surface. Recommend option (a) for this project.
- Per-command long help (`Long:`) on each scope-aware command describes what `--local` and `--global` mean for that command specifically.
- The root persistent flag description shifts to a short generic phrase ("Restrict scope; meaning is command-specific — see <command> --help") rather than implying a single meaning.

### Issue 3: ScopeMerged naming

The constant name reads as "field-level CUE unification of both configs". The actual behaviour is "union of module names with local replacing global on collision". Readers (including the project author) misremember the semantics because the name suggests something the code does not do.

Recommended fix, two options:

1. Rename `ScopeMerged` to `ScopeUnion` or `ScopeCombined`. Rename `config.ScopeMerged` references across the codebase. Add a doc comment on the constant explaining the semantics ("union of global and local module names; local wins on collision"). Mechanical rename across ~20 sites.
2. Keep the name; add a doc comment on the constant and on `mergeWithReplacement` that explicitly says "this is not CUE unification". Zero rename churn but leaves the misleading name in place.

Recommend option 1 if appetite allows; option 2 if not.

### Issue 4: Default merge behaviour undocumented at user-facing layer

A user running `start describe claude` with no flags has no way to know that the output reflects "global + local with local winning per name". The `--help` output does not mention it. The root command's long description does not mention it. The README does not mention it.

Recommended fix:

- Add a one-line description to the root command long help: "Without --local or --global, configs are combined: local overrides global per name."
- Document the merge semantics in the README in the section that explains global vs local config.
- The `start describe` (no-arg listing) output prints a `settings/` block and per-category sections; consider adding a line above the per-category sections that names the scope explicitly when both configs are present.

### Issue 5: Scope path test coverage

The project author does not use local configs in day-to-day practice. The merge / scope code is covered by tests but not by manual usage. Any regression introduced in this area surfaces only through CI or via downstream user reports.

Recommended fix: add at least one integration test per scope-aware command (describe, config info, config list, config edit, config remove) that exercises both-scopes-present + name collision. The fixture for each should:

- Declare module `foo` in global with one set of fields.
- Declare module `foo` in local with a different set of fields.
- Assert the no-flag run shows local's version (the `mergeWithReplacement` contract).
- Assert `--local` shows local's version.
- Assert `--global` shows global's version (after issue 1 lands; today only `describe` supports this).
- Assert `--local --global` errors (after issue 1 lands).

These tests are the safety net for any future refactor that touches the merge path.

### Issue 6: Tie-in with config get

When the planned `config get` command replaces `config info`, the scope-flag surface lands with it. If issues 1-4 are unaddressed, `config get` inherits the asymmetry and the migration to fix it later costs twice (once for `config info`, once for `config get`).

Recommended sequencing: land issues 1-4 before `config get` design starts, so that `config get` is designed against the cleaned-up flag surface.

## References

- `00-refactor-functions.md` — surfaced these issues during design discussion; the metadata-writer refactor explicitly punts them here.
- `internal/cue/loader.go:140` — `mergeWithReplacement` implementation.
- `internal/cli/describe.go:133` — describe-only `--global` registration.
- `internal/cli/root.go:95` — root persistent `--local` registration.
- `internal/cli/start.go:29` — `Flags` struct (today has `Local`, lacks `Global`).

## Requirements

### 1. --global is a root persistent flag

- `cmd.PersistentFlags().BoolVar(&flags.Global, "global", false, "Restrict scope to global config")` registered alongside `--local` in `root.go`.
- `Flags.Global bool` field added.
- `Flags.Scope() (config.Scope, error)` helper centralises the local/global/merged decision and the mutual-exclusion check.
- `describeScopeFromCmd` is removed; describe calls `flags.Scope()`.
- All other scope-aware commands call `flags.Scope()` where they currently read `flags.Local`.

### 2. --local semantic clarification

- Per-command long help describes what `--local` and `--global` mean for that command.
- `start --local` (run command) either errors clearly or the flag is removed from the run surface.
- Root persistent flag short help points at command-specific docs.

### 3. ScopeMerged naming

Either renamed to a name that reflects the actual semantics, or annotated with a doc comment that documents the actual semantics. Implementer chooses between rename (preferred) and doc-only.

### 4. Documentation of default behaviour

- Root command long help mentions the merge behaviour.
- README's global vs local section mentions the merge behaviour.

### 5. Integration tests for scope paths

One integration test per scope-aware command exercising both-scopes-present + name collision, asserting the no-flag, `--local`, and `--global` cases plus the mutual-exclusion error.

### 6. Sequencing

This project lands before `config get` design begins.

## Implementation Plan

The implementer sequences these as separate commits, in this order:

1. `Flags.Scope()` helper and the `Global bool` field. No flag registered yet; just the typed plumbing and a unit test.
2. Register `--global` as a root persistent flag. Remove the describe-only registration. Switch `describe` to call `flags.Scope()`.
3. Switch the other scope-aware commands to `flags.Scope()`. One commit per command keeps reviews small.
4. Per-command help-text pass.
5. `ScopeMerged` rename or doc comment.
6. README updates.
7. Integration tests for scope paths.

## Constraints

- No change to `mergeWithReplacement` semantics. The merge behaviour is correct; the surfaces are the problem.
- No change to JSON output shapes.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Acceptance Criteria

- `start config info claude --global` is accepted by cobra and shows global-only output (or "not found" if only declared locally).
- `start config info claude --local --global` returns a mutual-exclusion error.
- `start --local` either errors clearly or the flag is not advertised on the run command.
- `Flags.Scope()` is the single source of truth for scope decisions; `cmd.Flags().Lookup("global")` does not appear in the codebase.
- Integration tests for both-scopes-present + name collision pass for describe, config info, config list, config edit, config remove.
- `ScopeMerged` either has a new name or has a doc comment that documents the actual semantics.
- Root command help mentions the default merge behaviour.
- README's global vs local section documents the merge behaviour.
- `scripts/invoke-tests` passes.
