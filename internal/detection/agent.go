// Package detection handles detecting installed AI CLI tools.
package detection

import (
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/start-cli/start/internal/registry"
)

// DetectedAgent represents an agent that was found in PATH.
type DetectedAgent struct {
	Key        string // Index key, e.g., "ai/claude"
	Entry      registry.IndexEntry
	BinaryPath string // Full path to the binary
}

// DetectAgents checks which agents from the index are installed.
// It checks each agent's bin field against PATH in parallel.
//
// When multiple index entries reference the same binary (e.g. several
// "claude/*" registry variants all with bin: "claude"), only one entry is
// returned per resolved binary path. The chosen entry prefers keys ending in
// "/interactive" (the canonical user-facing default) and falls back to the
// alphabetically-first key. This keeps "Detected: <bin>" semantics tied to
// distinct tools rather than registry entries.
func DetectAgents(index *registry.Index) []DetectedAgent {
	if index == nil || len(index.Agents) == 0 {
		return nil
	}

	var (
		mu    sync.Mutex
		found []DetectedAgent
		wg    sync.WaitGroup
	)

	for key, entry := range index.Agents {
		if entry.Bin == "" {
			continue // No binary to check
		}

		wg.Add(1)
		go func(k string, e registry.IndexEntry) {
			defer wg.Done()

			path, err := exec.LookPath(e.Bin)
			if err != nil {
				return // Not found in PATH
			}

			mu.Lock()
			defer mu.Unlock()
			found = append(found, DetectedAgent{
				Key:        k,
				Entry:      e,
				BinaryPath: path,
			})
		}(key, entry)
	}

	wg.Wait()

	// Deterministic dedup: sort so that "/interactive" variants come first
	// (they're the canonical user-facing default for tools like claude that
	// publish multiple variants), with lexicographic key as the tiebreaker.
	// Then keep the first entry per BinaryPath.
	sort.Slice(found, func(i, j int) bool {
		iInter := strings.HasSuffix(found[i].Key, "/interactive")
		jInter := strings.HasSuffix(found[j].Key, "/interactive")
		if iInter != jInter {
			return iInter
		}
		return found[i].Key < found[j].Key
	})

	seen := make(map[string]bool, len(found))
	detected := make([]DetectedAgent, 0, len(found))
	for _, d := range found {
		if seen[d.BinaryPath] {
			continue
		}
		seen[d.BinaryPath] = true
		detected = append(detected, d)
	}

	return detected
}

// IsBinaryAvailable checks if a specific binary is available in PATH.
func IsBinaryAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
