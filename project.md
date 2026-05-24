# Project: CLI Contract Alignment

When implementing, bias toward the principled long-term solution that reduces maintenance and improves quality. Do not default to the smallest-diff resolution.

## Goal

Align the `start` CLI's machine-facing contract with the agent-CLI design guides. Replace the `--no-color` negation flag with a `--color` enum, rename the `config remove` confirmation bypass to the canonical `--force`, introduce semantic exit codes, and document the `--json`/exit-code contract through a new `help schemas` topic. The result is a CLI an agent can drive predictably: it can tell a fixable mistake from a retryable failure from a broken config, and it can read the output contract without trial and error.

Backward compatibility is not a goal. The removed flags (`--yes`/`-y`, `--no-color`) are deleted outright, not aliased — a clean break, not a deprecation shim.

## Scope

In scope:
- Rename the `config remove` confirmation bypass from `-y`/`--yes` to `--force`. `--yes`/`-y` are removed. The `remove` command name is unchanged.
- Replace the root `--no-color` boolean with a `--color=auto|always|never` enum. `--no-color` is removed.
- Introduce a semantic exit-code taxonomy and apply it across the existing error paths.
- Add a `start help schemas` topic (alias `schema`) documenting the `--json` output contract, the output shape of each `--json`-capable command, and the exit-code table.

Out of scope, by deliberate decision:
- Wrapping `--json` output in a `{"status":...,"data":...}` envelope. The documented-shapes-plus-exit-codes contract replaces it; do not introduce an envelope.
- Adding `--json` to commands that lack it. `get`, `prompt`, `task`, `describe`, and `install` are content or launch commands where JSON adds nothing.
- `--no-input`, `--fields`, `--limit`/`--next`, and `--dry-run` on `config` mutations. Low value for an interactive-first orchestrator over a small curated module set.
- Renaming `describe` or `doctor`. Both are kept against guide vocabulary on precedent (`kubectl describe` vs `get`; `brew doctor`/`flutter doctor`, which also hosts `doctor validate` well).
- Renaming `config add`/`edit`/`remove` to canonical `create`/`update`/`delete`. These are interactive wizards — they require a TTY and refuse to run otherwise. The canonical CRUD verbs carry an agent-facing contract (idempotency, `--dry-run`, `CONFLICT`/exit 5 on a differing create, non-interactive operation) that these commands neither satisfy nor should. `add`/`edit`/`remove` are the honest names for interactive human commands and stay as-is. Only the `remove` bypass flag changes (see In scope).

## Current State

- `cmd/start/main.go` calls `cli.Execute()` and does `os.Exit(1)` on every non-nil error, gated only by `IsSilentError`. There is no exit-code taxonomy and no `ExitError` type.
- `internal/cli/root.go` builds the command tree via the `NewRootCmd()` factory and registers a persistent `--no-color` boolean that sets `color.NoColor` in `PersistentPreRunE`.
- Nine commands support `--json`: `list`, `library`, `search`, `update`, `doctor`, `doctor validate`, `config get`, `config list`, `config settings`. Each emits its own bare shape (arrays or objects, no shared wrapper). On error, `main.go` prints to stderr and exits 1. Whether each of the nine leaves stdout free of partial JSON when it errors mid-render is not yet verified.
- `config` mutation subcommands are `config add`, `config edit`, `config remove`. `config remove` registers `-y`/`--yes` to skip its confirmation prompt.
- The `help` parent hosts embedded-markdown topics `agents`, `config`, and `templates`. There is no `schemas` topic.
- Errors originate predictably by package: `internal/registry/*` for index/module network I/O; `internal/cue/loader.go` and `internal/cue/validator.go` for user-config load and validation; `internal/cli/resolve.go`, `internal/cli/cross_resolve.go`, `internal/cli/config_helpers.go`, `internal/modules/install.go`, and `internal/orchestration/executor.go` for resolution and not-found conditions.

## References

- `~/.ai/docs/cli-design-for-agents.md` — the agent-CLI rule set (P0/P1/P2). Rule 4 (exit codes), Rule 9 (canonical vocabulary), Rule 10 (three-layer introspection) drive this work.
- `~/.ai/docs/golang-cli-design-guide.md` — Go implementation patterns: `ExitError`/`ExitCodeFromError`, the exit-code constants, `--color` handling, the embed-and-register help-topic pattern.
- `~/.ai/docs/project-writing-guide.md` — structure for this document.

## Requirements

1. `config remove` uses `--force` to bypass the confirmation prompt. `--yes` and `-y` are removed. The `remove` command name and its interactive behaviour are unchanged.
2. The root command exposes `--color` accepting `auto` (default), `always`, and `never`. `auto` enables colour only when stdout is a TTY. Precedence follows the golang-cli-design guide: `NO_COLOR` (set to any value) disables colour and wins even over `--color=always`; `TERM=dumb` disables colour; `FORCE_COLOR`/`CLICOLOR_FORCE` force colour on when stdout is not a TTY. An invalid `--color` value is a usage error (exit 2). `--no-color` is removed; its former behaviour is reached via `--color=never` or `NO_COLOR`.
3. An `ExitError{Code int}` type and an `ExitCodeFromError` mapper exist; `main.go` derives the process exit code from the returned error instead of always exiting 1.
4. Error paths carry the semantic exit codes in the Exit-Code Mapping table below. Classification is by fault domain — caller usage error, missing resource, transient failure, invalid user config, or internal bug — decided from the error type at one boundary, not at every call site. A whole package does not map to one code: `internal/cue` yields both invalid-user-CUE (78) and internal-manipulation bugs (1), and `internal/registry` yields both transient network I/O (75) and permanent setup/path failures (1/2/3). Where a single error value today conflates domains (the registry package; `task.go`'s combined not-found / registry-unavailable error), split it at the source into distinct typed or sentinel errors so the boundary maps each correctly; do not guess from the message.
5. Registry failures distinguish transient from permanent: network/index/fetch failures exit 75; a malformed module-path string exits 2; a module or version that does not exist exits 3. A registry outage must never present as a permanent error, and a typo'd module name must never present as transient. The distinction is carried in the error value as typed data (a typed error or a sentinel the mapper matches on), produced where the failure is known — at the registry boundary — not re-derived downstream by inspecting messages or status codes.
6. `start help schemas` (alias `start help schema`) prints a token-efficient reference covering: the `--json` contract (success emits the documented shape on stdout with exit 0; failure emits a message on stderr, leaves stdout empty, and returns a non-zero semantic exit code), the output shape of each of the nine `--json`-capable commands, and the exit-code table. The documented shapes are guarded against drift by a test that runs each of the nine commands under `--json` and asserts its real output matches what the topic documents, so the reference cannot silently diverge from behaviour.

## Constraints

- Pure Go, gofmt-clean, `golangci-lint` clean. `scripts/invoke-tests` (lint + tests) must pass.
- This is a breaking change by design. The removed flags (`--yes`/`-y` on `config remove`, the root `--no-color`) are deleted entirely — no aliases, no deprecation shims. Remove every internal reference to them, including help text, examples, completion, and the `AGENTS.md` command table.
- Follow the repo's established Cobra patterns: one file per command, command construction through the `NewRootCmd()` factory, `RunE` returning errors (never `os.Exit` outside `main`).
- Exit-code values are fixed to the taxonomy: 0 success, 1 general, 2 usage, 3 not-found, 4 permission, 5 conflict, 75 transient, 78 config. Define them as named constants.
- Classification is exhaustive over the packages named in the Exit-Code Mapping (`registry`, `cue`, `cli` resolution/validation, `modules`, `orchestration`): every user-reachable error from these resolves to a deliberate code. Exit 1 is a conscious "general/internal" classification, not the residue of paths left untouched. The work is not done while any of these paths returns 1 only because it was never examined.
- Exit-code classification keys off error type — sentinel errors and typed errors matched with `errors.Is`/`errors.As`. It must never branch on the text of an error message (no `strings.Contains(err.Error(), ...)`); message wording is not part of any contract and matching it is brittle. Where a needed distinction is not yet expressed in the error type, add the sentinel or typed error at the source rather than pattern-matching the string at the boundary.
- `help schemas` content is agent-facing: follow the token-efficient markdown conventions already used by the other `help` topics (no bold, no emoji, headings no deeper than `###`, single blank lines, tables and inline code allowed).

## Exit-Code Mapping

| Code | Meaning | Applies to |
| ---- | ------- | ---------- |
| 75 | Transient | Registry network I/O a retry could clear: index resolve/fetch (`internal/registry/index.go`), module fetch and source resolution after retries (`internal/registry/client.go`); the registry-unavailable cause in `task.go`. Excludes registry client construction and "source location does not provide OS path" (internal → 1) and a bad module path/version (→ 2/3) |
| 78 | Config | The user's own CUE is unloadable or invalid: instance load/build (`inst.Err` in `internal/cue/loader.go`), schema validation (`internal/cue/validator.go`), "is not a directory". Excludes internal merge/iterate/format failures in `loader.go`, which are our bugs (→ 1) |
| 3 | Not found | `cross_resolve.go`, `resolve.go`, `get.go`, `config_helpers.go`, `modules/install.go` "not found" conditions; agent config absent (`executor.go`); registry search yielding zero matches (`install.go` `errNoModules`); the not-found cause in `task.go`; a registry module or version that does not exist |
| 2 | Usage | Cobra arg-count and flag-parse failures; `--local`/`--global` mutual-exclusion violations; explicit input validation (three-character query minimum, `config settings` key/value checks, `doctor validate` index-path check, task category check); "interactive X requires a terminal" on the `config` mutation commands; a malformed registry module-path string; an invalid `--color` value |
| 5 | Conflict | No current call site; constant defined for taxonomy completeness (see Implementation Guidance) |
| 1 | General | Cobra unknown-command (left at 1 to match `git`/`gh`); internal-invariant failures (registry client construction and "no OS path", CUE merge/iterate/format); anything not otherwise classified |

Judgment calls resolved:
- Agent binary missing on PATH exits 78 (environment the user must fix), not 3.
- "No config found yet" / `ErrNoCUEFiles` is a normal first-run state, not a config error. Commands treat it as an empty result (exit 0) or a friendly general failure, never 78.

## Implementation Plan

1. Add the exit-code infrastructure: named exit-code constants, the `ExitError{Code int}` type with `Error`/`Unwrap`, and an `ExitCodeFromError(err) int` mapper. Wire `main.go` to call it. Default unclassified errors to 1, preserving current behaviour until classification lands.
2. Classify errors by fault domain. Introduce or reuse sentinel/typed errors so the mapper can branch with `errors.Is`/`errors.As`: a typed registry error distinguishing transient network I/O from permanent failures; user-CUE load and validation errors mapped to 78 while internal merge/iterate/format failures stay at 1; not-found sentinels mapped to 3; the `task.go` not-found / registry-unavailable error split into its two domains. Map Cobra usage failures and explicit input-validation errors to 2. Apply the Exit-Code Mapping table.
3. Handle the registry transient/permanent split at the registry boundary (Requirement 5) — the most involved of the source-level splits, since the package conflates retryable network I/O with bad-input and internal failures. Apply the same split-at-source treatment to `task.go`'s not-found / registry-unavailable error and to `cue`'s user-fault vs internal errors per Requirement 4.
4. Rename the `config remove` bypass flag to `--force`; remove `--yes`/`-y`. The command name and its interactive prompt are unchanged.
5. Replace `--no-color` with the `--color` enum: parse and validate the value, and drive `color.NoColor` from it under `auto` TTY detection. Remove the `--no-color` flag.
6. Add the `schemas` help topic: author the markdown, embed it, register the `schemas` command (alias `schema`) under the `help` parent following the existing topic pattern. Capture the documented per-command shapes from real `--json` output rather than hand-writing them.
7. Update tests for the renamed flag and the exit-code mapping (assert representative paths: registry-unreachable, bad module name, malformed user CUE, missing required arg, missing module), plus the `help schemas` drift guard (each of the nine commands' `--json` output matches its documented shape; reuse the existing snapshot harness in `snapshots_test.go`). Update the `AGENTS.md` command table and cross-reference `help schemas` from `help agents`.

## Implementation Guidance

- Classify exit codes by error type in the single `ExitCodeFromError` mapper, not by editing each `fmt.Errorf` site. The packages already segregate concerns, so type/sentinel-based branching covers most paths with few touch points.
- Exit 75 is the agent's retry signal; keep it strictly for failures that a retry could plausibly clear. Never let a permanent condition (bad name, missing resource, malformed input) reach it.
- `main.go` already routes error text to stderr, so the error half of the `--json` contract is partly in place. Before relying on it, verify each of the nine `--json` commands writes nothing to stdout when it errors — including failures that occur after rendering has begun — and fix any that emit a partial object or array. The settled contract is then: success → documented shape on stdout + exit 0; failure → stderr message + empty stdout + non-zero exit.
- Exit 5 has no current call site. Define the constant for taxonomy completeness but leave it unwired. A duplicate-name `config add` is the natural future home if that command is ever made to reject a conflicting existing item rather than proceed.
- Resolve colour to a single state in one place, computed from `--color`, `NO_COLOR`, and (for `auto`) stdout TTY detection — apply the precedence in Requirement 2. Do not reintroduce the removed `--no-color` boolean as a parallel input, and do not regress the `NO_COLOR` handling `fatih/color` currently provides. After resolution the rest of the code reads one settled value (the existing `color.NoColor` assignment).

## Acceptance Criteria

- `start config remove <name> --force` skips confirmation; `--yes` and `-y` are rejected as unknown flags. `config add`/`edit`/`remove` otherwise behave as before.
- `start --color=never`, `=always`, and `=auto` behave correctly; an invalid `--color` value exits 2; `--no-color` is rejected as an unknown flag.
- A registry-unreachable `install`/`update`/`search` exits 75; a typo'd module name exits 3; a malformed module-path string exits 2; malformed user CUE exits 78; a missing required argument or sub-minimum query exits 2; a missing installed module exits 3; a missing agent binary exits 78.
- `start help schemas` and `start help schema` print the `--json` contract, the nine commands' output shapes, and the exit-code table. A test asserts each command's real `--json` output matches its documented shape.
- Under `--json`, every error path on the nine commands writes nothing to stdout (verified by test), including failures mid-render.
- `scripts/invoke-tests` passes.
