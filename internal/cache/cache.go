// Package cache manages CLI cache files in the XDG cache directory.
//
// The cache stores metadata about the registry index (version and fetch timestamp)
// so that commands can reuse a known-good canonical version without a network call.
// The actual index data is cached by CUE's module cache; this package only tracks
// which version was last fetched and when.
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// cacheDir is the subdirectory name under the XDG cache base.
	cacheDir = "start"

	// cacheFile is the filename for the cache.
	cacheFile = "cache.cue"

	// DefaultMaxAge is the default staleness threshold for the index cache.
	DefaultMaxAge = 24 * time.Hour
)

// IndexCache holds the cached registry index metadata.
type IndexCache struct {
	Updated time.Time
	Version string
}

// IsFresh returns true if the cache was updated within maxAge.
func (c IndexCache) IsFresh(maxAge time.Duration) bool {
	return time.Since(c.Updated) < maxAge
}

// Dir returns the cache directory path, respecting XDG_CACHE_HOME.
func Dir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving cache directory: %w", err)
	}
	return filepath.Join(base, cacheDir), nil
}

// ReadIndex reads the index cache from disk.
// Returns an error if the file is missing or malformed.
func ReadIndex() (IndexCache, error) {
	dir, err := Dir()
	if err != nil {
		return IndexCache{}, err
	}

	data, err := os.ReadFile(filepath.Join(dir, cacheFile))
	if err != nil {
		return IndexCache{}, err
	}

	return parseCacheFile(data)
}

// WriteIndex writes the index version and current timestamp to the cache file.
// Creates the cache directory if needed. Errors are returned but callers
// should treat them as non-fatal (best-effort).
func WriteIndex(version string) error {
	dir, err := Dir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	content := formatCacheFile(version, time.Now())
	return os.WriteFile(filepath.Join(dir, cacheFile), content, 0o644)
}

// formatCacheFile produces the CUE content for the cache file.
func formatCacheFile(version string, updated time.Time) []byte {
	return []byte(fmt.Sprintf("index_updated: %q\nindex_version: %q\n",
		updated.Format(time.RFC3339), version))
}

// parseCacheFile extracts IndexCache from raw CUE cache file bytes.
func parseCacheFile(data []byte) (IndexCache, error) {
	fields, err := parseSimpleCUE(data)
	if err != nil {
		return IndexCache{}, fmt.Errorf("parsing cache file: %w", err)
	}

	updatedStr, ok := fields["index_updated"]
	if !ok {
		return IndexCache{}, fmt.Errorf("cache file missing index_updated field")
	}
	versionStr, ok := fields["index_version"]
	if !ok {
		return IndexCache{}, fmt.Errorf("cache file missing index_version field")
	}

	updated, err := time.Parse(time.RFC3339, updatedStr)
	if err != nil {
		return IndexCache{}, fmt.Errorf("parsing index_updated: %w", err)
	}

	return IndexCache{
		Updated: updated,
		Version: versionStr,
	}, nil
}

// parseSimpleCUE parses a minimal CUE file with only top-level string fields.
// Format: key: "value"\n
// This avoids importing the full CUE library for two simple fields.
// Note: does not handle escape sequences; values must not contain \ or ".
func parseSimpleCUE(data []byte) (map[string]string, error) {
	fields := make(map[string]string)
	line := 0
	i := 0
	src := string(data)

	for i < len(src) {
		line++

		// Find end of line.
		eol := i
		for eol < len(src) && src[eol] != '\n' {
			eol++
		}
		text := src[i:eol]
		if eol < len(src) {
			eol++ // skip newline
		}
		i = eol

		// Skip empty lines.
		if len(text) == 0 {
			continue
		}

		// Find colon separator.
		colonIdx := -1
		for j := 0; j < len(text); j++ {
			if text[j] == ':' {
				colonIdx = j
				break
			}
		}
		if colonIdx < 0 {
			return nil, fmt.Errorf("line %d: missing colon", line)
		}

		key := strings.TrimSpace(text[:colonIdx])
		val := strings.TrimSpace(text[colonIdx+1:])

		// Expect quoted string value.
		if len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
			return nil, fmt.Errorf("line %d: value for %q is not a quoted string", line, key)
		}
		fields[key] = val[1 : len(val)-1]
	}

	return fields, nil
}
