# Project: Unify module candidate gathering across all selecting surfaces

## Goal

Project 04 unified the resolution surfaces (`start task`, `--role`, `--context`,
`--agent`, `start get`/`start describe`) onto one name-only engine but
deliberately left `search`, `install`, `update`, `config get`, and `config edit`
on the older, per-surface matchers. The codebase now carries three parallel
candidate-gathering implementations, each re-scanning installed config and the
registry index its own way. Finish the unification: route every module-selecting
surface's candidate gathering through one shared, neutral enumeration primitive,
so the installed-and-registry scan lives in one place and each surface exposes a
single not-found point for cross-cutting features (notably the not-found
suggestions in issue #10) to attach to. The primitive only enumerates; the
merge-installed-over-registry-and-de-duplicate step is a resolution policy layered
on top by the resolution callers, not baked into the gather. Matching policy stays
deliberately per-surface: the resolution-style callers share one literal name-only matcher,
`search` and `install` share the regex/tag matcher, and `update` keeps its
category-name matcher — what is unified is the gathering beneath them, not the
match rule on top.

## Scope

In scope:

- One shared candidate-gathering primitive: given a scope (categories) and a
  choice of sources, enumerate every module in scope and return the full candidate
  set tagged by source and config scope (installed-local, installed-global,
  registry), un-deduplicated, with each candidate's index entry retained, and with
  no name/description/tag filtering applied. The installed sources are supplied by
  the caller as a list of (config value, scope) pairs rather than a fixed
  local/global pair: `search`, which loads local and global configs separately,
  passes two scoped sources; the resolver, which holds a single merged config
  (local overriding global), passes that one merged value as one source and ignores
  the scope tag. This keeps the primitive neutral and frees the resolver from
  re-deriving the merge/override semantics it gets for free today. Every candidate carries the entry's
  description, tags, and origin regardless of source: registry candidates from the
  index, installed candidates extracted from their CUE (reusing
  `extractIndexEntryFromCUE`, as `SearchInstalledConfig` does today). This is what
  lets `search` match and display installed entries by description and tag, not just
  name; resolution-style callers ignore the extra fields. The caller selects which
  sources to enumerate — installed-only (config get, update, removal),
  registry-only (install, which replaces its registry-scoped `modules.SearchIndex`
  and keeps installed status as a separate display annotation), or
  installed-and-registry (resolution, search) — so a caller never pays for a
  registry fetch it does not need and the resolver keeps its existing optimisation
  of skipping the registry when a lone installed exact match already resolves. This
  becomes the single place installed and registry candidates are enumerated.
- Merge and de-duplication are a resolution policy, not part of the gather. The
  installed-over-registry merge and `category:name` de-duplication
  (`mergeMatches`) is a separate step applied only by the resolution-style callers
  after gathering; the discovery surfaces consume the raw, source-tagged candidate
  set directly. This is what lets `search` keep its local/global/registry sections
  and installed★ markers and `install` keep each candidate's registry entry — both
  of which a merged-and-deduped set would destroy.
- Matching is a policy the caller applies on top of the gathered set: a reusable
  literal, case-insensitive, name-only matcher (exact / substring / prefix),
  expressing the engine's current rule, is provided alongside the primitive for the
  resolution-style callers; `search` supplies its own regex/tag matcher
  over the same candidates. The primitive itself never filters, so a caller can
  never lose a candidate the primitive would otherwise have matched on a narrower
  axis than that caller intends.
- Migrate `config get` and `config edit` off `resolveAllMatchingNames` /
  `searchAllConfigCategories` onto the shared enumeration primitive (installed-only)
  with the engine's literal name-only matcher, retiring the regex fast-path. Both
  surfaces share the same `searchAllConfigCategories` query entry point today, so
  both must move together — migrating only `config get` would either break
  `config edit` (its matcher deleted out from under it) or leave a second regex
  installed-selector surface beside the one just unified, recreating the parallel
  matchers this project removes. Unlike `config remove`, which reduces to a single
  module, `config get` keeps returning the full set of literal-name matches across
  categories (exact tier, else substring fallback), so the `--json` output stays an
  array and a multi-match still menus on a TTY / errors without one; `config edit`
  reuses the same enumeration and matcher and keeps its own single-selection menu.
  This is a deliberate behaviour change for both, not a transparent swap: the exact
  tier now short-circuits the substring fallback, so a bare name that is also a
  prefix of installed siblings resolves to that one name. For example, with
  `claude` and `claude/edit` both installed, `config get claude` (and
  `config edit claude`) previously returned both (the regex fast-path's
  sibling-fallthrough) and now resolves to only `claude`; the siblings are still
  reachable via a non-exact substring query. The `--json` array *shape* is preserved;
  its *contents* change in this prefix-sibling case, and `config get` now matches an
  exact name exactly as every other engine surface does.
- `resolveInstalledName` is not a query matcher after this migration: once
  `config get` and `config edit` resolve through the shared primitive, every
  remaining caller (the `config list` / `config get --json` builders in
  `buildConfigListItem`, and the migrated print/edit paths) passes an
  already-resolved exact name. Reduce it to a direct installed-name lookup, or
  inline it, so its regex/scoring fallback (`scoreAndSortNames`) is removed rather
  than left dormant. `scoreAndSortNames` is deleted once both its regex callers are
  gone.
- Rework `install` so its candidate gathering flows through the shared primitive
  instead of `modules.SearchIndex`. Only the enumeration layer is unified; install
  keeps its own matcher on top. Install is a discovery surface: it applies the same
  regex/tag matcher `search` uses over the registry candidates the
  primitive enumerates, so it keeps matching module names, descriptions, and tags
  exactly as `modules.SearchIndex` does today, alongside its multi-select,
  registry-first, auto-install front-door behaviour and its separate installed★
  annotation.
- `update` adopts the shared name-matcher but keeps its own enumeration. Its target
  set comes from `collectInstalledModules`, an origin-filtered installed-module
  inventory carrying the `InstalledVer` and `ConfigFile` metadata that
  `checkAndUpdate` needs to compare versions and rewrite config — data the shared
  candidate type does not hold (the primitive carries description/tags/origin, not
  installed version or config-file path), and which a candidate-primitive
  enumeration would drop. That inventory is left as-is; only `update`'s inline
  name half of the filter is replaced by the shared name-matcher; the category half
  keeps its current substring match (`strings.Contains` over the category), so a
  partial query such as `start update task` still selects every installed task
  exactly as the current `name OR category` filter does. `update` does not route its
  enumeration through the candidate primitive.
- Keep `search` consuming the shared primitive for raw candidate enumeration while
  retaining its regex and tag behaviour as a matcher it applies over the
  full candidate set. Because the primitive does not pre-filter by name, search
  still sees description-only and tag-only matches, so its user-visible results
  must not change.
- Ensure each migrated surface funnels its empty result through a single
  not-found point, so issue #10's suggester can attach once per surface.
- Remove the now-dead duplicate matchers and update `docs/module-resolution.md`
  and `AGENTS.md` so the literal/name-only description is accurate for every
  migrated surface.

Out of scope:

- The "did you mean?" suggestion feature itself. It is tracked by issue #10 and is
  written assuming this project is complete. This project only unifies gathering
  and exposes the clean not-found points; it adds no suggestion output.
- `--model` resolution. It resolves against the selected agent's `models` map, not
  config and registry modules, and is unchanged.
- Registry fetch, cache, and install mechanics (`ensureIndex`, `cache.ReadIndex`,
  `InstallModule`). They are called as before; their internals do not change.
- Search's user-visible matching. Its regex, tag, and description matching stay
  exactly as they are today.

## Current State

Three parallel candidate-gathering implementations exist where there should be one
shared enumeration layer. The matchers layered on top stay intentionally distinct
(literal name-only, regex/tag, category-name); what is duplicated today is
the installed-plus-registry scan beneath each.

- Engine (target model): `internal/cli/engine.go` `collectInstalledFrom` and
  `collectRegistry` perform literal, case-insensitive, name-only matching under an
  explicit mode (`modeExact` / `modeSubstring` / `modePrefix`). They feed the
  `matchSource` seam consumed by the registry-backed `resolver` and the
  installed-only `installedMatcher` (`internal/cli/removal.go`). This is the
  behaviour every surface should share.
- `internal/modules/search.go`: `SearchInstalledConfig` and `SearchIndex` perform
  regex matching (`pattern.MatchString`) and tag filtering, with no relevance
  scoring (results sort by category then name). Consumed by
  `start search` (`internal/cli/search.go`) and by `start install`
  (`internal/cli/install.go` calls `modules.SearchIndex` to build its candidate
  list, then `promptModuleSelection` for multi-select).
- `internal/cli/config_helpers.go`: `resolveAllMatchingNames` (with a regex
  fallback) and `searchAllConfigCategories` perform a cross-category installed
  scan. `searchAllConfigCategories` is consumed by both `start config get`
  (`internal/cli/config_get.go`) and `start config edit`
  (`internal/cli/config_edit.go`). The regex `resolveInstalledName` (sharing
  `scoreAndSortNames` with `resolveAllMatchingNames`) is consumed by the migrated
  print/edit paths and by `buildConfigListItem` (`internal/cli/config_list.go`),
  which backs `config get --json` and `config list --json`; all callers pass names
  already resolved to an exact installed entry.
- `internal/cli/update.go`: an inline `strings.Contains` filter over installed
  modules, matching the query against name or category.

Import direction: `internal/cli` imports `internal/modules`, not the reverse. The
shared primitive and its result type must therefore live in `internal/modules`
(or a new lower-level package) so both packages can consume it without an import
cycle. The engine's `ModuleMatch` currently lives in `internal/cli`; reconcile the
shared result type accordingly.

## References

- `04-unified-module-resolution.md` (git history): established the engine and the
  `matchSource` seam, and recorded the deliberate decision to leave `search` (and,
  by omission, `config get` / `install` / `update`) out of scope. This project
  completes that tail.
- `docs/module-resolution.md`: the match-rule specification the migrated surfaces
  must satisfy.
- `~/.agents/context/levenshtein`: vendored copy source for issue #10's suggester
  (not used by this project directly; noted for sequencing).

## Requirements

1. A single candidate-gathering primitive in `internal/modules` (or a new
   lower-level package) that enumerates the candidate set for a given scope and a
   caller-selected set of sources (installed-only or installed-and-registry),
   taking its installed sources as a caller-supplied list of (config value, scope)
   pairs — so the merged-config resolver passes one source and `search` passes its
   separate local and global configs as two — and returning the candidates tagged
   by source and config scope (installed-local, installed-global, registry),
   un-deduplicated, with each candidate's index entry
   (description, tags, origin) retained — registry candidates from the index,
   installed candidates extracted from their CUE — and no matching applied, so
   `search` can match and display installed entries by description and tag. The
   installed-over-registry merge and `category:name`
   de-duplication is a separate resolution-only step, not part of the primitive. A
   reusable literal name-only matcher (exact / substring / prefix) is provided
   alongside the primitive. The candidate result type is shared across
   `internal/cli` and `internal/modules`.
2. `start config get` and `start config edit` enumerate installed candidates
   through the shared primitive and apply the literal name-only matcher across all
   categories, returning every literal-name match (exact tier, else substring
   fallback) rather than reducing to one, so `config get --json` stays an array and
   the TTY menu still triggers on a genuine multi-match (`config edit` keeps its
   single-selection menu over the same match set). The exact tier short-circuits the
   substring fallback, so a bare exact name that is a prefix of installed siblings
   now resolves to that one name on both surfaces (a deliberate change from the regex
   sibling-fallthrough; siblings remain reachable via a non-exact substring query).
   `resolveAllMatchingNames`, `searchAllConfigCategories`, `scoreAndSortNames`, and
   the regex path are removed; `resolveInstalledName` is reduced to a direct
   exact-name lookup (or inlined) for the JSON builders, retaining no regex
   fallback.
3. `start install` builds its candidate list through the shared primitive,
   applying search's regex/tag matcher over the enumerated set so it keeps
   matching names, descriptions, and tags, while preserving multi-select,
   registry-first, and auto-install behaviour.
4. `start update` keeps its `collectInstalledModules` inventory (which carries the
   version/config-file/origin metadata it needs) and replaces only the name half of
   its inline `strings.Contains` filter with the shared name-matcher, leaving the
   category half as its current substring match, so both a full category query
   (`start update roles`) and a partial one (`start update task`) still select every
   installed module in that category, preserving its installed-filter behaviour. Its
   enumeration is not routed through the candidate primitive.
5. `start search` enumerates candidates through the shared primitive and applies
   its own regex and tag matcher over the full unfiltered set, with no
   change to its output. The shared candidate type is the internal gathering
   representation only: `search` projects the matched candidates back to
   `modules.SearchResult` (its existing `searchSection.Results` element type) for
   display and `--json`, so the documented `search --json` shape — an array of
   `{label, path?, results: [{category, name, entry}]}` sections, guarded by the
   schema drift test — is preserved exactly. The candidate type's extra source/scope
   fields are used to assign candidates to the local/global/registry sections and to
   drive the installed★ marker, not serialised into the output.
6. Each migrated surface produces its empty-result outcome at a single not-found
   point suitable for a later suggestion hook.
7. The duplicate matchers retired by the migration are deleted, not left dormant.
8. `docs/module-resolution.md` and `AGENTS.md` describe the unified gathering and
   correct the literal/no-regex claim for every migrated surface.

## Constraints

- Pure Go, no cgo.
- No import cycle: the shared primitive lives where both `internal/cli` and
  `internal/modules` can use it.
- `start search` user-visible behaviour (regex, tags, description matching) is
  preserved exactly.
- `start install` keeps matching names, descriptions, and tags (search's matcher)
  in addition to its multi-select / registry-first / auto-install semantics;
  `start update` installed-filter semantics are preserved.
- Migrated resolution-style surfaces match `docs/module-resolution.md`: literal,
  case-insensitive, name-only.
- Go 1.25.

## Implementation Plan

1. Define the shared candidate result type and the neutral enumeration primitive in
   `internal/modules` (or a new lower package). The primitive enumerates the
   caller-selected sources — installed sources passed as a list of (config value,
   scope) pairs, so the merged-config resolver feeds one and `search` feeds its
   local and global configs as two — and returns the full candidate set tagged by
   source and config scope, un-deduplicated and unfiltered, with each candidate's
   index entry
   (description, tags, origin) retained — populated from the registry index for
   registry candidates and via `extractIndexEntryFromCUE` for installed ones, so
   `search`'s installed description/tag matching and verbose display survive.
   Keep the installed-over-registry merge and `category:name` de-duplication
   (`mergeMatches`) as a separate resolution-only helper, not part of the
   primitive; provide the engine's mode-based literal name-only matching as a
   separate reusable matcher over the gathered set. Reconcile `ModuleMatch` so
   `internal/cli` and `internal/modules` share one representation without an import
   cycle.
2. Re-point the engine's `collectInstalledFrom` / `collectRegistry` at the shared
   primitive plus the literal name matcher and the resolution-only merge step (or
   make them thin wrappers), proving the trio reproduces the engine's current
   behaviour — including the registry-skip when a lone installed exact resolves —
   against the existing resolution test suite.
3. Migrate `config get` and `config edit` onto the shared enumeration primitive
   (installed-only) plus the literal name matcher, returning the full match set
   (exact tier, else substring fallback): `config get --json` emits the array, a
   genuine multi-match menus to pick one on a TTY and errors on ambiguity without
   one, and `config edit` selects from the same set. Delete `resolveAllMatchingNames`
   and `searchAllConfigCategories`; reduce `resolveInstalledName` to an exact-name
   lookup for the `config list` / `config get --json` builders and delete
   `scoreAndSortNames`. Assert the `config get --json` array shape
   (`TestConfigGetJSONShape`) holds and the multi-match menu/ambiguity behaviour is
   intact on both surfaces; update any test that asserted the regex
   sibling-fallthrough (a bare exact name that is a prefix of installed siblings now
   resolves to that one name).
4. Rework `install` to enumerate candidates through the shared primitive and apply
   search's regex/tag matcher on top, keeping `promptModuleSelection` and
   the registry-first / auto-install flow intact. Assert install still surfaces
   description-only and tag-only matches as it does today.
5. Rework `update` to replace only the name half of its inline `strings.Contains`
   filter with the shared name-matcher, keeping the category half as the existing
   substring match (the matcher is shared-name-match OR category-substring) over its
   existing `collectInstalledModules` inventory; leave that inventory and its
   version/config-file metadata untouched. Assert both `start update <cat>` and a
   partial category query still select every installed module in that category.
6. Re-base `search`'s candidate gathering on the shared primitive, enumerating
   installed-and-registry candidates with their source/scope tags so its local,
   global, and registry sections (and the installed★ marker on registry rows) come
   straight from the gathered set, and layering its regex / tag policy on
   top. Do not apply the resolution merge/de-dup here. Keep `modules.SearchResult`
   as the section element type that `search` serialises: convert the matched
   candidates to it rather than serialising the candidate type, so the `search
   --json` shape is unchanged. Assert identical search output before and after,
   including the `--json` shape under the existing drift guard.
7. Ensure each migrated surface routes its empty result through one not-found
   point. Do not add suggestion output here; just make the seam clean.
8. Remove dead matchers and update `docs/module-resolution.md` and `AGENTS.md`.

## Implementation Guidance

- The unifying axis is the gathering primitive, not a single god-function that
  resolves every surface. Resist collapsing the surfaces' differing policies
  (single-resolution vs multi-select vs list) behind one signature with boolean
  switches; keep those policies in their own callers on top of the shared
  primitive.
- `config get` is the strongest first migration: it is the read-only sibling of
  `config remove`, which already runs on the engine, so the enumeration and
  literal name matcher exist. It differs in cardinality — `config get` returns the
  full match set where `config remove` reduces to one — so it reuses the shared
  enumeration and matcher but not the resolve-to-one reducer.
- Search's regex/tag matching is a deliberate feature, not drift. Preserve it as a
  layer over the shared candidates rather than pushing it down into the primitive.

## Acceptance Criteria

- `config get`, `config edit`, `install`, and `search` all enumerate module
  candidates through the single shared primitive; none retains its own
  installed-plus-registry enumeration scan. `update` keeps its
  `collectInstalledModules` inventory (its metadata is required) but shares the
  name-matcher. Matching policy stays per-surface (literal name-only for `config get`
  and `config edit`, the shared regex/tag matcher for `search` and `install`,
  name-or-category for `update`).
- `resolveAllMatchingNames`, `searchAllConfigCategories`, `scoreAndSortNames`, and
  the other matchers retired by the migration no longer exist in the tree;
  `resolveInstalledName` survives only as a regex-free exact-name lookup (or is
  inlined).
- `start search` produces identical results (regex, tag, and description matches)
  to before the change.
- `start config get` and `start config edit` match by the literal
  exact-then-fallback rule and no longer use regex/scoring matching. `config get`
  returns the full match set as a `--json` array and menus a genuine multi-match on
  a TTY, with the documented ambiguity behaviour; `config edit` selects from the
  same set. A bare exact name that is a prefix of installed siblings resolves to
  that one name on both (the documented deliberate change from the regex
  sibling-fallthrough).
- Each migrated surface exposes a single not-found point for a later suggestion
  hook.
- `docs/module-resolution.md` and `AGENTS.md` describe the unified gathering and
  no longer claim literal/no-regex matching for any surface that does not have it.
