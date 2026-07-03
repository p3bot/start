# Configuration Merge Semantics

When loading configuration from global (`~/.config/start/`, or `$XDG_CONFIG_HOME/start` when `XDG_CONFIG_HOME` is set) and local (`./.start/`), two-level merge applies with distinct behaviour for collections versus settings. Local always takes precedence.

## Rules

Collections (`agents`, `contexts`, `roles`, `tasks`):

- Items merge additively by name — both sources contribute their items
- Same-named item: local completely replaces global (no field-level merge)

Settings (`settings`):

- Fields merge additively, one level deep — a same-named field is replaced wholesale by local, including a nested struct field (no recursive deep merge)
- The same one-level rule applies to any non-collection top-level struct key; `settings` is simply the common case

Scalar keys:

- Local completely replaces global (type changes allowed)

## Examples

### Collections: different names — both survive

```cue
// Global (~/.config/start/)
agents: {
    claude: { command: "claude", bin: "claude" }
}

// Local (./.start/)
agents: {
    gemini: { command: "gemini", bin: "gemini" }
}

// Result
agents: {
    claude: { command: "claude", bin: "claude" }
    gemini: { command: "gemini", bin: "gemini" }
}
```

### Collections: same name — local replaces entirely

```cue
// Global
roles: {
    reviewer: {
        description: "Global reviewer"
        prompt: "Global prompt"
        timeout: 60
    }
}

// Local
roles: {
    reviewer: {
        description: "Local reviewer"
        prompt: "Local prompt"
    }
}

// Result: local replaces entirely (timeout gone)
roles: {
    reviewer: {
        description: "Local reviewer"
        prompt: "Local prompt"
    }
}
```

### Settings: field-level merge

```cue
// Global
settings: {
    timeout: 120
    shell: "/bin/bash"
    default_agent: "claude"
}

// Local
settings: {
    default_agent: "gemini"
}

// Result
settings: {
    timeout: 120            // from global
    shell: "/bin/bash"      // from global
    default_agent: "gemini" // local overrides
}
```

## Config File Naming

Each file uses a key matching its filename. This is the write convention — every config writer (`install`, `update`, `add`, `edit`, reorder) targets the file named for the key — not a load-time rule: the loader reads and merges every `.cue` file in a config directory regardless of filename.

| File | Top-level key |
|------|---------------|
| `agents.cue` | `agents:` |
| `roles.cue` | `roles:` |
| `contexts.cue` | `contexts:` |
| `tasks.cue` | `tasks:` |
| `settings.cue` | `settings:` |
