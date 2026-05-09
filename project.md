# Library and module terminology rename

## Goal

Rename the legacy asset terminology in the start CLI to the library and module terminology that matches the post-migration model. The unit becomes a module; the collection becomes the library. After this project, start does not use or expose the word asset anywhere.

## Scope

In scope:

- Go package, type, function, constant, file, and test renames in `start/`
- CLI command group rename from `assets` to `modules`
- Documentation and script updates within `start/`
- Workspace-level `../AGENTS.md` updated to remove all `asset` references it inherits from `start/`
- Hard cutover with no backward-compatibility shims

Out of scope:

- Command tree flattening (separate roadmap project)
- Library schema or library content changes
- Releases, tags, or homebrew-tap edits

## Current State

- `start/` uses `asset(s)` for both the unit and the collection
- The CUE settings schema at `library/schemas/settings.cue` already uses `library_index`; the start Go code reads this key correctly via a function still named `resolveAssetsIndexPath`
- The `start assets …` command group is fully implemented with subcommands `browse`, `index`, `search`, `add`, `list`, `info`, `update`, `validate`
- Top-level cobra command lives in `internal/cli/assets.go` and exposes the `assets` `Use` value with an `asset` alias
- README.md contains a sentence noting that a future project will revisit the asset/module terminology and the command tree — this project closes the terminology half
- `start/` has no release tag; this work is pre-release

The full surface (from `rg -i -. 'asset'`) covers:

- Package directory `internal/assets/` and its files `install.go`, `search.go`, plus `_test.go` siblings
- CLI files `internal/cli/assets*.go` (10 files including `_test.go` siblings)
- `test/integration/assets_test.go`
- Identifiers including `InstalledAsset`, `AssetMatch`, `InstallAsset`, `ExtractAssetContent`, `UpdateAssetInConfig`, `AssetExists`, `formatAssetStruct`, `writeAssetToConfig`, `findAssetField`, `mergeAssetMatches`, `installAsset`, `installSingleAsset`, `promptAssetSelection`, `printAssetInfo`, `promptAssetInfoSelection`, `collectInstalledAssets`, `printInstalledAssets`, `addAssets*Command`, `runAssets*`, `errNoAssets`, `maxAssetResults`, `defaultAssetsBranch`, `DefaultAssetRepoURL`, `resolveAssetsIndexPath` (the prerequisite project removes `assetTypeToConfigFile` and its tests entirely)
- Test names beginning with `TestAsset…` and `TestAssets…`
- README.md, AGENTS.md, `internal/cli/help/agents.md`, `scripts/README.md`
- `scripts/validate-assets.sh` (deleted per requirement 9; see issue 1 for context)
- `scripts/test-supporting-commands.sh` and `scripts/manual-test` (`start assets …` invocations covered by the zero-match sweep)

## References

- Workspace roadmap at `../roadmap.md` item 2
- Workspace `AGENTS.md`
- `start/AGENTS.md`
- `library/AGENTS.md`
- `library/schemas/settings.cue` (canonical settings key `library_index`)
- README.md note acknowledging the upcoming terminology revisit

## Requirements

1. Top-level command group renamed from `assets` to `modules`, with `module` accepted as a singular alias; `asset` and `assets` are not recognised.
2. Subcommand names (`browse`, `index`, `search`, `add`, `list`, `info`, `update`, `validate`) preserved.
3. Go package `internal/assets/` renamed to `internal/modules/` with all imports updated.
4. CLI files `internal/cli/assets*.go` renamed to `internal/cli/modules*.go`, with their `_test.go` siblings renamed to match.
5. Integration test `test/integration/assets_test.go` renamed to `modules_test.go`.
6. Go identifiers renamed using the unit-vs-collection distinction:
   - Names referring to a single unit use `Module` / `module`
   - Names referring to the collection or the registry-side library use `Library` / `library`
   - For any identifier whose source word maps to category rather than module or library (typically a parameter or symbol whose argument is a category enum: roles, agents, contexts, tasks), use a `Category…` form
7. `SearchResult` is retained unchanged; the package qualifier (`modules.SearchResult`) carries the meaning.
8. All test names containing `Asset` are renamed; all `cmd.SetArgs` arguments using `"assets"` are updated to `"modules"`.
9. `scripts/validate-assets.sh` deleted along with its entry in `scripts/README.md`. The script is broken dead code superseded by `library/scripts/validate-{index,module}` and is not preserved under a renamed filename.
10. README.md, AGENTS.md, and `internal/cli/help/agents.md` updated so no `asset` wording remains; the README sentence acknowledging the upcoming terminology project is removed; the command-tree note may stay or be trimmed to reference only the upcoming flattening work, provided it contains no `asset` text.
11. Workspace-level `../AGENTS.md` updated so no `asset` wording remains: the `internal/` package list, the helper-scripts line (with `validate-assets.sh` removed entirely per requirement 9), the library description, and the cross-repo workflow note are all rewritten in module/library terminology.
12. The CUE settings schema key `library_index` is unchanged.
13. Final verification: `rg -i -. 'asset'` from `start/` and from `../AGENTS.md` returns zero matches.

## Implementation Plan

1. Audit
   Run `rg -i -. 'asset' .` from `start/` and use the result as the working list. Cross-check against the surfaces in Current State to confirm completeness.

2. Package and file moves
   Move `internal/assets/` to `internal/modules/`, updating package declarations and imports. Rename `internal/cli/assets*.go` and their tests to `internal/cli/modules*.go`. Rename `test/integration/assets_test.go` to `modules_test.go`.

3. Identifier renames
   Apply the unit/collection mapping to all identifiers, preserving `SearchResult`. The constants `DefaultAssetRepoURL` and `defaultAssetsBranch` describe the library repository and become `Library`-named. The function `resolveAssetsIndexPath` becomes `resolveLibraryIndexPath` (it already reads `library_index`). For any remaining identifier whose source word maps to category rather than module or library, use a `Category…` form. The prerequisite project (`function-refactor.md`) has already removed `assetTypeToConfigFile` outright, so no special-case rename is needed for it.

4. CLI command group update
   In the renamed `internal/cli/modules.go`, set cobra `Use` to `modules` with a `module` alias. Update `Short`, `Long`, and any inline help to use module/library terminology. Drop all `asset` and `assets` aliases.

5. Test updates
   Rename `TestAsset…` functions to `TestModule…`. Update `cmd.SetArgs([]string{"assets", …})` to `"modules"`. Test bodies that assert on help or output text are updated to match the new wording.

6. Script removal
   Delete `scripts/validate-assets.sh` and remove its entry from `scripts/README.md`. Do not introduce a renamed replacement — validation of the library lives in `library/scripts/`.

7. Documentation update
   Edit README.md, AGENTS.md, `internal/cli/help/agents.md`, and `../AGENTS.md` (the workspace-level document). Replace `start assets …` with `start modules …`. Replace narrative mentions of asset with module (for unit references) or library (for collection references). Remove the README sentence acknowledging the upcoming asset/module terminology project. In `../AGENTS.md` specifically: update the `internal/` package list (`assets` → `modules`), drop `validate-assets.sh` from the helper-scripts line, and rewrite the library description and cross-repo workflow note in the new terminology.

8. Final verification
   Run `rg -i -. 'asset'` from `start/` and `rg -i 'asset' ../AGENTS.md`. Zero matches in both is the success condition. For any remaining match, decide whether the wording should become module, library, or category, and apply the change.

9. Build and test sweep
   Run `gofmt -w .`, `go fix ./...`, `golangci-lint run`, and `scripts/invoke-tests` from `start/`.

## Constraints

- The prerequisite project `function-refactor.md` must land before this project starts; it eliminates `assetTypeToConfigFile` entirely by inlining the lookup, so the rename has no function to handle for this case
- Hard cutover: no backward-compatibility aliases, deprecation shims, or dual-name handling for the asset terminology in code, tests, CLI, or docs
- Do not modify any file under `library/` or `homebrew-tap/`
- Do not modify the CUE settings schema; `library_index` is already correct
- Do not produce a release tag, push to remotes, or edit homebrew-tap

## Implementation Guidance

- The unit-versus-collection distinction is the heart of the rename. Before each `asset → ?` substitution, ask whether the sentence or identifier names one item or the whole collection. Pick `module` for the former and `library` for the latter.
- The library schema is already correct (`library_index` in `library/schemas/settings.cue`). Only the Go side and documentation are stale.
- The `modules` command group is expected to be flattened in a future project. Treat this work as a renaming pass, not a redesign — do not introduce new structure inside the group.
- `SearchResult` is a value type that survives the package rename; do not rename it.
- The prerequisite project (`function-refactor.md`) has eliminated `assetTypeToConfigFile` entirely by inlining direct lookups against `internalcue.ConfigFiles`. No function for that mapping remains by the time this rename runs. The unit/collection rule's `Category…` form still applies to any other identifier whose source word maps to a category enum (roles, agents, contexts, tasks) rather than to module or library.
- The README sentence acknowledging the upcoming terminology project is removed in full because requirement 13 will fail otherwise; the command-tree half may stay, provided no `asset` text remains.

## Acceptance Criteria

- `rg -i -. 'asset'` from `start/` returns zero matches
- `rg -i 'asset' ../AGENTS.md` returns zero matches
- `start modules` and all eight subcommands run successfully and present module/library wording in help output
- `module` and `modules` invoke the same group; `asset` and `assets` are not recognised
- README.md, AGENTS.md, `internal/cli/help/agents.md`, and `../AGENTS.md` contain no `asset` wording
- The CUE settings schema key `library_index` is unchanged

## Issues Discovered

1. Disposition of `scripts/validate-assets.sh` (decision) — Resolved: delete the script.

   The script references `${REPO_ROOT}/context/start-assets` — a path that does not exist after the migration to `start-cli/library`. Its hard-coded `MODULES=(...)` list reflects the old `start-assets` layout and no longer matches the current `library/modules/` structure. The library repository now ships its own validation scripts (`validate-index`, `validate-module`, `publish-index`, `publish-module`) under `library/scripts/`. The script is therefore dead code that does not run in any state of the post-migration tree, and the project's directive to "use the chosen terminology" left the broken-path question unanswered.

   Resolution: the script is deleted outright rather than renamed, along with its entry in `scripts/README.md`. Validation of the library is the responsibility of `library/scripts/` and is not duplicated inside `start/`. Requirement 9 and Implementation Plan step 6 are updated to reflect deletion in place of a rename.

2. `assetTypeToConfigFile` is duplicated across two packages (gap) — Resolved: eliminate the function entirely in a prerequisite project.

   The Implementation Plan and identifier list originally referred to "the function currently named `AssetTypeToConfigFile`" in the singular, with exported casing. In the codebase the function is unexported and exists as two independent copies — one at `internal/assets/install.go` (table-driven via `internalcue.ConfigFiles`) and one at `internal/cli/assets_list.go` (hard-coded switch). They produce identical outputs for all inputs, and `internal/cli/assets_list.go` already imports `internal/cue`, so both copies are redundant indirection over the centralised `ConfigFiles` map.

   Resolution: a prerequisite project at `function-refactor.md` (root of `start/`) removes both function bodies and inlines direct lookups against `internalcue.ConfigFiles` at the two call sites. Both `TestAssetTypeToConfigFile` tests are deleted with them. By the time this rename project starts, no `assetTypeToConfigFile` exists anywhere in `start/`, so the `Category…` special case is reduced to general guidance for any other category-valued identifier the audit may surface. The identifier list, Requirement 6, Implementation Plan step 3, and Implementation Guidance are updated accordingly.

3. Workspace-level `../AGENTS.md` will reference a deleted or renamed script (risk) — Resolved: bring `../AGENTS.md` into scope for this project.

   The workspace-level `AGENTS.md` at the parent directory describes `start/` and contains four `asset`-bearing references: the implementation package list under `start/`, the helper-scripts line that names `validate-assets.sh`, the library description ("Provides the asset modules"), and the cross-repo workflow note ("CLI changes affecting asset resolution"). After issue 1, `validate-assets.sh` no longer exists; after the package rename, `internal/assets/` becomes `internal/modules/`.

   Resolution: update `../AGENTS.md` as part of this project. Scope is widened by one file, Requirements gain an explicit item for it, and the Acceptance Criteria sweep is extended to that path. The proportionate cost (a four-line edit) avoids a known-stale reference shipping outside `start/`.

