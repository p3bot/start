package registry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/mod/module"
)

func TestEffectiveIndexPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured string
		want       string
	}{
		{
			name:       "empty string returns default",
			configured: "",
			want:       IndexModulePath,
		},
		{
			name:       "non-empty string returns configured value",
			configured: "github.com/example/custom-assets/index@v0",
			want:       "github.com/example/custom-assets/index@v0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := EffectiveIndexPath(tt.configured)
			if got != tt.want {
				t.Errorf("EffectiveIndexPath(%q) = %q, want %q", tt.configured, got, tt.want)
			}
		})
	}
}

func TestLoadIndex_ValidIndex(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create a valid index CUE file
	indexCUE := `
package index

agents: {
	"ai/claude": {
		module:      "github.com/test/claude@v0"
		description: "Claude AI"
		bin:         "claude"
	}
	"ai/gemini": {
		module:      "github.com/test/gemini@v0"
		description: "Gemini AI"
		bin:         "gemini"
	}
}

roles: {
	"dev/expert": {
		module: "github.com/test/expert@v0"
	}
}

contexts: {
	"env/local": {
		module: "github.com/test/local@v0"
	}
}

tasks: {
	"golang/review": {
		module:      "github.com/test/review@v0"
		description: "Code review"
		tags:        ["golang", "review"]
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}

	index, err := LoadIndex(tmpDir, nil)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	// Verify agents
	if len(index.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(index.Agents))
	}

	claude, ok := index.Agents["ai/claude"]
	if !ok {
		t.Error("missing ai/claude agent")
	} else {
		if claude.Module != "github.com/test/claude@v0" {
			t.Errorf("wrong module: %s", claude.Module)
		}
		if claude.Bin != "claude" {
			t.Errorf("wrong bin: %s", claude.Bin)
		}
	}

	// Verify roles
	if len(index.Roles) != 1 {
		t.Errorf("expected 1 role, got %d", len(index.Roles))
	}

	// Verify contexts
	if len(index.Contexts) != 1 {
		t.Errorf("expected 1 context, got %d", len(index.Contexts))
	}

	// Verify tasks
	if len(index.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(index.Tasks))
	}

	review, ok := index.Tasks["golang/review"]
	if !ok {
		t.Error("missing golang/review task")
	} else {
		if len(review.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(review.Tags))
		}
	}
}

func TestLoadIndex_EmptyCategories(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Index with only agents
	indexCUE := `
package index

agents: {
	"ai/test": {
		module: "github.com/test/test@v0"
		bin:    "test"
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}

	index, err := LoadIndex(tmpDir, nil)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	if len(index.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(index.Agents))
	}
	if len(index.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(index.Tasks))
	}
	if len(index.Roles) != 0 {
		t.Errorf("expected 0 roles, got %d", len(index.Roles))
	}
	if len(index.Contexts) != 0 {
		t.Errorf("expected 0 contexts, got %d", len(index.Contexts))
	}
}

func TestLoadIndex_InvalidCUE(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Invalid CUE syntax
	indexCUE := `
package index

agents: {
	"ai/test": {
		module: "github.com/test/test@v0"
		// Missing closing brace
`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}

	_, err := LoadIndex(tmpDir, nil)
	if err == nil {
		t.Error("expected error for invalid CUE, got nil")
	}
}

func TestLoadIndex_NonexistentDir(t *testing.T) {
	t.Parallel()
	_, err := LoadIndex("/nonexistent/directory/12345", nil)
	if err == nil {
		t.Error("expected error for nonexistent directory, got nil")
	}
}

func TestLoadIndex_EmptyDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	_, err := LoadIndex(tmpDir, nil)
	if err == nil {
		t.Error("expected error for empty directory, got nil")
	}
}

func TestDecodeIndex_AllCategories(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create an index with all categories populated
	indexCUE := `
package index

agents: {
	"ai/claude": {
		module:      "github.com/test/claude@v0"
		description: "Claude AI"
		bin:         "claude"
		tags:        ["ai", "anthropic"]
		version:     "v0.1.0"
	}
}

roles: {
	"dev/expert": {
		module:      "github.com/test/expert@v0"
		description: "Expert developer role"
	}
}

contexts: {
	"env/local": {
		module:      "github.com/test/local@v0"
		description: "Local environment context"
	}
}

tasks: {
	"golang/review": {
		module:      "github.com/test/review@v0"
		description: "Code review task"
		tags:        ["golang", "review"]
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}

	index, err := LoadIndex(tmpDir, nil)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	// Verify all categories are populated
	if len(index.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(index.Agents))
	}
	if len(index.Roles) != 1 {
		t.Errorf("expected 1 role, got %d", len(index.Roles))
	}
	if len(index.Contexts) != 1 {
		t.Errorf("expected 1 context, got %d", len(index.Contexts))
	}
	if len(index.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(index.Tasks))
	}

	// Verify agent fields are decoded correctly
	claude := index.Agents["ai/claude"]
	if claude.Module != "github.com/test/claude@v0" {
		t.Errorf("wrong module: %s", claude.Module)
	}
	if claude.Description != "Claude AI" {
		t.Errorf("wrong description: %s", claude.Description)
	}
	if claude.Bin != "claude" {
		t.Errorf("wrong bin: %s", claude.Bin)
	}
	if len(claude.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(claude.Tags))
	}
	if claude.Version != "v0.1.0" {
		t.Errorf("wrong version: %s", claude.Version)
	}
}

func TestDecodeIndex_MinimalEntry(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create an index with minimal entries (only required fields)
	indexCUE := `
package index

agents: {
	"ai/minimal": {
		module: "github.com/test/minimal@v0"
		bin:    "minimal"
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}

	index, err := LoadIndex(tmpDir, nil)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	agent := index.Agents["ai/minimal"]
	if agent.Module != "github.com/test/minimal@v0" {
		t.Errorf("wrong module: %s", agent.Module)
	}
	if agent.Bin != "minimal" {
		t.Errorf("wrong bin: %s", agent.Bin)
	}
	// Optional fields should be empty/nil
	if agent.Description != "" {
		t.Errorf("expected empty description, got: %s", agent.Description)
	}
	if len(agent.Tags) != 0 {
		t.Errorf("expected no tags, got %d", len(agent.Tags))
	}
	if agent.Version != "" {
		t.Errorf("expected empty version, got: %s", agent.Version)
	}
}

func TestDecodeIndex_InvalidEntryType(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create an index with invalid entry type (string instead of struct)
	indexCUE := `
package index

agents: {
	"ai/invalid": "this should be a struct, not a string"
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}

	_, err := LoadIndex(tmpDir, nil)
	if err == nil {
		t.Error("expected error for invalid entry type, got nil")
	}
}

func TestDecodeIndex_WrongPackageName(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create CUE file with wrong package name
	indexCUE := `
package wrong_package

agents: {
	"ai/test": {
		module: "github.com/test/test@v0"
		bin:    "test"
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}

	_, err := LoadIndex(tmpDir, nil)
	if err == nil {
		t.Error("expected error for wrong package name, got nil")
	}
}

func TestDecodeIndex_MultipleFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create multiple CUE files that should be merged
	agentsCUE := `
package index

agents: {
	"ai/claude": {
		module: "github.com/test/claude@v0"
		bin:    "claude"
	}
}
`
	tasksCUE := `
package index

tasks: {
	"golang/review": {
		module: "github.com/test/review@v0"
	}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "agents.cue"), []byte(agentsCUE), 0644); err != nil {
		t.Fatalf("writing agents.cue: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "tasks.cue"), []byte(tasksCUE), 0644); err != nil {
		t.Fatalf("writing tasks.cue: %v", err)
	}

	index, err := LoadIndex(tmpDir, nil)
	if err != nil {
		t.Fatalf("LoadIndex failed: %v", err)
	}

	// Both categories should be populated from separate files
	if len(index.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(index.Agents))
	}
	if len(index.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(index.Tasks))
	}
}

// writeTestIndex creates a valid index CUE file in dir for FetchIndex tests.
func writeTestIndex(t *testing.T, dir string) {
	t.Helper()
	indexCUE := `
package index

agents: {
	"ai/claude": {
		module: "github.com/test/claude@v0"
		bin:    "claude"
	}
}

tasks: {
	"golang/review": {
		module:      "github.com/test/review@v0"
		description: "Code review"
		tags:        ["golang"]
	}
}
`
	if err := os.WriteFile(filepath.Join(dir, "index.cue"), []byte(indexCUE), 0644); err != nil {
		t.Fatalf("writing test index: %v", err)
	}
}

func TestFetchIndex_ReturnsIndexAndVersion(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	writeTestIndex(t, tmpDir)

	mock := &mockRegistry{
		versionsFunc: func(ctx context.Context, path string) ([]string, error) {
			return []string{"v0.0.1", "v0.3.46"}, nil
		},
		fetchFunc: func(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
			return module.SourceLoc{
				FS: &mockOSRootFS{root: tmpDir},
			}, nil
		},
	}

	client := &Client{
		registry: mock,
		retries:  1,
		baseWait: time.Millisecond,
	}

	ctx := context.Background()
	idx, version, err := client.FetchIndex(ctx, "github.com/test/index@v0")
	if err != nil {
		t.Fatalf("FetchIndex() error: %v", err)
	}

	// Verify index is populated.
	if idx == nil {
		t.Fatal("FetchIndex() returned nil index")
	}
	if len(idx.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(idx.Agents))
	}
	if len(idx.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(idx.Tasks))
	}

	// Verify resolved canonical version is returned.
	want := "github.com/test/index@v0.3.46"
	if version != want {
		t.Errorf("FetchIndex() version = %q, want %q", version, want)
	}
}

func TestFetchIndex_CanonicalVersionSkipsResolution(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	writeTestIndex(t, tmpDir)

	versionsCalled := false
	mock := &mockRegistry{
		versionsFunc: func(ctx context.Context, path string) ([]string, error) {
			versionsCalled = true
			return nil, nil
		},
		fetchFunc: func(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
			return module.SourceLoc{
				FS: &mockOSRootFS{root: tmpDir},
			}, nil
		},
	}

	client := &Client{
		registry: mock,
		retries:  1,
		baseWait: time.Millisecond,
	}

	ctx := context.Background()
	// Pass a canonical version — should not call ModuleVersions.
	idx, version, err := client.FetchIndex(ctx, "github.com/test/index@v0.3.46")
	if err != nil {
		t.Fatalf("FetchIndex() error: %v", err)
	}
	if idx == nil {
		t.Fatal("FetchIndex() returned nil index")
	}
	if version != "github.com/test/index@v0.3.46" {
		t.Errorf("version = %q, want canonical passthrough", version)
	}
	if versionsCalled {
		t.Error("ModuleVersions should not be called for canonical versions")
	}
}

func TestFetchIndex_VersionResolutionError(t *testing.T) {
	t.Parallel()

	mock := &mockRegistry{
		versionsFunc: func(ctx context.Context, path string) ([]string, error) {
			return nil, context.DeadlineExceeded
		},
	}

	client := &Client{
		registry: mock,
		retries:  1,
		baseWait: time.Millisecond,
	}

	ctx := context.Background()
	idx, version, err := client.FetchIndex(ctx, "github.com/test/index@v0")
	if err == nil {
		t.Fatal("FetchIndex() expected error for version resolution failure")
	}
	if !strings.Contains(err.Error(), "resolving index version") {
		t.Errorf("error = %v, want 'resolving index version' prefix", err)
	}
	if idx != nil {
		t.Error("expected nil index on error")
	}
	if version != "" {
		t.Errorf("expected empty version on error, got %q", version)
	}
}

func TestFetchIndex_FetchError(t *testing.T) {
	t.Parallel()

	mock := &mockRegistry{
		versionsFunc: func(ctx context.Context, path string) ([]string, error) {
			return []string{"v0.1.0"}, nil
		},
		fetchFunc: func(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
			return module.SourceLoc{}, context.DeadlineExceeded
		},
	}

	client := &Client{
		registry: mock,
		retries:  1,
		baseWait: time.Millisecond,
	}

	ctx := context.Background()
	idx, version, err := client.FetchIndex(ctx, "github.com/test/index@v0")
	if err == nil {
		t.Fatal("FetchIndex() expected error for fetch failure")
	}
	if !strings.Contains(err.Error(), "fetching index module") {
		t.Errorf("error = %v, want 'fetching index module' prefix", err)
	}
	if idx != nil {
		t.Error("expected nil index on error")
	}
	if version != "" {
		t.Errorf("expected empty version on error, got %q", version)
	}
}

func TestFetchIndex_LoadIndexError(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Write invalid CUE so LoadIndex fails.
	if err := os.WriteFile(filepath.Join(tmpDir, "index.cue"), []byte("not valid {{{"), 0644); err != nil {
		t.Fatalf("writing bad index: %v", err)
	}

	mock := &mockRegistry{
		versionsFunc: func(ctx context.Context, path string) ([]string, error) {
			return []string{"v0.1.0"}, nil
		},
		fetchFunc: func(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
			return module.SourceLoc{
				FS: &mockOSRootFS{root: tmpDir},
			}, nil
		},
	}

	client := &Client{
		registry: mock,
		retries:  1,
		baseWait: time.Millisecond,
	}

	ctx := context.Background()
	idx, version, err := client.FetchIndex(ctx, "github.com/test/index@v0")
	if err == nil {
		t.Fatal("FetchIndex() expected error for invalid index CUE")
	}
	if idx != nil {
		t.Error("expected nil index on error")
	}
	if version != "" {
		t.Errorf("expected empty version on error, got %q", version)
	}
}

func TestFetchIndex_DefaultIndexPath(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	writeTestIndex(t, tmpDir)

	var resolvedPath string
	mock := &mockRegistry{
		versionsFunc: func(ctx context.Context, path string) ([]string, error) {
			resolvedPath = path
			return []string{"v0.1.0"}, nil
		},
		fetchFunc: func(ctx context.Context, mv module.Version) (module.SourceLoc, error) {
			return module.SourceLoc{
				FS: &mockOSRootFS{root: tmpDir},
			}, nil
		},
	}

	client := &Client{
		registry: mock,
		retries:  1,
		baseWait: time.Millisecond,
	}

	ctx := context.Background()
	// Pass empty string — should use IndexModulePath default.
	_, _, err := client.FetchIndex(ctx, "")
	if err != nil {
		t.Fatalf("FetchIndex() error: %v", err)
	}
	if resolvedPath != IndexModulePath {
		t.Errorf("resolved path = %q, want default %q", resolvedPath, IndexModulePath)
	}
}
