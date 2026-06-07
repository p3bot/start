# Project: Make every role header state explicit

## Goal

The `start` and `start task` headers must always tell the user what happened with the role, in plain words. Today the role section is silent in two cases that should be visible: when no role is installed or configured, and when the user opted out with `--role none`. Replace both silences with explicit lines so the role state is never ambiguous.

## Scope

In scope:

- Render an explicit line for all role outcomes: a resolved role (table, unchanged), no role available, and a deliberate `--role none` opt-out.
- Distinguish "no role available" from "deliberate opt-out" at the source so every header call site renders correctly.
- Tests covering the render branches, the composer discriminator, and the end-to-end header.

Out of scope:

- The identical gap in the context section (`printContextTable`). That work is project `02-explicit-context-header.md` and mirrors this one. Do not change context rendering here. This project lands first and establishes the pattern; project 02 follows it.

## Current State

The role section of the header is rendered by `printRoleTable` in `internal/cli/output.go`. It early-returns when the resolutions slice is empty, printing nothing:

```go
func printRoleTable(w io.Writer, resolutions []orchestration.RoleResolution) {
	if len(resolutions) == 0 {
		return
	}
	...
}
```

`printRoleTable` is called from five sites, all passing `result.RoleResolutions`:

- `internal/cli/start.go`: `printExecutionInfo`, `printDryRunSummary`, `printComposeError`
- `internal/cli/task.go`: `printTaskExecutionInfo`, `printTaskDryRunSummary`

An empty `RoleResolutions` is produced by two semantically different paths that currently share one representation:

- Deliberate opt-out: `--role none` normalises to `flags.NoRole`, which routes to `Composer.Compose` (`start.go` and `task.go` branch on `flags.NoRole`). `Compose` performs no role logic, so `RoleResolutions` stays empty. `flags.NoRole` is derived skip state set only by none-sentinel normalisation, so this path always means the user opted out via `--role none`.
- No role available: `Composer.ComposeWithRole` calls `selectDefaultRole` (`internal/orchestration/composer.go`), which returns an empty roleName and nil error when the config defines no roles:

```go
roles := cfg.LookupPath(cue.ParsePath(internalcue.KeyRoles))
if !roles.Exists() {
	return "", nil, nil
}
```

This leaves `RoleResolutions` empty.

There is no existing field that distinguishes these two states. In both, `result.RoleName == ""` and `result.RoleResolutions` is empty, so the render layer cannot tell them apart and stays silent for both. `ComposeResult` is defined in `internal/orchestration/composer.go` and is the shared result type for both methods. The fix makes the producer that decides each outcome record it explicitly on the result, so the render layer reports it with a pure switch instead of re-deriving intent from list length or CLI flags.

Note on explicit roles: `ComposeWithRole` is also the path for an explicit `--role foo`. In that case it always appends a resolution, so the resolutions slice is non-empty and neither empty-state line fires. The empty-and-attempted state occurs only when no explicit role is given and no role is installed or configured.

Reference for the line style: `printAgentModel` in `internal/cli/output.go` renders `Model: <value> via <source>`, where the value is plain and the `via <source>` suffix is dimmed through `tui.Annotate`. The new role lines follow this idiom.

## Requirements

1. Add a producer-owned section-outcome type to `internal/orchestration/composer.go`, shared by the role and context headers (project `02-explicit-context-header.md` reuses it):

```go
// SectionState classifies how a header section resolved so the render layer
// can report it without re-deriving intent from list length or CLI flags.
type SectionState int

const (
	SectionNone    SectionState = iota // nothing available; render "<label>: none"
	SectionListed                      // entries resolved; render the table
	SectionSkipped                     // deliberate opt-out; render "<label>: skipped via <reason>"
)

// SectionOutcome is the producer-set result for one header section. Whichever
// layer makes the decision sets it; the render layer only switches over it.
type SectionOutcome struct {
	State  SectionState
	Reason string // opt-out flag (e.g. "--role none"); set only for SectionSkipped
}
```

   The zero value is `SectionNone`, so a path that never stamps an outcome degrades to the neutral none line rather than rendering an empty table.

2. `ComposeResult` carries `RoleOutcome SectionOutcome`. `ComposeWithRole` stamps it: `SectionListed` whenever it has appended any role resolution row (loaded, skipped, or error), `SectionNone` when role selection finds no configured roles. Stamp it wherever the resolutions slice is finalised, so the early-return paths (such as a failing required role, which returns with error rows present) still carry the correct outcome.
3. The deliberate opt-out is stamped by the caller that chose it, not inferred. In the `flags.NoRole` branch of `executeStart` (`internal/cli/start.go`) and the task execution path (`internal/cli/task.go`), set `result.RoleOutcome = orchestration.SectionOutcome{State: orchestration.SectionSkipped, Reason: roleSkipReason}` after `Compose` returns. `Compose` performs no role logic and cannot know why it was called, so the opt-out is owned here. Define `roleSkipReason = "--role none"` once as a constant rather than repeating the literal.
4. `printRoleTable` takes the outcome and switches on `outcome.State`:
   - `SectionListed`: render the table exactly as today.
   - `SectionNone`: render a single line whose value is `none`.
   - `SectionSkipped`: render a single line whose value is `skipped`, with `outcome.Reason` rendered as a dimmed `via <reason>` annotation.
5. All five call sites pass `result.RoleOutcome` through.
6. The line styling is honest and consistent with the existing `Model:` line:
   - The `none` value is rendered plainly. It must not be dimmed or parenthesised, and must not claim the role was "installed" (roles can be defined inline in config, not only installed from the registry).
   - The `skipped` value is rendered plainly; only the reason is a dimmed annotation, in the same manner as the model line's `via <source>` suffix.
7. The populated path (`SectionListed`) is unchanged and never prints either empty-state line.
8. Tests cover the render switch, the producer stamping, and the real header output at the call sites.

## Constraints

- The render layer must be a pure switch over `RoleOutcome.State`. `printRoleTable` must not re-derive the section state from `len(resolutions)`, `flags.NoRole`, or any other signal. This is the core of the change: the producer states the outcome, the consumer renders it. It removes the previous dependency on the non-local invariant that `Compose` is reached only via `--role none`, so a future second caller of `Compose` cannot silently mislabel the section.
- Stamp the outcome where the decision is made. `ComposeWithRole` owns `SectionListed`/`SectionNone` because it performs role resolution; the `--role none` opt-out is owned by the CLI branch that routes to `Compose`. These paths are disjoint, so there is no double-write.
- Define the opt-out reason once as a constant; do not scatter the `--role none` literal across call sites.
- `SectionState`/`SectionOutcome` is shared with project `02-explicit-context-header.md`. Define it once so the role and context headers use one mechanism rather than diverging.
- Use `tui.Annotate` only for the opt-out reason (the dimmed `via <flag>` suffix), matching the model line. Do not annotate the `none` value or the `skipped` value itself.
- Match the existing spacing and blank-line convention of the populated role table and the agent/model lines, so the header layout stays uniform.
- Preserve current behaviour exactly for explicit `--role <name>` and for configs that resolve a role: both stamp `SectionListed` and render the table unchanged. Only the two empty cases change, and both change from silent to explicit.
- Follow the repository's testing approach (`AGENTS.md`): real CUE validation, real files via `t.TempDir()`, table-driven cases.

## Implementation Plan

1. In `internal/orchestration/composer.go`, add `SectionState`, `SectionOutcome`, and the `RoleOutcome SectionOutcome` field on `ComposeResult`.
2. In `ComposeWithRole`, stamp `result.RoleOutcome` `SectionListed`/`SectionNone` as the resolutions slice is finalised, covering the early-return paths.
3. In `internal/cli/start.go` and `internal/cli/task.go`, stamp `result.RoleOutcome = {SectionSkipped, roleSkipReason}` in the `flags.NoRole` branch after `Compose` returns. Add the `roleSkipReason` constant.
4. In `internal/cli/output.go`, rewrite `printRoleTable` to take the outcome and switch over `outcome.State`. The `SectionListed` arm is the current table code unchanged; the other two arms print the single-line states, both matching the `Model:` line's format and trailing blank-line convention.
5. Update the five call sites in `internal/cli/start.go` and `internal/cli/task.go` to pass `result.RoleOutcome`.

## Acceptance Criteria

1. Running `start` (and `start task ...`) with a config that defines no roles prints a role line whose value is `none`.
2. Running with `--role none` prints a role line reporting the role was skipped and naming the flag that caused it (`skipped via --role none`).
3. The no-role line and the opt-out line are visibly distinct, so the two states cannot be confused.
4. Running with an explicit `--role <name>` or a config that resolves a role renders the role table as before, and never prints an empty-state line.
5. The `none` value is not dimmed, not parenthesised, and does not contain the word "installed". The opt-out reason is rendered as a dimmed annotation consistent with the model line.
6. Unit tests assert all `printRoleTable` arms: `SectionNone` prints the `none` line; `SectionSkipped` prints the opt-out line with the dimmed reason; `SectionListed` renders the table and prints neither line.
7. A composer test asserts `ComposeWithRole` stamps `RoleOutcome.State == SectionListed` for a config with a resolvable role and `SectionNone` for a config with no roles (resolutions empty); a CLI-level test asserts the `--role none` path yields `SectionSkipped` with `Reason == "--role none"`.
8. An end-to-end test drives the real `start` (and `start task`) execution path with a no-roles config and asserts the header contains the `none` role line, guarding the call sites against future mis-wiring.
