package assets

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

// InstallAsset installs an asset from the registry to the config directory.
// It fetches the asset module, extracts the content, and writes it to the appropriate config file.
// For tasks with role dependencies, the role is installed as a separate asset and the task
// references it by name. The index parameter enables role dependency resolution; pass nil
// to skip role dependency handling.
// If the asset is already installed, it is updated in place.
// Returns the resolved version that was installed (e.g., "v0.1.4"); empty if no version
// could be parsed from the resolved module path.
func InstallAsset(ctx context.Context, client *registry.Client, index *registry.Index, selected SearchResult, configDir string) (string, error) {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	// Fetch the actual asset module from registry
	modulePath := selected.Entry.Module
	if !strings.Contains(modulePath, "@") {
		modulePath += "@v0"
	}

	// Resolve to canonical version (e.g., @v0 -> @v0.0.1)
	resolvedPath, err := client.ResolveLatestVersion(ctx, modulePath)
	if err != nil {
		return "", fmt.Errorf("resolving asset version: %w", err)
	}

	fetchResult, err := client.Fetch(ctx, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("fetching asset module: %w", err)
	}

	// For tasks, detect and install role dependencies before extracting content.
	// The role is installed as a separate asset, and the task references it by name.
	var roleName string
	if selected.Category == "tasks" && index != nil {
		roleName, err = InstallRoleDependency(ctx, client, index, fetchResult.SourceDir, configDir)
		if err != nil {
			return "", fmt.Errorf("installing role dependency: %w", err)
		}
	}

	// Extract asset content from fetched module
	// Use resolved path with version for origin field (e.g., "github.com/.../task@v0.1.1")
	assetContent, err := ExtractAssetContent(fetchResult.SourceDir, selected, client.Registry(), resolvedPath, roleName)
	if err != nil {
		return "", fmt.Errorf("extracting asset content: %w", err)
	}

	// Determine the config file to write to based on asset type
	configFile := assetTypeToConfigFile(selected.Category)
	configPath := filepath.Join(configDir, configFile)

	// Write the asset to config
	if err := writeAssetToConfig(configPath, selected, assetContent, modulePath); err != nil {
		return "", fmt.Errorf("writing config: %w", err)
	}

	return VersionFromOrigin(resolvedPath), nil
}

// InstallRoleDependency checks if a task module has a role dependency and installs it
// as a separate asset if not already present. Returns the role name for use as a string
// reference in the task config, or empty string if no role dependency exists.
func InstallRoleDependency(ctx context.Context, client *registry.Client, index *registry.Index, moduleDir, configDir string) (string, error) {
	depPath := findRoleDependency(moduleDir)
	if depPath == "" {
		return "", nil
	}

	roleName, roleEntry, found := ResolveRoleName(index, depPath)
	if !found {
		// Role dependency exists in module but isn't in the index.
		// Fall back silently: the task keeps its inline role struct.
		return "", nil
	}

	// Skip if the role is already installed
	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(configDir)
	if err == nil && AssetExists(cfg, "roles", roleName) {
		return roleName, nil
	}

	// Install the role as a separate asset
	roleResult := SearchResult{
		Category: "roles",
		Name:     roleName,
		Entry:    roleEntry,
	}

	if _, err := InstallAsset(ctx, client, nil, roleResult, configDir); err != nil {
		return "", fmt.Errorf("role %q: %w", roleName, err)
	}

	return roleName, nil
}

// AssetExists checks if an asset with the given name exists in a loaded CUE config.
// Returns true if the asset is found, false otherwise.
func AssetExists(cfg cue.Value, category, name string) bool {
	return cfg.LookupPath(cue.ParsePath(category)).
		LookupPath(cue.MakePath(cue.Str(name))).Exists()
}

// assetTypeToConfigFile returns the config file name for an asset type.
func assetTypeToConfigFile(category string) string {
	if f, ok := internalcue.ConfigFiles[category]; ok {
		return f
	}
	return internalcue.ConfigFiles[internalcue.KeySettings]
}

// ExtractAssetContent loads the asset module and extracts its content as a CUE AST struct.
// originPath is the module path (without version) to store in the origin field.
// roleName, if non-empty, replaces an inline role struct with a string reference.
func ExtractAssetContent(moduleDir string, asset SearchResult, reg any, originPath, roleName string) (*ast.StructLit, error) {
	cctx := cuecontext.New()

	cfg := &load.Config{
		Dir: moduleDir,
	}

	// Add registry if provided
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

	// Extract the asset definition - try singular field name first
	// e.g., "task", "role", "agent", "context"
	singular := strings.TrimSuffix(asset.Category, "s")
	assetVal := v.LookupPath(cue.ParsePath(singular))
	if !assetVal.Exists() {
		// Try the asset name as key
		assetVal = v.LookupPath(cue.MakePath(cue.Str(asset.Name)))
	}
	if !assetVal.Exists() {
		return nil, fmt.Errorf("asset definition not found in module (tried %q)", singular)
	}

	// Build a concrete struct with just the fields we need
	return formatAssetStruct(assetVal, asset.Category, originPath, roleName)
}

// formatAssetStruct builds a CUE AST struct from a CUE value.
// originPath is written as the origin field to track registry provenance.
// roleName, if non-empty, replaces an inline role struct with a string reference.
func formatAssetStruct(v cue.Value, category, originPath, roleName string) (*ast.StructLit, error) {
	s := &ast.StructLit{}

	// Origin field first (tracks registry provenance)
	s.Elts = append(s.Elts, &ast.Field{
		Label: ast.NewIdent("origin"),
		Value: ast.NewString(originPath),
	})

	// Define which fields to extract based on category
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
		// Replace inline role struct with string reference when a role name is available
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

// findRoleDependency reads a task module's cue.mod/module.cue and returns
// the role dependency module path, if one exists.
// Returns an empty string if the module has no role dependency.
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

	// Sort dependency paths for deterministic selection when multiple role
	// dependencies exist. The alphabetically first match is chosen.
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

// ResolveRoleName finds a role's asset name in the index by matching its module path.
// The depPath is the dependency key (e.g., "github.com/.../roles/golang/agent@v0")
// which is matched against index entries' Module field.
// Returns the role name, its index entry, and whether a match was found.
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

// GetInstalledOrigin returns the origin field value for the named asset
// in a loaded CUE config. Returns an empty string if the asset is not found
// or has no origin field.
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

// VersionFromOrigin extracts the version string from an origin path.
// For example, "github.com/test/asset@v0.1.1" returns "v0.1.1".
// Returns an empty string if no version is found.
func VersionFromOrigin(origin string) string {
	if idx := strings.LastIndex(origin, "@"); idx != -1 {
		return origin[idx+1:]
	}
	return ""
}

// ModuleFromOrigin extracts the module path from an origin path.
// For example, "github.com/test/asset@v0.1.1" returns "github.com/test/asset".
// Returns the input unchanged if no version separator is found.
func ModuleFromOrigin(origin string) string {
	if idx := strings.LastIndex(origin, "@"); idx != -1 {
		return origin[:idx]
	}
	return origin
}

// writeAssetToConfig writes the asset content to the config file.
// If the asset already exists, it is updated in place (upsert).
func writeAssetToConfig(configPath string, asset SearchResult, content ast.Expr, modulePath string) error {
	// Read existing content if file exists
	var file *ast.File
	if data, err := os.ReadFile(configPath); err == nil && len(data) > 0 {
		file, err = parser.ParseFile(configPath, data, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parsing config file: %w", err)
		}
	}

	assetField := &ast.Field{
		Label: ast.NewStringLabel(asset.Name),
		Value: content,
	}

	if file == nil {
		// New file: build with comment header and category struct
		categoryStruct := &ast.StructLit{
			Elts: []ast.Decl{assetField},
		}
		categoryField := &ast.Field{
			Label: ast.NewIdent(asset.Category),
			Value: categoryStruct,
		}
		ast.AddComment(categoryField, &ast.CommentGroup{
			Doc: true,
			List: []*ast.Comment{
				{Text: "// start configuration"},
				{Text: "// Managed by 'start assets add'"},
			},
		})
		file = &ast.File{Decls: []ast.Decl{categoryField}}
	} else {
		// Find or create category
		catField := findCategoryField(file, asset.Category)
		if catField != nil {
			catStruct, ok := catField.Value.(*ast.StructLit)
			if !ok {
				return fmt.Errorf("category %q is not a struct", asset.Category)
			}
			// Upsert: update if exists, append if not
			if existing := findAssetField(catStruct, asset.Name); existing != nil {
				existing.Value = content
			} else {
				catStruct.Elts = append(catStruct.Elts, assetField)
			}
		} else {
			// New category
			categoryStruct := &ast.StructLit{
				Elts: []ast.Decl{assetField},
			}
			categoryField := &ast.Field{
				Label: ast.NewIdent(asset.Category),
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

// findCategoryField finds a top-level field in a CUE file by name.
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

// findAssetField finds a field in a struct by name.
func findAssetField(s *ast.StructLit, name string) *ast.Field {
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

// UpdateAssetInConfig replaces an existing asset entry in the config file.
func UpdateAssetInConfig(configPath, category, name string, newContent ast.Expr) error {
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
		return fmt.Errorf("asset %q not found in config", name)
	}

	catStruct, ok := catField.Value.(*ast.StructLit)
	if !ok {
		return fmt.Errorf("asset %q not found in config", name)
	}

	assetField := findAssetField(catStruct, name)
	if assetField == nil {
		return fmt.Errorf("asset %q not found in config", name)
	}

	assetField.Value = newContent

	formatted, err := format.Node(file, format.Simplify())
	if err != nil {
		return fmt.Errorf("formatting config: %w", err)
	}
	return os.WriteFile(configPath, formatted, 0644)
}
