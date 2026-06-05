package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/config"
)

func writeStore(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write store: %v", err)
	}
}

func TestCheckAliases_Absent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases", "aliases.cue")
	section := CheckAliases(path, nil)
	if section.Name != "Aliases" {
		t.Errorf("Name = %q, want Aliases", section.Name)
	}
	if section.Results[0].Status != StatusInfo {
		t.Errorf("absent store should be info, got %v", section.Results[0].Status)
	}
	if got := CheckAliasesSection(section); got {
		t.Error("absent store must not be an issue")
	}
}

func TestCheckAliases_Count(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases", "aliases.cue")
	writeStore(t, path, "aliases: {pc: [\"task\", \"x\"], dev: [\"--role\", \"go-expert\"]}\n")
	section := CheckAliases(path, map[string]bool{})
	if section.Summary != "2 valid" {
		t.Errorf("Summary = %q, want %q", section.Summary, "2 valid")
	}
}

func TestCheckAliases_Malformed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases", "aliases.cue")
	writeStore(t, path, "this is not valid cue {{{\n")
	section := CheckAliases(path, nil)
	if !hasFail(section) {
		t.Errorf("malformed store should produce a failure: %+v", section.Results)
	}
}

func TestCheckAliases_TypeError(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases", "aliases.cue")
	writeStore(t, path, "aliases: {pc: \"not a list\"}\n")
	section := CheckAliases(path, map[string]bool{})
	if !hasFail(section) {
		t.Errorf("type error should produce a failure: %+v", section.Results)
	}
}

func TestCheckAliases_NonAliasKeys(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases", "aliases.cue")
	writeStore(t, path, "aliases: {}\nroles: {x: {}}\n")
	section := CheckAliases(path, nil)
	if !hasFail(section) {
		t.Errorf("non-aliases keys should produce a failure: %+v", section.Results)
	}
}

func TestCheckAliases_NonStructAliasesField(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases", "aliases.cue")
	// Parses as valid CUE, but the aliases field is a string, not a map. This
	// must fail loudly rather than report "0 aliases".
	writeStore(t, path, "aliases: \"oops\"\n")
	section := CheckAliases(path, nil)
	if !hasFail(section) {
		t.Errorf("non-struct aliases field should produce a failure: %+v", section.Results)
	}
}

func TestCheckAliases_SubcommandCollisionWarns(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases", "aliases.cue")
	writeStore(t, path, "aliases: {config: [\"task\", \"x\"]}\n")
	section := CheckAliases(path, map[string]bool{"config": true})
	if section.WarnCountForTest() == 0 {
		t.Errorf("collision should warn: %+v", section.Results)
	}
}

// TestCheckConfiguration_OmitsAliasStore confirms the generic Configuration
// section never visits the aliases subdirectory: it lists the top-level sibling
// file but not the (malformed) store, and the merge validation still passes.
func TestCheckConfiguration_OmitsAliasStore(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	globalDir := filepath.Join(tmp, "start")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "roles.cue"), []byte("roles: {r: {prompt: \"x\"}}\n"), 0o644); err != nil {
		t.Fatalf("write roles: %v", err)
	}
	writeStore(t, filepath.Join(globalDir, "aliases", "aliases.cue"), "broken {{{\n")

	paths, err := config.ResolvePaths("")
	if err != nil {
		t.Fatalf("ResolvePaths: %v", err)
	}
	section := CheckConfiguration(paths)
	for _, r := range section.Results {
		if strings.Contains(r.Label, "aliases.cue") || strings.Contains(r.Message, "broken") {
			t.Errorf("Configuration section leaked the alias store: %+v", r)
		}
	}
	if !hasPass(section) {
		t.Errorf("merge validation should pass despite malformed subdir store: %+v", section.Results)
	}
}

func hasFail(s SectionResult) bool {
	for _, r := range s.Results {
		if r.Status == StatusFail {
			return true
		}
	}
	return false
}

func hasPass(s SectionResult) bool {
	for _, r := range s.Results {
		if r.Status == StatusPass {
			return true
		}
	}
	return false
}

// CheckAliasesSection reports whether the section carries an actionable issue.
func CheckAliasesSection(s SectionResult) bool {
	for _, r := range s.Results {
		if r.IsIssue() {
			return true
		}
	}
	return false
}

// WarnCountForTest counts warnings in a single section.
func (s SectionResult) WarnCountForTest() int {
	n := 0
	for _, r := range s.Results {
		if r.Status == StatusWarn {
			n++
		}
	}
	return n
}
