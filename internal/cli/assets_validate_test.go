package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/doctor"
	"github.com/start-cli/start/internal/registry"
)

// TestValidateDeriveRepoURL verifies URL derivation from index module paths.
func TestValidateDeriveRepoURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "default index path",
			input: "github.com/start-cli/library/index@v1",
			want:  "https://github.com/start-cli/library",
		},
		{
			name:  "canonical version",
			input: "github.com/start-cli/library/index@v1.0.1",
			want:  "https://github.com/start-cli/library",
		},
		{
			name:  "custom org",
			input: "github.com/example/custom-assets/index@v0",
			want:  "https://github.com/example/custom-assets",
		},
		{
			name:    "non-index subpath rejected",
			input:   "github.com/myorg/custom/registry@v0",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateDeriveRepoURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateDeriveRepoURL(%q) expected error, got %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateDeriveRepoURL(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("validateDeriveRepoURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateCacheDirName verifies cache directory name derivation from repo URLs.
func TestValidateCacheDirName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"https://github.com/start-cli/library", "start-cli-library"},
		{"https://github.com/example/custom-assets", "example-custom-assets"},
		{"https://github.com/myorg/my-assets", "myorg-my-assets"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := validateCacheDirName(tt.input)
			if got != tt.want {
				t.Errorf("validateCacheDirName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestValidateGitTagPrefix verifies tag prefix construction.
func TestValidateGitTagPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		category string
		name     string
		want     string
	}{
		{"agents", "claude", "agents/claude/"},
		{"roles", "golang", "roles/golang/"},
		{"tasks", "review/architecture", "tasks/review/architecture/"},
		{"contexts", "environment", "contexts/environment/"},
	}
	for _, tt := range tests {
		t.Run(tt.category+"/"+tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateGitTagPrefix(tt.category, tt.name)
			if got != tt.want {
				t.Errorf("validateGitTagPrefix(%q, %q) = %q, want %q", tt.category, tt.name, got, tt.want)
			}
		})
	}
}

// TestValidateTagVersions verifies filtering and sorting of tags.
func TestValidateTagVersions(t *testing.T) {
	t.Parallel()
	tags := []string{
		"agents/claude/v0.0.1",
		"agents/claude/v0.0.2",
		"agents/claude/v0.1.0",
		"agents/gemini/v0.1.0",
		"roles/golang/v0.1.0",
		"index/v0.1.8",
		"index/not-semver",
	}

	tests := []struct {
		prefix string
		want   []string
	}{
		{
			prefix: "agents/claude/",
			want:   []string{"v0.0.1", "v0.0.2", "v0.1.0"},
		},
		{
			prefix: "agents/gemini/",
			want:   []string{"v0.1.0"},
		},
		{
			prefix: "index/",
			want:   []string{"v0.1.8"},
		},
		{
			prefix: "tasks/missing/",
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			t.Parallel()
			got := validateTagVersions(tags, tt.prefix)
			if len(got) != len(tt.want) {
				t.Fatalf("validateTagVersions(%q) = %v, want %v", tt.prefix, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("validateTagVersions(%q)[%d] = %q, want %q", tt.prefix, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestValidateLatestTagVersion verifies latest version selection.
func TestValidateLatestTagVersion(t *testing.T) {
	t.Parallel()
	tags := []string{
		"agents/claude/v0.0.1",
		"agents/claude/v0.0.2",
		"agents/claude/v0.1.0",
		"roles/golang/v0.2.0",
	}

	tests := []struct {
		prefix string
		want   string
	}{
		{"agents/claude/", "v0.1.0"},
		{"roles/golang/", "v0.2.0"},
		{"missing/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			t.Parallel()
			got := validateLatestTagVersion(tags, tt.prefix)
			if got != tt.want {
				t.Errorf("validateLatestTagVersion(%q) = %q, want %q", tt.prefix, got, tt.want)
			}
		})
	}
}

// TestIndexVersionFromPath verifies version extraction from a resolved module path.
func TestIndexVersionFromPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"github.com/start-cli/library/index@v1.0.1", "v1.0.1"},
		{"github.com/start-cli/library/index@v1", ""}, // major only — not canonical
		{"github.com/start-cli/library/index@v1.0.0", "v1.0.0"},
		{"no-version-here", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := indexVersionFromPath(tt.input)
			if got != tt.want {
				t.Errorf("indexVersionFromPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIndexEntryCount verifies entry counting across categories.
func TestIndexEntryCount(t *testing.T) {
	t.Parallel()
	idx := makeTestRegistryIndex(3, 2, 4, 1)
	got := indexEntryCount(idx)
	want := 10
	if got != want {
		t.Errorf("indexEntryCount() = %d, want %d", got, want)
	}
}

// TestValidateFindFSModules verifies that module discovery via cue.mod/ works.
func TestValidateFindFSModules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a module structure:
	//   agents/claude/cue.mod/   → module
	//   agents/gemini/cue.mod/   → module
	//   agents/docs/             → NOT a module (no cue.mod)
	//   tasks/review/arch/cue.mod/ → nested module
	for _, p := range []string{
		"agents/claude/cue.mod",
		"agents/gemini/cue.mod",
		"agents/docs",
		"tasks/review/arch/cue.mod",
	} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("agents", func(t *testing.T) {
		got := validateFindFSModules("agents", dir)
		want := []string{"claude", "gemini"}
		if !stringSlicesEqual(got, want) {
			t.Errorf("validateFindFSModules(agents) = %v, want %v", got, want)
		}
	})

	t.Run("tasks nested", func(t *testing.T) {
		got := validateFindFSModules("tasks", dir)
		want := []string{"review/arch"}
		if !stringSlicesEqual(got, want) {
			t.Errorf("validateFindFSModules(tasks) = %v, want %v", got, want)
		}
	})

	t.Run("missing category", func(t *testing.T) {
		got := validateFindFSModules("roles", dir)
		if len(got) != 0 {
			t.Errorf("validateFindFSModules(roles) = %v, want empty", got)
		}
	})
}

// TestValidateIsStale verifies staleness detection using a real git repo.
func TestValidateIsStale(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@test.com")
	mustGit(t, dir, "config", "user.name", "Test")

	// Create module file and commit
	modDir := filepath.Join(dir, "agents", "claude")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "agent.cue"), []byte("package agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "initial")
	mustGit(t, dir, "tag", "agents/claude/v0.1.0")

	t.Run("not stale after tag", func(t *testing.T) {
		stale, err := validateIsStale(dir, "agents/claude/v0.1.0", "agents/claude")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stale {
			t.Error("expected not stale immediately after tag")
		}
	})

	// Modify the file
	if err := os.WriteFile(filepath.Join(modDir, "agent.cue"), []byte("package agent\n// updated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "update agent")

	t.Run("stale after content change", func(t *testing.T) {
		stale, err := validateIsStale(dir, "agents/claude/v0.1.0", "agents/claude")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !stale {
			t.Error("expected stale after content change post-tag")
		}
	})

	t.Run("different path not stale", func(t *testing.T) {
		stale, err := validateIsStale(dir, "agents/claude/v0.1.0", "agents/other")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stale {
			t.Error("expected not stale for unrelated path")
		}
	})
}

// TestPrintValidateStats verifies the statistics output format.
func TestPrintValidateStats(t *testing.T) {
	t.Parallel()

	t.Run("all pass", func(t *testing.T) {
		t.Parallel()
		cats := []validateCatResult{
			{name: "agents", modules: []validateModuleResult{
				{status: validateModulePass},
				{status: validateModulePass},
			}},
		}
		var buf bytes.Buffer
		hasFailure := printValidateStats(&buf, cats)
		if hasFailure {
			t.Error("expected no failure for all-pass")
		}
		out := buf.String()
		if !strings.Contains(out, "Checked:") {
			t.Errorf("output missing 'Checked:': %q", out)
		}
	})

	t.Run("with failures", func(t *testing.T) {
		t.Parallel()
		cats := []validateCatResult{
			{name: "agents", modules: []validateModuleResult{
				{status: validateModulePass},
				{status: validateModuleFail},
			}},
		}
		var buf bytes.Buffer
		hasFailure := printValidateStats(&buf, cats)
		if !hasFailure {
			t.Error("expected hasFailure=true")
		}
	})
}

// TestPrintValidateModulesDefault verifies default (non-verbose) output format.
func TestPrintValidateModulesDefault(t *testing.T) {
	t.Parallel()
	cats := []validateCatResult{
		{
			name: "agents",
			modules: []validateModuleResult{
				{name: "claude", version: "v0.1.0", status: validateModulePass},
				{name: "gemini", version: "v0.1.0", status: validateModulePass},
			},
		},
		{
			name: "contexts",
			modules: []validateModuleResult{
				{name: "environment", version: "v0.1.0", status: validateModulePass},
				{name: "project", version: "v0.1.0", status: validateModuleFail, issues: []string{"stale: content changed"}},
			},
		},
	}

	var buf bytes.Buffer
	printValidateModules(&buf, cats, false)
	out := buf.String()

	if !strings.Contains(out, "2/2 OK") {
		t.Errorf("expected agents 2/2 OK in output: %q", out)
	}
	if !strings.Contains(out, "1/2 FAIL") {
		t.Errorf("expected contexts 1/2 FAIL in output: %q", out)
	}
	if !strings.Contains(out, "project") {
		t.Errorf("expected failing module 'project' listed in output: %q", out)
	}
	if strings.Contains(out, "claude") {
		t.Errorf("passing module 'claude' should not appear in default output: %q", out)
	}
}

// TestPrintValidateModulesVerbose verifies verbose output lists all modules.
func TestPrintValidateModulesVerbose(t *testing.T) {
	t.Parallel()
	cats := []validateCatResult{
		{
			name: "agents",
			modules: []validateModuleResult{
				{name: "claude", version: "v0.1.0", status: validateModulePass},
				{name: "gemini", version: "v0.2.0", status: validateModuleFail, issues: []string{"index version mismatch"}},
			},
		},
	}

	var buf bytes.Buffer
	printValidateModules(&buf, cats, true)
	out := buf.String()

	if !strings.Contains(out, "claude") {
		t.Errorf("verbose output should include passing module 'claude': %q", out)
	}
	if !strings.Contains(out, "gemini") {
		t.Errorf("verbose output should include failing module 'gemini': %q", out)
	}
	if !strings.Contains(out, "index version mismatch") {
		t.Errorf("verbose output should include issue detail: %q", out)
	}
}

// TestValidateError verifies the validateError satisfies the SilentError interface.
func TestValidateError(t *testing.T) {
	t.Parallel()
	err := &validateError{}
	if IsSilentError(err) != true {
		t.Error("validateError should be a silent error")
	}
	if err.Error() == "" {
		t.Error("validateError.Error() should not be empty")
	}
}

// --- helpers ---

// makeTestRegistryIndex creates a *registry.Index with n stub entries per category.
func makeTestRegistryIndex(agents, roles, contexts, tasks int) *registry.Index {
	idx := &registry.Index{
		Agents:   make(map[string]registry.IndexEntry, agents),
		Roles:    make(map[string]registry.IndexEntry, roles),
		Contexts: make(map[string]registry.IndexEntry, contexts),
		Tasks:    make(map[string]registry.IndexEntry, tasks),
	}
	for i := 0; i < agents; i++ {
		idx.Agents[fmt.Sprintf("agent%d", i)] = registry.IndexEntry{Version: "v0.1.0"}
	}
	for i := 0; i < roles; i++ {
		idx.Roles[fmt.Sprintf("role%d", i)] = registry.IndexEntry{Version: "v0.1.0"}
	}
	for i := 0; i < contexts; i++ {
		idx.Contexts[fmt.Sprintf("ctx%d", i)] = registry.IndexEntry{Version: "v0.1.0"}
	}
	for i := 0; i < tasks; i++ {
		idx.Tasks[fmt.Sprintf("task%d", i)] = registry.IndexEntry{Version: "v0.1.0"}
	}
	return idx
}

// mustGit runs a git command in dir, failing the test on error.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// stringSlicesEqual compares two string slices for equality.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestValidateWalkModules verifies the recursive module walk logic.
func TestValidateWalkModules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Structure:
	//   top-level module: claude/cue.mod/
	//   nested: review/arch/cue.mod/  review/perf/cue.mod/
	//   not a module: docs/
	for _, p := range []string{
		"claude/cue.mod",
		"review/arch/cue.mod",
		"review/perf/cue.mod",
		"docs",
	} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	var found []string
	if err := validateWalkModules(dir, "", func(rel string) {
		found = append(found, rel)
	}); err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"claude":      true,
		"review/arch": true,
		"review/perf": true,
	}
	if len(found) != len(want) {
		t.Fatalf("found %v, want keys of %v", found, want)
	}
	for _, f := range found {
		if !want[f] {
			t.Errorf("unexpected module found: %q", f)
		}
	}
}

// TestValidateCheckIndexVersionExistsNoop verifies that paths with a major-only
// version component (e.g. @v0) return nil without touching the registry client.
func TestValidateCheckIndexVersionExistsNoop(t *testing.T) {
	t.Parallel()
	client, err := registry.NewClient()
	if err != nil {
		t.Skipf("skipping: cannot create registry client: %v", err)
	}
	ctx := context.Background()
	paths := []string{
		"github.com/start-cli/library/index@v1",
		"github.com/start-cli/library/index@v2",
		"github.com/start-cli/library/index", // no @ at all
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if err := validateCheckIndexVersionExists(ctx, client, path); err != nil {
				t.Errorf("validateCheckIndexVersionExists(%q) = %v, want nil", path, err)
			}
		})
	}
}

// TestPrintValidateStatsOutput verifies exact stat field presence.
func TestPrintValidateStatsOutput(t *testing.T) {
	t.Parallel()
	cats := []validateCatResult{
		{name: "agents", modules: []validateModuleResult{
			{status: validateModulePass},
			{status: validateModulePass},
			{status: validateModuleFail},
		}},
		{name: "roles", modules: []validateModuleResult{
			{status: validateModulePass},
		}},
	}

	var buf bytes.Buffer
	printValidateStats(&buf, cats)
	out := buf.String()

	// The stats line should contain the key fields
	for _, want := range []string{"Checked:", "Pass:", "Fail:"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats output missing %q: %q", want, out)
		}
	}
}

// TestOutputValidateJSON verifies that validate results marshal correctly.
func TestOutputValidateJSON(t *testing.T) {
	t.Parallel()

	indexSection := doctor.SectionResult{
		Name: "Index",
		Results: []doctor.CheckResult{
			{Status: doctor.StatusPass, Label: "Valid", Message: "v0.1.8"},
		},
	}

	cats := []validateCatResult{
		{name: "agents", modules: []validateModuleResult{
			{name: "ai/claude", version: "v0.2.0", status: validateModulePass},
			{name: "ai/gemini", version: "v0.1.0", status: validateModuleFail, issues: []string{"index version mismatch"}},
		}},
		{name: "roles", modules: []validateModuleResult{
			{name: "golang", version: "v0.1.0", status: validateModulePass},
		}},
	}

	var buf bytes.Buffer
	err := outputValidateJSON(&buf, indexSection, cats)
	if err != nil {
		t.Fatalf("outputValidateJSON failed: %v", err)
	}

	var result ValidateResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Index checks
	if len(result.Index.Checks) != 1 {
		t.Fatalf("expected 1 index check, got %d", len(result.Index.Checks))
	}
	if result.Index.Checks[0].Status != "pass" {
		t.Errorf("expected index check status 'pass', got %q", result.Index.Checks[0].Status)
	}
	if result.Index.Checks[0].Message != "v0.1.8" {
		t.Errorf("expected index check message 'v0.1.8', got %q", result.Index.Checks[0].Message)
	}

	// Categories
	if len(result.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(result.Categories))
	}
	if result.Categories[0].Name != "agents" {
		t.Errorf("expected first category 'agents', got %q", result.Categories[0].Name)
	}
	if len(result.Categories[0].Modules) != 2 {
		t.Fatalf("expected 2 agent modules, got %d", len(result.Categories[0].Modules))
	}
	if result.Categories[0].Modules[1].Status != "fail" {
		t.Errorf("expected gemini status 'fail', got %q", result.Categories[0].Modules[1].Status)
	}
	if len(result.Categories[0].Modules[1].Issues) != 1 {
		t.Errorf("expected 1 issue for gemini, got %d", len(result.Categories[0].Modules[1].Issues))
	}

	// Stats
	if result.Stats.Checked != 3 {
		t.Errorf("expected 3 checked, got %d", result.Stats.Checked)
	}
	if result.Stats.Pass != 2 {
		t.Errorf("expected 2 pass, got %d", result.Stats.Pass)
	}
	if result.Stats.Fail != 1 {
		t.Errorf("expected 1 fail, got %d", result.Stats.Fail)
	}
}

// TestOutputValidateJSONNilCategories verifies JSON output when categories are nil.
func TestOutputValidateJSONNilCategories(t *testing.T) {
	t.Parallel()

	indexSection := doctor.SectionResult{
		Name: "Index",
		Results: []doctor.CheckResult{
			{Status: doctor.StatusFail, Label: "Unreachable", Message: "cannot resolve"},
		},
	}

	var buf bytes.Buffer
	err := outputValidateJSON(&buf, indexSection, nil)
	if err != nil {
		t.Fatalf("outputValidateJSON failed: %v", err)
	}

	var result ValidateResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(result.Index.Checks) != 1 {
		t.Fatalf("expected 1 index check, got %d", len(result.Index.Checks))
	}
	if result.Index.Checks[0].Status != "fail" {
		t.Errorf("expected 'fail', got %q", result.Index.Checks[0].Status)
	}
	if len(result.Categories) != 0 {
		t.Errorf("expected empty categories, got %d", len(result.Categories))
	}
	if result.Stats.Checked != 0 {
		t.Errorf("expected 0 checked, got %d", result.Stats.Checked)
	}
}

// TestValidateHasFailure tests the validateHasFailure helper.
func TestValidateHasFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cats []validateCatResult
		want bool
	}{
		{
			name: "all pass",
			cats: []validateCatResult{
				{name: "agents", modules: []validateModuleResult{
					{status: validateModulePass},
				}},
			},
			want: false,
		},
		{
			name: "has failure",
			cats: []validateCatResult{
				{name: "agents", modules: []validateModuleResult{
					{status: validateModulePass},
					{status: validateModuleFail},
				}},
			},
			want: true,
		},
		{
			name: "empty",
			cats: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := validateHasFailure(tt.cats)
			if got != tt.want {
				t.Errorf("validateHasFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}
