# Unify --global flag registration on describe and get

## Goal

The `--global` flag on `start describe` and `start get` is registered and
enforced differently from every other flag in the codebase: bound through
cobra's internal storage rather than the `Flags` struct, with mutual
exclusion checked at runtime instead of by cobra. Migrate both commands to
the codebase's standard pattern (`BoolVar` binding plus
`MarkFlagsMutuallyExclusive`) so the third command that will gain
`--global` (`config get`, handled in a separate project) can register it
the same way without divergence.

## Scope

In scope:

- Add `Global bool` to the `Flags` struct in `internal/cli/start.go`.
- Migrate `describe` and `get` from
  `cmd.PersistentFlags().Bool("global", false, ...)` to
  `cmd.Flags().BoolVar(&flags.Global, "global", false, "Restrict to
  global config only")`.
- Call `cmd.MarkFlagsMutuallyExclusive("local", "global")` on each.
- Thread `flags *Flags` through `addDescribeCommand` and `addGetCommand`
  so the bind site has access to the struct.
- Replace `describeScopeFromCmd(cmd) (config.Scope, error)` with a new
  helper `scopeFromFlags(flags *Flags) config.Scope` that returns the
  scope from `flags.Global`/`flags.Local`. Delete
  `describeScopeFromCmd`.
- Update all current callers of `describeScopeFromCmd`
  (`runDescribeListing`, `runDescribeSearch`, `runGet`) to use
  `scopeFromFlags`.
- Update `TestGetLocalAndGlobalMutuallyExclusive` and its preceding doc
  comment so the assertion matches cobra's mutual-exclusion error
  format.

Out of scope:

- Adding `--global` to any new command. `config get --global` is a
  separate project.
- Refactoring any load helper (`loadAgentsForScope`, `ResolveAllSettings`,
  etc.).
- The settings-scope fix for `describe --global` — separate project.
- Changes to merge semantics.
- Promoting `--global` to a root persistent flag.
- Any change under `library/` or `homebrew-tap/`.

## Current State

The `Flags` struct at `internal/cli/start.go:29-41` defines `Local bool`
but no `Global` field. `--local` is bound at `internal/cli/root.go:103`
via `cmd.PersistentFlags().BoolVarP(&flags.Local, "local", "l", false,
...)`, following the codebase's standard binding pattern.

`--global` is registered at:

- `internal/cli/describe.go:131` —
  `describeCmd.PersistentFlags().Bool("global", false, ...)`
- `internal/cli/get.go:57` —
  `getCmd.PersistentFlags().Bool("global", false, ...)`

Both calls are unbound — the value lives in cobra's internal storage,
read via `cmd.Flags().GetBool("global")`. `PersistentFlags` is
unnecessary because neither command has subcommands. The pattern
diverges from every other flag in the codebase.

`describeScopeFromCmd` at `internal/cli/describe.go:424-440` reads
`--global` from cobra storage, reads `flags.Local` from the struct,
returns an error string `"--local and --global are mutually exclusive"`
when both are set, and otherwise returns the correct
`config.Scope`. It is called by `runDescribeListing`
(`describe.go:182`), `runDescribeSearch` (`describe.go:287`), and
`runGet` (`get.go:81`).

The codebase already uses `cmd.MarkFlagsMutuallyExclusive` at
`internal/cli/root.go:105` for `--role`/`--no-role`. Cobra v1.10.2 (pinned
in `go.mod`) handles mutual-exclusion at parse time and emits errors of
the form `"if any flags in the group [%v] are set none of the others can
be; %v were all set"` (verified at
`cobra@v1.10.2/flag_groups.go:204`). For mutual-exclusion between an
inherited persistent flag (`--local` from root) and a local flag
(`--global` on the subcommand), `MarkFlagsMutuallyExclusive` calls
`mergePersistentFlags()` before lookup so both flags are visible. Cobra's
`validateExclusiveFlagGroups` deduplicates by group name and silently
skips groups on commands where one of the flags is not defined, so
annotating `--local` from multiple commands is benign for other commands
that inherit `--local` but never register `--global`.

`NewRootCmd` at `internal/cli/root.go:42` creates the `Flags` struct and
passes commands by calling `addDescribeCommand(cmd)`, `addGetCommand(cmd)`,
etc. The flags pointer is not passed today.

`TestGetLocalAndGlobalMutuallyExclusive` at
`internal/cli/get_test.go:1202` asserts on the substring
`"--local and --global are mutually exclusive"`. Its doc comment at lines
1197-1201 references `describeScopeFromCmd` as the error-emitting site.

The corresponding describe test
`describe --local and --global are mutually exclusive` at
`internal/cli/describe_test.go:998-1002` asserts only `wantErr: true`
with no substring check, so it needs no update.

## Requirements

1. `Flags` struct in `internal/cli/start.go` has a `Global bool` field.

2. `describe` and `get` register `--global` via
   `cmd.Flags().BoolVar(&flags.Global, "global", false, "Restrict to
   global config only")`. Help text matches today's user-visible string.

3. `describe` and `get` each call
   `cmd.MarkFlagsMutuallyExclusive("local", "global")`.

4. A new helper `scopeFromFlags(flags *Flags) config.Scope` returns
   `ScopeGlobal` when `flags.Global` is true, `ScopeLocal` when
   `flags.Local` is true, and `ScopeMerged` otherwise. It returns no
   error — cobra enforces the mutual-exclusion invariant before this
   helper is called.

5. `describeScopeFromCmd` is deleted. All three call sites
   (`runDescribeListing`, `runDescribeSearch`, `runGet`) use
   `scopeFromFlags(getFlags(cmd))`.

6. `addDescribeCommand` and `addGetCommand` accept a `flags *Flags`
   parameter. `NewRootCmd` passes the struct to both.

7. `TestGetLocalAndGlobalMutuallyExclusive` asserts on a substring
   stable across cobra's mutual-exclusion error format. The substring
   `"none of the others can be"` is verified against the cobra version
   pinned in `go.mod` (v1.10.2).

8. The doc comment preceding
   `TestGetLocalAndGlobalMutuallyExclusive` (currently lines 1197-1201)
   no longer references `describeScopeFromCmd`. It points at cobra's
   parse-time validation as the error-emitting site instead.

9. All existing tests for `describe` and `get` continue to pass with no
   changes outside `TestGetLocalAndGlobalMutuallyExclusive` and its doc
   comment.

## Implementation Plan

1. Add `Global bool` to the `Flags` struct.

2. Update `addDescribeCommand` and `addGetCommand` to accept
   `flags *Flags`. Update `NewRootCmd` to pass `flags`.

3. Replace each `cmd.PersistentFlags().Bool("global", ...)` with
   `cmd.Flags().BoolVar(&flags.Global, "global", false, "Restrict to
   global config only")`. Add
   `cmd.MarkFlagsMutuallyExclusive("local", "global")` on each command.

4. Introduce `scopeFromFlags(*Flags) config.Scope`. Migrate
   `runDescribeListing`, `runDescribeSearch`, and `runGet` to use it.
   Delete `describeScopeFromCmd`.

5. Update `TestGetLocalAndGlobalMutuallyExclusive` assertion to match
   the new cobra error format. Update its doc comment to remove the
   `describeScopeFromCmd` reference.

6. Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- `cmd.Flags()` (local), not `cmd.PersistentFlags()`. Neither command has
  subcommands that should inherit `--global`.
- Do not promote `--global` to a root persistent flag.
- Do not change the user-visible flag name, default, or help string from
  what describe and get show today.
- Do not modify any file under `library/` or `homebrew-tap/`.

## Implementation Guidance

- `--local` is an inherited persistent flag from root; `--global` is a
  local flag on each subcommand. Both are visible in `cmd.Flags()` after
  cobra's `mergePersistentFlags()` runs inside
  `MarkFlagsMutuallyExclusive`, so the pairing is accepted.
- Cobra annotates the `--local` flag object once per command that calls
  `MarkFlagsMutuallyExclusive("local", "global")`. The same root flag
  receives duplicate group annotations. This is benign: cobra's
  `validateExclusiveFlagGroups` deduplicates by group name, and
  `hasAllFlags` skips groups on commands where `--global` is not
  registered. Commands like `start search`, `start task`, and
  `start prompt` that inherit `--local` but never register `--global`
  are unaffected at parse time.
- `scopeFromFlags` does not return an error because cobra rejects
  `--local --global` at parse time before `RunE` runs. Reflect that in
  the helper's signature.

## Acceptance Criteria

- `start describe --local --global` and `start get --local --global`
  return the cobra mutual-exclusion error at parse time and produce no
  stdout output.
- `start describe --global` and `start get --global` behave exactly as
  they do today (no change to the directory listing or module
  resolution).
- `describeScopeFromCmd` is removed; `scopeFromFlags` is the sole
  scope-derivation site.
- `TestGetLocalAndGlobalMutuallyExclusive` passes against the new cobra
  error format.
- `scripts/invoke-tests` passes.
