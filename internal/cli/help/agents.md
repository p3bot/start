# start Agent Reference

CLI orchestrator for AI agents. Manages prompt, role, and context injection.

- Global config: ~/.config/start/ (or $XDG_CONFIG_HOME/start/)
- Local config: ./.start/ (--local flag targets this)
- See `start help config` for config structure
- See `start help templates` for template placeholder syntax

```
start
start --role go-expert
start --role go-expert --model sonnet
start --agent gemini --model flash
start --context project,readme
start --no-role
start --dry-run
start prompt "Fix the bug in main.go"
start prompt ./notes.md
start prompt "Explain this" --role teacher
start prompt "Quick question" -c default
start task pre-commit-review
start task review "focus on error handling"
start task --tag golang
start task ./custom-task.md
start show
start show go-expert
start show --global
start show --local
start config
start config list
start config list agent
start config list role
start config list context
start config list task
start config search golang
start config export agent
start config settings
start config settings default_agent claude
start config settings timeout 120
start search golang
start search --tag review
start assets search golang
start assets add golang/code-review
start assets list
start assets update
start doctor
```

## Interactive Commands

These commands require a TTY and are not suitable for agent use.

```
start config add agent
start config add role
start config add context
start config add task
start config edit claude
start config remove claude
start config open agent
start config order role
```
