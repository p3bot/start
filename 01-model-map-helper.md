# Project: Hoist the Agent Models Walk into a Shared Helper

## Goal

Replace three copies of the agent `models` decoding walk with a single shared helper in `internal/cue/`. The walk that turns an agent's `models` field into resolved model ids is currently duplicated across two display surfaces and the runtime executor; consolidating it removes the risk that a future schema change updates one copy and silently diverges the others.

## Scope

In scope:

- A new exported helper in `internal/cue/` that owns the agent `models` walk and returns an alias-to-id map.
- Migration of all three current call sites to the helper.
- Unit coverage for the helper at its new home.
- Keeping the two existing object-form snapshot tests as the integration guard.
- Optional centralisation of the `models` and `id` key string literals into `internal/cue/keys.go`, consistent with the existing key constants.

Out of scope:

- Changing what counts as a valid `models` entry. The simple form (`alias: "id"`) and the object form (`alias: {id: "id"}`) remain the only accepted shapes; behaviour for both is preserved exactly.
- Adding validation, warnings, or errors on malformed entries. A malformed entry is skipped today and must keep being skipped silently. Hardening is a separate pass.
- The registry index `models` walk in `internal/cli/list.go` (see Current State). It is a different shape and is not touched.
- Any change to the published library, registry, or CUE schemas.

## Current State

`start` is a cobra-based Go CLI built on CUE. Agent definitions carry a `models` field mapping a short alias (for example `sonnet`, `opus`) to a model id. The schema permits the simple form `models: { sonnet: "model-id" }`; the object form `models: { sonnet: { id: "model-id" } }` is forbidden by the schema but accepted defensively by the decode paths.

The same fallback walk exists in three places. Each iterates the `models` fields, tries `cue.Value.String()` first (simple form), and falls back to `LookupPath("id").String()` (object form):

- `internal/cli/config_types.go`, `decodeAgentValue` — builds the full `map[string]string` for an `AgentConfig`. Its doc comment already concedes the duplication ("the same walk runs in executor.go and partialFillAgentCommand").
- `internal/orchestration/executor.go`, `extractAgentFields` — builds the full `map[string]string` for an `Agent`. Also reached via `internal/orchestration/autosetup.go`, which calls `extractAgentFields`; that path inherits the change transparently.
- `internal/cli/describe.go`, `partialFillAgentCommand` — does not build the full map. It resolves a single requested model key (the resolved `default_model` or `--model` override) to its id, looking the key up directly and applying the same string-or-`id` fallback. Unknown keys pass through as the literal id.

VERIFIED (do not re-investigate): all three files already import the package as `internalcue "github.com/start-cli/start/internal/cue"`. No new import wiring is needed at the call sites.

VERIFIED (do not re-investigate): `internal/cue/` is a leaf package for this purpose — it holds `keys.go`, `loader.go`, `validator.go`, and `errors.go`, and does not import `internal/cli` or `internal/orchestration`. Adding the helper there introduces no import cycle. `internal/cli/keys.go` already centralises CUE key constants (`KeyAgents`, `KeyRoles`, etc.).

VERIFIED (do not re-investigate): `internal/cli/list.go` also looks up a `models` field, but it walks the registry index metadata where `models` is a list of strings (`modelsVal.List()`), not the agent-config alias map. It is a different shape with different semantics and is explicitly out of scope.

Nil-versus-empty behaviour to preserve: in both `decodeAgentValue` and `extractAgentFields`, the agent's models map is left nil when the agent has no `models` field, and is a non-nil map (possibly empty if every entry is malformed) when the field is present. The helper must reproduce this: return nil when the `models` field is absent, and a non-nil map otherwise.

Existing tests that pin object-form behaviour on the display surfaces, both using the `snapshotObjectFormAgentCue` fixture in `internal/cli/snapshots_test.go`:

- `TestSnapshot_DescribeAgentObjectForm` — pins `start describe <agent>` for an object-form agent. Covers both the `Models:` block (via `decodeAgentValue`) and the resolved `Command:` line (via `partialFillAgentCommand`).
- `TestSnapshot_ConfigGetAgentObjectForm` — pins `start config get <agent>` for the object-form fixture.

The executor's `extractAgentFields` object-form path is not currently pinned by a dedicated test; the new helper unit tests provide that coverage at the shared home.

Tests use table-driven cases and real CUE compilation. The `internal/cue` package compiles CUE in isolation with the context's `CompileBytes`/`CompileString`; existing `internal/cue/*_test.go` files demonstrate the pattern for building a `cue.Value` from a literal source string in a unit test.

## References

- GitHub issue #2 ("refactor(cue): hoist agent models walk into shared helper"), `git@github.com:start-cli/start.git` — the originating request. Note: it cites a third snapshot test name `TestSnapshot_ConfigInfoAgentObjectForm`; the live name is `TestSnapshot_ConfigGetAgentObjectForm`. Its line numbers for the three sites have also drifted; locate the functions by name, not by line.
- Pre-commit review that surfaced the duplication: `.start/reviews/2026-05-16-pre-commit-01.md`, finding L1.

## Requirements

1. Shared helper. Add an exported function to `internal/cue/` that accepts an agent `cue.Value`, performs the `models` lookup internally, and returns a `map[string]string` of alias to resolved id. It accepts both the simple string form and the object `id` form, skips any entry that resolves to neither, returns nil when the agent has no `models` field, and returns a non-nil (possibly empty) map when the field is present. The single-entry string-or-`id` resolution should be factored so the function is built on one resolution primitive rather than inlining the fallback twice.

2. Full-map call sites. `decodeAgentValue` and `extractAgentFields` obtain their models map from the new helper instead of walking the field inline. The nil-versus-empty behaviour described in Current State is preserved.

3. Single-key call site. `partialFillAgentCommand` resolves its requested model key through the shared logic rather than its own inline fallback. Resolving via the helper map and indexing the requested key is acceptable: indexing a nil map is safe, and an absent or malformed key must still fall through unchanged so the literal id passes through, matching current behaviour.

4. Key constants. If the `models` and `id` field names are introduced as constants in `internal/cue/keys.go`, use them at the helper and remove the corresponding string literals. Do not introduce constants that are then left unused.

5. Helper unit tests. Add table-driven tests at the helper's new home covering: simple form, object form, a mix of both in one agent, an entry that is neither string nor object-with-`id` (skipped), an agent with no `models` field (nil result), and an empty `models` map (non-nil empty result).

6. Snapshot tests retained. `TestSnapshot_DescribeAgentObjectForm` and `TestSnapshot_ConfigGetAgentObjectForm` remain and continue to pass unchanged, confirming the display surfaces still accept the object form after the refactor.

## Constraints

- Go and CUE only; use the existing `cuelang.org/go/cue` API already used by the call sites. No new third-party dependencies.
- No behaviour change on any surface. Output of `describe`, `config get`, `list`, and agent execution is identical before and after, including for the object form, malformed entries, and the absent-`models` case.
- The helper lives in `internal/cue/` so both `internal/cli` and `internal/orchestration` can consume it without an import cycle.
- `internal/cli/list.go` is not modified.
- Run `scripts/invoke-tests` (lint plus tests) and resolve all findings before completion.

## Implementation Plan

1. Add the helper and its single-entry resolution primitive to a new file in `internal/cue/` (for example `models.go`). Implement the absent-field nil result and present-field non-nil result.

2. If introducing key constants, add `models` and `id` to `internal/cue/keys.go` and use them in the helper.

3. Migrate `decodeAgentValue` (`internal/cli/config_types.go`) to assign its models map from the helper. Update or remove the doc comment that concedes the duplication, since it no longer applies.

4. Migrate `extractAgentFields` (`internal/orchestration/executor.go`) to assign its models map from the helper.

5. Migrate `partialFillAgentCommand` (`internal/cli/describe.go`) to resolve the requested key through the helper, preserving the unknown-key pass-through.

6. Add the table-driven helper unit tests in `internal/cue/`.

7. Run the full pipeline (`scripts/invoke-tests`) and confirm the two retained snapshot tests pass.

## Acceptance Criteria

- A single exported helper in `internal/cue/` owns the agent `models` walk, and no inline `String()`-then-`id` fallback for an agent `models` map remains in `internal/cli/config_types.go`, `internal/orchestration/executor.go`, or `internal/cli/describe.go`.
- `decodeAgentValue`, `extractAgentFields`, and `partialFillAgentCommand` all route through the helper.
- The helper returns nil for an agent with no `models` field and a non-nil map when the field is present.
- Table-driven helper tests cover simple, object, mixed, malformed-skip, absent-field, and empty-map cases.
- `TestSnapshot_DescribeAgentObjectForm` and `TestSnapshot_ConfigGetAgentObjectForm` pass unchanged.
- `internal/cli/list.go` is unchanged.
- `scripts/invoke-tests` passes.
