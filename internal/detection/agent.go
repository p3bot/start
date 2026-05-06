// Package detection handles detecting installed AI CLI tools.
package detection

import (
	"os/exec"
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
func DetectAgents(index *registry.Index) []DetectedAgent {
	if index == nil || len(index.Agents) == 0 {
		return nil
	}

	var (
		mu       sync.Mutex
		detected []DetectedAgent
		wg       sync.WaitGroup
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
			detected = append(detected, DetectedAgent{
				Key:        k,
				Entry:      e,
				BinaryPath: path,
			})
		}(key, entry)
	}

	wg.Wait()
	return detected
}

// IsBinaryAvailable checks if a specific binary is available in PATH.
func IsBinaryAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
