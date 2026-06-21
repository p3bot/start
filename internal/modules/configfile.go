package modules

import (
	"fmt"
	"os"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

// CategoryFieldOrder returns the order in which a module category's fields are
// emitted to config files. Install and the config add/edit writers both iterate
// this single definition so their output cannot drift. The origin field, when
// present, is always written first and is not part of this list.
func CategoryFieldOrder(category string) []string {
	switch category {
	case "tasks":
		return []string{"description", "tags", "uses", "role", "file", "command", "prompt"}
	case "roles":
		return []string{"description", "tags", "uses", "file", "command", "prompt", "optional"}
	case "agents":
		return []string{"description", "tags", "uses", "bin", "command", "default_model", "models"}
	case "contexts":
		return []string{"description", "tags", "uses", "file", "command", "prompt", "required", "default"}
	default:
		return []string{"description", "tags", "uses", "prompt"}
	}
}

// PromptExpr renders a prompt string as a CUE AST expression: a """ heredoc for
// multi-line content, a quoted single-line string otherwise. The heredoc is
// produced through CUE's multiline quoter, which escapes backslashes, \(...)
// interpolation, and embedded """ so the stored block round-trips to the exact
// value for arbitrary content — a hand-written line-by-line block would corrupt
// any of those sequences.
func PromptExpr(s string) ast.Expr {
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: literal.String.WithOptionalTabIndent(1).Quote(s),
	}
}

// managedHeader is the comment header written at the top of a freshly created
// config file, shared by install and the config add path.
func managedHeader() *ast.CommentGroup {
	return &ast.CommentGroup{
		Doc: true,
		List: []*ast.Comment{
			{Text: "// start configuration"},
			{Text: "// Managed by 'start install'"},
		},
	}
}

// parseConfigFile parses the config file preserving comments. It returns a nil
// file (and nil error) when the file is absent or empty, signalling the caller
// to create one. A genuine read or parse failure is returned as an error.
func parseConfigFile(configPath string) (*ast.File, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	file, err := parser.ParseFile(configPath, data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}
	return file, nil
}

// writeConfigFile formats file with format.Simplify() and writes it to configPath.
func writeConfigFile(configPath string, file *ast.File) error {
	formatted, err := format.Node(file, format.Simplify())
	if err != nil {
		return fmt.Errorf("formatting config: %w", err)
	}
	return os.WriteFile(configPath, formatted, 0644)
}

// newCategoryField builds a top-level `category: { ... }` field holding the
// given declarations.
func newCategoryField(category string, elts ...ast.Decl) *ast.Field {
	return &ast.Field{
		Label: ast.NewIdent(category),
		Value: &ast.StructLit{Elts: elts},
	}
}

// UpsertConfigModule upserts content as the field named name within the category
// struct of the config file at configPath. A missing file is created with the
// managed header; an existing entry is replaced in place and a new one appended,
// leaving sibling entries and all comments intact.
func UpsertConfigModule(configPath, category, name string, content ast.Expr) error {
	file, err := parseConfigFile(configPath)
	if err != nil {
		return err
	}
	file, err = upsertModuleField(file, category, name, content)
	if err != nil {
		return err
	}
	return writeConfigFile(configPath, file)
}

// upsertModuleField inserts or replaces the name field within file's category
// struct, returning the (possibly newly created) file. A nil file yields a fresh
// file carrying the managed header.
func upsertModuleField(file *ast.File, category, name string, content ast.Expr) (*ast.File, error) {
	moduleField := &ast.Field{
		Label: ast.NewStringLabel(name),
		Value: content,
	}

	if file == nil {
		categoryField := newCategoryField(category, moduleField)
		ast.AddComment(categoryField, managedHeader())
		return &ast.File{Decls: []ast.Decl{categoryField}}, nil
	}

	catField := findCategoryField(file, category)
	if catField == nil {
		file.Decls = append(file.Decls, newCategoryField(category, moduleField))
		return file, nil
	}

	catStruct, ok := catField.Value.(*ast.StructLit)
	if !ok {
		return nil, fmt.Errorf("category %q is not a struct", category)
	}
	if existing := findModuleField(catStruct, name); existing != nil {
		existing.Value = content
	} else {
		catStruct.Elts = append(catStruct.Elts, moduleField)
	}
	return file, nil
}

// ReorderConfigCategory reorders the entries within the category struct of the
// config file to match order. Each entry's AST node — and the comments attached
// to it — moves with it. Entries absent from order keep their original relative
// position after the ordered ones.
func ReorderConfigCategory(configPath, category string, order []string) error {
	file, err := parseConfigFile(configPath)
	if err != nil {
		return err
	}
	if file == nil {
		return fmt.Errorf("config file not found: %s", configPath)
	}
	catField := findCategoryField(file, category)
	if catField == nil {
		return fmt.Errorf("category %q not found in %s", category, configPath)
	}
	catStruct, ok := catField.Value.(*ast.StructLit)
	if !ok {
		return fmt.Errorf("category %q is not a struct", category)
	}
	reorderStructFields(catStruct, order)
	return writeConfigFile(configPath, file)
}

// reorderStructFields rearranges s.Elts so the fields named in order appear
// first in that order; any remaining declarations follow in their original
// order. Field nodes are moved intact so their comments travel with them.
func reorderStructFields(s *ast.StructLit, order []string) {
	byName := make(map[string]*ast.Field, len(s.Elts))
	for _, elt := range s.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			continue
		}
		if name, _, err := ast.LabelName(field.Label); err == nil {
			byName[name] = field
		}
	}

	reordered := make([]ast.Decl, 0, len(s.Elts))
	placed := make(map[string]bool, len(order))
	for _, name := range order {
		if field, ok := byName[name]; ok && !placed[name] {
			reordered = append(reordered, field)
			placed[name] = true
		}
	}
	for _, elt := range s.Elts {
		if field, ok := elt.(*ast.Field); ok {
			if name, _, err := ast.LabelName(field.Label); err == nil && placed[name] {
				continue
			}
		}
		reordered = append(reordered, elt)
	}
	s.Elts = reordered
}

// findCategoryField returns the top-level field for category, or nil.
func findCategoryField(file *ast.File, category string) *ast.Field {
	for _, decl := range file.Decls {
		field, ok := decl.(*ast.Field)
		if !ok {
			continue
		}
		name, _, err := ast.LabelName(field.Label)
		if err != nil {
			continue
		}
		if name == category {
			return field
		}
	}
	return nil
}

// findModuleField returns the field named name within s, or nil.
func findModuleField(s *ast.StructLit, name string) *ast.Field {
	for _, elt := range s.Elts {
		field, ok := elt.(*ast.Field)
		if !ok {
			continue
		}
		labelName, _, err := ast.LabelName(field.Label)
		if err != nil {
			continue
		}
		if labelName == name {
			return field
		}
	}
	return nil
}
