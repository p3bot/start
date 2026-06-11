# start - AI Agent CLI Orchestrator

`start` is a command-line orchestrator for AI agents built on CUE. It manages prompt composition, context injection, and workflow automation by wrapping AI CLI tools (Claude, Gemini, GPT, etc.) with configurable roles, reusable tasks, and project-aware context documents.

## Project Status

Active development. The CLI is fully implemented with commands for agent launching, config management, module installation, and diagnostics. Built on CUE for type-safe, order-preserving configuration.

### Active Project

When an active project is set, continue by reading it. `none` means no project is queued.

Active Project: none

When a project is complete, update this file to point to the next active project (or `none` if nothing is queued)

## Build & Test

```bash
go build ./...                  # Build all packages
go build -o start ./cmd/start   # Build the CLI binary
scripts/invoke-tests            # Run the full pipeline (lint + tests)
scripts/invoke-linter           # Run golangci-lint only
scripts/invoke-linter -- --fix  # Apply auto-fixes
go test ./internal/...          # Run all internal package tests
go test ./internal/cli/...      # Run CLI tests only
```

Testing key principles:
- Test real behaviour over mocks (use actual CUE validation, real files via `t.TempDir()`)
- Design functions to accept interfaces/parameters rather than reaching for globals
- Use table-driven tests for multiple cases
- Existing tests use `setupStartTestConfig(t)` with `.start/` dir in temp, `os.Chdir`, and `$HOME` isolation
- `registry.NewClient()` connects to real CUE registry; set `skipRegistry: true` in tests touching the resolver
- `registry.Client` is an interface; registry-touching commands obtain their client through a per-instance provider (`getProvider(cmd)()`) stored on the command context, not by calling `registry.NewClient()` directly. New registry-backed code should consume the interface through the provider so it stays stubbable. Two paths are not yet on the seam and still call `registry.NewClient()` directly: the resolver (`resolve.go`, powering `describe`/`get`/auto-install — it holds no `cmd`), and first-run auto-setup (`internal/orchestration/autosetup.go` — it cannot import `cli`). Neither emits `--json`; migrate them only if they need offline stubbing
- Offline `--json` coverage for `library`/`search`/`update`/`doctor`: `setupStartTestConfigWithRegistry(t, idx)` isolates config plus a stub client, and `captureJSON(t, stub, args...)` runs a command with the stub injected and returns decoded JSON. `doctor validate` is excluded from the offline path; its `--json` shape is asserted by a `//go:build registry` integration test run with `go test -tags=registry`

## Commands

```bash
start                           # Start interactive session with default role
start --role go-expert          # Start with specific role
start task pre-commit-review    # Run a specific task
start describe                  # List all installed modules grouped by category
start describe <name>           # Inspect a module; auto-installs if needed (TTY: Markdown file body styled)
start get <name>                # Output module content to stdout (pipe-clean; TTY: Markdown styled)
start install <pkg>             # Install a module from the library
start list                      # List installed modules
start library                   # Show the available module library
start update                    # Update installed modules
start config list               # List configuration entries
start alias                     # List personal command aliases
start alias set pc task review  # Save an alias (value is the command without 'start')
start <alias>                   # Run a saved alias (e.g. 'start pc')
start search <term>             # Search installed config and the module registry
start doctor                    # Diagnose installation and configuration
start doctor validate           # Maintainer check: index/registry/tag consistency
start help schemas              # --json output shapes and exit-code reference
start prompt                    # Compose and preview a prompt
echo "summarise" | start        # Pipe text as a one-shot prompt (required contexts only)
echo "..." | start prompt       # Pipe text to fill prompt's [text] arg
echo "..." | start task review  # Pipe text to fill task's [instructions] arg
```

### Aliases

Personal, global-only shortcuts that expand a leading token into a saved `start`
command. An alias value is the saved argv minus the leading `start`, captured
verbatim and spliced back in before cobra dispatch (single-pass, never re-parsed
by a shell). The store is a managed file at `aliases/aliases.cue` under the
global config dir; its subdirectory keeps it out of every directory package
build, so a malformed store never breaks the main config load.

```bash
start alias                       # List all aliases (same as 'start alias list')
start alias set <name> <token>... # Create or update one alias (value captured verbatim)
start alias get <name>            # Show one alias as its expanded command
start alias delete <name>...      # Delete one or more aliases (alias: rm)
start alias open                  # Edit aliases/aliases.cue in $EDITOR
start alias export                # Print the store to stdout
start alias import [file]         # Merge aliases from stdin or a file (--replace to overwrite)
```

### Persistent Flags

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--agent` | `-a` | Override the configured agent |
| `--role` | `-r` | Override role (config name or file path); `none` skips role assignment |
| `--model` | `-m` | Override the model |
| `--context` | `-c` | Select contexts (tags or file paths, repeatable); `none` drops auto-loaded required/default contexts (`none,foo` keeps only foo) |
| `--dry-run` | | Preview execution without running |
| `--quiet` | `-q` | Suppress non-essential output |
| `--verbose` | | Show detailed output |
| `--debug` | | Debug output (implies --verbose) |
| `--color` | | Colour output: `auto` (default), `always`, `never` |
| `--local` | `-l` | Target local config |

## Architecture

### Package Structure

| Package | Path | Purpose |
| ------- | ---- | ------- |
| cli | `internal/cli/` | Command implementations (cobra) |
| orchestration | `internal/orchestration/` | Prompt composition and agent execution |
| modules | `internal/modules/` | Module search and installation |
| detection | `internal/detection/` | Detect installed AI CLI tools on PATH |
| cue | `internal/cue/` | CUE configuration loading and validation |
| registry | `internal/registry/` | CUE Central Registry client |
| config | `internal/config/` | Configuration path and settings management |
| doctor | `internal/doctor/` | Diagnostic checks and reporting |
| fault | `internal/fault/` | Cross-cutting error sentinels for exit-code mapping |
| cache | `internal/cache/` | Registry index caching |
| temp | `internal/temp/` | Temporary file and directory management |
| shell | `internal/shell/` | Shell detection and command execution |
| tui | `internal/tui/` | Terminal UI colour and formatting |

### Key Files

| File | Purpose |
| ---- | ------- |
| `internal/cli/engine.go` | Unified name-only resolution engine (exact tier → floor → substring/prefix fallback) shared by all surfaces |
| `internal/cli/resolve.go` | Resolver state, surface entry points (`resolveAgent`/`resolveRole`/`resolveContexts`), registry fetch and auto-install |
| `internal/cli/root.go` | Root command factory with all subcommands registered |
| `internal/cli/start.go` | Main `start` command: config loading and execution env setup |
| `internal/cli/task.go` | Task execution command |
| `internal/orchestration/composer.go` | Prompt composition with context injection |
| `internal/orchestration/executor.go` | Agent command execution |
| `internal/cli/exitcodes.go` | Maps `fault` sentinels to semantic exit codes (see `start help schemas`) |
| `internal/cue/keys.go` | Centralized CUE config key constants |

### Resolution Logic

One name-only engine (`engine.go`) resolves every module-selecting surface —
`start task`, `--role`, `--context`, `--agent`, and the cross-category
`start get`/`start describe` — against installed config and the registry index as
two equal sources, de-duplicated by `category:name` (installed wins). The match
rule, specified in `docs/module-resolution.md`, is:

1. Interpret the identifier. A leading `./`, `/`, `~`, or `~/` is a filesystem
   path read directly (no search); `--agent` rejects a path. A `category:name`
   prefix scopes to that category and selects prefix fallback; a mismatched or
   unknown category is a usage error.
2. Exact-whole-name tier first, for every non-path input. A single case-
   insensitive whole-name match resolves directly — even when the name is a
   substring of longer names, even without a TTY, and including a registry-only
   match (installed then used). This tier is exempt from the floor. A lone
   installed exact resolves offline on the category-specific surfaces; the cross-
   category surfaces also consult the registry here to detect a same-name twin.
3. Fallback tier when no exact match exists, over the names only: a bare term is
   a case-insensitive literal substring, a category-qualified term a literal
   prefix. The query must be at least three characters (counting the name,
   excluding any `category:` prefix). Zero matches is not-found, one is used, and
   more than one menus on a TTY or errors with the list otherwise.

Matching is literal and case-insensitive over names only — no regex, no
description/tag matching, no multi-term splitting. The registry index is fetched
lazily and its absence is non-fatal: an uninstalled name is not-found when the
index is reachable, and a transient (retry) error when it is unreachable, since
absence cannot be confirmed. Model resolution (`--model`) is out of scope; it
keeps the search-style match against the agent's `models` map.

### Architecture Principles

- CUE-native: All configuration, schemas, and validation in CUE
- Registry-driven: Packages distributed via CUE Central Registry, not a custom GitHub system
- Order-aware: Configuration order preserved for context injection
- Type-safe: CUE schemas prevent configuration errors
- Simple: Let CUE handle complexity instead of building custom systems

## Core Concepts

- Roles: Define AI agent behaviour and expertise (e.g., `go-expert`, `code-reviewer`)
- Tasks: Reusable prompts for common workflows (e.g., `pre-commit-review`, `debug-help`)
- Contexts: Environment-specific information loaded at runtime and injected into prompts
- Agents: AI model configurations (Claude, GPT, Gemini, etc.) with command templates
- Packages: Roles, tasks, and configurations distributed via CUE Central Registry

## Why CUE?

CUE (Configure Unify Execute) provides:
- Order preservation: Configuration order matters for context injection and prompt composition
- Built-in validation: Schema definition and validation are native features
- Type safety: Strong typing prevents configuration errors
- Packages and modules: CUE Central Registry provides proper package distribution
- Templating: Native support for constraints, defaults, and composition
- Data and logic together: Configuration can include validation rules and transformations

## What Changed From Prototype

| Aspect | Prototype (TOML) | This Version (CUE) |
| ------ | ---------------- | ------------------ |
| Config format | TOML (unordered tables) | CUE (ordered, typed) |
| Module distribution | Custom GitHub API system | CUE Central Registry |
| Validation | Custom Go code | CUE schemas |
| Package management | Custom catalog/cache | CUE modules |
| Schema definition | Documentation only | Enforced by CUE |
| Order preservation | Failed assumption | Native support |

## References

- CUE language: [cuelang.org](https://cuelang.org)
- CUE Central Registry: [registry.cuelang.org](https://registry.cuelang.org)

## Library Repository

The `./library/` directory contains the cloned [start-cli/library](https://github.com/start-cli/library) repository for local development and testing. This directory is git-ignored.

Use for: Developing and testing new modules, schemas, and registry content before publishing.

```
library/
├── agents/          # Agent definitions
├── contexts/        # Context definitions
├── docs/            # Library documentation
├── index/           # Registry index module
├── roles/           # Role definitions
├── schemas/         # CUE schema definitions for all module types
└── tasks/           # Task definitions
```
