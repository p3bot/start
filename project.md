# Doctor: verify @module/ paths against the CUE cache

## Goal

Make `start doctor` give an honest answer about whether a registry-installed role, context, or task is actually usable. Today every `@module/` file reference is reported as passing regardless of whether the module is extracted in the CUE cache, so `doctor` cannot detect the failure mode it should catch.

## Scope

In scope:

- Change `checkFileField` in `internal/doctor/checks.go` so that `@module/` paths are resolved against the role/context/task's `origin` field and stat'd in the CUE cache before being reported.
- Surface three distinct outcomes: extract present (pass with a useful message), extract missing (notfound with an actionable fix), origin missing on an `@module/` path (fail with a config-error message).
- Add unit tests in `internal/doctor/checks_test.go` covering all three outcomes for at least one category (roles is sufficient since `checkFileField` is shared across roles, contexts, and tasks).

Out of scope:

- Any change to `internal/orchestration/` (the runtime resolver already handles `@module/` correctly after the recent `selectDefaultRole` fix).
- Any change to module installation, registry lookup, or auto-install behaviour.
- New top-level commands or doctor sections.
- Changes to JSON output shape of `CheckResult` (existing fields are sufficient).
- Library or homebrew-tap changes; no release activity.

## Current State

`checkFileField` lives at `internal/doctor/checks.go:419` and is shared by the role, context, and task sections via calls at `checks.go:285`, `:328`, and `:371`. The relevant block:

```go
// @module/ paths are resolved at runtime via the CUE module cache
if strings.HasPrefix(filePath, "@module/") {
    return &CheckResult{
        Status:  StatusPass,
        Label:   name,
        Message: "(registry module)",
    }
}
```

Effect: every `@module/` reference is unconditionally passed. A user whose `~/.cache/cue` has been cleared, or whose `start modules install` failed silently, sees `start doctor` report all-green and then hits a runtime error when they try to use the module.

Resolver function already exists: `orchestration.ResolveModulePath(path, origin string) (string, error)` at `internal/orchestration/composer.go:771`. It takes the literal `@module/...` string and an origin like `github.com/start-cli/library/roles/start/library/assistant@v1.0.0` and returns the absolute extract path. It errors when the module is not in the cache, with a message containing `"not found in cache"`.

Origin extractor already exists: `orchestration.ExtractOrigin(v cue.Value) string` at `internal/orchestration/composer.go:758`. Returns empty string when no `origin` field is set on the CUE value.

`CheckResult` shape at `internal/doctor/doctor.go:63` includes `Status`, `Label`, `Message`, `Fix`, and `Details`. The `Fix` field is rendered in the report when status is `StatusFail` or `StatusWarn` (see `internal/doctor/reporter.go:140`), so actionable hints belong there.

Status values: `StatusPass`, `StatusWarn`, `StatusFail`, `StatusNotFound` (declared in `internal/doctor/doctor.go`; rendered as `✓`, `⚠`, `✗`, `○` per `doctor_test.go:39-43`).

Existing test patterns for `checkFileField` live in `internal/doctor/checks_test.go` around `TestCheckRoles_FileExists` (line 464), `TestCheckRoles_FileMissing` (line 484), and `TestCheckRoles_PromptFallback` (line 499). They use `cuecontext.New()`, build a CUE value with `CompileString`, and call `CheckRoles`. Follow the same shape.

Cache layout: `<CUE_CACHE_DIR>/mod/extract/<module-path-parent>/<module-base>@<version>/<file>`. The `CUE_CACHE_DIR` environment variable overrides the default location and is honoured by `orchestration.GetCUECacheDir` (`composer.go:836`). Tests that need a fake cache must use `t.Setenv("CUE_CACHE_DIR", t.TempDir())`, which is incompatible with `t.Parallel`; new tests must omit `t.Parallel()` even though existing sibling tests use it.

## References

- Project writing guide: `~/.ai/docs/project-writing-guide.md`
- Prior orchestration fix that exposed the doctor gap: `internal/orchestration/composer.go:641` (`roleFileAvailable` resolves `@module/` paths against the cache) and the regression tests `TestSelectDefaultRole_ModulePath` and `TestRoleFileAvailable` in `internal/orchestration/composer_test.go`. Read both for the cache-layout pattern and the test idiom used to fabricate an extract dir.
- `start/AGENTS.md` for build, test invocation, and project conventions.

## Requirements

1. `start doctor` reports `StatusPass` for an `@module/` role, context, or task only when the extracted file actually exists in the CUE cache.
2. `start doctor` reports `StatusNotFound` when the `origin` field is present but the corresponding cached extract is missing, including a `Fix` string that names the install command the user should run.
3. `start doctor` reports `StatusFail` when an `@module/` file path is declared without an `origin` field, since such a config cannot resolve at runtime either.
4. The pass-case message identifies the source as a registry module without losing the version information available in the origin (the implementer chooses the exact phrasing).
5. Roles, contexts, and tasks all benefit from the change with no per-section duplication of logic.
6. New tests cover the three outcomes (extract present, extract missing with origin, origin missing) and the existing `checks_test.go` tests continue to pass unchanged.
7. `go test ./...` and `go vet ./...` are clean.

## Implementation Plan

1. Read the prior orchestration fix referenced above. It establishes the exact integration pattern for `ResolveModulePath` + `ExtractOrigin` and the test idiom for fabricating an extract directory under `t.Setenv("CUE_CACHE_DIR", ...)`.
2. Modify `checkFileField` in `internal/doctor/checks.go` so the `@module/` branch:
   - Extracts the role/context/task's `origin` via `orchestration.ExtractOrigin`.
   - Returns `StatusFail` with an actionable message when origin is empty.
   - Calls `orchestration.ResolveModulePath` and `os.Stat`s the returned path.
   - Returns `StatusNotFound` with a `Fix` string when the stat fails, naming `start modules install` and the module name.
   - Returns `StatusPass` only when the extract is present, with a message that includes a hint of the version (the implementer decides whether to show the full origin, only the version, or only the trailing path segments).
3. Add tests in `internal/doctor/checks_test.go`:
   - Extract present: build a CUE config with `origin` and `file: "@module/role.md"`, fabricate the extract dir under a temp `CUE_CACHE_DIR`, assert `StatusPass`.
   - Extract missing: same config, no extract dir, assert `StatusNotFound` and a `Fix` string mentioning `modules install`.
   - Origin missing: config with `file: "@module/role.md"` and no `origin`, assert `StatusFail`.
4. Run `go test ./...` and `go vet ./...`. Confirm both pass.
5. Manually run `start doctor` against a config that exercises both the happy path and a deliberately broken case (e.g. temporarily rename one extract dir) to confirm the reporter renders the new statuses correctly.

## Constraints

- Go 1.25, module `github.com/start-cli/start`. No new dependencies.
- Reuse `orchestration.ResolveModulePath` and `orchestration.ExtractOrigin`; do not duplicate their logic in `internal/doctor`.
- Test conventions defined in `start/AGENTS.md` apply: prefer real CUE validation and real files via `t.TempDir()`; avoid mocks. New tests that use `t.Setenv` must not call `t.Parallel`.
- Preserve the existing `CheckResult` JSON schema (`Status`, `Label`, `Message`, `Fix`, `Details`). Do not add fields.
- License headers and import grouping follow the patterns already present in the touched files.

## Implementation Guidance

The change alters user-visible `start doctor` output. Some users may have configs that previously showed all-green and will start showing legitimate failures. That is the intended outcome — flag it in the commit message so the behavioural change is discoverable in `git log`, but do not soften it with a feature flag or compatibility shim.

`checkFileField` is shared across roles, contexts, and tasks. The branch added here therefore needs no per-category awareness — `name` is already passed in and is the right label for the `Fix` message. The implementer should not need to touch `CheckRoles`, `CheckContexts`, or `CheckTasks`.

## Acceptance Criteria

- `go test ./...` and `go vet ./...` pass.
- Three new tests exist in `internal/doctor/checks_test.go` covering: `@module/` extract present, `@module/` extract missing with origin set, `@module/` with no origin. Each asserts on `Status` and (where applicable) the `Fix` string content.
- Running `start doctor` against the workspace config with all modules installed shows the configured `@module/` roles and contexts as passing, with messages distinct from `(registry module)` so the new behaviour is visible.
- Running `start doctor` after `rm -rf` of one extract dir under `~/.cache/cue/mod/extract` reports that specific role/context/task as `StatusNotFound` with a `Fix` mentioning `start modules install`. Restoring the extract returns the report to passing.
- No changes outside `internal/doctor/` and its test file. Diff confined to one source file and one test file.
