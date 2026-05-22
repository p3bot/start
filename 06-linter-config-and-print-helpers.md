# Add linter config and strip print-with-discard prefix

## Goal

Install a Go linter at the repo root that catches unchecked errors
generally, with the `Fprint*` family excluded so the historical
`_, _ = fmt.Fprintln(w, ...)` pattern can drop its discard prefix and
read as a bare stdlib call. The pattern appears at roughly 700 sites
across the CLI today; left as-is it dominates the visual texture of
every command implementation and tempts copy-paste mistakes when error
handling actually matters. The linter is the durable safety net for
unchecked errors elsewhere in the codebase; the strip is a one-time
cleanup that lets the print sites read normally.

## Scope

In scope:

- Add a `.golangci.yml` at the repo root that runs `errcheck` against
  the codebase, with `fmt.Fprint`, `fmt.Fprintf`, `fmt.Fprintln`, and
  the fatih/color `Fprint*` methods listed under `exclude-functions`
  so bare calls to those functions do not trip the linter.
- Wire the linter into `scripts/invoke-tests` (or add a sibling
  script) so the lint runs alongside `go test`.
- Strip the `_, _ = ` prefix from every `_, _ = fmt.Fprint*` and
  `_, _ = tui.Color*.Fprint*` site in production code under `cmd/` and
  `internal/`, leaving the bare call (e.g.
  `fmt.Fprintln(w, "text")`).
- Update unit tests where they assert on output to confirm the strip
  preserves byte-for-byte behaviour.

Out of scope:

- Refactoring `_ = root.Help()`, `_ = cmd.Usage()`,
  `_ = cmd.Flags().MarkHidden(...)`, or other explicit single-blank
  discards. errcheck's default behaviour ignores `_ = ...`
  assignments, so these stay as-is naturally.
- Adding new linters beyond `errcheck` (no `gosec`, `revive`,
  `staticcheck` rule expansion in this project). The linter config is
  minimal and focused.
- Adding new print helpers. The existing helpers
  (`printWarning`, `printHeader`, `printSeparator`,
  `printContextTable`, `printAgentModel`, `printRoleTable`, the
  `Progress` methods, etc.) keep their current shape; only their
  internal `_, _ = ` prefixes are stripped.
- Restructuring command output, colour usage, or the `tui` package's
  public API.
- Library or homebrew-tap changes.
- Touching test files purely to swap the print pattern — strip test
  files only where the production change forces it.

## Current State

Production code uses the `_, _ = fmt.Fprintln(w, ...)` /
`_, _ = fmt.Fprintf(w, ...)` / `_, _ = fmt.Fprint(w, ...)` pattern at
roughly 700 sites across 30 files under `cmd/` and `internal/`.
Representative files:

- `internal/cli/start.go` — heavy user (interactive prompts, status
  output, prompt preview).
- `internal/cli/output.go` — hosts `printWarning`, `printHeader`,
  `printSeparator`, `printContextTable`, `printAgentModel`,
  `printRoleTable`. These call `fmt.Fprint*` and
  `tui.Color*.Fprint*` internally with the discard prefix.
- `internal/cli/config_*.go` — every config subcommand writes to its
  `cmd.OutOrStdout()` via the discard pattern.
- `internal/cli/describe.go`, `internal/cli/get.go`,
  `internal/cli/task.go`, `internal/cli/modules_*.go` — same pattern.
- `internal/orchestration/autosetup.go` — multiple sites on
  `a.stdout`.
- `internal/doctor/reporter.go` — multiple sites on the report
  writer.
- `internal/tui/tui.go` — two sites inside `Progress.Update` and
  `Progress.Done`.
- `cmd/start/main.go` — one site writing the top-level error to
  `os.Stderr`.

The colour-aware helpers in `internal/tui` (`ColorWarning`,
`ColorHeader`, `ColorSeparator`, `ColorAgents`, `ColorContexts`,
`ColorDim`, `ColorSuccess`, `ColorTasks`, and the rest) all expose
`Fprint`, `Fprintf`, `Fprintln` methods that return `(int, error)`.
The strip applies to these the same way as to bare `fmt.Fprint*`
calls; the `.golangci.yml` `exclude-functions` list names the
fatih/color methods so the bare calls are not flagged.

There is no `.golangci.yml` or any linter config at the repo root
today. `scripts/invoke-tests` runs `go test` only — no `go vet`, no
`errcheck`.

Go version: 1.25.0 (`go.mod`). Module path:
`github.com/start-cli/start`. fatih/color is at v1.18.0.

## References

- Project writing guide: `~/.ai/docs/project-writing-guide.md`
- `start/AGENTS.md` for build, test invocation, and project
  conventions.
- golangci-lint documentation: https://golangci-lint.run/
- `errcheck` linter and its `exclude-functions` config:
  https://github.com/kisielk/errcheck

## Requirements

1. A `.golangci.yml` (or equivalent) exists at the repo root with
   `errcheck` enabled. The `exclude-functions` list includes
   `fmt.Fprint`, `fmt.Fprintf`, `fmt.Fprintln`, and the fatih/color
   `Fprint*` methods so bare calls to those functions do not trigger
   errcheck.
2. The linter is invokable via a single command from `scripts/`
   (folded into `invoke-tests` or as `scripts/invoke-lint` — the
   implementer chooses).
3. The number of `_, _ = fmt.Fprint*` and `_, _ = tui.Color*.Fprint*`
   sites in production code under `cmd/` and `internal/` is zero.
4. Existing unit-test assertions on command output continue to pass.
   The strip produces byte-identical output for every command.
5. The linter is clean: errcheck reports zero issues against the
   codebase after the strip.
6. `go test ./...` and `go vet ./...` pass.

## Implementation Plan

1. Add `.golangci.yml` at the repo root. Enable `errcheck`.
   Configure `linters-settings.errcheck.exclude-functions` to list:
   - `fmt.Fprint`
   - `fmt.Fprintf`
   - `fmt.Fprintln`
   - `(*github.com/fatih/color.Color).Fprint`
   - `(*github.com/fatih/color.Color).Fprintf`
   - `(*github.com/fatih/color.Color).Fprintln`

   Leave errcheck's `check-blank` flag at its default (false) so
   existing `_ = root.Help()`-style discards are not flagged.
2. Wire the linter into `scripts/invoke-tests` (or
   `scripts/invoke-lint`). Run it before `go test` so a clean lint
   gates the test run; failing lint exits non-zero with a clear
   message. Match the existing script's bash style (set -o
   nounset/pipefail, declared functions, etc.). If `golangci-lint`
   is not on the PATH, fail with a clear install hint.
3. Strip the `_, _ = ` prefix from every `Fprint*` site in
   production code. Suggested batch order:
   - `internal/orchestration/autosetup.go`
   - `internal/doctor/reporter.go`
   - `internal/tui/tui.go`
   - `internal/cli/output.go` (the existing helpers' internal
     `_, _ = ` prefixes strip too)
   - `internal/cli/` — remaining files alphabetically
   - `cmd/start/main.go`

   After each batch, run `go build ./...`, `go vet ./...`,
   `golangci-lint run ./...`, and the relevant package tests. Don't
   move to the next batch until the current one is clean.
4. Run `go test ./...` and the linter against the whole module.
   Both must be clean before the project is complete.
5. Manually smoke-test the common commands to confirm output is
   visually identical: `start`, `start describe`, `start config
   list`, `start doctor`, `start modules list`, and one task
   invocation.

## Constraints

- Go 1.25, module `github.com/start-cli/start`.
- The linter must run in `scripts/invoke-tests` (or in a peer
  script that the existing test workflow invokes).
- No new external dependencies in `go.mod`. The strip introduces no
  new code; the linter is invoked as an external binary.
- The strip is mechanical and must not alter output bytes. Every
  test that asserts on command output continues to pass without
  assertion changes.
- Do not modify any file under `library/` or `homebrew-tap/`.

## Implementation Guidance

- The strip is repetitive and high-volume. Use `sed` or editor
  multi-cursor carefully; verify each batch compiles and lints
  before moving on. A single missed `_, _ = ` means the strip is
  incomplete and the codebase is inconsistent.
- Resist the temptation to fold in unrelated cleanups (renames,
  rewording log messages, restructuring helpers). The diff is
  already large; keeping it mechanical makes review tractable.
- Test files retain any `_, _ = fmt.Fprint*` pattern they currently
  have unless the strip of production code forces a test change.
  Test refactoring is a separate, optional cleanup.
- The linter is the durable enforcement mechanism for unchecked
  errors elsewhere in the codebase. The strip is a one-time cleanup
  that lets the Fprint sites read normally now that the linter
  doesn't demand the discard.

## Acceptance Criteria

- `.golangci.yml` exists at the repo root with errcheck enabled and
  the `exclude-functions` list naming the print family.
- `scripts/invoke-tests` (or `scripts/invoke-lint`) runs the linter;
  it exits non-zero on lint failure.
- `golangci-lint run ./...` reports zero issues.
- `rg "_, _ = (fmt\.Fprint|.*\.Fprint)" cmd/ internal/` returns no
  matches.
- `go test ./...` and `go vet ./...` pass.
- Manual smoke test of `start`, `start describe`, `start config
  list`, `start doctor`, and `start modules list` shows visually
  identical output before and after the strip.
- No changes under `library/` or `homebrew-tap/`.
