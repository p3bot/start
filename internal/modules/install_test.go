package modules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/registry"
)

// formatAST formats an AST node to a string for test assertions.
func formatAST(t *testing.T, node ast.Node) string {
	t.Helper()
	b, err := format.Node(node, format.Simplify())
	if err != nil {
		t.Fatalf("formatting AST: %v", err)
	}
	return string(b)
}

// parseCUEStruct parses a CUE struct literal string into an ast.Expr for test input.
func parseCUEStruct(t *testing.T, src string) ast.Expr {
	t.Helper()
	f, err := parser.ParseFile("test", "a: "+src)
	if err != nil {
		t.Fatalf("parsing CUE struct: %v", err)
	}
	return f.Decls[0].(*ast.Field).Value
}

// TestModuleExists tests the ModuleExists function.
func TestModuleExists(t *testing.T) {
	t.Parallel()

	// Create a temporary config directory
	configDir := t.TempDir()

	// Write a contexts.cue file with an existing module
	contextsFile := filepath.Join(configDir, "contexts.cue")
	existingContent := `// start configuration
contexts: {
	"cwd/agents-md": {
		origin: "github.com/test/contexts/cwd/agents-md@v0.1.0"
		description: "Read AGENTS.md file"
		file: "AGENTS.md"
		required: true
		default: true
	}
}
`
	if err := os.WriteFile(contextsFile, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Load config via CUE
	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(configDir)
	if err != nil {
		t.Fatalf("Failed to load CUE config: %v", err)
	}

	tests := []struct {
		name       string
		category   string
		moduleName string
		want       bool
	}{
		{
			name:       "existing module with quotes",
			category:   "contexts",
			moduleName: "cwd/agents-md",
			want:       true,
		},
		{
			name:       "non-existent module",
			category:   "contexts",
			moduleName: "cwd/other",
			want:       false,
		},
		{
			name:       "different category not found",
			category:   "roles",
			moduleName: "cwd/agents-md",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ModuleExists(cfg, tt.category, tt.moduleName)
			if got != tt.want {
				t.Errorf("ModuleExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestWriteModuleToConfig_NewCategory tests adding a module with a category
// that doesn't exist yet in an existing file.
func TestWriteModuleToConfig_NewCategory(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tasks.cue")

	existingContent := `// start configuration
contexts: {
	"existing": {
		origin: "test"
	}
}
`
	if err := os.WriteFile(configPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	module := SearchResult{
		Category: "tasks",
		Name:     "new/task",
		Entry:    registry.IndexEntry{Module: "github.com/test/tasks/new/task@v0"},
	}
	moduleContent := parseCUEStruct(t, `{
	origin: "github.com/test/tasks/new/task@v0.1.0"
	description: "A new task"
}`)

	err := writeModuleToConfig(configPath, module, moduleContent, module.Entry.Module)
	if err != nil {
		t.Fatalf("writeModuleToConfig() error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read result: %v", err)
	}

	result := string(data)
	// Should preserve the existing contexts block
	if !strings.Contains(result, "contexts:") {
		t.Error("result missing existing contexts block")
	}
	if !strings.Contains(result, "existing:") {
		t.Error("result missing existing module")
	}
	// Should have the new tasks block
	if !strings.Contains(result, "tasks:") {
		t.Error("result missing new tasks category")
	}
	if !strings.Contains(result, `"new/task":`) {
		t.Error("result missing new task module")
	}
}

// TestUpdateModuleInConfig tests the UpdateModuleInConfig function.
func TestUpdateModuleInConfig(t *testing.T) {
	t.Parallel()

	// Create a temporary directory
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "contexts.cue")

	// Write initial config
	initialContent := `// start configuration
contexts: {
	"cwd/agents-md": {
		origin: "github.com/test/contexts/cwd/agents-md@v0.1.0"
		description: "Old description"
		file: "AGENTS.md"
	}
}
`
	if err := os.WriteFile(configPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	newContent := parseCUEStruct(t, `{
	origin: "github.com/test/contexts/cwd/agents-md@v0.2.0"
	description: "New description"
	file: "AGENTS.md"
	required: true
}`)

	// Update the module
	err := UpdateModuleInConfig(configPath, "contexts", "cwd/agents-md", newContent)
	if err != nil {
		t.Fatalf("UpdateModuleInConfig() error: %v", err)
	}

	// Read back and verify
	updatedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	updatedContent := string(updatedData)

	// Verify new content is present
	if !strings.Contains(updatedContent, "v0.2.0") {
		t.Error("Updated config missing new version")
	}
	if !strings.Contains(updatedContent, "New description") {
		t.Error("Updated config missing new description")
	}
	if !strings.Contains(updatedContent, "required:") || !strings.Contains(updatedContent, "true") {
		t.Error("Updated config missing new field")
	}

	// Verify old content is gone
	if strings.Contains(updatedContent, "v0.1.0") {
		t.Error("Updated config still contains old version")
	}
	if strings.Contains(updatedContent, "Old description") {
		t.Error("Updated config still contains old description")
	}
}

// TestUpdateModuleInConfig_CategoryNotFound tests that updating a module
// in a non-existent category returns an error.
func TestUpdateModuleInConfig_CategoryNotFound(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(t.TempDir(), "contexts.cue")
	content := `contexts: {
	"existing": {
		origin: "test"
	}
}
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	newContent := parseCUEStruct(t, `{origin: "new"}`)
	err := UpdateModuleInConfig(configPath, "roles", "existing", newContent)
	if err == nil {
		t.Fatal("expected error for non-existent category, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// TestWriteModuleToConfig_RoundTrip verifies that written config files have
// correct CUE structure by parsing the output back and checking paths.
func TestWriteModuleToConfig_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := cuecontext.New()
	configPath := filepath.Join(t.TempDir(), "contexts.cue")

	// Write first module to new file
	first := SearchResult{
		Category: "contexts",
		Name:     "cwd/agents-md",
		Entry:    registry.IndexEntry{Module: "github.com/test/contexts/cwd/agents-md@v0"},
	}
	firstContent := parseCUEStruct(t, `{
	origin: "github.com/test/contexts/cwd/agents-md@v0.1.0"
	description: "AGENTS.md context"
	tags: ["agents", "cwd"]
	default: true
}`)
	if err := writeModuleToConfig(configPath, first, firstContent, first.Entry.Module); err != nil {
		t.Fatalf("first write: %v", err)
	}

	// Write second module to same file
	second := SearchResult{
		Category: "contexts",
		Name:     "cwd/project",
		Entry:    registry.IndexEntry{Module: "github.com/test/contexts/cwd/project@v0"},
	}
	secondContent := parseCUEStruct(t, `{
	origin: "github.com/test/contexts/cwd/project@v0.2.0"
	description: "Project context"
	required: false
}`)
	if err := writeModuleToConfig(configPath, second, secondContent, second.Entry.Module); err != nil {
		t.Fatalf("second write: %v", err)
	}

	// Parse back with CUE and verify structure
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading result: %v", err)
	}
	v := ctx.CompileBytes(data)
	if v.Err() != nil {
		t.Fatalf("output is not valid CUE: %v", v.Err())
	}

	// Verify first module at correct path
	origin1, err := v.LookupPath(cue.ParsePath(`contexts."cwd/agents-md".origin`)).String()
	if err != nil {
		t.Fatalf("looking up first module origin: %v", err)
	}
	if origin1 != "github.com/test/contexts/cwd/agents-md@v0.1.0" {
		t.Errorf("first module origin = %q, want v0.1.0 path", origin1)
	}

	desc1, _ := v.LookupPath(cue.ParsePath(`contexts."cwd/agents-md".description`)).String()
	if desc1 != "AGENTS.md context" {
		t.Errorf("first module description = %q", desc1)
	}

	default1, _ := v.LookupPath(cue.ParsePath(`contexts."cwd/agents-md".default`)).Bool()
	if !default1 {
		t.Error("first module default should be true")
	}

	// Verify second module at correct path
	origin2, err := v.LookupPath(cue.ParsePath(`contexts."cwd/project".origin`)).String()
	if err != nil {
		t.Fatalf("looking up second module origin: %v", err)
	}
	if origin2 != "github.com/test/contexts/cwd/project@v0.2.0" {
		t.Errorf("second module origin = %q, want v0.2.0 path", origin2)
	}

	required2, _ := v.LookupPath(cue.ParsePath(`contexts."cwd/project".required`)).Bool()
	if required2 {
		t.Error("second module required should be false")
	}

	// Verify tags list round-trips correctly
	tagsVal := v.LookupPath(cue.ParsePath(`contexts."cwd/agents-md".tags`))
	iter, err := tagsVal.List()
	if err != nil {
		t.Fatalf("listing tags: %v", err)
	}
	var tags []string
	for iter.Next() {
		s, _ := iter.Value().String()
		tags = append(tags, s)
	}
	if len(tags) != 2 || tags[0] != "agents" || tags[1] != "cwd" {
		t.Errorf("tags = %v, want [agents cwd]", tags)
	}
}

// TestWriteModuleToConfig tests the writeModuleToConfig function.
func TestWriteModuleToConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	tests := []struct {
		name            string
		existingFile    string
		existingContent string
		module          SearchResult
		moduleContent   string
		wantErr         bool
		wantContains    []string
		wantExcludes    []string
	}{
		{
			name:         "new file",
			existingFile: "",
			module: SearchResult{
				Category: "contexts",
				Name:     "cwd/agents-md",
				Entry: registry.IndexEntry{
					Module: "github.com/test/contexts/cwd/agents-md@v0",
				},
			},
			moduleContent: `{
	origin: "github.com/test/contexts/cwd/agents-md@v0.1.0"
	description: "Test context"
	file: "AGENTS.md"
}`,
			wantErr: false,
			wantContains: []string{
				"// start configuration",
				"// Managed by 'start modules add'",
				"contexts:",
				`"cwd/agents-md":`,
				"origin:",
				"v0.1.0",
			},
		},
		{
			name:         "append to existing file",
			existingFile: "contexts.cue",
			existingContent: `// start configuration
// Managed by 'start modules add'
contexts: {
	"other": {
		origin: "test"
	}
}
`,
			module: SearchResult{
				Category: "contexts",
				Name:     "cwd/agents-md",
				Entry: registry.IndexEntry{
					Module: "github.com/test/contexts/cwd/agents-md@v0",
				},
			},
			moduleContent: `{
	origin: "github.com/test/contexts/cwd/agents-md@v0.1.0"
	description: "Test context"
}`,
			wantErr: false,
			wantContains: []string{
				"// start configuration",
				"// Managed by 'start modules add'",
				"contexts:",
				"other:",
				`"cwd/agents-md":`,
				"v0.1.0",
			},
		},
		{
			name:            "empty existing file",
			existingFile:    "contexts.cue",
			existingContent: "",
			module: SearchResult{
				Category: "contexts",
				Name:     "cwd/agents-md",
				Entry: registry.IndexEntry{
					Module: "github.com/test/contexts/cwd/agents-md@v0",
				},
			},
			moduleContent: `{
	origin: "github.com/test/contexts/cwd/agents-md@v0.1.0"
	description: "Test context"
}`,
			wantErr: false,
			wantContains: []string{
				"// start configuration",
				"// Managed by 'start modules add'",
				"contexts:",
				`"cwd/agents-md":`,
				"v0.1.0",
			},
		},
		{
			name:         "duplicate module updates in place",
			existingFile: "contexts.cue",
			existingContent: `contexts: {
	"cwd/agents-md": {
		origin: "old-origin"
		description: "Old description"
	}
}
`,
			module: SearchResult{
				Category: "contexts",
				Name:     "cwd/agents-md",
			},
			moduleContent: `{
	origin: "new-origin"
	description: "New description"
}`,
			wantErr: false,
			wantContains: []string{
				"contexts:",
				`"cwd/agents-md":`,
				"new-origin",
				"New description",
			},
			wantExcludes: []string{
				"old-origin",
				"Old description",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configPath string
			if tt.existingFile != "" {
				configPath = filepath.Join(tempDir, tt.name, tt.existingFile)
				if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				if err := os.WriteFile(configPath, []byte(tt.existingContent), 0644); err != nil {
					t.Fatalf("Failed to write existing file: %v", err)
				}
			} else {
				configFileName, ok := internalcue.ConfigFiles[tt.module.Category]
				if !ok {
					configFileName = internalcue.ConfigFiles[internalcue.KeySettings]
				}
				configPath = filepath.Join(tempDir, tt.name, configFileName)
				if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
			}

			content := parseCUEStruct(t, tt.moduleContent)
			err := writeModuleToConfig(configPath, tt.module, content, tt.module.Entry.Module)

			if tt.wantErr {
				if err == nil {
					t.Error("writeModuleToConfig() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("writeModuleToConfig() unexpected error: %v", err)
				return
			}

			// Read back and verify
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatalf("Failed to read config file: %v", err)
			}

			result := string(data)
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("writeModuleToConfig() result missing %q\nGot:\n%s", want, result)
				}
			}
			for _, exclude := range tt.wantExcludes {
				if strings.Contains(result, exclude) {
					t.Errorf("writeModuleToConfig() result should not contain %q\nGot:\n%s", exclude, result)
				}
			}
		})
	}
}

// TestWriteModuleToConfig_BracesInStringValues verifies that modules are inserted
// into the correct category block when multiple top-level categories exist.
func TestWriteModuleToConfig_BracesInStringValues(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "contexts.cue")
	existingContent := `// start configuration
contexts: {
	"existing": {
		origin: "test"
		description: "An existing context"
	}
}

settings: {
	default_agent: "claude"
}
`
	if err := os.WriteFile(configPath, []byte(existingContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	module := SearchResult{
		Category: "contexts",
		Name:     "new-module",
		Entry: registry.IndexEntry{
			Module: "github.com/test/contexts/new-module@v0",
		},
	}
	moduleContent := parseCUEStruct(t, `{
	origin: "github.com/test/contexts/new-module@v0.1.0"
	description: "New module"
}`)

	err := writeModuleToConfig(configPath, module, moduleContent, module.Entry.Module)
	if err != nil {
		t.Fatalf("writeModuleToConfig() error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	result := string(data)

	// The result should contain both the existing and new modules
	if !strings.Contains(result, "existing:") {
		t.Error("result missing existing module")
	}
	if !strings.Contains(result, `"new-module":`) {
		t.Error("result missing new module")
	}

	// Verify the new module appears inside contexts: {} block, not settings: {} block.
	settingsPos := strings.Index(result, "settings:")
	newModulePos := strings.Index(result, `"new-module":`)
	if settingsPos == -1 || newModulePos == -1 {
		t.Fatal("cannot find settings or new-module in result")
	}
	if newModulePos > settingsPos {
		t.Errorf("BUG: new module was inserted into settings block instead of contexts block\n"+
			"new-module at pos %d, settings at pos %d\nResult:\n%s",
			newModulePos, settingsPos, result)
	}
}

// TestFindRoleDependency tests the findRoleDependency function.
func TestFindRoleDependency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		moduleCue   string
		wantDepPath string
	}{
		{
			name: "task with role dependency",
			moduleCue: `module: "github.com/test/tasks/golang/debug@v0"
language: {
	version: "v0.15.1"
}
deps: {
	"github.com/test/roles/golang/agent@v0": {
		v: "v0.1.1"
	}
	"github.com/test/schemas@v0": {
		v: "v0.1.0"
	}
}
`,
			wantDepPath: "github.com/test/roles/golang/agent@v0",
		},
		{
			name: "task without role dependency",
			moduleCue: `module: "github.com/test/tasks/jira/read-issue@v0"
language: {
	version: "v0.15.1"
}
deps: {
	"github.com/test/schemas@v0": {
		v: "v0.1.0"
	}
}
`,
			wantDepPath: "",
		},
		{
			name:        "missing module.cue",
			moduleCue:   "",
			wantDepPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			moduleDir := t.TempDir()

			if tt.moduleCue != "" {
				cueModDir := filepath.Join(moduleDir, "cue.mod")
				if err := os.MkdirAll(cueModDir, 0755); err != nil {
					t.Fatalf("creating cue.mod dir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(cueModDir, "module.cue"), []byte(tt.moduleCue), 0644); err != nil {
					t.Fatalf("writing module.cue: %v", err)
				}
			}

			gotPath := findRoleDependency(moduleDir)
			if gotPath != tt.wantDepPath {
				t.Errorf("findRoleDependency() depPath = %q, want %q", gotPath, tt.wantDepPath)
			}
		})
	}
}

// TestFindRoleDependency_MultipleRoleDeps verifies that when multiple role deps
// exist, one is still returned (not skipped). Any task-specific role is better
// than falling back to the default role.
func TestFindRoleDependency_MultipleRoleDeps(t *testing.T) {
	t.Parallel()

	moduleDir := t.TempDir()
	cueModDir := filepath.Join(moduleDir, "cue.mod")
	if err := os.MkdirAll(cueModDir, 0755); err != nil {
		t.Fatalf("creating cue.mod dir: %v", err)
	}

	moduleCue := `module: "github.com/test/tasks/multi@v0"
language: {
	version: "v0.15.1"
}
deps: {
	"github.com/test/roles/golang/agent@v0": {
		v: "v0.1.1"
	}
	"github.com/test/roles/golang/assistant@v0": {
		v: "v0.1.0"
	}
	"github.com/test/schemas@v0": {
		v: "v0.1.0"
	}
}
`
	if err := os.WriteFile(filepath.Join(cueModDir, "module.cue"), []byte(moduleCue), 0644); err != nil {
		t.Fatalf("writing module.cue: %v", err)
	}

	gotPath := findRoleDependency(moduleDir)
	// With sorted iteration, the alphabetically first role dep is returned.
	wantPath := "github.com/test/roles/golang/agent@v0"
	if gotPath != wantPath {
		t.Errorf("findRoleDependency() depPath = %q, want %q", gotPath, wantPath)
	}
}

// TestResolveRoleName tests the ResolveRoleName function.
func TestResolveRoleName(t *testing.T) {
	t.Parallel()

	index := &registry.Index{
		Roles: map[string]registry.IndexEntry{
			"golang/agent": {
				Module:      "github.com/test/roles/golang/agent@v0",
				Description: "Go expert",
			},
			"golang/assistant": {
				Module:      "github.com/test/roles/golang/assistant@v0",
				Description: "Go assistant",
			},
		},
	}

	tests := []struct {
		name      string
		index     *registry.Index
		depPath   string
		wantName  string
		wantFound bool
	}{
		{
			name:      "matching role",
			index:     index,
			depPath:   "github.com/test/roles/golang/agent@v0",
			wantName:  "golang/agent",
			wantFound: true,
		},
		{
			name:      "no match",
			index:     index,
			depPath:   "github.com/test/roles/unknown@v0",
			wantName:  "",
			wantFound: false,
		},
		{
			name:      "nil index",
			index:     nil,
			depPath:   "github.com/test/roles/golang/agent@v0",
			wantName:  "",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, _, gotFound := ResolveRoleName(tt.index, tt.depPath)
			if gotName != tt.wantName {
				t.Errorf("ResolveRoleName() name = %q, want %q", gotName, tt.wantName)
			}
			if gotFound != tt.wantFound {
				t.Errorf("ResolveRoleName() found = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

// TestFormatModuleStruct_RoleNameOverride tests that formatModuleStruct replaces
// an inline role struct with a string reference when roleName is provided.
func TestFormatModuleStruct_RoleNameOverride(t *testing.T) {
	t.Parallel()

	// Build a CUE value with a struct role field
	ctx := cuecontext.New()
	v := ctx.CompileString(`{
		description: "Test task"
		tags: ["test"]
		role: {
			description: "A role"
			file: "@module/role.md"
		}
		file: "@module/task.md"
		prompt: "Read {{.file}}"
	}`)
	if v.Err() != nil {
		t.Fatalf("compiling CUE: %v", v.Err())
	}

	tests := []struct {
		name         string
		roleName     string
		wantContains []string
		wantExcludes []string
	}{
		{
			name:     "role name replaces struct",
			roleName: "golang/agent",
			wantContains: []string{
				`"golang/agent"`,
				`"Test task"`,
				`"@module/task.md"`,
			},
			wantExcludes: []string{
				"@module/role.md",
				`"A role"`,
			},
		},
		{
			name:     "empty role name preserves struct",
			roleName: "",
			wantContains: []string{
				"role: {",
				`"A role"`,
				"@module/role.md",
			},
			wantExcludes: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			astResult, err := formatModuleStruct(v, "tasks", "github.com/test@v0.1.0", tt.roleName)
			if err != nil {
				t.Fatalf("formatModuleStruct() error: %v", err)
			}
			result := formatAST(t, astResult)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("result missing %q\nGot:\n%s", want, result)
				}
			}
			for _, exclude := range tt.wantExcludes {
				if strings.Contains(result, exclude) {
					t.Errorf("result should not contain %q\nGot:\n%s", exclude, result)
				}
			}
		})
	}
}

// createTestModule creates a minimal CUE module in a temp directory for testing.
// The module has no external dependencies, making it self-contained.
func createTestModule(t *testing.T, pkgName, cueContent string) string {
	t.Helper()
	moduleDir := t.TempDir()

	// Create cue.mod/module.cue
	modDir := filepath.Join(moduleDir, "cue.mod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatalf("creating cue.mod dir: %v", err)
	}
	moduleCue := `module: "test.example/module@v0"
language: version: "v0.15.1"
`
	if err := os.WriteFile(filepath.Join(modDir, "module.cue"), []byte(moduleCue), 0644); err != nil {
		t.Fatalf("writing module.cue: %v", err)
	}

	// Create the module definition file
	cueFile := filepath.Join(moduleDir, pkgName+".cue")
	if err := os.WriteFile(cueFile, []byte(cueContent), 0644); err != nil {
		t.Fatalf("writing %s.cue: %v", pkgName, err)
	}

	return moduleDir
}

func TestExtractModuleContent_Task(t *testing.T) {
	t.Parallel()

	moduleDir := createTestModule(t, "task", `package task

task: {
	description: "Debug Go code"
	tags: ["golang", "debug"]
	prompt: "Help me debug this Go code."
}
`)

	module := SearchResult{
		Category: "tasks",
		Name:     "golang/debug",
	}

	astResult, err := ExtractModuleContent(moduleDir, module, nil, "test.example/module@v0.1.0", "")
	if err != nil {
		t.Fatalf("ExtractModuleContent() error: %v", err)
	}
	result := formatAST(t, astResult)

	// Should contain origin from originPath
	if !strings.Contains(result, "origin:") || !strings.Contains(result, `"test.example/module@v0.1.0"`) {
		t.Errorf("missing origin field\nGot:\n%s", result)
	}
	// Should contain description
	if !strings.Contains(result, "description:") || !strings.Contains(result, `"Debug Go code"`) {
		t.Errorf("missing description\nGot:\n%s", result)
	}
	// Should contain tags
	if !strings.Contains(result, `"golang"`) || !strings.Contains(result, `"debug"`) {
		t.Errorf("missing tags\nGot:\n%s", result)
	}
	// Should contain prompt
	if !strings.Contains(result, "Help me debug this Go code") {
		t.Errorf("missing prompt\nGot:\n%s", result)
	}
}

func TestExtractModuleContent_Role(t *testing.T) {
	t.Parallel()

	moduleDir := createTestModule(t, "role", `package role

role: {
	description: "Go programming expert"
	tags: ["golang"]
	prompt: "You are an expert in Go."
}
`)

	module := SearchResult{
		Category: "roles",
		Name:     "golang/expert",
	}

	astResult, err := ExtractModuleContent(moduleDir, module, nil, "test.example/role@v0.2.0", "")
	if err != nil {
		t.Fatalf("ExtractModuleContent() error: %v", err)
	}
	result := formatAST(t, astResult)

	if !strings.Contains(result, "origin:") || !strings.Contains(result, `"test.example/role@v0.2.0"`) {
		t.Errorf("missing origin\nGot:\n%s", result)
	}
	if !strings.Contains(result, "description:") || !strings.Contains(result, `"Go programming expert"`) {
		t.Errorf("missing description\nGot:\n%s", result)
	}
	if !strings.Contains(result, "You are an expert in Go") {
		t.Errorf("missing prompt\nGot:\n%s", result)
	}
}

func TestExtractModuleContent_Agent(t *testing.T) {
	t.Parallel()

	moduleDir := createTestModule(t, "agent", `package agent

agent: {
	description: "Claude AI assistant"
	bin: "claude"
	command: "{{.bin}} --model {{.model}}"
	default_model: "sonnet"
	models: {
		sonnet: "claude-sonnet-4-20250514"
		opus: "claude-opus-4-20250514"
	}
}
`)

	module := SearchResult{
		Category: "agents",
		Name:     "claude",
	}

	astResult, err := ExtractModuleContent(moduleDir, module, nil, "test.example/agent@v0.1.0", "")
	if err != nil {
		t.Fatalf("ExtractModuleContent() error: %v", err)
	}
	result := formatAST(t, astResult)

	if !strings.Contains(result, "bin:") || !strings.Contains(result, `"claude"`) {
		t.Errorf("missing bin\nGot:\n%s", result)
	}
	if !strings.Contains(result, "default_model:") || !strings.Contains(result, `"sonnet"`) {
		t.Errorf("missing default_model\nGot:\n%s", result)
	}
	if !strings.Contains(result, "models:") {
		t.Errorf("missing models map\nGot:\n%s", result)
	}
}

func TestExtractModuleContent_RoleNameOverride(t *testing.T) {
	t.Parallel()

	moduleDir := createTestModule(t, "task", `package task

task: {
	description: "Code review task"
	role: {
		description: "Inline reviewer role"
		prompt: "You are a code reviewer."
	}
	prompt: "Review this code."
}
`)

	module := SearchResult{
		Category: "tasks",
		Name:     "review",
	}

	astResult, err := ExtractModuleContent(moduleDir, module, nil, "test.example/task@v0.1.0", "golang/reviewer")
	if err != nil {
		t.Fatalf("ExtractModuleContent() error: %v", err)
	}
	result := formatAST(t, astResult)

	// Role should be replaced with string reference
	if !strings.Contains(result, "role:") || !strings.Contains(result, `"golang/reviewer"`) {
		t.Errorf("expected role name override\nGot:\n%s", result)
	}
	// Inline role content should not appear
	if strings.Contains(result, "Inline reviewer role") {
		t.Errorf("inline role should be replaced\nGot:\n%s", result)
	}
}

func TestExtractModuleContent_NoModuleDefinition(t *testing.T) {
	t.Parallel()

	moduleDir := createTestModule(t, "other", `package other

something: {
	description: "Not a module"
}
`)

	module := SearchResult{
		Category: "tasks",
		Name:     "missing",
	}

	_, err := ExtractModuleContent(moduleDir, module, nil, "test.example/bad@v0", "")
	if err == nil {
		t.Fatal("expected error for missing module definition")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestExtractModuleContent_MultilinePrompt(t *testing.T) {
	t.Parallel()

	moduleDir := createTestModule(t, "task", `package task

task: {
	description: "Multi-line task"
	prompt: """
		Line one.
		Line two.
		Line three.
		"""
}
`)

	module := SearchResult{
		Category: "tasks",
		Name:     "multiline",
	}

	astResult, err := ExtractModuleContent(moduleDir, module, nil, "test.example/task@v0.1.0", "")
	if err != nil {
		t.Fatalf("ExtractModuleContent() error: %v", err)
	}
	result := formatAST(t, astResult)

	if !strings.Contains(result, "Line one") {
		t.Errorf("missing multi-line content\nGot:\n%s", result)
	}
	if !strings.Contains(result, "Line three") {
		t.Errorf("missing multi-line content\nGot:\n%s", result)
	}
}

func TestExtractModuleContent_OptionalRoleField(t *testing.T) {
	t.Parallel()

	moduleDir := createTestModule(t, "role", `package role

role: {
	description: "Optional role"
	prompt: "You might be needed."
	optional: true
}
`)

	module := SearchResult{
		Category: "roles",
		Name:     "optional-role",
	}

	astResult, err := ExtractModuleContent(moduleDir, module, nil, "test.example/role@v0.1.0", "")
	if err != nil {
		t.Fatalf("ExtractModuleContent() error: %v", err)
	}
	result := formatAST(t, astResult)

	if !strings.Contains(result, "optional:") || !strings.Contains(result, "true") {
		t.Errorf("missing optional field\nGot:\n%s", result)
	}
}

// TestGetInstalledOrigin tests the GetInstalledOrigin function.
func TestGetInstalledOrigin(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()

	// Write a contexts.cue file with a module that has an origin
	contextsFile := filepath.Join(configDir, "contexts.cue")
	content := `// start configuration
contexts: {
	"cwd/agents-md": {
		origin: "github.com/test/contexts/cwd/agents-md@v0.1.0"
		description: "Read AGENTS.md file"
		file: "AGENTS.md"
	}
	"cwd/env": {
		description: "No origin field"
		file: ".env"
	}
}
`
	if err := os.WriteFile(contextsFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Load config via CUE
	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(configDir)
	if err != nil {
		t.Fatalf("Failed to load CUE config: %v", err)
	}

	tests := []struct {
		name       string
		category   string
		moduleName string
		want       string
	}{
		{
			name:       "module with origin",
			category:   "contexts",
			moduleName: "cwd/agents-md",
			want:       "github.com/test/contexts/cwd/agents-md@v0.1.0",
		},
		{
			name:       "module without origin",
			category:   "contexts",
			moduleName: "cwd/env",
			want:       "",
		},
		{
			name:       "non-existent module",
			category:   "contexts",
			moduleName: "does/not-exist",
			want:       "",
		},
		{
			name:       "non-existent category",
			category:   "roles",
			moduleName: "cwd/agents-md",
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetInstalledOrigin(cfg, tt.category, tt.moduleName)
			if got != tt.want {
				t.Errorf("GetInstalledOrigin() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestVersionFromOrigin tests the VersionFromOrigin function.
// TestFormatFieldExpr tests each branch of the formatFieldExpr value-to-AST converter.
func TestFormatFieldExpr(t *testing.T) {
	t.Parallel()

	ctx := cuecontext.New()

	tests := []struct {
		name string
		cue  string
		want string
	}{
		{
			name: "string value",
			cue:  `"hello world"`,
			want: `"hello world"`,
		},
		{
			name: "bool true",
			cue:  `true`,
			want: `true`,
		},
		{
			name: "bool false",
			cue:  `false`,
			want: `false`,
		},
		{
			name: "string list",
			cue:  `["a", "b", "c"]`,
			want: `["a", "b", "c"]`,
		},
		{
			name: "empty list",
			cue:  `[]`,
			want: `[]`,
		},
		{
			name: "struct with string values",
			cue:  `{flash: "gemini-2.5-flash", pro: "gemini-2.5-pro"}`,
			want: `flash: "gemini-2.5-flash"`,
		},
		{
			name: "struct with mixed types",
			cue:  `{name: "test", enabled: true, tags: ["a", "b"]}`,
			want: `enabled: true`,
		},
		{
			name: "nested struct",
			cue:  `{outer: {inner: "value"}}`,
			want: `inner: "value"`,
		},
		{
			name: "int via default fallback",
			cue:  `42`,
			want: `42`,
		},
		{
			name: "float via default fallback",
			cue:  `3.14`,
			want: `3.14`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ctx.CompileString(tt.cue)
			if v.Err() != nil {
				t.Fatalf("compiling CUE: %v", v.Err())
			}

			expr, err := formatFieldExpr(v)
			if err != nil {
				t.Fatalf("formatFieldExpr() error: %v", err)
			}

			result := formatAST(t, expr)
			if !strings.Contains(result, tt.want) {
				t.Errorf("formatFieldExpr() result missing %q\nGot:\n%s", tt.want, result)
			}
		})
	}
}

// TestFormatFieldExpr_StructMixedTypes verifies the recursive struct branch
// produces a complete struct with all value types preserved.
func TestFormatFieldExpr_StructMixedTypes(t *testing.T) {
	t.Parallel()

	ctx := cuecontext.New()
	v := ctx.CompileString(`{
		name: "test"
		enabled: true
		tags: ["a", "b"]
		nested: {key: "val"}
	}`)
	if v.Err() != nil {
		t.Fatalf("compiling CUE: %v", v.Err())
	}

	expr, err := formatFieldExpr(v)
	if err != nil {
		t.Fatalf("formatFieldExpr() error: %v", err)
	}

	result := formatAST(t, expr)
	for _, want := range []string{
		`"test"`,
		`true`,
		`"a"`,
		`"b"`,
		`key:`,
		`"val"`,
	} {
		if !strings.Contains(result, want) {
			t.Errorf("result missing %q\nGot:\n%s", want, result)
		}
	}
}

func TestVersionFromOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		origin string
		want   string
	}{
		{"github.com/test/module@v0.1.1", "v0.1.1"},
		{"github.com/test/module@v0", "v0"},
		{"no-version", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			got := VersionFromOrigin(tt.origin)
			if got != tt.want {
				t.Errorf("VersionFromOrigin(%q) = %q, want %q", tt.origin, got, tt.want)
			}
		})
	}
}

func TestModuleFromOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		origin string
		want   string
	}{
		{"github.com/test/module@v0.1.1", "github.com/test/module"},
		{"github.com/test/module@v0", "github.com/test/module"},
		{"no-version", "no-version"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			got := ModuleFromOrigin(tt.origin)
			if got != tt.want {
				t.Errorf("ModuleFromOrigin(%q) = %q, want %q", tt.origin, got, tt.want)
			}
		})
	}
}
