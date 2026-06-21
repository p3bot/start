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
  `// Managed by 'start install'`) and unrelated/sibling user comments across add, edit, and
  reorder operations (see Requirement 2 for the edited-entry caveat).
- Preserve declared order for roles and contexts (their writers already take an `order`
  slice; the AST equivalent must honour it).
- Change the writer interface to operate on the on-disk AST, not the decoded map. add and
  edit take the single changed `(name, entry)` and upsert it into the freshly parsed file;
  order reorders the existing AST field nodes per the supplied order slice. Update all 10
  call sites in `config_add.go`, `config_edit.go`, and `config_order.go` together. This
  signature change is required, not optional: see Current State for why the full-map
  signature cannot preserve comments.

Out of scope:

- The removal path (`removeAgent`/`removeRole`/`removeContext`/`removeTask` via
  `removeConfigItem`), which is migrated by `01-uninstall-command.md`.
- Any change to `start install`'s observable behaviour. Its module set, field set, origin
  handling, resolution, and output bytes stay exactly as they are, with one exception: the
  shared prompt-rendering detail in requirement 4 (multi-line `prompt` values emitted as
  `"""` heredocs), a formatting concern both paths share. Behaviour-preserving refactors of
  install-side code are expected and permitted where they serve the shared layer — extracting
  the per-category field-order definition out of `formatModuleStruct`, routing the prompt
  field through the shared formatting helper, and sharing the AST-mutation primitives that
  `writeModuleToConfig` owns. These refactors change install's internal structure, not its
  output (apart from the heredoc change above), so they do not count as install behaviour
  changes.
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

Comments are discarded before the writer runs, not inside it. `loadAgentsFromDir` and its
role/context/task siblings decode each entry into a Go struct via `decodeAgentValue`, which
copies field values only — never comments. By the time a writer is called, the in-memory map
holds no comment information, so any writer that rebuilds the file from that map (even via
AST) cannot put comments back. Preservation is only possible if the write reparses the file
and mutates the target field node in place, leaving sibling nodes — and their attached
comments — untouched. The full-map signature also cannot do this selectively: it does not
know which entry changed. This is why the per-entry upsert / node-reorder interface is
mandatory rather than a stylistic choice. It happens to be exactly the shape
`writeModuleToConfig` already uses, and every call site mutates at most one entry (add: one
new, edit: one changed, order: none — only sequence).

## Requirements

1. `config add` adds an entry to the correct category file while preserving the comment
   header, all existing entries, and their comments.
2. `config edit` rewrites the edited entry in place while preserving the comment header and
   all sibling entries with their comments. Comments placed inside the edited entry itself
   are not preserved: edit rebuilds that entry's value from the in-memory struct, which holds
   no comment information — the same limitation install's update path already has.
3. `config order` rewrites roles/contexts in the new order while preserving the comment
   header, comments, and all entry content.
4. All four writers read, mutate, and write exclusively through `cuelang.org/go` AST
   packages — no string manipulation. Output matches the formatting install produces
   (`format.Simplify()`), with per-category field order drawn from the single shared
   definition install uses, not a hand-copied list. Map-valued fields (`agents.models`) are
   emitted in sorted-alias order for determinism, since add/edit read them from an unordered
   Go map; byte-identity with install is therefore guaranteed for scalar and list fields only
   (see Acceptance Criteria). Agents and tasks follow on-disk file order: add appends a new
   entry, edit replaces in place, and existing entries keep their positions — matching install,
   which also appends. The legacy alphabetical sort in `writeAgentsFile`/`writeTasksFile` is
   dropped; do not re-sort entries, as that would break byte-identity with install. Storage
   order is not user-facing — the display, get, and library surfaces sort for presentation.
   Multi-line `prompt` values render as
   CUE `"""` heredoc blocks rather than escaped single-line strings, so add/edit-written
   roles, contexts, and tasks stay human-editable. Install's prompt rendering
   (`formatFieldExpr` in `internal/modules/install.go`) is updated to emit the same
   heredoc form so installed roles, contexts, and tasks with multi-line prompts stay
   human-editable in the file too — the same motivation that drives the heredoc form for
   add/edit. Both paths share one prompt-formatting helper so they cannot drift, and that
   helper emits the heredoc through CUE's multiline quoter (`cuelang.org/go/cue/literal`)
   so the stored block round-trips to the exact prompt value for arbitrary content. Value
   round-trip, not byte-identity, is the binding oracle for the prompt field: byte-identical
   paths can share the same corruption, so the prompt is verified by parsing the written
   block back to its original string (see Acceptance Criteria).
5. A file created fresh by `config add` (no prior file) gains the same managed comment
   header install writes.
6. Existing behavioural `config add`/`edit`/`order` tests continue to pass. The writer
   signature change (full map → single `(name, entry)` for add/edit; order-slice node-reorder
   for order) means the tests that call `writeAgentsFile`/`writeRolesFile`/`writeContextsFile`/
   `writeTasksFile` directly are structural rewrites, not just header-string updates: they no
   longer compile against the old full-map signature. This covers the `Auto-generated`-header
   tests (`TestWriteAgentsFile` and its role/context/task siblings, which must now expect the
   unified managed header from requirement 5) and the behavioural guards
   `TestWriteContextsFile_PreservesOrder`, `TestWriteRolesFile_PreservesOrder`, and
   `TestWriteContextsFile_PreservesUsesAcrossEdit`. When rewriting these, carry forward their
   `uses` round-trip and declared-order preservation assertions onto the new AST writers — do
   not reduce them to header checks or drop them — so the `uses` and order guarantees stated in
   AGENTS.md stay protected. Add new coverage asserting comment-header and sibling-comment
   preservation across each command on top of the carried-forward assertions.

## Implementation Plan

1. Separate the two layers; only the mutation layer is shared.
   - Share the AST-mutation primitives in `internal/modules` (alongside `writeModuleToConfig`
     / `RemoveModuleFromConfig`): parse-or-create the file, find the category struct, upsert
     a field, reorder fields by a given order, reformat with `format.Simplify()`, and write.
     This is the logic install and remove already use; the four writers consume it too.
   - Make the per-category field order a single shared definition. The byte-identical
     guarantee of requirement 4 rests entirely on the field ordering, yet that ordering
     currently lives only in the private `switch` inside `formatModuleStruct`
     (`internal/modules/install.go`). Export it from `internal/modules` as one definition
     (for example a `CategoryFieldOrder(category) []string` helper or an exported table) and
     have both `formatModuleStruct` and the new add/edit content builder iterate it. The two
     writers keep separate value extraction — `formatModuleStruct` reads a `cue.Value`, the
     add/edit builder reads the Go structs — but share the one ordering, so install and
     add/edit cannot drift. Do not re-list the field order by hand in the cli builder.
   - Do not route add/edit through `formatModuleStruct`. It takes a `cue.Value` (a built
     module instance) and unconditionally writes `origin` (plus inline-role substitution),
     neither of which fits add/edit, whose inputs are the Go structs (`AgentConfig` etc.) and
     whose user-created entries have no origin. Give add/edit their own struct→AST content
     builder that omits `origin` when empty and iterates the shared per-category field order
     above, so output stays byte-identical to install (requirement 4) for scalar and list
     fields. The builder reads `models` from an unordered Go map, so it must emit aliases in
     sorted order for deterministic output; this is the one field where add/edit cannot match
     install's source-order emission, and requirement 4 scopes byte-identity accordingly.
   - Render multi-line `prompt` values as `"""` heredoc blocks through one shared prompt
     helper consumed by both the add/edit content builder and install's `formatFieldExpr`.
     Today `formatFieldExpr` emits every string via `ast.NewString`, which `format.Node`
     renders as an escaped single-line literal even for multi-line content; route the prompt
     field through the shared helper instead so install and add/edit agree. This is the one
     install touch permitted by Scope, and it is formatting only — no change to install's
     field set, module set, or origin handling.
   - The helper must produce the heredoc through CUE's own multiline quoter
     (`cuelang.org/go/cue/literal`, e.g. `literal.String.WithOptionalTabIndent(n).Quote(s)`),
     not by hand-concatenating content lines into a `"""` block. A heredoc still interprets
     backslash escapes and `\(...)` interpolation, and a content line containing `"""`
     terminates the block, so a raw line-by-line writer corrupts any prompt holding those
     sequences (code, regex, paths). The legacy string-writer `writeCUEPrompt` concatenates
     lines without escaping and carries exactly this latent bug; do not port it. Install is
     currently immune because `ast.NewString` escapes, so the quoter is what keeps the
     install touch behaviour-preserving for arbitrary prompt content. Byte-identity between
     add/edit and install does not detect this class of corruption — both paths would emit
     the same wrong bytes — so value round-trip (Acceptance Criteria) is the binding oracle
     for the prompt field.
2. Reimplement each writer over that helper, operating on the parsed file AST:
   - add/edit: parse-or-create the file, then upsert the single changed `(name, entry)` into
     the category struct — replace the field's value if present, append it otherwise —
     leaving every sibling node untouched so its comments survive.
   - order (roles/contexts): parse the file and reorder the existing category field nodes to
     match the supplied order slice, moving the actual AST nodes so each entry's comments
     move with it. Do not rebuild entries from the map.
   - In all cases reformat with `format.Simplify()` and write. Update the call sites to pass
     the single changed entry (add/edit) or the order slice (order) rather than the full map.
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
  install-managed comment header and all unrelated entries and comments. (A comment placed
  inside the entry being edited is not retained — its value is rebuilt from the struct.)
- A round-trip of install then `config order` then `config edit` leaves comments intact at
  every step.
- `config add` into a fresh (nonexistent) file writes the managed comment header.
- For scalar and list fields, output is byte-identical to what install would produce for
  the same content, install included (its prompt rendering is updated to the shared heredoc
  form). Map-valued fields (`agents.models`) are not byte-matched to install — install
  preserves the module's source order while add/edit have only an unordered Go map — and are
  instead emitted in deterministic sorted-alias order.
- Field order within each entry comes from the single shared per-category definition install
  uses, and the field set matches install.
- A role, context, or task with a multi-line `prompt` added or edited via the config
  commands stores that prompt as a `"""` heredoc block, not an escaped single-line string.
- Any prompt round-trips to its exact value through both add/edit and install, including
  prompts containing backslashes, `\(...)` interpolation sequences, and embedded `"""`.
  This value round-trip — not heredoc form or byte-identity — is the binding correctness
  oracle for the prompt field, since byte-identical paths can share the same corruption.
