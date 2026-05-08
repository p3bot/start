package detection

import (
	"testing"

	"github.com/start-cli/start/internal/registry"
)

func TestDetectAgents_EmptyIndex(t *testing.T) {
	t.Parallel()
	detected := DetectAgents(nil)
	if len(detected) != 0 {
		t.Errorf("expected empty slice for nil index, got %d agents", len(detected))
	}

	detected = DetectAgents(&registry.Index{})
	if len(detected) != 0 {
		t.Errorf("expected empty slice for empty index, got %d agents", len(detected))
	}
}

func TestDetectAgents_NoBinField(t *testing.T) {
	t.Parallel()
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"ai/test": {
				Module:      "github.com/test/agent@v0",
				Description: "Test agent without bin",
				// No Bin field
			},
		},
	}

	detected := DetectAgents(index)
	if len(detected) != 0 {
		t.Errorf("expected empty slice when agents have no bin field, got %d agents", len(detected))
	}
}

func TestDetectAgents_CommonBinaries(t *testing.T) {
	t.Parallel()
	// Test with binaries that are likely to exist on most systems
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"test/bash": {
				Module:      "github.com/test/bash@v0",
				Description: "Bash shell",
				Bin:         "bash",
			},
			"test/nonexistent": {
				Module:      "github.com/test/nonexistent@v0",
				Description: "Non-existent binary",
				Bin:         "this-binary-definitely-does-not-exist-12345",
			},
		},
	}

	detected := DetectAgents(index)

	// bash should be detected on most Unix systems
	var foundBash bool
	for _, d := range detected {
		if d.Key == "test/bash" {
			foundBash = true
			if d.Entry.Bin != "bash" {
				t.Errorf("expected bin 'bash', got %q", d.Entry.Bin)
			}
			if d.BinaryPath == "" {
				t.Error("expected non-empty binary path for bash")
			}
		}
		if d.Key == "test/nonexistent" {
			t.Error("non-existent binary should not be detected")
		}
	}

	if !foundBash {
		t.Log("bash not found in PATH (may be expected on some systems)")
	}
}

func TestIsBinaryAvailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		bin       string
		wantAvail bool
	}{
		{
			name:      "bash should exist",
			bin:       "bash",
			wantAvail: true,
		},
		{
			name:      "nonexistent binary",
			bin:       "this-binary-definitely-does-not-exist-12345",
			wantAvail: false,
		},
		{
			name:      "empty string",
			bin:       "",
			wantAvail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBinaryAvailable(tt.bin)
			// bash might not exist on all systems, so only check definitively false cases
			if tt.bin == "" || tt.bin == "this-binary-definitely-does-not-exist-12345" {
				if got != tt.wantAvail {
					t.Errorf("IsBinaryAvailable(%q) = %v, want %v", tt.bin, got, tt.wantAvail)
				}
			}
		})
	}
}

func TestDetectAgents_DedupesByBinaryPath(t *testing.T) {
	t.Parallel()
	// Skip if bash isn't available; the test relies on a real binary so the
	// dedup logic exercises a real BinaryPath rather than a synthetic value.
	if !IsBinaryAvailable("bash") {
		t.Skip("bash not available in PATH")
	}

	t.Run("alphabetical fallback when no /interactive variant", func(t *testing.T) {
		t.Parallel()
		index := &registry.Index{
			Agents: map[string]registry.IndexEntry{
				"test/bash/zeta":   {Module: "z@v0", Bin: "bash"},
				"test/bash/alpha":  {Module: "a@v0", Bin: "bash"},
				"test/bash/middle": {Module: "m@v0", Bin: "bash"},
			},
		}

		detected := DetectAgents(index)
		if len(detected) != 1 {
			t.Fatalf("expected 1 deduped result, got %d: %+v", len(detected), detected)
		}
		if detected[0].Key != "test/bash/alpha" {
			t.Errorf("expected alphabetically-first key 'test/bash/alpha', got %q", detected[0].Key)
		}
	})

	t.Run("prefers /interactive variant over alphabetical winner", func(t *testing.T) {
		t.Parallel()
		index := &registry.Index{
			Agents: map[string]registry.IndexEntry{
				"test/bash/aaa-first":   {Module: "a@v0", Bin: "bash"},
				"test/bash/interactive": {Module: "i@v0", Bin: "bash"},
				"test/bash/zzz-last":    {Module: "z@v0", Bin: "bash"},
			},
		}

		detected := DetectAgents(index)
		if len(detected) != 1 {
			t.Fatalf("expected 1 deduped result, got %d: %+v", len(detected), detected)
		}
		if detected[0].Key != "test/bash/interactive" {
			t.Errorf("expected '/interactive' variant to win, got %q", detected[0].Key)
		}
	})
}

func TestDetectAgents_ParallelExecution(t *testing.T) {
	t.Parallel()
	// Test that parallel detection works correctly with multiple agents
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"test/a": {Module: "a@v0", Bin: "nonexistent-a-12345"},
			"test/b": {Module: "b@v0", Bin: "nonexistent-b-12345"},
			"test/c": {Module: "c@v0", Bin: "nonexistent-c-12345"},
			"test/d": {Module: "d@v0", Bin: "nonexistent-d-12345"},
			"test/e": {Module: "e@v0", Bin: "nonexistent-e-12345"},
		},
	}

	// Run multiple times to catch race conditions
	for i := 0; i < 10; i++ {
		detected := DetectAgents(index)
		if len(detected) != 0 {
			t.Errorf("iteration %d: expected 0 detected agents, got %d", i, len(detected))
		}
	}
}
