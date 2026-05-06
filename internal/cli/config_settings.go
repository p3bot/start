package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/tui"
	"github.com/spf13/cobra"
)

// addConfigSettingsCommand adds the settings subcommand to config.
func addConfigSettingsCommand(parent *cobra.Command) {
	settingsCmd := &cobra.Command{
		Use:     "settings [key] [value]",
		Aliases: []string{"setting"},
		Short:   "Manage settings configuration",
		Long: `Manage settings for start.

Available settings:
  assets_index   CUE module path for the assets index (default: built-in)
  default_agent  Agent to use when --agent not specified
  shell          Shell for command execution (default: auto-detect)
  timeout        Command timeout in seconds`,
		Example: `  start config settings                                               List all settings
  start config settings <key>                                         Show a setting value
  start config settings <key> <val>                                   Set a setting value
  start config settings <key> --unset                                 Remove a setting value
  start config settings edit                                          Open settings.cue in $EDITOR

  start config settings assets_index                                  Show current index path
  start config settings assets_index "github.com/grantcarthew/start-assets/index@v0"
  start config settings assets_index --unset                          Restore default index
  start config settings default_agent claude
  start config settings shell /bin/bash
  start config settings timeout 120`,
		Args: cobra.MaximumNArgs(2),
		RunE: executeConfigSettings,
	}

	settingsCmd.Flags().Bool("unset", false, "Remove a setting value")
	settingsCmd.Flags().Bool("json", false, "Output as JSON")

	parent.AddCommand(settingsCmd)
}

// executeConfigSettings handles the settings command.
func executeConfigSettings(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}

	stdout := cmd.OutOrStdout()
	flags := getFlags(cmd)
	local := flags.Local
	unset, _ := cmd.Flags().GetBool("unset")
	jsonFlag, _ := cmd.Flags().GetBool("json")

	if unset {
		if len(args) == 0 {
			return fmt.Errorf("--unset requires a setting key")
		}
		if len(args) > 1 {
			return fmt.Errorf("--unset takes only one argument")
		}
		return unsetSetting(stdout, flags, args[0], local)
	}

	switch len(args) {
	case 0:
		if jsonFlag {
			return listSettingsJSON(stdout, local)
		}
		// List all settings
		return listSettings(stdout, local)
	case 1:
		if args[0] == "list" || args[0] == "ls" {
			if jsonFlag {
				return listSettingsJSON(stdout, local)
			}
			return listSettings(stdout, local)
		}
		if args[0] == "edit" {
			return editSettings(local)
		}
		if jsonFlag {
			return showSettingJSON(stdout, args[0], local)
		}
		// Show single setting
		return showSetting(stdout, args[0], local)
	case 2:
		// Set setting
		return setSetting(stdout, flags, args[0], args[1], local)
	default:
		return fmt.Errorf("too many arguments")
	}
}

// listSettings displays all settings with their values and sources.
func listSettings(w io.Writer, localOnly bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	// Show config paths
	_, _ = fmt.Fprintln(w)
	printConfigPaths(w, paths)
	_, _ = fmt.Fprintln(w)

	entries, err := config.ResolveAllSettings(paths, localOnly)
	if err != nil {
		return err
	}

	_, _ = tui.ColorSettings.Fprint(w, "settings")
	_, _ = fmt.Fprintln(w, "/")
	printSettingsEntries(w, entries)
	return nil
}

// printConfigPaths displays the configuration directory paths.
func printConfigPaths(w io.Writer, paths config.Paths) {
	_, _ = tui.ColorPaths.Fprintln(w, "Configuration Paths:")
	globalStatus := "not found"
	if paths.GlobalExists {
		globalStatus = "exists"
	}
	localStatus := "not found"
	if paths.LocalExists {
		localStatus = "exists"
	}
	_, _ = tui.ColorDim.Fprintf(w, "  Global: ")
	_, _ = fmt.Fprintf(w, "%s ", paths.Global)
	_, _ = fmt.Fprintln(w, tui.Annotate("%s", globalStatus))
	_, _ = tui.ColorDim.Fprintf(w, "  Local:  ")
	_, _ = fmt.Fprintf(w, "%s ", paths.Local)
	_, _ = fmt.Fprintln(w, tui.Annotate("%s", localStatus))
}

// printSettingsEntries displays resolved setting entries in a formatted table.
func printSettingsEntries(w io.Writer, entries map[string]config.SettingEntry) {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	maxLen := 0
	for _, k := range keys {
		if len(k) > maxLen {
			maxLen = len(k)
		}
	}

	for _, k := range keys {
		entry := entries[k]
		_, _ = tui.ColorDim.Fprintf(w, "%*s: ", maxLen, k)
		if entry.Source == "not set" {
			_, _ = fmt.Fprintln(w, tui.Annotate("not set"))
		} else {
			source := entry.Source
			if _, known := config.SettingsRegistry[k]; !known {
				source += ", unknown key"
			}
			_, _ = fmt.Fprintf(w, "%s %s\n", entry.Value, tui.Annotate("%s", source))
		}
	}
}

// showSetting displays a single setting value with its source.
func showSetting(w io.Writer, key string, localOnly bool) error {
	if _, valid := config.SettingsRegistry[key]; !valid {
		return fmt.Errorf("unknown setting %q\n\nValid settings: %s", key, config.ValidSettingsKeysString())
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	entries, err := config.ResolveAllSettings(paths, localOnly)
	if err != nil {
		return err
	}

	entry := entries[key]
	_, _ = tui.ColorDim.Fprintf(w, "%s: ", key)
	if entry.Source == "not set" {
		_, _ = fmt.Fprintln(w, tui.Annotate("not set"))
	} else {
		_, _ = fmt.Fprintf(w, "%s %s\n", entry.Value, tui.Annotate("%s", entry.Source))
	}

	return nil
}

// listSettingsJSON outputs all settings as a JSON object keyed by setting name.
func listSettingsJSON(w io.Writer, localOnly bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	entries, err := config.ResolveAllSettings(paths, localOnly)
	if err != nil {
		return err
	}

	if err := writeJSON(w, entries); err != nil {
		return fmt.Errorf("marshalling settings: %w", err)
	}
	return nil
}

// showSettingJSON outputs a single setting as a JSON object.
func showSettingJSON(w io.Writer, key string, localOnly bool) error {
	if _, valid := config.SettingsRegistry[key]; !valid {
		return fmt.Errorf("unknown setting %q\n\nValid settings: %s", key, config.ValidSettingsKeysString())
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	entries, err := config.ResolveAllSettings(paths, localOnly)
	if err != nil {
		return err
	}

	entry := entries[key]
	if err := writeJSON(w, entry); err != nil {
		return fmt.Errorf("marshalling setting: %w", err)
	}
	return nil
}

// setSetting sets a setting value.
func setSetting(w io.Writer, flags *Flags, key, value string, localOnly bool) error {
	info, valid := config.SettingsRegistry[key]
	if !valid {
		return fmt.Errorf("unknown setting %q\n\nValid settings: %s", key, config.ValidSettingsKeysString())
	}

	if info.Type == "int" {
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("setting %q requires an integer value", key)
		}
	}

	// Get config directory
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(localOnly)

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Load existing settings (error ignored: missing file means start fresh)
	settings, _ := config.LoadSettingsFromDir(configDir)
	if settings == nil {
		settings = make(map[string]string)
	}

	// Set the value
	settings[key] = value

	// Write settings file
	settingsPath := filepath.Join(configDir, "settings.cue")
	if err := writeSettingsFile(settingsPath, settings); err != nil {
		return fmt.Errorf("writing settings file: %w", err)
	}

	if !flags.Quiet {
		_, _ = fmt.Fprintf(w, "Set %s to %q\n", key, value)
	}

	return nil
}

// editSettings opens the settings file in the user's editor.
func editSettings(localOnly bool) error {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(localOnly)

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	settingsPath := filepath.Join(configDir, "settings.cue")

	// Create file if it doesn't exist
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		if err := writeSettingsFile(settingsPath, nil); err != nil {
			return fmt.Errorf("creating settings file: %w", err)
		}
	}

	return openInEditor(settingsPath)
}

// loadSettingsForScope loads settings from the appropriate scope.
// Returns an empty map if no settings are configured.
func loadSettingsForScope(localOnly bool) (map[string]string, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return nil, fmt.Errorf("resolving config paths: %w", err)
	}

	settings := make(map[string]string)

	if localOnly {
		if paths.LocalExists {
			localSettings, err := config.LoadSettingsFromDir(paths.Local)
			if err != nil {
				return nil, err
			}
			for k, v := range localSettings {
				settings[k] = v
			}
		}
	} else {
		if paths.GlobalExists {
			globalSettings, err := config.LoadSettingsFromDir(paths.Global)
			if err != nil {
				return nil, err
			}
			for k, v := range globalSettings {
				settings[k] = v
			}
		}
		if paths.LocalExists {
			localSettings, err := config.LoadSettingsFromDir(paths.Local)
			if err != nil {
				return nil, err
			}
			for k, v := range localSettings {
				settings[k] = v
			}
		}
	}

	return settings, nil
}

// hasNonSettingsContent checks if CUE content has non-settings top-level keys.
// This prevents accidental data loss when overwriting settings.cue.
func hasNonSettingsContent(content string) bool {
	ctx := cuecontext.New()
	v := ctx.CompileString(content)
	if v.Err() != nil {
		return false
	}

	nonSettingsKeys := []string{
		internalcue.KeyAgents, internalcue.KeyContexts,
		internalcue.KeyRoles, internalcue.KeyTasks,
	}
	for _, key := range nonSettingsKeys {
		if v.LookupPath(cue.ParsePath(key)).Exists() {
			return true
		}
	}
	return false
}

// writeSettingsFile writes the settings to a CUE file.
// It checks for existing non-settings content to prevent data loss.
func writeSettingsFile(path string, settings map[string]string) error {
	// Check if file exists with non-settings content
	if existingContent, err := os.ReadFile(path); err == nil {
		if hasNonSettingsContent(string(existingContent)) {
			return fmt.Errorf("settings.cue contains non-settings content (agents, roles, etc.)\n\nPlease edit the file manually: %s", path)
		}
	}

	var sb strings.Builder

	sb.WriteString("// Auto-generated by start config\n")
	sb.WriteString("// Edit this file to customize your settings\n\n")
	sb.WriteString("settings: {\n")

	if len(settings) > 0 {
		// Sort keys for consistent output
		var keys []string
		for k := range settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := settings[k]
			// Check if this is an int setting
			if info, exists := config.SettingsRegistry[k]; exists && info.Type == "int" {
				// Write as integer (no quotes)
				sb.WriteString(fmt.Sprintf("\t%s: %s\n", k, v))
			} else {
				// Write as string (with quotes)
				sb.WriteString(fmt.Sprintf("\t%s: %q\n", k, v))
			}
		}
	}

	sb.WriteString("}\n")

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

// unsetSetting removes a setting key from the settings file.
func unsetSetting(w io.Writer, flags *Flags, key string, localOnly bool) error {
	if _, valid := config.SettingsRegistry[key]; !valid {
		return fmt.Errorf("unknown setting %q\n\nValid settings: %s", key, config.ValidSettingsKeysString())
	}

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(localOnly)

	// Only load (and propagate errors from) the settings file if it exists.
	// Absence of the file is not an error — it means nothing is configured.
	settingsPath := filepath.Join(configDir, "settings.cue")
	if _, statErr := os.Stat(settingsPath); os.IsNotExist(statErr) {
		if !flags.Quiet {
			_, _ = fmt.Fprintf(w, "%s is not set\n", key)
		}
		return nil
	}

	settings, err := config.LoadSettingsFromDir(configDir)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}
	if len(settings) == 0 {
		if !flags.Quiet {
			_, _ = fmt.Fprintf(w, "%s is not set\n", key)
		}
		return nil
	}

	if _, exists := settings[key]; !exists {
		if !flags.Quiet {
			_, _ = fmt.Fprintf(w, "%s is not set\n", key)
		}
		return nil
	}

	delete(settings, key)

	if err := writeSettingsFile(settingsPath, settings); err != nil {
		return fmt.Errorf("writing settings file: %w", err)
	}

	if !flags.Quiet {
		_, _ = fmt.Fprintf(w, "Unset %s\n", key)
	}
	return nil
}
