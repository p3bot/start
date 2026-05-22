package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
)

func TestCheckIntro(t *testing.T) {
	t.Parallel()
	section := CheckIntro()

	if section.Name != "Repository" {
		t.Errorf("CheckIntro().Name = %q, want %q", section.Name, "Repository")
	}
	if !section.NoIcons {
		t.Error("CheckIntro().NoIcons should be true")
	}
	if len(section.Results) != 2 {
		t.Fatalf("CheckIntro() should have 2 results, got %d", len(section.Results))
	}
	if section.Results[0].Label != RepoURL {
		t.Errorf("First result should be repo URL, got %q", section.Results[0].Label)
	}
	if section.Results[1].Label != IssuesURL {
		t.Errorf("Second result should be issues URL, got %q", section.Results[1].Label)
	}
}

func TestCheckVersion(t *testing.T) {
	t.Parallel()
	info := BuildInfo{
		Version:      "v1.0.0",
		Commit:       "abc123",
		BuildDate:    "2025-01-01",
		GoVersion:    "go1.23.0",
		Platform:     "linux/amd64",
		IndexVersion: "v0.3.2",
	}

	section := CheckVersion(info)

	if section.Name != "Version" {
		t.Errorf("CheckVersion().Name = %q, want %q", section.Name, "Version")
	}
	if !section.NoIcons {
		t.Error("CheckVersion().NoIcons should be true")
	}
	if len(section.Results) != 6 {
		t.Fatalf("CheckVersion() should have 6 results, got %d", len(section.Results))
	}

	// Check version label includes version
	if section.Results[0].Label != "start v1.0.0" {
		t.Errorf("Version label = %q, want %q", section.Results[0].Label, "start v1.0.0")
	}

	// Check index version
	indexResult := section.Results[5]
	if indexResult.Label != "Index" {
		t.Errorf("Index label = %q, want %q", indexResult.Label, "Index")
	}
	if indexResult.Message != "v0.3.2" {
		t.Errorf("Index message = %q, want %q", indexResult.Message, "v0.3.2")
	}
	if indexResult.Status != StatusInfo {
		t.Errorf("Index status = %v, want StatusInfo", indexResult.Status)
	}
}

func TestCheckVersion_IndexUnavailable(t *testing.T) {
	t.Parallel()
	info := BuildInfo{
		Version:   "v1.0.0",
		Commit:    "abc123",
		BuildDate: "2025-01-01",
		GoVersion: "go1.23.0",
		Platform:  "linux/amd64",
	}

	section := CheckVersion(info)

	if len(section.Results) != 6 {
		t.Fatalf("CheckVersion() should have 6 results, got %d", len(section.Results))
	}

	indexResult := section.Results[5]
	if indexResult.Status != StatusWarn {
		t.Errorf("Index status = %v, want StatusWarn", indexResult.Status)
	}
	if indexResult.Message != "unavailable" {
		t.Errorf("Index message = %q, want %q", indexResult.Message, "unavailable")
	}
}

func TestCheckVersion_WithCustomIndexPath(t *testing.T) {
	t.Parallel()
	info := BuildInfo{
		Version:      "v1.0.0",
		Commit:       "abc123",
		BuildDate:    "2025-01-01",
		GoVersion:    "go1.23.0",
		Platform:     "linux/amd64",
		IndexVersion: "v0.3.2",
		IndexPath:    "github.com/example/custom-library/index@v0",
	}

	section := CheckVersion(info)

	var sourceResult *CheckResult
	for i := range section.Results {
		if section.Results[i].Label == "Index Source" {
			sourceResult = &section.Results[i]
			break
		}
	}
	if sourceResult == nil {
		t.Fatal("CheckVersion() missing 'Index Source' result")
		return
	}
	if sourceResult.Message != "github.com/example/custom-library/index@v0" {
		t.Errorf("Index Source message = %q, want %q", sourceResult.Message, "github.com/example/custom-library/index@v0")
	}
	if sourceResult.Status != StatusInfo {
		t.Errorf("Index Source status = %v, want StatusInfo", sourceResult.Status)
	}
}

func TestCheckVersion_NoIndexPath(t *testing.T) {
	t.Parallel()
	info := BuildInfo{
		Version:      "v1.0.0",
		Commit:       "abc123",
		BuildDate:    "2025-01-01",
		GoVersion:    "go1.23.0",
		Platform:     "linux/amd64",
		IndexVersion: "v0.3.2",
		// IndexPath not set — default behaviour
	}

	section := CheckVersion(info)

	for _, r := range section.Results {
		if r.Label == "Index Source" {
			t.Errorf("CheckVersion() without IndexPath should not include 'Index Source' result")
		}
	}
}

func TestCheckConfiguration_NoConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	paths := config.Paths{
		Global:       filepath.Join(tmpDir, "global"),
		Local:        filepath.Join(tmpDir, "local"),
		GlobalExists: false,
		LocalExists:  false,
	}

	section := CheckConfiguration(paths)

	if section.Name != "Configuration" {
		t.Errorf("CheckConfiguration().Name = %q, want %q", section.Name, "Configuration")
	}

	// Should have 4 results (global header, global "Not found", local header, local "Not found")
	if len(section.Results) != 4 {
		t.Fatalf("CheckConfiguration() should have 4 results, got %d", len(section.Results))
	}

	// Headers should be NoIcon with scope in label
	if !section.Results[0].NoIcon {
		t.Error("Global header should have NoIcon=true")
	}
	if !strings.Contains(section.Results[0].Label, "Global") {
		t.Errorf("Global header label should contain 'Global', got %q", section.Results[0].Label)
	}
	if !section.Results[2].NoIcon {
		t.Error("Local header should have NoIcon=true")
	}
	if !strings.Contains(section.Results[2].Label, "Local") {
		t.Errorf("Local header label should contain 'Local', got %q", section.Results[2].Label)
	}

	// Children should be indented info results with "Not found" label
	for _, i := range []int{1, 3} {
		r := section.Results[i]
		if r.Status != StatusInfo {
			t.Errorf("Result[%d] status should be StatusInfo, got %v", i, r.Status)
		}
		if r.Label != "Not found" {
			t.Errorf("Result[%d] label should be 'Not found', got %q", i, r.Label)
		}
		if r.Indent != 1 {
			t.Errorf("Result[%d] indent should be 1, got %d", i, r.Indent)
		}
	}
}

func TestCheckConfiguration_ValidConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write valid CUE file
	cueContent := `settings: { default_agent: "test" }`
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"), []byte(cueContent), 0644); err != nil {
		t.Fatal(err)
	}

	paths := config.Paths{
		Global:       globalDir,
		Local:        filepath.Join(tmpDir, "local"),
		GlobalExists: true,
		LocalExists:  false,
	}

	section := CheckConfiguration(paths)

	// Should have results for global (header + file), local, and validation
	hasPass := false
	for _, r := range section.Results {
		if r.Status == StatusPass {
			hasPass = true
		}
	}
	if !hasPass {
		t.Error("Valid config should have at least one StatusPass result")
	}
}

func TestCheckConfiguration_InvalidConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write invalid CUE file
	cueContent := `this is not valid cue {{{`
	if err := os.WriteFile(filepath.Join(globalDir, "bad.cue"), []byte(cueContent), 0644); err != nil {
		t.Fatal(err)
	}

	paths := config.Paths{
		Global:       globalDir,
		Local:        filepath.Join(tmpDir, "local"),
		GlobalExists: true,
		LocalExists:  false,
	}

	section := CheckConfiguration(paths)

	// Should have a failure result
	hasFail := false
	for _, r := range section.Results {
		if r.Status == StatusFail {
			hasFail = true
		}
	}
	if !hasFail {
		t.Error("Invalid config should have StatusFail result")
	}
}

func TestCheckEnvironment(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	globalDir := filepath.Join(tmpDir, "global")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}

	paths := config.Paths{
		Global:       globalDir,
		Local:        filepath.Join(tmpDir, "local"),
		GlobalExists: true,
		LocalExists:  false,
	}

	section := CheckEnvironment(paths)

	if section.Name != "Environment" {
		t.Errorf("CheckEnvironment().Name = %q, want %q", section.Name, "Environment")
	}

	// Should have results for config directory and working directory
	if len(section.Results) < 2 {
		t.Errorf("CheckEnvironment() should have at least 2 results, got %d", len(section.Results))
	}

	// Config directory should be writable (we just created it)
	hasWritable := false
	for _, r := range section.Results {
		if r.Label == "Config directory" && r.Status == StatusPass {
			hasWritable = true
		}
	}
	if !hasWritable {
		t.Error("Config directory should be writable")
	}
}

func TestExpandPath(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/test", filepath.Join(home, "test")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := expandPath(tt.input); got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestShortenPath(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	tests := []struct {
		input string
		want  string
	}{
		{filepath.Join(home, "test"), "~/test"},
		{"/other/path", "/other/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := shortenPath(tt.input); got != tt.want {
				t.Errorf("shortenPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- CheckAgents tests ---

func TestCheckAgents_NoneConfigured(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString("{}")

	section := CheckAgents(v)

	if section.Name != "Agents" {
		t.Errorf("Name = %q, want %q", section.Name, "Agents")
	}
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusInfo {
		t.Errorf("status = %v, want StatusInfo", section.Results[0].Status)
	}
	if section.Results[0].Label != "None configured" {
		t.Errorf("label = %q, want %q", section.Results[0].Label, "None configured")
	}
}

func TestCheckAgents_ValidBinary(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`agents: { myagent: { bin: "go" } }`)

	section := CheckAgents(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
	if section.Results[0].Label != "myagent" {
		t.Errorf("label = %q, want %q", section.Results[0].Label, "myagent")
	}
	if section.Summary != "1 configured" {
		t.Errorf("summary = %q, want %q", section.Summary, "1 configured")
	}
}

func TestCheckAgents_MissingBinary(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`agents: { broken: { bin: "nonexistent-binary-xyz-123" } }`)

	section := CheckAgents(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusFail {
		t.Errorf("status = %v, want StatusFail", section.Results[0].Status)
	}
	if section.Results[0].Fix == "" {
		t.Error("expected a fix suggestion for missing binary")
	}
}

func TestCheckAgents_NoBinField(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`agents: { nobin: { description: "no bin" } }`)

	section := CheckAgents(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusWarn {
		t.Errorf("status = %v, want StatusWarn", section.Results[0].Status)
	}
	if section.Results[0].Message != "No bin field" {
		t.Errorf("message = %q, want %q", section.Results[0].Message, "No bin field")
	}
}

// --- CheckRoles tests ---

func TestCheckRoles_NoneConfigured(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString("{}")

	section := CheckRoles(v)

	if section.Name != "Roles" {
		t.Errorf("Name = %q, want %q", section.Name, "Roles")
	}
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Label != "None configured" {
		t.Errorf("label = %q, want %q", section.Results[0].Label, "None configured")
	}
}

func TestCheckRoles_FileExists(t *testing.T) {
	t.Parallel()
	tmpFile := filepath.Join(t.TempDir(), "role.md")
	if err := os.WriteFile(tmpFile, []byte("role content"), 0644); err != nil {
		t.Fatal(err)
	}

	cctx := cuecontext.New()
	v := cctx.CompileString(`roles: { myrole: { file: "` + tmpFile + `" } }`)

	section := CheckRoles(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
}

func TestCheckRoles_FileMissing(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`roles: { badrole: { file: "/nonexistent/path/role.md" } }`)

	section := CheckRoles(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusNotFound {
		t.Errorf("status = %v, want StatusNotFound", section.Results[0].Status)
	}
}

func TestCheckRoles_PromptFallback(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`roles: { prole: { prompt: "You are a code reviewer" } }`)

	section := CheckRoles(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
	if section.Results[0].Message != "(inline prompt)" {
		t.Errorf("message = %q, want %q", section.Results[0].Message, "(inline prompt)")
	}
}

// TestCheckRoles_ModulePath covers the three @module/ outcomes:
// extract present (StatusPass), extract missing with origin (StatusNotFound),
// and origin missing (StatusFail). Each scenario is the only legitimate
// answer doctor can give without lying, and the Fix string is the user's
// recovery path. t.Setenv precludes t.Parallel.
func TestCheckRoles_ModulePath(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("CUE_CACHE_DIR", cacheDir)

	moduleDir := filepath.Join(cacheDir, "mod", "extract",
		"github.com", "test", "roles", "library", "assistant@v1.0.0")
	if err := os.MkdirAll(moduleDir, 0755); err != nil {
		t.Fatalf("mkdir module cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "role.md"),
		[]byte("Module role"), 0644); err != nil {
		t.Fatalf("write role.md: %v", err)
	}

	t.Run("extract present", func(t *testing.T) {
		cctx := cuecontext.New()
		v := cctx.CompileString(`roles: { myrole: {
			origin: "github.com/test/roles/library/assistant@v1.0.0"
			file: "@module/role.md"
		} }`)

		section := CheckRoles(v)
		if len(section.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(section.Results))
		}
		r := section.Results[0]
		if r.Status != StatusPass {
			t.Errorf("status = %v, want StatusPass", r.Status)
		}
		if !strings.Contains(r.Message, "v1.0.0") {
			t.Errorf("message = %q, want it to include the version", r.Message)
		}
		if r.Message == "(registry module)" {
			t.Errorf("message = %q, must differ from the legacy unverified placeholder", r.Message)
		}
	})

	t.Run("fallback resolves to cached version", func(t *testing.T) {
		// Config declares v2.0.0 but the only cached version is the
		// v1.0.0 fabricated at parent test setup. ResolveModulePath falls
		// back to v1.0.0. The pass-case message must reflect what was
		// actually resolved, not the version declared in config.
		cctx := cuecontext.New()
		v := cctx.CompileString(`roles: { myrole: {
			origin: "github.com/test/roles/library/assistant@v2.0.0"
			file: "@module/role.md"
		} }`)

		section := CheckRoles(v)
		if len(section.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(section.Results))
		}
		r := section.Results[0]
		if r.Status != StatusPass {
			t.Errorf("status = %v, want StatusPass", r.Status)
		}
		if strings.Contains(r.Message, "v2.0.0") {
			t.Errorf("message = %q, must not claim the declared v2.0.0 when the fallback resolved to v1.0.0", r.Message)
		}
		if !strings.Contains(r.Message, "v1.0.0") {
			t.Errorf("message = %q, want it to include the resolved version v1.0.0", r.Message)
		}
	})

	t.Run("extract present but file missing", func(t *testing.T) {
		// Same extract dir as the happy path, but the role's file: field
		// references a file that the module does not contain. Install
		// will not fix this — the Fix must point at the file path, not
		// the module.
		cctx := cuecontext.New()
		v := cctx.CompileString(`roles: { myrole: {
			origin: "github.com/test/roles/library/assistant@v1.0.0"
			file: "@module/nonexistent.md"
		} }`)

		section := CheckRoles(v)
		if len(section.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(section.Results))
		}
		r := section.Results[0]
		if r.Status != StatusNotFound {
			t.Errorf("status = %v, want StatusNotFound", r.Status)
		}
		if strings.Contains(r.Fix, "modules install") {
			t.Errorf("fix = %q, must not advise reinstall when the extract is present", r.Fix)
		}
		if !strings.Contains(r.Fix, "@module/nonexistent.md") {
			t.Errorf("fix = %q, want it to mention the offending file path", r.Fix)
		}
	})

	t.Run("stat returns non-IsNotExist error", func(t *testing.T) {
		// role.md exists as a regular file, so traversing through it as
		// "role.md/sub.md" returns ENOTDIR — distinct from IsNotExist.
		// Doctor must classify this as a real failure (StatusFail), not
		// a missing-file misconfiguration, since the recovery path is
		// different.
		cctx := cuecontext.New()
		v := cctx.CompileString(`roles: { myrole: {
			origin: "github.com/test/roles/library/assistant@v1.0.0"
			file: "@module/role.md/sub.md"
		} }`)

		section := CheckRoles(v)
		if len(section.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(section.Results))
		}
		r := section.Results[0]
		if r.Status != StatusFail {
			t.Errorf("status = %v, want StatusFail for non-IsNotExist stat error", r.Status)
		}
	})

	t.Run("extract missing with origin", func(t *testing.T) {
		cctx := cuecontext.New()
		v := cctx.CompileString(`roles: { myrole: {
			origin: "github.com/test/roles/missing/mod@v9.9.9"
			file: "@module/role.md"
		} }`)

		section := CheckRoles(v)
		if len(section.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(section.Results))
		}
		r := section.Results[0]
		if r.Status != StatusNotFound {
			t.Errorf("status = %v, want StatusNotFound", r.Status)
		}
		if !strings.Contains(r.Fix, "modules install") {
			t.Errorf("fix = %q, want it to mention 'modules install'", r.Fix)
		}
	})

	t.Run("origin missing", func(t *testing.T) {
		cctx := cuecontext.New()
		v := cctx.CompileString(`roles: { myrole: {
			file: "@module/role.md"
		} }`)

		section := CheckRoles(v)
		if len(section.Results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(section.Results))
		}
		r := section.Results[0]
		if r.Status != StatusFail {
			t.Errorf("status = %v, want StatusFail", r.Status)
		}
		if !strings.Contains(r.Fix, "origin") {
			t.Errorf("fix = %q, want it to mention the origin field", r.Fix)
		}
	})
}

func TestCheckRoles_NoFileOrPrompt(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`roles: { emptyrole: { description: "nothing useful" } }`)

	section := CheckRoles(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusWarn {
		t.Errorf("status = %v, want StatusWarn", section.Results[0].Status)
	}
	if section.Results[0].Message != "No file, prompt, or command" {
		t.Errorf("message = %q, want %q", section.Results[0].Message, "No file, prompt, or command")
	}
}

// --- CheckContexts tests ---

func TestCheckContexts_NoneConfigured(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString("{}")

	section := CheckContexts(v)

	if section.Name != "Contexts" {
		t.Errorf("Name = %q, want %q", section.Name, "Contexts")
	}
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Label != "None configured" {
		t.Errorf("label = %q, want %q", section.Results[0].Label, "None configured")
	}
}

func TestCheckContexts_FileExists(t *testing.T) {
	t.Parallel()
	tmpFile := filepath.Join(t.TempDir(), "context.md")
	if err := os.WriteFile(tmpFile, []byte("context content"), 0644); err != nil {
		t.Fatal(err)
	}

	cctx := cuecontext.New()
	v := cctx.CompileString(`contexts: { myctx: { file: "` + tmpFile + `" } }`)

	section := CheckContexts(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
}

// Missing context files are reported as StatusNotFound regardless of the
// `required` field. `required` is a composition rule used by the orchestrator
// to decide which contexts to include, not a doctor severity rule.
func TestCheckContexts_FileMissing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		config string
	}{
		{
			name:   "required",
			config: `contexts: { reqctx: { file: "/nonexistent/path/file.md", required: true } }`,
		},
		{
			name:   "optional",
			config: `contexts: { optctx: { file: "/nonexistent/path/file.md", required: false } }`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cctx := cuecontext.New()
			v := cctx.CompileString(tc.config)

			section := CheckContexts(v)

			if len(section.Results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(section.Results))
			}
			if section.Results[0].Status != StatusNotFound {
				t.Errorf("status = %v, want StatusNotFound", section.Results[0].Status)
			}
		})
	}
}

func TestCheckContexts_InlinePrompt(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`contexts: { promptctx: { prompt: "You are helpful" } }`)

	section := CheckContexts(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
	if section.Results[0].Message != "(inline prompt)" {
		t.Errorf("message = %q, want %q", section.Results[0].Message, "(inline prompt)")
	}
}

func TestCheckContexts_Command(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`contexts: { cmdctx: { command: "echo hello" } }`)

	section := CheckContexts(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
	if section.Results[0].Message != "(command)" {
		t.Errorf("message = %q, want %q", section.Results[0].Message, "(command)")
	}
}

// --- CheckTasks tests ---

func TestCheckTasks_NoneConfigured(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString("{}")

	section := CheckTasks(v)

	if section.Name != "Tasks" {
		t.Errorf("Name = %q, want %q", section.Name, "Tasks")
	}
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Label != "None configured" {
		t.Errorf("label = %q, want %q", section.Results[0].Label, "None configured")
	}
}

func TestCheckTasks_FileExists(t *testing.T) {
	t.Parallel()
	tmpFile := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(tmpFile, []byte("task content"), 0644); err != nil {
		t.Fatal(err)
	}

	cctx := cuecontext.New()
	v := cctx.CompileString(`tasks: { mytask: { file: "` + tmpFile + `" } }`)

	section := CheckTasks(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
	if section.Summary != "1 configured" {
		t.Errorf("summary = %q, want %q", section.Summary, "1 configured")
	}
}

func TestCheckTasks_InlinePrompt(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`tasks: { prompttask: { prompt: "Do the thing" } }`)

	section := CheckTasks(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
	if section.Results[0].Message != "(inline prompt)" {
		t.Errorf("message = %q, want %q", section.Results[0].Message, "(inline prompt)")
	}
}

// TestCheckTasks_ModulePathNoOrigin covers the failure case where an
// @module/ task path is declared without an origin field. The runtime
// cannot resolve such a path, so doctor must fail loudly rather than pass.
func TestCheckTasks_ModulePathNoOrigin(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`tasks: { modtask: { file: "@module/task.md" } }`)

	section := CheckTasks(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusFail {
		t.Errorf("status = %v, want StatusFail", section.Results[0].Status)
	}
	if !strings.Contains(section.Results[0].Fix, "origin") {
		t.Errorf("fix = %q, want it to mention the origin field", section.Results[0].Fix)
	}
}

func TestCheckTasks_RoleExists(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`
		roles: { dev: { prompt: "Developer role" } }
		tasks: { mytask: { prompt: "Do something", role: "dev" } }
	`)

	section := CheckTasks(v)

	// Should have 1 result for the task itself, no extra warning
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
}

func TestCheckTasks_RoleMissing(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`
		roles: { dev: { prompt: "Developer role" } }
		tasks: { mytask: { prompt: "Do something", role: "missing" } }
	`)

	section := CheckTasks(v)

	// Should have 2 results: task pass + role warning
	if len(section.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(section.Results))
	}
	if section.Results[1].Status != StatusWarn {
		t.Errorf("status = %v, want StatusWarn", section.Results[1].Status)
	}
	if section.Results[1].Indent != 1 {
		t.Errorf("indent = %d, want 1", section.Results[1].Indent)
	}
	if !strings.Contains(section.Results[1].Label, "missing") {
		t.Errorf("label = %q, should contain role name", section.Results[1].Label)
	}
}

func TestCheckTasks_NoRoleField(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`tasks: { mytask: { prompt: "Do something" } }`)

	section := CheckTasks(v)

	// Should have 1 result only (no role warning)
	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", section.Results[0].Status)
	}
}

func TestCheckTasks_FileMissing(t *testing.T) {
	t.Parallel()
	cctx := cuecontext.New()
	v := cctx.CompileString(`tasks: { badtask: { file: "/nonexistent/path/task.md" } }`)

	section := CheckTasks(v)

	if len(section.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(section.Results))
	}
	if section.Results[0].Status != StatusNotFound {
		t.Errorf("status = %v, want StatusNotFound", section.Results[0].Status)
	}
}

// --- CheckSettings tests ---

// settingsTestPaths creates a temp local directory and writes settings.cue if content is non-empty.
func settingsTestPaths(t *testing.T, content string) config.Paths {
	t.Helper()
	tmpDir := t.TempDir()
	localDir := filepath.Join(tmpDir, "local")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatal(err)
	}
	if content != "" {
		if err := os.WriteFile(filepath.Join(localDir, "settings.cue"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return config.Paths{
		Global:       filepath.Join(tmpDir, "global"),
		Local:        localDir,
		GlobalExists: false,
		LocalExists:  true,
	}
}

// findResult searches section results for a matching label.
func findResult(section SectionResult, label string) (CheckResult, bool) {
	for _, r := range section.Results {
		if r.Label == label {
			return r, true
		}
	}
	return CheckResult{}, false
}

func TestCheckSettings_ShowsAllSettings(t *testing.T) {
	t.Parallel()
	paths := settingsTestPaths(t, "")
	cctx := cuecontext.New()
	v := cctx.CompileString("{}")

	section := CheckSettings(paths, v)

	if section.Name != "Settings" {
		t.Errorf("Name = %q, want %q", section.Name, "Settings")
	}

	// Should have results for all 4 settings
	if len(section.Results) != len(config.SettingsRegistry) {
		t.Errorf("got %d results, want %d", len(section.Results), len(config.SettingsRegistry))
	}

	// default_agent should show as not set (no default)
	if r, ok := findResult(section, "default_agent"); ok {
		if r.Status != StatusInfo {
			t.Errorf("default_agent status = %v, want StatusInfo", r.Status)
		}
	} else {
		t.Error("missing default_agent result")
	}

	// library_index should have a default
	if r, ok := findResult(section, "library_index"); ok {
		if r.Status != StatusPass {
			t.Errorf("library_index status = %v, want StatusPass", r.Status)
		}
		if !strings.Contains(r.Message, "default") {
			t.Errorf("library_index message = %q, want containing 'default'", r.Message)
		}
	} else {
		t.Error("missing library_index result")
	}
}

func TestCheckSettings_DefaultAgentExists(t *testing.T) {
	t.Parallel()
	paths := settingsTestPaths(t, `settings: { default_agent: "claude" }`)
	cctx := cuecontext.New()
	v := cctx.CompileString(`
		agents: { claude: { bin: "echo" } }
		settings: { default_agent: "claude" }
	`)

	section := CheckSettings(paths, v)

	r, ok := findResult(section, "default_agent")
	if !ok {
		t.Fatal("missing default_agent result")
	}
	if r.Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", r.Status)
	}
	if !strings.Contains(r.Message, "claude") {
		t.Errorf("message = %q, want containing 'claude'", r.Message)
	}
	if !strings.Contains(r.Message, "local") {
		t.Errorf("message = %q, want containing 'local'", r.Message)
	}
}

func TestCheckSettings_DefaultAgentMissing(t *testing.T) {
	t.Parallel()
	paths := settingsTestPaths(t, `settings: { default_agent: "nonexistent" }`)
	cctx := cuecontext.New()
	v := cctx.CompileString(`
		agents: { claude: { bin: "echo" } }
		settings: { default_agent: "nonexistent" }
	`)

	section := CheckSettings(paths, v)

	r, ok := findResult(section, "default_agent")
	if !ok {
		t.Fatal("missing default_agent result")
	}
	if r.Status != StatusWarn {
		t.Errorf("status = %v, want StatusWarn", r.Status)
	}
	if r.Fix == "" {
		t.Error("expected a fix suggestion")
	}
}

func TestCheckSettings_DefaultAgentNoAgents(t *testing.T) {
	t.Parallel()
	paths := settingsTestPaths(t, `settings: { default_agent: "claude" }`)
	cctx := cuecontext.New()
	v := cctx.CompileString(`settings: { default_agent: "claude" }`)

	section := CheckSettings(paths, v)

	r, ok := findResult(section, "default_agent")
	if !ok {
		t.Fatal("missing default_agent result")
	}
	if r.Status != StatusWarn {
		t.Errorf("status = %v, want StatusWarn", r.Status)
	}
	if !strings.Contains(r.Fix, "No agents configured") {
		t.Errorf("fix = %q, should mention no agents configured", r.Fix)
	}
}

func TestCheckSettings_DefaultAgentNoConfig(t *testing.T) {
	t.Parallel()
	paths := settingsTestPaths(t, `settings: { default_agent: "claude" }`)
	var zero cue.Value // simulates config failed to load

	section := CheckSettings(paths, zero)

	r, ok := findResult(section, "default_agent")
	if !ok {
		t.Fatal("missing default_agent result")
	}
	if r.Status != StatusInfo {
		t.Errorf("status = %v, want StatusInfo", r.Status)
	}
	if !strings.Contains(r.Message, "claude") {
		t.Errorf("message = %q, want containing 'claude'", r.Message)
	}
	if !strings.Contains(r.Message, "cannot verify") {
		t.Errorf("message = %q, want containing 'cannot verify'", r.Message)
	}
}

func TestCheckSettings_ShellExists(t *testing.T) {
	t.Parallel()
	paths := settingsTestPaths(t, `settings: { shell: "sh" }`)
	cctx := cuecontext.New()
	v := cctx.CompileString(`settings: { shell: "sh" }`)

	section := CheckSettings(paths, v)

	r, ok := findResult(section, "shell")
	if !ok {
		t.Fatal("missing shell result")
	}
	if r.Status != StatusPass {
		t.Errorf("status = %v, want StatusPass", r.Status)
	}
}

func TestCheckSettings_ShellMissing(t *testing.T) {
	t.Parallel()
	paths := settingsTestPaths(t, `settings: { shell: "nonexistent-shell-xyz-123" }`)
	cctx := cuecontext.New()
	v := cctx.CompileString(`settings: { shell: "nonexistent-shell-xyz-123" }`)

	section := CheckSettings(paths, v)

	r, ok := findResult(section, "shell")
	if !ok {
		t.Fatal("missing shell result")
	}
	if r.Status != StatusWarn {
		t.Errorf("status = %v, want StatusWarn", r.Status)
	}
	if r.Fix == "" {
		t.Error("expected a fix suggestion")
	}
}

func TestCheckCache_missing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	section := CheckCache()

	if section.Name != "Cache" {
		t.Errorf("Name = %q, want %q", section.Name, "Cache")
	}
	if len(section.Results) != 1 {
		t.Fatalf("Results count = %d, want 1", len(section.Results))
	}
	if section.Results[0].Status != StatusNotFound {
		t.Errorf("Status = %v, want StatusNotFound", section.Results[0].Status)
	}
	if section.Results[0].Fix == "" {
		t.Error("expected fix suggestion for missing cache")
	}
}

func TestCheckCache_fresh(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	if err := cache.WriteIndex("test@v1.0.0"); err != nil {
		t.Fatalf("WriteIndex() error: %v", err)
	}

	section := CheckCache()

	if len(section.Results) != 1 {
		t.Fatalf("Results count = %d, want 1", len(section.Results))
	}
	if section.Results[0].Status != StatusPass {
		t.Errorf("Status = %v, want StatusPass", section.Results[0].Status)
	}
	if !strings.Contains(section.Results[0].Message, "fresh") {
		t.Errorf("Message = %q, should contain 'fresh'", section.Results[0].Message)
	}
}

func TestCheckCache_stale(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Write a cache file with a timestamp older than 24 hours.
	cacheDir := filepath.Join(tmp, "start")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	staleTime := time.Now().Add(-48 * time.Hour).Format(time.RFC3339)
	content := `index_updated: "` + staleTime + `"` + "\n" + `index_version: "test@v1.0.0"` + "\n"
	if err := os.WriteFile(filepath.Join(cacheDir, "cache.cue"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	section := CheckCache()

	if len(section.Results) != 1 {
		t.Fatalf("Results count = %d, want 1", len(section.Results))
	}
	if section.Results[0].Status != StatusWarn {
		t.Errorf("Status = %v, want StatusWarn", section.Results[0].Status)
	}
	if !strings.Contains(section.Results[0].Message, "stale") {
		t.Errorf("Message = %q, should contain 'stale'", section.Results[0].Message)
	}
	if section.Results[0].Fix == "" {
		t.Error("expected fix suggestion for stale cache")
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"zero", 0, "just now"},
		{"sub-second", 500 * time.Millisecond, "just now"},
		{"one second", 1 * time.Second, "1 second"},
		{"seconds", 30 * time.Second, "30 seconds"},
		{"one minute", 1 * time.Minute, "1 minute"},
		{"minutes", 5 * time.Minute, "5 minutes"},
		{"one hour", 1 * time.Hour, "1 hour"},
		{"hours", 3 * time.Hour, "3 hours"},
		{"one day", 24 * time.Hour, "1 day"},
		{"days", 72 * time.Hour, "3 days"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatDuration(tt.d)
			if got != tt.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
