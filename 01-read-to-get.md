# Rename read command to get

## Goal

Rename the `start read` command to `start get`. The verb `get` is the canonical name for single-resource retrieval in the project's CLI Design for Agents spec, and `read` is not in the canonical vocabulary. The command's behaviour does not change.

## Scope

In scope:

- Rename the cobra command from `read` to `get`
- Rename `internal/cli/read.go` and `internal/cli/read_test.go` to `get.go` and `get_test.go`
- Rename all `read`-rooted Go identifiers (functions, helpers, test names) in those files
- Update all comments, doc-comments, and help text that name the command or its identifiers
- Update cross-file comments in `internal/cli/show.go` and `internal/orchestration/template_test.go` that reference the renamed identifiers
- Update the command-list entry in `AGENTS.md`
- Hard cutover with no `read` alias preserved

Out of scope:

- Any other command rename or restructuring
- The `show` → `describe` rename. Cross-file comments in `show.go` are touched in passing for this project; retain the current `show` wording in those comments
- Behaviour changes to the command (output format, flags, resolution logic)
- Releases or tags

## Current State

The command is defined at `internal/cli/read.go` and registered from `internal/cli/root.go` via `addReadCommand(cmd)`. It has no aliases — there is no `Aliases:` field on the cobra command. The command resolves a module by name and writes its content to stdout in a pipe-clean fashion (data on stdout, all other diagnostics on stderr).

Identifier inventory (from `rg -n 'addReadCommand|readResolveQuery|readAgent|readUTD|printReadVerbose|runRead' internal/ test/`):

- `addReadCommand` — registration entry point (read.go)
- `runRead` — RunE handler (read.go)
- `readResolveQuery` — resolves the module query argument (read.go)
- `readAgent` — agent-rendering branch (read.go)
- `readUTD` — UTD-module-rendering branch (read.go)
- `printReadVerbose` — verbose metadata printer (read.go)
- `runReadCmd` — test helper (read_test.go)
- `TestRead*` — test functions in read_test.go (approximately 50)

Cross-file references that name the command or its identifiers:

- `internal/cli/root.go` — registration call to `addReadCommand`
- `internal/cli/show.go` — two comment references naming `runRead` / the `read` command
- `internal/orchestration/template_test.go` — three comment references naming `readUTD` / `read.go`
- `AGENTS.md` — the command-list entry `start read <name>`

The cobra `--global` flag on `read` has the description text `Read from global scope only`, which uses the verb form of the command name and must be updated alongside the command rename.

There are no references in `README.md`, `docs/`, `internal/cli/help/`, or the shell scripts under `scripts/` that name the `start read` command. Existing `read -r` invocations in scripts are unrelated bash built-ins.

## References

- CLI Design for Agents spec at https://github.com/start-cli/library/issues/2 — the canonical vocabulary that motivates this rename. Rule 9 lists `get` as the canonical verb for single-resource retrieval (never `info`, `show`, `describe`); `read` is not in the table.

## Requirements

1. The cobra command `Use` is `get [name]`. There is no `Aliases:` field. Invoking `start read` returns the standard cobra unknown-command error.
2. `internal/cli/read.go` is renamed to `internal/cli/get.go` and `internal/cli/read_test.go` to `internal/cli/get_test.go`. Package declarations are unchanged.
3. All `read`-rooted identifiers from the inventory above are renamed to their `get`-rooted equivalents. The implementer chooses the natural mapping (`addReadCommand` → `addGetCommand`, `runRead` → `runGet`, etc.). No `read`-rooted identifier remains in the renamed files.
4. The test helper is renamed alongside the others; all `TestRead*` function names are renamed to the `TestGet*` form; all `cmd.SetArgs` invocations using `"read"` are updated to `"get"`.
5. The cobra `Long:` help text and the inline doc-comments are updated so wording reflects the new command name. The `--global` flag description is updated to remove the verb form of the old name.
6. Cross-file references in `internal/cli/root.go`, `internal/cli/show.go`, and `internal/orchestration/template_test.go` are updated to name the new identifiers.
7. The `AGENTS.md` command-list line is updated to `start get <name>`.
8. Final verification: every reference to the old command name and old identifiers is gone. Incidental uses of the English word "read" in unrelated senses (file I/O verbs, the bash `read -r` built-in in scripts) are acceptable and must not be substituted.

## Implementation Plan

1. File rename
   Move `internal/cli/read.go` to `internal/cli/get.go` and `internal/cli/read_test.go` to `internal/cli/get_test.go`. Package declaration stays `cli`.

2. Identifier rename in renamed files
   Apply the mapping in requirement 3 to `get.go` and the test file. Apply the `TestRead*` → `TestGet*` mapping. Update `cmd.SetArgs([]string{"read", ...})` to `"get"` everywhere.

3. Cobra command surface
   Set `Use: "get [name]"`. Confirm no `Aliases:` field is present. Update the `--global` flag description.

4. Help text and doc-comments
   Edit the `Long:` string in `get.go` so wording reflects the new command name. Update internal doc-comments and inline commentary that name the command or the renamed identifiers.

5. Cross-file references
   Update the registration call in `internal/cli/root.go`. Update the two comment references in `internal/cli/show.go`. Update the three comment references in `internal/orchestration/template_test.go`.

6. Documentation
   Update the `AGENTS.md` command-list entry to `start get <name>`.

7. Verify
   Run `rg -n 'addReadCommand|readResolveQuery|readAgent|readUTD|printReadVerbose|runRead|runReadCmd|TestRead' .` and confirm zero matches outside this project document. Run `rg -n 'start read\b' .` and confirm zero matches outside this project document. Audit any remaining `read` matches in the renamed files, `show.go`, `root.go`, `template_test.go`, and `AGENTS.md` — resolve anything that names the command; leave incidental English uses untouched.

8. Build and test
   Run `gofmt -w .`, `go build ./...`, and `scripts/invoke-tests`.

## Constraints

- Hard cutover. No `read` alias on the cobra command. No backward-compatibility shim.
- Pure rename. The command's behaviour, flags, output format, error handling, and help-text content (other than the command name itself) are unchanged.
- Do not modify any file under `library/` or `homebrew-tap/`.
- Do not produce a release tag or push to remotes.

## Implementation Guidance

- The English word "read" appears in the help text and code comments in two distinct senses: naming the CLI command ("read outputs the file"), and describing file I/O ("file contents are read", "DefaultFileReader will read from"). The first sense must be updated. The second must not — substituting "got" for "read" in those phrases is wrong English. Audit each occurrence individually before substituting.
- The cross-file comments in `show.go` and `template_test.go` exist as navigation aids pointing back to the implementation. Updating the identifier names keeps the aid working; do not delete the comments unless they are already stale for some other reason.
- The `runReadCmd` test helper is referenced by every test in the file. Rename it once and the call sites follow with a straightforward substitution.

## Acceptance Criteria

- `start --help` lists `get` and does not list `read`.
- `start get <name>` produces the same output as `start read <name>` did before this project.
- `start read` exits with cobra's unknown-command error.
- `internal/cli/get.go` and `internal/cli/get_test.go` exist; `internal/cli/read.go` and `internal/cli/read_test.go` do not.
- `rg -n 'addReadCommand|readResolveQuery|readAgent|readUTD|printReadVerbose|runRead|runReadCmd|TestRead' .` returns no matches outside this project document.
- `rg -n 'start read\b' .` returns no matches outside this project document.
- `AGENTS.md` references `start get`, not `start read`.
- `scripts/invoke-tests` passes.
