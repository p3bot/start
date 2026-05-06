package cue

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cuelib "cuelang.org/go/cue"
)

func TestNewLoader(t *testing.T) {
	t.Parallel()
	l := NewLoader()
	if l == nil {
		t.Fatal("NewLoader() returned nil")
	}
	if l.ctx == nil {
		t.Fatal("NewLoader().ctx is nil")
	}
}

func TestLoader_Load(t *testing.T) {
	t.Parallel()
	t.Run("single directory with valid CUE", func(t *testing.T) {
		dir := t.TempDir()
		writeCUEFile(t, dir, "settings.cue", `
			name: "test"
			value: 42
		`)

		l := NewLoader()
		result, err := l.Load([]string{dir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if !result.GlobalLoaded {
			t.Error("GlobalLoaded = false, want true")
		}

		// Verify value was loaded
		name, err := result.Value.LookupPath(parsePath("name")).String()
		if err != nil {
			t.Fatalf("LookupPath(name) error = %v", err)
		}
		if name != "test" {
			t.Errorf("name = %q, want %q", name, "test")
		}
	})

	t.Run("two directories merge with additive semantics", func(t *testing.T) {
		globalDir := t.TempDir()
		localDir := t.TempDir()

		// Global config defines globalOnly field
		writeCUEFile(t, globalDir, "settings.cue", `
			globalOnly: true
			shared: "from-global"
		`)

		// Local config defines localOnly field
		// Different keys are additive (union)
		writeCUEFile(t, localDir, "settings.cue", `
			localOnly: true
		`)

		l := NewLoader()
		result, err := l.Load([]string{globalDir, localDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if !result.GlobalLoaded {
			t.Error("GlobalLoaded = false, want true")
		}
		if !result.LocalLoaded {
			t.Error("LocalLoaded = false, want true")
		}

		// Both unique fields should exist (additive for different keys)
		globalOnly, err := result.Value.LookupPath(parsePath("globalOnly")).Bool()
		if err != nil {
			t.Fatalf("LookupPath(globalOnly) error = %v", err)
		}
		if !globalOnly {
			t.Error("globalOnly = false, want true")
		}

		localOnly, err := result.Value.LookupPath(parsePath("localOnly")).Bool()
		if err != nil {
			t.Fatalf("LookupPath(localOnly) error = %v", err)
		}
		if !localOnly {
			t.Error("localOnly = false, want true")
		}

		// Global field should be present (not overridden by local)
		shared, err := result.Value.LookupPath(parsePath("shared")).String()
		if err != nil {
			t.Fatalf("LookupPath(shared) error = %v", err)
		}
		if shared != "from-global" {
			t.Errorf("shared = %q, want %q", shared, "from-global")
		}
	})

	t.Run("local replaces global for same key", func(t *testing.T) {
		globalDir := t.TempDir()
		localDir := t.TempDir()

		// Global config
		writeCUEFile(t, globalDir, "settings.cue", `
			name: "global-value"
			timeout: 30
		`)

		// Local config - same key should completely replace
		writeCUEFile(t, localDir, "settings.cue", `
			name: "local-value"
		`)

		l := NewLoader()
		result, err := l.Load([]string{globalDir, localDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Local should replace global for matching key
		name, err := result.Value.LookupPath(parsePath("name")).String()
		if err != nil {
			t.Fatalf("LookupPath(name) error = %v", err)
		}
		if name != "local-value" {
			t.Errorf("name = %q, want %q (local should replace global)", name, "local-value")
		}

		// Global-only field should still exist
		timeout, err := result.Value.LookupPath(parsePath("timeout")).Int64()
		if err != nil {
			t.Fatalf("LookupPath(timeout) error = %v", err)
		}
		if timeout != 30 {
			t.Errorf("timeout = %d, want 30", timeout)
		}
	})

	t.Run("skips non-existent directory", func(t *testing.T) {
		existingDir := t.TempDir()
		writeCUEFile(t, existingDir, "settings.cue", `name: "exists"`)

		nonExistent := filepath.Join(t.TempDir(), "does-not-exist")

		l := NewLoader()
		result, err := l.Load([]string{nonExistent, existingDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// First directory (global position) doesn't exist, so GlobalLoaded is false
		if result.GlobalLoaded {
			t.Error("GlobalLoaded = true, want false (non-existent dir was at global position)")
		}
		// Second directory (local position) was loaded
		if !result.LocalLoaded {
			t.Error("LocalLoaded = false, want true (existing dir was at local position)")
		}
	})

	t.Run("skips directory without CUE files", func(t *testing.T) {
		emptyDir := t.TempDir()
		cueDir := t.TempDir()
		writeCUEFile(t, cueDir, "settings.cue", `name: "cue"`)

		l := NewLoader()
		result, err := l.Load([]string{emptyDir, cueDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// First directory (global position) has no CUE files, so GlobalLoaded is false
		if result.GlobalLoaded {
			t.Error("GlobalLoaded = true, want false (empty dir was at global position)")
		}
		// Second directory (local position) was loaded
		if !result.LocalLoaded {
			t.Error("LocalLoaded = false, want true (cue dir was at local position)")
		}
	})

	t.Run("error on no directories", func(t *testing.T) {
		l := NewLoader()
		_, err := l.Load([]string{})
		if err == nil {
			t.Fatal("Load([]) should return error")
		}
	})

	t.Run("error on no valid CUE found", func(t *testing.T) {
		emptyDir := t.TempDir()

		l := NewLoader()
		_, err := l.Load([]string{emptyDir})
		if err == nil {
			t.Fatal("Load() with empty dir should return error")
		}
	})

	t.Run("error on invalid CUE syntax", func(t *testing.T) {
		dir := t.TempDir()
		writeCUEFile(t, dir, "bad.cue", `
			this is not valid CUE {{{
		`)

		l := NewLoader()
		_, err := l.Load([]string{dir})
		if err == nil {
			t.Fatal("Load() with invalid CUE should return error")
		}
	})

	t.Run("replacement allows type changes", func(t *testing.T) {
		globalDir := t.TempDir()
		localDir := t.TempDir()

		// Global defines name as string
		writeCUEFile(t, globalDir, "settings.cue", `name: "global"`)

		// Local defines name as number - replacement allows this
		writeCUEFile(t, localDir, "settings.cue", `name: 42`)

		l := NewLoader()
		result, err := l.Load([]string{globalDir, localDir})
		if err != nil {
			t.Fatalf("Load() error = %v (replacement should allow type changes)", err)
		}

		// Local value should replace global
		name, err := result.Value.LookupPath(parsePath("name")).Int64()
		if err != nil {
			t.Fatalf("LookupPath(name) error = %v", err)
		}
		if name != 42 {
			t.Errorf("name = %d, want 42", name)
		}
	})
}

func TestLoader_LoadSingle(t *testing.T) {
	t.Parallel()
	t.Run("valid directory", func(t *testing.T) {
		dir := t.TempDir()
		writeCUEFile(t, dir, "settings.cue", `value: 123`)

		l := NewLoader()
		v, err := l.LoadSingle(dir)
		if err != nil {
			t.Fatalf("LoadSingle() error = %v", err)
		}

		val, err := v.LookupPath(parsePath("value")).Int64()
		if err != nil {
			t.Fatalf("LookupPath(value) error = %v", err)
		}
		if val != 123 {
			t.Errorf("value = %d, want 123", val)
		}
	})

	t.Run("empty path", func(t *testing.T) {
		l := NewLoader()
		_, err := l.LoadSingle("")
		if err == nil {
			t.Fatal("LoadSingle(\"\") should return error")
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		l := NewLoader()
		_, err := l.LoadSingle("/does/not/exist")
		if err == nil {
			t.Fatal("LoadSingle() with non-existent dir should return error")
		}
	})

	t.Run("file instead of directory", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "file.txt")
		if err := os.WriteFile(file, []byte("content"), 0o644); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}

		l := NewLoader()
		_, err := l.LoadSingle(file)
		if err == nil {
			t.Fatal("LoadSingle() with file path should return error")
		}
	})

	t.Run("no CUE files", func(t *testing.T) {
		dir := t.TempDir()

		l := NewLoader()
		_, err := l.LoadSingle(dir)
		if err == nil {
			t.Fatal("LoadSingle() with no CUE files should return error")
		}
	})
}

func TestLoader_Context(t *testing.T) {
	t.Parallel()
	l := NewLoader()
	ctx := l.Context()
	if ctx == nil {
		t.Fatal("Context() returned nil")
	}

	// Verify the context is functional
	v := ctx.CompileString(`test: "value"`)
	if err := v.Err(); err != nil {
		t.Fatalf("Context not functional: %v", err)
	}
}

func TestHasCUEFiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{
			name:  "has cue file",
			files: map[string]string{"settings.cue": "x: 1"},
			want:  true,
		},
		{
			name:  "multiple cue files",
			files: map[string]string{"a.cue": "x: 1", "b.cue": "y: 2"},
			want:  true,
		},
		{
			name:  "no files",
			files: map[string]string{},
			want:  false,
		},
		{
			name:  "only non-cue files",
			files: map[string]string{"readme.md": "# test", "config.json": "{}"},
			want:  false,
		},
		{
			name:  "mixed files",
			files: map[string]string{"readme.md": "# test", "settings.cue": "x: 1"},
			want:  true,
		},
		{
			name:  "file named cue without extension",
			files: map[string]string{"cue": "not a cue file"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("Failed to create file %s: %v", name, err)
				}
			}

			got, err := HasCUEFiles(dir)
			if err != nil {
				t.Fatalf("HasCUEFiles() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("HasCUEFiles() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoader_MergeWithTestdataFixtures(t *testing.T) {
	t.Parallel()
	// Derive fixture paths from this file's location so the test is not sensitive
	// to the working directory at invocation time.
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(currentFile), "../..")
	globalDir := filepath.Join(repoRoot, "test/testdata/merge/global")
	localDir := filepath.Join(repoRoot, "test/testdata/merge/local")

	if _, err := os.Stat(globalDir); os.IsNotExist(err) {
		t.Skip("testdata/merge fixtures not found")
	}

	l := NewLoader()
	result, err := l.Load([]string{globalDir, localDir})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify both directories loaded
	if !result.GlobalLoaded {
		t.Error("GlobalLoaded = false, want true")
	}
	if !result.LocalLoaded {
		t.Error("LocalLoaded = false, want true")
	}

	// Verify agents from both sources exist (additive merge)
	claudeCmd, err := result.Value.LookupPath(parsePath("agents.claude.command")).String()
	if err != nil {
		t.Fatalf("agents.claude.command error = %v (should exist from global)", err)
	}
	if claudeCmd == "" {
		t.Error("agents.claude.command should not be empty")
	}

	geminiCmd, err := result.Value.LookupPath(parsePath("agents.gemini.command")).String()
	if err != nil {
		t.Fatalf("agents.gemini.command error = %v (should exist from local)", err)
	}
	if geminiCmd == "" {
		t.Error("agents.gemini.command should not be empty")
	}

	// Verify contexts from both sources exist
	envFile, err := result.Value.LookupPath(parsePath("contexts.environment.file")).String()
	if err != nil {
		t.Fatalf("contexts.environment.file error = %v", err)
	}
	if envFile == "" {
		t.Error("contexts.environment.file should not be empty")
	}

	projectFile, err := result.Value.LookupPath(parsePath("contexts.project.file")).String()
	if err != nil {
		t.Fatalf("contexts.project.file error = %v", err)
	}
	if projectFile == "" {
		t.Error("contexts.project.file should not be empty")
	}

	// Verify roles from both sources exist
	if !result.Value.LookupPath(parsePath("roles.assistant")).Exists() {
		t.Error("roles.assistant should exist from global")
	}
	if !result.Value.LookupPath(parsePath("roles.reviewer")).Exists() {
		t.Error("roles.reviewer should exist from local")
	}

	// Verify settings field-level merge
	// Global timeout should persist (not overridden by local)
	timeout, err := result.Value.LookupPath(parsePath("settings.timeout")).Int64()
	if err != nil {
		t.Fatalf("settings.timeout error = %v", err)
	}
	if timeout != 120 {
		t.Errorf("settings.timeout = %d, want 120 (from global)", timeout)
	}

	// Global shell should persist
	shell, err := result.Value.LookupPath(parsePath("settings.shell")).String()
	if err != nil {
		t.Fatalf("settings.shell error = %v", err)
	}
	if shell != "/bin/bash" {
		t.Errorf("settings.shell = %q, want %q (from global)", shell, "/bin/bash")
	}

	// Local should override default_agent
	agent, err := result.Value.LookupPath(parsePath("settings.default_agent")).String()
	if err != nil {
		t.Fatalf("settings.default_agent error = %v", err)
	}
	if agent != "gemini" {
		t.Errorf("settings.default_agent = %q, want %q (local override)", agent, "gemini")
	}
}

func TestLoader_MergeSemantics(t *testing.T) {
	t.Parallel()
	t.Run("collections merge additively by item name", func(t *testing.T) {
		globalDir := t.TempDir()
		localDir := t.TempDir()

		// Global defines agents.claude
		writeCUEFile(t, globalDir, "settings.cue", `
			agents: {
				claude: {
					command: "claude"
					bin: "claude"
				}
			}
		`)

		// Local defines agents.gemini
		writeCUEFile(t, localDir, "settings.cue", `
			agents: {
				gemini: {
					command: "gemini"
					bin: "gemini"
				}
			}
		`)

		l := NewLoader()
		result, err := l.Load([]string{globalDir, localDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Both agents should exist
		claudeCmd, err := result.Value.LookupPath(parsePath("agents.claude.command")).String()
		if err != nil {
			t.Fatalf("LookupPath(agents.claude.command) error = %v", err)
		}
		if claudeCmd != "claude" {
			t.Errorf("agents.claude.command = %q, want %q", claudeCmd, "claude")
		}

		geminiCmd, err := result.Value.LookupPath(parsePath("agents.gemini.command")).String()
		if err != nil {
			t.Fatalf("LookupPath(agents.gemini.command) error = %v", err)
		}
		if geminiCmd != "gemini" {
			t.Errorf("agents.gemini.command = %q, want %q", geminiCmd, "gemini")
		}
	})

	t.Run("same-named collection items replaced entirely", func(t *testing.T) {
		globalDir := t.TempDir()
		localDir := t.TempDir()

		// Global defines roles.reviewer with multiple fields
		writeCUEFile(t, globalDir, "settings.cue", `
			roles: {
				reviewer: {
					description: "Global reviewer"
					prompt: "You are a global reviewer."
					timeout: 60
				}
			}
		`)

		// Local defines roles.reviewer with different fields
		// This should completely replace, not merge fields
		writeCUEFile(t, localDir, "settings.cue", `
			roles: {
				reviewer: {
					description: "Local reviewer"
					prompt: "You are a local reviewer."
				}
			}
		`)

		l := NewLoader()
		result, err := l.Load([]string{globalDir, localDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Description should be from local
		desc, err := result.Value.LookupPath(parsePath("roles.reviewer.description")).String()
		if err != nil {
			t.Fatalf("LookupPath(roles.reviewer.description) error = %v", err)
		}
		if desc != "Local reviewer" {
			t.Errorf("roles.reviewer.description = %q, want %q", desc, "Local reviewer")
		}

		// Timeout should NOT exist (local replaced entirely, didn't merge fields)
		timeoutPath := result.Value.LookupPath(parsePath("roles.reviewer.timeout"))
		if timeoutPath.Exists() {
			t.Error("roles.reviewer.timeout should not exist (local replaces entire item)")
		}
	})

	t.Run("settings fields merge additively", func(t *testing.T) {
		globalDir := t.TempDir()
		localDir := t.TempDir()

		// Global settings
		writeCUEFile(t, globalDir, "settings.cue", `
			settings: {
				timeout: 120
				shell: "/bin/bash"
				default_agent: "claude"
			}
		`)

		// Local settings - only overrides some fields
		writeCUEFile(t, localDir, "settings.cue", `
			settings: {
				default_agent: "gemini"
			}
		`)

		l := NewLoader()
		result, err := l.Load([]string{globalDir, localDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Global-only field should persist
		timeout, err := result.Value.LookupPath(parsePath("settings.timeout")).Int64()
		if err != nil {
			t.Fatalf("LookupPath(settings.timeout) error = %v", err)
		}
		if timeout != 120 {
			t.Errorf("settings.timeout = %d, want 120", timeout)
		}

		// Global field should persist
		shell, err := result.Value.LookupPath(parsePath("settings.shell")).String()
		if err != nil {
			t.Fatalf("LookupPath(settings.shell) error = %v", err)
		}
		if shell != "/bin/bash" {
			t.Errorf("settings.shell = %q, want %q", shell, "/bin/bash")
		}

		// Overridden field should be from local
		agent, err := result.Value.LookupPath(parsePath("settings.default_agent")).String()
		if err != nil {
			t.Fatalf("LookupPath(settings.default_agent) error = %v", err)
		}
		if agent != "gemini" {
			t.Errorf("settings.default_agent = %q, want %q", agent, "gemini")
		}
	})

	t.Run("all collection types merge correctly", func(t *testing.T) {
		globalDir := t.TempDir()
		localDir := t.TempDir()

		// Global with all collection types
		writeCUEFile(t, globalDir, "settings.cue", `
			agents: {
				claude: { command: "claude" }
			}
			contexts: {
				environment: { file: "env.md" }
			}
			roles: {
				assistant: { prompt: "helpful" }
			}
			tasks: {
				review: { prompt: "review code" }
			}
		`)

		// Local adds to each collection
		writeCUEFile(t, localDir, "settings.cue", `
			agents: {
				gemini: { command: "gemini" }
			}
			contexts: {
				project: { file: "project.md" }
			}
			roles: {
				reviewer: { prompt: "reviewer" }
			}
			tasks: {
				test: { prompt: "run tests" }
			}
		`)

		l := NewLoader()
		result, err := l.Load([]string{globalDir, localDir})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		// Verify all items from both sources exist
		checks := []string{
			"agents.claude.command",
			"agents.gemini.command",
			"contexts.environment.file",
			"contexts.project.file",
			"roles.assistant.prompt",
			"roles.reviewer.prompt",
			"tasks.review.prompt",
			"tasks.test.prompt",
		}

		for _, path := range checks {
			if !result.Value.LookupPath(parsePath(path)).Exists() {
				t.Errorf("%s should exist", path)
			}
		}
	})
}

func TestLoader_LoadWithPackage(t *testing.T) {
	t.Parallel()
	t.Run("loads file with package declaration", func(t *testing.T) {
		dir := t.TempDir()
		writeCUEFile(t, dir, "settings.cue", `
			package myconfig

			name: "test"
			value: 42
		`)

		l := NewLoader()
		v, err := l.LoadSingle(dir)
		if err != nil {
			t.Fatalf("LoadSingle() error = %v", err)
		}

		name, err := v.LookupPath(parsePath("name")).String()
		if err != nil {
			t.Fatalf("LookupPath(name) error = %v", err)
		}
		if name != "test" {
			t.Errorf("name = %q, want %q", name, "test")
		}
	})

	t.Run("loads mixed files with and without package", func(t *testing.T) {
		dir := t.TempDir()
		// File without package
		writeCUEFile(t, dir, "a.cue", `foo: "bar"`)
		// Another file without package
		writeCUEFile(t, dir, "b.cue", `baz: 123`)

		l := NewLoader()
		v, err := l.LoadSingle(dir)
		if err != nil {
			t.Fatalf("LoadSingle() error = %v", err)
		}

		foo, err := v.LookupPath(parsePath("foo")).String()
		if err != nil {
			t.Fatalf("LookupPath(foo) error = %v", err)
		}
		if foo != "bar" {
			t.Errorf("foo = %q, want %q", foo, "bar")
		}

		baz, err := v.LookupPath(parsePath("baz")).Int64()
		if err != nil {
			t.Fatalf("LookupPath(baz) error = %v", err)
		}
		if baz != 123 {
			t.Errorf("baz = %d, want 123", baz)
		}
	})
}

func TestIdentifyBrokenFiles(t *testing.T) {
	t.Parallel()

	t.Run("all valid returns combined-failure message", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		validPath := filepath.Join(dir, "valid.cue")
		writeCUEFile(t, dir, "valid.cue", `x: 1`)

		result := IdentifyBrokenFiles([]string{validPath})
		if !strings.Contains(result, "(files parse individually but fail when combined)") {
			t.Errorf("expected fallback message, got: %q", result)
		}
	})

	t.Run("broken file is identified", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		validPath := filepath.Join(dir, "valid.cue")
		brokenPath := filepath.Join(dir, "broken.cue")
		writeCUEFile(t, dir, "valid.cue", `x: 1`)
		writeCUEFile(t, dir, "broken.cue", `this is not {{ valid cue`)

		result := IdentifyBrokenFiles([]string{validPath, brokenPath})
		if !strings.Contains(result, "broken.cue") {
			t.Errorf("expected broken.cue in result, got: %q", result)
		}
		if strings.Contains(result, "(files parse individually but fail when combined)") {
			t.Errorf("should not show fallback message when broken files exist, got: %q", result)
		}
	})

	t.Run("unreadable file is reported", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		missing := filepath.Join(dir, "nonexistent.cue")

		result := IdentifyBrokenFiles([]string{missing})
		if !strings.Contains(result, "nonexistent.cue") {
			t.Errorf("expected nonexistent.cue in result, got: %q", result)
		}
	})

	t.Run("empty list returns combined-failure message", func(t *testing.T) {
		t.Parallel()
		result := IdentifyBrokenFiles([]string{})
		if !strings.Contains(result, "(files parse individually but fail when combined)") {
			t.Errorf("expected fallback message for empty list, got: %q", result)
		}
	})
}

// Helper functions

func writeCUEFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write CUE file %s: %v", path, err)
	}
}

func parsePath(path string) cuelib.Path {
	return cuelib.ParsePath(path)
}
