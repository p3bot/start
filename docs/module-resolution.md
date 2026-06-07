# Module Resolution

Reference for how `start` turns a user-supplied identifier into a module to act
on. This procedure is uniform across tasks, roles, contexts, and agents. The
guiding principle is simple: search the installed config and the registry index
for matches, apply one match rule, and act on the result.

## Where it applies

| Surface | Identifier source | Category |
| ------- | ----------------- | -------- |
| `start task <id>` | positional argument | tasks |
| `--role <id>` (`-r`) | flag value | roles |
| `--context <id>` (`-c`) | flag value, repeatable | contexts |
| `--agent <id>` (`-a`) | flag value | agents |
| `start <id>` | positional argument | all four |
| `start get <id>`, `start describe <id>` | positional argument | all four |

Model resolution (`--model`) is out of scope. A model is resolved against the
selected agent's `models` map, not against config and registry modules.

## The match rule

Resolution checks for an exact whole-name match first, then falls back to a
substring query.

### Exact match

An exact match is unambiguous by definition: the identifier equals the full name
of exactly one module. It resolves to that module directly — even when the name
is also a substring of longer names, and even without a TTY. An exact match that
exists only in the registry is installed first, then used.

On cross-category surfaces (`start <id>`, `get`, `describe`) the same name can in
principle name one module in two categories. That is two exact matches, so it is
ambiguous and falls to the menu below. Within a single category names are unique,
so an exact match is always one module.

### Substring fallback

When there is no exact match, the identifier is a substring query that reduces to
a set of matches and one decision:

| Match count | Behaviour |
| ----------- | --------- |
| 0 | Error: not found. |
| 1 | Use it. If the match is registry-only, install it first, then use it. |
| more than 1 | On a TTY, present a selection menu. Without a TTY, error and list the matches. |

The defining property is that resolution never silently runs one module when the
identifier is a *partial* match for several. The menu is reserved for partial
input; an exact whole-name match is not partial input and is never subject to it,
so a module's complete canonical name always reaches that module, in scripts and
pipes as well as interactively.

The naming standard forbids any name from being an ancestor of another within a
category, and the registry index is validated to enforce this (see the naming
standard, Leaf-Only Names). The only way an exact name can overlap longer names
is therefore a substring sibling such as `jira/item/read` inside
`jira/item/read-only`; the exact-match tier resolves it.

## Search sources

Matches are collected from two sources with no priority between them:

1. Installed config (the merged global and local configuration).
2. The registry index.

Results are merged and de-duplicated by `category:name`. A module present in
both sources is one match. A match that exists only in the registry is installed
on selection (single match: install then use; menu: install the chosen entry).

## Input forms

The identifier is interpreted before matching:

| Input | Interpretation | Match mode |
| ----- | -------------- | ---------- |
| `foo` | bare term | substring over the name |
| `foo/bar` | bare term containing a slash | substring over the name |
| `tasks:foo` | category-qualified term | prefix over the name, scoped to that category |
| `/foo`, `./foo`, `~/foo` | filesystem path | no search; read the file directly |

Notes:

- Matching is case-insensitive and targets the module name only. Description and
  tag matching belong to `start search`, not to resolution.
- A bare term is a substring match: `foo/bar` matches `foofoo/barbar` because the
  literal `foo/bar` appears inside it. A slash in a bare term is an ordinary
  character, not a path separator.
- A leading `/`, `./`, or `~` marks a filesystem path. The path is read directly
  and the search procedure is skipped entirely.

## Category prefix

A `category:name` identifier names one of the four categories: `agents`, `roles`,
`contexts`, `tasks`. The prefix does two things: it scopes the search to that one
category, and it switches the name match from substring to prefix (the name must
start with the supplied term).

Prefix rules by surface:

- Category-specific surfaces (`start task`, `--role`, `--context`, `--agent`):
  the prefix is optional. When present it must equal the surface's own category;
  a mismatched prefix is an error (for example `roles:foo` passed to `start task`).
- Cross-category surfaces (`start <id>`, `get`, `describe`): no prefix searches
  all four categories; a prefix narrows to the named category.

Examples:

- `tasks:jira` matches `jira/item/review` and `jira/item/backlog/review` (both
  start with `jira`), scoped to tasks.
- `tasks:review` matches only names beginning with `review`. It does not match
  `jira/item/review`, because that name does not start with `review`.

## Selection

When more than one match is found and a TTY is present, list the matches and
prompt for a choice. Each entry is shown as `category:name` with its source
(installed or registry). Accept either the entry number or a typed name. A typed
name that uniquely identifies one shown entry selects it.

Without a TTY, return an error that lists the matches as `category:name` and
instructs the user to specify an exact name or run interactively. The listed
forms are valid command arguments that round-trip back to the same entry.

## Per-category behaviour

- Tasks, roles, agents: resolve to exactly one module via the hit rule.
- Contexts: each explicit `--context` term resolves independently via the hit
  rule. Multiple terms select multiple contexts; within a single term the hit
  rule still holds (one term that matches several contexts menus or errors). The
  `default` and `none` sentinels are not searched. Required and default contexts
  load automatically and are not subject to term resolution.
- Agents: the procedure runs only when `--agent` is supplied. Otherwise the
  configured default agent is used without resolution.

## Worked examples

Assume installed and indexed tasks include `cwd/project/review`,
`jira/item/review`, `jira/item/backlog/review`, and `gitlab/pipeline/review`.

| Command | Matches | Result |
| ------- | ------- | ------ |
| `start task review` | all four (substring `review`) | menu on TTY, ambiguity error otherwise |
| `start task jira/item/review` | exact match | run it |
| `start task pipeline` | one (substring) | run it |
| `start task tasks:jira` | the two `jira/...` tasks (prefix) | menu or error |
| `start task tasks:gitlab` | one (prefix) | run it |
| `start task ./my-task.md` | n/a | read the file, no search |
| `start task nonsense` | none | not-found error |

Exact match takes precedence over substring siblings. With installed tasks
`jira/item/read` and `jira/item/read-only`:

| Command | Matches | Result |
| ------- | ------- | ------ |
| `start task jira/item/read` | exact `jira/item/read`; substring sibling `jira/item/read-only` | run `jira/item/read` directly, including without a TTY |
| `start task read` | substring: both | menu on TTY, ambiguity error otherwise |

The first command never menus: the identifier is the complete name of one module.
The naming standard forbids a name that is an ancestor of another, so substring
siblings like these — neither an ancestor of the other — are the only way an
exactly-typed name overlaps longer names.
