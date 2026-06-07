# Module Resolution

Reference for how `start` turns a user-supplied identifier into a module to act
on. This procedure is uniform across tasks, roles, contexts, and agents. The
guiding principle is KISS: search the installed config and the registry index
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

## The hit rule

Every resolution reduces to a set of matches and one decision:

| Match count | Behaviour |
| ----------- | --------- |
| 0 | Error: not found. |
| 1 | Use it. If the match is registry-only, install it first, then use it. |
| more than 1 | On a TTY, present a selection menu. Without a TTY, error and list the matches. |

There is no priority tier. An exact whole-name match does not win over other
matches. If a name matches exactly and is also a substring of other names, the
result is multiple matches and the hit rule applies (menu or ambiguity error).
This is the defining property of the procedure: resolution never silently runs
one module when the identifier also matched others.

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
| `start task jira/item/review` | one (substring) | run it |
| `start task tasks:jira` | the two `jira/...` tasks (prefix) | menu or error |
| `start task tasks:gitlab` | one (prefix) | run it |
| `start task ./my-task.md` | n/a | read the file, no search |
| `start task nonsense` | none | not-found error |
