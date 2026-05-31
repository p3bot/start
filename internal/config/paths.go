// Package config handles configuration discovery, path resolution, and validation.
package config

import (
	"os"
	"path/filepath"
)

// Scope represents the configuration scope to load.
type Scope int

const (
	// ScopeMerged loads and merges both global and local configs.
	ScopeMerged Scope = iota
	// ScopeGlobal loads only global config (~/.config/start/).
	ScopeGlobal
	// ScopeLocal loads only local config (./.start/).
	ScopeLocal
)

// String returns the string representation of the scope.
func (s Scope) String() string {
	switch s {
	case ScopeGlobal:
		return "global"
	case ScopeLocal:
		return "local"
	default:
		return "merged"
	}
}

// ScopeFromLocal maps a local-only boolean to a Scope, for call sites that
// accept --local but not --global (choice is always local-only or merged).
func ScopeFromLocal(local bool) Scope {
	if local {
		return ScopeLocal
	}
	return ScopeMerged
}

// Paths holds the resolved configuration directory paths.
type Paths struct {
	// Global is the path to the global config directory (~/.config/start/).
	Global string
	// Local is the path to the local config directory (./.start/).
	Local string
	// GlobalExists indicates whether the global config directory exists.
	GlobalExists bool
	// LocalExists indicates whether the local config directory exists.
	LocalExists bool
}

// Dir returns the config directory for the given scope.
func (p Paths) Dir(local bool) string {
	if local {
		return p.Local
	}
	return p.Global
}

// ResolvePaths discovers configuration directories.
// workingDir specifies the base directory for local config resolution.
// If workingDir is empty, the current working directory is used.
func ResolvePaths(workingDir string) (Paths, error) {
	var p Paths

	globalPath, err := globalConfigDir()
	if err != nil {
		return p, err
	}
	p.Global = globalPath
	p.GlobalExists = dirExists(globalPath)

	if workingDir == "" {
		workingDir, err = os.Getwd()
		if err != nil {
			return p, err
		}
	}
	p.Local = filepath.Join(workingDir, ".start")
	p.LocalExists = dirExists(p.Local)

	return p, nil
}

// globalConfigDir uses XDG_CONFIG_HOME if set, otherwise ~/.config/start/.
func globalConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "start"), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "start"), nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// ForScope returns the paths that should be loaded for the given scope.
// Returns a slice of paths in load order (lowest priority first).
func (p Paths) ForScope(scope Scope) []string {
	switch scope {
	case ScopeGlobal:
		if p.GlobalExists {
			return []string{p.Global}
		}
		return nil
	case ScopeLocal:
		if p.LocalExists {
			return []string{p.Local}
		}
		return nil
	default:
		// global first (lower priority), then local (higher priority)
		var paths []string
		if p.GlobalExists {
			paths = append(paths, p.Global)
		}
		if p.LocalExists {
			paths = append(paths, p.Local)
		}
		return paths
	}
}

// AnyExists returns true if any configuration directory exists.
func (p Paths) AnyExists() bool {
	return p.GlobalExists || p.LocalExists
}
