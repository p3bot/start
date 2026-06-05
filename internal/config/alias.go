package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	cueformat "cuelang.org/go/cue/format"
)

// Alias storage lives in a managed file inside a subdirectory of the global
// config directory. The subdirectory keeps the file out of every directory
// package build, which is non-recursive, so a malformed store never breaks the
// main config load.
const (
	// KeyAliases is the sole top-level field of the alias store.
	KeyAliases = "aliases"
	// AliasesDirName is the subdirectory of the global config dir holding the store.
	AliasesDirName = "aliases"
	// AliasesFileName is the managed alias store file.
	AliasesFileName = "aliases.cue"
)

// aliasStoreHeader documents the managed file for anyone who opens it directly.
const aliasStoreHeader = "// Managed by start alias. Only the aliases field is preserved on save.\n\n"

// AliasStoreDir returns the directory holding the global alias store.
func AliasStoreDir(paths Paths) string {
	return filepath.Join(paths.Global, AliasesDirName)
}

// AliasStorePath returns the path to the global alias store file.
func AliasStorePath(paths Paths) string {
	return filepath.Join(paths.Global, AliasesDirName, AliasesFileName)
}

// NormalizeAliasName lowercases an alias name. Names are case-insensitive and
// stored lowercased; values are never touched.
func NormalizeAliasName(name string) string {
	return strings.ToLower(name)
}

// CompileAliasStore reads and compiles the store file in isolation. It returns
// exists=false (and a nil error) when the file is absent. The returned value may
// carry a parse error; callers inspect v.Err() to decide how to react, so a
// malformed store can be handled differently per surface (lenient for the
// resolver, loud for doctor).
func CompileAliasStore(ctx *cue.Context, path string) (v cue.Value, exists bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cue.Value{}, false, nil
	}
	if err != nil {
		return cue.Value{}, false, fmt.Errorf("reading alias store %s: %w", path, err)
	}
	return ctx.CompileBytes(data, cue.Filename(path)), true, nil
}

// AliasNames returns the alias names defined in a compiled store value, or nil
// when the aliases field is absent or not a struct. It is lenient by design: the
// resolver enumerates names without failing on individually malformed entries.
func AliasNames(v cue.Value) []string {
	field := v.LookupPath(cue.ParsePath(KeyAliases))
	if !field.Exists() || field.Kind() != cue.StructKind {
		return nil
	}
	iter, err := field.Fields()
	if err != nil {
		return nil
	}
	var names []string
	for iter.Next() {
		names = append(names, iter.Selector().Unquoted())
	}
	return names
}

// AliasEntryTokens returns the token list for one alias, validating that it is a
// list of strings. found is false when the named entry is absent; err is set
// when the entry exists but is not a well-formed list of strings.
func AliasEntryTokens(v cue.Value, name string) (tokens []string, found bool, err error) {
	field := v.LookupPath(cue.ParsePath(KeyAliases))
	if !field.Exists() {
		return nil, false, nil
	}
	entry := field.LookupPath(cue.MakePath(cue.Str(name)))
	if !entry.Exists() {
		return nil, false, nil
	}
	tokens, err = decodeAliasTokens(entry)
	if err != nil {
		return nil, true, err
	}
	return tokens, true, nil
}

// HasNonAliasTopLevelKeys reports whether a compiled store has any top-level
// field other than aliases. The write guard refuses to overwrite such a file.
func HasNonAliasTopLevelKeys(v cue.Value) bool {
	iter, err := v.Fields(cue.All())
	if err != nil {
		return false
	}
	for iter.Next() {
		if iter.Selector().Unquoted() != KeyAliases {
			return true
		}
	}
	return false
}

// DecodeAliases returns every alias as a map of name to token list, enforcing
// that each value is a list of strings. Used to load the store for upsert and to
// validate an import document's shape.
func DecodeAliases(v cue.Value) (map[string][]string, error) {
	result := make(map[string][]string)
	field := v.LookupPath(cue.ParsePath(KeyAliases))
	if !field.Exists() {
		return result, nil
	}
	if field.Kind() != cue.StructKind {
		return nil, fmt.Errorf("aliases must be a map of name to token list")
	}
	iter, err := field.Fields()
	if err != nil {
		return nil, fmt.Errorf("reading aliases: %w", err)
	}
	for iter.Next() {
		name := iter.Selector().Unquoted()
		tokens, err := decodeAliasTokens(iter.Value())
		if err != nil {
			return nil, fmt.Errorf("alias %q: %w", name, err)
		}
		result[name] = tokens
	}
	return result, nil
}

// decodeAliasTokens enforces the trivial structural #Aliases shape on one entry:
// a list whose every element is a string.
func decodeAliasTokens(entry cue.Value) ([]string, error) {
	if entry.Kind() != cue.ListKind {
		return nil, fmt.Errorf("value must be a list of strings")
	}
	iter, err := entry.List()
	if err != nil {
		return nil, fmt.Errorf("value must be a list of strings: %w", err)
	}
	var tokens []string
	for iter.Next() {
		s, err := iter.Value().String()
		if err != nil {
			return nil, fmt.Errorf("every token must be a string")
		}
		tokens = append(tokens, s)
	}
	return tokens, nil
}

// WriteAliasStore writes the alias map to path through the CUE formatter,
// creating the aliases subdirectory as needed. It fails closed: if an existing
// file does not parse or carries non-aliases top-level keys, nothing is written.
func WriteAliasStore(path string, aliases map[string][]string) error {
	if err := guardExistingStore(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating alias directory: %w", err)
	}
	data, err := formatAliasStore(aliases)
	if err != nil {
		return fmt.Errorf("formatting alias store: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// EnsureAliasStore creates the aliases subdirectory and seeds an empty store
// when the file is absent. Used by open, which hands the path to an external
// editor rather than writing through WriteAliasStore. It still honours the
// write guard so it never clobbers unrecognised content.
func EnsureAliasStore(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking alias store %s: %w", path, err)
	}
	return WriteAliasStore(path, nil)
}

func guardExistingStore(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading alias store %s: %w", path, err)
	}
	ctx := cuecontext.New()
	v := ctx.CompileBytes(data, cue.Filename(path))
	if v.Err() != nil {
		return fmt.Errorf("%s does not parse as CUE; fix or remove it before saving aliases", path)
	}
	if HasNonAliasTopLevelKeys(v) {
		return fmt.Errorf("%s contains non-aliases top-level keys; edit it manually so start does not overwrite that content", path)
	}
	return nil
}

// formatAliasStore renders the alias map as canonical CUE. Names are sorted for
// deterministic output; token strings are quoted by the CUE formatter so
// embedded quotes, commas, and colons round-trip losslessly.
func formatAliasStore(aliases map[string][]string) ([]byte, error) {
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	elts := make([]ast.Decl, 0, len(names))
	for _, name := range names {
		tokens := aliases[name]
		items := make([]ast.Expr, len(tokens))
		for i, tok := range tokens {
			items[i] = ast.NewString(tok)
		}
		elts = append(elts, &ast.Field{
			Label: ast.NewStringLabel(name),
			Value: ast.NewList(items...),
		})
	}

	file := &ast.File{Decls: []ast.Decl{
		&ast.Field{
			Label: ast.NewIdent(KeyAliases),
			Value: &ast.StructLit{Elts: elts},
		},
	}}

	formatted, err := cueformat.Node(file)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	buf.WriteString(aliasStoreHeader)
	buf.Write(formatted)
	return buf.Bytes(), nil
}
