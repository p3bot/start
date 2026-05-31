package orchestration

import (
	"os"
	"path/filepath"
	"strings"
)

// IsFilePath returns true if the string looks like a file path.
// Only bare ~ and ~/ count as tilde paths; ~user is unsupported by ExpandTilde.
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
func ExpandFilePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	expanded, err := ExpandTilde(path)
	if err != nil {
		return "", err
	}

	return filepath.Abs(expanded)
}

// ReadFilePath reads the content of a file path, expanding tilde if present.
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
