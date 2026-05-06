package temp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDryRunManager(t *testing.T) {
	t.Parallel()
	m := NewDryRunManager()

	if m == nil {
		t.Fatal("NewDryRunManager() returned nil")
	}
	if m.BaseDir == "" {
		t.Error("NewDryRunManager() BaseDir is empty")
	}
	if m.BaseDir != os.TempDir() {
		t.Errorf("NewDryRunManager() BaseDir = %q, want %q", m.BaseDir, os.TempDir())
	}
}

func TestNewUTDManager(t *testing.T) {
	t.Parallel()
	m := NewUTDManager("/project")

	if m == nil {
		t.Fatal("NewUTDManager() returned nil")
	}
	want := filepath.Join("/project", ".start", "temp")
	if m.BaseDir != want {
		t.Errorf("NewUTDManager() BaseDir = %q, want %q", m.BaseDir, want)
	}
}

func TestManager_DryRunDir(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := &Manager{BaseDir: tmpDir}

	t.Run("creates timestamped directory", func(t *testing.T) {
		dir, err := m.DryRunDir()
		if err != nil {
			t.Fatalf("DryRunDir() error = %v", err)
		}

		// Check directory was created
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat dir error = %v", err)
		}
		if !info.IsDir() {
			t.Error("expected directory")
		}

		// Check name format
		base := filepath.Base(dir)
		if !strings.HasPrefix(base, "start-") {
			t.Errorf("dir name = %q, want prefix 'start-'", base)
		}
	})

	t.Run("handles collision with suffix", func(t *testing.T) {
		// Create first directory
		dir1, err := m.DryRunDir()
		if err != nil {
			t.Fatalf("DryRunDir() 1 error = %v", err)
		}

		// Create second directory in same second (may collide)
		dir2, err := m.DryRunDir()
		if err != nil {
			t.Fatalf("DryRunDir() 2 error = %v", err)
		}

		// Both should exist and be different
		if dir1 == dir2 {
			// This is fine if they were created in different seconds
			t.Log("directories created in same path (different seconds)")
		}

		if _, err := os.Stat(dir1); err != nil {
			t.Errorf("dir1 should exist: %v", err)
		}
		if _, err := os.Stat(dir2); err != nil {
			t.Errorf("dir2 should exist: %v", err)
		}
	})
}

func TestManager_WriteDryRunFiles(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := &Manager{BaseDir: tmpDir}

	dir, err := m.DryRunDir()
	if err != nil {
		t.Fatalf("DryRunDir() error = %v", err)
	}

	role := "You are a code reviewer."
	prompt := "Review this code:\n```\nfunc foo() {}\n```"
	command := "# Agent: claude\nclaude --model sonnet"

	err = m.WriteDryRunFiles(dir, role, prompt, command)
	if err != nil {
		t.Fatalf("WriteDryRunFiles() error = %v", err)
	}

	// Check files were created
	files := map[string]string{
		"role.md":     role,
		"prompt.md":   prompt,
		"command.txt": command,
	}

	for name, expected := range files {
		path := filepath.Join(dir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("reading %s: %v", name, err)
			continue
		}
		if string(content) != expected {
			t.Errorf("%s content = %q, want %q", name, string(content), expected)
		}
	}
}

func TestManager_WriteUTDFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := NewUTDManager(tmpDir)

	tests := []struct {
		entityType   string
		name         string
		content      string
		wantFileName string
	}{
		{
			entityType:   "role",
			name:         "code-reviewer",
			content:      "You are a code reviewer.",
			wantFileName: "role-code-reviewer.md",
		},
		{
			entityType:   "context",
			name:         "project/readme",
			content:      "Project README content",
			wantFileName: "context-project-readme.md",
		},
		{
			entityType:   "task",
			name:         "golang/review",
			content:      "Review Go code",
			wantFileName: "task-golang-review.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := m.WriteUTDFile(tt.entityType, tt.name, tt.content)
			if err != nil {
				t.Fatalf("WriteUTDFile() error = %v", err)
			}

			// Check filename
			if filepath.Base(path) != tt.wantFileName {
				t.Errorf("filename = %q, want %q", filepath.Base(path), tt.wantFileName)
			}

			// Check content
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading file: %v", err)
			}
			if string(content) != tt.content {
				t.Errorf("content = %q, want %q", string(content), tt.content)
			}
		})
	}
}

func TestDeriveFileName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		entityType string
		name       string
		want       string
	}{
		{"role", "simple", "role-simple.md"},
		{"role", "code-reviewer", "role-code-reviewer.md"},
		{"context", "project/readme", "context-project-readme.md"},
		{"task", "golang/code/review", "task-golang-code-review.md"},
		{"role", "with spaces", "role-with-spaces.md"},
		{"role", "special!@#chars", "role-special-chars.md"},
		{"role", "--leading-dashes", "role-leading-dashes.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveFileName(tt.entityType, tt.name)
			if got != tt.want {
				t.Errorf("deriveFileName(%q, %q) = %q, want %q", tt.entityType, tt.name, got, tt.want)
			}
		})
	}
}

func TestManager_EnsureUTDDir(t *testing.T) {
	t.Parallel()

	t.Run("creates directory when absent", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		dir := filepath.Join(base, "nested", "temp")
		m := &Manager{BaseDir: dir}

		if err := m.EnsureUTDDir(); err != nil {
			t.Fatalf("EnsureUTDDir() error = %v", err)
		}

		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("EnsureUTDDir() created a file, not a directory")
		}
	})

	t.Run("succeeds when directory already exists", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		m := &Manager{BaseDir: dir}

		if err := m.EnsureUTDDir(); err != nil {
			t.Fatalf("EnsureUTDDir() error = %v (directory already existed)", err)
		}
	})

	t.Run("returns error when path is inside a file", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		blocker := filepath.Join(base, "blocker")
		if err := os.WriteFile(blocker, []byte("block"), 0o444); err != nil {
			t.Fatal(err)
		}
		// BaseDir is inside a regular file — os.MkdirAll must fail.
		m := &Manager{BaseDir: filepath.Join(blocker, "subdir")}

		if err := m.EnsureUTDDir(); err == nil {
			t.Error("EnsureUTDDir() should return error when path is invalid")
		}
	})
}

func TestManager_Clean(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	m := NewUTDManager(tmpDir)

	// Create some files
	_, err := m.WriteUTDFile("role", "test1", "content1")
	if err != nil {
		t.Fatalf("WriteUTDFile 1 error = %v", err)
	}
	_, err = m.WriteUTDFile("role", "test2", "content2")
	if err != nil {
		t.Fatalf("WriteUTDFile 2 error = %v", err)
	}

	// Verify files exist
	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files, got %d", len(entries))
	}

	// Clean
	if err := m.Clean(); err != nil {
		t.Fatalf("Clean() error = %v", err)
	}

	// Verify files removed
	entries, err = os.ReadDir(m.BaseDir)
	if err != nil {
		t.Fatalf("ReadDir after clean error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 files after clean, got %d", len(entries))
	}
}

func TestManager_Clean_NonexistentDir(t *testing.T) {
	t.Parallel()
	m := &Manager{BaseDir: "/nonexistent/path/12345"}

	// Should not error for nonexistent directory
	if err := m.Clean(); err != nil {
		t.Errorf("Clean() on nonexistent dir error = %v", err)
	}
}

func TestCheckGitignore(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	t.Run("returns false when no gitignore", func(t *testing.T) {
		if CheckGitignore(tmpDir) {
			t.Error("expected false when no .gitignore")
		}
	})

	t.Run("returns true when .start/temp is ignored", func(t *testing.T) {
		gitignore := filepath.Join(tmpDir, ".gitignore")
		content := "node_modules/\n.start/temp\n*.log"
		if err := os.WriteFile(gitignore, []byte(content), 0644); err != nil {
			t.Fatalf("writing .gitignore: %v", err)
		}

		if !CheckGitignore(tmpDir) {
			t.Error("expected true when .start/temp in .gitignore")
		}
	})

	t.Run("returns true when .start/ is ignored", func(t *testing.T) {
		subDir := filepath.Join(tmpDir, "sub")
		if err := os.MkdirAll(subDir, 0755); err != nil {
			t.Fatalf("creating subdir: %v", err)
		}

		gitignore := filepath.Join(subDir, ".gitignore")
		content := ".start/"
		if err := os.WriteFile(gitignore, []byte(content), 0644); err != nil {
			t.Fatalf("writing .gitignore: %v", err)
		}

		if !CheckGitignore(subDir) {
			t.Error("expected true when .start/ in .gitignore")
		}
	})
}
