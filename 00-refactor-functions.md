# Deduplicate per-category metadata writers shared by describe and config info

## Goal

Eliminate the field-selection and label-formatting duplication that emerged between `internal/cli/describe.go` (the new `printMetadataBlock` and its four `writeXxxMetadata` writers) and `internal/cli/config_info.go` (the long-standing `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` formatters). Both sites today render the same labelled lines for the same category-specific fields with the same skip-when-empty / always-emit rules; a field rename or new field has to be applied in both places with no compiler help if one site is missed. After this project, a single per-category writer is the source of truth for metadata rendering and both `start describe <name>` and `start config info <name>` route through it.

## Scope

In scope:

- Extract one shared metadata writer per category (agent, role, context, task) that takes a strongly-typed value and writes the labelled lines listed in `04-extend-describe.md` requirement 2.
- Have `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` in `config_info.go` call the shared writer after their `Source:` / `Origin:` header.
- Have `printMetadataBlock` (in `describe.go`) call the shared writer.
- Decide and apply one source-of-data strategy for the describe path (decode `cue.Value` into the typed struct, or widen `loadXxxForScope` to accept `config.Scope`). The implementer chooses; the constraint is that both call sites end up passing the same typed value into the shared writer.
- Delete the four `writeXxxMetadata` functions and any `cueLookup*` helpers that exist only to support them.
- Preserve byte-for-byte output of both commands for non-empty and empty field combinations.

Out of scope:

- The `Source:` / `Origin:` / `File:` / `Command:` header lines in `printXxxInfo`. They are config-info-specific framing and stay where they are.
- The verbose-dump framing in `printVerboseDump` (header, separators, Config, Origin, Cache, CUE Definition, File, Command, file contents). Only the metadata block called from inside it is touched.
- Any change to JSON output (`ConfigListItem`, `buildConfigListItem`, `--json` shapes).
- The `print*Info` interactive selection helpers (`runConfigInfoInteractive`, `promptSelectConfigMatch`, `promptSelectCategory`, `promptSelectOneFromList`).
- Field-set changes. The set of fields rendered per category does not change.
- The `_, _ = fmt.Fprintln(...)` lint pattern noted in the repo-root `todo.md`. That is a separate concern with its own follow-up.
- Any change under `library/` or `homebrew-tap/`.
- Releases or tags.

## Current State

This project assumes project `04-extend-describe.md` has completed. `internal/cli/describe.go` contains `printMetadataBlock` plus the four `writeXxxMetadata` writers and the `cueLookupString` / `cueLookupBool` / `cueLookupStringList` / `cueLookupStringMap` helpers. `internal/cli/config_info.go` contains the four `printXxxInfo` formatters unchanged from before project 04.

### Duplicated rendering rules

For each category, the two sites agree on this exact set of lines and these exact emission rules. Any change to this table must update both sites today.

Agents:

- `Description: <text>` when non-empty
- `Bin: <bin>` when non-empty (config_info also emits `Command: <command>` which is config-info-only and stays put)
- `Default Model: <model>` when non-empty
- `Tags: <comma-joined>` when non-empty
- `Models:` followed by indented `<alias> -> <id>` lines, sorted by alias, when the map is non-empty

Roles:

- `Description: <text>` when non-empty
- `Prompt: <truncatePrompt(prompt, 100)>` when non-empty
- `Optional: true` only when true (never `Optional: false`)
- `Tags: <comma-joined>` when non-empty

Contexts:

- `Description: <text>` when non-empty
- `Prompt: <truncatePrompt(prompt, 100)>` when non-empty
- `Required: <bool>` always
- `Default: <bool>` always
- `Tags: <comma-joined>` when non-empty

Tasks:

- `Description: <text>` when non-empty
- `Prompt: <truncatePrompt(prompt, 100)>` when non-empty
- `Role: <role>` when non-empty
- `Tags: <comma-joined>` when non-empty

`truncatePrompt` lives in `internal/cli/config_helpers.go:506`. It stays shared.

`tui.ColorDim` provides the label colouring; `tui.ColorBlue` provides the `->` arrow in the Models block. Colour usage stays as-is.

### Source-of-data mismatch between the two sites

`config_info.go` reads decoded typed structs:

- `loadAgentsForScope(local bool) (map[string]AgentConfig, []string, error)` at `internal/cli/config_types.go:30`
- `loadRolesForScope(local bool) (map[string]RoleConfig, []string, error)` at `internal/cli/config_types.go:221`
- `loadContextsForScope(local bool) (map[string]ContextConfig, []string, error)` at `internal/cli/config_types.go:345`
- `loadTasksForScope(local bool) (map[string]TaskConfig, []string, error)` at `internal/cli/config_types.go:472`

These loaders take a `bool` (local-only or merged), not `config.Scope`. They are called from ~15 sites: `config_interactive.go`, `config_helpers.go`, `config_list.go`, `config.go`, `config_edit.go`, `config_info.go`.

`describe.go` reads `cue.Value` directly via `cueLookupString` / `cueLookupBool` / `cueLookupStringList` / `cueLookupStringMap`. The describe handler operates on `cue.Value` for reasons that remain valid:

- `formatCUEDefinition(v cue.Value)` pretty-prints CUE syntax.
- `orchestration.ExtractOrigin(v cue.Value)` and `orchestration.ExtractUTDFields(v cue.Value)` read structured fields.
- The describe path supports `--global` / `--local` / merged via `config.Scope`. The typed loaders today expose only `local bool`.

Either side can be adapted to feed a shared writer. The implementer chooses (see Implementation Plan step 1).

### Struct definitions

`AgentConfig`, `RoleConfig`, `ContextConfig`, `TaskConfig` are defined in `internal/cli/config_types.go`. Tags today are `json:"..."` with `omitempty` for optional fields. `DefaultModel` has the JSON tag `defaultModel` (camelCase) while the CUE field is `default_model` (snake_case). Direct `cue.Value.Decode(&AgentConfig{})` is not guaranteed to work without an additional `cue:"default_model"` tag or a rename. If the implementer chooses the decode-into-struct approach (Implementation Plan step 1 option A), tagging is the cleanest answer; adding `cue` tags is additive and does not alter JSON output.

### Cross-file call inventory

Sites that call the legacy `printXxxInfo` formatters and stay as-is at the call-site level (they keep calling these functions; the functions themselves change to delegate to the new shared writers):

- `internal/cli/config_info.go:128-140` — `printConfigInfo` dispatch.

Sites that call into `printMetadataBlock` and stay as-is at the call-site level:

- `internal/cli/describe.go:519` — inside `printVerboseDump`.

There are no other callers. The shared writer is invoked by exactly two paths.

### Test files affected

- `internal/cli/describe_test.go`: `TestVerboseDumpMetadataBlock`, `TestVerboseDumpMetadataBlock_ModelsSortedByAlias`, `TestVerboseDumpMetadataBlock_PlacementBetweenCacheAndCUE`, `TestVerboseDumpMetadataBlock_EmptyDoesNotInsertBlankLine`. These continue to pass unchanged after the refactor — the public behaviour does not change.
- `internal/cli/config_test.go`: tests covering `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo`. Same — output must not change.
- New unit tests for the shared writer cover the per-category field rules in isolation, exercising the typed-struct input path so the writer can be tested without either of the two surrounding renderers.

### Codebase conventions confirmed

- `internal/cli/` is a single Go package; new files do not change package boundaries.
- `tui.ColorDim.Sprint` for labels and `tui.ColorBlue.Fprint` for the `->` arrow are the canonical colour calls used by both existing writers.
- `truncatePrompt(s, 100)` is the canonical truncation for the verbose surfaces; `truncatePrompt(s, 50)` is used by `config_edit.go` for inline edits and is out of scope.

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

- `internal/orchestration/executor.go:381-386` — runtime site (loads agents for execution). Accepts both forms: tries `modelVal.String()`, falls back to `modelVal.LookupPath("id").String()`.
- `internal/cli/describe.go:847-854` (`partialFillAgentCommand`) — runtime site (substitutes `{{.model}}` in command templates). Accepts both forms with the same try-string-then-id-field pattern.
- `internal/cli/config_types.go:82-93` (`loadAgentsFromDir`, feeds `printAgentInfo`) — display site. Accepts simple form only; an object-form entry is silently dropped.
- `internal/cli/describe.go:708-726` (`cueLookupStringMap`, feeds `writeAgentMetadata`) — display site. Accepts simple form only; an object-form entry is silently dropped.

Resulting bug: an agent declared with object-form models renders `Models:` followed by zero rows in both `start config info <agent>` and `start describe <agent>`. The agent still executes correctly because the two runtime sites accept the form.

In-tree exposure: no current CUE data uses the object form. All testdata (`test/testdata/valid/agent_full.cue`, `test/integration/registry/settings.cue`, `test/testdata/merge/...`) uses the simple form. The bug is latent — no live invocation reproduces it, but any user who hand-authors an agent with object-form models will hit it.

Why this matters for the refactor: the shared writer this project introduces is the single display reader for agent models. The implementer must decide what form(s) the writer accepts. Doing so consciously (and aligning the two display sites to the runtime sites) costs ~3 lines of reader code and one test fixture, removes the asymmetry, and means the schema-vs-defensive question gets settled in one place. See requirement 6.

## References

- Pre-commit review at `.start/reviews/2026-05-12-pre-commit-01.md` finding M1 — the duplication observation that motivated this project.
- `04-extend-describe.md` requirement 2 — the canonical per-category field list that both sites must render.

## Requirements

### 1. Single shared writer per category

One function per category writes the labelled metadata lines listed in Current State. The writer takes a typed value and an `io.Writer`. It does not emit a leading blank line, a trailing blank line, headers, separators, `Source:`, `Origin:`, `File:`, `Command:`, or any other framing. It writes only the category-specific metadata lines.

Output must match the existing output byte-for-byte for every input combination already exercised by `internal/cli/describe_test.go` and `internal/cli/config_test.go`. Snapshot equality before and after the refactor is a hard requirement, not a soft one.

### 2. Both renderers delegate to the shared writer

`printMetadataBlock` in `describe.go` produces its output by calling the shared writer. The leading blank-line emission and the "emit nothing when the writer produces nothing" rule remain on `printMetadataBlock`'s side, not the shared writer's side.

`printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` in `config_info.go` produce their post-header output (everything currently after the `Source:` / `Origin:` lines and before the trailing separator) by calling the shared writer. The `Description:` blank-line-before-section behaviour seen in `printRoleInfo:227`, `printContextInfo:279`, `printTaskInfo:331` — a `fmt.Fprintln(w)` before the `Description:` label when description is non-empty — stays as it is observable today.

The agent renderer's existing "blank line before `Models:`" behaviour at `config_info.go:185` is preserved.

### 3. Describe path source-of-data strategy

The describe path supplies the shared writer with the same typed value config-info supplies. The implementer chooses between two approaches; either is acceptable provided requirement 1's byte-for-byte equivalence holds and no resolver / scope semantics change.

Option A: decode `cue.Value` into the existing typed struct. Add `cue:"..."` tags to fields whose CUE name differs from the Go-field-name-derived default (specifically `DefaultModel` -> `default_model`). Confirm by unit test that all four structs round-trip from CUE source.

Option B: widen `loadAgentsForScope` / `loadRolesForScope` / `loadContextsForScope` / `loadTasksForScope` to accept `config.Scope` and update every caller. The describe handler then calls the loader for its in-scope item rather than walking `cue.Value` for metadata. Existing `cue.Value`-based code in `describe.go` (`formatCUEDefinition`, `orchestration.ExtractOrigin`, `orchestration.ExtractUTDFields`, file path / command rendering) is unaffected.

Whichever option is taken, the four `cueLookup*` helpers and the four `writeXxxMetadata` functions are removed.

### 4. Test parity

The existing `TestVerboseDumpMetadataBlock*` tests pass without source changes. The existing `print*Info` tests pass without source changes.

A new unit-test file exercises the shared writer in isolation per category: all-fields-populated, no-fields-populated, and the category-specific edge cases (agent `Models:` alias sort order; role `Optional: true` only when true; context always emits Required/Default).

### 5. Removal

After the refactor, `internal/cli/describe.go` no longer contains `writeAgentMetadata`, `writeRoleMetadata`, `writeContextMetadata`, `writeTaskMetadata`, `cueLookupString`, `cueLookupBool`, `cueLookupStringList`, or `cueLookupStringMap`. The shared writers replace them entirely.

### 6. Agent models reader unification

The shared agent-metadata writer reads the `models` map in a way that accepts both the simple form (`models: { sonnet: "<id>" }`) and the object form (`models: { sonnet: { id: "<id>" } }`), matching the existing behaviour of `internal/orchestration/executor.go:381-386` and `internal/cli/describe.go` `partialFillAgentCommand` lines 847-854.

The reader logic is one place after this project. If the implementer chooses Option A (decode into struct), the reader is the per-field `cue.Value` walk used to populate `AgentConfig.Models`. If the implementer chooses Option B (typed loaders), the reader is inside `loadAgentsFromDir` at `config_types.go:82-93`, which currently accepts simple form only and must be widened to also try the `id` sub-field on string-decode failure.

After this requirement is met:

- `start describe <object-form-agent>` renders every alias declared in `models` with its resolved id.
- `start config info <object-form-agent>` renders every alias declared in `models` with its resolved id.
- The four model readers (two runtime, two display) all accept both forms — the asymmetry described under Current State is gone.

`partialFillAgentCommand` and `executor.go`'s model loop are not modified by this project. They already accept both forms; the project aligns the display sites to match them, not the other way around. Tightening the runtime sites to schema-canonical-only (deleting the object-form fallbacks at `executor.go:381-386` and `describe.go:847-854`) is a separate concern and is out of scope here.

A unit-test fixture exercises object-form models against the shared writer (and against the typed loader under Option B). The fixture renders an agent with `models: { sonnet: { id: "claude-sonnet-..." } }` and asserts that the writer emits `sonnet -> claude-sonnet-...` exactly as it would for the simple form.

## Implementation Plan

1. Pick a source-of-data strategy (Option A decode-into-struct, or Option B widen typed loaders). Document the choice in a one-line commit message body. If Option B is chosen, the loader widening lands in its own commit ahead of the writer extraction so the diffs stay reviewable.
2. Add the new shared writers (one per category) in a new file. Implementer picks the file name and the function signatures; both must accept a typed value and an `io.Writer`.
3. Replace the bodies of `writeAgentMetadata` / `writeRoleMetadata` / `writeContextMetadata` / `writeTaskMetadata` in `describe.go` with calls to the new writers. Verify output is unchanged by re-running `internal/cli/describe_test.go`.
4. Replace the post-header section of `printAgentInfo` / `printRoleInfo` / `printContextInfo` / `printTaskInfo` in `config_info.go` with calls to the new writers. Verify output is unchanged by re-running `internal/cli/config_test.go`.
5. Delete `writeAgentMetadata` / `writeRoleMetadata` / `writeContextMetadata` / `writeTaskMetadata` from `describe.go`. Delete `cueLookupString` / `cueLookupBool` / `cueLookupStringList` / `cueLookupStringMap` from `describe.go` if they have no remaining callers (verify with `rg`).
6. Add the per-category unit tests for the shared writer described in requirement 4.
7. Run `gofmt -w .`, `go build ./...`, `go vet ./...`, and `scripts/invoke-tests`.

## Constraints

- Output of `start describe <name>` and `start config info <name>` must be byte-for-byte unchanged for all in-tree CUE data. The agent-models reader unification (requirement 6) is a behaviour change for object-form `models` declarations only — that form is not used by any fixture in the repository, so byte-for-byte equivalence against existing fixtures continues to hold. New fixtures added under requirement 6 are net-new assertions, not regressions.
- No change to JSON output paths.
- No change to resolver behaviour, scope semantics, or auto-install side effects.
- No deletion of `truncatePrompt` or the shared `tui` colour helpers.
- Do not introduce a new third-party dependency.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Acceptance Criteria

- `rg -n 'writeAgentMetadata|writeRoleMetadata|writeContextMetadata|writeTaskMetadata' internal/cli/` returns no matches.
- `rg -n 'cueLookupString\b|cueLookupBool\b|cueLookupStringList\b|cueLookupStringMap\b' internal/cli/` returns no matches.
- Exactly one shared writer per category exists in the package; each is called from at least two distinct sites (the describe verbose dump and the corresponding `printXxxInfo`).
- `internal/cli/describe_test.go` passes unchanged.
- `internal/cli/config_test.go` passes unchanged.
- New per-category unit tests for the shared writer exist and pass.
- `start describe <installed-agent>` and `start config info <installed-agent>` produce the same metadata lines they produced before the refactor (verified by manual diff against a captured baseline, or by snapshot fixtures if the implementer prefers).
- A new fixture/test renders an agent declared with object-form `models: { sonnet: { id: "<id>" } }` and asserts that both `start describe` and `start config info` emit a populated `Models:` block with the same `<alias> -> <id>` lines they would emit for the simple form. The four model readers (two runtime, two display) all accept both forms.
- `scripts/invoke-tests` passes.
