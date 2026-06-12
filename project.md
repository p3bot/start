# Project: Split Index Access Into Cache-Gated Display and Live Install Paths

## Goal

Establish one clear rule for how commands read the library index from the CUE Central Registry: read-only display commands consult the local 24-hour cache and run offline when the index is unchanged; every command that installs a module or checks for newer versions resolves live against the registry so it never acts on a stale view. Today the line is inconsistent — display commands hit the registry on every invocation while the auto-install resolution path can act on a stale cache and miss a freshly published module.

## Scope

In scope:

- Cache-gate the read-only display commands: `start list`, `start search`, and `start library` (default).
- A new `--refresh` flag on `start library` only, honoured across all of its output modes (default, `--json`, `--export`), to force a live index pull.
- Make the resolution / auto-install path live when, and only when, the query has no installed match. `internal/cli/resolve.go` `ensureIndex` currently short-circuits on a fresh cache for every caller; instead, force a live resolve when resolution found no installed module to satisfy the query, so a name about to be auto-installed (`start`, `start task`, `--role`, `--context`, `--agent`, and the cross-category `start get` / `start describe`) is resolved against the latest index, consistent with `start install`. A query satisfied by an installed module stays cache-gated.
- A new cache-gating helper for the display commands.

Out of scope:

- `start install`, `start update`, `start doctor validate`. All already resolve live; unchanged.
- The shared `fetchIndex` helper in `internal/cli/modules_shared.go`. Both its callers (`install`, `update`) resolve live, so it is unchanged.
- `start doctor` index-version resolution, which is already cache-first display.
- First-run auto-setup (`internal/orchestration/autosetup.go`). Nothing is cached on first run, so it must fetch.
- The CUE module cache (content storage). This project changes only how the index version is resolved, not how module content is cached.
- The `registry.Client` interface and the `cache` package public API.

## Current State

Every index access flows through `client.FetchIndex(indexPath)`, which performs two registry operations:

1. `ResolveLatestVersion` — metadata only (an OCI tag-list via `ModuleVersions`). It short-circuits with no network call when handed an already-canonical version (e.g. `...@v1.14.0`), but makes the metadata call when handed a bare major (e.g. `...@v1`).
2. `Fetch` — content. Served from CUE's local module cache when that version was pulled before; network only for a version not yet downloaded.

The `start` index cache lives at `cache.cue` in the XDG cache dir (`internal/cache/cache.go`). It records `index_version` (canonical) and `index_updated`. `IndexCache.IsFresh` returns true within `DefaultMaxAge` (24h). Passing `FetchIndex` a cached canonical version makes the call fully offline when unchanged; passing a bare major forces the metadata call.

Two distinct behaviours exist today, and both are being corrected:

- `internal/cli/resolve.go` `ensureIndex` (lines ~266-321) reads the cache, checks `IsFresh`, guards that the cached version belongs to the same module as the configured index path (`modules.ModuleFromOrigin`), and on a fresh hit passes the canonical version to `FetchIndex` for an offline resolve; on a miss it fetches and writes the cache. This cache-gating is correct except in one case: when resolution found no installed match and is about to auto-install, a fresh cache can hide a module published in the last 24h. The cache logic here is also the model for the new display helper.
- `start list` (`list.go:148` -> `checkForUpdates` -> `list.go:275`), `start search` (`search.go:167`), and `start library` (`library.go:75,85`) bypass the cache and call the registry on every invocation, firing a metadata request each time. These should become cache-gated. Note `library` does not use `FetchIndex`; it calls `ResolveLatestVersion` then `Fetch` directly and reads `result.SourceDir` (library.go:75,85,101,103,105), so routing it through the helper means feeding the helper's resolved version into a `Fetch` for `SourceDir`, not swapping in `FetchIndex`.

The governing rule is "resolve live when the query has no installed match; otherwise cache-gated," and it must be applied at every point `ensureIndex` is consulted — both tiers, every surface. Two structural details shape how that lands:

- The category-specific exact tier can skip the index entirely: a single installed exact match returns before `ensureIndex` is called (`resolveExact`, `engine.go:150`), so that case makes no registry call at all. This is an optimisation of the rule, not an exception to it.
- Every other path reaches `ensureIndex` and must apply the rule explicitly: the category-specific fallback (substring) tier (`resolveFallback`, `engine.go:196`) still consults the index to merge registry matches even when it already has an installed substring match, and the cross-category surfaces (`get`, `describe`) never short-circuit because they consult the index even for an installed name to detect a same-name twin in another category. In all of these, an installed match means cache-gated; no installed match means live.
- `ensureIndex` is memoized via `didFetch` and the exact tier reaches it first (`engine.go:156`, taken whenever no installed exact match short-circuits), before the fallback substring-installed set has been computed (`resolveFallback`, `engine.go:190`). The live-vs-cached choice is therefore fixed on that first call and held for the resolver's life — including across the multi-stage resolvers (`task.go:93`, `start.go:374`) that reuse one resolver across agent → role → context → task. The decision cannot be read off whatever installed set is in scope at each call site: at that first call only the exact-installed set is known, and it is empty precisely for a query a fallback substring match would satisfy from disk. It must instead be computed once from the combined exact + fallback installed match (see Requirement 4).

Keying liveness on the combined installed-match state — not on "every `ensureIndex` call", and not on the partial installed set visible at a single call site — forces live exactly when an auto-install may occur (any surface, no installed match) while keeping `get`/`describe` and fallback-matched queries that are already satisfied on disk cache-gated rather than hitting the registry live every time.

The auto-installed module's version is already resolved live independently of the index: `autoInstall` -> `modules.InstallModule` calls `ResolveLatestVersion` on the module path (`modules/install.go:36`). A stale index therefore never installs an outdated version; its only failure mode is making a newly-published module undiscoverable. The liveness fix targets exactly that discoverability gap.

`fetchIndex` in `modules_shared.go` is shared by `install` and `update`. It calls `FetchIndex(resolveLibraryIndexPath())` with no cache read, then writes the cache best-effort. Both callers resolve live, so this helper is unchanged.

Desired end state per command:

| Command | Reads 24h cache? | Rationale |
| --- | --- | --- |
| resolution, query has no installed match (`task`, `--role`, `--context`, `--agent`, `get`, `describe`) | no (change to live) | About to auto-install; must see the latest, like `install`. |
| resolution, query satisfied by an installed module | yes (cache-gated) | Category-specific exact match skips the index entirely; fallback-matched and `get`/`describe` queries reach `ensureIndex` but read the cache — no freshness need when already on disk. |
| `start list` | yes (change) | Read-only update-available display; 24h staleness is fine. |
| `start search` | yes (change) | Discovery over the index; same tolerance. |
| `start library` | yes (change), unless `--refresh` | Displays rarely-changing library data. |
| `start install` | no (already live) | Must install the latest; a stale index could hide a new module or version. |
| `start update` | no (already live) | Its job is to detect newer versions; must see live metadata. |
| `start doctor validate` | no (already live) | Maintainer consistency check; wants live truth. |
| `start doctor` (index ver) | yes (already cache-first) | Display only. |
| auto-setup | no (first run) | Nothing cached on first run. |

The line that results: live only when a resolve may auto-install a not-yet-installed module; cache-gated for everything else — pure display and any resolve already satisfied on disk.

## Requirements

1. A cache-gating helper that resolves the effective index version for the display commands. Its shared job is the version decision, not the output shape: given an effective index path and a client, it yields the canonical version to use — the fresh cached version when one is available, otherwise the latest resolved from the registry, writing the cache best-effort on a refresh. It must honour `IsFresh` against `DefaultMaxAge` and verify the cached version belongs to the same module as the effective index path (`modules.ModuleFromOrigin`) before reuse. Callers fetch from that version what they each need: `list` and `search` consume the parsed `*Index`; `library` needs the fetched module's source directory (`SourceDir`) for `--export` (raw CUE), `--json`, and the default view, all of which read from disk. The helper must serve both consumers — do not pin it to returning only a parsed `*Index`. Model the cache logic on what is currently inline in `ensureIndex`.

2. `start list` and `start search` resolve the index through the cache-gating helper. With a fresh cache they make no registry metadata request.

3. `start library` resolves through the cache-gating helper by default, and gains a `--refresh` flag that forces a live index pull bypassing the cache read. The flag exists only on `library` and is honoured across all output modes: default, `--json`, and `--export`. After a `--refresh` pull, the cache is updated so subsequent commands see the freshly pulled version.

4. Resolution resolves the index live when the query has no installed match, and cache-gated otherwise. This rule applies at every point `ensureIndex` is consulted, both tiers and every surface. When resolution finds no installed module to satisfy the query — on any surface, including the cross-category `get`/`describe` — it must resolve the index live so a module published within the last 24h is discoverable and auto-installed. When the query is satisfied by an installed module, resolution stays cache-gated: the category-specific exact tier skips `ensureIndex` entirely, while the category-specific fallback tier and `get`/`describe` reach `ensureIndex` but must read the cache rather than pull live.

   Two structural facts of the current resolver shape how this decision must be taken, and the obvious "decide from the installed set at the call site" reading does not satisfy the rule:

   - `ensureIndex` is memoized via `didFetch`: it does real work only on its first call per resolver, and later calls return the already-fetched index. The first call fixes the live-vs-cached choice for the whole resolver.
   - The exact tier calls `ensureIndex` first (`engine.go:156`) whenever there is no installed exact match — before the fallback substring-installed set is computed (`resolveFallback`, `engine.go:190`). At that first, decision-fixing call only the exact-installed set is known, and it is empty precisely for a query that a fallback substring match would satisfy from disk.

   The resolver must therefore decide liveness from the complete installed picture, not from whatever installed set is in scope at the lowest call site:

   - Before the exact tier runs, compute whether any installed module satisfies the query under both the exact and the fallback mode (both are network-free `collectInstalled` lookups) and thread that single decision into `ensureIndex` as a `wantLive` signal. A query satisfied by any installed match — exact or substring — is cache-gated; only a query no installed module satisfies forces live.
   - `ensureIndex` must record whether the index it holds was fetched live or cache-gated, so that across the multi-stage resolvers (`task.go:93`, `start.go:374`, which reuse one resolver across agent → role → context → task) a later surface arriving with `wantLive` upgrades a previously cache-gated fetch by re-resolving live, rather than inheriting a stale cache-gated index.

   `ensureIndex` must keep its graceful degradation (return a nil index with the failure recorded in `indexErr` when the registry is unavailable) and its best-effort cache write on any live resolve.

5. `start install`, `start update`, and `start doctor validate` continue to resolve the index live on every invocation, unchanged. `fetchIndex` is unchanged.

6. Target-module content fetches are unchanged. `install` and `update` still fetch the requested module's content after the index lookup; this project changes only index version resolution.

7. Cache-miss and different-module behaviour is preserved for the display path. A missing or stale (>24h) cache, or a cache whose version belongs to a different module than the current effective index path, triggers a single fetch and a cache write.

## Constraints

- Go, matching the repository's current toolchain. Build with `go build ./...`.
- Do not alter the `registry.Client` interface or the `cache` package's public API unless strictly required; prefer reusing `cache.ReadIndex`, `cache.WriteIndex`, `IndexCache.IsFresh`, and `cache.DefaultMaxAge`.
- Preserve graceful degradation. Registry unavailability must remain non-fatal where it currently is (the resolver returns a nil index with the failure recorded; `search` records `registryErr` and degrades). Cache writes remain best-effort and debug-logged only.
- Preserve the module-match guard (`modules.ModuleFromOrigin`) so a changed `library_index` setting invalidates cache reuse.
- Keep user-facing output semantics honest: a progress or debug line that signals a network resolve must reflect whether one actually happened. `ensureIndex`'s "Fetching registry index..." line should show when it resolves live (a no-installed-match query) and not when it serves a query from the cache.
- Follow the registry-client seam guidance in AGENTS.md: command paths obtain the client through `getProvider(cmd)()`; the resolver and auto-setup remain on their direct `registry.NewClient()` paths.

## Implementation Plan

1. Add the cache-gating helper described in requirement 1, reusing the cache read / `IsFresh` / module-match-guard / canonical-version logic currently inline in `ensureIndex`.

2. Make `ensureIndex` resolve live only when no installed module satisfies the query. In `resolve()`, before the exact tier, compute the combined installed-match state across both the exact and the fallback mode (network-free `collectInstalled` lookups) and pass it as a single `wantLive` signal into `ensureIndex`; do not key the decision on the exact-tier installed set alone, which is empty for a fallback substring-installed query and would force a spurious live resolve. `wantLive` (no installed match) takes the effective bare-major path to `FetchIndex`; an installed match keeps the existing cache-gated path. Extend `ensureIndex`'s memoization to record whether the held index was fetched live or cache-gated, so a later surface in a multi-stage resolver that arrives with `wantLive` re-resolves live instead of returning the stale cache-gated index. Keep the graceful fallback, the conditional progress/debug line, and the best-effort cache write on a live resolve.

3. Route `start list` (`checkForUpdates`) and `start search` through the cache-gating helper. For `start library`, feed the helper's resolved version into a `Fetch` for `result.SourceDir` rather than swapping in `FetchIndex`. Leave `fetchIndex` (and therefore `install` and `update`) unchanged.

4. Add the `--refresh` flag to the `library` command, default false. When set, resolve live and bypass the cache read; when unset, resolve through the helper. Apply this before the output-mode branch so default, `--json`, and `--export` all honour it. Update the cache after either pull. Document the flag in the command's long help.

5. Update AGENTS.md where it describes index/cache and resolution behaviour and the command listings, so the cache-gated display commands, the now-live resolution path, and the new `library --refresh` flag are documented.

6. Add or extend tests covering: a fresh-cache invocation of `list`/`search`/`library` performing no metadata resolve; a stale/missing cache triggering one fetch and a cache write on a display command; `library --refresh` forcing a live resolve regardless of cache freshness; resolution of a no-installed-match query resolving live even with a fresh cache; resolution of an installed-match query staying cache-gated (category-specific exact makes no call; a fallback substring-installed query — e.g. `--role expert` resolving to an installed `go-expert` — and `get`/`describe` of an installed module serve from a fresh cache with no metadata resolve); a multi-stage resolve (`start --context <substring-installed> task <uninstalled>`) where an earlier cache-gated surface precedes a later no-installed-match auto-install candidate still resolves that candidate live. Use the existing offline `--json` test harness (`setupStartTestConfigWithRegistry`, `captureJSON`) and stub client where applicable.

## Acceptance Criteria

- With a fresh (<24h) cache whose version matches the configured index module, `start list`, `start search`, and `start library` (without `--refresh`) complete without a registry metadata resolve, observable via `--debug` output or stubbed-client call counts.
- `start library --refresh` performs a live index resolve even when the cache is fresh, and updates the cache afterward, across default, `--json`, and `--export` modes.
- Resolving a name with no installed match (e.g. `start task foo`, `start --role foo`, `start describe foo` for an uninstalled `foo`) performs a live index resolve regardless of cache freshness, so a module published within the cache window is found and auto-installed — matching `start install`.
- Resolving a name satisfied by an installed module does not pull the index live: a category-specific exact match makes no registry call at all, and any other installed-match resolution (category-specific fallback, or `get`/`describe` of an installed module) serves from a fresh cache with no metadata resolve.
- `start install`, `start update`, and `start doctor validate` perform a live index resolve on every invocation, unchanged from current behaviour.
- A missing or stale cache causes exactly one index fetch followed by a cache write on the affected display command.
- The `--refresh` flag is present on `start library` and absent from all other commands.
- AGENTS.md reflects the cache-gated display commands, the live resolution path, and the new flag.
