# TODO

- Add the linter config and refactor common print and other calls `_, _ = fmt.Fprintln(r.stdout)`

## Extract `resolveModuleFile` helper in `internal/orchestration/composer.go`

Context: discovered during the 2026-05-17 pre-commit review (`.start/reviews/2026-05-17-pre-commit-01.md`, findings L2 + I2). The composer change in that commit tightened `@module/` origin handling across three resolver methods (`resolveContext`, `resolveRole`, `ResolveTask`), perpetuating an existing copy-paste pattern. The refactor was deferred to keep the review-scoped commit small.

### Problem

Three near-identical 11-line blocks in `internal/orchestration/composer.go` resolve `@module/` paths against a CUE value's `origin` field. They differ only in the return tuple shape and the parameter name for the CUE value (`ctxVal`, `roleVal`, `taskVal`):

- `resolveContext` (`composer.go:437-448`)
- `resolveRole` (`composer.go:498-509`)
- `ResolveTask` (`composer.go:701-712`)

A fourth copy lives in `roleFileAvailable` (`composer.go:641-662`) but is intentionally different (probe semantics, `(bool, string)` return, no install-hint in the message). Leave that one alone.

Consequence: any future change to the `@module/` resolution contract — error wording, debug logging, the install-hint phrasing, origin validation — has to land in three sites in lockstep. The "missing origin" error message is currently duplicated verbatim three times.

### Proposed shape

Add a pure function (no pointer mutation) so callers control assignment and the contract reads at the call site:

```go
// resolveModuleFile resolves an @module/ file path against v's origin field.
// Returns the input unchanged if the path is not an @module/ reference.
func resolveModuleFile(file string, v cue.Value) (string, error) {
    if !strings.HasPrefix(file, "@module/") {
        return file, nil
    }
    origin := ExtractOrigin(v)
    if origin == "" {
        return "", fmt.Errorf("missing origin for @module/ path %s\nRun 'start modules install' to reinstall", file)
    }
    resolved, err := ResolveModulePath(file, origin)
    if err != nil {
        return "", fmt.Errorf("resolving module path %s: %w\nRun 'start modules install' to reinstall", file, err)
    }
    return resolved, nil
}
```

Each call site collapses to four lines, e.g. in `resolveContext`:

```go
resolved, err := resolveModuleFile(fields.File, ctxVal)
if err != nil {
    return ProcessResult{}, err
}
fields.File = resolved
```

`resolveRole` returns `("", "", err)` and `ResolveTask` returns `(ProcessResult{}, err)` — same shape, different return tuple.

### Design notes (recording rejected alternatives)

- Mutation via `*UTDFields` was considered (one line shorter per call site) but rejected: the implicit-modification contract is worse than four explicit lines, and the existing `roleFileAvailable` helper next door is already pure-style — staying consistent reads better.
- Folding `roleFileAvailable` into the same helper was considered and rejected: the probe contract (returns `(bool, string)` for UI display, omits the install hint) is meaningfully different. A shared helper with a mode flag would make one of the two awkward. Two helpers, each clear.

### Test coverage (I2 from the review)

Today the strict "missing origin" path is only exercised through `selectDefaultRole` → `roleFileAvailable`. After extraction, add a single table-driven `TestResolveModuleFile` covering:

- non-`@module/` path returns input unchanged, no error
- `@module/` path with empty origin returns error containing `missing origin for @module/ path`
- `@module/` path with origin pointing at a cached extract returns the resolved absolute path
- `@module/` path with origin pointing at a missing cache entry returns error containing `resolving module path`

Use the same `t.Setenv("CUE_CACHE_DIR", t.TempDir())` + fabricated `mod/extract/...` directory idiom established by `TestSelectDefaultRole_ModulePath` and `TestRoleFileAvailable` in `composer_test.go`. Do not call `t.Parallel` — incompatible with `t.Setenv`.

With the helper extracted and tested directly, the three resolver methods become trivially correct by construction; no per-resolver test for the missing-origin branch is needed.

### Acceptance

- One new function `resolveModuleFile` in `internal/orchestration/composer.go`.
- Three call sites in `resolveContext`, `resolveRole`, `ResolveTask` switched to call it; the inline `@module/` blocks deleted.
- `roleFileAvailable` left as-is.
- One new test `TestResolveModuleFile` covering the four cases above.
- `go test ./...` and `go vet ./...` clean.
- No behaviour change at any call site — error wording and resolution semantics preserved.
