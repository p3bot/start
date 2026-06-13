# Project: Support and Validate the Module `uses` Field

## Goal

Teach the `start` CLI about a new optional module field, `uses`, which declares the other library modules a module pulls in at runtime via `start get` (for example a task that runs `start get contexts:start/library/publishing`). The CLI must preserve the field when modules are installed and updated, and `start doctor validate` must verify that every declared reference resolves to a real module. This makes cross-module reuse a checked, first-class relationship instead of an invisible string buried in prose.

## Scope

In scope:

- Add a `Uses []string` field to the four module config structs and decode it from CUE.
- Emit `uses` when writing a module to config, and preserve it through the install and update paths.
- Add a `doctor validate` check that each `uses` entry is a fully-qualified colon-form address resolving to a module present in the index.
- Test coverage for parse round-trip, install preservation, and the new validation check.

Out of scope:

- The schema definition of `uses`. That is added in the library repo (`library/02-module-reuse-and-publishing-workflow.md`); this project consumes it.
- Any runtime behaviour change to `start get` itself. Colon-form addresses already resolve (see Current State); recursive `start get` calls inside module content already work today. This project does not change resolution.
- The `--force` gate semantics of `doctor validate`, which are unchanged.

## Current State

The CLI mirrors the CUE module schemas with hand-written structs in `internal/cli/config_types.go`: `AgentConfig` (lines 16-26), `RoleConfig` (181-191), `ContextConfig` (299-310), `TaskConfig` (424-434). Each carries the schema fields plus `Source` and `Origin`. Decoding is explicit and permissive: each field is read with a guard, for example

```go
if v := val.LookupPath(cue.ParsePath("description")); v.Exists() {
    role.Description, _ = v.String()
}
```

Unknown CUE fields are silently ignored at decode time, so a module carrying `uses` loads today without error — but the field is dropped. List fields already have a precedent in the `tags` extraction helper.

Install and update do not round-trip arbitrary fields. `internal/modules/install.go` `formatModuleStruct` (lines 168-177) writes a module to config from a hardcoded field list; a field absent from that list is lost on install. `start update` (`internal/cli/update.go`) re-fetches and rewrites modules through the same path. So without a change, an installed or updated module silently loses its `uses` declaration.

`start doctor validate` lives in `internal/cli/doctor_validate.go` (subcommand registered at lines 92-115, `--force` gate at line 111). It iterates the index categories (agents, roles, contexts, tasks) in `validateModules` (line 553) and checks each indexed module in `validateOneModule` (line 602): index-version-versus-latest-published, missing or orphan git tags, content staleness, and filesystem modules missing from the index. It does not inspect module content fields.

Module address resolution already understands the colon form. `internal/cli/describe.go` `parseAddress` (lines 58-95) splits `category:name` on the first colon, validates the category against the known set, and treats a bare input as a cross-category name. `start get` (`internal/cli/get.go`, entry `runGet` at line 65) resolves through this and renders module content; a recursive `start get contexts:start/library/publishing` from inside a task therefore already works. `start update` (`internal/cli/update.go`) fetches the index and each installed module's latest version and rewrites changed modules.

Testing conventions (from AGENTS.md): prefer real behaviour over mocks, real files via `t.TempDir()`, table-driven cases, `setupStartTestConfig(t)` for config isolation. `doctor validate` is excluded from the offline `--json` harness; its `--json` shape is covered by a `//go:build registry` integration test run with `go test -tags=registry`.

## Requirements

1. Add `Uses []string` (with a `json:"uses,omitempty"` tag for `--json` output) to `AgentConfig`, `RoleConfig`, `ContextConfig`, and `TaskConfig` in `internal/cli/config_types.go`, and decode it from the CUE `uses` list in each decode path, following the existing list-extraction pattern used for `tags`. Practically only roles, contexts, and tasks will carry it, but adding it to all four keeps the structs uniform with `#Base`.

2. Emit `uses` when a module is written to config. Add `uses` to the field list in `internal/modules/install.go` `formatModuleStruct` (and any sibling writer) so the field survives `start install` and `start update`. A module installed with a `uses` declaration must retain it in the written config.

3. Add a validation check to `start doctor validate`. For each module that declares `uses`, every entry must be a fully-qualified colon-form address (`category:path`) whose category is known and whose `path` resolves to an entry present in the index for that category. Report each unresolved or malformed entry as a validation issue with a clear message naming the declaring module and the bad reference. A module with no `uses` field is unaffected. This check runs under the existing `--force`-gated flow alongside the other per-module checks.

4. Provide tests: a decode round-trip proving `uses` is parsed; an install/update test proving the field is preserved in written config; and a validation test proving a module whose `uses` entry resolves passes, while a module whose entry is malformed or points at a missing module fails. Follow the existing harness conventions, including the `registry`-tagged path for `doctor validate` assertions.

5. Update this repo's AGENTS.md: document the `uses` field and the new `doctor validate` reference check, and adopt Scoped Commits (https://scopedcommits.com) as this repo's commit convention — format `<scope>: <description>`, multiple scopes comma-separated, no `feat`/`fix` type prefix. This repo currently uses Conventional Commits (inferred from git history); the change applies going forward.

## Constraints

- Go, matching the repository toolchain. Build with `go build ./...`; verify with `scripts/invoke-tests`.
- Do not change the `start get` resolution path or `parseAddress`; colon-form resolution already exists and is relied on, not modified.
- Keep `doctor validate` behaviour additive: the new check must not alter existing checks, exit-code mapping, or the `--force` gate.
- Preserve the permissive decode contract: a module without `uses` must behave exactly as today.
- Follow the registry-client seam guidance in AGENTS.md: command paths obtain the client through the provider; do not introduce direct `registry.NewClient()` calls on a seam that needs stubbing.

## Implementation Plan

1. Add the `Uses` field and decode logic to the four structs in `config_types.go`, reusing the list-extraction pattern. Add a decode round-trip test.

2. Add `uses` to the writer field list in `modules/install.go` so install and update preserve it. Add a preservation test exercising install (and update if it uses a distinct writer).

3. Extend `validateOneModule` (or an adjacent helper) in `doctor_validate.go` to read a module's `uses` entries and resolve each against the index categories, emitting an issue per malformed or unresolvable reference. Reuse the existing colon-form parsing (`parseAddress`) rather than re-splitting. Add validation tests under the `registry`-tagged path.

4. Update AGENTS.md.

## Implementation Guidance

- Resolve a `uses` entry by parsing it with the existing `parseAddress`, then looking the `path` up in the matching category map already iterated by `validateModules`. The index is the source of truth doctor already loads; do not introduce a second fetch.
- Decide deliberately whether `doctor validate` reads `uses` from the installed/filesystem module or the published registry module, and keep it consistent with how the surrounding checks already source their data; state the choice where the check is added.

## Acceptance Criteria

- A module CUE value containing `uses: ["contexts:start/library/publishing"]` decodes to a config struct whose `Uses` slice contains that entry.
- Installing a module that declares `uses` writes the field into config; a subsequent `start update` of that module retains it.
- `start doctor validate --force` reports an issue when a module's `uses` entry is malformed (no colon, unknown category) or names a module absent from the index, and reports no such issue when every entry resolves.
- `start get contexts:start/library/publishing` resolves and outputs the module content (confirming the existing path, exercised as a guard against regression).
- A module with no `uses` field produces identical `doctor validate` output to before this change.
- This repo's AGENTS.md documents the Scoped Commits convention.
