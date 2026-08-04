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

	"github.com/p3bot/start/internal/cache"
	"github.com/p3bot/start/internal/config"
	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/detection"
	"github.com/p3bot/start/internal/modules"
	"github.com/p3bot/start/internal/registry"
	"github.com/p3bot/start/internal/tui"
)

// AutoSetupResult contains the result of auto-setup.
type AutoSetupResult struct {
	Agent      Agent
	ConfigPath string
}

// AutoSetup performs first-run auto-setup: detect AI CLI tools, prompt if needed, write config.
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
	client, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("creating registry client: %w", err)
	}

	fmt.Fprintln(a.stdout, "Fetching agent index...")
	index, indexVersion, err := client.FetchIndex(ctx, "") // use built-in default; auto-setup runs before user settings exist
	if err != nil {
		return nil, fmt.Errorf("fetching index: %w", err)
	}
	_ = cache.WriteIndex(indexVersion)

	detected := detection.DetectAgents(index)
	if len(detected) == 0 {
		return nil, a.noAgentsError(index)
	}

	selected, err := a.selectAgent(detected)
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(a.stdout, "Fetching configuration...")
	resolvedPath, err := client.ResolveLatestVersion(ctx, selected.Entry.Module)
	if err != nil {
		return nil, fmt.Errorf("resolving agent version: %w", err)
	}

	agentResult, err := client.Fetch(ctx, resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("fetching agent module: %w", err)
	}

	// selected.Key (slash-form, e.g. "claude/interactive") becomes both the agents.cue
	// label and the settings.cue default_agent value, matching 'start install' so the
	// two writers cannot drift.
	agent, err := loadAgentFromModule(agentResult.SourceDir, selected.Key, client.Registry())
	if err != nil {
		return nil, fmt.Errorf("loading agent: %w", err)
	}

	configPath, err := a.writeConfig(agent)
	if err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	fmt.Fprintf(a.stdout, "Configuration saved to %s\n", configPath)

	a.installDefaultModules(ctx, client, index)

	fmt.Fprintln(a.stdout)
	fmt.Fprintln(a.stdout, "Note: The generated configuration uses generic model aliases.")
	fmt.Fprintln(a.stdout, "If using Vertex AI, Bedrock, or other providers, you may need to")
	fmt.Fprintln(a.stdout, "specify explicit model IDs. Edit with: start config edit agent")

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
			fmt.Fprintf(&sb, "  %s - %s\n", ag.bin, ag.desc)
		} else {
			fmt.Fprintf(&sb, "  %s\n", ag.bin)
		}
	}

	sb.WriteString("\nThen run 'start' again.")
	return fmt.Errorf("%s", sb.String())
}

// selectAgent resolves a single detected agent from the full slice returned by
// detection. It handles four cases:
//
//   - one bin, one variant: use it without prompting
//   - one bin, multiple variants: TTY variant prompt; non-TTY heuristic
//   - multiple bins, single variants: TTY tool prompt; non-TTY pick-first bin with feedback
//   - multiple bins, multi-variant somewhere: TTY tool prompt then variant prompt
//     for the chosen bin if needed; non-TTY pick-first bin then heuristic
func (a *AutoSetup) selectAgent(detected []detection.DetectedAgent) (detection.DetectedAgent, error) {
	groups, binNames := groupAgentsByBin(detected)

	if len(binNames) == 1 {
		bin := binNames[0]
		variants := groups[bin]
		if len(variants) == 1 {
			fmt.Fprintf(a.stdout, "Detected: %s\n", variants[0].Key)
			return variants[0], nil
		}
		if a.isTTY {
			return a.promptVariantSelection(bin, variants, bufio.NewReader(a.stdin))
		}
		chosen := pickVariant(variants)
		fmt.Fprintf(a.stdout,
			"Detected %s with multiple variants; using %s. Override with default_agent in config.\n",
			bin, chosen.Key)
		return chosen, nil
	}

	if a.isTTY {
		reps := make([]detection.DetectedAgent, 0, len(binNames))
		for _, bin := range binNames {
			reps = append(reps, pickVariant(groups[bin]))
		}
		// Shared reader: bufio look-ahead would otherwise drop bytes between
		// the tool prompt and the cascading variant prompt.
		reader := bufio.NewReader(a.stdin)
		chosen, err := a.promptSelection(reps, reader)
		if err != nil {
			return detection.DetectedAgent{}, err
		}
		variants := groups[chosen.Entry.Bin]
		if len(variants) == 1 {
			return variants[0], nil
		}
		return a.promptVariantSelection(chosen.Entry.Bin, variants, reader)
	}

	// Non-TTY multi-bin: deterministic pick-first with stdout feedback.
	chosenBin := binNames[0]
	chosen := pickVariant(groups[chosenBin])
	fmt.Fprintf(a.stdout,
		"Detected multiple AI CLI tools (%s); using %s. Override with default_agent in config.\n",
		strings.Join(binNames, ", "), chosen.Key)
	return chosen, nil
}

// groupAgentsByBin groups detected agents by their bin name. The returned
// binNames slice is sorted lexicographically and each group is sorted by Key
// to keep auto-setup output deterministic regardless of map iteration order.
func groupAgentsByBin(detected []detection.DetectedAgent) (map[string][]detection.DetectedAgent, []string) {
	groups := make(map[string][]detection.DetectedAgent)
	for _, d := range detected {
		groups[d.Entry.Bin] = append(groups[d.Entry.Bin], d)
	}
	binNames := make([]string, 0, len(groups))
	for bin := range groups {
		binNames = append(binNames, bin)
	}
	sort.Strings(binNames)
	for _, bin := range binNames {
		variants := groups[bin]
		sort.Slice(variants, func(i, j int) bool {
			return variants[i].Key < variants[j].Key
		})
		groups[bin] = variants
	}
	return groups, binNames
}

// pickVariant chooses a single variant from a group sharing a bin. Priority:
//  1. key ends with "/interactive"
//  2. key has no slash (bare-name entry)
//  3. lex-first key
//
// Callers must pass a non-empty slice. The function sorts a local copy so the
// result is stable regardless of input order. groupAgentsByBin already sorts
// each group, so the in-flow re-sort is a defensive no-op for production
// callers — it exists so unit tests can pass unsorted slices and still get
// deterministic output.
func pickVariant(variants []detection.DetectedAgent) detection.DetectedAgent {
	sorted := make([]detection.DetectedAgent, len(variants))
	copy(sorted, variants)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})
	for _, v := range sorted {
		if strings.HasSuffix(v.Key, "/interactive") {
			return v
		}
	}
	for _, v := range sorted {
		if !strings.Contains(v.Key, "/") {
			return v
		}
	}
	return sorted[0]
}

// promptSelection prompts the user to choose between detected bins. Each
// element of reps is the heuristic representative for one bin; the bin name is
// shown in the menu and the representative's description is reused for context.
// Variant selection (if the chosen bin has multiple variants) is the caller's
// responsibility — promptSelection only resolves the bin.
//
// reader is the shared bufio.Reader created by selectAgent so a follow-up
// variant prompt sees the same buffered stream.
func (a *AutoSetup) promptSelection(reps []detection.DetectedAgent, reader *bufio.Reader) (detection.DetectedAgent, error) {
	tui.ColorHeader.Fprintln(a.stdout, "Multiple AI CLI tools detected:")
	fmt.Fprintln(a.stdout)

	binWidth := 0
	for _, r := range reps {
		if len(r.Entry.Bin) > binWidth {
			binWidth = len(r.Entry.Bin)
		}
	}

	for i, r := range reps {
		fmt.Fprintf(a.stdout, "  %d. ", i+1)
		tui.ColorAgents.Fprintf(a.stdout, "%-*s", binWidth, r.Entry.Bin)
		if r.Entry.Description != "" {
			fmt.Fprint(a.stdout, "  ")
			tui.ColorDim.Fprintln(a.stdout, r.Entry.Description)
		} else {
			fmt.Fprintln(a.stdout)
		}
	}

	fmt.Fprintln(a.stdout)
	fmt.Fprint(a.stdout, "Select agent: ")

	input, err := readSelection(reader)
	if err != nil {
		return detection.DetectedAgent{}, err
	}

	if choice, convErr := strconv.Atoi(input); convErr == nil {
		if choice >= 1 && choice <= len(reps) {
			return reps[choice-1], nil
		}
		return detection.DetectedAgent{}, fmt.Errorf("invalid selection: %s (choose 1-%d)", input, len(reps))
	}

	inputLower := strings.ToLower(input)
	for _, r := range reps {
		if strings.ToLower(r.Entry.Bin) == inputLower || strings.ToLower(r.Key) == inputLower {
			return r, nil
		}
	}

	return detection.DetectedAgent{}, fmt.Errorf("invalid selection: %s", input)
}

// promptVariantSelection prompts the user to pick one variant for a single bin.
// Each row spans two lines: the slash-form key on the first line and an
// indented description (when present) on the second. reader is shared with the
// (optional) preceding tool prompt so bufio doesn't drop bytes between reads.
func (a *AutoSetup) promptVariantSelection(bin string, variants []detection.DetectedAgent, reader *bufio.Reader) (detection.DetectedAgent, error) {
	tui.ColorHeader.Fprintf(a.stdout, "Multiple variants of %s detected:\n", bin)
	fmt.Fprintln(a.stdout)

	for i, v := range variants {
		fmt.Fprintf(a.stdout, "  %d. ", i+1)
		tui.ColorAgents.Fprintln(a.stdout, v.Key)
		if v.Entry.Description != "" {
			tui.ColorDim.Fprintf(a.stdout, "     %s\n", v.Entry.Description)
		}
		fmt.Fprintln(a.stdout)
	}

	fmt.Fprint(a.stdout, "Select agent: ")

	input, err := readSelection(reader)
	if err != nil {
		return detection.DetectedAgent{}, err
	}

	if choice, convErr := strconv.Atoi(input); convErr == nil {
		if choice >= 1 && choice <= len(variants) {
			return variants[choice-1], nil
		}
		return detection.DetectedAgent{}, fmt.Errorf("invalid selection: %s (choose 1-%d)", input, len(variants))
	}

	inputLower := strings.ToLower(input)
	for _, v := range variants {
		if strings.ToLower(v.Key) == inputLower {
			return v, nil
		}
	}

	return detection.DetectedAgent{}, fmt.Errorf("invalid selection: %s", input)
}

// readSelection reads a single trimmed line from the shared bufio.Reader.
func readSelection(reader *bufio.Reader) (string, error) {
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

// loadAgentFromModule loads an agent from a fetched module directory.
func loadAgentFromModule(dir, key string, reg modconfig.Registry) (Agent, error) {
	cctx := cuecontext.New()

	cfg := &load.Config{
		Dir:      dir,
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

	return extractAgentFromValue(v, key)
}

// extractAgentFromValue extracts agent config from a CUE value, trying multiple
// lookup paths to handle both user config and registry module formats.
func extractAgentFromValue(v cue.Value, name string) (Agent, error) {
	agentVal := v.LookupPath(cue.ParsePath(internalcue.KeyAgents)).LookupPath(cue.MakePath(cue.Str(name)))
	if !agentVal.Exists() {
		// Singular "agent" field is the registry module style.
		agentVal = v.LookupPath(cue.ParsePath("agent"))
	}
	if !agentVal.Exists() {
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

	if err := os.MkdirAll(paths.Global, 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	agentContent := generateAgentCUE(agent)
	agentPath := filepath.Join(paths.Global, "agents.cue")
	if err := os.WriteFile(agentPath, []byte(agentContent), 0644); err != nil {
		return "", fmt.Errorf("writing agents file: %w", err)
	}

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
	fmt.Fprintf(&sb, "\t%q: {\n", agent.Name)
	fmt.Fprintf(&sb, "\t\tbin:     %q\n", agent.Bin)
	fmt.Fprintf(&sb, "\t\tcommand: %q\n", agent.Command)

	if agent.DefaultModel != "" {
		fmt.Fprintf(&sb, "\t\tdefault_model: %q\n", agent.DefaultModel)
	}

	if agent.Description != "" {
		fmt.Fprintf(&sb, "\t\tdescription: %q\n", agent.Description)
	}

	if len(agent.Models) > 0 {
		sb.WriteString("\t\tmodels: {\n")

		// Sort for deterministic output.
		var modelNames []string
		for name := range agent.Models {
			modelNames = append(modelNames, name)
		}
		sort.Strings(modelNames)

		for _, name := range modelNames {
			fmt.Fprintf(&sb, "\t\t\t%q: %q\n", name, agent.Models[name])
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
	fmt.Fprintf(&sb, "\tdefault_agent: %q\n", defaultAgent)
	sb.WriteString("}\n")

	return sb.String()
}

// installDefaultModules installs commonly-needed contexts during auto-setup.
// Errors are logged to stderr but don't fail the setup process.
func (a *AutoSetup) installDefaultModules(ctx context.Context, client registry.Client, index *registry.Index) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		fmt.Fprintf(a.stderr, "Warning: Failed to resolve config paths: %v\n", err)
		return
	}
	configDir := paths.Global

	defaultModules := []struct {
		category string
		name     string
	}{
		{category: "contexts", name: "cwd/agents-md"},
	}

	// On error with no CUE files (fresh install), cfg is a zero-value cue.Value;
	// LookupPath then returns non-existent, so ModuleExists correctly returns false.
	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(configDir)
	if err != nil {
		if matches, _ := filepath.Glob(filepath.Join(configDir, "*.cue")); len(matches) > 0 {
			fmt.Fprintf(a.stderr, "Warning: invalid config in %s:\n%s\n",
				configDir, internalcue.IdentifyBrokenFiles(matches))
			return
		}
	}

	for _, mod := range defaultModules {
		if modules.ModuleExists(cfg, mod.category, mod.name) {
			continue
		}

		var entry *registry.IndexEntry
		switch mod.category {
		case "contexts":
			if e, ok := index.Contexts[mod.name]; ok {
				entry = &e
			}
		case "roles":
			if e, ok := index.Roles[mod.name]; ok {
				entry = &e
			}
		case "tasks":
			if e, ok := index.Tasks[mod.name]; ok {
				entry = &e
			}
		}

		if entry == nil {
			fmt.Fprintf(a.stderr, "Warning: Default module %s/%s not found in registry\n", mod.category, mod.name)
			continue
		}

		searchResult := modules.SearchResult{
			Category: mod.category,
			Name:     mod.name,
			Entry:    *entry,
		}

		if _, err := modules.InstallModule(ctx, client, index, searchResult, configDir); err != nil {
			fmt.Fprintf(a.stderr, "Warning: Failed to install %s/%s: %v\n", mod.category, mod.name, err)
		}
	}
}
