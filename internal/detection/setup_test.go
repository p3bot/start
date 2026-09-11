package detection

import (
	"testing"

	"github.com/p3bot/start/internal/registry"
)

func TestDetectSetupAgents_IgnoresFoundWithoutRecipe(t *testing.T) {
	t.Parallel()
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"gemini/interactive": {Bin: "gemini", Description: "Gemini"},
		},
	}
	products := []CatalogProduct{
		{ID: "codex", Bin: "codex", BinaryPath: "/usr/bin/codex"},
	}
	got := DetectSetupAgents(index, products)
	for _, d := range got {
		if d.Key == "codex/interactive" {
			t.Fatalf("Found catalog id without index recipe must be ignored, got %+v", got)
		}
	}
}

func TestDetectSetupAgents_UnjoinedCollapsesToInteractive(t *testing.T) {
	t.Parallel()
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"shell/edit":        {Bin: "bash"},
			"shell/interactive": {Bin: "bash"},
			"shell/aaa":         {Bin: "bash"},
		},
	}
	got := DetectSetupAgents(index, nil)
	if len(got) == 0 {
		t.Skip("bash not on PATH")
	}
	if len(got) != 1 {
		t.Fatalf("got %d agents, want 1 collapsed product: %+v", len(got), got)
	}
	if got[0].Key != "shell/interactive" {
		t.Errorf("Key = %q, want shell/interactive", got[0].Key)
	}
}

func TestDetectSetupAgents_CatalogRecipe(t *testing.T) {
	t.Parallel()
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"claude-code/interactive": {Description: "Claude"},
			"claude-code/edit":        {Description: "Edit", Bin: "claude"},
		},
	}
	products := []CatalogProduct{
		{ID: "claude-code", Bin: "claude", BinaryPath: "/opt/claude"},
	}
	got := DetectSetupAgents(index, products)
	if len(got) != 1 {
		t.Fatalf("got %d agents, want 1: %+v", len(got), got)
	}
	if got[0].Key != "claude-code/interactive" {
		t.Errorf("Key = %q, want claude-code/interactive", got[0].Key)
	}
	if got[0].BinaryPath != "/opt/claude" {
		t.Errorf("BinaryPath = %q, want /opt/claude", got[0].BinaryPath)
	}
	if got[0].Entry.Bin != "claude" {
		t.Errorf("Entry.Bin = %q, want claude (filled from catalog)", got[0].Entry.Bin)
	}
}
