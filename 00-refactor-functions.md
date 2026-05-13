# Deduplicate per-category metadata writers shared by describe and config info

## Goal

Eliminate the field-selection and label-formatting duplication that emerged between `internal/cli/describe.go` (the new `printMetadataBlock` and its four `writeXxxMetadata` writers) and `internal/cli/config_info.go` (the long-standing `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` formatters). Both sites today render the same labelled lines for the same category-specific fields with the same skip-when-empty / always-emit rules; a field rename or new field has to be applied in both places with no compiler help if one site is missed. After this project, a single per-category writer is the source of truth for metadata rendering and both `start describe <name>` and `start config info <name>` route through it.

## Decision: Option A (decode cue.Value into typed struct)

The implementer route is Option A: in describe, decode the per-item `cue.Value` into the existing `AgentConfig` / `RoleConfig` / `ContextConfig` / `TaskConfig` struct and pass that typed value to the shared writer. The alternative (Option B: widen the four `loadXxxForScope` loaders to accept `config.Scope` and call them from describe) is rejected for three reasons:

1. Describe still needs `cue.Value` for `formatCUEDefinition`, `orchestration.ExtractOrigin`, `orchestration.ExtractUTDFields`, and `partialFillAgentCommand`. Option B does not remove `cue.Value` from describe; it adds a second load on top of the existing one.
2. Option B ripples through ~15 callers of `loadXxxForScope(localOnly bool)` across `config_interactive.go`, `config_edit.go`, `config_list.go`, `config_helpers.go`, `config.go`, and `config_info.go`. None of those callers need the third (global-only) scope today, so the signature widening is mechanical busywork buying nothing for the callers themselves.
3. The `config get` work that will replace `config info` is on the horizon. Pre-committing to "typed loaders are the way" before that design decision is made is premature. Option A leaves both paths open.

The original Option A / Option B fork in this project is collapsed accordingly.

The merge-semantics concern raised against Option B during the design discussion turned out to be moot. Both `internalcue.Loader.Load` (used by describe and the runtime) and the typed `loadXxxForScope` loaders implement the same struct-replace-by-name semantics; the apparent divergence was a misread of `mergeWithReplacement` at `internal/cue/loader.go:140`. Option A is preferred for the three reasons above, not for semantics.

## Scope

In scope:

- Extract one shared metadata writer per category (agent, role, context, task) that takes a strongly-typed value and an `io.Writer`.
- Decode `cue.Value` into the typed struct on the describe side. Add `cue:"..."` tags where the CUE field name differs from the Go-field-name-derived default (specifically `AgentConfig.DefaultModel` -> `cue:"default_model"`).
- Have `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` in `config_info.go` call the shared writer after their `Source:` / `Origin:` header.
- Have `printMetadataBlock` (in `describe.go`) call the shared writer.
- Reorder `printAgentInfo` so that Bin and Default Model move from the header section into the metadata section, matching describe's line order. This is an intentional user-visible change to `start config info <agent>`.
- Delete the four `writeXxxMetadata` functions and the `cueLookupString` / `cueLookupBool` / `cueLookupStringList` / `cueLookupStringMap` helpers from `describe.go`.
- Widen `loadAgentsFromDir` at `config_types.go:82-93` to accept both the simple and the object form for the `models` field. This is independent of the writer extraction but lands in the same project.
- Capture expected-output strings for `start describe` and `start config info` for all four categories before starting, plus one new fixture exercising object-form agent models.
- The shared agent writer emits a blank line before the `Models:` section. This adds a blank line to `start describe <agent>` between Tags and Models that does not exist today; it preserves the current `start config info <agent>` blank-before-Models behaviour. See "Agent Models spacing" in Current State.

Out of scope:

- The `Source:` / `Origin:` / `File:` / `Command:` header lines in `printXxxInfo`. They are config-info-specific framing and stay where they are.
- The verbose-dump framing in `printVerboseDump` (header, separators, Config, Origin, Cache, CUE Definition, File, Command, file contents). Only the metadata block called from inside it is touched.
- Any change to JSON output (`ConfigListItem`, `buildConfigListItem`, `--json` shapes).
- The `print*Info` interactive selection helpers (`runConfigInfoInteractive`, `promptSelectConfigMatch`, `promptSelectCategory`, `promptSelectOneFromList`).
- Field-set changes. The set of fields rendered per category does not change.
- The `_, _ = fmt.Fprintln(...)` lint pattern noted in the repo-root `todo.md`. That is a separate concern with its own follow-up.
- Any change under `library/` or `homebrew-tap/`.
- Releases or tags.
- Scope-flag plumbing (`--global` asymmetry, `--local` semantic overload, `ScopeMerged` misnaming, default-merge undocumented). Tracked in `01-scope-flag-cleanup.md`.

## Current State

This project assumes project `04-extend-describe.md` has completed. `internal/cli/describe.go` contains `printMetadataBlock` plus the four `writeXxxMetadata` writers and the `cueLookupString` / `cueLookupBool` / `cueLookupStringList` / `cueLookupStringMap` helpers. `internal/cli/config_info.go` contains the four `printXxxInfo` formatters unchanged from before project 04.

### Duplicated rendering rules

For each category, the two sites agree on the set of fields rendered. The line ordering differs only for agents — see "Agent line ordering" below.

Agents:

- `Description: <text>` when non-empty
- `Bin: <bin>` when non-empty (config_info also emits `Command: <command>` which is config-info-only and stays in the header)
- `Default Model: <model>` when non-empty
- `Tags: <comma-joined>` when non-empty
- Blank line (emitted by the shared writer when Models is non-empty — see Agent Models spacing)
- `Models:` followed by indented `<alias> -> <id>` lines, sorted by alias, when the map is non-empty.

Roles:

- `Description: <text>` when non-empty. Preceded by a blank line in config_info.
- `Prompt: <truncatePrompt(prompt, 100)>` when non-empty
- `Optional: true` only when true (never `Optional: false`)
- `Tags: <comma-joined>` when non-empty

Contexts:

- `Description: <text>` when non-empty. Preceded by a blank line in config_info.
- `Prompt: <truncatePrompt(prompt, 100)>` when non-empty
- `Required: <bool>` always
- `Default: <bool>` always
- `Tags: <comma-joined>` when non-empty

Tasks:

- `Description: <text>` when non-empty. Preceded by a blank line in config_info.
- `Prompt: <truncatePrompt(prompt, 100)>` when non-empty
- `Role: <role>` when non-empty
- `Tags: <comma-joined>` when non-empty

`truncatePrompt` lives in `internal/cli/config_helpers.go:506`. It stays shared.

`tui.ColorDim` provides the label colouring; `tui.ColorBlue` provides the `->` arrow in the Models block. Colour usage stays as-is.

### Agent line ordering

The describe writer (`writeAgentMetadata`) emits Description -> Bin -> Default Model -> Tags -> Models, contiguously, no internal blank lines.

The config-info writer (`printAgentInfo`) today emits a header section (Source, Origin, Bin, Command, Default Model) followed by a metadata section (Description, Tags, blank line, Models). Bin and Default Model live in the header section; a blank line separates Tags from Models.

These two orderings cannot both be served by a single shared writer without duplicating those two fields or reordering one of the call sites. The decision is to change `printAgentInfo` so that Bin and Default Model move out of the header and into the shared writer's output. Roles, contexts, and tasks already agree on order; only agents reorder.

### Agent Models spacing

A second wrinkle: today's describe agent block emits Models directly after Tags with no blank line between; today's config-info agent block emits a blank line between Tags and Models. A single shared writer cannot preserve both behaviours — the call-site cannot inject a blank line in the middle of the writer's output.

Resolution: the shared agent writer emits a blank line before `Models:` when the models map is non-empty. This:

- Preserves config-info's current blank-before-Models behaviour.
- Adds a blank line to describe's agent output between Tags and Models that does not exist today.

This is the third intentional user-visible change introduced by this project (alongside the line reorder in requirement 7 and the object-form models fix in requirement 6). It applies only to agents; roles, contexts, and tasks have no internal blank-line variations to reconcile.

The `TestVerboseDumpMetadataBlock*` tests use `Contains` assertions and survive the added blank line without modification. The pre-refactor baseline literal for `start describe <agent>` captured in implementation-plan step 1 gets updated in step 6 to include the new blank line.

The new `printAgentInfo` layout:

```
agents:<name>
─────...
Source: <source>
Origin: <origin>      (if non-empty)
Command: <command>

Description: <text>   (if non-empty, preceded by blank line emitted by call-site)
Bin: <bin>            (if non-empty)
Default Model: <m>    (if non-empty)
Tags: <tags>          (if non-empty)

Models:               (if non-empty, preceded by blank line emitted by the shared writer — see Agent Models spacing)
  alias -> id
  ...
─────...
```

### Source-of-data strategy

Describe decodes `cue.Value` into the typed struct via `cue.Value.Decode`. Fields whose CUE name differs from the Go default get `cue:"..."` tags:

- `AgentConfig.DefaultModel` -> `cue:"default_model"`

Other struct fields either match by name (`description`, `file`, `command`, `prompt`, `tags`, `optional`, `required`, `default`, `role`, `origin`) or are populated outside the CUE decode (`Source`, `Name`). For the latter, `cue.Value.Decode` (cue v0.16.1) is expected to leave them zero when the field is absent from the CUE value. If decode rejects unknown Go fields, the implementer verifies the correct tag syntax for skipping (the codebase has no existing `cue:"..."` tags to crib from). Confirm by unit test per struct.

`AgentConfig.Models map[string]string` does not decode the object form via `cue.Value.Decode` because the schema declares the value type as `string` while the object form supplies a struct. The describe-side decoder must therefore hand-roll the models field after the `cue.Value.Decode` call, using the try-string-then-`id` pattern, and assign the resulting map to `AgentConfig.Models` before passing the struct to the shared writer. The writer itself reads `agent.Models` as an already-populated `map[string]string` and does no CUE parsing.

### Agent models field: form support asymmetry

The agent `models` field has two shapes in the codebase, and the four sites that read it disagree on which to accept.

Schema-canonical form (only this is permitted by the schema at `test/testdata/schemas/agent.cue`, which declares `models?: [string]: string & !=""`):

```cue
models: {
    sonnet: "claude-sonnet-4-20250514"
    opus:   "claude-opus-4-20250514"
}
```

Object form (not schema-permitted, but defensively supported in two runtime sites):

```cue
models: {
    sonnet: { id: "claude-sonnet-4-20250514" }
    opus:   { id: "claude-opus-4-20250514" }
}
```

Reader behaviour today:

- `internal/orchestration/executor.go:381-386` — runtime site. Accepts both forms.
- `internal/cli/describe.go:847-854` (inside `partialFillAgentCommand`) — runtime site. Accepts both forms.
- `internal/cli/config_types.go:82-93` (`loadAgentsFromDir`) — feeds `AgentConfig.Models`. Accepts simple form only; an object-form entry is silently dropped.
- `internal/cli/describe.go:708-726` (`cueLookupStringMap`) — display site, being removed by this project. Accepts simple form only.

`AgentConfig.Models` populated by `loadAgentsFromDir` is consumed by:

- `internal/cli/resolve.go:200,212` — `--model` flag resolution (lookup by alias).
- `internal/cli/config_edit.go:172,193` — interactive model prompts.
- `internal/cli/config_list.go:52,131` — display and JSON output.
- `internal/cli/config_info.go:184` — display.

The latent bug therefore affects four consumers, not one. Widening `loadAgentsFromDir` for both forms fixes all of them in one place. Describe does not go through `loadAgentsFromDir`; it has its own decoder that reads `cue.Value` and produces `AgentConfig`. That decoder must also accept both forms when populating `AgentConfig.Models`. Implementer may factor the both-forms walk into a shared helper called by both `loadAgentsFromDir` and the describe-side decoder, or duplicate the three-line pattern in each site — either is acceptable.

In-tree exposure: no current CUE data uses the object form. All testdata (`test/testdata/valid/agent_full.cue`, `test/integration/registry/settings.cue`, `test/testdata/merge/...`) uses the simple form. The bug is latent — no live invocation reproduces it, but a user who hand-authors an agent with object-form models hits it.

### Cross-file call inventory

Sites that call the legacy `printXxxInfo` formatters and stay as-is at the call-site level (the functions themselves change to delegate to the new shared writers):

- `internal/cli/config_info.go:128-140` — `printConfigInfo` dispatch.

Sites that call into `printMetadataBlock` and stay as-is at the call-site level:

- `internal/cli/describe.go:519` — inside `printVerboseDump`.

There are no other callers. The shared writer is invoked by exactly two paths.

### Test files affected

- `internal/cli/describe_test.go`: `TestVerboseDumpMetadataBlock`, `TestVerboseDumpMetadataBlock_ModelsSortedByAlias`, `TestVerboseDumpMetadataBlock_PlacementBetweenCacheAndCUE`, `TestVerboseDumpMetadataBlock_EmptyDoesNotInsertBlankLine`. These continue to pass unchanged (Contains-style assertions tolerate the new blank line in `start describe <agent>`).
- `internal/cli/config_test.go`: `TestConfigInfo_Agent` and related. The existing assertions are `Contains`-style and survive the line reorder unchanged — they are not modified by this project. The new snapshot tests (below) are what pin the new line ordering.
- New per-category unit tests for the shared writer cover the field rules in isolation.
- New snapshot tests for each of the eight rendering combinations: `start describe <a|r|c|t>` and `start config info <a|r|c|t>`, asserting exact-string equality against captured baselines. After the refactor, three baselines reflect the intentional changes (describe agent gains a blank line; config-info agent reorders; both object-form agent surfaces populate Models). The remaining five stay byte-identical to the step-1 captures. These tests become permanent regression guards.
- New fixture: an agent declared with object-form `models: { sonnet: { id: "..." } }`. Asserts that `loadAgentsFromDir`, `printAgentInfo`, `printMetadataBlock`, `start describe`, `start config info`, `--model` resolution, `config list --json`, and `config edit` all render or read the models correctly.

### Codebase conventions confirmed

- `internal/cli/` is a single Go package; new files do not change package boundaries.
- `tui.ColorDim.Sprint` for labels and `tui.ColorBlue.Fprint` for the `->` arrow are the canonical colour calls used by both existing writers.
- `truncatePrompt(s, 100)` is the canonical truncation for the verbose surfaces; `truncatePrompt(s, 50)` is used by `config_edit.go` for inline edits and is out of scope.

## References

- Pre-commit review at `.start/reviews/2026-05-12-pre-commit-01.md` finding M1 — the duplication observation that motivated this project.
- `04-extend-describe.md` requirement 2 — the canonical per-category field list that both sites render.
- `01-scope-flag-cleanup.md` — tracks related issues (scope flag asymmetry, default-merge documentation, test coverage) that surfaced during this project's design discussion but are out of scope here.

## Requirements

### 1. Single shared writer per category

One function per category writes the labelled metadata lines listed in Current State. The writer takes a typed value and an `io.Writer`. It does not emit a leading blank line, a trailing blank line, headers, separators, `Source:`, `Origin:`, `File:`, `Command:`, or any other framing. It writes only the category-specific metadata lines.

For roles, contexts, and tasks: output must match the existing output byte-for-byte for every input combination already exercised by `internal/cli/describe_test.go` and `internal/cli/config_test.go`.

For agents: output of `start describe <agent>` gains one blank line between Tags and Models when models are present (see Agent Models spacing). Output of `start config info <agent>` reorders Bin and Default Model from the header section to the metadata section per requirement 7. Both agent-renderer changes are intentional and called out in Constraints. The object-form-models behaviour change (requirement 6) is tracked separately because it applies to a code path orthogonal to the writer extraction.

### 2. Both renderers delegate to the shared writer

`printMetadataBlock` in `describe.go` produces its output by calling the shared writer with a decoded typed value. The leading blank-line emission and the "emit nothing when the writer produces nothing" rule remain on `printMetadataBlock`'s side, not the shared writer's side.

`printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` in `config_info.go` produce their post-header output by calling the shared writer with the typed struct already loaded by `loadXxxForScope`. The `Description:` blank-line-before-section behaviour seen at `config_info.go:176` (agent), `:227` (role), `:279` (context), `:331` (task) stays on the call-site, not the shared writer.

The agent renderer's existing "blank line before `Models:`" behaviour at `config_info.go:185` moves into the shared writer (it cannot stay on the call-site since the writer emits Tags and Models in one pass). See Agent Models spacing.

### 3. Describe path decodes cue.Value into typed struct

The describe handler decodes the per-item `cue.Value` into `AgentConfig` / `RoleConfig` / `ContextConfig` / `TaskConfig` and passes the typed struct to the shared writer. `cue.Value.Decode` is the canonical mechanism; `cue:"..."` tags are added where field names diverge.

The four `cueLookup*` helpers and the four `writeXxxMetadata` functions are removed.

### 4. Test parity and new tests

The existing `TestVerboseDumpMetadataBlock*` tests pass without source changes. The existing `print*Info` `Contains`-style tests pass without source changes.

New unit tests:

- One per category for the shared writer in isolation: all-fields-populated, no-fields-populated, category-specific edge cases (agent `Models:` alias sort order; role `Optional: true` only when true; context always emits Required/Default).
- Eight exact-equality snapshot tests for `start describe <a|r|c|t>` and `start config info <a|r|c|t>`, asserting against captured baselines (see implementation-plan step 1). Three of the eight reflect intentional post-refactor changes; the other five stay byte-identical to their pre-refactor captures. Together these become the regression-guard surface for any future change to rendering.
- Object-form models fixture exercising the loader and all four consumers (resolve, edit, list, info) plus describe.

### 5. Removal

After the refactor, `internal/cli/describe.go` no longer contains `writeAgentMetadata`, `writeRoleMetadata`, `writeContextMetadata`, `writeTaskMetadata`, `cueLookupString`, `cueLookupBool`, `cueLookupStringList`, or `cueLookupStringMap`.

### 6. Agent models reader widened in loadAgentsFromDir

`loadAgentsFromDir` at `config_types.go:82-93` is widened to accept both the simple form and the object form, matching the try-string-then-`id` pattern used by `executor.go:381-386` and `partialFillAgentCommand`.

After this requirement is met:

- `--model` flag resolution finds object-form aliases.
- `config edit` lists object-form aliases in the interactive prompt.
- `config list agent --json` emits object-form aliases in the JSON output.
- `start config info <object-form-agent>` renders every alias declared in `models` with its resolved id.
- `start describe <object-form-agent>` renders every alias declared in `models` with its resolved id.

The describe-side decoder that converts `cue.Value` to `AgentConfig` for the shared writer also accepts both forms when populating `AgentConfig.Models`. Implementer may share this logic with `loadAgentsFromDir` via a helper, or duplicate the three-line pattern — see Agent models field: form support asymmetry above.

`partialFillAgentCommand` and `executor.go`'s model loop are not modified by this project. They already accept both forms; the project aligns the display + resolve + edit + list sites to match them, not the other way around. Tightening the runtime sites to schema-canonical-only is a separate concern and is out of scope.

### 7. Agent line ordering change in printAgentInfo

`printAgentInfo` in `config_info.go` is restructured. After `Source:` / `Origin:` / `Command:`, it calls the shared writer. Bin and Default Model move from the header section to the shared writer's output. The blank line before Description stays on the call-site. The blank line before Models moves into the shared writer (see requirement 2 and Agent Models spacing).

The new layout is documented in Current State / Agent line ordering above.

## Implementation Plan

1. Capture expected-output strings for `start describe <a|r|c|t>` and `start config info <a|r|c|t>` against in-tree fixtures. The project does not currently use golden files; capture as inline string literals in new test functions (consistent with the existing `stdout.String()` assertion style in `config_test.go`). These literals are the pre-refactor baseline that step 11 compares against. Disable colour for the new exact-equality tests — the writers use `tui.ColorDim` and `tui.ColorBlue` which emit ANSI escapes via `fatih/color`. Set `color.NoColor = true` in test setup, or `t.Setenv("NO_COLOR", "1")`, before invoking the command. The existing `Contains`-style tests are unaffected because their substrings are contiguous; only the new exact-equality tests need the colour suppression.
2. Add a new in-tree fixture: an agent declared with object-form `models: { sonnet: { id: "..." } }`. Add corresponding describe and config-info baseline test functions for the fixture (initially with empty `Models:` blocks — the step-3 widening will populate them and the literals get updated then).
3. Widen `loadAgentsFromDir` at `config_types.go:82-93` for both-forms models. Verify by running `--model` resolution, `config edit`, `config list --json`, `config info`, and `start describe` against the new fixture. Commit independently.
4. Add `cue:"default_model"` to `AgentConfig.DefaultModel`. Verify each of the four structs round-trips via `cue.Value.Decode` with a unit test per struct. Commit.
5. Add the four shared writers in a new file (implementer picks the file name). Each accepts the typed struct and an `io.Writer`. Hand-roll the agent models walk inside the shared writer to accept both forms.
6. Replace the bodies of `writeAgentMetadata` / `writeRoleMetadata` / `writeContextMetadata` / `writeTaskMetadata` in `describe.go` with calls to the new writers, decoding `cue.Value` into the typed struct at the call site. For agents the decoder populates `AgentConfig.Models` with both-forms support (see Source-of-data strategy). The existing `TestVerboseDumpMetadataBlock*` tests in `describe_test.go` pass without source modification. The new describe-agent snapshot test (created in step 1) gets its baseline literal updated at this step to include the new blank line between Tags and Models.
7. Restructure `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` in `config_info.go` to delegate to the new writers per requirement 2. For agents, the line reorder lands here. Verify `internal/cli/config_test.go` passes; update or add the snapshot for `config info <agent>`.
8. Delete `writeAgentMetadata` / `writeRoleMetadata` / `writeContextMetadata` / `writeTaskMetadata` from `describe.go`. Delete `cueLookupString` / `cueLookupBool` / `cueLookupStringList` / `cueLookupStringMap` from `describe.go`. Verify with `rg` that no callers remain.
9. Add the per-category unit tests for the shared writer described in requirement 4.
10. Run `gofmt -w .`, `go build ./...`, `go vet ./...`, and `scripts/invoke-tests`.
11. Verify the eight snapshot tests against their captured baselines. Three baseline literals are expected to change, and each is updated in the same commit as the step that introduces the change:
    - `start describe <agent>` (non-object-form): gains one blank line between Tags and Models. Literal updated at step 6, when the shared writer's blank-before-Models behaviour comes online.
    - `start config info <agent>` (non-object-form): lines reorder per requirement 7. Literal updated at step 7, where the reorder lands.
    - Object-form agent fixture for both `describe` and `config info`: now shows populated `Models:` blocks where they previously rendered empty. Literals updated at step 3, when `loadAgentsFromDir` is widened (config-info side) and step 6 when the shared writer comes online (describe side).
    The remaining five baselines (describe r/c/t and config info r/c/t) stay byte-identical to the step-1 captures and act as regression guards.

## Constraints

- Output of `start describe <r|c|t>` (roles, contexts, tasks) is byte-for-byte unchanged.
- Output of `start describe <agent>` gains one blank line between Tags and Models when the models map is non-empty (see Agent Models spacing). Newly populated `Models:` for object-form agents per requirement 6. No other changes.
- Output of `start config info <r|c|t>` (roles, contexts, tasks) is byte-for-byte unchanged.
- Output of `start config info <agent>` reorders Bin and Default Model per requirement 7. Newly populated `Models:` for object-form agents per requirement 6. The blank line before Models (today on the call-site) moves into the shared writer; the resulting output emits the same blank line in the same position. No other changes.
- No change to JSON output paths.
- No change to resolver behaviour, scope semantics, or auto-install side effects.
- No deletion of `truncatePrompt` or the shared `tui` colour helpers.
- Do not introduce a new third-party dependency.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.
- Scope-flag plumbing is explicitly out of scope. Track in `01-scope-flag-cleanup.md`.

## Acceptance Criteria

- `rg -n 'writeAgentMetadata|writeRoleMetadata|writeContextMetadata|writeTaskMetadata' internal/cli/` returns no matches.
- `rg -n 'cueLookupString\b|cueLookupBool\b|cueLookupStringList\b|cueLookupStringMap\b' internal/cli/` returns no matches.
- Exactly one shared writer per category exists in the package; each is called from at least two distinct sites (the describe verbose dump and the corresponding `printXxxInfo`).
- `internal/cli/describe_test.go` passes unchanged.
- `internal/cli/config_test.go` passes; the new agent-info snapshot test passes.
- New per-category unit tests for the shared writer exist and pass.
- `loadAgentsFromDir` accepts both simple and object form for the `models` field.
- The object-form agent fixture renders models correctly via `start describe`, `start config info`, `config list --json`, `config edit`, and `--model` resolution.
- `start describe <agent>` and `start config info <agent>` produce the same metadata lines they produced before the refactor for the agent surfaces, modulo three intentional changes: (i) requirement 7 reorder for `config info <agent>`; (ii) requirement 6 populated Models for object-form agents on both renderers; (iii) one extra blank line between Tags and Models on `describe <agent>` per Agent Models spacing. Verified against the updated baseline literals.
- `scripts/invoke-tests` passes.
- The four model readers all accept both forms: two runtime (`executor.go` `extractAgentFields`, `describe.go` `partialFillAgentCommand`) and two display (`loadAgentsFromDir` and the describe-side decoder that produces `AgentConfig` for the shared writer). The shared writer itself does not parse CUE; it reads `agent.Models` as `map[string]string`.
