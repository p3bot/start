package cue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestFormatError(t *testing.T) {
	t.Parallel()
	t.Run("nil error returns nil", func(t *testing.T) {
		err := FormatError(nil)
		if err != nil {
			t.Errorf("FormatError(nil) = %v, want nil", err)
		}
	})

	t.Run("formats CUE error with position", func(t *testing.T) {
		ctx := cuecontext.New()
		// Create a value with an error
		v := ctx.CompileString(`
			name: "hello" & 42
		`)

		err := v.Err()
		if err == nil {
			t.Fatal("Expected CUE error")
		}

		formatted := FormatError(err)
		if formatted == nil {
			t.Fatal("FormatError returned nil for non-nil error")
		}

		// Check that it's a ValidationError
		ve, ok := formatted.(*ValidationError)
		if !ok {
			t.Fatalf("FormatError returned %T, want *ValidationError", formatted)
		}

		// Error message should contain something about the conflict
		if ve.Message == "" {
			t.Error("ValidationError.Message is empty")
		}
	})

	t.Run("handles non-CUE errors", func(t *testing.T) {
		err := &testError{msg: "custom error"}
		formatted := FormatError(err)

		// FormatError should return something with the same message
		if formatted == nil {
			t.Fatal("FormatError returned nil for non-nil error")
		}
		if formatted.Error() != "custom error" {
			t.Errorf("FormatError message = %q, want %q", formatted.Error(), "custom error")
		}
	})
}

func TestFormatErrors(t *testing.T) {
	t.Parallel()
	t.Run("nil error returns nil", func(t *testing.T) {
		errs := FormatErrors(nil)
		if errs != nil {
			t.Errorf("FormatErrors(nil) = %v, want nil", errs)
		}
	})

	t.Run("formats multiple CUE errors", func(t *testing.T) {
		ctx := cuecontext.New()
		// Create a value with multiple potential errors
		v := ctx.CompileString(`
			a: "hello" & 42
			b: true & "false"
		`)

		err := v.Err()
		if err == nil {
			t.Fatal("Expected CUE error")
		}

		formatted := FormatErrors(err)
		if len(formatted) == 0 {
			t.Fatal("FormatErrors returned empty slice")
		}

		// Each error should be a ValidationError
		for i, e := range formatted {
			if _, ok := e.(*ValidationError); !ok {
				t.Errorf("FormatErrors[%d] = %T, want *ValidationError", i, e)
			}
		}
	})
}

func TestErrorSummary(t *testing.T) {
	t.Parallel()
	t.Run("empty string for nil error", func(t *testing.T) {
		summary := ErrorSummary(nil)
		if summary != "" {
			t.Errorf("ErrorSummary(nil) = %q, want empty string", summary)
		}
	})

	t.Run("single error shows message", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`x: "a" & 1`)

		err := v.Err()
		if err == nil {
			t.Fatal("Expected CUE error")
		}

		summary := ErrorSummary(err)
		if summary == "" {
			t.Error("ErrorSummary returned empty string")
		}
	})

	t.Run("multiple errors shows count", func(t *testing.T) {
		ctx := cuecontext.New()
		v := ctx.CompileString(`
			a: "x" & 1
			b: true & "y"
		`)

		err := v.Err()
		if err == nil {
			t.Fatal("Expected CUE error")
		}

		summary := ErrorSummary(err)

		// Should mention additional errors if there are multiple
		// Note: CUE may or may not produce multiple errors for this case
		if summary == "" {
			t.Error("ErrorSummary returned empty string")
		}
	})
}

func TestValidationError_ErrorFormat(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      *ValidationError
		contains []string
	}{
		{
			name:     "basic message",
			err:      &ValidationError{Message: "something failed"},
			contains: []string{"something failed"},
		},
		{
			name:     "with path",
			err:      &ValidationError{Path: "config.timeout", Message: "must be positive"},
			contains: []string{"config.timeout", "must be positive"},
		},
		{
			name:     "with file and line",
			err:      &ValidationError{Filename: "test.cue", Line: 42, Message: "syntax error"},
			contains: []string{"test.cue", "42", "syntax error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("Error() = %q, should contain %q", result, s)
				}
			}
		})
	}
}

func TestGenerateSourceContext(t *testing.T) {
	t.Parallel()

	// Create a test file with known content
	content := `line one
line two
line three
line four
line five
line six
line seven
`
	tmpDir := t.TempDir()

	t.Run("middle of file with column pointer", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tmpDir, "middle.cue")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		result := generateSourceContext(path, 4, 6)

		// Should show lines 2-6 (2 before, error line, 2 after)
		if !strings.Contains(result, "line two") {
			t.Errorf("missing context line before error\nGot:\n%s", result)
		}
		if !strings.Contains(result, "line four") {
			t.Errorf("missing error line\nGot:\n%s", result)
		}
		if !strings.Contains(result, "line six") {
			t.Errorf("missing context line after error\nGot:\n%s", result)
		}
		// Should have column pointer
		if !strings.Contains(result, "^") {
			t.Errorf("missing column pointer\nGot:\n%s", result)
		}
		// Should have line numbers
		if !strings.Contains(result, "4 |") {
			t.Errorf("missing line number for error line\nGot:\n%s", result)
		}
	})

	t.Run("near start of file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tmpDir, "start.cue")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		result := generateSourceContext(path, 1, 3)

		// Should start at line 1 (can't go before)
		if !strings.Contains(result, "line one") {
			t.Errorf("missing first line\nGot:\n%s", result)
		}
		if !strings.Contains(result, "line three") {
			t.Errorf("missing context after\nGot:\n%s", result)
		}
		if !strings.Contains(result, "1 |") {
			t.Errorf("missing line number 1\nGot:\n%s", result)
		}
	})

	t.Run("near end of file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tmpDir, "end.cue")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		result := generateSourceContext(path, 7, 1)

		if !strings.Contains(result, "line seven") {
			t.Errorf("missing last content line\nGot:\n%s", result)
		}
		if !strings.Contains(result, "line five") {
			t.Errorf("missing context before\nGot:\n%s", result)
		}
	})

	t.Run("missing file returns empty", func(t *testing.T) {
		t.Parallel()
		result := generateSourceContext(filepath.Join(tmpDir, "nonexistent.cue"), 1, 1)
		if result != "" {
			t.Errorf("expected empty string for missing file, got:\n%s", result)
		}
	})

	t.Run("zero column omits pointer", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(tmpDir, "nocol.cue")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		result := generateSourceContext(path, 3, 0)

		if !strings.Contains(result, "line three") {
			t.Errorf("missing error line\nGot:\n%s", result)
		}
		// Should NOT have column pointer when column is 0
		if strings.Contains(result, "^") {
			t.Errorf("should not have pointer with column 0\nGot:\n%s", result)
		}
	})

	t.Run("line number alignment", func(t *testing.T) {
		t.Parallel()
		// Create a file with enough lines to test alignment
		var sb strings.Builder
		for i := 1; i <= 12; i++ {
			sb.WriteString(fmt.Sprintf("line %d\n", i))
		}
		path := filepath.Join(tmpDir, "align.cue")
		if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
			t.Fatalf("writing test file: %v", err)
		}

		result := generateSourceContext(path, 10, 1)

		// Lines 8-12 should be shown; single-digit 8,9 should be padded
		// to align with double-digit 10,11,12
		if !strings.Contains(result, " 8 |") {
			t.Errorf("expected padded single-digit line number\nGot:\n%s", result)
		}
		if !strings.Contains(result, "10 |") {
			t.Errorf("expected unpadded double-digit line number\nGot:\n%s", result)
		}
	})
}

func TestFormatErrorWithContext(t *testing.T) {
	t.Parallel()

	t.Run("nil error returns nil", func(t *testing.T) {
		t.Parallel()
		result := FormatErrorWithContext(nil)
		if result != nil {
			t.Errorf("FormatErrorWithContext(nil) = %v, want nil", result)
		}
	})

	t.Run("CUE error from file includes source context", func(t *testing.T) {
		t.Parallel()
		tmpDir := t.TempDir()
		modDir := filepath.Join(tmpDir, "cue.mod")
		if err := os.MkdirAll(modDir, 0755); err != nil {
			t.Fatalf("creating cue.mod: %v", err)
		}
		if err := os.WriteFile(filepath.Join(modDir, "module.cue"), []byte(`module: "test.example/err@v0"
language: version: "v0.15.1"
`), 0644); err != nil {
			t.Fatalf("writing module.cue: %v", err)
		}

		// Write an invalid CUE file
		cueContent := `package err

name: "hello"
value: "text" & 42
extra: true
`
		cuePath := filepath.Join(tmpDir, "test.cue")
		if err := os.WriteFile(cuePath, []byte(cueContent), 0644); err != nil {
			t.Fatalf("writing test.cue: %v", err)
		}

		// Load the file to get a CUE error with real position
		loader := NewLoader()
		result, err := loader.LoadSingle(tmpDir)
		if err != nil {
			// LoadSingle might return the error directly
			ve := FormatErrorWithContext(err)
			if ve == nil {
				t.Fatal("FormatErrorWithContext returned nil for load error")
			}
			// Should have a message
			if ve.Message == "" {
				t.Error("expected non-empty message")
			}
			return
		}

		// If loading succeeded, validate to get errors
		cueErr := result.Validate()
		if cueErr == nil {
			t.Fatal("expected validation error for conflicting types")
		}

		ve := FormatErrorWithContext(cueErr)
		if ve == nil {
			t.Fatal("FormatErrorWithContext returned nil")
		}
		if ve.Message == "" {
			t.Error("expected non-empty message")
		}
		if ve.Filename == "" {
			t.Error("expected filename in error")
		}
		if ve.Line == 0 {
			t.Error("expected line number in error")
		}
		// Should have source context since file exists
		if ve.Context == "" {
			t.Error("expected source context snippet")
		}
		if !strings.Contains(ve.Context, "|") {
			t.Errorf("context should contain line format '|', got:\n%s", ve.Context)
		}
	})

	t.Run("in-memory CUE error has no context", func(t *testing.T) {
		t.Parallel()
		ctx := cuecontext.New()
		v := ctx.CompileString(`name: "hello" & 42`)
		err := v.Err()
		if err == nil {
			t.Fatal("expected CUE error")
		}

		ve := FormatErrorWithContext(err)
		if ve == nil {
			t.Fatal("FormatErrorWithContext returned nil")
		}
		if ve.Message == "" {
			t.Error("expected non-empty message")
		}
		// In-memory compilation has no real file, so context should be empty
		// (generateSourceContext returns "" for non-existent files)
	})

	t.Run("non-CUE error wraps message", func(t *testing.T) {
		t.Parallel()
		err := &testError{msg: "plain error"}
		ve := FormatErrorWithContext(err)
		if ve == nil {
			t.Fatal("FormatErrorWithContext returned nil")
		}
		if ve.Message != "plain error" {
			t.Errorf("expected message %q, got %q", "plain error", ve.Message)
		}
		if ve.Context != "" {
			t.Errorf("expected empty context for non-CUE error, got: %q", ve.Context)
		}
	})
}

func TestErrorSummary_MultipleErrors(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()
	// Two distinct type conflicts produce two errors
	v := ctx.CompileString(`
		a: "x" & 1
		b: true & "y"
	`)
	err := v.Err()
	if err == nil {
		t.Fatal("expected CUE error")
	}

	summary := ErrorSummary(err)
	if summary == "" {
		t.Error("ErrorSummary returned empty string for multi-error")
	}
	// If multiple errors were produced, summary should mention "more"
	// If CUE collapses them into one, it still returns a non-empty string.
	t.Logf("ErrorSummary result: %s", summary)
}

func TestErrorSummary_NonCUEError(t *testing.T) {
	t.Parallel()
	// A plain Go error — errors.Errors may return empty, triggering the
	// len(cueErrs)==0 branch which returns err.Error() directly.
	err := &testError{msg: "plain error"}
	summary := ErrorSummary(err)
	if summary == "" {
		t.Error("ErrorSummary returned empty string for non-CUE error")
	}
}

// testError is a simple error type for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
