# Project: Unify module resolution on the documented match rule

## Goal

Module resolution in `start` has drifted from its specification. Resolution
reuses the `start search` scoring engine, carries a "short-name" matching tier
that the spec does not define, duplicates and diverges across multiple code
paths, and treats an exact whole-name match as ambiguous whenever the name is
also a substring of a longer name. Refactor resolution so a single, name-only engine
implements the match rule described in `docs/module-resolution.md` uniformly
across tasks, roles, contexts, agents, and the cross-category `get`/`describe`
surfaces.

## Scope

In scope:

- One resolution engine that all five surfaces consume: `start task`, `--role`,
  `--context`, `--agent`, and `start get`/`start describe`.
- Name-only matching with three behaviours: exact whole-name (case-insensitive),
  substring fallback (bare terms), prefix fallback (category-qualified terms).
- The exact-match-wins rule: a complete canonical name resolves directly, even
  when it is a substring of longer names, and even without a TTY.
- The three-character floor on fallback queries, exempting the exact tier.
- Merging installed config and the registry index as two equal sources,
  de-duplicated by `category:name`, with registry-only matches installed on use.
- Per-term, single-match context resolution (replacing multi-select-above-
  threshold), with a not-found error when an explicit term matches nothing.
- Removal of the duplicated task-specific and context-specific resolution paths
  and of `resolveCrossCategory`'s bespoke tier logic, all replaced by the shared
  engine.
- Removal of the `--tag` filter from `start task`. Tag (and description) matching
  is a discovery concern that belongs to `start search`: you search to find a
  module, then use it by name. The name-only resolver has no tag dimension, so
  `start task --tag` is removed along with the task-specific search helpers it
  depends on, and the coupling that today suppresses the exact tier whenever a
  tag filter is set goes with it. `start search --tag` is unchanged.
- Rewriting the resolution test suite to assert the documented behaviour,
  including inverting the exact-match tests that project 03 deferred here.
- Updating the resolution description in AGENTS.md to match the new model.

Out of scope:

- Model resolution (`--model`). It resolves against the selected agent's `models`
  map, not against config and registry modules. `resolveModelName` is unchanged.
  It therefore stays the one resolution surface that keeps the search-style match
  (multi-term substring via `ParseSearchTerms`, passthrough on ambiguity) rather
  than the unified name-only rule; the divergence is intentional, because the
  match target is an agent's model map rather than the module sources.
- The `start search` command and its scoring engine (`matchScorePatterns`,
  description/tag weighting, multi-term AND). Search keeps description and tag
  matching; resolution must stop using it. Do not change search behaviour.
- The registry fetch, cache, and install mechanics (`ensureIndex`,
  `cache.ReadIndex`, `InstallModule`). Resolution continues to call them; their
  internals do not change.
- The execution seam and test-isolation work owned by project 03 (see Current
  State). This project depends on that seam but does not modify it.

## Current State

Resolution today is spread across three implementations that should be one.

`internal/cli/resolve.go` holds the `resolver` type and `resolveModule`, used by
`--role` and `--agent`. It runs a two-phase strategy: an exact installed-key
check (`isExactInstalledKey`), then a merged installed+registry search via
`searchInstalled`/`searchRegistryCategory` and `selectSingleMatch`. The search
helpers delegate to `modules.SearchInstalledConfig` and
`modules.SearchCategoryEntries`, which score names, descriptions, and tags.

`internal/cli/cross_resolve.go` holds `resolveCrossCategory`, used by
`start get` and `start describe`. It re-implements the tiers across all four
categories and contains the exact-falls-through-to-menu logic this project
removes (lines that return the exact match only when `len(installedMatches) <= 1`,
i.e. "no neighbours").

`internal/cli/task.go` (`executeTask`, roughly lines 188-291) re-implements the
same strategy a third time with task-specific types: `findInstalledTasks`,
`findRegistryTasks`, `mergeTaskMatches`, `TaskMatch`, `promptTaskSelection`.

`resolveContexts` in `resolve.go` resolves `--context` terms but diverges from
the match rule: it collects every match above `contextScoreThreshold` (multi-
select within a single term), checks the registry only when installed produced
nothing, and passes unresolved terms through for the composer to warn on.

Key divergences from `docs/module-resolution.md`:

1. Resolution matches against description and tags, not the name only.
   `matchScorePatterns` scores name (3), description (1), and tags (1). The spec:
   "Matching is case-insensitive and targets the module name only."
2. A short-name tier matches a trailing path segment as if exact:
   `findExactInstalledName` and `findExactInRegistry` resolve `assistant` to
   `golang/assistant`. The spec has no such tier; `assistant` is a substring
   fallback.
3. No three-character floor on the category-specific surfaces. The spec's
   `start task rv` must error before the fallback search; today it does not. The
   cross-category surfaces do have a floor, but at the wrong layer and with the
   wrong semantics: `runDescribe` (`describe.go:139`) and `runGet` (`get.go:149`)
   reject a query shorter than three characters before `resolveCrossCategory`
   runs. That check measures the whole raw argument (so `tasks:ab` passes on
   length while bare `ci` is rejected), precedes the exact tier (so an exact
   two-character name never resolves), and on a TTY `describe` re-prompts for a
   new query rather than erroring. All three behaviours conflict with the spec's
   floor (name-only length excluding the `category:` prefix, exact tier exempt,
   error rather than re-prompt) and must be replaced by the in-engine floor.
4. Category-qualified terms do not switch to prefix matching. `tasks:review`
   matches by substring like a bare term; the spec requires prefix matching
   (names that start with `review`).
5. Exact match is case-sensitive. `isExactInstalledKey` uses a verbatim CUE key
   lookup. The spec compares the whole name case-insensitively.
6. Exact-plus-substring is treated as ambiguous. With tasks `review` and
   `start/review`, `start task review` currently falls through to a menu/error.
   The spec resolves the exact `review` directly.
7. Sources are not merged with equal priority. `resolveContexts` and
   `resolveCrossCategory` gate the registry behind an empty installed result.
   The spec merges both sources and de-duplicates by `category:name`.
8. Matching is regex-based (`CompileSearchTerms` compiles `(?i)term`). The spec
   treats the identifier as a literal: a slash is an ordinary character and
   `foo/bar` matches `foofoo/barbar` because the literal substring appears.
9. Resolution splits the identifier into multiple AND terms
   (`ParseSearchTerms`). A resolution identifier is a single name.

Beyond these divergences from the spec, the surfaces also disagree with each
other on one axis the spec must settle: the exit code when the registry index is
unreachable and the name is not installed. `resolveModule` (`--role`/`--agent`)
swallows the index error in `ensureIndex` and reports not-found; `executeTask`
(`task.go:225-234`) preserves the typed `indexErr` and returns a transient error
so the retry signal survives, with a comment defending that choice. The unified
engine resolves this by certainty (see Requirement 5 and the Constraints): a
reachable index with no match is not-found, an unreachable index for an
uninstalled name is transient, applied uniformly.

Tests embody the pre-spec behaviour and must be rewritten. The most important:
`TestTaskResolution_ExactMatchFallsThrough`, `TestResolveCrossCategory_ExactPlusSubstringFallThrough`,
`TestResolveCrossCategory_ExactPlusCrossCategorySubstring`, and
`TestTaskResolution_RegistryGuardAmbiguous` all assert the exact-falls-through
behaviour that the spec reverses. Short-name tests
(`TestFindExactInstalledName` short-name subtests, `TestFindExactInRegistry`,
`TestFindExactInRegistryAmbiguous`, `TestResolveCrossCategory_AmbiguousShortNameNonTTY`)
assert a tier that no longer exists. `TestContextScoreThreshold_LowScoreExcluded`
asserts the score threshold that is being removed.

Project dependency: project 03 (honest test execution) introduces an injectable
execution seam so resolution tests that reach the execution path record the
command instead of calling `syscall.Exec`. Project 03 also deferred
`TestTaskResolution_ExactMatchFallsThrough` to this project. Project 04 assumes
that seam is in place; resolution tests that exercise `executeTask` use the
recorder rather than running an agent. This project supersedes project 03's
deferral by rewriting that test to assert exact-wins, not by un-skipping it.

## References

- `docs/module-resolution.md` — the authoritative specification for this
  project. Every behaviour requirement below traces to it.
- `library/schemas/` and `start get start/library/naming` — the naming standard
  the spec relies on (lowercase kebab-case, leaf-only names, no name is an
  ancestor of another within a category). The exact-match guarantees depend on
  these invariants; do not re-derive or weaken them.

## Requirements

1. Provide one resolution engine, consumed by every surface in scope. Tasks,
   roles, agents, and each context term resolve to at most one module through the
   same code path. `get`/`describe` resolve across all four categories through
   the same path with a wider category scope. The task-specific
   (`findInstalledTasks`, `findRegistryTasks`, `mergeTaskMatches`, `TaskMatch`,
   `promptTaskSelection`) and context-specific resolution code is removed in
   favour of the shared engine.

2. Interpret the identifier before matching, per the spec's Input forms:
   - A leading `/`, `./`, `~`, or `~/` marks a filesystem path: read the file
     directly, no search. This applies to every document-yielding surface
     (`--role`, `--context`, `start task`, `get`, `describe`). `--agent` does not
     accept a path; a path supplied to `--agent` is an error.
   - A `category:name` prefix naming one of `agents`, `roles`, `contexts`,
     `tasks` scopes the search to that category and selects prefix fallback. On a
     category-specific surface the prefix is optional and must equal that
     surface's category; a mismatched prefix is an error. An unknown category is
     an error.
   - A bare term selects substring fallback. A slash in a bare term is an
     ordinary character, not a path separator.

3. Run the exact-whole-name tier first for every non-path input, bare or
   qualified. Compare the whole name case-insensitively against module names
   only. A single exact match resolves directly — including when the name is a
   substring of longer names, including without a TTY, and including a registry-
   only match (install then use). The exact tier is exempt from the three-
   character floor. On the cross-category surfaces the exact tier spans both
   sources across every scoped category, so a same-name exact in another category
   is detected wherever it lives, installing a registry-only one first. Two or
   more exacts across categories — two installed, an installed one alongside a
   registry-only one in another category, or two registry-only ones when nothing
   is installed — are a genuine ambiguity that falls to selection, resolved by
   category-qualifying the name; this is identical with or without a TTY (menu on
   a terminal, ambiguity error in a pipe).

4. When no exact match exists, run the fallback query against module names only:
   - Bare term: case-insensitive literal substring.
   - Category-qualified term: case-insensitive literal prefix.
   - The identifier is a single literal term. Do not split it into multiple AND
     terms and do not interpret it as a regex.
   - Reject a fallback query shorter than three characters with an error before
     searching. The length counts the name only, excluding any `category:`
     prefix (`ab` and `tasks:ab` are both rejected). The floor is uniform across
     surfaces and both fallback modes.

5. Collect matches from installed config and the registry index as two sources
   with no priority. Merge and de-duplicate by `category:name`: a module present
   in both sources is one match, the installed entry is used, and no install
   occurs. A registry-only match is installed on selection. The registry index
   is fetched lazily and its absence is non-fatal: when it cannot be reached,
   resolution proceeds against installed config alone. An uninstalled identifier
   is classified by certainty: with the index reachable and no match in either
   source it is a not-found error, but with the index unreachable its absence
   cannot be confirmed and it is reported as a transient (retry) error, matching
   how `install`, `update`, and `search` treat an unreachable registry. A single
   installed exact match resolves directly on the category-specific surfaces
   (`--role`, `--agent`, `start task`) without consulting the registry index:
   names are unique within a category, so no same-name twin can exist, and the
   resolution stays fast and offline regardless of the index cache state.
   The cross-category surfaces (`get`, `describe`) do not skip the index; their
   exact tier and its ambiguity resolution follow Requirement 3.

6. Reduce the fallback match set to one decision: zero matches is a not-found
   error; one match is used (installed first if a registry-only match is
   chosen); more than one presents a selection menu on a TTY and returns an
   error listing the matches otherwise. The menu and the non-TTY error list each
   entry with its source (installed or registry): a bare name on the
   category-specific surfaces, where every match shares the one category, and
   `category:name` on the cross-category surfaces, where the category
   distinguishes the matches. The listed forms are valid arguments that round-trip
   back to the same entry. The menu accepts an entry number or a typed name that
   uniquely identifies one shown entry. Use one selection implementation for all
   surfaces, parameterised by whether its scope spans one category or four.

7. Apply per-category behaviour:
   - Tasks, roles, agents: resolve to at most one module.
   - Contexts: each explicit `--context` term resolves independently through the
     match rule, including the not-found error when a term matches nothing.
     Multiple terms select multiple contexts. The `none` and `default` sentinels
     are not searched. Required and default contexts continue to load
     automatically and are not subject to term resolution. The score threshold
     and multi-select-per-term behaviour are removed.
   - Agents: resolution runs only when `--agent` is supplied; otherwise the
     configured default agent is used without resolution.
   - The `--role none` sentinel continues to skip role assignment without
     resolution.

8. Decouple module resolution from the search scoring engine. The module match
   over installed config and the registry index — excluding model resolution —
   must not call `SearchInstalledConfig`, `SearchCategoryEntries`,
   `matchScorePatterns`, `ParseSearchTerms`, or `CompileSearchTerms`. Those remain
   in use by `start search` and must keep their current behaviour. Module
   resolution gets its own name-only matching over installed config fields and
   registry index entries. Model resolution (`resolveModelName`) is out of scope
   and its existing `ParseSearchTerms` use is retained unchanged.

9. Rewrite the resolution test suite to assert the documented behaviour:
   - Invert the exact-match tests so an exact whole-name match that is also a
     substring of longer names resolves directly, on a TTY and without one,
     across `start task`, `--role`, `--agent`, and cross-category `get`/`describe`.
   - Remove the short-name tier tests; convert the relevant cases to substring-
     fallback tests (`assistant` now matches `golang/assistant` by substring).
   - Add the three-character floor cases, including the exact-tier exemption
     (`tasks:ci` resolves, `tasks:ab` and `ab` are rejected).
   - Add tests distinguishing prefix from substring fallback (`tasks:review`
     prefix versus `review` substring return different sets).
   - Add cross-category exact cases by source, all resolving identically with and
     without a TTY (menu on a terminal, ambiguity error in a pipe): two installed
     exacts in different categories are ambiguous; an installed exact alongside a
     registry-only same-name exact in another category is likewise ambiguous (the
     registry twin is detected, offered in the menu and installed if chosen, and
     listed in the non-TTY error); a single exact in one category resolves directly
     whichever source holds it.
   - Add name-only tests: an identifier that matches only a description or tag
     resolves to not-found.
   - Replace the context score-threshold and multi-select tests with single-
     match-per-term tests, including the not-found error on an unresolved term.
   - Add exit-code certainty cases: an uninstalled name with the index reachable
     and no match is not-found; the same name with the index unreachable is
     transient (retry), not not-found; an installed name resolves regardless. Assert
     this across `start task`, `--role`, and `--agent` so the previously divergent
     paths agree.
   - Tests that reach `executeTask`'s execution path use project 03's execution
     recorder seam; prefer asserting the resolver unit directly where possible.

10. Update the resolution description in AGENTS.md (the "Resolution Logic" and
    "Resolution" subsections under Architecture, and the `resolve.go` row in Key
    Files) to describe the unified, name-only match rule instead of the current
    "three-tier (exact config → registry → substring)" description.

## Constraints

- `docs/module-resolution.md` is authoritative. Where the current code, current
  tests, or project 03's framing conflict with it, the spec wins. Do not weaken
  or amend the spec to fit the code.
- Resolution matching is literal and case-insensitive over names only, with the
  case-insensitivity carried by folding both operands before comparison:
  case-folded equality (`strings.EqualFold`) for exact, `strings.Contains` over
  lower-cased names for substring, `strings.HasPrefix` over lower-cased names for
  prefix. No regular expressions, no description or tag matching, no multi-term
  splitting.
- The naming-standard invariants hold and are relied upon: within a category
  names are unique case-insensitively and no name is an ancestor of another.
  Resolution may assume these; it does not re-validate them.
- `start search` behaviour is unchanged. The scoring engine stays where it is.
- Production behaviour for real invocations is otherwise preserved: lazy registry
  fetch, graceful fallback when the index is unreachable, auto-install of a
  selected registry-only module to the global config, and config reload after an
  install so the new module is visible.
- Error classification and exit codes follow the certainty of the outcome. A
  genuine absence — the index was reachable and the name matched in neither
  source — is a not-found error (`notFoundError`). An uninstalled name whose
  absence cannot be confirmed because the index is unreachable is a transient,
  retry-able error (`registry.FetchError`/`ExitTransient`), consistent with
  `install`, `update`, and `search`. This unifies a pre-existing divergence
  (today `--role`/`--agent` swallow the index error and report not-found while
  `start task` preserves the transient signal): every resolution surface now
  applies the same certainty split. An ambiguity without a TTY and a too-short
  query are usage errors (`usageError`). The malformed-input conditions this
  project adds are also usage errors (`usageError`): a filesystem path supplied
  to `--agent`, a category prefix that does not equal a category-specific
  surface's own category, and an unknown category prefix. See
  `internal/cli/exitcodes.go` and `start help schemas`.
- Follow the repository testing approach (AGENTS.md): real CUE validation, real
  files via `t.TempDir()`, table-driven cases, dependencies injected as
  parameters or interfaces, `skipRegistry`/stub client for offline tests.

## Implementation Plan

1. Build the name-only matching primitive. Given a query, a fallback mode
   (exact-only, substring, or prefix), and a set of candidate names, return the
   matching names. Source it from two collectors: installed config fields (the
   keys under `agents`/`roles`/`contexts`/`tasks`) and registry index entries.
   Keep it independent of `modules.search`.

2. Build the unified resolver flow on top of the primitive. Parameterise it by
   the scoped categories (one for category-specific surfaces, four for cross-
   category) and whether file paths are accepted. The flow is: interpret input
   (path bypass, category prefix, fallback mode) → exact tier over the merged
   sources within scope → if no exact match, apply the three-character floor,
   then the fallback tier over the merged sources → reduce to one decision
   (not-found, use, or select) → install a chosen registry-only match → return.
   On the category-specific surfaces a single installed exact resolves without
   fetching the registry index, since names are unique within a category. The
   cross-category surfaces do not short-circuit: the exact tier must consult the
   registry across all scoped categories to detect a same-name exact in another
   category, then resolve a lone exact directly or fall to selection when more than
   one exists. The result is identical with or without a TTY.

3. Collapse selection into one implementation. Replace `selectSingleMatch`/
   `promptModuleSelection`, `promptCrossCategorySelection`, and
   `promptTaskSelection` with a single selector that renders entries with source —
   bare names on a category-specific surface (the existing `start task` menu
   format), `category:name` on a cross-category one — handles the non-TTY error
   list, and accepts a number or a uniquely-identifying typed name.

4. Route `--role` and `--agent` through the unified resolver (single category;
   `--role` accepts a path, `--agent` rejects one). Route `start get` and
   `start describe` through it with all four categories. Remove
   `resolveCrossCategory`'s bespoke tier logic. Remove the command-level
   three-character floors in `runDescribe` (`describe.go:139`) and `runGet`
   (`get.go:149`), including the `describe` TTY re-prompt, so the unified engine's
   exact-exempt, prefix-excluding floor is the only one these surfaces apply.

5. Route `start task` through the unified resolver. Keep the file-path read and
   the template processing, and keep the task-role follow-up as a feature
   (`GetTaskRole` still supplies the role a task declares), but route its role
   resolution through the unified resolver as well: drop the
   `findExactInstalledName` pre-check and the short-name branching around it
   (`task.go:307-336`) so a task's role resolves by the same match rule as
   `--role`. Replace the inline installed/registry search and selection with the
   shared engine, and remove `findInstalledTasks`, `findRegistryTasks`,
   `mergeTaskMatches`, `TaskMatch`, and `promptTaskSelection`.

6. Reimplement `resolveContexts` as a loop over terms that, per term, handles the
   path bypass and the `default` passthrough, then delegates to the unified
   single-match resolver. The `none` sentinel is not handled here: it is consumed
   upstream by the sentinel layer (`sentinel.go`, applied in `root.go`) before
   `resolveContexts` is called, so a `none` term never reaches this loop and must
   not be re-handled in it. Remove `contextScoreThreshold`, the multi-select
   collection, and the unresolved-term pass-through. An explicit term that
   matches nothing is a not-found error.

7. Remove now-dead resolution helpers: `findExactInstalledName`,
   `findExactInRegistry`, `isExactInstalledKey`, `searchInstalled`,
   `searchRegistryCategory`, and the score-threshold constant, once no caller
   remains. Replace `mergeModuleMatches` with engine-owned merging that
   de-duplicates by `category:name` (the installed entry wins) and orders matches
   deterministically — installed before registry, then lexically by name — with no
   score-based ordering, since resolution no longer scores.

8. Rewrite the resolution tests across `resolve_test.go`, `cross_resolve_test.go`,
   and the task-resolution tests in `start_test.go` per requirement 9. Invert the
   deferred exact-match tests, drop the short-name and score-threshold tests, and
   add the floor, prefix-versus-substring, and name-only cases.

9. Update AGENTS.md's resolution description per requirement 10.

10. Run the full pipeline (`scripts/invoke-tests`) and the tagged suites
    (`integration`, `e2e`, `registry`). Resolve every failure; none of the
    rewritten resolution tests is left skipped.

## Implementation Guidance

- The exact-versus-fallback distinction is the spine of the spec. Keep the exact
  tier a separate, first step that is exempt from the floor and never reaches a
  menu for a single in-category match. Every reported regression in the old
  behaviour traces to the exact tier being entangled with the fallback search.
- "No priority between sources" governs which matches are collected, not de-
  duplication. The same `category:name` in both sources is one match and the
  installed copy wins. Two different names, one per source, are two matches.
- A category prefix scopes the search to one category and navigates its namespace
  from the root, so its fallback is a prefix match. On a category-specific surface
  the prefix is not a scoped synonym for the bare form — `start task review`
  (substring) and `start task tasks:review` (prefix) return different sets.
  Preserve that.
- Case-insensitive exact match means iterating the category's names and comparing
  lower-cased, not a verbatim CUE key lookup.
- The worked examples in `docs/module-resolution.md` are a ready-made test
  matrix. Mirror them directly: the `review` family, `jira/item/read` versus
  `jira/item/read-only`, `tasks:jira` versus `tasks:review`, and `start task rv`.

## Acceptance Criteria

1. With installed tasks `review` and `start/review`, `start task review`
   resolves and runs `review` directly with no menu, on a TTY and with a non-TTY
   stdin. The same holds for `--role`, `--agent`, and `get`/`describe` when the
   typed name is an exact whole name that is also a substring of a longer name.
2. `start task rv` errors with a too-short-query message and never reaches the
   fallback search. `start task tasks:ci`, with an installed task named `ci`,
   resolves via the exact tier despite the two-character name. The exact-tier
   exemption also holds on `get`/`describe`: with an installed module named `ci`,
   `start get ci` and `start describe ci` resolve it rather than being rejected by
   a length floor, while a bare two-character non-exact query and `tasks:ab` are
   both rejected.
3. With installed tasks including `jira/item/review`, `start task review`
   (substring) and `start task tasks:review` (prefix) return different match
   sets; the prefix form excludes `jira/item/review`.
4. An identifier that matches only a module's description or tag, and no name,
   resolves to a not-found error on every surface.
5. A registry-only module whose exact name is typed is installed and then used,
   without a TTY, when the index is reachable; when the index is unreachable the
   same identifier is reported as a transient (retry) error — not not-found,
   since its absence cannot be confirmed — while installed modules still resolve.
   A name that matches in neither source with the index reachable is a not-found
   error. This holds uniformly across `start task`, `--role`, and `--agent`.
6. On `get`/`describe`, a bare name that is an exact match in two installed
   categories produces a selection menu on a TTY and a non-TTY error listing both
   as `category:name`; category-qualifying the name resolves it without a menu.
   The same holds when the collision is between an installed exact and a
   registry-only same-name exact in another category: it is ambiguous identically
   with and without a TTY — a menu on a terminal (installing the registry-only
   entry if chosen), an ambiguity error listing both as `category:name` in a pipe
   — and category-qualifying resolves it. A single exact in one category resolves
   directly regardless of source.
7. Input forms are interpreted before matching. A leading `/`, `./`, or `~`
   reads the file directly with no search on every document surface (`--role`,
   `--context`, `start task`, `get`/`describe`). A filesystem path supplied to
   `--agent`, a category prefix that does not equal a category-specific surface's
   own category, and an unknown category prefix each fail as a usage error.
8. Each `--context` term resolves to a single module via the match rule; a term
   that matches several contexts menus on a TTY or errors without one, and an
   explicit term that matches nothing is a not-found error. The score threshold
   no longer affects results.
9. Module resolution (the name match over installed config and the registry
   index, excluding model resolution) contains no call to `SearchInstalledConfig`,
   `SearchCategoryEntries`, `matchScorePatterns`, `ParseSearchTerms`, or
   `CompileSearchTerms`. `resolveModelName`'s out-of-scope `ParseSearchTerms` use
   is the sole permitted exception. `start search` output is unchanged.
10. The task, role, agent, context, and cross-category surfaces share one
    resolution implementation and one selection implementation; the task-specific
    and context-specific resolution code paths are gone.
11. `scripts/invoke-tests` passes and the `integration`, `e2e`, and `registry`
    tagged suites pass, with no resolution test skipped.
