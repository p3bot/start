package cue

import (
	"strconv"

	"cuelang.org/go/cue"
)

// ValidationOption configures validation behaviour.
type ValidationOption func(*validationConfig)

type validationConfig struct {
	concrete bool
}

// Concrete requires all values to be concrete (no definitions or constraints).
func Concrete(v bool) ValidationOption {
	return func(c *validationConfig) {
		c.concrete = v
	}
}

// Validator validates CUE values.
type Validator struct{}

// NewValidator creates a new CUE validator.
func NewValidator() *Validator {
	return &Validator{}
}

// Validate checks a CUE value for errors.
// By default, it only checks for structural errors.
// Use Concrete(true) to require all values to be concrete.
func (v *Validator) Validate(value cue.Value, opts ...ValidationOption) error {
	cfg := &validationConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Check for structural errors first
	if err := value.Err(); err != nil {
		return FormatError(err)
	}

	// If concrete validation requested, validate completeness
	if cfg.concrete {
		if err := value.Validate(cue.Concrete(true)); err != nil {
			return FormatError(err)
		}
	}

	return nil
}

// ValidatePath validates a specific path within a CUE value.
func (v *Validator) ValidatePath(value cue.Value, path string, opts ...ValidationOption) error {
	pathValue := value.LookupPath(cue.ParsePath(path))
	if !pathValue.Exists() {
		return &ValidationError{
			Path:    path,
			Message: "path does not exist",
		}
	}
	return v.Validate(pathValue, opts...)
}

// Exists checks if a path exists in the CUE value.
func (v *Validator) Exists(value cue.Value, path string) bool {
	return value.LookupPath(cue.ParsePath(path)).Exists()
}

// ValidationError represents a validation error with context.
type ValidationError struct {
	Path     string
	Message  string
	Line     int
	Column   int
	Filename string
	Context  string // Source context snippet around the error
}

// Error returns a concise error string with file:line:message format.
func (e *ValidationError) Error() string {
	if e.Filename != "" {
		if e.Line > 0 {
			return e.Filename + ":" + strconv.Itoa(e.Line) + ": " + e.Message
		}
		return e.Filename + ": " + e.Message
	}
	if e.Path != "" {
		return e.Path + ": " + e.Message
	}
	return e.Message
}

// DetailedError returns a formatted error with source context.
// This is intended for display to users when they need to fix config errors.
func (e *ValidationError) DetailedError() string {
	var result string

	// Header with file path
	if e.Filename != "" {
		result = "Configuration error in " + e.Filename + "\n\n"
	} else {
		result = "Configuration error\n\n"
	}

	// Location and message
	if e.Line > 0 {
		result += "  Line " + strconv.Itoa(e.Line)
		if e.Column > 0 {
			result += ", column " + strconv.Itoa(e.Column)
		}
		result += ": " + e.Message + "\n"
	} else {
		result += "  " + e.Message + "\n"
	}

	// Source context if available
	if e.Context != "" {
		result += "\n" + e.Context
	}

	return result
}
