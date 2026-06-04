# Project: Multi-Segment Prompt Composition

## Goal

Refactor `start prompt` to accept any number of positional arguments, resolve each one independently (literal text or a file path beginning with `./`, `/`, `~`, or `~/`), and join the resolved segments so there is exactly one blank line between adjacent segments. This generalises today's single-argument handling so a prompt can be assembled from several files and inline strings in one invocation, with predictable spacing regardless of how each file is terminated:

```
start prompt ./intro.md ./body.md "wrap up please"
```

becomes a single prompt body of:

```
<contents of intro.md>

<contents of body.md>

wrap up please
```

The real use case is combining several files (optionally with a short inline instruction) into one body. Joining inline words into a sentence is not a goal; it is merely an allowed side effect of arbitrary arguments. The one-blank-line rule is chosen for that file-combining case, where document boundaries must be visible and consistent.

This project also introduces the shared segment-composition helper that the task project (`02-task-arg-feature.md`) reuses, and it is the prerequisite for prompt-valued aliases (`03-user-alias-feature.md`).

## Scope

In scope:

- Change the `prompt` argument arity from at most one to any number, including zero (`cobra.ArbitraryArgs`).
- Per-argument resolution: each argument is independently treated as inline text or a file path.
- A shared `orchestration` helper that resolves a list of arguments into segments and joins them under the one-blank-line rule.
- Preserve byte-identical behaviour for the existing single-argument case (one literal string, or one file path).
- Preserve the existing zero-argument behaviour: piped stdin, then interactive entry on a TTY.
- Update the command `Use`/`Long` help text and the metadata test that asserts `Use`.

Out of scope:

- Any task or alias work. Task is `02-task-arg-feature.md`; aliases are `03-user-alias-feature.md`. This project only changes the `prompt` command and adds the shared helper.
- Changing context selection (`IncludeRequired`/`IncludeDefaults`/tags), stdin precedence, or interactive prompting.
- Changing `orchestration.IsFilePath` or `ReadFilePath` semantics, including tilde expansion (`~`, `~/`) and the supported prefixes.
- Trimming spaces or tabs from any segment. The seam logic touches newline characters only.
- Glob expansion, directory reading, or `~user` syntax (`IsFilePath` already excludes these).

## Current State

`start prompt` is implemented in `internal/cli/prompt.go`. The command declares `Args: cobra.MaximumNArgs(1)` and `Use: "prompt [text]"`. `runPrompt` resolves the prompt body as follows:

- If one positional argument is given, it is treated as a file path when `orchestration.IsFilePath(arg)` is true (read via `orchestration.ReadFilePath`, errors wrapped as `reading prompt file %q: %w`), otherwise as literal text.
- If no argument is given and stdin is piped, the piped content is the prompt body (`readPipedStdin` in `internal/cli/root.go` returns `("", false, nil)` for a TTY).
- If no argument is given and stdin is a TTY, `promptText` collects the body interactively; an empty interactive entry returns without launching.

A positional argument therefore wins over piped stdin (`TestRunPrompt_ArgWinsOverPipedStdin`). After resolving the body, `runPrompt` builds `ContextSelection{IncludeRequired: true, IncludeDefaults: false, Tags: flags.Context}` and calls `executeStart(...)`.

File-path detection lives in `internal/orchestration/filepath.go`: `IsFilePath` matches `./`, `/`, bare `~`, and `~/` prefixes; `ReadFilePath` expands the tilde, resolves to an absolute path, and reads the file.

cobra/pflag use interspersed flag parsing (nothing in `internal/cli` disables it), so persistent flags may appear before, among, or after positionals. `start prompt a b --model x` and `start prompt a --model x b` both yield positionals `["a","b"]`. A literal segment that begins with `-` is parsed as a flag; the standard `--` terminator is the escape hatch. This is existing CLI-wide behaviour, not introduced here.

Tests are in `internal/cli/prompt_test.go`, using `setupPromptTestConfig(t)` (temp `.start/`, `os.Chdir`, `$HOME`/`$XDG_CONFIG_HOME` isolation, an `echo` agent) and `--dry-run` to assert the composed prompt without launching an agent. `TestPromptCommand_Metadata` asserts `Use == "prompt [text]"` exactly, so it must be updated. `TestRunPrompt_ArgWinsOverPipedStdin` and `TestRunPrompt_PipedStdin` pin the stdin precedence that must be preserved.

## The Seam Rule

The body is built from resolved segments so that between any two adjacent non-empty segments there is exactly one blank line, never more and never fewer, regardless of how many trailing or leading newlines each segment carries.

Governing principle: the seam logic only ever adds or removes newline characters. It never adds or removes spaces or tabs. Removing a blank line is spacing; stripping `foo  ` to `foo` would be content mutation, and the helper does not do it.

Algorithm:

1. Resolve each argument to a segment string: file contents when `IsFilePath(arg)` is true, otherwise the argument verbatim.
2. If exactly one argument was given, the body is that resolved segment verbatim — no drop, no edge stripping — so the single-argument case is byte-identical to today. This rule keys off the argument count, not the post-drop segment count, so a lone empty or all-newline argument also passes through unchanged (`""` stays `""`, a file of `"\n\n"` stays `"\n\n"`). The drop-empty and seam steps below apply only when composing two or more arguments.
3. With two or more arguments, drop any segment that is empty after stripping newline characters only (a segment of `""` or `"\n\n"` is dropped; a segment of `"   \n"` is kept, because spaces are content).
4. If zero segments remain, the body is the empty string. If exactly one segment remains, the body is that segment verbatim.
5. Otherwise, at each seam strip the trailing newline run from the left segment and the leading newline run from the right segment, then join with `\n\n`. The first segment keeps its leading bytes and the last keeps its trailing bytes; only the gaps between segments are normalised.

`joinSegments` stays the pure multi-segment core: it drops empty-after-newline-strip segments first, then joins what remains under the seam rule (zero remaining → the empty string; one remaining → that segment verbatim; two or more → seam-joined). Because the drop happens first, a single all-newline segment passed to `joinSegments` resolves to the empty string, not verbatim. The single-argument verbatim guarantee (step 2) therefore lives entirely in `ComposeSegments`, which resolves the arguments and, when exactly one was given, returns that resolved segment directly without routing it through `joinSegments`; with two or more it defers to `joinSegments`.

Reference shape (the exact home and naming are an implementation choice; place it beside `IsFilePath`/`ReadFilePath` in `internal/orchestration`):

```go
// ComposeSegments resolves each argument as a file path or literal and joins the
// resolved segments with exactly one blank line between adjacent non-empty
// segments. With exactly one argument it returns that resolved segment verbatim,
// bypassing the drop-empty step, so single-argument behaviour is byte-identical
// to the previous single-argument handling. fileNoun names the segment kind in
// read errors (for example "prompt file" or "instructions file").
func ComposeSegments(args []string, fileNoun string) (string, error)

// joinSegments applies the seam rule to already-resolved segments. Exported or
// not as needed; it is the pure, separately testable core.
func joinSegments(segs []string) string
```

`ComposeSegments(args, "prompt file")` preserves the existing `reading prompt file %q: %w` error wording for `prompt`. The `fileNoun` parameter lets the task project reuse the same helper with `"instructions file"`.

## Requirements

1. Argument arity. The `prompt` command accepts any number of positional arguments, including zero (`cobra.ArbitraryArgs`). The previous one-argument cap is removed.

2. Per-argument resolution. Each positional argument is resolved independently, in order: when `IsFilePath(arg)` is true the segment is the file's contents read via `ReadFilePath(arg)`; otherwise the segment is the argument string verbatim. A read failure wraps as `reading prompt file %q: %w`, naming the offending argument.

3. Seam join. Resolved segments are joined per the seam rule above: exactly one blank line between adjacent non-empty segments, empty-after-newline-strip segments dropped, newline characters the only characters added or removed, spaces and tabs never touched.

4. Single-argument parity. With exactly one positional argument, the prompt body is byte-identical to current behaviour: a lone literal string or a lone file path resolves exactly as it does today, with no edge stripping and no drop-empty handling. The guarantee keys off the argument count; drop-empty and seam normalisation apply only when two or more arguments are present (see The Seam Rule, step 2, for the degenerate cases this covers). This is a hard backward-compatibility guarantee.

5. Zero-argument behaviour unchanged. With no positional arguments, the command reads piped stdin and otherwise falls back to interactive entry, exactly as today, including the empty-interactive early return. The stdin/interactive fallback is gated solely on `len(args) == 0`.

6. Stdin precedence preserved. Whenever one or more positional arguments are present, piped stdin is ignored, extending the existing arg-wins-over-stdin rule to any argument count. This holds even when all present arguments resolve to empty and the body is empty.

7. Error handling. The first unreadable file-path argument aborts the command with the error from Requirement 2; no partial or fallback prompt is composed or launched.

8. Help text. `Use` becomes `prompt [text|file ...]`. `Long` documents that multiple arguments are accepted, that each is independently treated as inline text or a file path (`./`, `/`, `~`, `~/`), and that resolved segments are joined with one blank line between them. The metadata test asserts the new `Use` string.

## Constraints

- Reuse `IsFilePath` and `ReadFilePath` unchanged; do not duplicate or alter path detection or tilde expansion.
- No new dependencies.
- Leave the `ContextSelection` construction and the `executeStart` call unchanged; only the construction of the prompt body (`customText`) changes.
- The seam helper lives in `internal/orchestration` so the task project can reuse it without importing `cli`.
- Follow existing test conventions (`setupPromptTestConfig`, `--dry-run`, table-driven where natural, real files via `t.TempDir()`).

## Implementation Plan

1. Helper. Add `ComposeSegments` and `joinSegments` to `internal/orchestration` (a new `segments.go` beside `filepath.go`). Unit-test `joinSegments` directly for the seam rule, drop-empty, single-segment parity, and newlines-only behaviour.

2. Arity. Change `Args` from `cobra.MaximumNArgs(1)` to `cobra.ArbitraryArgs` in `addPromptCommand`.

3. Body composition. In `runPrompt`, when `len(args) > 0`, set `customText, err = orchestration.ComposeSegments(args, "prompt file")` and return the error if any. Keep the existing `else` branch (piped stdin, then interactive) gated on `len(args) == 0`.

4. Help text. Update `Use` to `prompt [text|file ...]` and expand `Long` per Requirement 8.

5. Tests. Update `TestPromptCommand_Metadata`; add coverage for multiple files, mixed text-and-file arguments, an absolute/tilde path argument, single-argument parity, the seam rule (exactly one blank line, including files with and without trailing newlines), a dropped empty segment, and an unreadable file-path argument. Confirm the stdin tests pass unchanged.

## Acceptance Criteria

- `start prompt ./intro.md ./body.md "wrap up please"` sends a body equal to the contents of `intro.md`, one blank line, the contents of `body.md`, one blank line, `wrap up please`, regardless of whether the files end in a newline.
- `start prompt a b c` sends the body `a` blank-line `b` blank-line `c`.
- Two files where the first ends in `\n` and the second does not produce exactly one blank line between them; the same holds when neither or both end in `\n`.
- `start prompt ./only.md` sends the file contents byte-identically to today's single-file behaviour.
- `start prompt "text"` sends `text` byte-identically to today's single-literal behaviour.
- A trailing-space content line (segment ending `"foo  \n"`) keeps its trailing spaces; only the newlines at the seam are normalised.
- An empty literal or all-newline file between two segments is dropped, leaving one blank line between the surrounding segments.
- `start prompt` with no arguments still reads piped stdin, and falls back to interactive entry on a TTY with the same empty-entry early return.
- An unreadable file-path argument anywhere in the list fails with `reading prompt file "<arg>"` and launches nothing.
- One or more positional arguments still override piped stdin.

## Tests

Add to `internal/orchestration` for the helper:

- `joinSegments`: two segments with varied trailing/leading newlines all collapse to one blank line; trailing spaces preserved; empty and all-newline segments dropped during a multi-segment join; a single non-empty segment returned verbatim, while a single empty or all-newline segment yields the empty string; zero segments give the empty string.
- `ComposeSegments`: file and literal resolution in order; single-argument verbatim passthrough, including a lone all-newline file that passes through unchanged rather than dropping (byte-identical to today); `reading prompt file %q` error wording on an unreadable path.

Add to `internal/cli/prompt_test.go`, following `setupPromptTestConfig` and `--dry-run`:

- Metadata: `Use == "prompt [text|file ...]"`.
- Multiple files joined with one blank line, files created under `t.TempDir()` and referenced by relative `./` paths.
- Mixed literal and file-path arguments in order.
- An absolute or `~/` path argument resolves to file contents.
- Single-argument parity: one literal and one file path each match current behaviour.
- An unreadable file-path argument returns the `reading prompt file` error and does not launch.
- Retain `TestRunPrompt_PipedStdin` and `TestRunPrompt_ArgWinsOverPipedStdin` unchanged.

## References

- `02-task-arg-feature.md` — reuses the `ComposeSegments`/`joinSegments` helper for task instructions.
- `03-user-alias-feature.md` — prompt-valued aliases (Approach A) depend on `prompt` accepting a trailing positional and composing it into the fixed prompt body.
- `internal/cli/prompt.go` — the command and `runPrompt`.
- `internal/orchestration/filepath.go` — `IsFilePath`, `ReadFilePath`, tilde expansion; new `segments.go` sits beside it.
- `internal/cli/prompt_test.go` — existing test conventions and stdin-precedence tests.
