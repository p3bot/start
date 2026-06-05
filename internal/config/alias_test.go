package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

// loadStoreMap reads the store at path and decodes it to a map for assertions.
func loadStoreMap(t *testing.T, path string) map[string][]string {
	t.Helper()
	ctx := cuecontext.New()
	v, exists, err := CompileAliasStore(ctx, path)
	if err != nil {
		t.Fatalf("CompileAliasStore: %v", err)
	}
	if !exists {
		return map[string][]string{}
	}
	if v.Err() != nil {
		t.Fatalf("store does not parse: %v", v.Err())
	}
	m, err := DecodeAliases(v)
	if err != nil {
		t.Fatalf("DecodeAliases: %v", err)
	}
	return m
}

func TestAliasStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, AliasesDirName, AliasesFileName)

	// Tokens with spaces, commas, colons, and embedded quotes must survive verbatim.
	in := map[string][]string{
		"pc":  {"task", "review/pre-commit"},
		"foo": {"prompt", `say "hi", ok: yes`},
		"dev": {"--role", "go-expert", "--context", "cwd/agents-md"},
	}
	if err := WriteAliasStore(path, in); err != nil {
		t.Fatalf("WriteAliasStore: %v", err)
	}

	got := loadStoreMap(t, path)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("round-trip mismatch:\n got %#v\nwant %#v", got, in)
	}
}

func TestAliasStore_TokenCasePreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, AliasesDirName, AliasesFileName)

	// The config layer stores keys verbatim; lowercasing is the caller's job.
	in := map[string][]string{"pc": {"TASK", "REVIEW/PRE-COMMIT"}}
	if err := WriteAliasStore(path, in); err != nil {
		t.Fatalf("WriteAliasStore: %v", err)
	}
	got := loadStoreMap(t, path)
	if !reflect.DeepEqual(got["pc"], []string{"TASK", "REVIEW/PRE-COMMIT"}) {
		t.Fatalf("token case not preserved: %#v", got["pc"])
	}
}

func TestAliasStore_EmptyStoreSeedsAliasesField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, AliasesDirName, AliasesFileName)
	if err := WriteAliasStore(path, nil); err != nil {
		t.Fatalf("WriteAliasStore: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading store: %v", err)
	}
	if !strings.Contains(string(data), "aliases: {}") {
		t.Fatalf("empty store should contain 'aliases: {}', got:\n%s", data)
	}
}

func TestAliasStore_WriteCreatesSubdir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, AliasesDirName, AliasesFileName)
	if err := WriteAliasStore(path, map[string][]string{"pc": {"task", "x"}}); err != nil {
		t.Fatalf("WriteAliasStore: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("subdirectory not created: %v", err)
	}
}

func TestAliasStore_GuardRefusesUnparseable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, AliasesDirName, AliasesFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not valid cue {{{"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := WriteAliasStore(path, map[string][]string{"pc": {"task", "x"}})
	if err == nil {
		t.Fatal("expected refusal to overwrite unparseable store")
	}
	if !strings.Contains(err.Error(), "does not parse") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAliasStore_GuardRefusesNonAliasKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, AliasesDirName, AliasesFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("aliases: {}\nroles: {x: {}}\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	err := WriteAliasStore(path, map[string][]string{"pc": {"task", "x"}})
	if err == nil {
		t.Fatal("expected refusal to overwrite store with non-aliases keys")
	}
	if !strings.Contains(err.Error(), "non-aliases top-level keys") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAliasStore_CompileAbsent(t *testing.T) {
	ctx := cuecontext.New()
	_, exists, err := CompileAliasStore(ctx, filepath.Join(t.TempDir(), "missing.cue"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatal("absent store should report exists=false")
	}
}

func TestAliasEntryTokens_TypeError(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`aliases: {pc: "not a list"}`)
	if v.Err() != nil {
		t.Fatalf("compile: %v", v.Err())
	}
	_, found, err := AliasEntryTokens(v, "pc")
	if !found {
		t.Fatal("pc should be found")
	}
	if err == nil {
		t.Fatal("expected type error for non-list value")
	}
}

func TestAliasNames_LenientOnMissingField(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`other: 1`)
	if names := AliasNames(v); names != nil {
		t.Fatalf("expected nil names, got %#v", names)
	}
}

func TestHasNonAliasTopLevelKeys(t *testing.T) {
	ctx := cuecontext.New()
	cases := []struct {
		src  string
		want bool
	}{
		{`aliases: {}`, false},
		{`aliases: {pc: ["task"]}`, false},
		{`aliases: {}` + "\n" + `roles: {}`, true},
		{`settings: {x: 1}`, true},
	}
	for _, tc := range cases {
		v := ctx.CompileString(tc.src)
		if got := HasNonAliasTopLevelKeys(v); got != tc.want {
			t.Errorf("HasNonAliasTopLevelKeys(%q) = %v, want %v", tc.src, got, tc.want)
		}
	}
}
