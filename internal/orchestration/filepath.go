package orchestration

import (
	"os"
	"path/filepath"
	"strings"
)

// IsFilePath returns true if the string looks like a file path.
// Strings starting with ./, /, or ~ are file paths. Only bare ~ and ~/ are
// recognised as tilde paths; ~user syntax is not supported by ExpandTilde
// and is therefore not treated as a file path.
func IsFilePath(s string) bool {
	if s == "" {
		return false
	}
	return strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "/") ||
		s == "~" ||
		strings.HasPrefix(s, "~/")
}

// ExpandTilde expands a leading ~ or ~/ to the user's home directory.
// Only handles bare ~ and ~/path, not ~user syntax.
// Returns the path unchanged if it does not start with ~ or ~/.
func ExpandTilde(path string) (string, error) {
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

// ExpandFilePath expands tilde and converts to absolute path.
// Returns the expanded path and any error.
func ExpandFilePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	expanded, err := ExpandTilde(path)
	if err != nil {
		return "", err
	}

	// Convert to absolute path
	return filepath.Abs(expanded)
}

// ReadFilePath reads the content of a file path, expanding tilde if present.
// Returns the content and any error.
func ReadFilePath(path string) (string, error) {
	expanded, err := ExpandFilePath(path)
	if err != nil {
		return "", err
	}

	content, err := os.ReadFile(expanded)
	if err != nil {
		return "", err
	}

	return string(content), nil
}
