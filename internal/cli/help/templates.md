# start Template Reference

Go template syntax: `{{.placeholder}}`. Two contexts with different placeholders.

Agent command templates (agents.cue `command` field). Do NOT quote the one-word placeholders below — non-empty values are shell-escaped automatically. Empty values stay empty, so `{{if}}` is false (they are not quoted as `''`):
```
{{.bin}}{{if .model}} --model {{.model}}{{end}} --print {{.prompt}}
gemini --model {{.model}} --print {{.prompt}}
{{.bin}} --print {{.prompt}}
```

Keep the leading space inside the `if` so tokens do not glue. Recipes that always set `default_model` may keep unconditional `--model {{.model}}`.

Placeholders shell-escaped as one word: `{{.bin}}` `{{.model}}` `{{.role}}` `{{.role_file}}` `{{.prompt}}` `{{.datetime}}`

Raw fragments, inserted with their own leading space and not shell-quoted: `{{.permission}}` `{{.effort}}` `{{.output}}` `{{.resume}}` `{{.print}}`

Permission, effort, output, and resume are built from the agent module's `flags` table when that flag is supplied, and they insert nothing when it is omitted. `--print` omitted still inserts the table's `print.off` words. A word that is exactly `{{.prompt}}` or `{{.resume}}` is replaced at launch and shell-quoted; every other word is a literal. Zero words insert nothing, not a space. Concatenate the slots with no extra space between them:

```
{{.bin}}{{if .model}} --model {{.model}}{{end}}{{.permission}}{{.effort}}{{.output}}{{.resume}}{{.print}}
```

`--print` omitted inserts the table's `print.off` words. Bare `--resume` inserts `resume.latest`. `--resume=<id>` inserts `resume.id` with `{{.resume}}` replaced by the id. Do not quote the fragment slots.

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
