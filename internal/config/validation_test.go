package config

import (
	"os"
	"path/filepath"
	"testing"

	internalcue "github.com/start-cli/start/internal/cue"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		setupGlobal     func(dir string) // nil means don't create directory
		setupLocal      func(dir string) // nil means don't create directory
		wantGlobalValid bool
		wantLocalValid  bool
		wantGlobalErr   bool
		wantLocalErr    bool
		wantAnyValid    bool
	}{
		{
			name:            "no directories exist",
			setupGlobal:     nil,
			setupLocal:      nil,
			wantGlobalValid: false,
			wantLocalValid:  false,
			wantAnyValid:    false,
		},
		{
			name: "empty global directory",
			setupGlobal: func(dir string) {
				// Just create the directory, no files
			},
			setupLocal:      nil,
			wantGlobalValid: false,
			wantLocalValid:  false,
			wantAnyValid:    false,
			wantGlobalErr:   false, // Empty dir is not an error
		},
		{
			name: "valid global config",
			setupGlobal: func(dir string) {
				content := `agents: { test: { bin: "test", command: "{role}" } }`
				_ = os.WriteFile(filepath.Join(dir, "config.cue"), []byte(content), 0644)
			},
			setupLocal:      nil,
			wantGlobalValid: true,
			wantLocalValid:  false,
			wantAnyValid:    true,
		},
		{
			name: "invalid global config - syntax error",
			setupGlobal: func(dir string) {
				content := `agents: { test: { bin: "test" command: "{role}" } }` // missing comma
				_ = os.WriteFile(filepath.Join(dir, "config.cue"), []byte(content), 0644)
			},
			setupLocal:      nil,
			wantGlobalValid: false,
			wantLocalValid:  false,
			wantGlobalErr:   true,
			wantAnyValid:    false,
		},
		{
			name:        "valid local config",
			setupGlobal: nil,
			setupLocal: func(dir string) {
				content := `roles: { expert: { content: "You are an expert" } }`
				_ = os.WriteFile(filepath.Join(dir, "config.cue"), []byte(content), 0644)
			},
			wantGlobalValid: false,
			wantLocalValid:  true,
			wantAnyValid:    true,
		},
		{
			name: "both valid",
			setupGlobal: func(dir string) {
				content := `agents: { test: { bin: "test", command: "{role}" } }`
				_ = os.WriteFile(filepath.Join(dir, "config.cue"), []byte(content), 0644)
			},
			setupLocal: func(dir string) {
				content := `roles: { expert: { content: "You are an expert" } }`
				_ = os.WriteFile(filepath.Join(dir, "config.cue"), []byte(content), 0644)
			},
			wantGlobalValid: true,
			wantLocalValid:  true,
			wantAnyValid:    true,
		},
		{
			name: "global valid, local invalid",
			setupGlobal: func(dir string) {
				content := `agents: { test: { bin: "test", command: "{role}" } }`
				_ = os.WriteFile(filepath.Join(dir, "config.cue"), []byte(content), 0644)
			},
			setupLocal: func(dir string) {
				content := `roles: { expert: { content: "unclosed string } }`
				_ = os.WriteFile(filepath.Join(dir, "config.cue"), []byte(content), 0644)
			},
			wantGlobalValid: true,
			wantLocalValid:  false,
			wantLocalErr:    true,
			wantAnyValid:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directories
			tempDir := t.TempDir()
			globalDir := filepath.Join(tempDir, "global")
			localDir := filepath.Join(tempDir, "local")

			// Setup directories based on test case
			if tt.setupGlobal != nil {
				_ = os.MkdirAll(globalDir, 0755)
				tt.setupGlobal(globalDir)
			}
			if tt.setupLocal != nil {
				_ = os.MkdirAll(localDir, 0755)
				tt.setupLocal(localDir)
			}

			// Create Paths struct
			paths := Paths{
				Global:       globalDir,
				Local:        localDir,
				GlobalExists: tt.setupGlobal != nil,
				LocalExists:  tt.setupLocal != nil,
			}

			// Run validation
			result := ValidateConfig(paths)

			// Check results
			if result.GlobalValid != tt.wantGlobalValid {
				t.Errorf("GlobalValid = %v, want %v", result.GlobalValid, tt.wantGlobalValid)
			}
			if result.LocalValid != tt.wantLocalValid {
				t.Errorf("LocalValid = %v, want %v", result.LocalValid, tt.wantLocalValid)
			}
			if result.AnyValid() != tt.wantAnyValid {
				t.Errorf("AnyValid() = %v, want %v", result.AnyValid(), tt.wantAnyValid)
			}
			if (result.GlobalError != nil) != tt.wantGlobalErr {
				t.Errorf("GlobalError = %v, wantErr %v", result.GlobalError, tt.wantGlobalErr)
			}
			if (result.LocalError != nil) != tt.wantLocalErr {
				t.Errorf("LocalError = %v, wantErr %v", result.LocalError, tt.wantLocalErr)
			}
		})
	}
}

func TestCUEFilesInDir(t *testing.T) {
	t.Parallel()

	t.Run("returns only cue files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		for _, name := range []string{"a.cue", "b.cue"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x: 1"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# docs"), 0o644); err != nil {
			t.Fatal(err)
		}

		files, err := CUEFilesInDir(dir)
		if err != nil {
			t.Fatalf("CUEFilesInDir() error = %v", err)
		}
		if len(files) != 2 {
			t.Errorf("CUEFilesInDir() len = %d, want 2", len(files))
		}
		for _, f := range files {
			if filepath.Ext(f) != ".cue" {
				t.Errorf("CUEFilesInDir() returned non-.cue file: %q", f)
			}
		}
	})

	t.Run("empty directory returns nil", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		files, err := CUEFilesInDir(dir)
		if err != nil {
			t.Fatalf("CUEFilesInDir() error = %v", err)
		}
		if len(files) != 0 {
			t.Errorf("CUEFilesInDir() = %v, want empty", files)
		}
	})

	t.Run("nonexistent directory returns error", func(t *testing.T) {
		t.Parallel()
		_, err := CUEFilesInDir("/nonexistent/path/xyz")
		if err == nil {
			t.Error("CUEFilesInDir() on nonexistent dir should return error")
		}
	})

	t.Run("subdirectories are skipped", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		subdir := filepath.Join(dir, "subdir.cue") // dir named like a .cue file
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "real.cue"), []byte("x: 1"), 0o644); err != nil {
			t.Fatal(err)
		}

		files, err := CUEFilesInDir(dir)
		if err != nil {
			t.Fatalf("CUEFilesInDir() error = %v", err)
		}
		if len(files) != 1 {
			t.Errorf("CUEFilesInDir() len = %d, want 1 (subdirectory should be skipped)", len(files))
		}
	})
}

func TestValidationResult_HasErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		result     ValidationResult
		wantErrors bool
	}{
		{
			name:       "no errors",
			result:     ValidationResult{},
			wantErrors: false,
		},
		{
			name:       "global error only",
			result:     ValidationResult{GlobalError: &internalcue.ValidationError{Message: "test"}},
			wantErrors: true,
		},
		{
			name:       "local error only",
			result:     ValidationResult{LocalError: &internalcue.ValidationError{Message: "test"}},
			wantErrors: true,
		},
		{
			name: "both errors",
			result: ValidationResult{
				GlobalError: &internalcue.ValidationError{Message: "global"},
				LocalError:  &internalcue.ValidationError{Message: "local"},
			},
			wantErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.HasErrors(); got != tt.wantErrors {
				t.Errorf("HasErrors() = %v, want %v", got, tt.wantErrors)
			}
		})
	}
}
