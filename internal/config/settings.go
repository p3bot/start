package config

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/registry"
	"github.com/p3bot/start/internal/shell"
)

// SettingInfo describes a valid settings key with its type.
type SettingInfo struct {
	Type string // "string" or "int"
}

// SettingEntry holds a resolved setting value and its source.
type SettingEntry struct {
	Value  string `json:"value"`
	Source string `json:"source"` // "default", "global", "local", or "not set"
}

// SettingsRegistry defines all valid settings keys and their types.
var SettingsRegistry = map[string]SettingInfo{
	"library_index": {Type: "string"},
	"default_agent": {Type: "string"},
	"shell":         {Type: "string"},
	"timeout":       {Type: "int"},
}

// ValidSettingsKeysString returns a sorted, comma-separated list of valid setting keys.
func ValidSettingsKeysString() string {
	keys := make([]string, 0, len(SettingsRegistry))
	for k := range SettingsRegistry {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// SettingDefault returns the default value for a setting key, or "" if none.
func SettingDefault(key string) string {
	switch key {
	case "library_index":
		return registry.IndexModulePath
	case "shell":
		if detected, err := shell.DetectShell(); err == nil {
			return strings.TrimSuffix(detected, " -c")
		}
		return ""
	case "timeout":
		return strconv.Itoa(shell.DefaultTimeout)
	default:
		return ""
	}
}

// ResolveAllSettings resolves all valid settings with their values and sources.
// Defaults are seeded first, then scope-selected overrides apply on top:
// ScopeLocal local-only, ScopeGlobal global-only, ScopeMerged global then local
// (local wins).
func ResolveAllSettings(paths Paths, scope Scope) (map[string]SettingEntry, error) {
	entries := make(map[string]SettingEntry, len(SettingsRegistry))

	for key := range SettingsRegistry {
		if def := SettingDefault(key); def != "" {
			entries[key] = SettingEntry{Value: def, Source: "default"}
		} else {
			entries[key] = SettingEntry{Source: "not set"}
		}
	}

	switch scope {
	case ScopeLocal:
		if paths.LocalExists {
			localSettings, err := LoadSettingsFromDir(paths.Local)
			if err != nil {
				return nil, err
			}
			for k, v := range localSettings {
				entries[k] = SettingEntry{Value: v, Source: "local"}
			}
		}
	case ScopeGlobal:
		if paths.GlobalExists {
			globalSettings, err := LoadSettingsFromDir(paths.Global)
			if err != nil {
				return nil, err
			}
			for k, v := range globalSettings {
				entries[k] = SettingEntry{Value: v, Source: "global"}
			}
		}
	default:
		if paths.GlobalExists {
			globalSettings, err := LoadSettingsFromDir(paths.Global)
			if err != nil {
				return nil, err
			}
			for k, v := range globalSettings {
				entries[k] = SettingEntry{Value: v, Source: "global"}
			}
		}
		if paths.LocalExists {
			localSettings, err := LoadSettingsFromDir(paths.Local)
			if err != nil {
				return nil, err
			}
			for k, v := range localSettings {
				entries[k] = SettingEntry{Value: v, Source: "local"}
			}
		}
	}

	return entries, nil
}

// LoadSettingsFromDir loads settings from a specific directory.
func LoadSettingsFromDir(dir string) (map[string]string, error) {
	settings := make(map[string]string)

	loader := internalcue.NewLoader()
	result, err := loader.Load([]string{dir})
	if err != nil {
		if errors.Is(err, internalcue.ErrNoCUEFiles) {
			return settings, nil
		}
		return nil, err
	}

	settingsVal := result.Value.LookupPath(cue.ParsePath(internalcue.KeySettings))
	if !settingsVal.Exists() {
		return settings, nil
	}

	iter, err := settingsVal.Fields(cue.Concrete(true))
	if err != nil {
		return nil, fmt.Errorf("iterating settings: %w", err)
	}

	for iter.Next() {
		key := iter.Selector().Unquoted()

		switch iter.Value().Kind() {
		case cue.StringKind:
			if str, err := iter.Value().String(); err == nil {
				settings[key] = str
			}
		case cue.IntKind:
			if i, err := iter.Value().Int64(); err == nil {
				settings[key] = strconv.FormatInt(i, 10)
			}
		}
	}

	return settings, nil
}
