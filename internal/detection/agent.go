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

// DetectAgents returns every index entry whose Bin is non-empty and resolvable
// on PATH, sorted by key. Entries sharing a bin are all returned; choosing among
// variants is the caller's job (auto-setup prompts in TTY, heuristic otherwise).
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
			continue
		}

		wg.Add(1)
		go func(k string, e registry.IndexEntry) {
			defer wg.Done()

			path, err := exec.LookPath(e.Bin)
			if err != nil {
				return
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
