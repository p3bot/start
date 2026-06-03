# Project: Multi-Argument Prompt Composition

## Goal

Refactor `start prompt` to accept any number of positional arguments, resolve each one independently (literal text or a file path beginning with `./`, `/`, `~`, or `~/`), and join the resolved segments with a single space into one prompt body. This generalises today's single-argument handling so a prompt can be assembled from several inline strings and file contents in one invocation:

```
start prompt "hi" ./prompt.md "bye"
  → prompt body = "hi" + " " + <contents of ./prompt.md> + " " + "bye"
```

It is also the prerequisite for prompt-valued aliases (see `02-user-alias-feature.md`). Under Approach A of that design, `start foo "extra"` expands to `start prompt "<fixed prompt>" "extra"`, so the `prompt` command must combine multiple positionals into a single prompt rather than reject them. Landing this first keeps the alias resolver a pure, type-agnostic argv rewrite with no prompt-specific merging logic.

## Scope

In scope:

- Change the `prompt` argument arity from at most one to any number, including zero.
- Per-argument file detection and reading: each argument is independently treated as inline text or a file path.
- Join the resolved segments with a single space, in argument order, into one prompt body.
- Preserve byte-identical behaviour for the existing single-argument case (one literal string, or one file path).
- Preserve the existing zero-argument behaviour: piped stdin, then interactive entry on a TTY.
- Update the command `Use`/`Long` help text and the metadata test that asserts `Use`.

Out of scope:

- Any alias work. That is `02-user-alias-feature.md`; this project only changes the `prompt` command.
- Changing context selection (`IncludeRequired`/`IncludeDefaults`/tags), stdin precedence, or interactive prompting.
- Changing `orchestration.IsFilePath` or `ReadFilePath` semantics, including tilde expansion (`~`, `~/`) and the supported prefixes.
- A separator other than a single space; no delimiter flag and no per-argument formatting.
- Trimming or transforming file contents. Segments are inserted verbatim.
- Glob expansion, directory reading, or `~user` syntax (`IsFilePath` already excludes these).

## Current State

`start prompt` is implemented in `internal/cli/prompt.go`. The command declares `Args: cobra.MaximumNArgs(1)` and `Use: "prompt [text]"`. `runPrompt` resolves the prompt body as follows:

- If one positional argument is given, it is treated as a file path when `orchestration.IsFilePath(arg)` is true (read via `orchestration.ReadFilePath`, errors wrapped as `reading prompt file %q: %w`), otherwise as literal text.
- If no argument is given and stdin is piped, the piped content is the prompt body (`readPipedStdin` in `internal/cli/root.go` returns `("", false, nil)` for a TTY).
- If no argument is given and stdin is a TTY, `promptText` collects the body interactively; an empty interactive entry returns without launching.

A positional argument therefore wins over piped stdin (`TestRunPrompt_ArgWinsOverPipedStdin`). After resolving the body, `runPrompt` builds `ContextSelection{IncludeRequired: true, IncludeDefaults: false, Tags: flags.Context}` and calls `executeStart(...)`.

File-path detection lives in `internal/orchestration/filepath.go`: `IsFilePath` matches `./`, `/`, bare `~`, and `~/` prefixes; `ReadFilePath` expands the tilde, resolves to an absolute path, and reads the file.

Tests are in `internal/cli/prompt_test.go`, using `setupPromptTestConfig(t)` (temp `.start/`, `os.Chdir`, `$HOME`/`$XDG_CONFIG_HOME` isolation, an `echo` agent) and `--dry-run` to assert the composed prompt without launching an agent. `TestPromptCommand_Metadata` asserts `Use == "prompt [text]"` exactly, so it must be updated. `TestRunPrompt_ArgWinsOverPipedStdin` and `TestRunPrompt_PipedStdin` pin the stdin precedence that must be preserved.

## Requirements

1. Argument arity. The `prompt` command accepts any number of positional arguments, including zero (`cobra.ArbitraryArgs`). The previous one-argument cap is removed.

2. Per-argument resolution. Each positional argument is resolved independently, in order: when `orchestration.IsFilePath(arg)` is true the segment is the file's contents read via `orchestration.ReadFilePath(arg)`; otherwise the segment is the argument string verbatim. A read failure wraps with the existing format, `reading prompt file %q: %w`, naming the offending argument.

3. Join. The resolved segments are joined with a single ASCII space, in argument order, to form the prompt body. Segments are inserted verbatim — no trimming — so a file segment retains its bytes, including any leading or trailing whitespace. The single-space separator is the only character inserted between segments.

4. Single-argument parity. With exactly one positional argument, the prompt body is byte-identical to the current behaviour: a one-segment join introduces no separators, so a lone literal string or a lone file path resolves exactly as it does today. This is a hard backward-compatibility guarantee, not an approximation.

5. Zero-argument behaviour unchanged. With no positional arguments, the command reads piped stdin and otherwise falls back to interactive entry, exactly as today, including the empty-interactive early return. The stdin/interactive fallback is gated solely on there being zero positionals.

6. Stdin precedence preserved. Whenever one or more positional arguments are present, piped stdin is ignored, extending the existing arg-wins-over-stdin rule to any argument count.

7. Error handling. The first unreadable file-path argument aborts the command with the error from Requirement 2; no partial or fallback prompt is composed or launched.

8. Help text. `Use` becomes `prompt [text|file ...]`. `Long` documents that multiple arguments are accepted, that each argument is independently treated as inline text or a file path (`./`, `/`, `~`, `~/`), and that the resolved segments are joined with a single space. The metadata test is updated to assert the new `Use` string.

## Constraints

- Reuse `orchestration.IsFilePath` and `orchestration.ReadFilePath` unchanged; do not duplicate or alter path detection or tilde expansion.
- No new dependencies.
- Leave the `ContextSelection` construction and the `executeStart` call unchanged; only the construction of the prompt body (`customText`) changes.
- Follow existing test conventions (`setupPromptTestConfig`, `--dry-run`, table-driven where natural, real files via `t.TempDir()`).

## Implementation Plan

1. Arity. Change `Args` from `cobra.MaximumNArgs(1)` to `cobra.ArbitraryArgs` in `addPromptCommand`.

2. Body composition. In `runPrompt`, replace the single-argument block with a loop over `args` that builds a `[]string` of resolved segments — reading file-path arguments and taking others verbatim — then `strings.Join(segments, " ")` into `customText`. Keep the existing `else` branch (piped stdin, then interactive) gated on `len(args) == 0`.

3. Help text. Update `Use` to `prompt [text|file ...]` and expand `Long` per Requirement 8.

4. Tests. Update `TestPromptCommand_Metadata` for the new `Use`; add coverage for multiple literal arguments, mixed text-and-file arguments, an absolute/tilde path argument, single-argument parity, and an unreadable file-path argument. Confirm the existing stdin tests still pass unchanged.

## Implementation Guidance

- Build the segments in a slice and join once with `strings.Join(segments, " ")`; this keeps the single-argument case a true no-op (a one-element join adds nothing) and makes the separator explicit in one place.
- Resolve each argument with the same `IsFilePath`/`ReadFilePath` pair used today, so multi-argument behaviour is consistent with the single-argument path and inherits the same tilde and absolute-path handling.
- Gate the stdin/interactive fallback on `len(args) == 0` exactly as today, so any positional argument bypasses stdin and the precedence rule holds for every argument count.
- File contents are inserted verbatim. A file with a trailing newline placed between other segments will carry that newline into the joined body; this is intentional fidelity, consistent with the current single-file behaviour, not a defect to paper over with trimming.

## Acceptance Criteria

- `start prompt "hi" ./prompt.md "bye"` sends a prompt body equal to `"hi"` + `" "` + the verbatim contents of `./prompt.md` + `" "` + `"bye"`.
- `start prompt "a" "b" "c"` sends the body `a b c`.
- `start prompt ./only.md` sends the file contents byte-identically to today's single-file behaviour.
- `start prompt "text"` sends `text` byte-identically to today's single-literal behaviour.
- `start prompt` with no arguments still reads piped stdin, and falls back to interactive entry on a TTY with the same empty-entry early return.
- An unreadable file-path argument anywhere in the list fails with `reading prompt file "<arg>"` and launches nothing.
- One or more positional arguments still override piped stdin.

## Tests

Add to `internal/cli/prompt_test.go`, following `setupPromptTestConfig` and `--dry-run` conventions:

- Metadata: `Use == "prompt [text|file ...]"`.
- Multiple literal arguments joined with single spaces.
- Mixed literal and file-path arguments, with the file created under `t.TempDir()` and referenced by a relative `./` path; assert both the literal segments and the file contents appear in the composed prompt in order.
- An absolute or `~/` path argument resolves to file contents.
- Single-argument parity: one literal and one file path each match current behaviour.
- An unreadable file-path argument returns the `reading prompt file` error and does not launch.
- Retain `TestRunPrompt_PipedStdin` and `TestRunPrompt_ArgWinsOverPipedStdin` unchanged to pin stdin precedence.

## References

- `02-user-alias-feature.md` — the consumer of this behaviour; prompt-valued aliases (Approach A) depend on `prompt` joining multiple positionals.
- `internal/cli/prompt.go` — the command and `runPrompt`.
- `internal/orchestration/filepath.go` — `IsFilePath`, `ReadFilePath`, tilde expansion.
- `internal/cli/prompt_test.go` — existing test conventions and stdin-precedence tests.
