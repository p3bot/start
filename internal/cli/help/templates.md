# start Template Reference

Go template syntax: `{{.placeholder}}`. Two contexts with different placeholders.

Agent command templates (agents.cue `command` field). Do NOT quote `{{.prompt}}` — shell escaping is automatic:
```
claude --print {{.prompt}}
gemini --model {{.model}} --print {{.prompt}}
{{.bin}} --print {{.prompt}}
```

Placeholders: `{{.bin}}` `{{.model}}` `{{.role}}` `{{.role_file}}` `{{.prompt}}` `{{.datetime}}`

Note: `{{.role_file}}` is a path to a temp file containing the role content. Use it with whatever system-prompt flag your agent binary accepts.

UTD templates (roles, contexts, tasks — `prompt` field or file content):

Placeholders: `{{.instructions}}` `{{.file}}` `{{.file_contents}}` `{{.command}}` `{{.command_output}}` `{{.datetime}}`

Environment placeholders (available in UTD templates only — not in agent command templates):

| Placeholder | Value |
| ----------- | ----- |
| `{{.cwd}}` | Current working directory |
| `{{.home}}` | User home directory |
| `{{.user}}` | Current username |
| `{{.hostname}}` | Machine hostname |
| `{{.os}}` | OS identifier (e.g. `linux`, `darwin`) |
| `{{.os_name}}` | Human-readable OS/distro name |
| `{{.shell}}` | Current shell (e.g. `bash`, `zsh`) — empty if `$SHELL` is unset |
| `{{.git_branch}}` | Current git branch |
| `{{.git_root}}` | Git repository root path |
| `{{.git_user}}` | Git config user.name |
| `{{.git_email}}` | Git config user.email |

Common mistakes:
- `claude "{{.prompt}}"` — wrong, causes double-quoting; use `claude {{.prompt}}`
- `{prompt}` — wrong, use `{{.prompt}}`
