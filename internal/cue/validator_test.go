package cue

import (
	"strings"
	"testing"

	"cuelang.org/go/cue/cuecontext"
)

func TestNewValidator(t *testing.T) {
	t.Parallel()
	v := NewValidator()
	if v == nil {
		t.Fatal("NewValidator() returned nil")
	}
}

func TestValidator_Validate(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	t.Run("valid concrete value", func(t *testing.T) {
		value := ctx.CompileString(`
			name: "test"
			count: 42
			enabled: true
		`)

		v := NewValidator()
		err := v.Validate(value)
		if err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("valid concrete value with Concrete option", func(t *testing.T) {
		value := ctx.CompileString(`
			name: "test"
			count: 42
		`)

		v := NewValidator()
		err := v.Validate(value, Concrete(true))
		if err != nil {
			t.Errorf("Validate(Concrete(true)) error = %v, want nil", err)
		}
	})

	t.Run("non-concrete value without Concrete option", func(t *testing.T) {
		value := ctx.CompileString(`
			name: string
			count: int
		`)

		v := NewValidator()
		// Without Concrete(true), non-concrete values are allowed
		err := v.Validate(value)
		if err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("non-concrete value with Concrete option", func(t *testing.T) {
		value := ctx.CompileString(`
			name: string
			count: int
		`)

		v := NewValidator()
		err := v.Validate(value, Concrete(true))
		if err == nil {
			t.Error("Validate(Concrete(true)) should return error for non-concrete value")
		}
	})

	t.Run("structural error", func(t *testing.T) {
		value := ctx.CompileString(`
			name: "hello" & 42
		`)

		v := NewValidator()
		err := v.Validate(value)
		if err == nil {
			t.Error("Validate() should return error for conflicting values")
		}
	})
}

func TestValidator_ValidatePath(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	t.Run("existing path", func(t *testing.T) {
		value := ctx.CompileString(`
			config: {
				name: "test"
				count: 42
			}
		`)

		v := NewValidator()
		err := v.ValidatePath(value, "config.name")
		if err != nil {
			t.Errorf("ValidatePath() error = %v, want nil", err)
		}
	})

	t.Run("non-existing path", func(t *testing.T) {
		value := ctx.CompileString(`
			config: {
				name: "test"
			}
		`)

		v := NewValidator()
		err := v.ValidatePath(value, "config.missing")
		if err == nil {
			t.Error("ValidatePath() should return error for non-existing path")
		}

		// Check error message
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("error message should contain 'does not exist', got: %v", err)
		}
	})

	t.Run("path with Concrete option", func(t *testing.T) {
		value := ctx.CompileString(`
			config: {
				name: string
			}
		`)

		v := NewValidator()
		err := v.ValidatePath(value, "config.name", Concrete(true))
		if err == nil {
			t.Error("ValidatePath(Concrete(true)) should return error for non-concrete value")
		}
	})
}

func TestValidator_Exists(t *testing.T) {
	t.Parallel()
	ctx := cuecontext.New()

	value := ctx.CompileString(`
		config: {
			name: "test"
			nested: {
				value: 123
			}
		}
	`)

	v := NewValidator()

	tests := []struct {
		path   string
		expect bool
	}{
		{"config", true},
		{"config.name", true},
		{"config.nested", true},
		{"config.nested.value", true},
		{"config.missing", false},
		{"missing", false},
		{"config.nested.missing", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := v.Exists(value, tt.path)
			if got != tt.expect {
				t.Errorf("Exists(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

func TestValidationError_DetailedError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		err         *ValidationError
		wantContain []string
	}{
		{
			name:        "message only no filename",
			err:         &ValidationError{Message: "something went wrong"},
			wantContain: []string{"Configuration error", "something went wrong"},
		},
		{
			name:        "with filename no line",
			err:         &ValidationError{Filename: "settings.cue", Message: "bad value"},
			wantContain: []string{"Configuration error in settings.cue", "bad value"},
		},
		{
			name:        "with filename and line",
			err:         &ValidationError{Filename: "config.cue", Line: 10, Message: "type mismatch"},
			wantContain: []string{"Configuration error in config.cue", "Line 10", "type mismatch"},
		},
		{
			name:        "with filename line and column",
			err:         &ValidationError{Filename: "config.cue", Line: 5, Column: 3, Message: "syntax error"},
			wantContain: []string{"Line 5", "column 3", "syntax error"},
		},
		{
			name: "with source context",
			err: &ValidationError{
				Message: "bad field",
				Context: "  5 | name: 42\n",
			},
			wantContain: []string{"bad field", "5 | name: 42"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.DetailedError()
			for _, want := range tt.wantContain {
				if !strings.Contains(result, want) {
					t.Errorf("DetailedError() = %q, want containing %q", result, want)
				}
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    *ValidationError
		expect string
	}{
		{
			name:   "message only",
			err:    &ValidationError{Message: "something went wrong"},
			expect: "something went wrong",
		},
		{
			name:   "with path",
			err:    &ValidationError{Path: "config.name", Message: "invalid value"},
			expect: "config.name: invalid value",
		},
		{
			name:   "with filename",
			err:    &ValidationError{Filename: "settings.cue", Message: "syntax error"},
			expect: "settings.cue: syntax error",
		},
		{
			name:   "with filename and line",
			err:    &ValidationError{Filename: "settings.cue", Line: 10, Message: "unexpected token"},
			expect: "settings.cue:10: unexpected token",
		},
		{
			name:   "filename takes precedence over path",
			err:    &ValidationError{Filename: "settings.cue", Path: "x.y", Message: "error"},
			expect: "settings.cue: error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expect {
				t.Errorf("Error() = %q, want %q", got, tt.expect)
			}
		})
	}
}
