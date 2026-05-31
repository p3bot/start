# Project: Markdown styling for `get` and `describe` output

## Goal

Render Markdown module content with terminal styling (colours, headings, formatted code blocks) when `start get` and `start describe` write to an interactive terminal, while keeping piped and redirected output byte-for-byte raw. This makes interactive inspection of roles, contexts, and tasks readable without compromising the pipe-clean contract that scripting depends on.

## Scope

In scope:
- Markdown styling of resolved content emitted by `start get`.
- Markdown styling of the file-body section emitted by `start describe`.
- A reusable rendering helper in the `tui` package.
- Adopting the Charm v2 styling stack (`charm.land/glamour/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/colorprofile`).

Out of scope:
- Any new CLI flag. The existing `--color` decision is the single gate.
- Styling agent command templates, command-sourced output, or non-Markdown file bodies.
- Styling the CUE definition dump or any metadata/label lines in `describe`.
- Migrating interactive prompts to a TUI library. The dependency choice deliberately pre-stages that future work, but no prompt rewrite is part of this project.

## Current State

`start get` (internal/cli/get.go) resolves a module and writes its content to the injected stdout writer (`cmd.OutOrStdout()`), keeping stdout pipe-clean by routing all menus, progress, and verbose metadata to stderr. It has two output paths:
- `getAgent` (get.go:155): emits an agent command template with static placeholders filled. This is never Markdown.
- `getUTD` (get.go:187): resolves a UTD module. A trim block (get.go:224-229) collapses the source to exactly one of file, prompt, or command before `Process` runs, so after it the winning source is unambiguous: `fields.File != ""` means file won, else `fields.Prompt != ""` means prompt won, else command won. Final content is written at get.go:251.

`start describe` (internal/cli/describe.go) prints a structured verbose dump via `printVerboseDump` (describe.go:434). The dump is composed of separated sections: a header line, `Config:`/`Origin:`/`Cache:` labels, a metadata block, a CUE definition dump (`formatCUEDefinition`, CUE source that must stay literal), a File section, and a Command section. The only full Markdown body in the entire dump is the file content printed at describe.go:480 (`fmt.Fprint(w, content)`). The `prompt` field is shown only as a truncated one-line label (metadata_writers.go:80/106/134), never as a rendered body.

Colour is already settled once per invocation. `PersistentPreRunE` (root.go:90-119) resolves `--color` plus `NO_COLOR`/`FORCE_COLOR`/`CLICOLOR_FORCE`/`TERM`/TTY into one boolean via `resolveColorMode` (root.go:229) and stores it as the global `color.NoColor` (root.go:100). `--color=auto` decorates only on a TTY; `--color=always` forces decoration even when piped; `--color=never` and `NO_COLOR` disable it. Existing tests drive commands through buffers (non-TTY), so `color.NoColor` is true under test and output stays raw.

The `tui` package (internal/tui/tui.go) owns colour and terminal concerns and already imports `github.com/fatih/color` and `golang.org/x/term`. It is the natural home for a Markdown rendering helper.

`go.mod` currently depends on `golang.org/x/term`; it does not yet depend on any Charm package.

## References

- GitHub issue start-cli/start#1 ("feat(get): add markdown styling output for tty"): origin of the request; lists `get` as primary and `describe` as an optional extension.
- ajira `RenderMarkdown` (/home/grant/Projects/ajira/internal/cli/root.go:178): the prior-art pattern — TTY/no-colour gate, terminal-width word wrap with 80 fallback, raw fallback on render error. Written against glamour v1; the v1 API it uses (`WithAutoStyle`) does not exist in v2, so treat it as intent, not a template.
- glamour v2.0.0 source, cloned for local reference at .ai/context/glamour. Key files: glamour.go (renderer options), UPGRADE_GUIDE_V2.md (v1 to v2 breaking changes), examples/artichokes/main.go (the `colorprofile.NewWriter` downsampling pattern for an arbitrary `io.Writer`).
- glamour UPGRADE_GUIDE_V2.md highlights relevant here: import path is `charm.land/glamour/v2`; `WithAutoStyle()` removed, default style is `"dark"`; `WithColorProfile()` removed — v2 emits TrueColor only and downsampling is delegated to `colorprofile`/`lipgloss`; auto light/dark detection now uses `lipgloss.HasDarkBackground()`.

## Requirements

1. A `tui` helper renders a Markdown string with terminal styling and writes it to a caller-supplied `io.Writer`. It gates on the settled `color.NoColor` state: when decoration is off it writes the input through unchanged (raw passthrough); when on it renders. Any glamour construction or render error falls back to writing the raw input. An empty input produces no styled artefacts.
2. The helper word-wraps to the terminal width of the destination writer, with an 80-column fallback when the width cannot be determined. No upper cap on width.
3. The helper selects the glamour style by auto-detecting the terminal background (light vs dark) via `lipgloss/v2`, defaulting to dark when detection is unavailable.
4. Rendered output is downsampled to the destination terminal's colour capability by writing through a `colorprofile` writer that wraps the destination. When the settled colour decision says decorate but the destination is not a TTY (the `--color=always | pipe` case), the writer's profile is forced to a coloured profile so colour is preserved rather than stripped.
5. A pure, unit-testable predicate decides whether a given resolved body is Markdown that should be styled, from its source kind and (for file sources) its extension. The rule: style iff the source is a rendered prompt, or a file whose extension is `.md` or `.markdown`. Agent command templates, command-sourced output, and non-Markdown file bodies are never styled.
6. `start get` applies the predicate to its resolved content: prompt-sourced UTD content is styled; file-sourced UTD content is styled only for `.md`/`.markdown` files; command-sourced UTD content and agent command templates are emitted raw.
7. `start describe` applies the predicate to its file-body section only (describe.go:480). Every other section — header, `Config:`/`Origin:`/`Cache:`/`File:`/`Path:`/`Command:` labels, metadata block, and the CUE definition dump — is emitted unchanged.
8. When decoration is off (piped, redirected, `--color=never`, `NO_COLOR`, or non-TTY auto), `start get` output is byte-for-byte identical to its pre-change output, preserving the pipe-clean contract.

## Constraints

- Use the Charm v2 styling stack, pinned: `charm.land/glamour/v2` v2.0.0, `charm.land/lipgloss/v2` (latest v2 patch), `github.com/charmbracelet/colorprofile` (the version glamour v2 requires or newer). Do not introduce glamour v1 or `github.com/charmbracelet/lipgloss` v1 — mixing v1 and v2 of the styling libraries in one module graph is prohibited.
- The settled `color.NoColor` state is the only decoration gate. Do not add a CLI flag and do not add a second, independent TTY probe that could diverge from it.
- `start get` must stay pipe-clean: when not decorating, content output is unchanged from current behaviour.
- Markdown rendering must never alter agent command templates, command-sourced output, non-`.md` file bodies, the CUE definition dump, or any metadata/label line.
- The rendering helper lives in the `tui` package and writes to the injected writer; do not write to `os.Stdout` directly (it would break the writer-injection test pattern).
- Validate with `scripts/invoke-tests` (lint plus tests) and `go test ./internal/...`.

## Implementation Plan

1. Add and pin the dependencies (`charm.land/glamour/v2`, `charm.land/lipgloss/v2`, `github.com/charmbracelet/colorprofile`); commit the resulting `go.mod`/`go.sum`.
2. Add the `tui` rendering helper. Responsibilities: gate on `color.NoColor` (raw passthrough when off); determine destination width via `golang.org/x/term` with an 80 fallback; choose style via `lipgloss/v2` background detection (default dark); build a glamour v2 renderer with the chosen style and word wrap; write the rendered bytes through a `colorprofile` writer wrapping the destination, forcing a coloured profile when decorating to a non-TTY; fall back to writing the raw input on any error. A writer-based signature (helper takes the destination `io.Writer`) fits the v2 downsampling-at-write model better than returning a string.
   - Integration snippet for intent:
     ```
     // decorate == !color.NoColor
     cw := colorprofile.NewWriter(dst, os.Environ())
     if decorate && cw.Profile == colorprofile.NoTTY { cw.Profile = colorprofile.ANSI256 }
     r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(width))
     // ... render, write to cw, fall back to raw dst on err
     ```
3. Add the pure render-decision predicate (source kind plus file extension to bool) and table-driven tests for it. Keep it independent of the glamour call so the rule is tested without ANSI assertions.
4. Wire `start get`: in `getUTD`, after `Process`, route `result.Content` through the helper when the predicate passes (prompt won, or file won with a Markdown extension), otherwise emit raw. Leave `getAgent` untouched. Reconcile glamour's own trailing newline and left margin with the existing `ensureTrailingNewline` handling so output gains no stray blank lines.
5. Wire `start describe`: route only the file-body `content` (describe.go:480) through the predicate (Markdown extension) and helper. Leave all other sections, including the CUE definition dump, literal.
6. Tests: predicate table tests; helper raw-passthrough when `color.NoColor` is true; `get` non-TTY output unchanged (existing snapshot/golden behaviour holds); agent templates, command output, and non-`.md` bodies never styled.
7. Update user-facing docs if they describe `get`/`describe` output behaviour (AGENTS.md command notes, README) to mention TTY-gated Markdown styling.

## Implementation Guidance

- Two gates compose and are distinct: the predicate answers "is this body Markdown content?" (caller, source-aware), and the helper answers "is decoration on?" (`color.NoColor`). Keep them separate.
- Verify two API details against the pinned versions rather than assuming: the `lipgloss/v2` background-detection function and its signature, and the exact `colorprofile` call to inspect/force the writer profile. The v2 upgrade guide and the cloned `examples/artichokes` show the shapes but may lag the pinned patch.
- The `--color=always | pipe` path is the one easy regression: `colorprofile` will strip colour from a non-TTY writer unless its profile is forced. Cover it explicitly so "always means always" holds.

## Acceptance Criteria

1. `start get <md-file-or-prompt module>` on a TTY emits ANSI-styled Markdown.
2. `start get <same module>` piped or redirected emits output byte-identical to the pre-change raw content.
3. `start get <agent>` on a TTY emits the raw command template with no styling.
4. `start get <command-sourced module>` on a TTY emits raw command output with no Markdown styling.
5. `start get <non-.md file-sourced module>` on a TTY emits the raw file body with no styling.
6. `start get <md module> --color=always` piped emits styled, coloured output (colour not stripped).
7. `start get <md module> --color=never` on a TTY emits raw output.
8. `start describe <md-file module>` on a TTY styles the file-body section while the header, label lines, metadata block, and CUE definition dump remain unchanged.
9. `start describe <non-.md file module>` on a TTY emits the file body raw.
10. The module graph contains `charm.land/lipgloss/v2` and no `github.com/charmbracelet/lipgloss` v1.
