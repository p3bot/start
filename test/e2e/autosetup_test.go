//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// binaryPath returns the path to the start binary.
// The binary must be built before running E2E tests.
func binaryPath(t *testing.T) string {
	t.Helper()

	// Look for binary relative to project root
	// Tests run from test/e2e/, so go up two levels
	paths := []string{
		"../../bin/start",
		"./bin/start",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			abs, err := filepath.Abs(p)
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}
			return abs
		}
	}

	t.Fatal("start binary not found - run 'go build -o bin/start ./cmd/start' first")
	return ""
}

// setupTestEnv creates an isolated test environment with custom HOME and PATH.
// Returns a cleanup function that must be called to remove the temp directory.
// We manage cleanup manually because CUE creates read-only cache files that
// t.TempDir() cannot clean up.
//
// PATH contains exactly the directories in pathDirs — system directories like
// /usr/bin and /bin are deliberately excluded so AI CLI tools installed in
// system locations don't leak into "no agents" or "single agent" tests.
func setupTestEnv(t *testing.T, pathDirs []string) (tmpDir string, env []string, cleanup func()) {
	t.Helper()

	var err error
	tmpDir, err = os.MkdirTemp("", "start-e2e-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	configDir := filepath.Join(tmpDir, ".config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create config dir: %v", err)
	}

	path := strings.Join(pathDirs, ":")

	env = []string{
		"HOME=" + tmpDir,
		"XDG_CONFIG_HOME=" + configDir,
		"PATH=" + path,
	}

	// Cleanup function that handles CUE's read-only cache files
	cleanup = func() {
		// Make all files writable before removal
		filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Ignore errors during walk
			}
			if !info.IsDir() {
				os.Chmod(path, 0644)
			} else {
				os.Chmod(path, 0755)
			}
			return nil
		})
		os.RemoveAll(tmpDir)
	}

	return tmpDir, env, cleanup
}

// sterileBinDir creates a temp directory containing symlinks to the named
// binaries (resolved via PATH) and returns the directory path. Tests that
// need an exact set of executables on PATH should use this instead of the
// binaries' real parent directory, which may contain unrelated tools.
func sterileBinDir(t *testing.T, bins []string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "start-e2e-bin-*")
	if err != nil {
		t.Fatalf("failed to create sterile bin dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	for _, bin := range bins {
		src, err := exec.LookPath(bin)
		if err != nil {
			t.Skipf("%s not installed - skipping test", bin)
		}
		if err := os.Symlink(src, filepath.Join(dir, bin)); err != nil {
			t.Fatalf("failed to symlink %s: %v", bin, err)
		}
	}
	return dir
}

func TestE2E_AutoSetup_SingleAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	binary := binaryPath(t)

	// Sterile PATH: only claude is reachable.
	binDir := sterileBinDir(t, []string{"claude"})

	tmpDir, env, cleanup := setupTestEnv(t, []string{binDir})
	defer cleanup()

	cmd := exec.Command(binary)
	cmd.Env = env
	cmd.Dir = tmpDir

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Auto-setup should succeed (agent launch may fail due to no prompt)
	// Check that auto-setup messages are present
	if !strings.Contains(outputStr, "Fetching agent index") {
		t.Errorf("expected 'Fetching agent index' in output:\n%s", outputStr)
	}

	// Auto-setup picks one variant of the claude bin. The exact key depends on
	// the index, but it must be slash-form (e.g. "claude/interactive") so the
	// label matches what 'start modules add' produces. The detection output may
	// take either of two shapes depending on whether the index ships one or
	// many variants for the bin:
	//   single variant : "Detected: claude/interactive"
	//   many variants  : "Detected claude with multiple variants; using claude/interactive. Override with default_agent in config."
	if !strings.Contains(outputStr, "claude/") {
		t.Errorf("expected slash-form claude variant in detection output:\n%s", outputStr)
	}

	if !strings.Contains(outputStr, "Configuration saved") {
		t.Errorf("expected 'Configuration saved' in output:\n%s", outputStr)
	}

	// Verify config files were created
	agentsFile := filepath.Join(tmpDir, ".config", "start", "agents.cue")
	if _, err := os.Stat(agentsFile); os.IsNotExist(err) {
		t.Error("agents.cue was not created")
	}

	configFile := filepath.Join(tmpDir, ".config", "start", "settings.cue")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("settings.cue was not created")
	}

	// Check agents.cue content: label must be the slash-form registry key.
	agentsContent, err := os.ReadFile(agentsFile)
	if err != nil {
		t.Fatalf("failed to read agents.cue: %v", err)
	}

	if !strings.Contains(string(agentsContent), `"claude/`) {
		t.Errorf("agents.cue should use slash-form registry key as label:\n%s", string(agentsContent))
	}
	if !strings.Contains(string(agentsContent), `bin:`) {
		t.Error("agents.cue should contain bin field")
	}

	// Check settings.cue content: default_agent must be the same slash-form key.
	configContent, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read settings.cue: %v", err)
	}

	if !strings.Contains(string(configContent), `default_agent: "claude/`) {
		t.Errorf("settings.cue should set default_agent to slash-form registry key:\n%s", string(configContent))
	}
}

func TestE2E_AutoSetup_NoAgents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	binary := binaryPath(t)

	// Use PATH with no AI tools
	tmpDir, env, cleanup := setupTestEnv(t, []string{})
	defer cleanup()

	cmd := exec.Command(binary)
	cmd.Env = env
	cmd.Dir = tmpDir

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Should exit with error
	if err == nil {
		t.Error("expected error when no agents detected")
	}

	// Check for helpful error message
	if !strings.Contains(outputStr, "No AI CLI tools detected") {
		t.Errorf("expected 'No AI CLI tools detected' in output:\n%s", outputStr)
	}

	if !strings.Contains(outputStr, "Install one of") {
		t.Errorf("expected 'Install one of' suggestion in output:\n%s", outputStr)
	}

	// Should list available agents
	if !strings.Contains(outputStr, "claude") {
		t.Errorf("expected 'claude' in available agents list:\n%s", outputStr)
	}

	if !strings.Contains(outputStr, "run 'start' again") {
		t.Errorf("expected 'run start again' suggestion:\n%s", outputStr)
	}

	// Config should NOT be created
	agentsFile := filepath.Join(tmpDir, ".config", "start", "agents.cue")
	if _, err := os.Stat(agentsFile); err == nil {
		t.Error("agents.cue should not be created when no agents detected")
	}
}

func TestE2E_AutoSetup_MultipleAgents_NonTTY(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	binary := binaryPath(t)

	// Find which AI tools are installed; sterilise PATH to that exact set so
	// unrelated tools in /usr/bin can't influence the detection result.
	var availableTools []string
	for _, tool := range []string{"claude", "gemini", "aichat"} {
		if _, err := exec.LookPath(tool); err == nil {
			availableTools = append(availableTools, tool)
		}
	}

	if len(availableTools) < 2 {
		t.Skip("need at least 2 AI tools installed for multiple agents test")
	}

	binDir := sterileBinDir(t, availableTools)

	tmpDir, env, cleanup := setupTestEnv(t, []string{binDir})
	defer cleanup()

	cmd := exec.Command(binary)
	cmd.Env = env
	cmd.Dir = tmpDir
	// Ensure non-TTY by not attaching stdin to terminal

	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Non-TTY multi-bin no longer errors: auto-setup picks the lex-first bin
	// and emits a feedback line naming all detected bins, the chosen one, and
	// the override mechanism.
	if !strings.Contains(outputStr, "Detected multiple AI CLI tools") {
		t.Errorf("expected pick-first feedback line in output:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "default_agent") {
		t.Errorf("feedback line should mention default_agent override:\n%s", outputStr)
	}
	for _, tool := range availableTools {
		if !strings.Contains(outputStr, tool) {
			t.Errorf("feedback line should name detected tool %q:\n%s", tool, outputStr)
		}
	}
	if !strings.Contains(outputStr, "Configuration saved") {
		t.Errorf("expected configuration to be saved in non-TTY multi-bin mode:\n%s", outputStr)
	}

	// agents.cue should be created with a slash-form key for the lex-first bin.
	agentsFile := filepath.Join(tmpDir, ".config", "start", "agents.cue")
	if _, err := os.Stat(agentsFile); os.IsNotExist(err) {
		t.Fatal("agents.cue should be created in non-TTY multi-bin mode")
	}
	agentsContent, err := os.ReadFile(agentsFile)
	if err != nil {
		t.Fatalf("failed to read agents.cue: %v", err)
	}
	sortedTools := append([]string(nil), availableTools...)
	sort.Strings(sortedTools)
	chosenBin := sortedTools[0]
	wantPrefix := `"` + chosenBin + `/`
	if !strings.Contains(string(agentsContent), wantPrefix) {
		t.Errorf("agents.cue should contain a %q label (lex-first bin), got:\n%s", wantPrefix, string(agentsContent))
	}
}

func TestE2E_AutoSetup_ExistingConfig_SkipsSetup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	binary := binaryPath(t)

	// Sterile PATH containing exactly one AI tool, whichever is installed.
	var tool string
	for _, candidate := range []string{"claude", "gemini", "aichat"} {
		if _, err := exec.LookPath(candidate); err == nil {
			tool = candidate
			break
		}
	}
	if tool == "" {
		t.Skip("no AI tools installed")
	}
	binDir := sterileBinDir(t, []string{tool})

	tmpDir, env, cleanup := setupTestEnv(t, []string{binDir})
	defer cleanup()

	// Create existing config
	configDir := filepath.Join(tmpDir, ".config", "start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write minimal valid config
	agentsContent := `agents: {
	"test": {
		bin: "echo"
		command: "{{.bin}} test"
	}
}
`
	if err := os.WriteFile(filepath.Join(configDir, "agents.cue"), []byte(agentsContent), 0644); err != nil {
		t.Fatalf("failed to write agents.cue: %v", err)
	}

	settingsContent := `settings: {
	default_agent: "test"
}
`
	if err := os.WriteFile(filepath.Join(configDir, "settings.cue"), []byte(settingsContent), 0644); err != nil {
		t.Fatalf("failed to write settings.cue: %v", err)
	}

	cmd := exec.Command(binary)
	cmd.Env = env
	cmd.Dir = tmpDir

	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Should NOT show auto-setup messages
	if strings.Contains(outputStr, "Fetching agent index") {
		t.Errorf("should skip auto-setup when config exists:\n%s", outputStr)
	}

	// Should try to use the existing config
	if strings.Contains(outputStr, "auto-setup") {
		t.Errorf("should not mention auto-setup when config exists:\n%s", outputStr)
	}
}
