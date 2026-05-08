package orchestration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/mod/modconfig"

	"github.com/start-cli/start/internal/assets"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/detection"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
)

// AutoSetupResult contains the result of auto-setup.
type AutoSetupResult struct {
	Agent      Agent
	ConfigPath string
}

// AutoSetup performs first-run auto-setup.
// It detects installed AI CLI tools, prompts if needed, and writes config.
type AutoSetup struct {
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
	isTTY  bool
}

// NewAutoSetup creates a new auto-setup handler.
func NewAutoSetup(stdout, stderr io.Writer, stdin io.Reader, isTTY bool) *AutoSetup {
	return &AutoSetup{
		stdout: stdout,
		stderr: stderr,
		stdin:  stdin,
		isTTY:  isTTY,
	}
}

// NeedsSetup checks if auto-setup is required.
func NeedsSetup(paths config.Paths) bool {
	return !paths.AnyExists()
}

// Run executes the auto-setup flow.
func (a *AutoSetup) Run(ctx context.Context) (*AutoSetupResult, error) {
	// Create registry client
	client, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("creating registry client: %w", err)
	}

	// Fetch index
	_, _ = fmt.Fprintln(a.stdout, "Fetching agent index...")
	index, indexVersion, err := client.FetchIndex(ctx, "") // use built-in default; auto-setup runs before user settings exist
	if err != nil {
		return nil, fmt.Errorf("fetching index: %w", err)
	}
	_ = cache.WriteIndex(indexVersion)

	// Detect installed agents
	detected := detection.DetectAgents(index)
	if len(detected) == 0 {
		return nil, a.noAgentsError(index)
	}

	// Sort for consistent ordering
	sort.Slice(detected, func(i, j int) bool {
		return detected[i].Key < detected[j].Key
	})

	// Select agent
	var selected detection.DetectedAgent
	if len(detected) == 1 {
		selected = detected[0]
		_, _ = fmt.Fprintf(a.stdout, "Detected: %s\n", selected.Entry.Bin)
	} else {
		selected, err = a.promptSelection(detected)
		if err != nil {
			return nil, err
		}
	}

	// Resolve to canonical version and fetch agent module
	_, _ = fmt.Fprintln(a.stdout, "Fetching configuration...")
	resolvedPath, err := client.ResolveLatestVersion(ctx, selected.Entry.Module)
	if err != nil {
		return nil, fmt.Errorf("resolving agent version: %w", err)
	}

	agentResult, err := client.Fetch(ctx, resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("fetching agent module: %w", err)
	}

	// Load agent from fetched module
	agent, err := loadAgentFromModule(agentResult.SourceDir, selected.Key, client.Registry())
	if err != nil {
		return nil, fmt.Errorf("loading agent: %w", err)
	}

	// Use the binary name as the agent's user-facing key so the generated
	// config reads naturally (agents: { claude: {...} }, default_agent: "claude").
	// The registry key ("claude/interactive") is an implementation detail.
	// extractAgentFromValue guarantees agent.Bin is non-empty by this point.
	agent.Name = agent.Bin

	// Write config
	configPath, err := a.writeConfig(agent)
	if err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	_, _ = fmt.Fprintf(a.stdout, "Configuration saved to %s\n", configPath)

	// Install default assets (contexts that are commonly needed)
	a.installDefaultAssets(ctx, client, index)

	_, _ = fmt.Fprintln(a.stdout)
	_, _ = fmt.Fprintln(a.stdout, "Note: The generated configuration uses generic model aliases.")
	_, _ = fmt.Fprintln(a.stdout, "If using Vertex AI, Bedrock, or other providers, you may need to")
	_, _ = fmt.Fprintln(a.stdout, "specify explicit model IDs. Edit with: start config edit agent")

	return &AutoSetupResult{
		Agent:      agent,
		ConfigPath: configPath,
	}, nil
}

// noAgentsError returns a helpful error when no agents are detected.
func (a *AutoSetup) noAgentsError(index *registry.Index) error {
	var sb strings.Builder
	sb.WriteString("No AI CLI tools detected in PATH.\n\n")
	sb.WriteString("Install one of:\n")

	// List available agents from index
	var agents []struct {
		bin  string
		desc string
	}
	for _, entry := range index.Agents {
		if entry.Bin != "" {
			agents = append(agents, struct {
				bin  string
				desc string
			}{entry.Bin, entry.Description})
		}
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].bin < agents[j].bin
	})

	for _, ag := range agents {
		if ag.desc != "" {
			sb.WriteString(fmt.Sprintf("  %s - %s\n", ag.bin, ag.desc))
		} else {
			sb.WriteString(fmt.Sprintf("  %s\n", ag.bin))
		}
	}

	sb.WriteString("\nThen run 'start' again.")
	return fmt.Errorf("%s", sb.String())
}

// promptSelection prompts the user to select an agent.
func (a *AutoSetup) promptSelection(detected []detection.DetectedAgent) (detection.DetectedAgent, error) {
	if !a.isTTY {
		var names []string
		for _, d := range detected {
			names = append(names, d.Entry.Bin)
		}
		return detection.DetectedAgent{}, fmt.Errorf(
			"multiple AI CLI tools detected: %s\nRun interactively to select, or set default_agent in config",
			strings.Join(names, ", "),
		)
	}

	_, _ = tui.ColorHeader.Fprintln(a.stdout, "Multiple AI CLI tools detected:")
	_, _ = fmt.Fprintln(a.stdout)

	// Calculate key column width for alignment
	keyWidth := 0
	for _, d := range detected {
		if len(d.Key) > keyWidth {
			keyWidth = len(d.Key)
		}
	}

	for i, d := range detected {
		_, _ = fmt.Fprintf(a.stdout, "  %d. ", i+1)
		_, _ = tui.ColorAgents.Fprintf(a.stdout, "%-*s", keyWidth, d.Key)
		if d.Entry.Description != "" {
			_, _ = fmt.Fprint(a.stdout, "  ")
			_, _ = tui.ColorDim.Fprintln(a.stdout, d.Entry.Description)
		} else {
			_, _ = fmt.Fprintln(a.stdout)
		}
	}

	_, _ = fmt.Fprintln(a.stdout)
	_, _ = fmt.Fprint(a.stdout, "Select agent: ")

	reader := bufio.NewReader(a.stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return detection.DetectedAgent{}, fmt.Errorf("reading input: %w", err)
	}

	input = strings.TrimSpace(input)

	// Try parsing as number first
	if choice, err := strconv.Atoi(input); err == nil {
		if choice >= 1 && choice <= len(detected) {
			return detected[choice-1], nil
		}
		return detection.DetectedAgent{}, fmt.Errorf("invalid selection: %s (choose 1-%d)", input, len(detected))
	}

	// Try matching by name
	inputLower := strings.ToLower(input)
	for _, d := range detected {
		if strings.ToLower(d.Entry.Bin) == inputLower || strings.ToLower(d.Key) == inputLower {
			return d, nil
		}
	}

	return detection.DetectedAgent{}, fmt.Errorf("invalid selection: %s", input)
}

// loadAgentFromModule loads an agent from a fetched module directory.
func loadAgentFromModule(dir, key string, reg modconfig.Registry) (Agent, error) {
	cctx := cuecontext.New()

	// Extract package name from key (e.g., "ai/claude" -> "claude")
	pkgName := key
	if idx := strings.LastIndex(key, "/"); idx != -1 {
		pkgName = key[idx+1:]
	}

	cfg := &load.Config{
		Dir:      dir,
		Package:  pkgName,
		Registry: reg,
	}

	insts := load.Instances([]string{"."}, cfg)
	if len(insts) == 0 {
		return Agent{}, fmt.Errorf("no CUE instances found in %s", dir)
	}

	inst := insts[0]
	if inst.Err != nil {
		return Agent{}, fmt.Errorf("loading module: %w", inst.Err)
	}

	v := cctx.BuildInstance(inst)
	if err := v.Err(); err != nil {
		return Agent{}, fmt.Errorf("building module: %w", err)
	}

	return extractAgentFromValue(v, pkgName)
}

// extractAgentFromValue extracts agent config from a CUE value.
// It tries multiple lookup paths to handle both user config and registry module formats.
func extractAgentFromValue(v cue.Value, name string) (Agent, error) {
	// Try looking up under "agents" map first (user config style)
	agentVal := v.LookupPath(cue.ParsePath(internalcue.KeyAgents)).LookupPath(cue.MakePath(cue.Str(name)))
	if !agentVal.Exists() {
		// Try singular "agent" field (registry module style)
		agentVal = v.LookupPath(cue.ParsePath("agent"))
	}
	if !agentVal.Exists() {
		// Try root level as last resort
		agentVal = v
	}

	agent := extractAgentFields(agentVal, name)

	if agent.Bin == "" {
		return agent, fmt.Errorf("agent %s missing required 'bin' field", name)
	}
	if agent.Command == "" {
		return agent, fmt.Errorf("agent %s missing required 'command' field", name)
	}

	return agent, nil
}

// writeConfig writes the agent configuration to the global config directory.
func (a *AutoSetup) writeConfig(agent Agent) (string, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return "", err
	}

	// Create config directory
	if err := os.MkdirAll(paths.Global, 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	// Write agents.cue
	agentContent := generateAgentCUE(agent)
	agentPath := filepath.Join(paths.Global, "agents.cue")
	if err := os.WriteFile(agentPath, []byte(agentContent), 0644); err != nil {
		return "", fmt.Errorf("writing agents file: %w", err)
	}

	// Write settings.cue with default agent with settings
	configContent := generateSettingsCUE(agent.Name)
	configPath := filepath.Join(paths.Global, "settings.cue")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return configPath, nil
}

// generateAgentCUE generates CUE content for an agent.
func generateAgentCUE(agent Agent) string {
	var sb strings.Builder

	sb.WriteString("// Auto-generated by start auto-setup\n")
	sb.WriteString("// Edit this file to customize your agent configuration\n")
	sb.WriteString("//\n")
	sb.WriteString("// Note: Model values below are generic aliases. If using Vertex AI, Bedrock,\n")
	sb.WriteString("// or other providers, you may need to replace them with explicit model IDs.\n")
	sb.WriteString("// Example for Vertex AI: \"opus\" -> \"claude-opus-4-5@20251101\"\n\n")
	sb.WriteString("agents: {\n")
	sb.WriteString(fmt.Sprintf("\t%q: {\n", agent.Name))
	sb.WriteString(fmt.Sprintf("\t\tbin:     %q\n", agent.Bin))
	sb.WriteString(fmt.Sprintf("\t\tcommand: %q\n", agent.Command))

	if agent.DefaultModel != "" {
		sb.WriteString(fmt.Sprintf("\t\tdefault_model: %q\n", agent.DefaultModel))
	}

	if agent.Description != "" {
		sb.WriteString(fmt.Sprintf("\t\tdescription: %q\n", agent.Description))
	}

	if len(agent.Models) > 0 {
		sb.WriteString("\t\tmodels: {\n")

		// Sort model names for consistent output
		var modelNames []string
		for name := range agent.Models {
			modelNames = append(modelNames, name)
		}
		sort.Strings(modelNames)

		for _, name := range modelNames {
			// Quote model names that contain special characters
			sb.WriteString(fmt.Sprintf("\t\t\t%q: %q\n", name, agent.Models[name]))
		}
		sb.WriteString("\t\t}\n")
	}

	sb.WriteString("\t}\n")
	sb.WriteString("}\n")

	return sb.String()
}

// generateSettingsCUE generates CUE content for settings.
func generateSettingsCUE(defaultAgent string) string {
	var sb strings.Builder

	sb.WriteString("// Auto-generated by start auto-setup\n")
	sb.WriteString("// Edit this file to customize your settings\n\n")
	sb.WriteString("settings: {\n")
	sb.WriteString(fmt.Sprintf("\tdefault_agent: %q\n", defaultAgent))
	sb.WriteString("}\n")

	return sb.String()
}

// installDefaultAssets installs commonly-needed contexts during auto-setup.
// Errors are logged to stderr but don't fail the setup process.
func (a *AutoSetup) installDefaultAssets(ctx context.Context, client *registry.Client, index *registry.Index) {
	// Get global config directory
	paths, err := config.ResolvePaths("")
	if err != nil {
		_, _ = fmt.Fprintf(a.stderr, "Warning: Failed to resolve config paths: %v\n", err)
		return
	}
	configDir := paths.Global

	// List of default assets to install (currently just cwd/agents-md)
	defaultAssets := []struct {
		category string
		name     string
	}{
		{category: "contexts", name: "cwd/agents-md"},
	}

	// Load CUE config once for existence checks.
	// On error with no CUE files (fresh install), cfg is a zero-value cue.Value;
	// LookupPath on it returns non-existent, so AssetExists correctly returns false.
	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(configDir)
	if err != nil {
		if matches, _ := filepath.Glob(filepath.Join(configDir, "*.cue")); len(matches) > 0 {
			_, _ = fmt.Fprintf(a.stderr, "Warning: invalid config in %s:\n%s\n",
				configDir, internalcue.IdentifyBrokenFiles(matches))
			return
		}
	}

	for _, asset := range defaultAssets {
		// Check if already installed (skip silently)
		if assets.AssetExists(cfg, asset.category, asset.name) {
			continue
		}

		// Look up the asset in the index
		var entry *registry.IndexEntry
		switch asset.category {
		case "contexts":
			if e, ok := index.Contexts[asset.name]; ok {
				entry = &e
			}
		case "roles":
			if e, ok := index.Roles[asset.name]; ok {
				entry = &e
			}
		case "tasks":
			if e, ok := index.Tasks[asset.name]; ok {
				entry = &e
			}
		}

		if entry == nil {
			_, _ = fmt.Fprintf(a.stderr, "Warning: Default asset %s/%s not found in registry\n", asset.category, asset.name)
			continue
		}

		// Create SearchResult for installation
		searchResult := assets.SearchResult{
			Category: asset.category,
			Name:     asset.name,
			Entry:    *entry,
		}

		// Install the asset (silent on success, log errors)
		if _, err := assets.InstallAsset(ctx, client, index, searchResult, configDir); err != nil {
			_, _ = fmt.Fprintf(a.stderr, "Warning: Failed to install %s/%s: %v\n", asset.category, asset.name, err)
		}
	}
}
