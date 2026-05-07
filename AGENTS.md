# start - AI Agent CLI Orchestrator

`start` is a command-line orchestrator for AI agents built on CUE. It manages prompt composition, context injection, and workflow automation by wrapping AI CLI tools (Claude, Gemini, GPT, etc.) with configurable roles, reusable tasks, and project-aware context documents.

## Project Status

Active development. The CLI is fully implemented with commands for agent launching, config management, asset installation, and diagnostics. Built on CUE for type-safe, order-preserving configuration.

### Active Project

Projects are stored in `.ai/projects/`. Continue by reading the active project.

Active Project: None

Project Workflow:
- Active projects are in `.ai/projects/`
- When a project is complete, move it to `.ai/projects/completed/`
- Update this file to point to the next active project
- Update the Development Status section above

## Build & Test

```bash
go build ./...                  # Build all packages
go build -o start ./cmd/start   # Build the CLI binary
scripts/invoke-tests            # Run the full test suite
go test ./internal/...          # Run all internal package tests
go test ./internal/cli/...      # Run CLI tests only
```

Testing guidelines are in `.ai/design/testing-strategy.md`. Key principles:
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
start show agents               # Show installed agents
start show roles                # Show installed roles
start show tasks                # Show installed tasks
start show contexts             # Show installed contexts
start read <name>               # Output asset content to stdout (pipe-clean)
start assets install <pkg>      # Install an asset package
start config list               # List configuration entries
start search <term>             # Search installed assets
start doctor                    # Diagnose installation and configuration
start prompt                    # Compose and preview a prompt
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
| assets | `internal/assets/` | Asset search and installation |
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
| `internal/cli/resolve.go` | Three-tier asset resolution (exact config → registry → substring) |
| `internal/cli/root.go` | Root command factory with all subcommands registered |
| `internal/cli/start.go` | Main `start` command: config loading and execution env setup |
| `internal/cli/task.go` | Task execution command |
| `internal/orchestration/composer.go` | Prompt composition with context injection |
| `internal/orchestration/executor.go` | Agent command execution |
| `internal/cue/keys.go` | Centralized CUE config key constants |

### Resolution Logic

Asset resolution follows a three-tier strategy:

1. Exact match against installed config names
2. Exact match against CUE Central Registry index
3. Substring search across installed assets

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

## Documentation

- `.ai/` - AI agent working files (projects, design, tasks)
- `.ai/design/` - Technical standards and reference documents
- `.ai/design/testing-strategy.md` - Testing approach and patterns
- `.ai/design/cli-command-structure.md` - CLI command structure spec
- `.ai/design/cli-flags.md` - Flag definitions and behaviour
- `.ai/projects/` - Project documents (active and completed)
- `.ai/tasks/` - Task prompts

## What Changed From Prototype

| Aspect | Prototype (TOML) | This Version (CUE) |
| ------ | ---------------- | ------------------ |
| Config format | TOML (unordered tables) | CUE (ordered, typed) |
| Asset distribution | Custom GitHub API system | CUE Central Registry |
| Validation | Custom Go code | CUE schemas |
| Package management | Custom catalog/cache | CUE modules |
| Schema definition | Documentation only | Enforced by CUE |
| Order preservation | Failed assumption | Native support |

## References

- CUE language: [cuelang.org](https://cuelang.org)
- CUE Central Registry: [registry.cuelang.org](https://registry.cuelang.org)

## Library Repository

The `./library/` directory contains the cloned [start-cli/library](https://github.com/start-cli/library) repository for local development and testing. This directory is git-ignored.

Use for: Developing and testing new assets, schemas, and registry content before publishing.

```
library/
├── agents/          # Agent definitions
├── contexts/        # Context definitions
├── docs/            # Library documentation
├── index/           # Registry index module
├── roles/           # Role definitions
├── schemas/         # CUE schema definitions for all asset types
└── tasks/           # Task definitions
```
