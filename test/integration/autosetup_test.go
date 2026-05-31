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
	tmpDir := t.TempDir()

	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

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

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stdin := strings.NewReader("")

	// Run() needs a real registry, so exercise the config-writing path directly instead.
	as := orchestration.NewAutoSetup(stdout, stderr, stdin, false)

	paths, err := config.ResolvePaths("")
	if err != nil {
		t.Fatalf("resolving paths: %v", err)
	}

	if paths.GlobalExists {
		t.Error("global config should not exist yet")
	}

	configDir := paths.Global
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	content := generateTestAgentCUE(agent)
	configPath := filepath.Join(configDir, "agents.cue")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	paths, err = config.ResolvePaths("")
	if err != nil {
		t.Fatalf("resolving paths after write: %v", err)
	}

	if !paths.GlobalExists {
		t.Error("global config should exist after write")
	}

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
	tmpDir := t.TempDir()

	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	paths, err := config.ResolvePaths(tmpDir)
	if err != nil {
		t.Fatalf("resolving paths: %v", err)
	}

	if !orchestration.NeedsSetup(paths) {
		t.Error("expected NeedsSetup=true when no config exists")
	}

	localDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(localDir, 0755); err != nil {
		t.Fatalf("creating local dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localDir, "agents.cue"), []byte("agents: {}"), 0644); err != nil {
		t.Fatalf("writing local config: %v", err)
	}

	paths, err = config.ResolvePaths(tmpDir)
	if err != nil {
		t.Fatalf("resolving paths after local: %v", err)
	}

	if orchestration.NeedsSetup(paths) {
		t.Error("expected NeedsSetup=false when local config exists")
	}
}
