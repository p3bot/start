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
| `start get <id>`, `start describe <id>` | positional argument | all four |

Model resolution (`--model`) is out of scope. A model is resolved against the
selected agent's `models` map, not against config and registry modules.

## The match rule

Resolution checks for an exact whole-name match first — for every non-path input
form, bare or category-qualified — then falls back to a query. The fallback mode
depends on the input form: a bare term falls back to a substring query, a
category-qualified term to a prefix query (see Input forms and Category prefix).

### Exact match

A single exact match is unambiguous by definition: the identifier equals the full name
of exactly one module, compared case-insensitively (any casing of a module's
complete name resolves directly). It resolves to that module directly — even when
the name is also a substring of longer names, and even without a TTY. An exact
match that exists only in the registry is installed first, then used.

On cross-category surfaces (`get`, `describe`) the same name can in
principle name one module in two categories — two exact matches. The exact tier
here spans both sources across every scoped category, so a same-name exact in
another category is detected whether it is installed or registry-only. A single
exact match is the canonical result and resolves directly (a registry-only one
installs first). When the name is an exact match in more than one category —
two installed, an installed one alongside a registry-only one in another category,
or two registry-only ones when nothing is installed — it is a genuine ambiguity
that falls to the menu below, resolved by category-qualifying the name. This
behaves the same way with or without a TTY: a terminal shows the menu (installing
a chosen registry-only entry), a pipe returns the ambiguity error. Within a single
category names are unique — the naming standard's lowercase kebab-case keeps them
unique case-insensitively — so an exact match is always one module.

Note: because the cross-category exact tier consults the registry, a bare name on
`get`/`describe` depends on registry contents as well as installed config. A
script that resolves a bare name today, as the sole exact match, can begin
returning an ambiguity error later if a same-name module is published in another
category. Category-qualify the name (`category:name`) in scripts and pipes to pin
resolution to one category and stay immune to new registry twins. The
category-specific surfaces (`--role`, `--agent`, `start task`) are not affected:
their exact tier is scoped to one category, so no twin can arise.

### Fallback query

When there is no exact match, the identifier becomes a fallback query — a
substring query for a bare term, a prefix query for a category-qualified term
(see Category prefix) — that reduces to a set of matches and one decision:

| Match count | Behaviour |
| ----------- | --------- |
| 0 | Error: not found — or, when the registry index was unreachable, the retry-able error described under Search sources. |
| 1 | Use it. If the match is registry-only, install it first, then use it. |
| more than 1 | On a TTY, present a selection menu. Without a TTY, error and list the matches. |

The fallback query must be at least three characters; a shorter query is rejected
with an error before the fallback search runs. The minimum counts the name being
matched, excluding any `category:` prefix: both `ab` and `tasks:ab` are rejected,
because the prefix only narrows the category and adds nothing to the name match.
This floor is uniform across every surface and applies to both the substring and
prefix fallback modes. The exact-whole-name tier runs first and is exempt: a
complete canonical name resolves at any length, including names shorter than three
characters. The discriminator is exact-versus-partial, not length: `tasks:ci`
still resolves because `ci` is a complete name matched by the exact tier, while
the equally short `tasks:ab` is rejected because `ab` is not a complete name and
falls to a broad prefix scan.

The defining property is that resolution never silently runs one module when the
identifier is a *partial* match for several. The menu is reserved for partial
input; an exact whole-name match is not partial input and is never subject to it,
so within its category a module's complete canonical name always reaches that
module, in scripts and pipes as well as interactively (a registry-only name
additionally needs a reachable index; see Search sources). The one exception is a
cross-category surface (`get`, `describe`) where the same name is an exact match
in two *installed* categories: the bare name is then ambiguous and must be
category-qualified (`tasks:<name>`) to resolve without a menu. This covers a
registry-only same-name exact in another category too: the exact tier consults
the registry, detects the twin, and treats it as the same kind of ambiguity —
the same in a pipe (ambiguity error) as on a terminal (menu).

The naming standard forbids any name from being an ancestor of another within a
category, and the registry index is validated to enforce this (see the naming
standard's Leaf-Only Names section: `start get start/library/naming`). An exact
name can still appear as a substring of longer, unrelated names — as a prefix, an
interior segment, or a suffix, and whether or not they share a parent path.
Examples: `jira/item/read` inside `jira/item/read-only`, or `review` inside
`gitlab/pipeline/review`. The exact-match tier resolves the typed name directly
in every such case.

## Search sources

Matches are collected from two sources with no priority between them:

1. Installed config (the merged global and local configuration).
2. The registry index.

Results are merged and de-duplicated by `category:name`. A module present in
both sources is one match; the installed entry is used and no install occurs, so
"no priority" governs only which matches are collected, not this de-duplication.
A match that exists only in the registry is installed on selection (single match:
install then use; menu: install the chosen entry).

The registry index is fetched lazily and its absence is non-fatal: when the index
cannot be reached, resolution proceeds against installed config alone. The
guarantees above about registry-only modules — exact-match install-then-use and
registry entries in the menu — are therefore conditional on a reachable index.
Installed modules resolve unconditionally. An uninstalled identifier is a
different matter: with the index reachable and the name found in neither source it
is genuinely not found (a not-found error), but with the index unreachable its
absence cannot be confirmed — the name may well exist in the registry — so the
failure is reported as a transient, retry-able error rather than not-found. This
matches how the rest of `start` treats an unreachable registry (`install`,
`update`, and `search` with no local match all report the same transient error).
Scripts and pipes that must resolve a registry module should install it ahead of
time.

## Input forms

The identifier is interpreted before matching. The exact-whole-name tier runs
first for every non-path form; the Fallback mode column applies only when no
exact match exists:

| Input | Interpretation | Fallback mode |
| ----- | -------------- | ------------- |
| `foo` | bare term | substring over the name |
| `foo/bar` | bare term containing a slash | substring over the name |
| `tasks:foo` | category-qualified term | prefix over the name, scoped to that category |
| `/foo`, `./foo`, `~`, `~/foo` | filesystem path | no search; read the file directly |

Notes:

- The exact tier precedes the fallback for qualified input as well as bare: a
  category-qualified complete canonical name (`tasks:jira/item/read`) resolves
  to that module directly, even when it is a string-prefix of a sibling
  (`jira/item/read-only`) that the prefix fallback would otherwise also match.
- Matching is case-insensitive and targets the module name only. Description and
  tag matching belong to `start search`, not to resolution.
- A bare term is a substring match: `foo/bar` matches `foofoo/barbar` because the
  literal `foo/bar` appears inside it. A slash in a bare term is an ordinary
  character, not a path separator.
- A leading `/`, `./`, or `~` (including a bare `~`) marks a filesystem path. The
  path is read directly and the search procedure is skipped entirely. This applies
  to every surface that yields a document body: the `--role` and `--context` flags,
  `start task`, and the cross-category `get`/`describe`, which read and display the
  file directly. The sole exception is `--agent`: it does not accept a filesystem
  path, because an agent is a structured configuration rather than a document body,
  so a path supplied to `--agent` is an error.

## Category prefix

A `category:name` identifier names one of the four categories — `agents`, `roles`,
`contexts`, `tasks` — and navigates that category's namespace from its root. Names
within a category are paths (`jira/item/review`), so the qualifier scopes the
search to the named category and descends from `name`: the fallback matches names
that begin with the supplied term (a prefix), where a bare term matches the term
anywhere in the name (a substring). The exact-whole-name tier still runs first, so
a qualified complete name resolves directly; prefix matching applies only when no
exact match exists.

The two fallback modes are the two ways you reach for a module. A bare term is a
fuzzy "find it anywhere" search. A category-qualified term is precise namespace
navigation: you name the category and the start of the path. `start task review`
finds any task whose name contains `review`; `start task tasks:review` finds only
tasks whose name begins with `review`. The qualified form is therefore not a
scoped synonym for the bare form — it asks a different question, and on a
category-specific surface (where the category is already fixed) that different
question is the only reason to add the prefix.

Prefix rules by surface:

- Category-specific surfaces (`start task`, `--role`, `--context`, `--agent`):
  the prefix is optional. When present it must equal the surface's own category;
  a mismatched prefix is an error (for example `roles:foo` passed to `start task`).
- Cross-category surfaces (`get`, `describe`): no prefix searches
  all four categories; a prefix narrows to the named category.

Examples:

- `tasks:jira` matches `jira/item/review` and `jira/item/backlog/review` (both
  begin with `jira`), scoped to tasks.
- `tasks:review` matches only names beginning with `review`. It does not match
  `jira/item/review`, because that name does not begin with `review`; use the bare
  term `review` for the anywhere-in-the-name search.

## Selection

When more than one match is found and a TTY is present, list the matches and
prompt for a choice. Each entry is shown with its source (installed or registry).
On the category-specific surfaces (`start task`, `--role`, `--context`,
`--agent`) the entry is its bare name, since every match shares the surface's one
category; on the cross-category surfaces (`get`, `describe`) it is shown as
`category:name`, because the category is what distinguishes the matches. Accept
either the entry number or a typed name. A typed name that uniquely identifies one
shown entry selects it.

Without a TTY, return an error that lists the matches in the same form — bare
names on a category-specific surface, `category:name` on a cross-category one —
and instructs the user to specify an exact name, category-qualify it
(`category:name`) when the collision spans categories, or run interactively. The
listed forms are valid command arguments that round-trip back to the same entry.

## Per-category behaviour

- Tasks, roles, agents: resolve to at most one module via the match rule — a
  single result is acted on directly; several matches menu on a TTY or error
  without one.
- Contexts: each explicit `--context` term resolves independently via the match
  rule. Multiple terms select multiple contexts; within a single term the match
  rule still holds (one term that matches several contexts menus or errors). The
  `default` and `none` sentinels are not searched. Required and default contexts
  load automatically and are not subject to term resolution.
- Agents: the procedure runs only when `--agent` is supplied. Otherwise the
  configured default agent is used without resolution. An agent identifier is
  always a name; `--agent` does not accept a filesystem path.
- Sentinels skip resolution entirely: `--role none` skips role assignment, just
  as the context `none` and `default` sentinels above are never searched.

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
| `start task rv` | exact tier: no match | error: fallback query under three characters |

Exact match takes precedence over substring matches. With installed tasks
`jira/item/read` and `jira/item/read-only`:

| Command | Matches | Result |
| ------- | ------- | ------ |
| `start task jira/item/read` | exact `jira/item/read`; substring match `jira/item/read-only` | run `jira/item/read` directly, including without a TTY |
| `start task read` | substring: both | menu on TTY, ambiguity error otherwise |

The first command never menus: the identifier is the complete name of one module.
The naming standard forbids a name that is an ancestor of another, but an
exactly-typed name can still be a substring of longer, unrelated names — here
`jira/item/read` inside `jira/item/read-only`, and elsewhere across different
parent paths. The exact-match tier resolves the typed name regardless of how it
overlaps those longer names.
