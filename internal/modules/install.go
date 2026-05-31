package modules

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/mod/modconfig"
	"cuelang.org/go/mod/modfile"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/registry"
)

// InstallModule installs a module from the registry to the config directory.
// The index parameter enables role dependency resolution; pass nil to skip it.
// Returns the resolved version installed (e.g., "v0.1.4"), empty if unparseable.
func InstallModule(ctx context.Context, client registry.Client, index *registry.Index, selected SearchResult, configDir string) (string, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	modulePath := selected.Entry.Module
	if !strings.Contains(modulePath, "@") {
		modulePath += "@v0"
	}

	resolvedPath, err := client.ResolveLatestVersion(ctx, modulePath)
	if err != nil {
		return "", fmt.Errorf("resolving module version: %w", err)
	}

	fetchResult, err := client.Fetch(ctx, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("fetching module: %w", err)
	}

	// Role deps must be installed before extracting content so the task can reference by name.
	var roleName string
	if selected.Category == "tasks" && index != nil {
		roleName, err = InstallRoleDependency(ctx, client, index, fetchResult.SourceDir, configDir)
		if err != nil {
			return "", fmt.Errorf("installing role dependency: %w", err)
		}
	}

	moduleContent, err := ExtractModuleContent(fetchResult.SourceDir, selected, client.Registry(), resolvedPath, roleName)
	if err != nil {
		return "", fmt.Errorf("extracting module content: %w", err)
	}

	configFile, ok := internalcue.ConfigFiles[selected.Category]
	if !ok {
		configFile = internalcue.ConfigFiles[internalcue.KeySettings]
	}
	configPath := filepath.Join(configDir, configFile)

	if err := writeModuleToConfig(configPath, selected, moduleContent, modulePath); err != nil {
		return "", fmt.Errorf("writing config: %w", err)
	}

	return VersionFromOrigin(resolvedPath), nil
}

// InstallRoleDependency checks if a task module has a role dependency and installs it
// as a separate module if not already present. Returns the role name, or empty if none.
func InstallRoleDependency(ctx context.Context, client registry.Client, index *registry.Index, moduleDir, configDir string) (string, error) {
	depPath := findRoleDependency(moduleDir)
	if depPath == "" {
		return "", nil
	}

	roleName, roleEntry, found := ResolveRoleName(index, depPath)
	if !found {
		// Dep absent from index: fall back to the task's inline role struct.
		return "", nil
	}

	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(configDir)
	if err == nil && ModuleExists(cfg, "roles", roleName) {
		return roleName, nil
	}

	roleResult := SearchResult{
		Category: "roles",
		Name:     roleName,
		Entry:    roleEntry,
	}

	if _, err := InstallModule(ctx, client, nil, roleResult, configDir); err != nil {
		return "", fmt.Errorf("role %q: %w", roleName, err)
	}

	return roleName, nil
}

// ModuleExists checks if a module with the given name exists in a loaded CUE config.
func ModuleExists(cfg cue.Value, category, name string) bool {
	return cfg.LookupPath(cue.ParsePath(category)).
		LookupPath(cue.MakePath(cue.Str(name))).Exists()
}

// ExtractModuleContent loads the module and extracts its content as a CUE AST struct.
// originPath is stored in the origin field; roleName, if non-empty, replaces an inline
// role struct with a string reference.
func ExtractModuleContent(moduleDir string, module SearchResult, reg any, originPath, roleName string) (*ast.StructLit, error) {
	cctx := cuecontext.New()

	cfg := &load.Config{
		Dir: moduleDir,
	}

	if regVal, ok := reg.(modconfig.Registry); ok {
		cfg.Registry = regVal
	}

	insts := load.Instances([]string{"."}, cfg)
	if len(insts) == 0 {
		return nil, fmt.Errorf("no CUE instances found in %s", moduleDir)
	}

	inst := insts[0]
	if inst.Err != nil {
		return nil, fmt.Errorf("loading module: %w", inst.Err)
	}

	v := cctx.BuildInstance(inst)
	if err := v.Err(); err != nil {
		return nil, fmt.Errorf("building module: %w", err)
	}

	// Try singular field name (task/role/agent/context) first, then module name as key.
	singular := strings.TrimSuffix(module.Category, "s")
	moduleVal := v.LookupPath(cue.ParsePath(singular))
	if !moduleVal.Exists() {
		moduleVal = v.LookupPath(cue.MakePath(cue.Str(module.Name)))
	}
	if !moduleVal.Exists() {
		return nil, fmt.Errorf("module definition not found in module (tried %q)", singular)
	}

	return formatModuleStruct(moduleVal, module.Category, originPath, roleName)
}

// formatModuleStruct builds a CUE AST struct from a CUE value.
// originPath is written as the origin field; roleName, if non-empty, replaces an
// inline role struct with a string reference.
func formatModuleStruct(v cue.Value, category, originPath, roleName string) (*ast.StructLit, error) {
	s := &ast.StructLit{}

	s.Elts = append(s.Elts, &ast.Field{
		Label: ast.NewIdent("origin"),
		Value: ast.NewString(originPath),
	})

	var fields []string
	switch category {
	case "tasks":
		fields = []string{"description", "tags", "role", "file", "command", "prompt"}
	case "roles":
		fields = []string{"description", "tags", "file", "command", "prompt", "optional"}
	case "agents":
		fields = []string{"description", "tags", "bin", "command", "default_model", "models"}
	case "contexts":
		fields = []string{"description", "tags", "file", "command", "prompt", "required", "default"}
	default:
		fields = []string{"description", "tags", "prompt"}
	}

	for _, field := range fields {
		if field == "role" && roleName != "" {
			s.Elts = append(s.Elts, &ast.Field{
				Label: ast.NewIdent("role"),
				Value: ast.NewString(roleName),
			})
			continue
		}

		fieldVal := v.LookupPath(cue.ParsePath(field))
		if !fieldVal.Exists() {
			continue
		}

		expr, err := formatFieldExpr(fieldVal)
		if err != nil {
			return nil, fmt.Errorf("formatting field %q: %w", field, err)
		}
		s.Elts = append(s.Elts, &ast.Field{
			Label: ast.NewIdent(field),
			Value: expr,
		})
	}

	return s, nil
}

// formatFieldExpr converts a CUE value into an AST expression node.
func formatFieldExpr(v cue.Value) (ast.Expr, error) {
	switch v.Kind() {
	case cue.StringKind:
		s, err := v.String()
		if err != nil {
			return nil, err
		}
		return ast.NewString(s), nil

	case cue.BoolKind:
		b, err := v.Bool()
		if err != nil {
			return nil, err
		}
		return ast.NewBool(b), nil

	case cue.ListKind:
		iter, err := v.List()
		if err != nil {
			return nil, err
		}
		var items []ast.Expr
		for iter.Next() {
			item, err := formatFieldExpr(iter.Value())
			if err != nil {
				return nil, fmt.Errorf("list element: %w", err)
			}
			items = append(items, item)
		}
		return ast.NewList(items...), nil

	case cue.StructKind:
		iter, err := v.Fields()
		if err != nil {
			return nil, err
		}
		inner := &ast.StructLit{}
		for iter.Next() {
			key := iter.Selector().Unquoted()
			val, err := formatFieldExpr(iter.Value())
			if err != nil {
				return nil, fmt.Errorf("struct field %q: %w", key, err)
			}
			inner.Elts = append(inner.Elts, &ast.Field{
				Label: ast.NewStringLabel(key),
				Value: val,
			})
		}
		return inner, nil

	default:
		syn := v.Syntax()
		if expr, ok := syn.(ast.Expr); ok {
			return expr, nil
		}
		return nil, fmt.Errorf("unsupported value kind: %v", v.Kind())
	}
}

// findRoleDependency reads a task module's cue.mod/module.cue and returns the
// role dependency module path, or empty if none.
func findRoleDependency(moduleDir string) string {
	moduleFile := filepath.Join(moduleDir, "cue.mod", "module.cue")
	data, err := os.ReadFile(moduleFile)
	if err != nil {
		return ""
	}

	f, err := modfile.Parse(data, moduleFile)
	if err != nil {
		return ""
	}

	// Sort so the alphabetically first match wins deterministically across multiple role deps.
	var depPaths []string
	for path := range f.Deps {
		depPaths = append(depPaths, path)
	}
	sort.Strings(depPaths)

	for _, path := range depPaths {
		if strings.Contains(path, "/roles/") {
			return path
		}
	}

	return ""
}

// ResolveRoleName finds a role's module name in the index by matching depPath
// against index entries' Module field.
func ResolveRoleName(index *registry.Index, depPath string) (name string, entry registry.IndexEntry, found bool) {
	if index == nil {
		return "", registry.IndexEntry{}, false
	}

	for roleName, roleEntry := range index.Roles {
		if roleEntry.Module == depPath {
			return roleName, roleEntry, true
		}
	}

	return "", registry.IndexEntry{}, false
}

// GetInstalledOrigin returns the origin field value for the named module in a
// loaded CUE config, or empty if absent.
func GetInstalledOrigin(cfg cue.Value, category, name string) string {
	originVal := cfg.LookupPath(cue.ParsePath(category)).
		LookupPath(cue.MakePath(cue.Str(name))).
		LookupPath(cue.ParsePath("origin"))
	if !originVal.Exists() {
		return ""
	}
	s, _ := originVal.String()
	return s
}

// VersionFromOrigin extracts the version string from an origin path
// (e.g. "github.com/test/module@v0.1.1" returns "v0.1.1"), or empty if none.
func VersionFromOrigin(origin string) string {
	if idx := strings.LastIndex(origin, "@"); idx != -1 {
		return origin[idx+1:]
	}
	return ""
}

// ModuleFromOrigin extracts the module path from an origin path
// (e.g. "github.com/test/module@v0.1.1" returns "github.com/test/module"),
// or the input unchanged if no version separator is found.
func ModuleFromOrigin(origin string) string {
	if idx := strings.LastIndex(origin, "@"); idx != -1 {
		return origin[:idx]
	}
	return origin
}

// writeModuleToConfig upserts the module content into the config file.
func writeModuleToConfig(configPath string, module SearchResult, content ast.Expr, modulePath string) error {
	var file *ast.File
	if data, err := os.ReadFile(configPath); err == nil && len(data) > 0 {
		file, err = parser.ParseFile(configPath, data, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parsing config file: %w", err)
		}
	}

	moduleField := &ast.Field{
		Label: ast.NewStringLabel(module.Name),
		Value: content,
	}

	if file == nil {
		categoryStruct := &ast.StructLit{
			Elts: []ast.Decl{moduleField},
		}
		categoryField := &ast.Field{
			Label: ast.NewIdent(module.Category),
			Value: categoryStruct,
		}
		ast.AddComment(categoryField, &ast.CommentGroup{
			Doc: true,
			List: []*ast.Comment{
				{Text: "// start configuration"},
				{Text: "// Managed by 'start install'"},
			},
		})
		file = &ast.File{Decls: []ast.Decl{categoryField}}
	} else {
		catField := findCategoryField(file, module.Category)
		if catField != nil {
			catStruct, ok := catField.Value.(*ast.StructLit)
			if !ok {
				return fmt.Errorf("category %q is not a struct", module.Category)
			}
			if existing := findModuleField(catStruct, module.Name); existing != nil {
				existing.Value = content
			} else {
				catStruct.Elts = append(catStruct.Elts, moduleField)
			}
		} else {
			categoryStruct := &ast.StructLit{
				Elts: []ast.Decl{moduleField},
			}
			categoryField := &ast.Field{
				Label: ast.NewIdent(module.Category),
				Value: categoryStruct,
			}
			file.Decls = append(file.Decls, categoryField)
		}
	}

	formatted, err := format.Node(file, format.Simplify())
	if err != nil {
		return fmt.Errorf("formatting config: %w", err)
	}
	return os.WriteFile(configPath, formatted, 0644)
}

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

// UpdateModuleInConfig replaces an existing module entry in the config file.
func UpdateModuleInConfig(configPath, category, name string, newContent ast.Expr) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	file, err := parser.ParseFile(configPath, data, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	catField := findCategoryField(file, category)
	if catField == nil {
		return fmt.Errorf("module %q not found in config", name)
	}

	catStruct, ok := catField.Value.(*ast.StructLit)
	if !ok {
		return fmt.Errorf("module %q not found in config", name)
	}

	moduleField := findModuleField(catStruct, name)
	if moduleField == nil {
		return fmt.Errorf("module %q not found in config", name)
	}

	moduleField.Value = newContent

	formatted, err := format.Node(file, format.Simplify())
	if err != nil {
		return fmt.Errorf("formatting config: %w", err)
	}
	return os.WriteFile(configPath, formatted, 0644)
}
