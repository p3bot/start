# Extract resolveModuleFile helper in composer.go

## Goal

Collapse three near-identical 11-line `@module/` resolution blocks in
`internal/orchestration/composer.go` into a single helper so future
changes to the resolution contract — error wording, debug logging, the
install-hint phrasing, origin validation — land in one place instead
of three. Behaviour at every call site is preserved exactly.

## Scope

In scope:

- Add one new pure function `resolveModuleFile` in
  `internal/orchestration/composer.go`.
- Switch three call sites — `resolveContext` (`composer.go:437-448`),
  `resolveRole` (`composer.go:498-509`), `ResolveTask`
  (`composer.go:701-712`) — to call the helper and delete the inline
  `@module/` blocks they currently host.
- Add one new test `TestResolveModuleFile` in the same package's test
  file, covering the four cases listed under Requirements.

Out of scope:

- `roleFileAvailable` at `composer.go:641-662`. Its `@module/` block
  is intentionally different (probe semantics, `(bool, string)`
  return, no install-hint in the message). Leave it as-is.
- Any change to `ResolveModulePath` or `ExtractOrigin`.
- Any change to the public API or signatures of `resolveContext`,
  `resolveRole`, or `ResolveTask`.
- New per-resolver tests for the missing-origin branch. The helper
  test is sufficient; the resolvers become trivially correct by
  construction.
- Behaviour changes at any call site — error wording and resolution
  semantics are preserved verbatim.
- Library or homebrew-tap changes.

## Current State

Three resolver methods in `internal/orchestration/composer.go` each
contain a near-identical 11-line block resolving `@module/` paths
against a CUE value's `origin` field. They differ only in the return
tuple shape and the parameter name for the CUE value
(`ctxVal`, `roleVal`, `taskVal`):

- `resolveContext` at `composer.go:437-448` — returns
  `(ProcessResult, error)`.
- `resolveRole` at `composer.go:498-509` — returns
  `(string, string, error)`.
- `ResolveTask` at `composer.go:701-712` — returns
  `(ProcessResult, error)`.

A fourth copy lives in `roleFileAvailable` at `composer.go:641-662`
but is intentionally different (probe semantics, `(bool, string)`
return, omits the install-hint). It stays as-is.

The `"missing origin"` error message is currently duplicated verbatim
across the three in-scope blocks. Any future change to the
`@module/` resolution contract has to land in three sites in lockstep.

Resolver helpers already exist:

- `ExtractOrigin(v cue.Value) string` at `composer.go:758` — returns
  empty when no `origin` field is set.
- `ResolveModulePath(path, origin string) (string, error)` at
  `composer.go:771` — resolves the literal `@module/...` against the
  origin and returns the absolute extract path. Errors when the
  module is not in the cache, with a message containing
  `"not found in cache"`.

Existing test idiom for `@module/` cache fabrication lives in
`internal/orchestration/composer_test.go`:

- `TestSelectDefaultRole_ModulePath` and `TestRoleFileAvailable` use
  `t.Setenv("CUE_CACHE_DIR", t.TempDir())` to redirect the cache, then
  fabricate `mod/extract/<module-path-parent>/<module-base>@<version>/<file>`
  on disk.
- These tests do not call `t.Parallel()` — `t.Setenv` is incompatible
  with parallelism. New helper tests must follow the same rule.

`CUE_CACHE_DIR` is honoured by `GetCUECacheDir` at `composer.go:836`.

## References

- Project writing guide: `~/.ai/docs/project-writing-guide.md`
- 2026-05-17 pre-commit review notes:
  `.start/reviews/2026-05-17-pre-commit-01.md` (findings L2 + I2). The
  refactor was deferred from that review to keep the review-scoped
  commit small.
- `start/AGENTS.md` for build, test invocation, and project conventions.

## Requirements

1. One new function `resolveModuleFile` exists in
   `internal/orchestration/composer.go`. It is pure (no pointer
   mutation): given a file string and a `cue.Value`, it returns the
   resolved file string and an error.
2. When the input file string does not begin with `@module/`, the
   helper returns it unchanged with a nil error.
3. When the input file string begins with `@module/` and the CUE
   value has no `origin` field, the helper returns the empty string
   and an error whose message contains
   `missing origin for @module/ path` and includes a hint pointing
   the user at `start modules install`.
4. When the input file string begins with `@module/` and the origin
   is present, the helper delegates to `ResolveModulePath` and
   returns its result. On error, the returned error message includes
   `resolving module path` and the same install hint.
5. The three call sites in `resolveContext`, `resolveRole`, and
   `ResolveTask` call the helper and assign its result to their
   local `fields.File` (or equivalent). The inline `@module/` blocks
   they hosted are deleted.
6. `roleFileAvailable` at `composer.go:641-662` is unchanged.
7. One new table-driven test `TestResolveModuleFile` exists in
   `internal/orchestration/composer_test.go`, covering four cases:
   - Non-`@module/` path returns input unchanged with nil error.
   - `@module/` path with empty origin returns an error whose
     message contains `missing origin for @module/ path`.
   - `@module/` path with origin pointing at a cached extract returns
     the resolved absolute path.
   - `@module/` path with origin pointing at a missing cache entry
     returns an error whose message contains
     `resolving module path`.
8. `go test ./...` and `go vet ./...` pass cleanly.
9. No behavioural change at any of the three call sites — error
   wording and resolution semantics match what the inline blocks
   produced before.

## Implementation Plan

1. Read the three call sites and confirm the inline blocks are
   identical save for the return tuple shape and the local variable
   name for the CUE value.
2. Add `resolveModuleFile` near the other helpers in
   `composer.go` (the implementer decides exact placement). Suggested
   shape:

   ```go
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

3. Update the three call sites. Each collapses to a call,
   error-return, and field assignment. Example for `resolveContext`:

   ```go
   resolved, err := resolveModuleFile(fields.File, ctxVal)
   if err != nil {
       return ProcessResult{}, err
   }
   fields.File = resolved
   ```

   `resolveRole` returns `("", "", err)`; `ResolveTask` returns
   `(ProcessResult{}, err)`. Otherwise identical.
4. Add `TestResolveModuleFile` in `composer_test.go`. Use the
   `t.Setenv("CUE_CACHE_DIR", t.TempDir())` + fabricated
   `mod/extract/...` directory idiom from the existing
   `TestSelectDefaultRole_ModulePath` and `TestRoleFileAvailable`
   tests. Do not call `t.Parallel()`.
5. Run `go test ./...` and `go vet ./...`. Confirm both pass.
6. Diff-check the three call sites to confirm the error wording and
   resolution semantics match the inline blocks they replaced.

## Constraints

- Go 1.25, module `github.com/start-cli/start`. No new dependencies.
- The helper is pure — no pointer mutation, no side effects beyond
  the return value. Mutation via `*UTDFields` was considered and
  rejected: the implicit-modification contract is worse than the
  three explicit lines at the call site, and the existing
  `roleFileAvailable` helper is already pure-style.
- Do not fold `roleFileAvailable` into the same helper. Its probe
  contract (returns `(bool, string)`, omits the install hint) is
  meaningfully different. A shared helper with a mode flag would
  make one of the two awkward.
- Error wording must match the inline blocks exactly. The contract
  is the user-visible message; preserve it character-for-character.
- No behaviour change at any call site. Existing tests pass without
  assertion changes.
- Tests that use `t.Setenv` must not call `t.Parallel`.
- Do not modify any file under `library/` or `homebrew-tap/`.

## Implementation Guidance

- The refactor is narrowly scoped. Resist the temptation to clean up
  surrounding code in `resolveContext`, `resolveRole`, or
  `ResolveTask` — the diff stays trivially reviewable when it
  touches only the three blocks plus the new helper.
- With the helper extracted and tested directly, the three resolver
  methods become trivially correct by construction. No per-resolver
  test for the missing-origin branch is needed; today that path is
  only exercised through `selectDefaultRole` → `roleFileAvailable`,
  and it can stay that way.
- Place the new test next to the existing `@module/` tests in
  `composer_test.go` for discoverability.

## Acceptance Criteria

- One new function `resolveModuleFile` exists in
  `internal/orchestration/composer.go`.
- The three inline `@module/` blocks in `resolveContext`,
  `resolveRole`, and `ResolveTask` are deleted and replaced with
  calls to the helper.
- `roleFileAvailable` is unchanged.
- One new table-driven test `TestResolveModuleFile` exists in
  `internal/orchestration/composer_test.go` and covers the four
  cases listed in Requirements.
- `go test ./...` and `go vet ./...` are clean.
- Diff confined to one source file and one test file.
- No changes under `library/` or `homebrew-tap/`.
