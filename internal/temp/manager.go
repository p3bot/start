// Package temp handles temporary file and directory management.
package temp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// unsafeCharsRe matches characters not safe for filenames.
var unsafeCharsRe = regexp.MustCompile(`[^a-zA-Z0-9-_.]`)

// consecutiveDashesRe matches runs of two or more consecutive dashes.
var consecutiveDashesRe = regexp.MustCompile(`-{2,}`)

// Manager handles temporary file creation and management.
type Manager struct {
	// BaseDir is the base directory for temp files.
	// For dry-run: /tmp
	// For UTD: .start/temp
	BaseDir string
}

// NewDryRunManager creates a manager for dry-run output files.
// Files are written to /tmp/start-YYYYMMDDHHmmss/
func NewDryRunManager() *Manager {
	return &Manager{BaseDir: os.TempDir()}
}

// NewUTDManager creates a manager for UTD temp files.
// Files are written to .start/temp/
func NewUTDManager(workingDir string) *Manager {
	return &Manager{BaseDir: filepath.Join(workingDir, ".start", "temp")}
}

// DryRunDir creates a timestamped directory for dry-run output.
// Returns the directory path.
func (m *Manager) DryRunDir() (string, error) {
	timestamp := time.Now().Format("20060102150405")
	dirName := fmt.Sprintf("start-%s", timestamp)
	dirPath := filepath.Join(m.BaseDir, dirName)

	// Handle collision by appending suffix
	const maxSuffixAttempts = 1000
	originalPath := dirPath
	for suffix := 1; ; suffix++ {
		_, err := os.Stat(dirPath)
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("checking dry-run directory: %w", err)
		}
		if suffix > maxSuffixAttempts {
			return "", fmt.Errorf("could not create unique dry-run directory after %d attempts", maxSuffixAttempts)
		}
		dirPath = fmt.Sprintf("%s-%d", originalPath, suffix)
	}

	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("creating dry-run directory: %w", err)
	}

	return dirPath, nil
}

// WriteDryRunFiles writes the dry-run output files.
func (m *Manager) WriteDryRunFiles(dir string, role, prompt, command string) error {
	files := map[string]string{
		"role.md":     role,
		"prompt.md":   prompt,
		"command.txt": command,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
	}

	return nil
}

// EnsureUTDDir ensures the UTD temp directory exists.
func (m *Manager) EnsureUTDDir() error {
	if err := os.MkdirAll(m.BaseDir, 0755); err != nil {
		return fmt.Errorf("creating UTD temp directory: %w", err)
	}
	return nil
}

// WriteUTDFile writes a temp file with a path-derived name.
// entityType is "role", "context", or "task".
// name is the entity name (e.g., "code-reviewer").
// Returns the path to the written file.
func (m *Manager) WriteUTDFile(entityType, name, content string) (string, error) {
	if err := m.EnsureUTDDir(); err != nil {
		return "", err
	}

	fileName := deriveFileName(entityType, name)
	filePath := filepath.Join(m.BaseDir, fileName)

	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("writing UTD file: %w", err)
	}

	return filePath, nil
}

// deriveFileName creates a filename from entity type and name.
// Examples:
//   - ("role", "code-reviewer") -> "role-code-reviewer.md"
//   - ("context", "project/readme") -> "context-project-readme.md"
func deriveFileName(entityType, name string) string {
	// Replace path separators with dashes
	safeName := strings.ReplaceAll(name, "/", "-")
	safeName = strings.ReplaceAll(safeName, "\\", "-")

	// Remove or replace unsafe characters
	safeName = unsafeCharsRe.ReplaceAllString(safeName, "-")

	// Remove consecutive dashes
	safeName = consecutiveDashesRe.ReplaceAllString(safeName, "-")

	// Trim leading/trailing dashes
	safeName = strings.Trim(safeName, "-")

	return fmt.Sprintf("%s-%s.md", entityType, safeName)
}

// Clean removes all files from the UTD temp directory.
func (m *Manager) Clean() error {
	entries, err := os.ReadDir(m.BaseDir)
	if os.IsNotExist(err) {
		return nil // Directory doesn't exist, nothing to clean
	}
	if err != nil {
		return fmt.Errorf("reading temp directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Don't remove subdirectories
		}
		path := filepath.Join(m.BaseDir, entry.Name())
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// CheckGitignore checks if .start/temp is in .gitignore.
// Returns true if it appears to be ignored.
func CheckGitignore(workingDir string) bool {
	gitignorePath := filepath.Join(workingDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		return false
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == ".start/temp" ||
			line == ".start/temp/" ||
			line == ".start/" ||
			line == ".start" {
			return true
		}
	}

	return false
}
