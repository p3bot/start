# Project: Skills Lifecycle and Doctor

## Goal

Complete the skills feature in the start CLI with the lifecycle and health surfaces:
listing installed skills, updating them to newer published versions, and doctor checks
that reconcile the manifest against disk and validate SKILL.md frontmatter.

## Scope

In scope:

- `start list` including skills, reconciling the manifest with an on-disk scan.
- `start update` for skills: re-materialise newer registry versions.
- doctor reconciliation of the skills manifest against the materialised bundles.
- doctor validation of SKILL.md frontmatter for managed skills.

Out of scope:

- Install, uninstall, get, describe, and the category plumbing. These are delivered by the
  skills category and installation project.
- Any library or schema changes.

## Dependencies

- The skills category and installation project must be in place: the `skills.cue` manifest,
  the materialisation path, the target-resolution helper, and the category plumbing all
  exist and are the basis for this work.

## Current State

- After the installation project, installed skills are recorded in `skills.cue` (global at
  `~/.config/start/`, local at `./.start/`) as per-leaf entries with origin, version, and
  the list of target directories. Bundles are materialised at those directories.
- `internal/cli/list.go` lists installed modules per category from config.
  `internal/cli/update.go` updates installed modules; `internal/modules/install.go` holds
  version helpers (`VersionFromOrigin`, `ModuleFromOrigin`, `GetInstalledOrigin`) and
  `UpdateModuleInConfig`. Update for the four categories re-fetches and rewrites the config
  entry.
- `internal/doctor/` holds the doctor checks. `internal/doctor/schema.go` validates config
  entries against schemas for the four categories and settings, tolerating extra fields and
  ignoring unknown top-level keys.
- A target-resolution helper from the installation project returns an agent's skills
  directories and a skill's destination directories.
- The full design, including the doctor behaviour, is in
  `/home/grant/Projects/start-cli/design-skills.md`.

## References

- `/home/grant/Projects/start-cli/design-skills.md` — the authoritative skills design,
  especially the doctor section.
- https://agentskills.io/specification — the SKILL.md frontmatter rules to validate.

## Requirements

1. `start list` and `start list skills` include installed skills. The listing is the
   manifest reconciled with an on-disk scan of the recorded target directories, so a skill
   present in the manifest but missing on disk, and a skill on disk in a known agent skills
   directory but absent from the manifest, are both distinguishable in the output.
2. `start update` updates skills to newer published versions: for each managed skill,
   resolve the latest registry version, and if newer, re-materialise the bundle into its
   recorded target directories and update the manifest version. Updating skills follows the
   same selection conventions as updating other categories (all, or a named target).
3. doctor reconciles the manifest against disk: for each manifest skill, confirm each
   recorded target directory exists and contains a SKILL.md. A missing recorded target is
   reported as drift with an offer to re-materialise it. doctor must not push a skill into
   agents it was not installed into. A skill directory present in a known agent skills
   directory but absent from the manifest is reported as unmanaged.
4. doctor validates SKILL.md frontmatter for managed skills against the Agent Skills rules:
   name length and character set, no leading, trailing, or consecutive hyphens, name equal
   to the parent directory name, and description present and within length. doctor does not
   cross-check the SKILL.md description against the skill.cue description.

## Constraints

- Go 1.25, cobra, cuelang.org/go.
- Manifest reads and writes use the official `cuelang.org/go` packages, consistent with the
  installation project.
- Re-materialisation during update reuses the installation project's materialisation and
  target-resolution helpers rather than duplicating bundle-copy logic.
- doctor frontmatter validation mirrors the checks the agentskills skills-ref validator
  performs.

## Implementation Plan

1. Extend the list surface to read the manifest and scan the recorded target directories,
   marking manifest-only and disk-only states distinctly.
2. Extend the update surface to resolve latest versions for managed skills, re-materialise
   when newer, and rewrite the manifest version.
3. Add a doctor reconciliation check that walks the manifest, verifies each target, offers
   re-materialisation for missing targets, and flags unmanaged on-disk skills.
4. Add a doctor frontmatter validation check that parses each managed skill's SKILL.md and
   applies the Agent Skills rules.
5. Test: list with healthy, manifest-only, and disk-only skills; update when newer and when
   current; doctor drift detection and re-materialisation offer; doctor unmanaged-skill
   detection; doctor frontmatter validation passing and failing cases (bad name, name not
   matching directory, missing or over-length description).

## Acceptance Criteria

- `start list skills` shows installed skills and visibly distinguishes a manifest entry
  whose bundle is missing on disk and an on-disk skill absent from the manifest.
- `start update` re-materialises a skill when a newer version is published and updates the
  manifest version; it makes no change when the skill is current.
- doctor reports a manifest skill with a missing target directory as drift and offers to
  re-materialise it, and never adds a skill to an untargeted agent.
- doctor reports an on-disk skill in a known agent skills directory that is absent from the
  manifest as unmanaged.
- doctor flags a SKILL.md whose name violates the rules, whose name does not match its
  directory, or whose description is missing or over length, and passes a valid SKILL.md.
