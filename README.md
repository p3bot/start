# start

[![License: MPL 2.0](https://img.shields.io/badge/License-MPL_2.0-brightgreen.svg)](https://opensource.org/licenses/MPL-2.0)
[![Go Report Card](https://goreportcard.com/badge/github.com/start-cli/start)](https://goreportcard.com/report/github.com/start-cli/start)
[![Go Reference](https://pkg.go.dev/badge/github.com/start-cli/start.svg)](https://pkg.go.dev/github.com/start-cli/start)
[![GitHub Release](https://img.shields.io/github/v/release/start-cli/start)](https://github.com/start-cli/start/releases)

Context-aware AI agent launcher powered by CUE.

A future project will flatten the command tree.

## Why start?

**Stop re-explaining yourself to every AI session.**

Every time you open an AI coding session you provide the same background: what the project does, what role the agent should play, what you're working on today. start eliminates this by composing intelligent prompts from your project's context files and launching your configured AI agent — every time, consistently, with zero ceremony.

- **Role-based sessions** - Define agent expertise once, reuse across projects (`golang/assistant`, `gitlab/teacher`, `cwd/role-md`)
- **Reusable tasks** - Package common workflows as shareable prompts (`github/issue/triage`, `review/git-diff`, `jira/item/read`)
- **Automatic context injection** - Project files, environment info, and documentation included without manual setup
- **Multi-agent support** - Works with Claude, Gemini, aichat, aider, opencode, or any AI CLI tool
- **CUE-powered configuration** - Type-safe, validated, order-preserving config with built-in schema enforcement
- **Registry packages** - Install curated roles, contexts, and tasks from the CUE Central Registry

**Perfect for:**

- Developers who run AI coding sessions daily and want consistent context
- Teams sharing prompt engineering patterns across projects
- Anyone tired of repeating themselves at the start of every session

## Quick Start

```bash
# Install
brew tap start-cli/tap
brew install start-cli/tap/start

# Auto-setup detects your installed AI agent and writes initial config
# Launch an AI session with full project context
start

# Use a specific role
start --role golang/agent

# Run a reusable task
start task review/security

# Add extra context to a task
start task git-diff "Only focus on the documentation changes."

# Send a one-off prompt (minimal context, focused output)
start prompt "Explain this error message: 404 Not Found"
```

## Installation

### Homebrew (Linux/macOS)

```bash
brew tap start-cli/tap
brew install start-cli/tap/start
```

### Go Install

```bash
go install github.com/start-cli/start/cmd/start@latest
```

### Build from Source

```bash
git clone https://github.com/start-cli/start.git
cd start
go build ./cmd/start
./start --version
```

## How It Works

start is built around four concepts: **agents**, **roles**, **contexts**, and **tasks**. These are all defined in CUE and distributed as packages through the CUE Central Registry.

### Agents

An agent is your AI CLI tool — Claude Code, Gemini, or anything else. You configure which agent to use, and start handles the command construction and process handoff.

```bash
# Use your default configured agent
start

# Switch to a different agent for this session
start --agent gemini
```

_Note: start is not an agent harness, it is a launcher._

To make this clear, here is the command configuration for the Claude Code Interactive agent:

```
command: "{{.bin}} --model {{.model}} --permission-mode default --append-system-prompt-file {{.role_file}} {{.prompt}}"
```

### Roles

A role defines how the AI agent should behave — its expertise, tone, and focus area. Roles become the system prompt for your session.

```bash
# Start with a Golang expert role
start --role golang/assistant

# Use a role from a local file (must start with ./ or /)
start --role ./prompts/senior-reviewer.md
```

Roles are installed from the registry:

```bash
start modules install golang/teacher
start modules install git/agent
```

Roles come in three modes:

- agent mode: fully hands off operation
- assistant mode: interactive sessions
- teacher mode: to learn as you build

### Contexts

Contexts are document fragments injected into the prompt such as project overviews, environment details, coding standards, or anything else the agent needs to know. Contexts are tagged and selectively included.

```bash
# Include specific contexts by tag
start --context security,performance

# Include a context from a local file (must start with ./ or /)
start --context ./AGENTS.md
```

Your project's context files (like `AGENTS.md`, `README.md`, or `PROJECT.md`) are mapped to context definitions in config, so start knows exactly what to include and when.

```bash
# Add the ./AGENTS.md context
start modules install contexts:cwd/agents-md

# Use the ./AGENTS.md context (it is a required context)
start
```

### Tasks

A task is a reusable, parameterisable prompt for a specific workflow. Run a task instead of typing the same instructions repeatedly.

```bash
# Run a configured task
start task review/git-diff

# Pass instructions to a parameterised task
start task github/issue/triage "Implement the feature in issue #87"

# Run a task from a local file (must start with ./ or /)
start task ./tasks/my-review.md
```

Tasks only include required contexts by default, keeping prompts focused. Tasks are also available from the registry:

```bash
start modules install review/git-diff
start modules install jira/item/research
```

### Configuration

Configuration is stored in CUE format in `~/.config/start/` (global) and `./.start/` (project-local). Each directory can contain one or more `.cue` files. The `--local` flag targets project config instead of global.

```bash
# View effective configuration
start config

# List all configured items
start config list

# Add a new item interactively
start config add

# Edit an item by name
start config edit claude

# Remove an item
start config remove claude --yes

# Show raw config fields for an item
start config info claude

# Open a config file directly in $EDITOR
start config open

# Set a setting
start config settings default_agent claude

# Use project-local config
start --local
```

### Inspection

Use `start describe` to inspect resolved configuration — what agents, roles, contexts, and tasks are actually configured and what their content looks like after merging global and local config:

```bash
# List all configured items with descriptions
start describe

# Search across all categories and dump full detail
start describe golang/assistant
```

The `--global` and `--local` flags restrict output to a single config scope; omitting both shows the effective merged configuration.

### Dry Run

Run the full composition pipeline without launching the agent:

```bash
start --dry-run
start task review/duplication --dry-run
start prompt "My question" --dry-run
```

Dry run writes the composed inputs to `/tmp/start-<timestamp>/` for post-run inspection:

```
/tmp/start-<timestamp>/
├── role.md       # System prompt (role content)
├── prompt.md     # Full composed prompt
└── command.txt   # Exact command that would execute
```

## Usage

### Core Commands

```bash
# Launch interactive session with full context
start [flags]

# Send a focused one-off prompt
start prompt [text] [flags]

# Run a reusable predefined task
start task <name> [instructions] [flags]
```

### Inspection

```bash
# List all configured items with descriptions
start describe

# Inspect a specific resource by name (searches all categories)
start describe <name>
```

### Modules Management

```bash
# Browse available registry packages
start modules browse

# Show the full registry catalog
start modules index

# Show details for a specific module
start describe golang/assistant

# Install a package
start modules install golang/teacher
start modules install review/git-diff

# List installed modules
start modules list

# Update installed packages
start modules update

# Validate index and module version consistency (maintainer tool)
start modules validate --yes
```

### Configuration

```bash
# Display current configuration
start config

# List all configured items
start config list

# List by category
start config list agent
start config list role
start config list context
start config list task

# Add a new item (prompts for category if omitted)
start config add
start config add agent

# Edit an item by name (search across all categories)
start config edit
start config edit claude
start config edit gemini/interactive

# Show raw config fields for an item
start config info
start config info claude

# Remove an item
start config remove claude
start config remove claude --yes

# Reorder contexts or roles
start config order
start config order context
start config order role

# Open a config file directly in $EDITOR
start config open

# Export config as text to stdout
start config export

# Manage settings
start config settings default_agent claude
```

### Search and Discovery

```bash
# Search global config, local config, and the module registry index
start search go
```

### Diagnostics

```bash
# Diagnose setup, validate configuration, suggest fixes
start doctor
```

### Shell Completions

```bash
# Install tab-completion for your shell
start completion bash
start completion zsh
start completion fish
```

## CLI Reference

### Global Flags

| Flag         | Short | Description                                             |
| ------------ | ----- | ------------------------------------------------------- |
| `--agent`    | `-a`  | Override agent for this session                         |
| `--role`     | `-r`  | Override role (config name or file path)                |
| `--model`    | `-m`  | Override model selection                                |
| `--context`  | `-c`  | Select contexts (tags or file paths, repeatable)        |
| `--dry-run`  |       | Preview execution without launching                     |
| `--local`    | `-l`  | Use project-local config (`./.start/`)                  |
| `--quiet`    | `-q`  | Suppress output                                         |
| `--verbose`  |       | Detailed output                                         |
| `--debug`    |       | Debug output (implies `--verbose`)                      |
| `--no-color` |       | Disable coloured output                                 |
| `--no-role`  |       | Skip role assignment (mutually exclusive with `--role`) |

### File Path Support

The `--role`, `--context`, and task name arguments accept file paths alongside config names. Detected by prefix:

```bash
start --role ./roles/custom.md
start --context /absolute/path/context.md
start --context ~/shared/project-overview.md
start task ./tasks/my-workflow.md "Additional instructions"
```

### Task Resolution Order

1. Exact full name match in installed configuration
2. Combined search across installed config and registry — merged results presented for selection
3. Auto-install from registry when a single unambiguous match is found

## Contributing

Contributions welcome! Please:

1. Check existing issues: https://github.com/start-cli/start/issues
2. Create an issue for bugs or feature requests
3. Submit pull requests against the `main` branch

## License

`start` is licensed under the [Mozilla Public License 2.0](LICENSE).

## Author

Grant Carthew <grant@carthew.net>
