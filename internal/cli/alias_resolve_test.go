package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/p3bot/start/internal/config"
)

// seedStore writes the alias store directly for resolver setup.
func seedStore(t *testing.T, aliases map[string][]string) {
	t.Helper()
	paths, err := config.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	if err := config.WriteAliasStore(config.AliasStorePath(paths), aliases); err != nil {
		t.Fatalf("WriteAliasStore: %v", err)
	}
}

// seedRawStore writes raw bytes to the store (for malformed-store cases).
func seedRawStore(t *testing.T, data string) {
	t.Helper()
	paths, err := config.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	path := config.AliasStorePath(paths)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestResolveAliasArgs_Rewrites(t *testing.T) {
	setupAliasTest(t)
	seedStore(t, map[string][]string{
		"pc":  {"task", "review/pre-commit"},
		"dev": {"--role", "go-expert", "--context", "cwd/agents-md"},
	})

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"bare alias", []string{"pc"}, []string{"task", "review/pre-commit"}},
		{"trailing arg", []string{"pc", "fix the lint"}, []string{"task", "review/pre-commit", "fix the lint"}},
		{"trailing flag", []string{"pc", "--model", "opus"}, []string{"task", "review/pre-commit", "--model", "opus"}},
		{"flag before token", []string{"--debug", "pc"}, []string{"--debug", "task", "review/pre-commit"}},
		{"flags-only alias", []string{"dev"}, []string{"--role", "go-expert", "--context", "cwd/agents-md"}},
		{"case-insensitive", []string{"PC"}, []string{"task", "review/pre-commit"}},
		// A help flag AFTER the token flows into the expansion (target's help).
		{"trailing help flag", []string{"pc", "--help"}, []string{"task", "review/pre-commit", "--help"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAliasArgs(NewRootCmd(), tc.args)
			if err != nil {
				t.Fatalf("resolveAliasArgs: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestResolveAliasArgs_MultiWordTokenVerbatim(t *testing.T) {
	setupAliasTest(t)
	// A stored token containing spaces, commas, colons, and quotes must reach
	// the target as a single argv element, unchanged.
	tricky := `say "hi", ok: yes`
	seedStore(t, map[string][]string{"foo": {"prompt", tricky}})

	got, err := resolveAliasArgs(NewRootCmd(), []string{"foo", "trailing"})
	if err != nil {
		t.Fatalf("resolveAliasArgs: %v", err)
	}
	want := []string{"prompt", tricky, "trailing"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestResolveAliasArgs_NoRewrite(t *testing.T) {
	setupAliasTest(t)
	seedStore(t, map[string][]string{"pc": {"task", "review/pre-commit"}})

	cases := []struct {
		name string
		args []string
	}{
		{"flag value equals alias (long)", []string{"--role", "pc"}},
		{"flag value equals alias (short)", []string{"-r", "pc"}},
		// A help flag BEFORE the token is a help request, not execution.
		{"help before token (long)", []string{"--help", "pc"}},
		{"help before token (short)", []string{"-h", "pc"}},
		{"help before token with flag", []string{"--debug", "--help", "pc"}},
		{"unknown token", []string{"zzz"}},
		{"known subcommand", []string{"task", "pc"}},
		{"subcommand name as first token", []string{"config"}},
		{"bare start", []string{}},
		{"only flags", []string{"--debug"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAliasArgs(NewRootCmd(), tc.args)
			if err != nil {
				t.Fatalf("resolveAliasArgs: %v", err)
			}
			if !reflect.DeepEqual(got, tc.args) {
				t.Errorf("args mutated: got %#v, want unchanged %#v", got, tc.args)
			}
		})
	}
}

func TestResolveAliasArgs_SinglePass(t *testing.T) {
	setupAliasTest(t)
	// a -> b, b -> task x. Resolving "a" must yield exactly "b", never task x.
	seedStore(t, map[string][]string{
		"a": {"b"},
		"b": {"task", "x"},
	})
	got, err := resolveAliasArgs(NewRootCmd(), []string{"a"})
	if err != nil {
		t.Fatalf("resolveAliasArgs: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"b"}) {
		t.Errorf("single-pass violated: got %#v, want [b]", got)
	}
}

func TestResolveAliasArgs_MatchScopedFailure(t *testing.T) {
	setupAliasTest(t)
	// Store parses, but pc's value is the wrong type.
	seedRawStore(t, "aliases: {\n\tpc: \"not a list\"\n}\n")

	// A matching token surfaces the entry's error.
	if _, err := resolveAliasArgs(NewRootCmd(), []string{"pc"}); err == nil {
		t.Error("expected error for matched malformed entry")
	}
	// An unrelated unknown token falls through unchanged (no error).
	got, err := resolveAliasArgs(NewRootCmd(), []string{"zzz"})
	if err != nil {
		t.Errorf("unrelated token should not error: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"zzz"}) {
		t.Errorf("unrelated token mutated: %#v", got)
	}
}

func TestResolveAliasArgs_UnparseableStoreFallsThrough(t *testing.T) {
	setupAliasTest(t)
	seedRawStore(t, "not valid cue {{{\n")
	// Cannot enumerate names: any token falls through unchanged, no error.
	got, err := resolveAliasArgs(NewRootCmd(), []string{"pc"})
	if err != nil {
		t.Fatalf("unparseable store should not error on resolve: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"pc"}) {
		t.Errorf("got %#v, want [pc]", got)
	}
}

func TestResolveAliasArgs_EmptyValueErrors(t *testing.T) {
	setupAliasTest(t)
	// A hand-edited empty-value alias cannot be created through the tool, but the
	// resolver must surface it loudly rather than splice an empty token list.
	seedRawStore(t, "aliases: {\n\tpc: []\n}\n")
	if _, err := resolveAliasArgs(NewRootCmd(), []string{"pc"}); err == nil {
		t.Fatal("expected error for empty-value alias")
	} else if !strings.Contains(err.Error(), "empty value") {
		t.Errorf("error = %q, want substring %q", err.Error(), "empty value")
	}
}

// TestRunRoot_ExpandsAndExecutes covers the real alias entrypoint end-to-end:
// runRoot resolves the leading token, splices the stored command, and dispatches
// it. The other resolver tests assert resolveAliasArgs in isolation; this one
// exercises the resolve -> SetArgs -> Execute wiring that runs in production.
func TestRunRoot_ExpandsAndExecutes(t *testing.T) {
	setupAliasTest(t)
	seedStore(t, map[string][]string{
		"pc":  {"task", "review/pre-commit"},
		"ls2": {"alias", "list"},
	})

	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	if err := runRoot(root, []string{"ls2"}); err != nil {
		t.Fatalf("runRoot: %v", err)
	}
	// ls2 expanded to `alias list`, which renders the stored pc alias.
	if !strings.Contains(out.String(), "start task review/pre-commit") {
		t.Errorf("runRoot did not expand and execute the alias: %q", out.String())
	}
}

func TestResolveAliasArgs_AbsentStore(t *testing.T) {
	setupAliasTest(t)
	got, err := resolveAliasArgs(NewRootCmd(), []string{"pc"})
	if err != nil {
		t.Fatalf("absent store: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"pc"}) {
		t.Errorf("got %#v, want [pc]", got)
	}
}

// --- Subdirectory isolation: a malformed store must not affect any other surface.

func TestAliasStore_SubdirectoryIsolation(t *testing.T) {
	tmp := setupAliasTest(t)
	globalDir := filepath.Join(tmp, "start")

	// A valid sibling config file at the top level of the global dir.
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "roles.cue"), []byte("roles: {r: {prompt: \"x\"}}\n"), 0o644); err != nil {
		t.Fatalf("write roles: %v", err)
	}
	// A malformed alias store in the subdirectory.
	seedRawStore(t, "this is not valid cue {{{\n")

	t.Run("config export omits the store", func(t *testing.T) {
		out, _, err := runAliasCmd(t, "", "config", "export")
		if err != nil {
			t.Fatalf("config export: %v", err)
		}
		if strings.Contains(out, "aliases.cue") || strings.Contains(out, "not valid cue") {
			t.Errorf("config export leaked the alias store:\n%s", out)
		}
		if !strings.Contains(out, "roles.cue") {
			t.Errorf("config export should include the sibling roles.cue:\n%s", out)
		}
	})

	t.Run("main config load unaffected", func(t *testing.T) {
		// config list loads the merged directory config; the malformed subdir
		// file must not break it.
		if _, _, err := runAliasCmd(t, "", "config", "list"); err != nil {
			t.Errorf("config list broke with malformed store present: %v", err)
		}
	})

	t.Run("broken-file globs skip the subdirectory", func(t *testing.T) {
		// CUEFilesInDir and the *.cue glob feed the install/autosetup
		// broken-file diagnostics; both are non-recursive.
		files, err := config.CUEFilesInDir(globalDir)
		if err != nil {
			t.Fatalf("CUEFilesInDir: %v", err)
		}
		for _, f := range files {
			if strings.Contains(f, "aliases.cue") {
				t.Errorf("CUEFilesInDir visited the subdirectory: %s", f)
			}
		}
		matches, _ := filepath.Glob(filepath.Join(globalDir, "*.cue"))
		for _, m := range matches {
			if strings.Contains(m, filepath.Join("aliases", "aliases.cue")) {
				t.Errorf("glob visited the subdirectory: %s", m)
			}
		}
	})
}
