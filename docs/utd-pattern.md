# Unified Template Design (UTD)

A consistent pattern for building prompt text across `start` configuration.

## Overview

The Unified Template Design (UTD) provides a flexible way to build prompt text from static files, dynamic command output, and template text. It's the foundation for composing rich, context-aware prompts for AI agents.

**Used by:**

- Tasks - Build task prompt text
- Roles - Build role/system prompt text
- Contexts - Build context document text

**Not used by:**

- Agents - Use different structure (command execution templates; see Agent Command Placeholders below)

## Core Concept

UTD uses three optional fields that work together:

**Fields:**

- `file` - Path to a file (local path or `@module/` cache path; see File Resolution below)
- `command` - Shell command to execute
- `prompt` - Template text

**At least one field must be present.** The Go runtime validates this constraint.

**Resolution priority:** `prompt` > `file` > `command`

- If `prompt` is present, it wins (other fields are inputs)
- If only `file` and `command`, file wins (command can be injected into file)
- If only one field, that field determines the output

## Placeholders

All placeholders use Go template syntax: `{{.name}}`

**UTD Placeholders (available everywhere):**

- `{{.file}}` - File path (absolute path for local files; temp path for `@module/` files — see File Resolution)
- `{{.file_contents}}` - File contents (actual text)
- `{{.command}}` - Command string (e.g., `git status --short`)
- `{{.command_output}}` - Command execution output (stdout + stderr)
- `{{.datetime}}` - Current timestamp (ISO 8601 format)
- `{{.cwd}}` - Current working directory
- `{{.home}}` - User home directory
- `{{.user}}` - Current username
- `{{.hostname}}` - Machine hostname
- `{{.os}}` - Operating system (`linux`, `darwin`, `windows`)
- `{{.shell}}` - Current shell basename (e.g., `bash`, `zsh`; empty if `$SHELL` is unset)
- `{{.git_branch}}` - Current git branch (empty if not in a git repo)
- `{{.git_root}}` - Git repository root directory (empty if not in a git repo)
- `{{.git_user}}` - Git user name from `git config user.name` (empty if not set)
- `{{.git_email}}` - Git user email from `git config user.email` (empty if not set)
- `{{.os_name}}` - OS/distro name (e.g., `EndeavourOS`, `macOS`, falls back to `{{.os}}`)

**Task-Specific Placeholder:**

- `{{.instructions}}` - Additional instructions from command line argument

When a task's template contains no bare `{{.instructions}}` reference and instructions were supplied, the instructions are appended to the rendered output instead. Detection covers bare references only (`{{.instructions}}`, `{{ .instructions }}`, and trim-marker variants); `.instructions` inside a pipeline or function call is not recognised, so the instructions are appended as well.

**Go Template Features:**

- Full Go template support: conditionals (`{{if}}`), loops (`{{range}}`), functions
- See [Go template documentation](https://pkg.go.dev/text/template) for complete syntax

## Resolution Flow

The UTD resolver builds final prompt text based on which fields are present and which placeholders are used.

### Lazy Evaluation

**Optimization:** Only perform I/O when necessary:

- File is read only if `{{.file_contents}}` appears in template
- Command is executed only if `{{.command_output}}` appears in template
- `{{.file}}` and `{{.command}}` are just strings (no I/O)

Detection matches the exact spellings `{{.file_contents}}` / `{{ .file_contents }}` (likewise for `{{.command_output}}`); other spacing, or use inside a pipeline or function call, is not detected and the read or execution is silently skipped.

### Resolution Rules

**1. Only `file`**

```cue
file: "./ROLE.md"
```

Result: File contents become the prompt text.

---

**2. Only `command`**

```cue
command: "git status --short"
```

Result: Command output becomes the prompt text.

---

**3. Only `prompt`**

```cue
prompt: "Review this code for security issues."
```

Result: Prompt text (with runtime placeholders like `{{.datetime}}` resolved).

---

**4. `file` + `command` (no `prompt`)**

```cue
file:    "./PROJECT.md"
command: "git log -5 --oneline"
```

If the file contains `{{.command_output}}`:

- Execute command
- Template file contents with command output → prompt text

Otherwise:

- The command is not executed; `{{.command}}` (the literal command string) is still available, but no output is produced
- Use file contents → prompt text

---

**5. `file` + `prompt`**

```cue
file:   "~/reference/ENVIRONMENT.md"
prompt: "Read {{.file}} for environment context."
```

If the prompt contains `{{.file_contents}}`:

- Read file
- Template prompt → prompt text

Otherwise:

- The file is not read; `{{.file}}` (the path string) is still available, but its contents are not
- Template prompt → prompt text

---

**6. `command` + `prompt`**

```cue
command: "git status --short"
prompt:  "Current status:\n{{.command_output}}"
```

If the prompt contains `{{.command_output}}`:

- Execute command
- Template prompt → prompt text

Otherwise:

- The command is not executed; `{{.command}}` (the command string) is still available, but no output is produced
- Template prompt → prompt text

---

**7. `file` + `command` + `prompt`**

```cue
file:    "./PROJECT.md"
command: "git status --short"
prompt: """
Project: {{.file_contents}}

Status: {{.command_output}}
"""
```

- Read file only if `{{.file_contents}}` is used (`{{.file}}` alone is just the path string)
- Execute command only if `{{.command_output}}` is used (`{{.command}}` alone is just the command string)
- Template prompt → prompt text

## File Resolution

Defines how the `file` field is resolved and where `{{.file}}` points.

### File Locality

Files are classified as either local (within the working directory) or external (outside it).

Local files (relative paths or absolute paths under the working directory):

- Existence validated with `os.Stat()`
- No temp file created — the file is used in place
- `{{.file}}` returns the absolute path (tilde-expanded, relative input resolved via `filepath.Abs` — `./task.md` surfaces as `/path/to/cwd/task.md`)

External files (CUE module cache, absolute paths outside cwd):

- Copied to `.start/temp/` with a name derived from the entity name
- `{{.file}}` returns the temp path (CUE cache is inaccessible to agents)

Temp file naming: `<type>-<name>.md`, where `<name>` is the sanitised CUE map key of the role/task/context, independent of the source file path.
Example: a role named `golang-assistant` with `file: "@module/role.md"` → `.start/temp/role-golang-assistant.md`

### @module/ Prefix

The `@module/` prefix in a `file` field indicates the path should resolve relative to the CUE module cache, not the working directory.

```cue
file: "@module/task.md"   // resolves to CUE cache location
file: "./task.md"          // resolves relative to working directory
```

Resolution algorithm:

1. If path starts with `@module/`, strip prefix
2. Resolve against the CUE cache extraction directory: `$CUE_CACHE_DIR/mod/extract/<module-dir>/<module-base>@<version>/` (the module path is split into directory and base). When the exact versioned directory is absent, the highest semver-sorted `@v*` directory for that module is used instead
3. Copy file to `.start/temp/<type>-<name>.md`

Cache directory:

| Platform | Default |
|----------|---------|
| macOS | `~/Library/Caches/cue` |
| Linux | `~/.cache/cue` |
| Windows | `%LocalAppData%/cue` |

Override with `CUE_CACHE_DIR` environment variable.

### Temp Directory

Location: `.start/temp/`

Created only when external files are used (e.g., registry assets with `@module/` paths). Projects using only local files will never see this directory.

Cleanup is manual. Add `.start/temp/` to `.gitignore` when using registry assets — `start` does not warn when it is missing.

### Processing Steps

1. Extract UTD fields (`file`, `command`, `prompt`)
2. Resolve `@module/` paths using origin field from module metadata
3. Classify file as local or external
4. For local files: validate existence
5. For external files: copy to `.start/temp/`
6. Scan template for `{{.file_contents}}` — read file only if present
7. Scan template for `{{.command_output}}` — execute command only if present
8. Process through `text/template` engine
9. Return rendered text

## Shell Configuration

### Per-Section Fields

Set the shell for specific UTD instances:

```cue
contexts: {
 "node-version": {
  command: "console.log(process.version)"
  shell:   "node -e"
  timeout: 5
  prompt:  "Node version: {{.command_output}}"
 }
}
```

**Priority:**

1. Section-specific `shell` field (if present)
2. Auto-detected shell (`bash` if available, otherwise `sh`)

The same applies to `timeout`: the section-specific field if present, otherwise the built-in default of 30 seconds. The `settings.shell` and `settings.timeout` keys in `settings.cue` are not consulted at execution time — they are surfaced by `config` and `doctor` output only. Only the per-section fields affect command execution.

### Shell Invocation

The `shell` value is split on whitespace: the first token is the executable and any remaining tokens are its arguments. If you provide only an executable name, `-c` is appended automatically. The command string is passed as the final argument.

- `shell: "bash"` runs `bash -c <command>`
- `shell: "zsh"` runs `zsh -c <command>`
- `shell: "python3 -c"` runs `python3 -c <command>`

There is no per-interpreter flag table. Any executable that does not take a script via `-c` must have its flag given explicitly. For example, Node evaluates inline scripts with `-e`, so use `shell: "node -e"` — a bare `shell: "node"` would run `node -c <command>`, which only syntax-checks the script and does not execute it.

### Command Timeout

Commands are subject to timeout limits (default 30 seconds):

```cue
contexts: {
 "quick-check": {
  command: "git status"
  timeout: 5   // 5 seconds
 }

 "slow-analysis": {
  command: "npm run analyze"
  timeout: 120  // 2 minutes
 }
}
```

**Behavior:**

- Command exceeds timeout → Killed, warning emitted
- `{{.command_output}}` renders empty — output from a timed-out (or otherwise failed) command is discarded, never injected

### Working Directory

Commands execute in the current working directory (the directory `start` is run from).

## Error Handling

The fail-versus-warn split is driven by whether the failing file or command is the **primary source** (no `prompt` — the file or command IS the output) or a **placeholder-injected input** (`prompt` present, feeding `{{.file_contents}}` / `{{.command_output}}`):

**Injected inputs (any module type):**

- A failed `{{.file_contents}}` read or `{{.command_output}}` execution → **Warn + continue**; the placeholder renders empty. This applies to tasks, roles, and contexts alike — an injected failure never fails the module.

**Primary source failure and template syntax errors** are an error for the module; the consequence depends on how the module was selected:

**Tasks:**

- Primary file missing or command fails → **Fail** (task cannot proceed)
- Template syntax error → **Fail** (invalid configuration)

**Roles:**

- Explicitly requested (`--role`) → **Fail** (an explicit request that cannot be satisfied is fatal)
- Default-selected → **Warn + skip** role; agent runs without role/system prompt

**Contexts:**

- Resolution failure → **Warn + skip** entire context; session continues without it

**General principle:** an injected input never fails the module; a failed primary source fails the module, and whether that fails the run depends on whether the module was explicitly requested.

## Agent Command Placeholders

Agents do not use UTD; their `command` templates use a separate placeholder set:

| Placeholder | Value |
|-------------|-------|
| `{{.bin}}` | Agent binary (from `bin` field) |
| `{{.model}}` | Resolved model identifier |
| `{{.prompt}}` | Assembled prompt text |
| `{{.role}}` | Resolved role content (inline text) |
| `{{.role_file}}` | Path to role file |
| `{{.datetime}}` | Current timestamp (ISO 8601) |

`{{.role_file}}` follows the same locality rules as `{{.file}}`:

| Role source | `{{.role_file}}` value |
|-------------|------------------------|
| `file:` with local path | Absolute file path |
| `file:` with `@module/` path | Temp file path |
| `prompt:` or `command:` (inline) | Temp file path (content written to temp) |

All placeholder values are automatically shell-escaped (single-quote wrapped) by the executor. Do NOT add quotes around placeholders in command templates:

```cue
// Correct
command: "{{.bin}} --model {{.model}} --append-system-prompt {{.role}} {{.prompt}}"

// Wrong — causes double-quoting
command: "{{.bin}} --prompt '{{.prompt}}'"
```

The executor rejects quoted placeholders with a clear error. It also rejects single-brace `{name}` syntax, which is never valid in a Go template command.

## Escaping Template Syntax

To output a literal `{{` in content:

```markdown
In Go templates, use {{"{{"}} .variable {{"}}"}} for substitution.
```

## Scope

Template processing applies to all asset types:

| Type | UTD placeholders | Agent placeholders |
|------|------------------|--------------------|
| Roles | Yes | No |
| Tasks | Yes | No |
| Contexts | Yes | No |
| Agent commands | No | Yes |

## Examples

### Simple File

```cue
roles: {
 "code-reviewer": {
  file: "./ROLE.md"
 }
}
```

Uses file contents directly as role text.

### Simple Command

```cue
contexts: {
 "git-status": {
  command: "git status --short"
 }
}
```

Uses command output directly as context text.

### Inline Prompt

```cue
contexts: {
 note: {
  prompt: "Important: This project uses Go 1.21"
 }
}
```

Uses prompt text directly.

### File with Template

```cue
contexts: {
 environment: {
  file:   "~/reference/ENVIRONMENT.md"
  prompt: "Read {{.file}} for environment context."
 }
}
```

Injects file path into prompt template.

### Command with Template

```cue
contexts: {
 "recent-changes": {
  command: "git log -5 --oneline"
  prompt: """
Recent commits:
{{.command_output}}

Focus on these changes during the session.
"""
 }
}
```

Injects command output into prompt template.

### File with Command Injection

**File: PROJECT.md**

```markdown
# My Project

## Recent Activity

{{.command_output}}

## Status

Work in progress.
```

**Config:**

```cue
contexts: {
 project: {
  file:    "./PROJECT.md"
  command: "git log -3 --oneline"
 }
}
```

Command output replaces `{{.command_output}}` in the file.

### Combined: File + Command + Prompt

```cue
contexts: {
 "complete-status": {
  file:    "./PROJECT.md"
  command: "git status --short"
  prompt: """
# Full Project Context

## Documentation
{{.file_contents}}

## Working Tree
{{.command_output}}
"""
 }
}
```

Both file contents and command output injected into prompt.

### Task with Instructions

```cue
tasks: {
 "code-review": {
  command: "git diff --staged"
  prompt: """
Review these changes:

{{.command_output}}

Instructions: {{.instructions}}
"""
 }
}
```

Used via: `start task code-review "focus on security"`

The `{{.instructions}}` placeholder receives `"focus on security"`.

### Multi-line Script with Node.js

```cue
contexts: {
 "package-info": {
  shell:   "node -e"
  command: """
const pkg = require('./package.json');
console.log(`${pkg.name}@${pkg.version}`);
console.log(`Dependencies: ${Object.keys(pkg.dependencies).length}`);
"""
  prompt: "Package details:\n{{.command_output}}"
 }
}
```

### Go Template Features

```cue
contexts: {
 "file-list": {
  command: "ls -1"
  prompt: """
{{if .command_output}}
Files found:
{{.command_output}}
{{else}}
No files in directory.
{{end}}
"""
 }
}
```

Uses Go template conditional.

## Security Considerations

**Command execution runs shell scripts with full system access:**

1. **Validate command sources** - Only execute commands from trusted configurations
2. **Review local configs** - Local `./.start/` configs can execute arbitrary commands
3. **Be cautious with shared configs** - Review before using configs from others
4. **Timeout protection** - Commands are killed after timeout
5. **No automatic sudo** - Commands run with current user permissions
6. **CUE validation** - Schemas validate structure, not security of commands

**Best practices:**

- Keep sensitive commands in local config (not committed to git)
- Review any config before running `start`
- Use minimal permissions for command execution
- Prefer static files over dynamic commands when possible

## Schema Usage

In CUE, UTD is defined as a reusable definition. These schema files live in the library repository and, for local development, under `test/testdata/schemas/`; there are no production-embedded schema files, and the "at least one field" constraint is enforced in Go rather than by CUE:

```cue
// utd.cue
package schemas

#UTD: {
 file?:    string
 command?: string
 prompt?:  string
 shell?:   string & !=""
 timeout?: int & >=1 & <=3600

 // Note: Go validates at least one of file/command/prompt required
}
```

Tasks, roles, and contexts embed `#UTD`:

```cue
// role.cue
#Role: {
 #UTD
 description?: string
}
```

This ensures consistent UTD behavior across all use cases.

## See Also

- [Go text/template package](https://pkg.go.dev/text/template)
