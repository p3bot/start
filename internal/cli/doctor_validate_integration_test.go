//go:build registry

// Built only under -tags=registry. Exercises `doctor validate --json` against
// the live CUE registry and a real library clone. The per-module walk is too
// expensive to stub, so it is carved out of the offline drift guard and gated
// behind the build tag, keeping the default `go test` run offline.
package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"testing"
)

func TestDoctorValidateJSONIntegration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH; doctor validate requires git")
	}

	cmd := NewRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"doctor", "validate", "--force", "--json"})

	// A non-nil error is expected when the registry reports inconsistencies
	// (validateError is silent); JSON is still written, so decode is the real
	// assertion.
	execErr := cmd.Execute()

	var result ValidateResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("doctor validate --json did not produce a ValidateResult: %v (execute error: %v)\noutput: %s",
			err, execErr, buf.String())
	}

	if result.Index.Checks == nil {
		t.Error("ValidateResult.Index.Checks should be present")
	}
	for _, cat := range result.Categories {
		if cat.Name == "" {
			t.Error("ValidateCategoryResult.Name should be non-empty")
		}
		for _, m := range cat.Modules {
			if m.Name == "" {
				t.Errorf("ValidateModuleResult.Name should be non-empty in category %q", cat.Name)
			}
			if m.Status != "pass" && m.Status != "fail" {
				t.Errorf("ValidateModuleResult.Status = %q, want pass|fail", m.Status)
			}
		}
	}
}
