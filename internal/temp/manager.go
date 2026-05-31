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

var unsafeCharsRe = regexp.MustCompile(`[^a-zA-Z0-9-_.]`)

var consecutiveDashesRe = regexp.MustCompile(`-{2,}`)

// Manager handles temporary file creation and management.
type Manager struct {
	BaseDir string
}

// NewDryRunManager creates a manager writing dry-run output to /tmp/start-YYYYMMDDHHmmss/.
func NewDryRunManager() *Manager {
	return &Manager{BaseDir: os.TempDir()}
}

// NewUTDManager creates a manager writing UTD temp files to .start/temp/.
func NewUTDManager(workingDir string) *Manager {
	return &Manager{BaseDir: filepath.Join(workingDir, ".start", "temp")}
}

// DryRunDir creates a timestamped directory for dry-run output.
func (m *Manager) DryRunDir() (string, error) {
	timestamp := time.Now().Format("20060102150405")
	dirName := fmt.Sprintf("start-%s", timestamp)
	dirPath := filepath.Join(m.BaseDir, dirName)

	// Append a numeric suffix on collision (same-second invocations).
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

// WriteUTDFile writes a temp file named from entityType ("role"/"context"/"task") and name,
// returning the written path.
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

// deriveFileName sanitises name into a filesystem-safe "entityType-name.md".
func deriveFileName(entityType, name string) string {
	safeName := strings.ReplaceAll(name, "/", "-")
	safeName = strings.ReplaceAll(safeName, "\\", "-")

	safeName = unsafeCharsRe.ReplaceAllString(safeName, "-")

	safeName = consecutiveDashesRe.ReplaceAllString(safeName, "-")

	safeName = strings.Trim(safeName, "-")

	return fmt.Sprintf("%s-%s.md", entityType, safeName)
}

// Clean removes all files from the UTD temp directory.
func (m *Manager) Clean() error {
	entries, err := os.ReadDir(m.BaseDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading temp directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(m.BaseDir, entry.Name())
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// CheckGitignore reports whether .start/temp appears to be ignored by .gitignore.
func CheckGitignore(workingDir string) bool {
	gitignorePath := filepath.Join(workingDir, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		return false
	}

	lines := strings.SplitSeq(string(content), "\n")
	for line := range lines {
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
