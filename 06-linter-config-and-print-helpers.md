# Add linter config and refactor print-with-discard pattern

## Goal

Install a linter configuration that catches discarded write errors, then
retire the repeated `_, _ = fmt.Fprintln(w, ...)` pattern that exists
across the CLI so write calls read cleanly instead of trailing the
`_, _ =` discard. The pattern appears 576 times across roughly 30 files
today; left as-is it dominates the visual texture of every command
implementation and tempts copy-paste mistakes when error handling
actually matters.

## Scope

In scope:

- Add a `.golangci.yml` (or equivalent) at the repo root that runs
  `errcheck` against the codebase with rules tuned so write-to-stdout
  calls funnel through helpers rather than being discarded inline.
- Wire the linter into `scripts/invoke-tests` (or add a sibling script)
  so CI catches future regressions.
- Add or extend print helpers in `internal/cli/output.go` (and an
  equivalent helper for `internal/orchestration` and `internal/tui`
  where the same pattern appears) that absorb the discard, so callers
  write `out.Println(w, ...)` or similar instead of
  `_, _ = fmt.Fprintln(w, ...)`.
- Refactor every existing `_, _ = fmt.Fprint*` call in production code
  under `cmd/` and `internal/` to use the new helpers.
- Update unit tests where they assert on output to confirm the
  refactor preserves byte-for-byte behaviour.

Out of scope:

- Refactoring `_ = root.Help()` and similar error-ignoring patterns
  that are not print calls.
- Adding new linters beyond what is needed for the discard pattern
  (e.g. no `gosec`, `revive`, `staticcheck` rule expansion in this
  project). The linter config is minimal and focused.
- Restructuring command output, colour usage, or the `tui` package's
  public API.
- Library or homebrew-tap changes.
- Touching test files purely to swap the print pattern — refactor test
  files only where the production change forces it.

## Current State

Production code uses the `_, _ = fmt.Fprintln(w, ...)` /
`_, _ = fmt.Fprintf(w, ...)` / `_, _ = fmt.Fprint(w, ...)` pattern in
576 sites across roughly 30 files. Representative files:

- `internal/cli/start.go` — heavy user (interactive prompts, status
  output, prompt preview).
- `internal/cli/output.go` — already hosts `printWarning`,
  `printHeader`, `printSeparator`, `printContextTable`. These helpers
  themselves contain the discard pattern internally, but absorb it
  away from their callers.
- `internal/cli/config_*.go` — every config subcommand writes to its
  `cmd.OutOrStdout()` via the discard pattern.
- `internal/cli/describe.go`, `internal/cli/get.go`,
  `internal/cli/task.go`, `internal/cli/modules_*.go` — same pattern.
- `internal/orchestration/autosetup.go` — eleven sites on
  `a.stdout`.
- `internal/doctor/reporter.go` — twelve sites on the report writer.
- `internal/tui/tui.go` — two sites inside `progress` formatting.
- `cmd/start/main.go` — one site writing the top-level error to
  `os.Stderr`.

Helpers already present in `internal/cli/output.go`:

- `writeJSON(w io.Writer, v any) error` — returns the error.
- `printWarning(w io.Writer, format string, args ...any)`.
- `printHeader(w io.Writer, text string)`.
- `printSeparator(w io.Writer)`.
- `printContextTable(...)` and other table helpers.

The colour-aware helpers in `internal/tui` (`ColorWarning`,
`ColorHeader`, `ColorSeparator`, `ColorAgents`, `ColorContexts`,
`ColorDim`, `ColorSuccess`, `ColorTasks`) all expose `Fprint`,
`Fprintf`, `Fprintln` methods that also return `(int, error)` and are
currently discarded inline. The refactor must cover the
`_, _ = tui.ColorX.Fprint*` variant as well as the bare `fmt.Fprint*`
variant.

There is no `.golangci.yml`, `.golangci.toml`, or any linter config at
the repo root today. `scripts/invoke-tests` runs `go test` only — no
`go vet`, no `errcheck`.

Go version: 1.25.0 (`go.mod`). Module path:
`github.com/start-cli/start`.

## References

- Project writing guide: `~/.ai/docs/project-writing-guide.md`
- `start/AGENTS.md` for build, test invocation, and project conventions.
- golangci-lint documentation: https://golangci-lint.run/
- `errcheck` linter (the closest fit for the discard pattern):
  https://github.com/kisielk/errcheck

## Requirements

1. A linter configuration file exists at the repo root and configures
   `errcheck` (or an equivalent rule from the chosen linter) so that
   discarded `Fprint*` errors fail the check.
2. The linter is invokable via a single command from `scripts/`
   (either folded into `invoke-tests` or as `scripts/invoke-lint` —
   the implementer chooses).
3. The number of `_, _ = fmt.Fprint*` and `_, _ = tui.Color*.Fprint*`
   sites in production code under `cmd/` and `internal/` is reduced
   to zero, with the exception of the helper bodies themselves.
4. New print helpers exist in `internal/cli/output.go` (and any
   sibling package where the pattern occurs — `internal/orchestration`,
   `internal/doctor`, `internal/tui`) that absorb the write-error
   discard. Callers write a single function call, not a `_, _ =` line.
5. Helpers compose with the colour API in `internal/tui` so callers
   that want coloured output do not regress to inlining
   `tui.ColorX.Fprint*` with a discard.
6. Existing unit-test assertions on command output continue to pass.
   The refactor produces byte-identical output for every command.
7. The linter is clean: `errcheck` (or chosen rule) reports zero
   issues after the refactor.
8. `go test ./...` and `go vet ./...` pass.

## Implementation Plan

1. Pick the linter tool. Default to `golangci-lint` with `errcheck`
   enabled and tuned to flag `(*os.File).Write`, `fmt.Fprint`,
   `fmt.Fprintf`, `fmt.Fprintln`, and the `fatih/color` `Fprint*`
   methods. Alternatives (standalone `errcheck`, `staticcheck`) are
   acceptable if the implementer prefers; document the choice in the
   config file header.
2. Add the linter config at the repo root. Configure it to scan
   `./cmd/...` and `./internal/...`. Exclude `_test.go` files from
   the discard-error rule (tests legitimately ignore writes to test
   buffers).
3. Wire the linter into `scripts/invoke-tests` (or
   `scripts/invoke-lint`). Run it before `go test` so a clean lint
   gates the test run; failing lint exits non-zero with a clear
   message. Match the existing script's bash style (set -o
   nounset/pipefail, declared functions, etc.).
4. Design the helper API. Sketch options:
   - A `Writer` wrapper type in `internal/cli/output.go` exposing
     `Println`, `Printf`, `Print` methods that absorb the error.
   - Free functions `output.Println(w, ...)`, `output.Printf(w, ...)`,
     `output.Print(w, ...)` in the same file.
   - A package-level `Out` value bound to `os.Stdout` at init, with
     methods that delegate to an injected writer.
   The implementer picks the shape that minimises churn at call
   sites while staying explicit about the target writer.
5. Mirror the helper for coloured output. Either add `output.Println`
   variants that accept an optional `*color.Color`, or extend
   `internal/tui` with discard-absorbing `Println`/`Printf`/`Print`
   methods on the existing `Color*` values.
6. Refactor in batches by package. Suggested order:
   - `internal/orchestration/autosetup.go` (11 sites).
   - `internal/doctor/reporter.go` (12 sites).
   - `internal/tui/tui.go` (2 sites).
   - `internal/cli/output.go` (the helpers themselves move to the
     new pattern).
   - `internal/cli/` — remaining files alphabetically.
   - `cmd/start/main.go`.
   After each batch, run `go build ./...`, `go vet ./...`, and the
   relevant package tests. Don't move to the next batch until the
   current one is clean.
7. Run `go test ./...` and the linter. Both must be clean before the
   project is complete.
8. Manually smoke-test the common commands to confirm output is
   visually identical: `start`, `start describe`, `start config list`,
   `start doctor`, `start modules list`, and one task invocation.

## Constraints

- Go 1.25, module `github.com/start-cli/start`.
- The linter must run in CI (folded into `scripts/invoke-tests` or
  added as a peer script that the existing test workflow invokes).
- No new external dependencies in `go.mod` unless the chosen helper
  approach genuinely needs one. The standard library is sufficient
  for the helper API.
- The refactor is mechanical and must not alter output bytes. Every
  test that asserts on command output continues to pass without
  assertion changes.
- Do not refactor `_ = root.Help()`, `_ = cmd.Usage()`, or other
  non-print error discards. They are out of scope and the linter
  rule must allow them (either via the rule's default exclusions or
  an explicit allowlist in the config).
- The helper functions absorb the write error silently. Do not
  redirect it to a logger, return it, or panic — the existing code
  already discards it, and changing that behaviour is out of scope.
- Do not modify any file under `library/` or `homebrew-tap/`.

## Implementation Guidance

- The refactor is repetitive and high-volume. Use `gofmt`, `goimports`,
  and editor multi-cursor or `sed` carefully; verify each batch
  compiles before moving on. A single missed pattern means the linter
  fails and the whole batch needs revisiting.
- Resist the temptation to fold in unrelated cleanups (renames,
  rewording log messages, restructuring helpers). The diff is already
  large; keeping it mechanical makes review tractable.
- If the helper signature decision drifts during implementation,
  decide once and stick with it. Mixing `output.Println(w, ...)` in
  some files and `out.Println(...)` in others is worse than either
  choice consistently applied.
- Test files retain the `_, _ = fmt.Fprint*` pattern unless the
  refactor of production code forces a test change. Test
  refactoring is a separate, optional cleanup.
- The linter is the durable enforcement mechanism. The refactor is a
  one-time cleanup. Without the linter, the pattern returns within a
  few PRs.

## Acceptance Criteria

- A linter config file exists at the repo root and is referenced from
  `scripts/invoke-tests` (or `scripts/invoke-lint`).
- Running the linter against the codebase reports zero issues.
- `rg "_, _ = (fmt\.Fprint|.*\.Fprint)" cmd/ internal/` returns only
  helper-body sites (the discard-absorbing helpers themselves), and
  nothing in callers.
- `go test ./...` and `go vet ./...` pass.
- `scripts/invoke-tests` runs the linter and the tests, exiting
  non-zero on linter failure.
- Manual smoke test of `start`, `start describe`, `start config list`,
  `start doctor`, and `start modules list` shows visually identical
  output before and after the refactor.
- No changes under `library/` or `homebrew-tap/`.
