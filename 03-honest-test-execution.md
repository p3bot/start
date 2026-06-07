# Project: Make test execution honest and unmaskable

## Goal

The Go test suite currently reports `ok` while tests are failing. A test that reaches the real agent-execution path triggers `syscall.Exec`, which replaces the running test process and forces exit code 0, discarding the testing framework's failure tally and every result after it. Make it structurally impossible for a test to replace the process or to mask a failure, then surface and fix the real failures that masking has been hiding.

## Scope

In scope:

- Introduce an injectable execution seam so no test ever replaces its own process via `syscall.Exec`.
- Add a regression guard so a future test that reaches the process-replacing path fails loudly instead of exiting 0.
- Standardise test environment isolation through one shared setup helper.
- Fix the real test failures that the mask has been hiding (the stale address-format assertions).
- Audit the full suite once it runs honestly and resolve every test-side failure surfaced.

Out of scope:

- The module-resolution behaviour bug. The test that exposes it (`TestTaskResolution_ExactMatchFallsThrough`) asserts correct post-fix behaviour; this project defers it to project 04 via a tracked skip and does not change resolution logic.
- The golangci-lint linter curation (GitHub issue 8), including the test-correctness linters.
- The production runtime behaviour: real `start` invocations must still replace the process with the agent via `syscall.Exec`. Only tests are affected.

## Current State

The orchestration executor replaces the process during normal execution. `Executor.ExecuteCommand` in `internal/orchestration/executor.go` ends with a direct `syscall.Exec(shell, args, env)` call. `Execute` builds the command and delegates to `ExecuteCommand`. A sibling `ExecuteWithoutReplace` runs the command via `exec.Command` and captures output, but it is not on the interactive launch path.

The CLI dispatches to execution in `executeStart` (`internal/cli/start.go`) and `executeTask` (`internal/cli/task.go`). Both branch on `flags.DryRun`: dry-run writes preview files and returns; otherwise they call `env.Executor.ExecuteCommand`. Most command tests use dry-run or never reach execution, so they are unaffected. A test that reaches a non-dry-run execution path with a runnable agent triggers `syscall.Exec`.

The masking mechanism, confirmed by a single verbose run of `go test -count=1 -v ./internal/cli`:

- 562 tests are declared; only 382 report a PASS/FAIL result.
- The last line is `=== RUN TestTaskResolution_ExactMatchFallsThrough`, followed by `test` (the output of the agent command `echo test`), followed by the package summary `ok`. That test never prints a result line.
- The same run prints `--- FAIL: TestConfigRole_FullWorkflow` and `--- FAIL: TestConfigContext_FullWorkflow` above, yet `go test` exits 0 and the package reports `ok`.

The cause: `TestTaskResolution_ExactMatchFallsThrough` (`internal/cli/start_test.go`) configures an `echo` agent whose command is `echo test`, then calls `executeTask` expecting an ambiguity error. Resolution instead reaches `ExecuteCommand`, which runs `echo test` via `syscall.Exec`. The test process is overwritten by the shell, prints `test`, and exits 0. Every failure already printed to stdout is left standing while the process exit code becomes 0, so the test runner reports success. Tests ordered after the replacement never run.

Two real failures are hidden behind the mask. Both are stale assertions left over from the address-scheme migration that moved module display from `category/name` to `category:name`:

- `TestConfigRole_FullWorkflow/get_role_details` (`internal/cli/config_integration_test.go`) asserts the output contains `roles/reviewer`. The current and correct output is `roles:reviewer` (see `printRoleGet` in `internal/cli/config_get.go` and `formatAddress` in `internal/cli/describe.go`).
- `TestConfigContext_FullWorkflow/get_context_details` asserts `contexts/project`; the correct output is `contexts:project`.

Both tests also under-isolate. They set only `XDG_CONFIG_HOME` and call `chdir`, unlike `setupStartTestConfig` (`internal/cli/start_test.go`), which isolates `HOME`, `XDG_CONFIG_HOME`, and `XDG_CACHE_HOME`. `setupStartTestConfig` bundles that isolation with a full seed config, so it is not a drop-in for these from-scratch workflow tests; only its isolation half is shared (see requirement 3). They fail deterministically when run alone; they appeared green only because the mask suppressed the package exit code.

A seam pattern already exists in the codebase. The registry client is injected through a per-command provider stored on the command context (`WithProvider`/`getProvider`, exercised in `internal/cli/seam_coverage_test.go` and `internal/cli/json_capture_test.go`). It demonstrates the AGENTS.md principle of injecting dependencies through construction or context rather than reaching for globals. It is not the model for the execution seam, however: that provider rides on the cobra command context, which the execution path does not carry (see Implementation Guidance). The execution seam applies the same inject-not-globals principle through construction-time injection instead.

## Requirements

1. Add an execution seam to `Executor` so process replacement is an injectable dependency. Give `Executor` a process-replacement function field, populated by `NewExecutor` from an unexported package-level default that performs the current `syscall.Exec` behaviour. Production compiles only this default, so real behaviour is unchanged. The default is swappable in test builds via `TestMain` (requirement 2), never through a runtime environment check. After this change, no test path calls `syscall.Exec`.

2. Add a regression guard so the masking class of bug cannot recur silently. In the test builds of the packages that can reach execution (`internal/cli` and `internal/orchestration`), the default replacer must fail the test loudly rather than replace the process or exit 0. Install it in each package's `TestMain`: `internal/orchestration` swaps its own package default, and `internal/cli` installs the guard into orchestration's seam through an exported setter — because `TestMain` runs only for the package under test, the `cli` test binary compiles `internal/orchestration` in non-test mode with its production default otherwise intact. The guard is the deliverable of this requirement. Once requirement 5 skips the one execution-reaching test, no remaining test exercises real execution, so a non-replacing recorder that captures the command and returns is added only if the requirement 6 audit surfaces a test that must execute and return.

3. Standardise environment isolation. Extract a shared isolation-only helper that sets `HOME`, `XDG_CONFIG_HOME`, and `XDG_CACHE_HOME` together and registers the read-only-cache cleanup, without writing any seed config. Route the config-workflow tests, and any other test found setting only a subset of the isolation variables, through it, and have `setupStartTestConfig` compose the same helper so a single definition of isolation is shared by both the from-scratch and the seeded tests. A test must produce the same result run alone as it does in the full suite.

4. Correct the two stale assertions to the current `category:name` address scheme so `get_role_details` and `get_context_details` pass in isolation and in the suite.

5. Defer the resolution-behaviour test. `TestTaskResolution_ExactMatchFallsThrough` asserts the correct hit-model behaviour that project 04 will implement. With the seam in place it will fail honestly (resolution executes instead of returning an ambiguity error). Mark it skipped with a comment naming project 04 as the owner. Do not change resolution logic to make it pass. Project 04 removes the skip.

6. Audit the unmasked suite. With the seam and guard in place, run the full unit suite with `-count=1` and `-shuffle=on`, and run the build-tagged suites (`integration`, `e2e`, `registry`). Resolve every failure surfaced: test-side defects are fixed in this project; the single resolution-behaviour failure is the one deferred via requirement 5.

## Constraints

- Production behaviour is unchanged. A real interactive `start` or `start task` invocation must still replace the process with the agent via `syscall.Exec`. The seam alters only what tests run.
- Do not modify module-resolution logic. This project only defers the test that depends on the project 04 fix.
- Use injection, not a package-level global toggled by an environment check. Supply the replacer through construction (`NewExecutor`), defaulting to production behaviour, and swap the default in test builds via `TestMain` — never via a runtime environment check.
- Keep `ExecuteWithoutReplace` available; the new seam is the mechanism for the interactive (replacing) path, not a replacement for the output-capturing path.
- Follow the repository testing approach (AGENTS.md): real CUE validation, real files via `t.TempDir()`, table-driven cases, dependencies passed as parameters or interfaces.

## Implementation Plan

1. Add the execution seam to `Executor` in `internal/orchestration/executor.go`. Give the executor a process-replacement function field, populated by `NewExecutor` from an unexported package-level default that wraps the current `syscall.Exec` call, and route `ExecuteCommand` through the field. Export a setter so a sibling package's `TestMain` can swap the default. The directory change and shell lookup stay as they are; only the final replacement call goes through the seam.

2. Install the guard in both execution-reaching packages. Add a `TestMain` to `internal/orchestration` that swaps the package default to a guard that fails loudly, and a `TestMain` to `internal/cli` that calls orchestration's exported setter to install the same guard (the `cli` test binary does not run orchestration's `TestMain`). Defer the recorder implementation until the requirement 6 audit confirms a test must execute and return; if none does, the guard alone satisfies the seam.

3. Apply the requirement 5 skip to `TestTaskResolution_ExactMatchFallsThrough`. If the audit surfaces another test that must execute and return, migrate it onto a recorder seam; otherwise the guard alone covers the execution path.

4. Standardise isolation. Extract the env-var isolation and read-only-cache cleanup from `setupStartTestConfig` into a shared isolation-only helper, have `setupStartTestConfig` call it, and move the config-workflow tests in `internal/cli/config_integration_test.go` onto that helper. Apply it to any other partially-isolated test found during the audit.

5. Fix the two stale assertions to the `category:name` form.

6. Run the audit (`-count=1`, `-shuffle=on`, and each tagged suite) and resolve every surfaced test-side failure.

## Implementation Guidance

- Do not mirror the registry-client provider seam (`WithProvider`/`getProvider`) for execution. That seam rides on the cobra command context, but the execution path (`executeStart`/`executeTask` → `buildExecutionEnv` → `NewExecutor`) carries no context, and the execution-reaching tests call `executeTask` directly, bypassing cobra. Inject at construction instead: `NewExecutor` reads an unexported package default that `TestMain` swaps in test builds. This is construction-time injection, not an environment-toggled global, so it satisfies the injection constraint without threading a context through three signatures and every direct test call site.
- The blast radius of tests that reach real execution is expected to be small, because dry-run is the normal test path. The audit confirms the exact set; do not assume it is only the one known test until the suite runs honestly end to end.
- The address scheme is `category:name`. The slash form in the two failing assertions predates that migration and is the defect, not the code.

## Acceptance Criteria

1. A full verbose run of the unit suite prints a PASS, FAIL, or SKIP result line for every `=== RUN` line. The number of reported results equals the number of declared tests; no test silently drops out.
2. Running the `internal/cli` tests via the compiled test binary directly and via `go test -count=1 ./internal/cli` produces identical pass/fail results.
3. Introducing a deliberate failing assertion in any `internal/cli` test causes `go test ./internal/cli` to exit nonzero. Removing it restores a clean run.
4. A test that reaches the execution path under the default (guard) replacer fails loudly; it does not replace the process and does not yield exit code 0.
5. `get_role_details` asserts `roles:reviewer` and `get_context_details` asserts `contexts:project`; both pass run in isolation and in the full suite.
6. `go test -count=1 -shuffle=on ./...` reports a result for every test, and the `integration`, `e2e`, and `registry` tagged suites do the same. The only deferred test is `TestTaskResolution_ExactMatchFallsThrough`, reported as skipped with a comment naming project 04.
7. A real (non-dry-run) `start` invocation still replaces the process with the agent command, confirming the seam left production behaviour intact.
