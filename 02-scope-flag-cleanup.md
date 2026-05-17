# Scope flag cleanup and merge-semantics documentation

## Goal

Address the cluster of design inconsistencies in scope-flag handling and merge-semantics documentation that surfaced during the metadata-writer refactor design discussion. None of these are bugs that affect today's runtime behaviour for the primary global-only usage pattern. They are quality issues that complicate maintenance, mislead readers, and undermine the user-facing contract for anyone who exercises local configs or both-scopes-present workflows.

This project is sequenced after `01-drop-redundant-search.md` and before the eventual `config get` work. The `config get` design will inherit the scope-flag surface that lands here, so getting it right before that work starts saves a downstream migration.

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
- `--global` is registered on both `describe` (`internal/cli/describe.go:131`) and `get` (`internal/cli/get.go:57`). No other command parses it. Both commands resolve scope via the shared `describeScopeFromCmd` helper.
- The `Flags` struct (`internal/cli/start.go:29`) has `Local bool` but no `Global` field.
- `--global` is read ad-hoc inside `describeScopeFromCmd` via `cmd.Flags().Lookup("global")`.
- Mutual exclusion (`--local` and `--global` both set is an error) is enforced only inside `describeScopeFromCmd`. No other command can express that check.

Consequence: `start config info claude --global` returns "unknown flag: --global". `start describe claude --global` works. Users have no signal in `--help` output that `--global` is describe-specific.

Recommended fix: promote `--global` to a root persistent flag. Add `Global bool` to `Flags`. Add two scope helpers that share a mutual-exclusion check:

- `Flags.Scope() (config.Scope, error)` for read commands. Returns `ScopeLocal` for `--local`, `ScopeGlobal` for `--global`, `ScopeMerged` when neither is set, and an error when both are set.
- `Flags.WriteScope() (config.Scope, error)` for write-target commands. Returns `ScopeLocal` for `--local`, `ScopeGlobal` when neither is set, and an error when `--global` is set (write-target commands already default to global; `--global` is redundant and surfaces as an explicit error rather than a silent no-op). The combined `--local --global` case is covered by the same error since `--global` alone is enough to reject.

Read commands (`describe`, `get`, `config info`, `config list`, the root `config` listing) call `Scope()`. Write commands (`config edit`, `config remove`, `config add`, `config settings set`/`unset`/`edit`, `config order`, `config open`) call `WriteScope()`. Primary mutual-exclusion enforcement is via `cmd.MarkFlagsMutuallyExclusive("local", "global")` registered in `root.go` — cobra rejects the combination at parse time, mirroring how `role`/`no-role` is already handled. The shared internal helper inside the two `Scope` methods keeps the same check as redundant defence in depth.

Files affected:

- `internal/cli/root.go` — register `--global` as persistent.
- `internal/cli/start.go` — add `Global bool` to `Flags`; add the `Scope()` and `WriteScope()` helpers plus their shared mutual-exclusion check.
- `internal/cli/describe.go` — remove the describe-specific `--global` registration; remove the local `describeScopeFromCmd`; call `Scope()`.
- `internal/cli/get.go` — remove the get-specific `--global` registration; switch the scope read from `describeScopeFromCmd` to `Scope()`.
- `internal/cli/get_test.go` — update the assertion at `get_test.go:559` that checks `getCmd.Flag("global")` to account for the flag being inherited from root rather than locally registered.
- `internal/cli/config_info.go`, `config_list.go`, `config.go` — call `Scope()` where they currently read `flags.Local`.
- `internal/cli/config_edit.go`, `config_remove.go`, `config_add.go`, `config_order.go`, `config_open.go` — call `WriteScope()` where they currently read `flags.Local`.
- `internal/cli/config_settings.go` — mixed file: `list` and `show` are read operations and call `Scope()`; `set`, `unset`, and `edit` are write operations and call `WriteScope()`. Convert each call site according to its semantic, not as a single sweep.
- `internal/cli/config_helpers.go` — adjust any helpers that take a `local bool` to take a `config.Scope` (or keep the bool and have callers translate, depending on which is simpler at each site).

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

Recommended fix: add at least one integration test per scope-aware command exercising both-scopes-present + name collision. The assertions differ for read vs write commands.

Read commands (`describe`, `get`, `config info`, `config list`). Fixture declares module `foo` in global with one set of fields and in local with a different set. Assertions:

- No-flag run shows local's version (the `mergeWithReplacement` contract).
- `--local` shows local's version.
- `--global` shows global's version (after Issue 1 lands).
- `--local --global` errors (cobra mutual-exclusion error after Issue 1 lands).

Write commands (`config edit`, `config remove`). Fixture declares module `foo` in both global and local. Assertions:

- No-flag invocation modifies the global file (the write-target default).
- `--local` invocation modifies the local file.
- `--global` returns an explicit error ("redundant on write commands"), no file is modified.
- `--local --global` errors (cobra mutual-exclusion error).

These tests are the safety net for any future refactor that touches the merge path or the scope-flag plumbing.

### Issue 6: Tie-in with config get

When the planned `config get` command replaces `config info`, the scope-flag surface lands with it. If issues 1-4 are unaddressed, `config get` inherits the asymmetry and the migration to fix it later costs twice (once for `config info`, once for `config get`).

Recommended sequencing: land issues 1-4 before `config get` design starts, so that `config get` is designed against the cleaned-up flag surface.

## References

- `internal/cue/loader.go:140` — `mergeWithReplacement` implementation.
- `internal/cli/describe.go:131` and `internal/cli/get.go:57` — current `--global` registrations (both per-command, both to be removed and replaced by a root persistent flag).
- `internal/cli/describe.go:424` — `describeScopeFromCmd` (called by both `describe` and `get`; to be removed).
- `internal/cli/root.go:95` — root persistent `--local` registration.
- `internal/cli/root.go:105` — existing `cmd.MarkFlagsMutuallyExclusive("role", "no-role")` (template for the new `local`/`global` mark).
- `internal/cli/start.go:29` — `Flags` struct (today has `Local`, lacks `Global`).

## Requirements

### 1. --global is a root persistent flag

- `cmd.PersistentFlags().BoolVar(&flags.Global, "global", false, "Restrict scope to global config")` registered alongside `--local` in `root.go`.
- `cmd.MarkFlagsMutuallyExclusive("local", "global")` registered in `root.go` alongside the existing `role`/`no-role` mark. Cobra rejects conflicting flag combinations at parse time before any `RunE` runs.
- `Flags.Global bool` field added.
- `Flags.Scope() (config.Scope, error)` helper for read commands: returns `ScopeLocal` / `ScopeGlobal` / `ScopeMerged` based on the flags, errors on `--local --global`.
- `Flags.WriteScope() (config.Scope, error)` helper for write-target commands: returns `ScopeLocal` for `--local`, `ScopeGlobal` as the default, errors when `--global` is set (rejected as redundant on write commands).
- Both helpers share a single internal mutual-exclusion check as defence in depth in case any caller reads `flags.Local` / `flags.Global` directly.
- `describeScopeFromCmd` is removed; `describe` and `get` call `flags.Scope()`.
- Read commands (`describe`, `get`, `config info`, `config list`, root `config`) call `flags.Scope()`.
- Write commands (`config edit`, `config remove`, `config add`, `config settings set`/`unset`/`edit`, `config order`, `config open`) call `flags.WriteScope()`.

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

1. `Flags.Scope()` and `Flags.WriteScope()` helpers, the `Global bool` field, and the shared mutual-exclusion check. No flag registered yet; just the typed plumbing and unit tests for each helper covering all flag combinations.
2. Register `--global` as a root persistent flag. Add `cmd.MarkFlagsMutuallyExclusive("local", "global")` in `NewRootCmd`. Remove the describe-specific and get-specific `--global` registrations. Switch `describe` and `get` to call `flags.Scope()`. Update `get_test.go:559` for the now-inherited flag.
3. Switch the remaining read commands (`config info`, `config list`, root `config`) to `flags.Scope()`. One commit per command keeps reviews small.
4. Switch the write commands (`config edit`, `config remove`, `config add`, `config settings set`/`unset`/`edit`, `config order`, `config open`) to `flags.WriteScope()`. One commit per command.
5. Run-command scope-flag rejection: in `runStart`, reject `flags.Local` or `flags.Global` with a clear error ("scope flags have no effect on the run command — runtime always uses the merged scope"). Behaviour change, not help text.
6. Per-command help-text pass: update each scope-aware command's `Long:` to describe what `--local` and `--global` mean for that command specifically; shorten the root persistent flag short-help to a generic phrase.
7. `ScopeMerged` rename or doc comment.
8. README updates.
9. Integration tests for scope paths (read and write assertions per Issue 5).

## Issues Discovered

1. Current State misstates where `--global` is registered (gap) — Resolved: amended Current State and Files affected.

   The project said `--global` was "a describe-only persistent flag" at `internal/cli/describe.go:133`, but the codebase also registers it on `get` at `internal/cli/get.go:57`, and `get` already calls `describeScopeFromCmd`. The "Files affected" list under Issue 1 was therefore incomplete, and Implementation Plan step 2 ("Remove the describe-only registration") needed to remove both registrations, not one.
   Resolution: Current State now lists both registration sites. Issue 1's Files affected list now includes `internal/cli/get.go` (with the same registration-removal and helper-switch treatment) and `internal/cli/get_test.go` (test assertion at line 559 needs updating once `--global` moves to root).

2. `Flags.Scope()` conflates read-filter and write-target semantics (design) — Resolved: option A (two helpers).

   The original proposed helper returned `ScopeMerged` when neither flag is set. That is the right default for read-style commands (`describe`, `config info`, `config list`, `get`) where the user wants the effective view. It was the wrong default for write-target commands (`config edit`, `config remove`, `config settings set`/`unset`, `config order`, `config open`) where `--local` toggles between two real targets and the default has always been global — there is no "merged" file to write to.
   Resolution: split into two helpers — `Flags.Scope()` for read paths (default `ScopeMerged`) and `Flags.WriteScope()` for write paths (default `ScopeGlobal`). They share a single internal mutual-exclusion check so the rule lives in one place. `WriteScope()` rejects `--global` explicitly (no silent no-op) since it adds nothing to the existing default. Issue 1 (Scope), Requirements §1, and the Implementation Plan have been updated to reflect the read/write split.

3. `config_settings.go` missing from affected-files list (gap) — Resolved: added to Files affected with read/write split note.

   Issue 1's original "Files affected" list enumerated `config_info.go`, `config_list.go`, `config_edit.go`, `config_remove.go`, `config.go`, `config_helpers.go`, but `config_settings.go` also reads `flags.Local` across `listSettings`, `showSetting`, `setSetting`, `unsetSetting`, `editSettings`, `loadSettingsForScope`.
   Resolution: `config_settings.go` is now listed as a separate entry in Issue 1's Files affected list, called out as a mixed file — `list`/`show` adopt `Scope()` (read), `set`/`unset`/`edit` adopt `WriteScope()` (write). Per-call-site conversion required, not a single sweep.

4. Mutual exclusion via cobra not considered (design) — Resolved: add cobra mark; keep helper check as defence in depth.

   The project originally planned to enforce `--local` + `--global` exclusion only inside `Flags.Scope()`. Cobra already exposes `cmd.MarkFlagsMutuallyExclusive("local", "global")` and the codebase uses the same mechanism for `--role`/`--no-role` at `internal/cli/root.go:105`. Marking the pair mutually exclusive on the root command lets cobra reject conflicting invocations before any `RunE` runs and yields a consistent error message across every command.
   Resolution: add `cmd.MarkFlagsMutuallyExclusive("local", "global")` in `NewRootCmd` alongside the existing role/no-role mark as the primary enforcement; keep the same check inside the shared helper used by `Scope()` / `WriteScope()` as a redundant guard for any direct flag reads. Requirements §1 and Issue 1's "Recommended fix" updated.

## Constraints

- No change to `mergeWithReplacement` semantics. The merge behaviour is correct; the surfaces are the problem.
- No change to JSON output shapes.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Acceptance Criteria

- `start config info claude --global` is accepted by cobra and shows global-only output (or "not found" if only declared locally).
- `start config info claude --local --global` returns a mutual-exclusion error.
- `start --local` either errors clearly or the flag is not advertised on the run command.
- `Flags.Scope()` and `Flags.WriteScope()` are the only consumers of `flags.Local` / `flags.Global`; `cmd.Flags().Lookup("global")` and `describeScopeFromCmd` do not appear in the codebase.
- `start config edit --global` (and other write-target commands) returns a clear error rather than silently no-opping.
- Integration tests for both-scopes-present + name collision pass for describe, config info, config list, config edit, config remove.
- `ScopeMerged` either has a new name or has a doc comment that documents the actual semantics.
- Root command help mentions the default merge behaviour.
- README's global vs local section documents the merge behaviour.
- `scripts/invoke-tests` passes.
