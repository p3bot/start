# Vision for `start`

## The Problem

Developers working with AI agents need to repeatedly:

- Set up context (environment, project state, documentation)
- Define roles for different types of work
- Remember different AI tool commands and flags
- Maintain consistency across projects

This creates friction and reduces the value of AI-assisted development.

## The Solution

`start` is a **context-aware AI agent launcher** that:

1. **Detects your project context** automatically (documentation files, project state)
2. **Builds intelligent initial prompts** based on what files exist
3. **Launches the right AI agent** with proper configuration
4. **Works with your existing AI tools** (claude, gemini, opencode, aichat) - it's a launcher, not a replacement

## The Pattern

`start` reads **context files** from your project to build intelligent prompts. These files are fully configurable - you define what files to read and how they're used.

**Example configuration** (your current setup):

```
your-project/
├── ROLE.md          # AI should act as X (e.g., "Senior Go Developer")
├── AGENTS.md        # Repository/codebase overview
├── PROJECT.md       # Current project goals and tasks
└── reference/       # Project-specific docs
```

Run `start` and it:

- Detects which context files exist
- Builds a prompt instructing the agent to read them
- Sets the role/system prompt
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
- **Extendable** - Add new agents via config, not code

## Non-Goals

- **Not** replacing AI agents or their CLIs
- **Not** making API calls to AI services (delegates to existing tools)
- **Not** managing conversations or history
- **Not** orchestrating multi-step AI workflows

## Success Criteria

Someone should be able to:

1. Install the binary easily
2. Configure their AI tool
3. Add context files to their project
4. Launch an AI session with full context
