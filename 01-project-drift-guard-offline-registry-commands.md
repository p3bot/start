# Registry-Command Injection Seam for the help schemas Drift Guard

Source: project-doc-review on 2026-05-27
Category: design
Location: project.md (CLI Contract Alignment) — Implementation Plan step 7, Requirement 6, Acceptance Criteria

## Goal

Enable the `start help schemas` drift-guard test to assert each `--json`-capable command's real output shape offline, deterministically, without reaching the live CUE Central Registry. This is a prerequisite for the CLI Contract Alignment project's Requirement 6 acceptance criterion ("A test asserts each command's real `--json` output matches its documented shape"). The parent project's drift guard cannot be built until this seam exists.

## Scope

In scope:
- A registry/index injection seam that lets tests supply a fixed in-memory `*registry.Index` (and, where needed, a stub registry client) to the registry-backed `--json` commands: `library`, `search`, `update`, `doctor`.
- Routing those commands' index acquisition through the seam so production behaviour is unchanged while tests can run them offline.
- A coverage strategy for `doctor validate`, whose `--json` output is produced by fetching and schema-validating every module in the index (not by reading the index alone).

Out of scope:
- The drift-guard test content itself and the `help schemas` topic — those belong to the parent project and are written once this seam is in place.
- `list`, `config get`, `config list`, `config settings` — these emit `--json` from local config with no registry dependency (`list` touches the registry only under `--verbose`, which the guard does not pass). They need no seam.
- Changing what `internal/cache` stores. It deliberately caches a version string, not index content; that stays.

## Current State

The parent project's Plan step 7 assumes two mechanisms that do not work against the current code:

1. It assumes `setupStartTestConfig(t)` with `skipRegistry: true` stops the registry-backed commands from dialing the registry. It does not. `skipRegistry` is a field on the `resolver` struct (`internal/cli/resolve.go:59`), consulted only in `resolver.ensureIndex()` (`resolve.go:674`). The registry-backed commands do not go through the resolver — they construct their own client. AGENTS.md states the scope precisely: skipRegistry applies "in tests touching the resolver."
2. It assumes a "local cached-index fixture (via `internal/cache`)" lets those commands "emit a shape offline." It cannot. `internal/cache` stores only a version string (`WriteIndex(version string)`, `ReadIndex().Version`). It short-circuits version resolution; the index content is still fetched from the registry (or CUE's on-disk module cache) by `FetchIndex`/`Fetch`.

How the commands acquire the index today:

| Command | Acquisition site | Notes |
| ------- | ---------------- | ----- |
| `update`, `install` | `fetchIndex(ctx, cmd, prog, msg)` in `internal/cli/modules_shared.go:26` | Already centralized. Returns `(*registry.Index, *registry.Client, error)`. Calls `registry.NewClient()` then `client.FetchIndex(...)` then `cache.WriteIndex(...)`. |
| `search` | `internal/cli/search.go:167` | Inline `registry.NewClient()` + `client.FetchIndex(...)`. Graceful fallback: registry errors degrade to local-only results. |
| `library` | `internal/cli/library.go:68` | Inline `registry.NewClient()` + `ResolveLatestVersion` + `Fetch`. Has an `--export` branch that dumps raw CUE; the `--json` path needs the parsed index. |
| `doctor` | `internal/cli/doctor.go:218` and `:274` | Two inline `registry.NewClient()` calls: one for schema validation, one for the index check. Reads `cache.ReadIndex()` only for the version. |
| `doctor validate` | `internal/cli/doctor_validate.go:196` | Inline `registry.NewClient()`, then `validateIndex` and `validateModules` → `validateOneModule` (`:646`) which calls `client.Fetch` per module and schema-validates each. |

There is no existing seam to inject a fake registry or index into any of these commands. `internal/cli/snapshots_test.go` exercises only text-mode `describe`/`config get`; it does not capture `--json` output. The test helper is `setupStartTestConfig` (see `internal/cli/config_testhelpers_test.go`).

The cost is asymmetric. Index acquisition for `update`/`install`/`search`/`library`/`doctor` is shallow — they need a parsed `*registry.Index`. `doctor validate` is deep — it fetches and validates every module in the index per run, so making it emit a `--json` shape offline requires stubbing the per-module fetch/validate path, not just the index.

## Requirements

1. The registry-backed `--json` commands `library`, `search`, `update`, and `doctor` acquire their index through a single injectable seam that defaults to the real registry fetch in production and can be replaced in tests with an in-memory `*registry.Index`.
2. With the seam stubbed, each of those four commands runs under `--json` offline (no network, no live registry) and produces its documented output shape deterministically.
3. Production behaviour of all four commands is unchanged: same index source, same `cache.WriteIndex` side effect, same graceful-degradation semantics (notably `search`, which must still fall back to local-only results when the registry is unavailable).
4. `doctor validate` is covered by a separately-gated integration test that exercises the real registry path and asserts its `--json` shape (`ValidateReport` / `ValidateCategoryResult` / `ValidateModuleResult`). It is excluded from the offline drift guard. The gate must keep it out of the default `scripts/invoke-tests` registry-free run.
5. The seam does not change what `internal/cache` persists.

## Implementation Plan

1. Define the seam at the existing centralization point. `fetchIndex` in `modules_shared.go` already returns `(*registry.Index, *registry.Client, error)` — the exact shape the consumers need. Make it injectable: either a package-level function variable (`var fetchIndexFn = fetchIndex`) that tests override and restore, or a small interface threaded through the command constructors. Prefer the approach that matches the repo's established factory pattern (`NewRootCmd()` builds isolated instances) so parallel tests do not race on shared global state; a per-instance provider stored alongside `Flags` is cleaner than a package var if global mutation would leak across tests.
2. Route the inline acquirers through the seam. Migrate `search` (`search.go:167`) and `library` (`library.go:68`) and the `doctor` index check (`doctor.go`) to obtain the index via the seam rather than constructing `registry.NewClient()` directly. Preserve `library`'s `--export` branch (it bypasses JSON) and `search`'s graceful fallback.
3. Build the test fixture. Provide an in-memory `*registry.Index` builder for tests with representative entries across agents/roles/contexts/tasks. Wire it through the seam in the harness (extend `setupStartTestConfig` or add a focused helper) so a test can run `library/search/update/doctor --json` and capture stdout.
4. Extend the snapshot/capture harness for JSON mode. `snapshots_test.go` renders text only; add JSON-mode capture so the drift guard (built later, in the parent project) can assert structural shape — keys and types, not data values.
5. Carve out `doctor validate`. Add a build-tagged or env-gated integration test (for example `//go:build registry` or skipped unless `START_REGISTRY_TESTS=1`) that runs `doctor validate --json` against the real registry and asserts the report shape. Document the gate so it is not run by the default test pipeline.
6. Update the parent `project.md` accordingly when this lands: Plan step 7 and Requirement 6 should describe the index-provider seam and the `doctor validate` carve-out, and drop the incorrect `skipRegistry`/`internal/cache`-fixture description.

Decision left to the implementer: whether to instead build a full offline stub for `doctor validate`'s per-module fetch/validate walk (a second seam over `client.Fetch` and schema validation) and keep it in the offline drift guard. This is more faithful but materially more work and more brittle. The recommended path is the carve-out (step 5), because `doctor validate`'s output shape is simple and stable while its execution path is the most expensive to stub.

## Constraints

- Pure Go, gofmt-clean, `golangci-lint` clean. `scripts/invoke-tests` (lint + tests) must pass and must not reach the live registry.
- Follow the repo's Cobra patterns: command construction through `NewRootCmd()`, `RunE` returning errors. Do not introduce shared mutable global state that leaks across parallel tests; prefer per-instance injection consistent with the existing factory.
- Honour AGENTS.md testing principles: real behaviour over mocks where practical (real CUE validation, real files via `t.TempDir()`), interfaces/parameters over globals.
- No production behaviour change. The seam's default must be the current `registry.NewClient()` + `FetchIndex` path, including the `cache.WriteIndex` side effect and `search`'s registry-unavailable fallback.

## Acceptance Criteria

- `library --json`, `search --json`, `update --json`, and `doctor --json` run to completion offline in tests against an injected in-memory index and emit their documented shapes; no test in the default pipeline contacts the live registry.
- Running the same commands without the stub still fetches from the registry exactly as before (manually verifiable; `cache.WriteIndex` still fires; `search` still degrades gracefully when the registry is unreachable).
- `doctor validate --json` shape is asserted by an integration test that is excluded from the default registry-free run and gated explicitly.
- The JSON-capture harness exists and is usable by the parent project's `help schemas` drift guard.
