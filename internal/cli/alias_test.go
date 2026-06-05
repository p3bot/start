package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/config"
)

// setupAliasTest isolates HOME/XDG so the global alias store lives in a temp dir.
func setupAliasTest(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	chdir(t, tmp)
	return tmp
}

// runAliasCmd executes the root command with the given args and the given stdin.
func runAliasCmd(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out, errb bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// readStore decodes the on-disk alias store for assertions.
func readStore(t *testing.T) map[string][]string {
	t.Helper()
	paths, err := config.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	path := config.AliasStorePath(paths)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string][]string{}
	}
	m, err := currentAliases(path)
	if err != nil {
		t.Fatalf("currentAliases: %v", err)
	}
	return m
}

func TestAliasSet_CaptureVerbatim(t *testing.T) {
	setupAliasTest(t)

	// A value containing flags must be stored intact under DisableFlagParsing.
	if _, _, err := runAliasCmd(t, "", "alias", "set", "dev", "--role", "go-expert", "--context", "cwd/agents-md"); err != nil {
		t.Fatalf("set: %v", err)
	}
	got := readStore(t)["dev"]
	want := []string{"--role", "go-expert", "--context", "cwd/agents-md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored value = %#v, want %#v", got, want)
	}
}

func TestAliasSet_UpsertPreservesOthers(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "pc", "task", "review/pre-commit")
	mustRun(t, "alias", "set", "dev", "--role", "go-expert")
	// Overwrite pc; dev must survive.
	mustRun(t, "alias", "set", "pc", "task", "lint")

	store := readStore(t)
	if !reflect.DeepEqual(store["pc"], []string{"task", "lint"}) {
		t.Errorf("pc not upserted: %#v", store["pc"])
	}
	if !reflect.DeepEqual(store["dev"], []string{"--role", "go-expert"}) {
		t.Errorf("dev not preserved: %#v", store["dev"])
	}
}

func TestAliasSet_NameLowercasedValuePreserved(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "PC", "TASK", "REVIEW/PRE-COMMIT")
	store := readStore(t)
	if _, ok := store["pc"]; !ok {
		t.Fatalf("name not lowercased: %#v", store)
	}
	if !reflect.DeepEqual(store["pc"], []string{"TASK", "REVIEW/PRE-COMMIT"}) {
		t.Errorf("token case not preserved: %#v", store["pc"])
	}
}

func TestAliasSet_Rejections(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"empty value", []string{"alias", "set", "lonely"}, "needs a value"},
		{"leading start", []string{"alias", "set", "bad", "start", "task", "x"}, "drop the leading start"},
		{"dash name", []string{"alias", "set", "-x", "task", "x"}, "must not start with '-'"},
		{"subcommand collision", []string{"alias", "set", "config", "task", "x"}, "is a built-in command"},
		{"cobra alias collision", []string{"alias", "set", "tasks", "task", "x"}, "is a built-in command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupAliasTest(t)
			_, _, err := runAliasCmd(t, "", tc.args...)
			if err == nil {
				t.Fatalf("expected rejection, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
			// Nothing is written on rejection.
			if len(readStore(t)) != 0 {
				t.Errorf("store should be empty after rejection, got %#v", readStore(t))
			}
		})
	}
}

func TestAliasSet_GlobalOnly(t *testing.T) {
	tmp := setupAliasTest(t)
	// A local config dir exists; aliases must still land in global config only.
	if err := os.MkdirAll(filepath.Join(tmp, ".start"), 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}
	mustRun(t, "alias", "set", "pc", "task", "review/pre-commit")

	paths, err := config.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if _, err := os.Stat(config.AliasStorePath(paths)); err != nil {
		t.Errorf("alias not written to global store: %v", err)
	}
	// No alias store should appear under the local .start/ tree.
	localStore := filepath.Join(paths.Local, config.AliasesDirName, config.AliasesFileName)
	if _, err := os.Stat(localStore); !os.IsNotExist(err) {
		t.Errorf("alias unexpectedly written under local config: %v", err)
	}
}

func TestAliasSet_HelpDoesNotWrite(t *testing.T) {
	for _, args := range [][]string{
		{"alias", "set", "--help"},
		{"help", "alias", "set"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			setupAliasTest(t)
			out, _, err := runAliasCmd(t, "", args...)
			if err != nil {
				t.Fatalf("help: %v", err)
			}
			if !strings.Contains(out, "Create or update an alias") {
				t.Errorf("expected set help text, got %q", out)
			}
			if len(readStore(t)) != 0 {
				t.Errorf("help must not write the store")
			}
		})
	}
}

func TestAliasList_ExpandedShellQuoted(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "foo", "prompt", "this is the prompt")

	out, _, err := runAliasCmd(t, "", "alias", "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Display renders the expanded, shell-quoted command, not a raw token dump.
	if !strings.Contains(out, "start prompt 'this is the prompt'") {
		t.Errorf("list output missing expanded command: %q", out)
	}
	if strings.Contains(out, "[") {
		t.Errorf("list should not show a raw token list: %q", out)
	}
}

func TestAliasList_NoArgsDispatchesList(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "pc", "task", "review/pre-commit")

	bare, _, err := runAliasCmd(t, "", "alias")
	if err != nil {
		t.Fatalf("bare alias: %v", err)
	}
	listed, _, err := runAliasCmd(t, "", "alias", "list")
	if err != nil {
		t.Fatalf("alias list: %v", err)
	}
	if bare != listed {
		t.Errorf("bare 'alias' should match 'alias list':\n bare=%q\nlist=%q", bare, listed)
	}
}

func TestAliasGet(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "pc", "task", "review/pre-commit")

	out, _, err := runAliasCmd(t, "", "alias", "get", "pc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !strings.Contains(out, "start task review/pre-commit") {
		t.Errorf("get output = %q", out)
	}

	// Absent name reports "not set".
	missing, _, err := runAliasCmd(t, "", "alias", "get", "nope")
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if !strings.Contains(missing, "not set") {
		t.Errorf("expected 'not set', got %q", missing)
	}
}

func TestAliasDelete_VariadicAndAbsent(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "a", "task", "x")
	mustRun(t, "alias", "set", "b", "task", "y")
	mustRun(t, "alias", "set", "c", "task", "z")

	// Variadic delete plus an absent name (reports "not set", does not error).
	out, _, err := runAliasCmd(t, "", "alias", "delete", "a", "b", "ghost")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(out, "not set") {
		t.Errorf("expected 'not set' for ghost, got %q", out)
	}
	store := readStore(t)
	if _, ok := store["a"]; ok {
		t.Error("a not deleted")
	}
	if _, ok := store["b"]; ok {
		t.Error("b not deleted")
	}
	if _, ok := store["c"]; !ok {
		t.Error("c should remain")
	}
}

func TestAliasImport_MergeRoundTripNoOp(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "pc", "task", "review/pre-commit")
	mustRun(t, "alias", "set", "dev", "--role", "go-expert")

	exported, _, err := runAliasCmd(t, "", "alias", "export")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	before := readStore(t)

	if _, _, err := runAliasCmd(t, exported, "alias", "import"); err != nil {
		t.Fatalf("import: %v", err)
	}
	after := readStore(t)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("export|import not a no-op:\nbefore=%#v\nafter =%#v", before, after)
	}
}

func TestAliasImport_MergeUpsert(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "pc", "task", "review/pre-commit")

	doc := "aliases: {\n\tdev: [\"--role\", \"go-expert\"]\n}\n"
	if _, _, err := runAliasCmd(t, doc, "alias", "import"); err != nil {
		t.Fatalf("import: %v", err)
	}
	store := readStore(t)
	if _, ok := store["pc"]; !ok {
		t.Error("merge dropped existing pc")
	}
	if !reflect.DeepEqual(store["dev"], []string{"--role", "go-expert"}) {
		t.Errorf("merge did not add dev: %#v", store)
	}
}

func TestAliasImport_ReplaceFromFile(t *testing.T) {
	tmp := setupAliasTest(t)
	mustRun(t, "alias", "set", "pc", "task", "review/pre-commit")

	file := filepath.Join(tmp, "repl.cue")
	if err := os.WriteFile(file, []byte("aliases: {\n\tonly: [\"prompt\", \"hi\"]\n}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, err := runAliasCmd(t, "", "alias", "import", "--replace", file); err != nil {
		t.Fatalf("import --replace: %v", err)
	}
	store := readStore(t)
	if _, ok := store["pc"]; ok {
		t.Error("replace should drop pc")
	}
	if !reflect.DeepEqual(store["only"], []string{"prompt", "hi"}) {
		t.Errorf("replace did not set only: %#v", store)
	}
}

func TestAliasImport_AtomicRejections(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"invalid entry", "aliases: {config: [\"task\", \"x\"]}\n", "built-in command"},
		{"parse failure", "aliases: {{{ not cue", "does not parse"},
		{"non-aliases keys", "roles: {x: {}}\n", "non-aliases top-level keys"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupAliasTest(t)
			mustRun(t, "alias", "set", "keep", "task", "x")
			before := readStore(t)

			_, _, err := runAliasCmd(t, tc.doc, "alias", "import")
			if err == nil {
				t.Fatalf("expected rejection")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
			if !reflect.DeepEqual(readStore(t), before) {
				t.Errorf("store mutated despite rejection")
			}
		})
	}
}

func TestAliasImport_NormalizationCollisionRejected(t *testing.T) {
	setupAliasTest(t)
	mustRun(t, "alias", "set", "keep", "task", "x")
	before := readStore(t)

	// PC and pc are distinct CUE fields but fold to one alias name. The document
	// is ambiguous, so import must reject it atomically and leave the store as-is.
	doc := "aliases: {\n\tPC: [\"task\", \"a\"]\n\tpc: [\"task\", \"b\"]\n}\n"
	_, _, err := runAliasCmd(t, doc, "alias", "import")
	if err == nil {
		t.Fatalf("expected rejection for normalization collision")
	}
	if !strings.Contains(err.Error(), "normalize") {
		t.Errorf("error = %q, want substring %q", err.Error(), "normalize")
	}
	if !reflect.DeepEqual(readStore(t), before) {
		t.Errorf("store mutated despite rejected collision")
	}
}

func TestAliasImport_RefusesMangledExistingStore(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "merge"
		if replace {
			name = "replace"
		}
		t.Run(name, func(t *testing.T) {
			setupAliasTest(t)
			// Seed a store with non-aliases keys the tool cannot round-trip.
			paths, _ := config.ResolvePaths("")
			path := config.AliasStorePath(paths)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(path, []byte("aliases: {}\nroles: {x: {}}\n"), 0o644); err != nil {
				t.Fatalf("seed: %v", err)
			}

			args := []string{"alias", "import"}
			if replace {
				args = append(args, "--replace")
			}
			_, _, err := runAliasCmd(t, "aliases: {dev: [\"--role\", \"x\"]}\n", args...)
			if err == nil {
				t.Fatalf("expected refusal against mangled store")
			}
		})
	}
}

// mustRun runs an alias command and fails the test on error.
func mustRun(t *testing.T, args ...string) {
	t.Helper()
	if _, _, err := runAliasCmd(t, "", args...); err != nil {
		t.Fatalf("command %v failed: %v", args, err)
	}
}
