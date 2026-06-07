# Project: Make every context header state explicit

## Goal

The `start` and `start task` headers must always tell the user what happened with contexts, in plain words. Today the context section is silent in two cases that should be visible: when no contexts are loaded, and when the user opted out with `--context none`. Replace both silences with explicit lines so the context state is never ambiguous, matching the role section delivered by project `01-explicit-role-header.md`.

## Scope

In scope:

- Render an explicit line for all context outcomes: one or more contexts loaded (table, unchanged), no contexts loaded, and a deliberate `--context none` opt-out that resolved no explicit contexts.
- Distinguish "no contexts loaded" from "deliberate opt-out" using context selection state.
- Tests covering the render branches, the discriminator, and the end-to-end header.

Out of scope:

- The role section. That work is project `01-explicit-role-header.md`. This project mirrors it for contexts and should be implemented after it, reusing the same wording and styling conventions.

## Current State

The context section of the header is rendered by `printContextTable` in `internal/cli/output.go`. It early-returns when the contexts slice is empty, printing nothing:

```go
func printContextTable(w io.Writer, contexts []orchestration.Context, selection orchestration.ContextSelection) {
	if len(contexts) == 0 {
		return
	}
	...
}
```

Unlike roles, contexts have no `Compose` vs `ComposeWithRole` split. `Composer.Compose` (`internal/orchestration/composer.go`) always runs context selection. The opt-out lives in the `ContextSelection` passed in, not in a separate method.

The `--context none` sentinel sets `flags.NoImplicitContexts`, which in `executeStart` (`internal/cli/start.go`) zeroes the implicit selectors while leaving explicit tags and paths intact:

```go
if flags.NoImplicitContexts {
	selection.IncludeRequired = false
	selection.IncludeDefaults = false
}
```

So `--context none,foo` still loads `foo` (non-empty list, table renders), while `--context none` alone with no explicit selectors yields an empty list.

The empty list is reached by two semantically different states:

- Deliberate opt-out: `flags.NoImplicitContexts` was set and no explicit context resolved. The user turned implicit contexts off.
- No contexts loaded: contexts were attempted normally but none were configured or matched.

The print layer cannot distinguish these from the selection flags alone. `IncludeDefaults` is false both when `NoImplicitContexts` is set and for piped one-shot invocations (`start.go` passes `IncludeDefaults: false` for piped text), so a false `IncludeDefaults` does not imply opt-out. An explicit signal is required.

`ContextSelection` is defined in `internal/orchestration/composer.go` and flows into `Compose`. It is the natural carrier for the input fact that implicit contexts were suppressed; `Compose`, which assembles the final contexts list, is the producer that turns that fact plus its result into the section outcome.

Note on skipped-default rows: `Compose` appends any configured default context that was not loaded as a `skipped` row (`composer.go`), so `result.Contexts` is non-empty whenever the config defines default contexts. The empty-list state that the new lines address is reached only when no context — required, default, or explicit — ends up in the list. With `--context none` against a config that defines defaults, the table still renders those defaults as skipped rows; the opt-out line appears only when the resulting list is genuinely empty.

Reference for the line style: `printAgentModel` in `internal/cli/output.go` renders `Model: <value> via <source>`, where the value is plain and the `via <source>` suffix is dimmed through `tui.Annotate`. Project 01 established `Role: none` and `Role: skipped via --role none` on this idiom; this project mirrors that wording for contexts.

## Requirements

1. Reuse the shared `SectionState`/`SectionOutcome` type defined by project `01-explicit-role-header.md` in `internal/orchestration/composer.go`. `ComposeResult` carries `ContextOutcome SectionOutcome`.
2. `ContextSelection` carries `SuppressImplicit bool`, an input flag recording that the `--context none` sentinel suppressed implicit contexts. Set it where `flags.NoImplicitContexts` zeroes the implicit selectors, for both the `start` path (`internal/cli/start.go`) and the `start task` path (`internal/cli/task.go`). Document why it exists: to separate a deliberate opt-out from contexts simply being absent. A false `IncludeDefaults` cannot carry this, because piped one-shot invocations also set `IncludeDefaults: false`.
3. `Compose` stamps `result.ContextOutcome` once it has assembled the final contexts list:
   - `SectionListed` when `len(result.Contexts) > 0`.
   - `SectionSkipped` (Reason `--context none`) when the list is empty and `selection.SuppressImplicit` is set.
   - `SectionNone` when the list is empty and implicit contexts were not suppressed.
   `Compose` is the producer of the contexts list, so it owns the outcome; the render layer never sees `SuppressImplicit`. Define `contextSkipReason = "--context none"` once as a constant.
4. `printContextTable` switches on `outcome.State`:
   - `SectionListed`: render the table exactly as today (still using `selection` for the populated `required, default, <tag>` annotation).
   - `SectionNone`: render a single line whose value is `none`.
   - `SectionSkipped`: render a single line whose value is `skipped`, with `outcome.Reason` rendered as a dimmed `via <reason>` annotation.
5. The populated path is unchanged. An opt-out that still resolves explicit contexts (for example `--context none,foo`) yields a non-empty list, so `Compose` stamps `SectionListed` and the table renders normally.
6. The line styling matches the role section from project 01 and the existing `Model:` line: the `none` value is plain; the `skipped` value is plain with the flag rendered as a dimmed annotation.
7. Tests cover the render switch, the `Compose` stamping across the three states, and the real header output at the call sites.

## Constraints

- The render layer must be a pure switch over `ContextOutcome.State`. `printContextTable` must not re-derive the section state from `len(contexts)` or the selection flags for the empty case; it still reads `selection` only to build the populated-table annotation.
- Stamp the outcome in `Compose`, the producer of the contexts list. Carry only the input fact (`SuppressImplicit`) on the selection; do not thread `flags.NoImplicitContexts` or the reason string into `printContextTable`.
- Reuse the shared `SectionState`/`SectionOutcome` defined by project 01. Keep wording and styling consistent so the role and context sections read as a matched pair.
- Define the opt-out reason once as a constant.
- Use `tui.Annotate` only for the opt-out reason (the dimmed `via <flag>` suffix). Do not annotate the `none` value or the `skipped` value itself.
- Match the existing spacing and blank-line convention of the populated context table and the agent/model lines.
- Preserve current behaviour exactly when one or more contexts (including skipped default rows) are present. Only the genuinely-empty cases change, and both change from silent to explicit.
- Follow the repository's testing approach (`AGENTS.md`): real CUE validation, real files via `t.TempDir()`, table-driven cases.

## Implementation Plan

1. Reuse `SectionState`/`SectionOutcome` from project 01. Add `ContextOutcome SectionOutcome` to `ComposeResult` and `SuppressImplicit bool` to `ContextSelection`, both documented.
2. In `internal/cli/start.go` and `internal/cli/task.go`, set `selection.SuppressImplicit = true` in the same branch that zeroes the implicit selectors under `flags.NoImplicitContexts`, so it travels into `Compose` on the selection.
3. In `internal/orchestration/composer.go`, stamp `result.ContextOutcome` at the end of `Compose` across the three states. Add the `contextSkipReason` constant.
4. In `internal/cli/output.go`, rewrite `printContextTable` to switch over `outcome.State`, passing `result.ContextOutcome` from the five call sites. The `SectionListed` arm is the current table code unchanged; the other two arms print the single-line states, both matching the `Model:` line format and trailing blank-line convention.

## Acceptance Criteria

1. Running `start` (and `start task ...`) with no contexts configured (and no default rows to show) prints a context line whose value is `none`.
2. Running with `--context none` and no explicit selectors, against a config with no default contexts, prints a context line reporting contexts were skipped and naming the flag that caused it (`skipped via --context none`).
3. Running with `--context none,<name>` (or any selector that resolves a context) renders the context table as before and prints neither empty-state line.
4. The no-contexts line and the opt-out line are visibly distinct, and both are consistent in wording and styling with the role lines from project 01.
5. The `none` value is not dimmed or parenthesised. The opt-out reason is rendered as a dimmed annotation consistent with the model line.
6. Unit tests assert the `printContextTable` arms: `SectionNone` prints the `none` line; `SectionSkipped` prints the opt-out line; `SectionListed` renders the table and prints neither.
7. A composer test asserts `Compose` stamps `ContextOutcome` `SectionListed` (contexts present), `SectionSkipped` with Reason `--context none` (empty list, `SuppressImplicit` set), and `SectionNone` (empty list, not suppressed).
8. An end-to-end test drives the real `start` (and `start task`) execution path with a no-contexts config and asserts the header contains the `none` context line.
