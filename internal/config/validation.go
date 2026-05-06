package config

import (
	"os"
	"path/filepath"
	"strings"

	internalcue "github.com/start-cli/start/internal/cue"
)

// ValidationResult holds the result of validating configuration directories.
type ValidationResult struct {
	// GlobalValid indicates the global config directory has valid CUE files.
	GlobalValid bool
	// LocalValid indicates the local config directory has valid CUE files.
	LocalValid bool
	// GlobalError contains the validation error for global config, if any.
	GlobalError *internalcue.ValidationError
	// LocalError contains the validation error for local config, if any.
	LocalError *internalcue.ValidationError
}

// AnyValid returns true if at least one config location is valid.
func (r ValidationResult) AnyValid() bool {
	return r.GlobalValid || r.LocalValid
}

// HasErrors returns true if there are any validation errors.
func (r ValidationResult) HasErrors() bool {
	return r.GlobalError != nil || r.LocalError != nil
}

// ValidateConfig validates the CUE configuration in the given paths.
// It checks each directory that exists for valid CUE files.
// A directory is considered valid if it contains at least one .cue file
// that can be successfully parsed and loaded by CUE.
//
// Empty directories (no .cue files) are treated as "no config" rather than errors.
// Only directories with invalid CUE files produce errors.
func ValidateConfig(paths Paths) ValidationResult {
	var result ValidationResult

	// Validate global config if directory exists
	if paths.GlobalExists {
		valid, err := validateDirectory(paths.Global)
		result.GlobalValid = valid
		result.GlobalError = err
	}

	// Validate local config if directory exists
	if paths.LocalExists {
		valid, err := validateDirectory(paths.Local)
		result.LocalValid = valid
		result.LocalError = err
	}

	return result
}

// validateDirectory checks if a directory contains valid CUE configuration.
// Returns:
//   - (true, nil) if directory contains valid CUE files
//   - (false, nil) if directory is empty or has no CUE files (not an error, just no config)
//   - (false, error) if directory has CUE files but they are invalid
func validateDirectory(dir string) (valid bool, err *internalcue.ValidationError) {
	// First check if directory contains any CUE files
	hasCUE, readErr := internalcue.HasCUEFiles(dir)
	if readErr != nil {
		return false, &internalcue.ValidationError{
			Filename: dir,
			Message:  "failed to read directory: " + readErr.Error(),
		}
	}
	if !hasCUE {
		// No CUE files - not an error, just no config present
		return false, nil
	}

	// Try to load the CUE configuration
	loader := internalcue.NewLoader()
	_, loadErr := loader.LoadSingle(dir)
	if loadErr != nil {
		// Convert to ValidationError with context
		ve := internalcue.FormatErrorWithContext(loadErr)
		if ve != nil {
			return false, ve
		}
		return false, &internalcue.ValidationError{
			Filename: dir,
			Message:  loadErr.Error(),
		}
	}

	return true, nil
}

// CUEFilesInDir returns a list of .cue files in the directory.
func CUEFilesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".cue") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}
