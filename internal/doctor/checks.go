package doctor

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/orchestration"
)

// CheckIntro returns the intro section with repository info.
func CheckIntro() SectionResult {
	return SectionResult{
		Name:    "Repository",
		NoIcons: true,
		Results: []CheckResult{
			{Status: StatusInfo, Label: RepoURL},
			{Status: StatusInfo, Label: IssuesURL},
		},
	}
}

// CheckVersion returns the version section with build info.
func CheckVersion(info BuildInfo) SectionResult {
	section := SectionResult{
		Name:    "Version",
		NoIcons: true,
		Results: []CheckResult{
			{Status: StatusInfo, Label: "start " + info.Version},
			{Status: StatusInfo, Label: "Commit", Message: info.Commit},
			{Status: StatusInfo, Label: "Built", Message: info.BuildDate},
			{Status: StatusInfo, Label: "Go", Message: info.GoVersion},
			{Status: StatusInfo, Label: "Platform", Message: info.Platform},
		},
	}

	if info.IndexVersion != "" {
		section.Results = append(section.Results, CheckResult{
			Status: StatusInfo, Label: "Index", Message: info.IndexVersion,
		})
	} else {
		section.Results = append(section.Results, CheckResult{
			Status: StatusWarn, Label: "Index", Message: "unavailable",
		})
	}

	if info.IndexPath != "" {
		section.Results = append(section.Results, CheckResult{
			Status: StatusInfo, Label: "Index Source", Message: info.IndexPath,
		})
	}

	return section
}

// CheckConfiguration validates CUE configuration files.
func CheckConfiguration(paths config.Paths) SectionResult {
	section := SectionResult{Name: "Configuration"}

	// Check global config
	globalResults := checkConfigDir(paths.Global, "Global", paths.GlobalExists)
	section.Results = append(section.Results, globalResults...)

	// Check local config
	localResults := checkConfigDir(paths.Local, "Local", paths.LocalExists)
	section.Results = append(section.Results, localResults...)

	// If both exist, try to load and merge
	if paths.GlobalExists || paths.LocalExists {
		loader := internalcue.NewLoader()
		dirs := paths.ForScope(config.ScopeMerged)
		_, err := loader.Load(dirs)
		if err != nil {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusFail,
				Label:   "Merge",
				Message: "Failed",
				Fix:     fmt.Sprintf("Fix CUE syntax: %v", err),
			})
		} else {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusPass,
				Label:   "Validation",
				Message: "Valid",
			})
		}
	}

	return section
}

// checkConfigDir checks a single configuration directory.
func checkConfigDir(dir, scope string, exists bool) []CheckResult {
	var results []CheckResult

	// Build display path like "~/.config/start" for check output
	shortDir := shortenPath(dir)

	// Directory header (always present, no icon)
	header := CheckResult{
		Status: StatusInfo,
		Label:  fmt.Sprintf("%s (%s)", scope, shortDir),
		NoIcon: true,
	}

	if !exists {
		results = append(results, header)
		results = append(results, CheckResult{
			Status: StatusInfo,
			Label:  "Not found",
			Indent: 1,
		})
		return results
	}

	// List CUE files in directory
	entries, err := os.ReadDir(dir)
	if err != nil {
		results = append(results, header)
		results = append(results, CheckResult{
			Status:  StatusFail,
			Label:   "Cannot read",
			Message: fmt.Sprintf("%v", err),
			Indent:  1,
			Fix:     "Check directory permissions",
		})
		return results
	}

	var cueFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".cue") {
			cueFiles = append(cueFiles, entry.Name())
		}
	}

	if len(cueFiles) == 0 {
		results = append(results, header)
		results = append(results, CheckResult{
			Status: StatusInfo,
			Label:  "No CUE files",
			Indent: 1,
		})
		return results
	}

	// Try to load the directory to validate CUE syntax
	loader := internalcue.NewLoader()
	_, err = loader.LoadSingle(dir)
	if err != nil {
		results = append(results, header)
		results = append(results, CheckResult{
			Status:  StatusFail,
			Label:   "Invalid",
			Message: fmt.Sprintf("%v", err),
			Indent:  1,
			Fix:     "Fix CUE syntax errors in this directory",
		})
	} else {
		results = append(results, header)
		for _, f := range cueFiles {
			results = append(results, CheckResult{
				Status: StatusPass,
				Label:  f,
				Indent: 1,
			})
		}
	}

	return results
}

// CheckAgents validates configured agent binaries are available.
func CheckAgents(cfgValue cue.Value) SectionResult {
	section := SectionResult{Name: "Agents"}

	agents := cfgValue.LookupPath(cue.ParsePath(internalcue.KeyAgents))
	if !agents.Exists() {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusInfo,
			Label:   "None configured",
			Message: "",
		})
		return section
	}

	iter, err := agents.Fields()
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Error",
			Message: fmt.Sprintf("Cannot read agents: %v", err),
		})
		return section
	}

	count := 0
	for iter.Next() {
		count++
		name := iter.Selector().Unquoted()
		agent := iter.Value()

		binVal := agent.LookupPath(cue.ParsePath("bin"))
		if !binVal.Exists() {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusWarn,
				Label:   name,
				Message: "No bin field",
			})
			continue
		}

		bin, err := binVal.String()
		if err != nil {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusWarn,
				Label:   name,
				Message: "Invalid bin field",
			})
			continue
		}

		path, err := exec.LookPath(bin)
		if err != nil {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusFail,
				Label:   name,
				Message: "NOT FOUND",
				Fix:     fmt.Sprintf("Install %s or remove from config", bin),
			})
		} else {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusPass,
				Label:   name,
				Message: path,
			})
		}
	}

	if count > 0 {
		section.Summary = fmt.Sprintf("%d configured", count)
	}

	return section
}

// CheckRoles validates configured role files exist.
func CheckRoles(cfgValue cue.Value) SectionResult {
	section := SectionResult{Name: "Roles"}

	roles := cfgValue.LookupPath(cue.ParsePath(internalcue.KeyRoles))
	if !roles.Exists() {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusInfo,
			Label:   "None configured",
			Message: "",
		})
		return section
	}

	iter, err := roles.Fields()
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Error",
			Message: fmt.Sprintf("Cannot read roles: %v", err),
		})
		return section
	}

	count := 0
	for iter.Next() {
		count++
		name := iter.Selector().Unquoted()
		role := iter.Value()

		result := checkFileField(role, name)
		if result != nil {
			section.Results = append(section.Results, *result)
		}
	}

	if count > 0 {
		section.Summary = fmt.Sprintf("%d configured", count)
	}

	return section
}

// CheckContexts validates configured context files exist.
func CheckContexts(cfgValue cue.Value) SectionResult {
	section := SectionResult{Name: "Contexts"}

	contexts := cfgValue.LookupPath(cue.ParsePath(internalcue.KeyContexts))
	if !contexts.Exists() {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusInfo,
			Label:   "None configured",
			Message: "",
		})
		return section
	}

	iter, err := contexts.Fields()
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Error",
			Message: fmt.Sprintf("Cannot read contexts: %v", err),
		})
		return section
	}

	count := 0
	for iter.Next() {
		count++
		name := iter.Selector().Unquoted()
		ctx := iter.Value()

		result := checkFileField(ctx, name)
		if result != nil {
			section.Results = append(section.Results, *result)
		}
	}

	if count > 0 {
		section.Summary = fmt.Sprintf("%d configured", count)
	}

	return section
}

// CheckTasks validates configured task files exist.
func CheckTasks(cfgValue cue.Value) SectionResult {
	section := SectionResult{Name: "Tasks"}

	tasks := cfgValue.LookupPath(cue.ParsePath(internalcue.KeyTasks))
	if !tasks.Exists() {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusInfo,
			Label:   "None configured",
			Message: "",
		})
		return section
	}

	iter, err := tasks.Fields()
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Error",
			Message: fmt.Sprintf("Cannot read tasks: %v", err),
		})
		return section
	}

	count := 0
	for iter.Next() {
		count++
		name := iter.Selector().Unquoted()
		task := iter.Value()

		result := checkFileField(task, name)
		if result != nil {
			section.Results = append(section.Results, *result)
		}

		if roleResult := checkTaskRole(task, name, cfgValue); roleResult != nil {
			section.Results = append(section.Results, *roleResult)
		}
	}

	if count > 0 {
		section.Summary = fmt.Sprintf("%d configured", count)
	}

	return section
}

// checkTaskRole checks if a task's role field references an existing role.
func checkTaskRole(taskVal cue.Value, taskName string, cfgValue cue.Value) *CheckResult {
	roleVal := taskVal.LookupPath(cue.ParsePath("role"))
	if !roleVal.Exists() {
		return nil
	}

	roleName, err := roleVal.String()
	if err != nil {
		// Role is a struct (inline definition), skip validation
		return nil
	}

	roles := cfgValue.LookupPath(cue.ParsePath(internalcue.KeyRoles))
	if roles.Exists() {
		role := roles.LookupPath(cue.MakePath(cue.Str(roleName)))
		if role.Exists() {
			return nil
		}
	}

	return &CheckResult{
		Status:  StatusWarn,
		Label:   fmt.Sprintf("role %q", roleName),
		Message: "not found in roles config",
		Fix:     fmt.Sprintf("Add %q to roles or fix the reference in task %q", roleName, taskName),
		Indent:  1,
	}
}

// checkFileField checks if a config item has a valid file field.
func checkFileField(v cue.Value, name string) *CheckResult {
	fileVal := v.LookupPath(cue.ParsePath("file"))
	if !fileVal.Exists() {
		// No file field - check for prompt or command instead
		if prompt := v.LookupPath(cue.ParsePath("prompt")); prompt.Exists() {
			return &CheckResult{
				Status:  StatusPass,
				Label:   name,
				Message: "(inline prompt)",
			}
		}
		if cmd := v.LookupPath(cue.ParsePath("command")); cmd.Exists() {
			return &CheckResult{
				Status:  StatusPass,
				Label:   name,
				Message: "(command)",
			}
		}
		return &CheckResult{
			Status:  StatusWarn,
			Label:   name,
			Message: "No file, prompt, or command",
		}
	}

	filePath, err := fileVal.String()
	if err != nil {
		return &CheckResult{
			Status:  StatusWarn,
			Label:   name,
			Message: "Invalid file field",
		}
	}

	// @module/ paths resolve via the role/context/task's origin field against
	// the CUE module cache. Honestly report whether the extract is present.
	if strings.HasPrefix(filePath, "@module/") {
		origin := orchestration.ExtractOrigin(v)
		if origin == "" {
			return &CheckResult{
				Status:  StatusFail,
				Label:   name,
				Message: "@module/ path without origin field",
				Fix:     fmt.Sprintf("Add an origin field to %q, or replace the @module/ path with a local file", name),
			}
		}
		resolved, err := orchestration.ResolveModulePath(filePath, origin)
		if err != nil {
			// Extract dir is genuinely missing from the cache.
			return &CheckResult{
				Status:  StatusNotFound,
				Label:   name,
				Message: fmt.Sprintf("module not extracted (%s)", originVersion(origin)),
				Fix:     fmt.Sprintf("Run 'start modules install %s' to fetch the module", name),
			}
		}
		if _, err := os.Stat(resolved); os.IsNotExist(err) {
			// Extract dir is present but the declared file is missing
			// from it — config path is wrong, or the module shipped
			// without the file. Reinstall will not help.
			return &CheckResult{
				Status:  StatusNotFound,
				Label:   name,
				Message: fmt.Sprintf("%s not in module (%s)", filePath, originVersion(origin)),
				Fix:     fmt.Sprintf("Verify %q exists in module %s, or update the file field on %q", filePath, origin, name),
			}
		} else if err != nil {
			return &CheckResult{
				Status:  StatusFail,
				Label:   name,
				Message: fmt.Sprintf("%s (%v)", filePath, err),
			}
		}
		// Source the version from the resolved path: ResolveModulePath
		// can fall back to a different cached version than declared, and
		// the user needs to see what they will actually get.
		relativePath := strings.TrimPrefix(filePath, "@module/")
		versionedDir := filepath.Base(strings.TrimSuffix(resolved, string(filepath.Separator)+relativePath))
		return &CheckResult{
			Status:  StatusPass,
			Label:   name,
			Message: fmt.Sprintf("(registry module %s)", originVersion(versionedDir)),
		}
	}

	// Expand ~ to home directory
	expandedPath := expandPath(filePath)

	if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
		return &CheckResult{
			Status:  StatusNotFound,
			Label:   name,
			Message: shortenPath(filePath),
		}
	} else if err != nil {
		return &CheckResult{
			Status:  StatusFail,
			Label:   name,
			Message: fmt.Sprintf("%s (%v)", shortenPath(filePath), err),
		}
	}

	return &CheckResult{
		Status:  StatusPass,
		Label:   name,
		Message: shortenPath(filePath),
	}
}

// CheckSettings resolves and displays all settings with their sources,
// and validates specific settings (default_agent, shell).
func CheckSettings(paths config.Paths, cfgValue cue.Value) SectionResult {
	section := SectionResult{Name: "Settings"}

	entries, err := config.ResolveAllSettings(paths, config.ScopeMerged)
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusWarn,
			Label:   "Error",
			Message: fmt.Sprintf("cannot resolve settings: %v", err),
		})
		return section
	}

	keys := sortedKeys(entries)

	for _, key := range keys {
		entry := entries[key]

		switch key {
		case "default_agent":
			section.Results = append(section.Results, checkDefaultAgent(entry, cfgValue))
		case "shell":
			section.Results = append(section.Results, checkShell(entry))
		default:
			section.Results = append(section.Results, settingResult(key, entry))
		}
	}

	return section
}

// settingResult creates a CheckResult for a setting with its value and source.
func settingResult(key string, entry config.SettingEntry) CheckResult {
	if entry.Source == "not set" {
		return CheckResult{
			Status:  StatusInfo,
			Label:   key,
			Message: "(not set)",
		}
	}
	return CheckResult{
		Status:  StatusPass,
		Label:   key,
		Message: fmt.Sprintf("%s (%s)", entry.Value, entry.Source),
	}
}

// checkDefaultAgent validates the default_agent setting.
func checkDefaultAgent(entry config.SettingEntry, cfgValue cue.Value) CheckResult {
	if entry.Source == "not set" {
		return settingResult("default_agent", entry)
	}

	message := fmt.Sprintf("%s (%s)", entry.Value, entry.Source)

	// Config not loaded (zero cue.Value) — show the value but skip agent verification.
	if !cfgValue.Exists() {
		return CheckResult{
			Status:  StatusInfo,
			Label:   "default_agent",
			Message: message + " (cannot verify: config not loaded)",
		}
	}

	agents := cfgValue.LookupPath(cue.ParsePath(internalcue.KeyAgents))
	if !agents.Exists() {
		return CheckResult{
			Status:  StatusWarn,
			Label:   "default_agent",
			Message: message,
			Fix:     "No agents configured; add an agents section or remove default_agent",
		}
	}

	agent := agents.LookupPath(cue.MakePath(cue.Str(entry.Value)))
	if agent.Exists() {
		return CheckResult{
			Status:  StatusPass,
			Label:   "default_agent",
			Message: message,
		}
	}

	return CheckResult{
		Status:  StatusWarn,
		Label:   "default_agent",
		Message: message,
		Fix:     fmt.Sprintf("Agent %q not found in config; check spelling or add it to agents", entry.Value),
	}
}

// checkShell validates the shell setting.
func checkShell(entry config.SettingEntry) CheckResult {
	if entry.Source == "not set" {
		return settingResult("shell", entry)
	}

	message := fmt.Sprintf("%s (%s)", entry.Value, entry.Source)

	_, lookErr := exec.LookPath(entry.Value)
	if lookErr != nil {
		return CheckResult{
			Status:  StatusWarn,
			Label:   "shell",
			Message: message,
			Fix:     fmt.Sprintf("Install %s or update settings.shell", entry.Value),
		}
	}

	return CheckResult{
		Status:  StatusPass,
		Label:   "shell",
		Message: message,
	}
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// CheckEnvironment validates runtime environment.
func CheckEnvironment(paths config.Paths) SectionResult {
	section := SectionResult{Name: "Environment"}

	// Check config directory is writable (if it exists)
	if paths.GlobalExists {
		if isWritable(paths.Global) {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusPass,
				Label:   "Config directory",
				Message: "writable",
			})
		} else {
			section.Results = append(section.Results, CheckResult{
				Status:  StatusFail,
				Label:   "Config directory",
				Message: "not writable",
				Fix:     fmt.Sprintf("Check permissions on %s", paths.Global),
			})
		}
	}

	// Check working directory is accessible
	wd, err := os.Getwd()
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusFail,
			Label:   "Working directory",
			Message: fmt.Sprintf("Cannot access: %v", err),
		})
	} else {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusPass,
			Label:   "Working directory",
			Message: shortenPath(wd),
		})
	}

	return section
}

// CheckCache checks the registry index cache status.
func CheckCache() SectionResult {
	section := SectionResult{Name: "Cache"}

	cached, err := cache.ReadIndex()
	if err != nil {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusNotFound,
			Label:   "Index cache",
			Message: "not found",
			Fix:     "Run any registry command (e.g., start modules list) to create the cache",
		})
		return section
	}

	age := time.Since(cached.Updated)
	ageStr := formatDuration(age)

	if cached.IsFresh(cache.DefaultMaxAge) {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusPass,
			Label:   "Index cache",
			Message: fmt.Sprintf("fresh (%s ago)", ageStr),
		})
	} else {
		section.Results = append(section.Results, CheckResult{
			Status:  StatusWarn,
			Label:   "Index cache",
			Message: fmt.Sprintf("stale (%s ago)", ageStr),
			Fix:     "Run any registry command to refresh the cache",
		})
	}

	return section
}

// formatDuration returns a human-readable duration string.
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		s := int(d.Seconds())
		if s == 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", s)
	}
	if d < time.Hour {
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", m)
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	days := int(math.Round(d.Hours() / 24))
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// originVersion returns the version suffix from an origin string, or the
// origin itself if no version is present.
func originVersion(origin string) string {
	if idx := strings.LastIndex(origin, "@"); idx != -1 {
		return origin[idx+1:]
	}
	return origin
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// shortenPath replaces home directory with ~.
func shortenPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

// isWritable checks if a directory is writable.
func isWritable(dir string) bool {
	testFile := filepath.Join(dir, ".write-test")
	f, err := os.Create(testFile)
	if err != nil {
		return false
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(testFile)
		return false
	}
	_ = os.Remove(testFile)
	return true
}
