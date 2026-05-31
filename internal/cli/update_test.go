package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestUpdateResultJSON tests that UpdateResult marshals with error as string and omits Error field.
func TestUpdateResultJSON(t *testing.T) {
	t.Parallel()
	results := []UpdateResult{
		{
			Module:     InstalledModule{Category: "agents", Name: "ai/claude", Scope: "global", Origin: "test"},
			OldVersion: "v0.1.0",
			NewVersion: "v0.2.0",
			Updated:    true,
		},
		{
			Module:       InstalledModule{Category: "roles", Name: "golang", Scope: "global", Origin: "test"},
			OldVersion:   "v0.1.0",
			Updated:      false,
			Error:        fmt.Errorf("network timeout"),
			ErrorMessage: "network timeout",
		},
	}

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent failed: %v", err)
	}
	output := string(data)

	if !strings.Contains(output, `"oldVersion": "v0.1.0"`) {
		t.Errorf("output missing oldVersion, got: %s", output)
	}
	if !strings.Contains(output, `"newVersion": "v0.2.0"`) {
		t.Errorf("output missing newVersion, got: %s", output)
	}
	if !strings.Contains(output, `"updated": true`) {
		t.Errorf("output missing updated=true, got: %s", output)
	}

	if !strings.Contains(output, `"error": "network timeout"`) {
		t.Errorf("output missing error string, got: %s", output)
	}

	// The Error interface field must be excluded from JSON via json:"-".
	var decoded []map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	for _, item := range decoded {
		if _, ok := item["Error"]; ok {
			t.Errorf("Error interface field should be excluded via json:\"-\", got: %v", item)
		}
	}
}
