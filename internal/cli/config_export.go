package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
)

// addConfigExportCommand registers the "config export [category]" subcommand.
func addConfigExportCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:   "export [category]",
		Short: "Output raw CUE config to stdout",
		Long: `Output raw CUE configuration files to stdout for piping or inspection.

Provide a category (agent, role, context, task, setting) to export a single
file, or omit it to export all config files. Plural aliases are accepted.

Use --local to target project-specific configuration (.start/).`,
		Args: cobra.MaximumNArgs(1),
		RunE: runConfigExport,
	}
	parent.AddCommand(cmd)
}

// runConfigExport handles the "config export [category]" command.
func runConfigExport(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	flags := getFlags(cmd)
	local := flags.Local
	w := cmd.OutOrStdout()

	paths, err := config.ResolvePaths("")
	if err != nil {
		return fmt.Errorf("resolving config paths: %w", err)
	}

	configDir := paths.Dir(local)
	exists := paths.GlobalExists
	if local {
		exists = paths.LocalExists
	}
	if !exists {
		return fmt.Errorf("no configuration directory found at %s; run start to set up", configDir)
	}

	if len(args) > 0 {
		return exportSingleCategory(w, local, args[0])
	}
	return printCueFiles(w, configDir)
}

// exportSingleCategory outputs a single CUE config file (no header).
func exportSingleCategory(w io.Writer, local bool, category string) error {
	path, err := resolveConfigOpenPath(local, category)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("configuration file not found: %s", path)
		}
		return fmt.Errorf("reading %s: %w", path, err)
	}

	_, _ = fmt.Fprint(w, string(data))
	return nil
}

// printCueFiles reads and prints all .cue files from dir with // filename headers.
func printCueFiles(w io.Writer, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".cue" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		_, _ = fmt.Fprintf(w, "// %s\n", entry.Name())
		_, _ = fmt.Fprint(w, string(data))
		_, _ = fmt.Fprintln(w)
	}
	return nil
}
