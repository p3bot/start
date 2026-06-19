# Migrate config add/edit/order onto AST writers

Source: project-doc-review on 2026-06-19
Category: design
Location: `internal/cli/config_types.go` (`writeAgentsFile`, `writeRolesFile`, `writeContextsFile`, `writeTasksFile`); callers in `config_add.go`, `config_edit.go`, `config_order.go`

## Goal

Make every config-file mutation preserve comments and CUE formatting uniformly. The
uninstall project (`01-uninstall-command.md`) migrates the removal path onto an AST-based,
comment-preserving writer but deliberately leaves `config add`, `config edit`, and
`config order` on the legacy string-rebuild writers. This project finishes the job so
install, add, edit, order, and remove all mutate config through one AST layer, eliminating
the inconsistency where some commands preserve the install-managed comment header and
others silently destroy it on the same files.

## Scope

In scope:

- Replace the string-rebuild bodies of `writeAgentsFile`, `writeRolesFile`,
  `writeContextsFile`, and `writeTasksFile` with AST-based reads/mutations/writes using
  the official `cuelang.org/go` packages (`cue/parser`, `cue/ast`, `cue/format`), matching
  the pattern already used by `internal/modules/install.go` (`writeModuleToConfig`,
  `findCategoryField`, `findModuleField`).
- Preserve the install-managed comment header (`// start configuration` /
  `// Managed by 'start install'`) and any user comments across add, edit, and reorder
  operations.
- Preserve declared order for roles and contexts (their writers already take an `order`
  slice; the AST equivalent must honour it).
- Keep the existing function signatures so the call sites in `config_add.go`,
  `config_edit.go`, and `config_order.go` are unaffected (or update all call sites together
  if a signature change is cleaner).

Out of scope:

- The removal path (`removeAgent`/`removeRole`/`removeContext`/`removeTask` via
  `removeConfigItem`), which is migrated by `01-uninstall-command.md`.
- Any change to `start install` behaviour.
- The skills category.

## Current State

The four `writeXFile` functions in `internal/cli/config_types.go` rebuild each config file
from an in-memory struct via string building. This loses the install-managed comment header
and any user comments, and (before the uninstall project's removal fix) leaves an emptied
category as `agents: {}`. They are called from 10 sites:

- `config_add.go` — adds a new entry to each category file.
- `config_edit.go` — rewrites an edited entry.
- `config_order.go` — rewrites roles/contexts after a reorder.

Install already mutates config the right way: `internal/modules/install.go`'s
`writeModuleToConfig` parses the file with `parser.ParseComments`, locates the category and
module fields with `findCategoryField`/`findModuleField`, upserts the module field, and
reformats with `format.Simplify()` — preserving comments and unrelated entries. This is the
pattern to generalise.

Because add and edit need comment-preserving insertion and update (not just deletion), and
order needs an order-preserving rewrite, this is a larger change than the delete-only
removal migration — hence its separation from the uninstall project.

## Requirements

1. `config add` adds an entry to the correct category file while preserving the comment
   header, all existing entries, and their comments.
2. `config edit` rewrites the edited entry in place while preserving the comment header,
   sibling entries, and comments.
3. `config order` rewrites roles/contexts in the new order while preserving the comment
   header, comments, and all entry content.
4. All four writers read, mutate, and write exclusively through `cuelang.org/go` AST
   packages — no string manipulation. Output matches the formatting install produces
   (`format.Simplify()`).
5. A file created fresh by `config add` (no prior file) gains the same managed comment
   header install writes.
6. Existing `config add`/`edit`/`order` tests pass; add coverage asserting comment-header
   and sibling-comment preservation across each command.

## Implementation Plan

1. Factor a shared AST upsert/reorder helper in `internal/modules` (or alongside
   `writeModuleToConfig`) that the four writers and the install path can both use, so the
   AST mutation logic lives in one place.
2. Reimplement each `writeXFile` over that helper: parse-or-create the file, upsert each
   entry (add/edit) or rebuild the category struct in the supplied order (roles/contexts),
   reformat with `format.Simplify()`, and write.
3. Verify the removal path (migrated by the uninstall project) and these writers share the
   same parse/format conventions so files stay stable across mixed command sequences.
4. Extend tests for add, edit, and order to assert comment and sibling preservation.

## Constraints

- Go 1.25, cobra, `cuelang.org/go`, matching the existing module.
- All CUE file reads, mutations, and writes use the official `cuelang.org/go` packages. Do
  not edit config files by string manipulation.
- Reuse the existing scope and config-path helpers; do not reimplement path discovery.
- Do this after `01-uninstall-command.md` lands, so both efforts converge on one shared AST
  mutation layer rather than two parallel ones.

## Acceptance Criteria

- After `config add`, `config edit`, or `config order`, the target file retains the
  install-managed comment header and all unrelated entries and comments.
- A round-trip of install then `config order` then `config edit` leaves comments intact at
  every step.
- `config add` into a fresh (nonexistent) file writes the managed comment header.
- Output formatting is byte-identical to what install would produce for the same content.
