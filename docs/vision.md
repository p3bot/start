# Vision for `start`

## The Problem

Developers working with AI agents need to repeatedly:

- Set up context (environment, project state, documentation)
- Define roles for different types of work
- Remember different AI tool commands and flags
- Maintain consistency across projects

This creates friction and reduces the value of AI-assisted development.

## The Solution

`start` is an **AI agent CLI orchestrator** that:

1. **Injects your configured context** (project docs, environment, live command output) into every session
2. **Composes intelligent initial prompts** from reusable roles, tasks, and contexts
3. **Launches the right AI agent** with proper configuration
4. **Works with your existing AI tools** (Claude, Gemini, and any CLI described by an agent module) - it's an orchestrator, not a replacement

## The Pattern

`start` composes prompts from **installed modules** - roles, tasks, and contexts defined in CUE configuration (global `~/.config/start/`, local `./.start/`) and distributed via the CUE Central Registry. A context module points at a file, a shell command, or inline prompt text; required and default contexts load automatically.

```
your-project/.start/
├── roles.cue        # AI should act as X (e.g., "Senior Go Developer")
├── contexts.cue     # Project docs and live state to inject
├── tasks.cue        # Reusable prompts for common workflows
└── agents.cue       # How to invoke each AI CLI
```

Run `start` and it:

- Loads your merged global and local configuration
- Resolves the role and contexts, injecting file contents and command output
- Composes the initial prompt
- Launches your AI agent

## Target Users

1. **You** - Standardize your AI workflow across projects
2. **Your colleagues** - Share the pattern, easy installation
3. **Open source users** - Complement your other GitHub tools

## Key Value Props

- **Zero ceremony** - Just type `start` in any project
- **Consistent pattern** - Same structure across all projects
- **Tool agnostic** - Works with any AI CLI
- **Easy to adopt** - Single binary, minimal config
- **Extendable** - Add new agents, roles, tasks, and contexts as installable modules and config, not code

## Non-Goals

- **Not** replacing AI agents or their CLIs
- **Not** making API calls to AI services (delegates to existing tools)
- **Not** managing conversations or history
- **Not** chaining multi-step agent pipelines - a task is a single reusable prompt, not a workflow engine

## Success Criteria

Someone should be able to:

1. Install the binary easily
2. Let first-run auto-setup detect their AI tools
3. Install or define roles, tasks, and contexts for their project
4. Launch an AI session with full context
