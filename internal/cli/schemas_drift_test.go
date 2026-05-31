package cli

import (
	"strings"
	"testing"
)

// Drift guard for `start help schemas`: runs the --json-capable commands and
// asserts their output matches the documented shape (structural keys and types,
// not data values). The registry-backed four are covered in json_capture_test.go;
// the local four here. doctor validate is covered by a registry integration test.

// requireKeys fails the test if obj is missing any of the named keys.
func requireKeys(t *testing.T, label string, obj map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			t.Errorf("%s: missing key %q; got %v", label, k, mapKeys(obj))
		}
	}
}

// TestListJSONShape asserts list --json emits an array of installed-module
// objects carrying the documented required keys.
func TestListJSONShape(t *testing.T) {
	tmpDir, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
	writeInstalledRegistryAgent(t, tmpDir)

	decoded, _ := captureJSON(t, stub, "list", "--json")
	arr, ok := decoded.([]any)
	if !ok {
		t.Fatalf("list --json should decode to an array, got %T", decoded)
	}
	if len(arr) == 0 {
		t.Fatal("list --json returned no installed modules")
	}
	first, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("list item should be an object, got %T", arr[0])
	}
	requireKeys(t, "list item", first, "category", "name", "scope", "origin", "configFile")
}

// TestConfigListJSONShape asserts config list --json emits an array of
// config-item objects with the documented required keys.
func TestConfigListJSONShape(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())

	decoded, _ := captureJSON(t, stub, "config", "list", "--json")
	arr, ok := decoded.([]any)
	if !ok {
		t.Fatalf("config list --json should decode to an array, got %T", decoded)
	}
	if len(arr) == 0 {
		t.Fatal("config list --json returned no items")
	}
	for i, raw := range arr {
		obj, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("config list item %d should be an object, got %T", i, raw)
		}
		requireKeys(t, "config list item", obj, "category", "name", "source")
	}
}

// TestConfigGetJSONShape asserts config get --json emits an array of config
// items matching a query.
func TestConfigGetJSONShape(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())

	decoded, _ := captureJSON(t, stub, "config", "get", "assistant", "--json")
	arr, ok := decoded.([]any)
	if !ok {
		t.Fatalf("config get --json should decode to an array, got %T", decoded)
	}
	if len(arr) == 0 {
		t.Fatal("config get assistant --json returned no items")
	}
	first := arr[0].(map[string]any)
	requireKeys(t, "config get item", first, "category", "name", "source")
}

// TestConfigSettingsJSONShape asserts both config settings --json forms: the
// keyless object-map shape and the single-entry shape.
func TestConfigSettingsJSONShape(t *testing.T) {
	_, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())

	// Keyless: object mapping each setting name to {value, source}.
	decoded, _ := captureJSON(t, stub, "config", "settings", "--json")
	obj, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("config settings --json should decode to an object, got %T", decoded)
	}
	entry, ok := obj["default_agent"].(map[string]any)
	if !ok {
		t.Fatalf("config settings --json missing default_agent entry; got %v", mapKeys(obj))
	}
	requireKeys(t, "settings entry", entry, "value", "source")

	// Single key: the entry object directly.
	decodedOne, _ := captureJSON(t, stub, "config", "settings", "default_agent", "--json")
	one, ok := decodedOne.(map[string]any)
	if !ok {
		t.Fatalf("config settings <key> --json should decode to an object, got %T", decodedOne)
	}
	requireKeys(t, "single settings entry", one, "value", "source")
}

// TestJSONErrorEmptyStdout asserts the error half of the --json contract: a
// failing --json command writes nothing to stdout and returns a non-nil error.
// doctor is excluded — an issues-found report is a successful (exit 1) report,
// not an error path, and intentionally still writes its JSON to stdout.
func TestJSONErrorEmptyStdout(t *testing.T) {
	cases := []struct {
		name string
		args []string
		// withRegistryError makes FetchIndex fail, for commands whose error
		// path is a registry failure rather than input validation.
		withRegistryError bool
		// installAgent seeds a registry-origin module so update proceeds to the
		// registry rather than short-circuiting on an empty install set.
		installAgent bool
	}{
		{name: "list unknown category", args: []string{"list", "bogus", "--json"}},
		{name: "library unknown category", args: []string{"library", "bogus", "--json"}},
		{name: "config list unknown category", args: []string{"config", "list", "bogus", "--json"}},
		{name: "config settings unknown key", args: []string{"config", "settings", "no-such-key", "--json"}},
		{name: "search query too short", args: []string{"search", "ab", "--json"}},
		{name: "update registry unreachable", args: []string{"update", "--json"}, withRegistryError: true, installAgent: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir, stub := setupStartTestConfigWithRegistry(t, stubLibraryIndex())
			if tc.installAgent {
				writeInstalledRegistryAgent(t, tmpDir)
			}
			if tc.withRegistryError {
				stub.SetFetchIndexError(transientFetchErr())
			}

			out, err := captureText(t, stub, tc.args...)
			if err == nil {
				t.Fatalf("expected an error; got nil (output: %q)", out)
			}
			if strings.TrimSpace(out) != "" {
				t.Errorf("error path under --json wrote to stdout: %q", out)
			}
		})
	}
}
