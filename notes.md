# Project Notes

- Reviewing 04 obo style
- Search is for descriptions and tags, use is by name
- Discovered the search scoring feature which is not needed for search, hence 05
- The --tag feature is not needed on tasks because the new name search doesn't use it

## Section walk of 04 (obo by heading)

- Goal: "three code paths" → "multiple" (Current State enumerates four flows)
- Scope: removal bullet now names the cross-category tier logic too; --tag clause reworded as today's coupling being removed
- Current State: verified accurate against code (symbols, line ranges, all 9 named tests exist); left unchanged
- Requirements/Constraints: req 2 added three malformed-input errors with no exit-code class → classified as usageError in Constraints (path-to-agent, mismatched prefix, unknown category)
- Constraints: matching primitives said "case-insensitive" but named case-sensitive funcs → now folds operands (EqualFold for exact); guards against re-introducing divergence #5
- Plan: step 5 vs step 7 contradicted on the task-role follow-up (kept findExactInstalledName while step 7 removed it) → step 5 now routes the task role through the unified resolver, dropping the short-name pre-check
- Implementation Guidance: sound, no change
- Acceptance Criteria: added criterion 7 for input-form interpretation + the three usage errors (was uncovered); fixed garbled context-criterion wording; renumbered to 11

## Menu and Display Issue

Commands like `start lib` still show the old / on categories.

```
agents/ (16)
    aichat/interactive
```

With the category change, it should be:

```
agents: (16)
    aichat/interactive
```
