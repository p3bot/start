# Project: Multi-Segment Task Instructions

## Goal

Extend `start task` so the instructions for a task may be supplied as any number of trailing positional arguments, each resolved independently as a file path or literal and joined under the same one-blank-line seam rule as `start prompt`. Today a task takes at most one instructions argument; this lifts that cap so a task can be driven by several files and inline strings in one invocation:

```
start task review/pre-commit ./checklist.md "follow the checklist exactly"
```

resolves the task `review/pre-commit` with instructions equal to:

```
<contents of checklist.md>

follow the checklist exactly
```

The first positional remains the task name, address, or file path; every positional after it is an instruction segment. This reuses the segment-composition helper introduced in `01-prompt-arg-feature.md` so `prompt` and `task` share one mental model: trailing positionals are segments joined by exactly one blank line.

It also fixes a related gap. Today, if a task body has no `{{.instructions}}` placeholder, the instructions are silently discarded. This project makes instructions append to the rendered body when no placeholder is present, so arguments the user passed are never silently dropped.

## Scope

In scope:

- Lift `task`'s instructions arity: the first positional is the task name/address/file; all remaining positionals are instruction segments (`cobra.ArbitraryArgs`, with zero args still listing tasks).
- Resolve and join the instruction segments via the `01` helper (`orchestration.ComposeSegments(args[1:], "instructions file")`).
- Template processor change: when a task body contains no `{{.instructions}}` placeholder and instructions are non-empty, append the instructions after the rendered body using the one-blank-line seam, instead of discarding them. When the placeholder is present, substitute in place as today and do not also append.
- Update the command `Use`/`Long` help text and add a metadata test that asserts `Use` (mirroring `TestPromptCommand_Metadata`).

Out of scope:

- The `prompt` command and the segment helper itself. Those are `01-prompt-arg-feature.md`. This project depends on that helper existing.
- Alias work. That is `03-user-alias-feature.md`.
- Task name resolution: the three-tier search, registry auto-install, ambiguity prompt, and task-role selection are unchanged. Only the first positional is consumed as the name, exactly as today.
- Context selection, model resolution, agent/role resolution, and dry-run output, except where instructions content flows into them unchanged.
- Trimming spaces or tabs. The seam logic touches newline characters only, identical to `01`.

## Current State

`start task` is implemented in `internal/cli/task.go`. The command declares `Args: cobra.RangeArgs(0, 2)` and `Use: "task [name] [instructions]"`. `runTask`:

- With zero args, lists configured tasks and returns.
- Otherwise `taskName = args[0]`. If `len(args) > 1`, `instructions = args[1]`; else it reads piped stdin and uses it as instructions when piped. There is no interactive instructions entry. The positional wins over stdin; empty pipes are accepted.
- Calls `executeTask(..., taskName, instructions, tags)`.

`executeTask`:

- If `IsFilePath(taskName)`, reads the file (`reading task file %q: %w`) and runs it through `env.Composer.ProcessContent(content, instructions)`.
- Otherwise resolves the name (config, then registry with possible auto-install, then ambiguity prompt) and runs `env.Composer.ResolveTask(cfg, resolvedName, instructions)`.

Both paths reach `internal/orchestration/template.go` `TemplateProcessor.Process(fields, instructions)`. That function sets `data["instructions"] = instructions` and renders the task body as a Go `text/template`. If the body contains `{{.instructions}}` (or the spaced form `{{ .instructions }}`), the instructions land there. If it does not, `text/template` never references the value and the instructions are silently dropped. `Option("missingkey=zero")` does not affect this, because the key is present and simply unused.

The processor already detects other placeholders by substring before rendering: `needsFileContents` and `needsCommandOutput` check for `{{.file_contents}}`/`{{.command_output}}` (and their spaced forms) at `template.go:106-109`.

`Process` is also called for contexts and roles, which pass an empty instructions string. Any append behaviour must be a no-op when instructions are empty so those paths are unaffected.

Tests: the task command is exercised in `internal/cli/start_test.go` (there is no `task_test.go`); `internal/orchestration/template_test.go` and `composer_test.go` cover the processor. There is no existing task metadata test — `TestPromptCommand_Metadata` in `internal/cli/prompt_test.go` is the pattern to mirror. Existing tests may assert the current silent-drop behaviour for a placeholder-less body; those must be reviewed and updated to the append behaviour.

## The Seam Rule

Instruction segments are joined exactly as in `01-prompt-arg-feature.md`: exactly one blank line between adjacent non-empty segments, empty-after-newline-strip segments dropped, newline characters the only characters added or removed, spaces and tabs never touched, and a single segment returned verbatim. The append-to-body step uses the same seam, treating the rendered body as the first segment and the joined instructions as the second.

## Requirements

1. Instructions arity. `task` accepts any number of positionals (`cobra.ArbitraryArgs`). With zero args it lists tasks as today. With one arg it is the task name and instructions come from piped stdin as today. With two or more, the first is the name and the rest are instruction segments.

2. Instruction composition. When two or more positionals are present, instructions are `orchestration.ComposeSegments(args[1:], "instructions file")`: each trailing argument resolved independently as a file path or literal and joined under the seam rule. A read failure aborts with `reading instructions file %q: %w`, naming the offending argument; nothing is launched.

3. Stdin precedence. Piped stdin supplies instructions only when exactly one positional (the name) is present, exactly as today. Any instruction positional bypasses stdin.

4. Append when no placeholder. In `TemplateProcessor.Process`, detect a bare `{{.instructions}}` reference in the task body with a regexp that tolerates internal whitespace and trim markers (for example `{{-?\s*\.instructions\s*-?}}`), so every whitespace and trim-marker variant of a bare reference is recognised. When the placeholder is present, render as today (the instructions substitute in place) and do not append. When it is absent and instructions are non-empty, append the instructions after the rendered body using the seam rule (one blank line between body and instructions; an empty body yields just the instructions). Substring matching on two fixed forms is insufficient here: unlike `file_contents`, `data["instructions"]` is always populated (`template.go:115`), so `text/template` substitutes any valid `.instructions` reference inline regardless of whitespace or trim markers. A detection miss on a bare reference would therefore both substitute inline and append, duplicating the instructions. The regexp gates the append decision only; `text/template` still performs the substitution of a present placeholder.

   Detection scope is the bare reference only. Two reference shapes are intentionally outside it and are an accepted limitation, not a bug:

   - A non-emitting construct that references `.instructions` without an inline `{{.instructions}}` (for example `{{if .instructions}}…{{end}}`) is treated as having no placeholder and receives the append.
   - An emitting reference that wraps `.instructions` in a pipeline or function call (for example `{{.instructions | html}}` or `{{printf "%s" .instructions}}`) is likewise not detected, so it both substitutes inline and receives the append, duplicating the instructions.

   These forms do not occur for this placeholder in practice — task templates use bare UTD placeholders — and detecting them reliably would require the template AST inspection the Constraints section rules out. The regexp deliberately covers bare references only; the metadata and variant tests assert that scope, not exhaustive coverage of every `text/template` reference form.

5. Empty-instructions no-op. When instructions are empty, `Process` behaves exactly as today. The append branch is gated on non-empty instructions, so context and role processing are unaffected.

6. Name handling unchanged. The first positional is consumed as the task name/address/file and resolved exactly as today, including file-path tasks, `tasks:`-prefixed addresses, the three-tier search, registry auto-install, the ambiguity prompt, and task-role selection.

7. Help text. `Use` becomes `task [name] [instructions ...]`. `Long` documents that instructions may be multiple arguments, each independently a file path or inline text, joined with one blank line, and that instructions append after the body when the task has no `{{.instructions}}` placeholder. A metadata test added to `internal/cli/start_test.go` (mirroring `TestPromptCommand_Metadata`) asserts the new `Use` string.

## Constraints

- Reuse `orchestration.ComposeSegments`/`joinSegments` from `01`; do not duplicate the seam logic.
- Reuse `IsFilePath`/`ReadFilePath` unchanged.
- No new dependencies.
- The `{{.instructions}}` placeholder check uses a regexp matching the whitespace and trim-marker variants `text/template` accepts (`{{-?\s*\.instructions\s*-?}}` or equivalent), gating the append decision only; do not introduce template AST inspection. This intentionally differs from the two-form substring checks used for `file_contents` and `command_output` (see Requirement 4).
- Leave name resolution, context selection, model/agent/role resolution, and dry-run output untouched except for the instructions value flowing through them.
- Follow existing test conventions (`setupStartTestConfig`/task test helpers, `--dry-run`, real files via `t.TempDir()`).

## Implementation Plan

1. Arity. Change `Args` from `cobra.RangeArgs(0, 2)` to `cobra.ArbitraryArgs` in `addTaskCommand`. Keep the `len(args) == 0` list path in `runTask`.

2. Instructions composition. In `runTask`, when `len(args) > 1`, set `instructions, err = orchestration.ComposeSegments(args[1:], "instructions file")` and return on error. When `len(args) == 1`, keep the existing piped-stdin path. Pass `instructions` to `executeTask` unchanged.

3. Append behaviour. In `TemplateProcessor.Process`, add a package-level `instructionsRef` regexp compiled from the pattern `{{-?\s*\.instructions\s*-?}}` (use `regexp.MustCompile` with a raw-string literal), and set `hasInstructions := instructionsRef.MatchString(templateStr)`. After rendering into `result.Content`, when `instructions != "" && !hasInstructions`, set `result.Content = joinSegments([]string{result.Content, instructions})`. The regexp gates the append only; substitution of a present placeholder is still performed by `text/template`.

4. Help text. Update `Use` to `task [name] [instructions ...]` and expand `Long` per Requirement 7.

5. Tests. Add a task metadata test in `internal/cli/start_test.go` asserting the new `Use` (mirroring `TestPromptCommand_Metadata`); add command-level coverage for multiple instruction segments, mixed file-and-literal instructions, an unreadable instructions file, and a file-path task name followed by instruction segments. Add processor-level coverage for append-when-absent (with and without a trailing newline on the body), substitute-when-present including a trim-marker/whitespace variant such as `{{- .instructions}}` (substituted inline, no append), and the empty-instructions no-op. Review and update any existing test that asserts the old silent-drop behaviour.

## Acceptance Criteria

- `start task review/pre-commit ./checklist.md "follow the checklist exactly"` resolves `review/pre-commit` with instructions equal to the contents of `checklist.md`, one blank line, then `follow the checklist exactly`.
- `start task ./foo ./bar ./baz` reads `./foo` as the task body and composes instructions from `./bar` and `./baz` joined with one blank line.
- `start task ./foo "more instructions" ./bar` composes instructions of `more instructions`, one blank line, then the contents of `./bar`.
- When the task body contains `{{.instructions}}`, the joined instructions appear at that placeholder and are not also appended.
- When the task body has no `{{.instructions}}` placeholder, the joined instructions are appended after the body with exactly one blank line between them; previously they were dropped.
- A task body with no placeholder and empty instructions renders exactly as today (no trailing blank line added).
- `start task <name>` with one positional still takes instructions from piped stdin; any instruction positional overrides piped stdin.
- An unreadable instructions file anywhere in the trailing arguments fails with `reading instructions file "<arg>"` and launches nothing.
- `start task` with no arguments still lists tasks.
- Context, role, and dry-run behaviour are unchanged for a given instructions value.

## Tests

Add to `internal/orchestration` (processor):

- Append when the body lacks `{{.instructions}}`: body with and without a trailing newline each produce exactly one blank line before the appended instructions.
- Substitute when the body has `{{.instructions}}`: instructions land at the placeholder and nothing is appended.
- Substitute when the body uses a whitespace or trim-marker placeholder variant (for example `{{- .instructions}}` or `{{.instructions }}`): instructions land at the placeholder inline and are not also appended.
- Empty instructions: a placeholder-less body renders byte-identically to today, with no trailing blank line.

Add to `internal/cli/start_test.go` (where the task command tests live):

- Metadata: a test mirroring `TestPromptCommand_Metadata` asserts `Use == "task [name] [instructions ...]"`.
- Multiple instruction segments joined with one blank line, files under `t.TempDir()` referenced by `./` paths.
- Mixed literal and file-path instruction arguments in order.
- A file-path task name followed by instruction segments.
- An unreadable instructions file returns the `reading instructions file` error and does not launch.
- One positional still takes instructions from piped stdin; an instruction positional overrides stdin.

## References

- `01-prompt-arg-feature.md` — defines and tests `ComposeSegments`/`joinSegments`; land it first.
- `03-user-alias-feature.md` — independent of this project; both depend only on `01`.
- `internal/cli/task.go` — `addTaskCommand`, `runTask`, `executeTask`.
- `internal/cli/start_test.go` — home of the existing task command tests; `internal/cli/prompt_test.go` holds `TestPromptCommand_Metadata`, the metadata-test pattern to mirror.
- `internal/orchestration/template.go` — `TemplateProcessor.Process` and the existing placeholder-detection pattern.
- `internal/orchestration/composer.go` — `ResolveTask`, `ProcessContent`.
