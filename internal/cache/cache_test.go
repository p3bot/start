package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDir_XDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	want := filepath.Join(tmp, "start")
	if dir != want {
		t.Errorf("Dir() = %q, want %q", dir, want)
	}
}

func TestDir_non_empty(t *testing.T) {
	t.Parallel()
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}
	if dir == "" {
		t.Error("Dir() returned empty string")
	}
}

func TestWriteIndex_and_ReadIndex(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	version := "github.com/grantcarthew/start-assets/index@v0.3.46"

	if err := WriteIndex(version); err != nil {
		t.Fatalf("WriteIndex() error: %v", err)
	}

	// Verify file was created.
	cacheFile := filepath.Join(tmp, "start", "cache.cue")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("Cache file not created: %v", err)
	}

	cached, err := ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex() error: %v", err)
	}

	if cached.Version != version {
		t.Errorf("Version = %q, want %q", cached.Version, version)
	}

	// Updated should be within the last few seconds.
	if time.Since(cached.Updated) > 5*time.Second {
		t.Errorf("Updated = %v, expected within last 5 seconds", cached.Updated)
	}
}

func TestWriteIndex_creates_directory(t *testing.T) {
	tmp := t.TempDir()
	nested := filepath.Join(tmp, "deep", "nested")
	t.Setenv("XDG_CACHE_HOME", nested)

	if err := WriteIndex("test@v1.0.0"); err != nil {
		t.Fatalf("WriteIndex() error: %v", err)
	}

	cacheFile := filepath.Join(nested, "start", "cache.cue")
	if _, err := os.Stat(cacheFile); err != nil {
		t.Fatalf("Cache file not created in nested directory: %v", err)
	}
}

func TestReadIndex_missing_file(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	_, err := ReadIndex()
	if err == nil {
		t.Error("ReadIndex() should error when file is missing")
	}
}

func TestReadIndex_malformed_file(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	cacheDir := filepath.Join(tmp, "start")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		content string
	}{
		{"empty file", ""},
		{"no colon", "just some text"},
		{"missing version", `index_updated: "2026-03-01T10:30:00+10:00"` + "\n"},
		{"missing updated", `index_version: "test@v1.0.0"` + "\n"},
		{"unquoted value", "index_updated: not-quoted\n"},
		{"bad timestamp", `index_updated: "not-a-timestamp"` + "\n" + `index_version: "test@v1.0.0"` + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(cacheDir, "cache.cue"), []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ReadIndex()
			if err == nil {
				t.Errorf("ReadIndex() should error for %s", tt.name)
			}
		})
	}
}

func TestIsFresh(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		age    time.Duration
		maxAge time.Duration
		want   bool
	}{
		{"just created", 0, DefaultMaxAge, true},
		{"1 hour old with 24h max", 1 * time.Hour, DefaultMaxAge, true},
		{"23 hours old with 24h max", 23 * time.Hour, DefaultMaxAge, true},
		{"25 hours old with 24h max", 25 * time.Hour, DefaultMaxAge, false},
		{"48 hours old with 24h max", 48 * time.Hour, DefaultMaxAge, false},
		{"1 minute old with 1 minute max", 30 * time.Second, 1 * time.Minute, true},
		{"2 minutes old with 1 minute max", 2 * time.Minute, 1 * time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := IndexCache{
				Updated: time.Now().Add(-tt.age),
				Version: "test@v1.0.0",
			}
			got := c.IsFresh(tt.maxAge)
			if got != tt.want {
				t.Errorf("IsFresh(%v) = %v, want %v", tt.maxAge, got, tt.want)
			}
		})
	}
}

func TestParseSimpleCUE(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "valid two fields",
			input: `index_updated: "2026-03-01T10:30:00+10:00"` + "\n" + `index_version: "test@v1.0.0"` + "\n",
			want:  map[string]string{"index_updated": "2026-03-01T10:30:00+10:00", "index_version": "test@v1.0.0"},
		},
		{
			name:  "empty lines ignored",
			input: "\n" + `key: "value"` + "\n\n",
			want:  map[string]string{"key": "value"},
		},
		{
			name:  "whitespace around key and value",
			input: `  key  :  "value"  ` + "\n",
			want:  map[string]string{"key": "value"},
		},
		{
			name:    "missing colon",
			input:   "no colon here\n",
			wantErr: true,
		},
		{
			name:    "unquoted value",
			input:   "key: value\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSimpleCUE([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Error("parseSimpleCUE() should have returned an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSimpleCUE() error: %v", err)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseSimpleCUE()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestFormatCacheFile(t *testing.T) {
	t.Parallel()

	version := "test@v1.0.0"
	ts := time.Date(2026, 3, 1, 10, 30, 0, 0, time.FixedZone("AEST", 10*60*60))

	content := formatCacheFile(version, ts)
	got := string(content)

	want := `index_updated: "2026-03-01T10:30:00+10:00"` + "\n" + `index_version: "test@v1.0.0"` + "\n"
	if got != want {
		t.Errorf("formatCacheFile() =\n%s\nwant:\n%s", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	version := "github.com/example/mod@v1.2.3"

	if err := WriteIndex(version); err != nil {
		t.Fatalf("WriteIndex() error: %v", err)
	}

	cached, err := ReadIndex()
	if err != nil {
		t.Fatalf("ReadIndex() error: %v", err)
	}

	if cached.Version != version {
		t.Errorf("Version = %q, want %q", cached.Version, version)
	}
	if !cached.IsFresh(DefaultMaxAge) {
		t.Error("Freshly written cache should be fresh")
	}
}
