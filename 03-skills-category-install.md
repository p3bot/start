# Project: Skills Category and Installation

## Goal

Add the skills category to the start CLI and implement skill installation. A skill is an
Agent Skills bundle (a SKILL.md plus optional resources) that materialises to
agent-specific skill directories rather than composing into a prompt. This project makes
skills a first-class category and delivers install, uninstall, get, and describe for it.

## Scope

In scope:

- Fifth-category plumbing so skills are recognised alongside agents, roles, contexts, and
  tasks.
- A `skills.cue` manifest recording installed skills.
- `start install` for skills: materialise a skill bundle from the registry into the
  resolved agent skill directories and record the manifest.
- The skills branch of `start uninstall`, plugged into the category dispatch seam built
  in the uninstall project.
- `start get skills:<group>/<name>` emitting the SKILL.md body.
- `start describe skills:<group>/<name>` showing metadata, the bundle file list, and the
  resolved install targets.

Out of scope:

- `list` of installed skills, `update` of skills, and doctor reconciliation and
  frontmatter validation. These are the skills lifecycle project.
- Any library or schema authoring. The library project owns `#Skill`, the `#Agent.skills`
  attribute, the index entry, and the example skill.

## Dependencies

- The library skills project must be published first: the example skill module, the
  `#Agent.skills` attribute, and the `skills` index map must be resolvable from the
  registry.
- The uninstall project must be in place: this project adds the skills removal path at its
  category dispatch seam.

## Current State

- `internal/cli/install.go` defines install; `internal/modules/install.go` defines
  `InstallModule`, which fetches a module, extracts its fields with `formatModuleStruct`,
  and inlines them into a `<category>.<name>` struct in the category's `*.cue` file. The
  agent field allowlist in `formatModuleStruct` currently copies description, tags, uses,
  bin, command, default_model, and models. It does not copy a skills attribute.
- `client.Fetch` returns a `SourceDir` containing the fetched module tree
  (definition file, cue.mod, and any bundled files such as markdown).
- `internal/cue/keys.go` holds `KeyAgents`..`KeySettings` and the `ConfigFiles` map.
  `internal/cue/loader.go` holds `collectionKeys`. The loader is open: an unknown
  top-level key loads and merges without error, and orchestration only reads the keys it
  needs.
- `internal/registry/index.go` defines the `Index` struct with Agents, Roles, Contexts,
  and Tasks maps.
- `internal/cli/describe.go` defines `describeCategories`. `internal/cli/config_helpers.go`
  defines `normalizeCategoryArg`. `internal/cli/library.go` lists the library and
  enumerates installed names.
- `internal/cli/engine.go` is the resolver (cross-category, `category:name` qualifier,
  menu, installed versus registry source). `internal/cli/get.go` branches on category:
  agents emit the command template, UTD modules render. `internal/config/paths.go`
  resolves global (`~/.config/start/`) and local (`./.start/`) directories and scope.
- `internal/orchestration/executor.go` extracts agent fields (bin, command, models,
  default_model, description) from config. It does not currently read a skills attribute.
- The full design is in `/home/grant/Projects/p3bot/docs/skills-design.md`.

## References

- `/home/grant/Projects/p3bot/docs/skills-design.md` — the authoritative skills design.
  Read it first.
- https://agentskills.io/specification — the SKILL.md and bundle format.

## Requirements

1. Skills recognised as a category across the codebase: a `KeySkills` key, a `skills.cue`
   entry in `ConfigFiles`, `skills` added to `collectionKeys`, a `Skills` map on the
   registry `Index`, a skills entry in `describeCategories`, `skill` accepted by
   `normalizeCategoryArg`, and skills included in the `start library` listing and
   `start library skills` filter.
2. The agent install path persists the `skills` attribute into `agents.cue` going forward,
   so an agent's skill directories are known from installed config without re-fetching.
3. `start install skills:<group>/<name>` materialises the bundle:
   a. Resolve target agents: the `--agent name[,name]` list if given, else the configured
      default agent, else the sole skill-capable installed agent. Error if no default is
      set, no `--agent` is given, and more than one skill-capable agent is installed. An
      agent named in `--agent` that declares no skills attribute is an error.
   b. For each target agent read its skills path: `skills.local` under `--local`, else
      `skills.global`. If an installed agent predates the persisted attribute and has no
      skills path, fail with guidance to update that agent (which re-fetches and records
      it). Dedupe the resolved paths.
   c. The destination for each path is `<path>/<leaf>`, where leaf is the skill name. If
      the destination directory or its SKILL.md already exists, error (collision check).
   d. Copy the fetched module tree minus `cue.mod/` and `skill.cue` into each destination.
   e. Record one manifest entry keyed by leaf with the skill origin, resolved version, and
      the list of destination directories written.
4. The manifest lives at `skills.cue` in the scope's config directory (global by default,
   `./.start/` under `--local`), read and written with the official CUE AST, parser, and
   format packages, consistent with the other config writers.
5. `start uninstall skills:<name>` removes the skill: delete each destination directory
   recorded in the manifest entry for the selected scope, then remove the manifest entry.
   This is registered at the uninstall command's category dispatch seam.
6. `start get skills:<group>/<name>` writes the SKILL.md body to stdout, matching the
   pipe-clean behaviour of get for file bodies.
7. `start describe skills:<group>/<name>` shows the skill metadata, the list of files in
   the bundle, and the resolved install target directories for the current scope and agent
   selection.

## Constraints

- Go 1.25, cobra, cuelang.org/go.
- All config and manifest CUE operations use the official `cuelang.org/go` packages.
  Bundle materialisation is plain file-tree copying; it does not pass through the CUE
  config writers.
- Reuse the resolver engine, scope helpers, and config-path helpers. Do not fork
  resolution or path discovery.
- Install and uninstall are polymorphic on category. The four existing categories keep
  config-entry semantics; skills use bundle materialisation and the manifest. Do not
  special-case skills inside the existing config-merge path; branch at the category
  boundary.
- The shared module cache is the source for the fetched bundle; do not mutate it during
  materialisation.

## Implementation Plan

1. Add the category plumbing from requirement 1. Confirm the loader and doctor tolerate
   the new `skills` key (the loader is open; doctor ignores unknown categories), and add
   structural recognition so skills surface in listing and resolution.
2. Extend the agent install allowlist so `skills` is persisted into `agents.cue`, and add
   a reader that returns an installed agent's global and local skills paths.
3. Add a skills install path in `internal/modules` that fetches the skill module, resolves
   and dedupes target directories, performs the collision check, copies the bundle tree
   minus `cue.mod/` and `skill.cue`, and returns the written destinations.
4. Add manifest read and write helpers for `skills.cue` using the CUE AST, mirroring the
   existing config writers, with per-leaf entries carrying origin, version, and targets.
5. Wire `start install` to dispatch skills to the materialisation path and record the
   manifest. Wire the skills removal path into the uninstall dispatch seam to delete
   recorded destinations and the manifest entry.
6. Add the get and describe skill branches.
7. Test: category recognition and listing; agent skills persistence; install target
   resolution including default-agent, `--agent` multi, no-target, and unknown-skills-path
   cases; dedupe; collision; bundle copy correctness (resources included, cue.mod and
   skill.cue excluded); manifest round-trip; uninstall removing destinations and manifest
   entry; get output; describe output.

## Implementation Guidance

- Resolution and selection already exist for the other categories; the new surface area is
  the materialisation and the manifest, not matching logic. Keep skills routed through the
  same resolver so `category:name` qualification and menus behave identically.
- Make the target-resolution helper return the concrete destination directories so install,
  uninstall, and describe all report the same paths and stay consistent.
- The collision check protects user-authored or differently-sourced skills already on
  disk. Fail clearly rather than overwriting.

## Acceptance Criteria

- `start library skills` lists available skills, and `start install skills:workflows/one-by-one`
  materialises the bundle into the default agent's global skills directory as
  `<skills_path>/one-by-one/` containing SKILL.md and any bundled resources, without
  cue.mod or skill.cue.
- A manifest entry for `one-by-one` is written recording origin, version, and the target
  directories.
- `--agent a,b` materialises into both agents' resolved directories, deduplicated; `--local`
  materialises into local skills paths and writes the manifest to `./.start/skills.cue`.
- Installing over an existing destination directory errors without overwriting.
- Installing with no default agent, no `--agent`, and multiple skill-capable agents errors;
  naming an agent without a skills attribute errors.
- `start uninstall skills:one-by-one` removes the recorded destination directories and the
  manifest entry.
- `start get skills:workflows/one-by-one` prints the SKILL.md body; `start describe
  skills:workflows/one-by-one` prints metadata, the bundle file list, and resolved targets.
- Installing a claude agent records its `skills` paths into `agents.cue`.
