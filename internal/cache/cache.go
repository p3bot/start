// Package cache manages CLI cache files in the XDG cache directory.
//
// It tracks only the registry index version and fetch timestamp so commands can
// reuse a known-good version without a network call; the index data itself is
// cached by CUE's module cache.
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	cacheDir = "start"

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
// Callers should treat errors as non-fatal (best-effort).
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

func formatCacheFile(version string, updated time.Time) []byte {
	return fmt.Appendf(nil, "index_updated: %q\nindex_version: %q\n",
		updated.Format(time.RFC3339), version)
}

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

// parseSimpleCUE parses top-level `key: "value"` fields, avoiding the full CUE
// library for two fields. Does not handle escapes; values must not contain \ or ".
func parseSimpleCUE(data []byte) (map[string]string, error) {
	fields := make(map[string]string)
	line := 0
	i := 0
	src := string(data)

	for i < len(src) {
		line++

		eol := i
		for eol < len(src) && src[eol] != '\n' {
			eol++
		}
		text := src[i:eol]
		if eol < len(src) {
			eol++ // skip newline
		}
		i = eol

		if len(text) == 0 {
			continue
		}

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

		if len(val) < 2 || val[0] != '"' || val[len(val)-1] != '"' {
			return nil, fmt.Errorf("line %d: value for %q is not a quoted string", line, key)
		}
		fields[key] = val[1 : len(val)-1]
	}

	return fields, nil
}
