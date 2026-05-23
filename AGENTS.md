# start - AI Agent CLI Orchestrator

`start` is a command-line orchestrator for AI agents built on CUE. It manages prompt composition, context injection, and workflow automation by wrapping AI CLI tools (Claude, Gemini, GPT, etc.) with configurable roles, reusable tasks, and project-aware context documents.

## Project Status

Active development. The CLI is fully implemented with commands for agent launching, config management, module installation, and diagnostics. Built on CUE for type-safe, order-preserving configuration.

### Active Project

Continue by reading the active project.

Active Project: [07-extract-resolve-module-file-helper.md](./07-extract-resolve-module-file-helper.md)

When a project is complete, update this file to point to the next active project

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

## Commands

```bash
start                           # Start interactive session with default role
start --role go-expert          # Start with specific role
start task pre-commit-review    # Run a specific task
start describe                  # List all installed modules grouped by category
start describe <name>           # Inspect a module; auto-installs from registry if needed
start get <name>                # Output module content to stdout (pipe-clean)
start modules install <pkg>     # Install a module from the library
start config list               # List configuration entries
start search <term>             # Search installed config and the module registry
start doctor                    # Diagnose installation and configuration
start prompt                    # Compose and preview a prompt
echo "summarise" | start        # Pipe text as a one-shot prompt (required contexts only)
echo "..." | start prompt       # Pipe text to fill prompt's [text] arg
echo "..." | start task review  # Pipe text to fill task's [instructions] arg
```

### Persistent Flags

| Flag | Short | Description |
| ---- | ----- | ----------- |
| `--agent` | `-a` | Override the configured agent |
| `--role` | `-r` | Override role (config name or file path) |
| `--model` | `-m` | Override the model |
| `--context` | `-c` | Select contexts (tags or file paths, repeatable) |
| `--dry-run` | | Preview execution without running |
| `--quiet` | `-q` | Suppress non-essential output |
| `--verbose` | | Show detailed output |
| `--debug` | | Debug output (implies --verbose) |
| `--no-color` | | Disable colored output |
| `--local` | `-l` | Target local config |
| `--no-role` | | Skip role assignment (mutually exclusive with --role) |

## Architecture

### Package Structure

| Package | Path | Purpose |
| ------- | ---- | ------- |
| cli | `internal/cli/` | Command implementations (cobra) |
| orchestration | `internal/orchestration/` | Prompt composition and agent execution |
| modules | `internal/modules/` | Module search and installation |
| cue | `internal/cue/` | CUE configuration loading and validation |
| registry | `internal/registry/` | CUE Central Registry client |
| config | `internal/config/` | Configuration path and settings management |
| doctor | `internal/doctor/` | Diagnostic checks and reporting |
| cache | `internal/cache/` | Registry index caching |
| shell | `internal/shell/` | Shell detection and command execution |
| tui | `internal/tui/` | Terminal UI colour and formatting |

### Key Files

| File | Purpose |
| ---- | ------- |
| `internal/cli/resolve.go` | Three-tier module resolution (exact config → registry → substring) |
| `internal/cli/root.go` | Root command factory with all subcommands registered |
| `internal/cli/start.go` | Main `start` command: config loading and execution env setup |
| `internal/cli/task.go` | Task execution command |
| `internal/orchestration/composer.go` | Prompt composition with context injection |
| `internal/orchestration/executor.go` | Agent command execution |
| `internal/cue/keys.go` | Centralized CUE config key constants |

### Resolution Logic

Module resolution follows a three-tier strategy:

1. Exact match against installed config names
2. Exact match against CUE Central Registry index
3. Substring search across installed modules

File paths (starting with `./`, `/`, or `~`) bypass search entirely.

CUE config lookup pattern:
```go
cfg.LookupPath(cue.ParsePath(key)).LookupPath(cue.MakePath(cue.Str(name)))
```

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
