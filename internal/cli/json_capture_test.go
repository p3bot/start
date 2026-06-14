package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/registry"
)

// captureJSON runs a --json command through the shared captureStreams scaffold
// and decodes its stdout, returning the decoded JSON and raw bytes. Threading
// stub as a parameter makes a bypass a compile error. A non-nil Execute error
// is tolerated as long as JSON was still produced (doctor returns a silent
// exit-code error alongside its report).
func captureJSON(t *testing.T, stub *registryStub, args ...string) (decoded any, raw []byte) {
	t.Helper()

	stdout, _, execErr := captureStreams(t, stub, args...)
	raw = []byte(stdout)

	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("captureJSON(%v): output did not decode as JSON: %v (execute error: %v)\noutput: %s",
			args, err, execErr, raw)
	}
	return decoded, raw
}

// sentinelAgentName is an index entry the real registry would never carry, so
// its presence in output proves the stub was consulted, not bypassed.
const sentinelAgentName = "stub-sentinel-agent"

// stubLibraryIndex builds an index with representative entries across every
// category, including the sentinel agent.
func stubLibraryIndex() *registry.Index {
	return &registry.Index{
		Agents: map[string]registry.IndexEntry{
			sentinelAgentName: {
				Module:      "github.com/start-cli/library/agents/" + sentinelAgentName + "@v1",
				Description: "Sentinel agent proving the stub was consulted",
				Bin:         "sentinel",
				Version:     stubVersion,
			},
			"claude": {
				Module:      "github.com/start-cli/library/agents/claude@v1",
				Description: "Anthropic Claude",
				Bin:         "claude",
				Version:     stubVersion,
				Tags:        []string{"anthropic", "ai"},
			},
		},
		Roles: map[string]registry.IndexEntry{
			"go-expert": {
				Module:      "github.com/start-cli/library/roles/go-expert@v1",
				Description: "Go expert role",
				Version:     stubVersion,
			},
		},
		Contexts: map[string]registry.IndexEntry{
			"environment": {
				Module:      "github.com/start-cli/library/contexts/environment@v1",
				Description: "Environment context",
				Version:     stubVersion,
			},
		},
		Tasks: map[string]registry.IndexEntry{
			"review/pre-commit": {
				Module:      "github.com/start-cli/library/tasks/review/pre-commit@v1",
				Description: "Pre-commit review",
				Version:     stubVersion,
				Tags:        []string{"review", "git"},
			},
		},
	}
}

// TestCaptureJSON_LibrarySentinel asserts the sentinel entry, present only in
// the injected stub, appears in library --json output, ruling out a silent
// bypass of the provider seam.
func TestCaptureJSON_LibrarySentinel(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())

	decoded, raw := captureJSON(t, stub, "library", "--json")

	if stub.providerCalls != 1 {
		t.Errorf("library should consult the provider once, got %d", stub.providerCalls)
	}
	if !strings.Contains(string(raw), sentinelAgentName) {
		t.Fatalf("library --json output missing sentinel %q; provider stub was bypassed\noutput: %s",
			sentinelAgentName, raw)
	}

	obj, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("library --json should decode to an object, got %T", decoded)
	}
	agents, ok := obj["agents"].(map[string]any)
	if !ok {
		t.Fatalf("library --json missing agents object; got keys %v", mapKeys(obj))
	}
	if _, ok := agents[sentinelAgentName]; !ok {
		t.Errorf("agents object missing sentinel %q; got %v", sentinelAgentName, mapKeys(agents))
	}
}

func TestLibraryJSONOffline(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())

	decoded, _ := captureJSON(t, stub, "library", "--json")

	obj, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("library --json should decode to an object, got %T", decoded)
	}
	for _, key := range []string{"agents", "roles", "contexts", "tasks"} {
		if _, ok := obj[key]; !ok {
			t.Errorf("library --json missing %q key; got %v", key, mapKeys(obj))
		}
	}
}

// TestSearchJSONOffline asserts search --json emits its array shape offline
// with the stub-served registry section populated.
func TestSearchJSONOffline(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())

	decoded, raw := captureJSON(t, stub, "search", "sentinel", "--json")

	if stub.providerCalls != 1 {
		t.Errorf("search should consult the provider once, got %d", stub.providerCalls)
	}
	sections, ok := decoded.([]any)
	if !ok {
		t.Fatalf("search --json should decode to an array, got %T", decoded)
	}
	if len(sections) == 0 {
		t.Fatalf("search --json returned no sections; stub registry results missing\noutput: %s", raw)
	}
	if !strings.Contains(string(raw), sentinelAgentName) {
		t.Errorf("search --json missing sentinel %q; provider stub was bypassed\noutput: %s",
			sentinelAgentName, raw)
	}
}

// TestSearchJSONOffline_RegistryUnavailable asserts search degrades to
// local-only results when the index is unavailable. The provider is still
// consulted (the failure is in FetchIndex), and the registry-only sentinel
// must not appear.
func TestSearchJSONOffline_RegistryUnavailable(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	stub.SetFetchIndexError(fmt.Errorf("registry unavailable"))

	decoded, raw := captureJSON(t, stub, "search", "assistant", "--json")

	if stub.providerCalls != 1 {
		t.Errorf("search should consult the provider once, got %d", stub.providerCalls)
	}
	if _, ok := decoded.([]any); !ok {
		t.Fatalf("search --json should decode to an array, got %T", decoded)
	}
	if strings.Contains(string(raw), sentinelAgentName) {
		t.Errorf("search should have degraded to local-only; registry sentinel leaked\noutput: %s", raw)
	}
	// The local "assistant" role from setupStartTestConfig still matches.
	if !strings.Contains(string(raw), "assistant") {
		t.Errorf("search should still return local matches when the registry is down\noutput: %s", raw)
	}
}

// TestUpdateJSONOffline asserts update --json runs offline through the provider
// and emits its array shape. The fixture installs a registry-origin agent so
// update collects it rather than short-circuiting on an empty install set.
func TestUpdateJSONOffline(t *testing.T) {
	tmpDir, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	writeInstalledRegistryAgent(t, tmpDir)

	decoded, raw := captureJSON(t, stub, "update", "--json")

	if stub.providerCalls != 1 {
		t.Errorf("update should consult the provider once, got %d", stub.providerCalls)
	}
	results, ok := decoded.([]any)
	if !ok {
		t.Fatalf("update --json should decode to an array, got %T", decoded)
	}
	if len(results) == 0 {
		t.Fatalf("update --json returned no results; installed module was not collected\noutput: %s", raw)
	}
	first, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("update result should be an object, got %T", results[0])
	}
	if _, ok := first["updated"]; !ok {
		t.Errorf("update result missing 'updated' key; got %v", mapKeys(first))
	}
	// The matched version comes only from the stub index; the live registry
	// carries no sentinel module, so this pins the result to the stub and would
	// fail if the seam were bypassed.
	if first["newVersion"] != stubVersion {
		t.Errorf("update newVersion = %v, want %q from stub index", first["newVersion"], stubVersion)
	}
}

// TestDoctorJSONOffline asserts doctor --json runs offline and its
// schema-validation section degrades to "Skipped" when the stub errors on the
// schema module.
func TestDoctorJSONOffline(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())

	decoded, raw := captureJSON(t, stub, "doctor", "--json")

	// doctor builds one client to resolve the index version and one to fetch
	// schemas — two provider invocations. A zero here would mean it reached the
	// live registry instead of the stub.
	if stub.providerCalls != 2 {
		t.Errorf("doctor should consult the provider twice, got %d", stub.providerCalls)
	}
	obj, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("doctor --json should decode to an object, got %T", decoded)
	}
	if _, ok := obj["sections"]; !ok {
		t.Fatalf("doctor --json missing 'sections' key; got %v", mapKeys(obj))
	}
	if !strings.Contains(string(raw), "Schema Validation") {
		t.Errorf("doctor --json missing Schema Validation section\noutput: %s", raw)
	}
	if !strings.Contains(string(raw), "Skipped") {
		t.Errorf("doctor --json schema section did not degrade to Skipped offline\noutput: %s", raw)
	}
}

// writeInstalledRegistryAgent appends a registry-origin agent to the local
// config so update collects it. Its origin version matches the stub index, so
// update finds no upgrade and stays offline (no per-module Fetch).
func writeInstalledRegistryAgent(t *testing.T, tmpDir string) {
	t.Helper()
	content := `
agents: {
	"` + sentinelAgentName + `": {
		bin:     "sentinel"
		command: "{{.bin}} run"
		origin:  "github.com/start-cli/library/agents/` + sentinelAgentName + `@` + stubVersion + `"
		version: "` + stubVersion + `"
	}
}
`
	path := filepath.Join(tmpDir, ".start", "installed.cue")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing installed agent fixture: %v", err)
	}
}

// mapKeys returns the sorted keys of a decoded JSON object for error messages.
func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
