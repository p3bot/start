// Package detection finds installed AI CLI tools for first-run setup.
package detection

import (
	"os/exec"
	"sort"
	"strings"

	"github.com/p3bot/start/internal/registry"
)

// DetectedAgent is one first-run product: an index recipe whose binary is
// present (catalog Found path, or unjoined LookPath).
type DetectedAgent struct {
	Key        string // Index key, e.g. "claude-code/interactive"
	Entry      registry.IndexEntry
	BinaryPath string
}

// DefaultSetupVariant is the recipe suffix first-run installs for a catalogued
// product. Catalogued Found ids are eligible only when the index has this key.
const DefaultSetupVariant = "/interactive"

// CatalogProduct is a Found agentdex agent considered for first-run setup.
type CatalogProduct struct {
	ID         string
	Bin        string
	BinaryPath string
}

// DetectSetupAgents returns one DetectedAgent per eligible first-run product.
// Catalogued Found ids are eligible only when the index has <id>/interactive.
// Unjoined index entries (tool segment not a catalogued Found id) still detect
// via index bin + LookPath and collapse to /interactive when that variant exists.
func DetectSetupAgents(index *registry.Index, products []CatalogProduct) []DetectedAgent {
	if index == nil {
		return nil
	}

	known := make(map[string]bool, len(products))
	var out []DetectedAgent
	for _, p := range products {
		if p.ID == "" {
			continue
		}
		known[p.ID] = true
		key := p.ID + DefaultSetupVariant
		entry, ok := index.Agents[key]
		if !ok {
			continue
		}
		if entry.Bin == "" {
			entry.Bin = p.Bin
		}
		out = append(out, DetectedAgent{
			Key:        key,
			Entry:      entry,
			BinaryPath: p.BinaryPath,
		})
	}

	var unjoined []DetectedAgent
	for key, entry := range index.Agents {
		if entry.Bin == "" {
			continue
		}
		tool, _, _ := strings.Cut(key, "/")
		if known[tool] {
			continue
		}
		path, err := exec.LookPath(entry.Bin)
		if err != nil {
			continue
		}
		unjoined = append(unjoined, DetectedAgent{
			Key:        key,
			Entry:      entry,
			BinaryPath: path,
		})
	}
	out = append(out, collapseUnjoined(unjoined)...)

	sort.Slice(out, func(i, j int) bool {
		return out[i].Key < out[j].Key
	})
	return out
}

func collapseUnjoined(detected []DetectedAgent) []DetectedAgent {
	if len(detected) == 0 {
		return nil
	}
	groups, binNames := groupByBin(detected)
	out := make([]DetectedAgent, 0, len(binNames))
	for _, bin := range binNames {
		out = append(out, pickInteractive(groups[bin]))
	}
	return out
}

func groupByBin(detected []DetectedAgent) (map[string][]DetectedAgent, []string) {
	groups := make(map[string][]DetectedAgent)
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

func pickInteractive(variants []DetectedAgent) DetectedAgent {
	sorted := make([]DetectedAgent, len(variants))
	copy(sorted, variants)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})
	for _, v := range sorted {
		if strings.HasSuffix(v.Key, DefaultSetupVariant) {
			return v
		}
	}
	return sorted[0]
}
