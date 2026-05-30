//go:build registry

// This file is compiled only under the `registry` build tag (go test
// -tags=registry). It exercises `doctor validate --json` against the live CUE
// Central Registry and a real clone of the library repository, asserting the
// ValidateResult / ValidateCategoryResult / ValidateModuleResult shape.
//
// doctor validate's per-module fetch-and-validate walk is the most expensive
// registry interaction to stub, so it is carved out of the offline drift guard
// (which covers library/search/update/doctor via the provider stub) and
// covered here instead. The tag keeps this file out of the default compile and
// the default `go test` discovery, so scripts/invoke-tests — which does not
// pass -tags=registry — never reaches the network.
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
	// --force confirms intent to make network requests; --json selects the shape.
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
