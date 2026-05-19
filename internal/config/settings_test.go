package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/shell"
)

func TestSettingDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		want    string
		nonzero bool // true = just check it's non-empty (for env-dependent values)
	}{
		{"library_index", "library_index", registry.IndexModulePath, false},
		{"shell", "shell", "", true},
		{"timeout", "timeout", strconv.Itoa(shell.DefaultTimeout), false},
		{"default_agent has no default", "default_agent", "", false},
		{"unknown key", "nonexistent", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SettingDefault(tt.key)
			if tt.nonzero {
				if got == "" {
					t.Errorf("SettingDefault(%q) = empty, want non-empty", tt.key)
				}
			} else if got != tt.want {
				t.Errorf("SettingDefault(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestValidSettingsKeysString(t *testing.T) {
	t.Parallel()

	result := ValidSettingsKeysString()

	for key := range SettingsRegistry {
		if !strings.Contains(result, key) {
			t.Errorf("ValidSettingsKeysString() missing key %q, got: %s", key, result)
		}
	}

	// Verify sorted
	keys := strings.Split(result, ", ")
	if len(keys) != len(SettingsRegistry) {
		t.Errorf("ValidSettingsKeysString() returned %d keys, want %d", len(keys), len(SettingsRegistry))
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] > keys[i] {
			t.Errorf("ValidSettingsKeysString() not sorted: %q before %q", keys[i-1], keys[i])
		}
	}
}

func TestResolveAllSettings_DefaultsOnly(t *testing.T) {
	t.Parallel()

	paths := Paths{
		Global:       filepath.Join(t.TempDir(), "global"),
		Local:        filepath.Join(t.TempDir(), "local"),
		GlobalExists: false,
		LocalExists:  false,
	}

	entries, err := ResolveAllSettings(paths, ScopeMerged)
	if err != nil {
		t.Fatalf("ResolveAllSettings() error = %v", err)
	}

	if len(entries) != len(SettingsRegistry) {
		t.Errorf("got %d entries, want %d", len(entries), len(SettingsRegistry))
	}

	// library_index should have a default
	if e := entries["library_index"]; e.Source != "default" {
		t.Errorf("library_index source = %q, want %q", e.Source, "default")
	}

	// default_agent should be not set
	if e := entries["default_agent"]; e.Source != "not set" {
		t.Errorf("default_agent source = %q, want %q", e.Source, "not set")
	}

	// timeout should have a default
	if e := entries["timeout"]; e.Source != "default" {
		t.Errorf("timeout source = %q, want %q", e.Source, "default")
	}
}

func TestResolveAllSettings_GlobalOverride(t *testing.T) {
	t.Parallel()

	globalDir := filepath.Join(t.TempDir(), "global")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"),
		[]byte(`settings: { default_agent: "claude" }`), 0644); err != nil {
		t.Fatal(err)
	}

	paths := Paths{
		Global:       globalDir,
		Local:        filepath.Join(t.TempDir(), "local"),
		GlobalExists: true,
		LocalExists:  false,
	}

	entries, err := ResolveAllSettings(paths, ScopeMerged)
	if err != nil {
		t.Fatalf("ResolveAllSettings() error = %v", err)
	}

	e := entries["default_agent"]
	if e.Value != "claude" {
		t.Errorf("default_agent value = %q, want %q", e.Value, "claude")
	}
	if e.Source != "global" {
		t.Errorf("default_agent source = %q, want %q", e.Source, "global")
	}
}

func TestResolveAllSettings_LocalOverridesGlobal(t *testing.T) {
	t.Parallel()

	globalDir := filepath.Join(t.TempDir(), "global")
	localDir := filepath.Join(t.TempDir(), "local")
	for _, dir := range []string{globalDir, localDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"),
		[]byte(`settings: { default_agent: "claude" }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "settings.cue"),
		[]byte(`settings: { default_agent: "gemini" }`), 0644); err != nil {
		t.Fatal(err)
	}

	paths := Paths{
		Global:       globalDir,
		Local:        localDir,
		GlobalExists: true,
		LocalExists:  true,
	}

	entries, err := ResolveAllSettings(paths, ScopeMerged)
	if err != nil {
		t.Fatalf("ResolveAllSettings() error = %v", err)
	}

	e := entries["default_agent"]
	if e.Value != "gemini" {
		t.Errorf("default_agent value = %q, want %q", e.Value, "gemini")
	}
	if e.Source != "local" {
		t.Errorf("default_agent source = %q, want %q", e.Source, "local")
	}
}

func TestResolveAllSettings_LocalOnly(t *testing.T) {
	t.Parallel()

	globalDir := filepath.Join(t.TempDir(), "global")
	localDir := filepath.Join(t.TempDir(), "local")
	for _, dir := range []string{globalDir, localDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"),
		[]byte(`settings: { default_agent: "claude" }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "settings.cue"),
		[]byte(`settings: { timeout: 120 }`), 0644); err != nil {
		t.Fatal(err)
	}

	paths := Paths{
		Global:       globalDir,
		Local:        localDir,
		GlobalExists: true,
		LocalExists:  true,
	}

	entries, err := ResolveAllSettings(paths, ScopeLocal)
	if err != nil {
		t.Fatalf("ResolveAllSettings() error = %v", err)
	}

	// Global default_agent should NOT be picked up
	if e := entries["default_agent"]; e.Source != "not set" {
		t.Errorf("default_agent source = %q, want %q (global should be ignored)", e.Source, "not set")
	}

	// Local timeout should be picked up
	e := entries["timeout"]
	if e.Value != "120" {
		t.Errorf("timeout value = %q, want %q", e.Value, "120")
	}
	if e.Source != "local" {
		t.Errorf("timeout source = %q, want %q", e.Source, "local")
	}
}

func TestResolveAllSettings_GlobalOnly(t *testing.T) {
	t.Parallel()

	globalDir := filepath.Join(t.TempDir(), "global")
	localDir := filepath.Join(t.TempDir(), "local")
	for _, dir := range []string{globalDir, localDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(globalDir, "settings.cue"),
		[]byte(`settings: { default_agent: "claude" }`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "settings.cue"),
		[]byte(`settings: { default_agent: "gemini", timeout: 120 }`), 0644); err != nil {
		t.Fatal(err)
	}

	paths := Paths{
		Global:       globalDir,
		Local:        localDir,
		GlobalExists: true,
		LocalExists:  true,
	}

	entries, err := ResolveAllSettings(paths, ScopeGlobal)
	if err != nil {
		t.Fatalf("ResolveAllSettings() error = %v", err)
	}

	// Global default_agent should be picked up — local must NOT override
	e := entries["default_agent"]
	if e.Value != "claude" {
		t.Errorf("default_agent value = %q, want %q (local should be ignored)", e.Value, "claude")
	}
	if e.Source != "global" {
		t.Errorf("default_agent source = %q, want %q", e.Source, "global")
	}

	// Local timeout should NOT leak through — defaults remain
	if e := entries["timeout"]; e.Source != "default" {
		t.Errorf("timeout source = %q, want %q (local should be ignored)", e.Source, "default")
	}
}

func TestLoadSettingsFromDir_Empty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	settings, err := LoadSettingsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadSettingsFromDir() error = %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("got %d settings, want 0", len(settings))
	}
}

func TestLoadSettingsFromDir_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.cue"),
		[]byte(`settings: { default_agent: "claude", timeout: 60 }`), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadSettingsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadSettingsFromDir() error = %v", err)
	}

	if settings["default_agent"] != "claude" {
		t.Errorf("default_agent = %q, want %q", settings["default_agent"], "claude")
	}
	if settings["timeout"] != "60" {
		t.Errorf("timeout = %q, want %q", settings["timeout"], "60")
	}
}

func TestLoadSettingsFromDir_NoSettingsBlock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agents.cue"),
		[]byte(`agents: { test: { bin: "echo", command: "{role}" } }`), 0644); err != nil {
		t.Fatal(err)
	}

	settings, err := LoadSettingsFromDir(dir)
	if err != nil {
		t.Fatalf("LoadSettingsFromDir() error = %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("got %d settings, want 0", len(settings))
	}
}
