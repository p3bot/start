package orchestration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsFilePath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		// File paths (should return true)
		{name: "relative dot slash", input: "./file.md", want: true},
		{name: "relative nested", input: "./path/to/file.md", want: true},
		{name: "absolute path", input: "/usr/local/file.md", want: true},
		{name: "absolute root", input: "/file.md", want: true},
		{name: "tilde home", input: "~/file.md", want: true},
		{name: "tilde nested", input: "~/path/to/file.md", want: true},
		{name: "just dot slash", input: "./", want: true},
		{name: "just slash", input: "/", want: true},
		{name: "just tilde", input: "~", want: true},
		{name: "tilde user syntax", input: "~user/bin", want: false},
		{name: "tilde without slash", input: "~foo", want: false},

		// Config names (should return false)
		{name: "simple name", input: "go-expert", want: false},
		{name: "namespaced", input: "golang/code-review", want: false},
		{name: "with hyphen", input: "pre-commit-review", want: false},
		{name: "with underscore", input: "my_role", want: false},
		{name: "relative without dot", input: "path/to/file.md", want: false},
		{name: "empty string", input: "", want: false},
		{name: "single dot", input: ".", want: false},
		{name: "double dot", input: "..", want: false},
		{name: "hidden file style", input: ".hidden", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsFilePath(tt.input)
			if got != tt.want {
				t.Errorf("IsFilePath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestExpandFilePath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home dir: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "absolute path unchanged",
			input: "/usr/local/file.md",
			want:  "/usr/local/file.md",
		},
		{
			name:  "tilde expands to home",
			input: "~/file.md",
			want:  filepath.Join(homeDir, "file.md"),
		},
		{
			name:  "tilde nested path",
			input: "~/path/to/file.md",
			want:  filepath.Join(homeDir, "path/to/file.md"),
		},
		{
			name:  "relative path becomes absolute",
			input: "./file.md",
			want:  filepath.Join(cwd, "file.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandFilePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandFilePath(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandFilePath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestReadFilePath(t *testing.T) {
	// Create temp directory with test files
	tmpDir := t.TempDir()

	// Create a test file
	testContent := "This is test content.\nLine 2."
	testFile := filepath.Join(tmpDir, "test.md")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "read existing file",
			path: testFile,
			want: testContent,
		},
		{
			name:    "read non-existent file",
			path:    filepath.Join(tmpDir, "missing.md"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadFilePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ReadFilePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
