// Package detection handles detecting installed AI CLI tools.
package detection

import (
	"os/exec"
	"sort"
	"sync"

	"github.com/start-cli/start/internal/registry"
)

// DetectedAgent represents an agent that was found in PATH.
type DetectedAgent struct {
	Key        string // Index key, e.g., "claude/interactive"
	Entry      registry.IndexEntry
	BinaryPath string // Full path to the binary
}

// DetectAgents checks which agents from the index are installed.
// It checks each agent's bin field against PATH in parallel and returns
// every index entry whose Bin is non-empty and resolvable on PATH, sorted
// lexicographically by key. Entries with an empty Bin are skipped.
//
// When several index entries share a bin (e.g. all "claude/*" variants point
// at "claude"), every variant is returned and the caller is responsible for
// picking one — auto-setup prompts the user in TTY mode and applies a
// deterministic heuristic in non-TTY mode.
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

	sort.Slice(found, func(i, j int) bool {
		return found[i].Key < found[j].Key
	})

	return found
}

// IsBinaryAvailable checks if a specific binary is available in PATH.
func IsBinaryAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
