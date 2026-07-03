# Terminal Colour Standard

All terminal colour usage in `start` follows this standard. Colours are defined centrally in `internal/tui/tui.go` and accessed via the exported `Color*` variables or the `CategoryColor()` helper. The rest of the codebase (e.g. `internal/cli/output.go`) consumes these as `tui.ColorWarning`, `tui.CategoryColor(...)`, etc., rather than defining its own.

## Message Types

| Element | Colour | Variable | Usage |
|---------|--------|----------|-------|
| Errors | Red | `ColorError` | Error prefix and messages |
| Warnings | Yellow | `ColorWarning` | Warning prefix and messages |
| Success markers | Green | `ColorSuccess` | Checkmarks (✓), confirmations, selection markers (↪), interactive input prompts (`> `), table status cells |
| Headers/titles | Green | `ColorHeader` | Section headers |
| Separators | Magenta | `ColorSeparator` | Horizontal rules (───) |
| Dim/secondary text | Faint | `ColorDim` | Descriptions, metadata, de-emphasised text |

## Module Categories

| Category | Colour | Variable |
|----------|--------|----------|
| agents | Blue | `ColorAgents` |
| roles | Green | `ColorRoles` |
| contexts | Cyan | `ColorContexts` |
| tasks | HiYellow | `ColorTasks` |
| settings | Magenta | `ColorSettings` |

Access via `CategoryColor(category)` which returns the appropriate `*color.Color`. It accepts both plural (`roles`) and singular (`role`) forms case-insensitively, and any unknown category falls back to `ColorDim`. Apply to category headers in search results, list output, and any command that groups output by module type.

Note: `ColorPrompts` exists (Faint) but is not a `CategoryColor()` case — there is no `prompts` module category.

Two category variables are also reused outside category headers: `ColorTasks` (HiYellow) colours the `○` non-loaded status marker in the role/context tables and alias names in `start alias` output, and the update output uses `ColorSuccess` for the new-version string.

## Markers

| Marker | Colour | Variable | Usage |
|--------|--------|----------|-------|
| Installed `★` | HiGreen | `ColorInstalled` | Left-side prefix on installed modules |
| Default `→` | HiGreen | `ColorInstalled` | Left-side prefix on default agent/role |
| Version arrows `->` | Blue | `ColorBlue` | Version transition in update output |
| Delimiters `()` `[]` | Cyan | `ColorCyan` | Bracketing metadata (version info, status, flags) |

## General Utility

| Variable | Colour | Usage |
|----------|--------|-------|
| `ColorCyan` | Cyan | General-purpose accent |
| `ColorBlue` | Blue | General-purpose accent |
| `ColorHiYellow` | HiYellow | Emphasis accent |
| `ColorPaths` | HiCyan | Filesystem paths |
| `ColorRegistry` | Yellow | Registry-sourced items |

## Formatting Rules

- Category names are coloured; trailing `/` is default colour
- Module names are default terminal colour
- Descriptions and metadata are dim (faint)
- Markers (`★`, `✓`) use their assigned colour
- When colours conflict in context, the more specific role wins
- Respect the `--color` flag and `NO_COLOR` environment variable. The decision logic lives in `resolveColorMode` (`internal/cli/root.go`) — `NO_COLOR` wins over `--color=always`, and `auto` detects a TTY — and is applied by setting the process-global `color.NoColor`, which `fatih/color` honours at render time

## Implementation

All colour variables and the `CategoryColor()` helper are defined in `internal/tui/tui.go`.

```go
func CategoryColor(category string) *color.Color
```

Surfaces consuming the standard include the cli command output (`internal/cli/output.go` and friends), the doctor reporter (`internal/doctor/reporter.go` — `ColorSeparator` rules, `ColorError` failure counts, `CategoryColor` section headers), and first-run auto-setup (`internal/orchestration/autosetup.go`).

Documented exception: Markdown module bodies shown by `start get`/`start describe` are rendered through charm glamour (`internal/tui/markdown.go`, `RenderMarkdown`) using its own dark/light theme, gated on `color.NoColor`. Glamour's styling is outside this palette by design; everything else follows the tables above.

A reference script at `scripts/show-colours` displays all standard ANSI colours in the terminal for visual comparison during development.

## Output Example

```
Found 5 matches:

roles/                    <- green "roles", default "/"
  ★ cwd/dotai/default       Project-specific default role
    cwd/role-md              Project-specific role from role.md

contexts/                 <- cyan "contexts", default "/"
  ★ cwd/agents-md           Repository-specific AI agent guidelines
    cwd/project              Project-specific documentation
```

- Category names: coloured per module type
- Module names: default terminal colour
- Descriptions: dim/faint
- Installed `★`: HiGreen, left-side prefix
- Default `→`: HiGreen, left-side prefix
