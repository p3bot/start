package modules

import (
	"fmt"
	"os"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/literal"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"

	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/fault"
)

// CategoryFieldOrder returns the order in which a module category's fields are
// emitted to config files. Install and the config add/edit writers both iterate
// this single definition so their output cannot drift. The origin field, when
// present, is always written first and is not part of this list.
func CategoryFieldOrder(category string) []string {
	switch category {
	case "tasks":
		return []string{"description", "tags", "uses", "role", "file", "command", "prompt", "min_members"}
	case "roles":
		return []string{"description", "tags", "uses", "file", "command", "prompt", "priority"}
	case "agents":
		return []string{"description", "tags", "uses", "agentdex", "bin", "command", "flags", "default_model", "models", "session_locator"}
	case "contexts":
		return []string{"description", "tags", "uses", "file", "command", "prompt", "required", "default"}
	default:
		return []string{"description", "tags", "uses", "prompt"}
	}
}

// CategoryHasField reports whether name is a CUE field of category.
// The set is the schema. It includes fields CategoryFieldOrder does not rewrite,
// such as origin, shell, timeout, and a task's agent.
func CategoryHasField(category, name string) bool {
	switch category {
	case "agents":
		return fieldName(name, "description", "tags", "origin", "uses", "agentdex", "bin", "command", "flags", "default_model", "models", "session_locator")
	case "roles":
		return fieldName(name, "description", "tags", "origin", "uses", "file", "command", "prompt", "shell", "timeout", "priority")
	case "contexts":
		return fieldName(name, "description", "tags", "origin", "uses", "file", "command", "prompt", "shell", "timeout", "required", "default")
	case "tasks":
		return fieldName(name, "description", "tags", "origin", "uses", "file", "command", "prompt", "shell", "timeout", "role", "agent", "min_members")
	case "skills":
		return fieldName(name, "description", "tags", "origin", "uses", "file")
	default:
		return false
	}
}

func fieldName(name string, names ...string) bool {
	for _, n := range names {
		if name == n {
			return true
		}
	}
	return false
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

// writeConfigFile formats file and writes it to configPath.
func writeConfigFile(configPath string, file *ast.File) error {
	formatted, err := formatConfigFile(file)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, formatted, 0644)
}

// formatConfigFile renders file with format.Simplify(), as every writer stores it.
func formatConfigFile(file *ast.File) ([]byte, error) {
	formatted, err := format.Node(file, format.Simplify())
	if err != nil {
		return nil, fmt.Errorf("formatting config: %w", err)
	}
	return formatted, nil
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

// PatchConfigModule writes content into the entry named name within the
// category struct of the config file at configPath. In a stored entry only the
// fields content manages (origin and CategoryFieldOrder) are replaced, inserted,
// or removed to match content; every other field and comment is kept. A stored
// entry that is not a plain struct, or that embeds a struct or holds a
// comprehension, is refused, since those can supply a managed field the patch
// would then conflict with. An absent entry or file is created as
// UpsertConfigModule would. A result that leaves the config directory unable
// to build is refused and nothing is written.
func PatchConfigModule(configPath, category, name string, content *ast.StructLit) error {
	file, err := parseConfigFile(configPath)
	if err != nil {
		return err
	}
	if stored := findModuleEntry(file, category, name); stored != nil {
		entry, ok := stored.Value.(*ast.StructLit)
		if !ok || !onlyDirectFields(entry) {
			return fault.UserConfig(fmt.Errorf("%s %q is not a plain struct of its own fields, so it cannot be edited without losing or conflicting with fields; edit it with 'start config open'", category, name))
		}
		patchManagedFields(entry, content, append([]string{"origin"}, CategoryFieldOrder(category)...))
		return writeBuildingConfigFile(configPath, category, name, file)
	}
	file, err = upsertModuleField(file, category, name, content)
	if err != nil {
		return err
	}
	return writeBuildingConfigFile(configPath, category, name, file)
}

// writeBuildingConfigFile writes file to configPath only when the config
// directory still builds with it. The patch replaces expressions with literals,
// which a split declaration or a let the entry read can turn into a conflict
// or an unused let that only evaluation reveals.
func writeBuildingConfigFile(configPath, category, name string, file *ast.File) error {
	formatted, err := formatConfigFile(file)
	if err != nil {
		return err
	}
	if err := internalcue.CheckBuildsWith(configPath, formatted); err != nil {
		return fault.UserConfig(fmt.Errorf("%s %q cannot be written without leaving the config unable to load, so nothing was written; fix it with 'start config open': %w", category, name, err))
	}
	return os.WriteFile(configPath, formatted, 0644)
}

// onlyDirectFields reports whether entry holds no embedding or comprehension,
// so every field it supplies is one of its own direct fields.
func onlyDirectFields(entry *ast.StructLit) bool {
	for _, elt := range entry.Elts {
		switch elt.(type) {
		case *ast.EmbedDecl, *ast.Comprehension:
			return false
		}
	}
	return true
}

// findModuleEntry returns the name field within file's category struct, or nil.
func findModuleEntry(file *ast.File, category, name string) *ast.Field {
	if file == nil {
		return nil
	}
	catField := findCategoryField(file, category)
	if catField == nil {
		return nil
	}
	catStruct, ok := catField.Value.(*ast.StructLit)
	if !ok {
		return nil
	}
	return findModuleField(catStruct, name)
}

// patchManagedFields makes the managed fields of stored match content. A kept
// field keeps its node, so its comments stay, and becomes a regular field; a new
// one goes after the nearest earlier managed field in order.
func patchManagedFields(stored, content *ast.StructLit, order []string) {
	rank := make(map[string]int, len(order))
	for i, name := range order {
		rank[name] = i
	}
	wanted := make(map[string]ast.Expr, len(content.Elts))
	for _, elt := range content.Elts {
		if field, ok := elt.(*ast.Field); ok {
			if name, _, err := ast.LabelName(field.Label); err == nil {
				wanted[name] = field.Value
			}
		}
	}

	placed := make(map[string]bool, len(wanted))
	kept := make([]ast.Decl, 0, len(stored.Elts)+len(wanted))
	for _, elt := range stored.Elts {
		name, managed := managedFieldName(elt, rank)
		if !managed {
			kept = append(kept, elt)
			continue
		}
		expr, ok := wanted[name]
		if !ok || placed[name] {
			continue
		}
		field := elt.(*ast.Field)
		field.Value = expr
		// A ? or ! marker would make the written value a constraint rather than the value.
		field.Constraint = token.ILLEGAL
		placed[name] = true
		kept = append(kept, elt)
	}

	for _, name := range order {
		expr, ok := wanted[name]
		if !ok || placed[name] {
			continue
		}
		at := 0
		for i, elt := range kept {
			if other, managed := managedFieldName(elt, rank); managed && rank[other] < rank[name] {
				at = i + 1
			}
		}
		field := &ast.Field{Label: ast.NewIdent(name), Value: expr}
		ast.SetRelPos(field, token.Newline)
		kept = append(kept[:at], append([]ast.Decl{field}, kept[at:]...)...)
		placed[name] = true
	}
	if !stored.Lbrace.IsValid() && len(kept) > 1 {
		// A short-form entry (name: field: value) needs braces to hold a second field.
		stored.Lbrace, stored.Rbrace = token.Blank.Pos(), token.Newline.Pos()
		for _, elt := range kept {
			ast.SetRelPos(elt, token.Newline)
		}
	}
	stored.Elts = kept
}

// managedFieldName returns elt's label when elt is a field named in rank.
func managedFieldName(elt ast.Decl, rank map[string]int) (string, bool) {
	field, ok := elt.(*ast.Field)
	if !ok {
		return "", false
	}
	name, _, err := ast.LabelName(field.Label)
	if err != nil {
		return "", false
	}
	_, managed := rank[name]
	return name, managed
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
