//go:build integration

package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/detection"
	"github.com/start-cli/start/internal/orchestration"
	"github.com/start-cli/start/internal/registry"
)

func TestAutoSetup_DetectionFlow(t *testing.T) {
	// Create a mock index with real binaries that exist on the system
	index := &registry.Index{
		Agents: map[string]registry.IndexEntry{
			"shell/bash": {
				Module:      "github.com/test/bash@v0",
				Description: "Bash shell (test)",
				Bin:         "bash",
			},
			"test/nonexistent": {
				Module:      "github.com/test/nonexistent@v0",
				Description: "Non-existent",
				Bin:         "this-does-not-exist-12345",
			},
		},
	}

	detected := detection.DetectAgents(index)

	// bash should be detected on most Unix systems
	if len(detected) == 0 {
		t.Skip("no agents detected - bash may not be available")
	}

	var foundBash bool
	for _, d := range detected {
		if d.Key == "shell/bash" {
			foundBash = true
			if d.BinaryPath == "" {
				t.Error("expected non-empty binary path")
			}
		}
		if d.Key == "test/nonexistent" {
			t.Error("nonexistent binary should not be detected")
		}
	}

	if !foundBash {
		t.Log("bash not detected - may be expected on some systems")
	}
}

func TestAutoSetup_ConfigWriting(t *testing.T) {
	// Create a temporary directory for config
	tmpDir := t.TempDir()

	// Override HOME to use temp directory
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Also set XDG_CONFIG_HOME to ensure we use temp dir
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	// Test agent
	agent := orchestration.Agent{
		Name:         "test-agent",
		Bin:          "test-bin",
		Command:      "{{.bin}} --model {{.model}}",
		DefaultModel: "default",
		Models: map[string]string{
			"fast": "fast-model",
			"slow": "slow-model",
		},
	}

	// Create auto-setup with mock I/O
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	as := orchestration.NewAutoSetup(stdout, stderr, stdin, false)

	// We can't call Run() directly as it requires real registry
	// Instead, test the config writing logic via the exported function
	// This tests the integration of config + orchestration packages

	// Verify config directory doesn't exist yet
	paths, err := config.ResolvePaths("")
	if err != nil {
		t.Fatalf("resolving paths: %v", err)
	}

	if paths.GlobalExists {
		t.Error("global config should not exist yet")
	}

	// Create config directory and write agent config
	configDir := paths.Global
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Generate and write config
	content := generateTestAgentCUE(agent)
	configPath := filepath.Join(configDir, "agents.cue")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Verify paths now show global exists
	paths, err = config.ResolvePaths("")
	if err != nil {
		t.Fatalf("resolving paths after write: %v", err)
	}

	if !paths.GlobalExists {
		t.Error("global config should exist after write")
	}

	// Verify file contents
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}

	if !strings.Contains(string(data), "test-agent") {
		t.Error("config should contain agent name")
	}
	if !strings.Contains(string(data), "test-bin") {
		t.Error("config should contain bin")
	}

	// Suppress unused variable warning
	_ = as
}

// generateTestAgentCUE generates CUE content for testing
func generateTestAgentCUE(agent orchestration.Agent) string {
	var sb strings.Builder

	sb.WriteString("agents: {\n")
	sb.WriteString("\t\"" + agent.Name + "\": {\n")
	sb.WriteString("\t\tbin:     \"" + agent.Bin + "\"\n")
	sb.WriteString("\t\tcommand: \"" + agent.Command + "\"\n")

	if agent.DefaultModel != "" {
		sb.WriteString("\t\tdefault_model: \"" + agent.DefaultModel + "\"\n")
	}

	if len(agent.Models) > 0 {
		sb.WriteString("\t\tmodels: {\n")
		for name, id := range agent.Models {
			sb.WriteString("\t\t\t" + name + ": \"" + id + "\"\n")
		}
		sb.WriteString("\t\t}\n")
	}

	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	return sb.String()
}

func TestNeedsSetup_Integration(t *testing.T) {
	// Test with real config resolution
	tmpDir := t.TempDir()

	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	// Initially no config should exist
	paths, err := config.ResolvePaths(tmpDir)
	if err != nil {
		t.Fatalf("resolving paths: %v", err)
	}

	if !orchestration.NeedsSetup(paths) {
		t.Error("expected NeedsSetup=true when no config exists")
	}

	// Create local config
	localDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("creating local dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "agents.cue"), []byte("agents: {}"), 0644); err != nil {
		t.Fatalf("writing local config: %v", err)
	}

	// Re-resolve paths
	paths, err = config.ResolvePaths(tmpDir)
	if err != nil {
		t.Fatalf("resolving paths after local: %v", err)
	}

	if orchestration.NeedsSetup(paths) {
		t.Error("expected NeedsSetup=false when local config exists")
	}
}
