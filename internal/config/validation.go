package config

import (
	"os"
	"path/filepath"
	"strings"

	internalcue "github.com/p3bot/start/internal/cue"
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
// Empty directories (no .cue files) count as "no config", not errors; only
// directories with invalid CUE files produce errors.
func ValidateConfig(paths Paths) ValidationResult {
	var result ValidationResult

	if paths.GlobalExists {
		valid, err := validateDirectory(paths.Global)
		result.GlobalValid = valid
		result.GlobalError = err
	}

	if paths.LocalExists {
		valid, err := validateDirectory(paths.Local)
		result.LocalValid = valid
		result.LocalError = err
	}

	return result
}

// validateDirectory returns (true, nil) for valid CUE files, (false, nil) when
// no CUE files are present, and (false, error) when CUE files are invalid.
func validateDirectory(dir string) (valid bool, err *internalcue.ValidationError) {
	hasCUE, readErr := internalcue.HasCUEFiles(dir)
	if readErr != nil {
		return false, &internalcue.ValidationError{
			Filename: dir,
			Message:  "failed to read directory: " + readErr.Error(),
		}
	}
	if !hasCUE {
		return false, nil
	}

	loader := internalcue.NewLoader()
	_, loadErr := loader.LoadSingle(dir)
	if loadErr != nil {
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
