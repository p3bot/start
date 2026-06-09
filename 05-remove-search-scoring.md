# Project: Remove relevance scoring from start search

## Goal

`start search` ranks results by a numeric relevance score (name 3, description 1,
tags 1, summed per term) and exposes that score as `matchScore` in `--json`
output. The score adds complexity without earning it: matching is what users
need, ranking by a hand-tuned weight is noise. Remove scoring entirely. Search
keeps matching by name, description, and tags, but results are ordered
deterministically by category then name, and no score is computed, stored, or
emitted.

## Scope

In scope:

- Remove the `MatchScore` field from `modules.SearchResult` and its
  `matchScore` JSON tag.
- Replace the score-summing matcher (`matchScorePatterns`) with a boolean
  matcher that preserves the current match semantics: a term matches when it
  appears in the name, the description, or any tag; multiple terms combine with
  AND (every term must match at least one field).
- Remove score-based ordering. `SearchIndex` orders by category order then name;
  the single-category sort (`sortResults`) orders by name. Ordering is otherwise
  unchanged and deterministic.
- Update the `--json` search shape: drop `matchScore` from the documented schema
  (`internal/cli/help/schemas.md`) and from the JSON serialization test
  (`internal/cli/modules_shared_test.go`).
- Rewrite the search test suite to assert match-or-not and alphabetical ordering
  rather than score values and score-descending ordering.

Out of scope:

- Resolution. Project 04 removed resolution's use of the search engine
  (`SearchInstalledConfig`, `SearchCategoryEntries`, `matchScorePatterns`,
  `ParseSearchTerms`, `CompileSearchTerms`) and its own `ModuleMatch.Score`
  field. This project does not touch resolution and must not reintroduce a
  coupling to it. This project depends on project 04 being complete (see
  Constraints).
- The search matching surface. Search still matches names, descriptions, and
  tags; still combines terms with AND; still supports regex term patterns; still
  enforces the three-character query floor (`ValidateSearchQuery`); still
  supports `--tag` filtering with OR semantics. Only the score and the
  score-based ranking are removed.
- The term-parsing and pattern-compiling helpers (`ParseSearchTerms`,
  `ParseSearchPatterns`, `CompileSearchTerms`). They remain in use by the search
  command and by `internal/cli/config_helpers.go`, and their behaviour does not
  change.
- The registry fetch, cache, and index mechanics. Search continues to call them
  unchanged.

## Current State

Scoring lives in `internal/modules/search.go`.

`SearchResult` (line 15) carries `MatchScore int json:"matchScore"`.

`matchScorePatterns` (line 186) sums per-term field scores: name 3, description
1, tags 1, and returns 0 when any term matches no field (the AND gate). A score
of 0 means "not a match".

`searchCategory` (line 132) has three branches — patterns-and-tags, tags-only,
patterns-only — and assigns `MatchScore` from `matchScorePatterns` or the
literal `1` in the tags-only branch. The branch comment notes they stay explicit
because `matchScorePatterns` returns 0 for nil patterns.

`SearchIndex` (line 89) sorts results by score descending, then category order,
then name. `sortResults` (line 278), used by `SearchCategoryEntries` and
`SearchInstalledConfig`, sorts by score descending, then name.

Consumers of search results:

- The search command (`internal/cli/search.go`). `printSearchSections` groups
  results by category and prints them in arrival order; it does not read
  `MatchScore`. With scoring removed, arrival order within a category must be
  alphabetical by name, which the sort change provides.
- `start install` (`internal/cli/install.go`, line 144) calls `SearchIndex`. It
  auto-installs only when there is exactly one result and otherwise presents a
  selection menu; it does not rely on score ordering to pick a "best" match.
  Removing score only changes menu ordering to alphabetical.
- `--json` output. `searchSection.Results` serializes `[]SearchResult`, so the
  `matchScore` field currently appears in `start search --json`. The shape is
  documented at `internal/cli/help/schemas.md` line 55 and asserted in
  `internal/cli/modules_shared_test.go` (the `"matchScore": 5` substring).

Tests that embody scoring and must change:

- `internal/modules/search_test.go`: `TestMatchScorePatterns` (asserts numeric
  scores) and the "results sorted by score then name" case.
- `internal/cli/search_test.go`: builds `SearchResult` inputs with `MatchScore`
  literals and asserts score-based ordering.
- `internal/cli/modules_shared_test.go`: asserts `"matchScore": 5` in serialized
  JSON.

## Requirements

1. `modules.SearchResult` has no `MatchScore` field and no `matchScore` JSON
   key. The serialized object is `{ category, name, entry }`.

2. A boolean matcher replaces `matchScorePatterns`. A term matches an entry when
   its pattern matches the name, the description, or any tag. An entry is a match
   only when every term matches (AND across terms). The matcher reports a
   boolean, not a score. The name/description/tags fields searched are unchanged.

3. `searchCategory` collects an entry as a result when it matches under the
   active mode: patterns-and-tags requires both a pattern match and an OR tag
   match; tags-only requires an OR tag match; patterns-only requires a pattern
   match. No score is assigned.

4. Result ordering is deterministic and score-free. `SearchIndex` orders by
   category order (`CategoryOrder`) then name ascending. The single-category sort
   orders by name ascending. Apply the same case handling the current sort uses.

5. The `--json` search schema documentation and the JSON serialization test
   reflect the field removal: no `matchScore` appears in the documented shape or
   in the asserted output.

6. The search test suite asserts the new contract: an entry matches or it does
   not, and results come back in category-then-name (or name) order. No test
   asserts a numeric score or score-descending ordering.

## Constraints

- This project depends on project 04 (unified module resolution) being complete.
  Project 04 removes the only resolution-side reader of `SearchResult.MatchScore`
  (`resolve.go` assigned `Score: r.MatchScore`). Removing the field before 04
  lands would break resolution. Do not start this project until 04 is merged, and
  confirm no caller outside the search command and `start install` reads
  `MatchScore`.
- Search behaviour other than ranking is preserved exactly: name/description/tag
  matching, AND-across-terms, regex term support, the three-character floor, and
  `--tag` OR filtering all remain. A query that matches today must still match,
  and a query that does not match today must still not match.
- Ordering must stay deterministic. Do not leave results in map-iteration order;
  the name-based sort is what makes output stable across runs.
- Follow the repository testing approach (AGENTS.md): real CUE validation, real
  files via `t.TempDir()`, table-driven cases, offline `--json` coverage via the
  stub client where the search path is exercised.

## Implementation Plan

1. Remove the `MatchScore` field from `SearchResult`.

2. Replace `matchScorePatterns` with a boolean matcher implementing the AND gate
   over name/description/tags. Update `searchCategory` to append results based on
   the boolean instead of a score, dropping the `MatchScore: 1` tags-only
   assignment and any score plumbing.

3. Change `SearchIndex`'s sort to category-order then name, and `sortResults` to
   name only. Remove the now-unused score comparison.

4. Update `internal/cli/help/schemas.md` to drop `matchScore` from the search
   shape, and update the JSON serialization assertions in
   `internal/cli/modules_shared_test.go`.

5. Rewrite the affected search tests in `internal/modules/search_test.go` and
   `internal/cli/search_test.go` to assert matching and alphabetical ordering.

6. Run the full pipeline (`scripts/invoke-tests`) and the tagged suites
   (`integration`, `e2e`, `registry`). Resolve every failure.

## Acceptance Criteria

1. `start search <query> --json` emits objects of shape
   `{ category, name, entry: { ... } }` with no `matchScore` key, and the
   documented schema in `start help schemas` matches.

2. A query that matches an entry only by description or only by tag still returns
   that entry; a query whose terms do not all match is still excluded. AND
   semantics across terms and OR semantics across `--tag` values are unchanged.

3. `start search` results within a category are ordered alphabetically by name,
   and `SearchIndex` results are ordered by category then name, deterministically
   across runs.

4. `grep -r MatchScore internal/` returns no matches; `matchScore` does not
   appear in any documentation, JSON output, or test.

5. `scripts/invoke-tests` passes and the `integration`, `e2e`, and `registry`
   tagged suites pass.
