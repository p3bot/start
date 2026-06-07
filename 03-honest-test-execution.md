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

Both tests also under-isolate. They set only `XDG_CONFIG_HOME` and call `chdir`, unlike the shared helper `setupStartTestConfig` (`internal/cli/start_test.go`), which isolates `HOME`, `XDG_CONFIG_HOME`, and `XDG_CACHE_HOME`. They fail deterministically when run alone; they appeared green only because the mask suppressed the package exit code.

A seam pattern already exists in the codebase and is the model for the execution seam. The registry client is injected through a per-command provider stored on the command context (`WithProvider`/`getProvider`, exercised in `internal/cli/seam_coverage_test.go` and `internal/cli/json_capture_test.go`). The principle, stated in AGENTS.md, is to inject dependencies through construction or context rather than reaching for globals.

## Requirements

1. Add an execution seam to `Executor` so process replacement is an injectable dependency. The production default performs the current `syscall.Exec` behaviour unchanged. Tests inject a non-replacing implementation that records the command and arguments and returns normally. After this change, no test path calls `syscall.Exec`.

2. Add a regression guard so the masking class of bug cannot recur silently. In the test builds of the packages that can reach execution (`internal/cli` and `internal/orchestration`), the default seam used when a test has not injected one must fail the test loudly rather than replace the process or exit 0. A test that reaches the execution path without injecting a recorder must produce a visible failure.

3. Standardise environment isolation. Route the config-workflow tests, and any other test found setting only a subset of the isolation variables, through one shared setup helper that isolates `HOME`, `XDG_CONFIG_HOME`, and `XDG_CACHE_HOME` together. A test must produce the same result run alone as it does in the full suite.

4. Correct the two stale assertions to the current `category:name` address scheme so `get_role_details` and `get_context_details` pass in isolation and in the suite.

5. Defer the resolution-behaviour test. `TestTaskResolution_ExactMatchFallsThrough` asserts the correct hit-model behaviour that project 04 will implement. With the seam in place it will fail honestly (resolution executes instead of returning an ambiguity error). Mark it skipped with a comment naming project 04 as the owner. Do not change resolution logic to make it pass. Project 04 removes the skip.

6. Audit the unmasked suite. With the seam and guard in place, run the full unit suite with `-count=1` and `-shuffle=on`, and run the build-tagged suites (`integration`, `e2e`, `registry`). Resolve every failure surfaced: test-side defects are fixed in this project; the single resolution-behaviour failure is the one deferred via requirement 5.

## Constraints

- Production behaviour is unchanged. A real interactive `start` or `start task` invocation must still replace the process with the agent via `syscall.Exec`. The seam alters only what tests run.
- Do not modify module-resolution logic. This project only defers the test that depends on the project 04 fix.
- Use injection, not a package-level global toggled by an environment check. The seam must follow the existing registry-provider philosophy: a dependency supplied through construction or context, defaulting to production behaviour.
- Keep `ExecuteWithoutReplace` available; the new seam is the mechanism for the interactive (replacing) path, not a replacement for the output-capturing path.
- Follow the repository testing approach (AGENTS.md): real CUE validation, real files via `t.TempDir()`, table-driven cases, dependencies passed as parameters or interfaces.

## Implementation Plan

1. Add the execution seam to `Executor` in `internal/orchestration/executor.go`. Give the executor an injectable process-replacement dependency that defaults to the current `syscall.Exec` behaviour, and route `ExecuteCommand` through it. The directory change and shell lookup stay as they are; only the final replacement call goes through the seam.

2. Provide a test seam and guard for the packages that can reach execution. Supply a recorder implementation that captures the command and returns, for tests that legitimately exercise the execution path. Make the default-in-tests behaviour a guard that fails loudly, so a test reaching execution without injecting fails visibly instead of replacing the process. A `TestMain` is an acceptable place to install the guard if a per-test default is not cleaner.

3. Migrate `TestTaskResolution_ExactMatchFallsThrough` and any other execution-reaching test onto the recorder seam, then apply the requirement 5 skip to the resolution assertion.

4. Standardise isolation. Move the config-workflow tests in `internal/cli/config_integration_test.go` onto the shared full-isolation helper, and apply it to any other partially-isolated test found during the audit.

5. Fix the two stale assertions to the `category:name` form.

6. Run the audit (`-count=1`, `-shuffle=on`, and each tagged suite) and resolve every surfaced test-side failure.

## Implementation Guidance

- The registry-client provider seam (`WithProvider`/`getProvider`, used in `seam_coverage_test.go` and `json_capture_test.go`) is the reference for injecting a dependency that defaults to production and is overridden in tests. Mirror its shape for the execution seam.
- The blast radius of tests that reach real execution is expected to be small, because dry-run is the normal test path. The audit confirms the exact set; do not assume it is only the one known test until the suite runs honestly end to end.
- The address scheme is `category:name`. The slash form in the two failing assertions predates that migration and is the defect, not the code.

## Acceptance Criteria

1. A full verbose run of the unit suite prints a PASS, FAIL, or SKIP result line for every `=== RUN` line. The number of reported results equals the number of declared tests; no test silently drops out.
2. Running the `internal/cli` tests via the compiled test binary directly and via `go test -count=1 ./internal/cli` produces identical pass/fail results.
3. Introducing a deliberate failing assertion in any `internal/cli` test causes `go test ./internal/cli` to exit nonzero. Removing it restores a clean run.
4. A test that reaches the execution path without injecting the seam fails loudly; it does not replace the process and does not yield exit code 0.
5. `get_role_details` asserts `roles:reviewer` and `get_context_details` asserts `contexts:project`; both pass run in isolation and in the full suite.
6. `go test -count=1 -shuffle=on ./...` reports a result for every test, and the `integration`, `e2e`, and `registry` tagged suites do the same. The only deferred test is `TestTaskResolution_ExactMatchFallsThrough`, reported as skipped with a comment naming project 04.
7. A real (non-dry-run) `start` invocation still replaces the process with the agent command, confirming the seam left production behaviour intact.
