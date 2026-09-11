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

	"github.com/p3bot/agentdex"
	"github.com/p3bot/start/internal/cache"
	"github.com/p3bot/start/internal/config"
	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/detection"
	"github.com/p3bot/start/internal/modules"
	"github.com/p3bot/start/internal/registry"
	"github.com/p3bot/start/internal/skills"
	"github.com/p3bot/start/internal/tui"
)

// AutoSetupResult contains the result of auto-setup.
type AutoSetupResult struct {
	Agent      Agent
	ConfigPath string
}

// AutoSetup performs first-run auto-setup: detect AI CLI tools, prompt if needed, write config.
type AutoSetup struct {
	stdout      io.Writer
	stderr      io.Writer
	stdin       io.Reader
	isTTY       bool
	catalogOpts []agentdex.Option
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

// SetCatalogOpts injects agentdex options so tests stay offline.
func (a *AutoSetup) SetCatalogOpts(opts []agentdex.Option) {
	a.catalogOpts = opts
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

	catalog, catalogErr := a.catalogAgents(ctx)
	detected, err := firstRunAgents(index, catalog, catalogErr)
	if err != nil {
		return nil, err
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

	// selected.Key (slash-form, e.g. "claude-code/interactive") becomes both the
	// agents.cue label and the settings.cue default_agent value, matching
	// 'start install' so the two writers cannot drift.
	agent, agentVal, err := loadAgentFromModule(agentResult.SourceDir, selected.Key, client.Registry())
	if err != nil {
		return nil, fmt.Errorf("loading agent: %w", err)
	}

	configPath, err := a.writeConfig(agent, agentVal, resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	fmt.Fprintf(a.stdout, "Configuration saved to %s\n", configPath)

	a.installDefaultModules(ctx, client, index)

	if len(agent.Models) > 0 {
		fmt.Fprintln(a.stdout)
		fmt.Fprintln(a.stdout, "Note: The generated configuration uses generic model aliases.")
		fmt.Fprintln(a.stdout, "If using Vertex AI, Bedrock, or other providers, you may need to")
		fmt.Fprintln(a.stdout, "specify explicit model IDs. Edit with: start config edit agent")
	}

	return &AutoSetupResult{
		Agent:      agent,
		ConfigPath: configPath,
	}, nil
}

// noAgentsError returns a helpful error when no agents are detected.
// catalog nil lists unique bins from the index only. A non-nil catalog
// (even empty) lists catalog products that have an <id>/interactive recipe,
// then unique bins from unjoined index entries.
func noAgentsError(index *registry.Index, catalog []agentdex.Agent) error {
	var sb strings.Builder
	sb.WriteString("No AI CLI tools detected in PATH.\n\n")
	sb.WriteString("Install one of:\n")

	for _, ag := range setupInstallHints(index, catalog) {
		if ag.desc != "" {
			fmt.Fprintf(&sb, "  %s - %s\n", ag.bin, ag.desc)
		} else {
			fmt.Fprintf(&sb, "  %s\n", ag.bin)
		}
	}

	sb.WriteString("\nThen run 'start' again.")
	return fmt.Errorf("%s", sb.String())
}

type installHint struct {
	bin  string
	desc string
}

func setupInstallHints(index *registry.Index, catalog []agentdex.Agent) []installHint {
	if catalog == nil {
		return indexBinHints(index)
	}

	known := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		if item.ID != "" {
			known[item.ID] = true
		}
	}

	seen := map[string]bool{}
	var out []installHint
	add := func(bin, desc string) {
		if bin == "" || seen[bin] {
			return
		}
		seen[bin] = true
		out = append(out, installHint{bin: bin, desc: desc})
	}

	for _, item := range catalog {
		if index == nil {
			continue
		}
		if _, ok := index.Agents[item.ID+detection.DefaultSetupVariant]; !ok {
			continue
		}
		bin := item.Bin
		if bin == "" {
			bin = item.ID
		}
		desc := item.Description
		if desc == "" {
			desc = item.Name
		}
		add(bin, desc)
	}
	if index != nil {
		for key, entry := range index.Agents {
			if entry.Bin == "" {
				continue
			}
			tool, _, _ := strings.Cut(key, "/")
			if known[tool] {
				continue
			}
			add(entry.Bin, entry.Description)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].bin < out[j].bin
	})
	return out
}

func indexBinHints(index *registry.Index) []installHint {
	if index == nil {
		return nil
	}
	var out []installHint
	seen := map[string]bool{}
	for _, entry := range index.Agents {
		if entry.Bin == "" || seen[entry.Bin] {
			continue
		}
		seen[entry.Bin] = true
		out = append(out, installHint{bin: entry.Bin, desc: entry.Description})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].bin < out[j].bin
	})
	return out
}

// firstRunAgents is the first-run candidate set. A catalog error aborts
// before LookPath: joined recipes have no index bin, so a degraded scan
// would miss catalogued tools and could write the wrong default. Unjoined
// index bins are considered only after a successful catalog list.
func firstRunAgents(index *registry.Index, catalog []agentdex.Agent, catalogErr error) ([]detection.DetectedAgent, error) {
	if catalogErr != nil {
		return nil, catalogErr
	}
	detected := detection.DetectSetupAgents(index, foundCatalogProducts(catalog))
	if len(detected) == 0 {
		return nil, noAgentsError(index, catalog)
	}
	return detected, nil
}

func (a *AutoSetup) catalogAgents(ctx context.Context) ([]agentdex.Agent, error) {
	// First-run may wait on the registry: Latest so setup sees current recipes
	// and warms the cache later Cached joins read without a network round-trip.
	opts := make([]agentdex.Option, 0, 1+len(a.catalogOpts))
	opts = append(opts, agentdex.WithCatalogFetch(agentdex.FetchLatest))
	opts = append(opts, a.catalogOpts...)
	idx, err := skills.OpenIndex("", opts...)
	if err != nil {
		return nil, mapSetupCatalogErr(err)
	}
	res, err := idx.Agents.List(ctx, agentdex.AgentQuery{Enrich: agentdex.EnrichNone})
	if err != nil {
		return nil, mapSetupCatalogErr(err)
	}
	return res.Items, nil
}

func foundCatalogProducts(items []agentdex.Agent) []detection.CatalogProduct {
	if len(items) == 0 {
		return nil
	}
	var out []detection.CatalogProduct
	for _, item := range items {
		if !item.Detection.Found {
			continue
		}
		out = append(out, detection.CatalogProduct{
			ID:         item.ID,
			Bin:        item.Bin,
			BinaryPath: item.Detection.BinaryPath,
		})
	}
	return out
}

// selectAgent resolves one product from DetectSetupAgents. Variants are already
// collapsed there; this only menus or picks among products.
func (a *AutoSetup) selectAgent(detected []detection.DetectedAgent) (detection.DetectedAgent, error) {
	if len(detected) == 0 {
		return detection.DetectedAgent{}, fmt.Errorf("no agents detected")
	}
	products := append([]detection.DetectedAgent(nil), detected...)
	sort.Slice(products, func(i, j int) bool {
		return products[i].Key < products[j].Key
	})

	if len(products) == 1 {
		fmt.Fprintf(a.stdout, "Detected: %s\n", products[0].Key)
		return products[0], nil
	}

	if a.isTTY {
		return a.promptSelection(products, bufio.NewReader(a.stdin))
	}

	bins := make([]string, 0, len(products))
	seen := map[string]bool{}
	for _, p := range products {
		if p.Entry.Bin == "" || seen[p.Entry.Bin] {
			continue
		}
		seen[p.Entry.Bin] = true
		bins = append(bins, p.Entry.Bin)
	}
	sort.Strings(bins)
	chosen := products[0]
	fmt.Fprintf(a.stdout,
		"Detected multiple AI CLI tools (%s); using %s. Override with default_agent in config.\n",
		strings.Join(bins, ", "), chosen.Key)
	return chosen, nil
}

// promptSelection prompts the user to choose among detected products. Each
// element is one product (already collapsed to the recipe that would be
// installed). The bin name is shown in the menu.
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

// readSelection reads a single trimmed line from the shared bufio.Reader.
func readSelection(reader *bufio.Reader) (string, error) {
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	return strings.TrimSpace(input), nil
}

// loadAgentFromModule loads an agent from a fetched module directory.
func loadAgentFromModule(dir, key string, reg modconfig.Registry) (Agent, cue.Value, error) {
	cctx := cuecontext.New()

	cfg := &load.Config{
		Dir:      dir,
		Registry: reg,
	}

	insts := load.Instances([]string{"."}, cfg)
	if len(insts) == 0 {
		return Agent{}, cue.Value{}, fmt.Errorf("no CUE instances found in %s", dir)
	}

	inst := insts[0]
	if inst.Err != nil {
		return Agent{}, cue.Value{}, fmt.Errorf("loading module: %w", inst.Err)
	}

	v := cctx.BuildInstance(inst)
	if err := v.Err(); err != nil {
		return Agent{}, cue.Value{}, fmt.Errorf("building module: %w", err)
	}

	return extractAgentFromValue(v, key)
}

// extractAgentFromValue extracts agent config from a CUE value, trying multiple
// lookup paths to handle both user config and registry module formats.
func extractAgentFromValue(v cue.Value, name string) (Agent, cue.Value, error) {
	agentVal := v.LookupPath(cue.ParsePath(internalcue.KeyAgents)).LookupPath(cue.MakePath(cue.Str(name)))
	if !agentVal.Exists() {
		// Singular "agent" field is the registry module style.
		agentVal = v.LookupPath(cue.ParsePath("agent"))
	}
	if !agentVal.Exists() {
		agentVal = v
	}

	agent := extractAgentFields(agentVal, name)

	if agent.Command == "" {
		return agent, agentVal, fmt.Errorf("agent %s missing required 'command' field", name)
	}
	if agent.Bin == "" && agent.Agentdex == "" {
		return agent, agentVal, fmt.Errorf("agent %s missing required 'bin' field", name)
	}

	return agent, agentVal, nil
}

// writeConfig writes the agent entry through the shared AST upsert (same as
// install) and settings.cue for default_agent.
func (a *AutoSetup) writeConfig(agent Agent, agentVal cue.Value, origin string) (string, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(paths.Global, 0755); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	entry, err := modules.FormatModuleStruct(agentVal, "agents", origin, "")
	if err != nil {
		return "", fmt.Errorf("formatting agent entry: %w", err)
	}
	agentPath := filepath.Join(paths.Global, "agents.cue")
	if err := modules.UpsertConfigModule(agentPath, "agents", agent.Name, entry); err != nil {
		return "", fmt.Errorf("writing agents file: %w", err)
	}

	configContent := generateSettingsCUE(agent.Name)
	configPath := filepath.Join(paths.Global, "settings.cue")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return configPath, nil
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
