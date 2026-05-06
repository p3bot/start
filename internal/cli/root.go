package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// IsSilentError returns true if the error should not be printed to stderr.
// Used by main.go to suppress output for errors that only set the exit code.
func IsSilentError(err error) bool {
	type silent interface {
		Silent() bool
	}
	if s, ok := err.(silent); ok {
		return s.Silent()
	}
	return false
}

// Build-time variables set via ldflags
var (
	cliVersion = "dev"
	commit     = "unknown"
	buildDate  = "unknown"
	repoURL    = "https://github.com/grantcarthew/start"
)

var versionTemplate = fmt.Sprintf(`start version %s
%s
%s/issues/new
`, cliVersion, repoURL, repoURL)

// NewRootCmd creates a new root command instance with all subcommands attached.
// This factory function ensures tests get isolated command instances with their own Flags.
func NewRootCmd() *cobra.Command {
	// Create flags scoped to this command instance
	flags := &Flags{}

	cmd := &cobra.Command{
		Use:   "start",
		Short: "AI agent CLI orchestrator",
		Long: `start is a command-line orchestrator for AI agents built on CUE.
It manages prompt composition, context injection, and workflow automation.

Examples:
  start                           Launch agent with default role and contexts
  start --role go-expert          Launch with a specific role
  start task review/pre-commit    Run a predefined task
  start doctor                    Check installation and configuration`,
		Version: cliVersion,
		// SilenceUsage prevents usage from being printed on RunE errors.
		// Usage is still shown for flag/argument parsing errors.
		SilenceUsage: true,
		// SilenceErrors prevents Cobra from printing errors - we handle them
		// ourselves in main.go with colored output.
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Store flags in context for access by all commands
			ctx := context.WithValue(cmd.Context(), flagsKey{}, flags)
			cmd.SetContext(ctx)

			// Apply --no-color flag to disable colors globally
			if flags.NoColor {
				color.NoColor = true
			}

			// Debug implies verbose
			if flags.Debug {
				flags.Verbose = true
			}
			return nil
		},
	}

	// Custom version template
	cmd.SetVersionTemplate(versionTemplate)

	// Add persistent flags bound to this instance's Flags struct
	cmd.PersistentFlags().StringVarP(&flags.Agent, "agent", "a", "", "Override agent selection")
	cmd.PersistentFlags().StringVarP(&flags.Role, "role", "r", "", "Override role (config name or file path)")
	cmd.PersistentFlags().StringVarP(&flags.Model, "model", "m", "", "Override model selection")
	cmd.PersistentFlags().StringSliceVarP(&flags.Context, "context", "c", nil, "Select contexts (tags or file paths)")
	cmd.PersistentFlags().BoolVar(&flags.DryRun, "dry-run", false, "Preview execution without launching agent")
	cmd.PersistentFlags().BoolVarP(&flags.Quiet, "quiet", "q", false, "Suppress output")
	cmd.PersistentFlags().BoolVar(&flags.Verbose, "verbose", false, "Detailed output")
	cmd.PersistentFlags().BoolVar(&flags.Debug, "debug", false, "Debug output (implies --verbose)")
	cmd.PersistentFlags().BoolVar(&flags.NoColor, "no-color", false, "Disable colored output")
	cmd.PersistentFlags().BoolVarP(&flags.Local, "local", "l", false, "Target local config (./.start/) instead of global")
	cmd.PersistentFlags().BoolVar(&flags.NoRole, "no-role", false, "Skip role assignment")
	cmd.MarkFlagsMutuallyExclusive("role", "no-role")

	// Set RunE on root command for `start` execution
	cmd.RunE = runStart

	// Define command groups for help output
	cmd.AddGroup(
		&cobra.Group{ID: "commands", Title: "Commands:"},
		&cobra.Group{ID: "utilities", Title: "Utilities:"},
	)

	// Add subcommands
	addShowCommand(cmd)
	addReadCommand(cmd)
	addPromptCommand(cmd)
	addTaskCommand(cmd)
	addAssetsCommand(cmd)
	addConfigCommand(cmd)
	addSearchCommand(cmd)
	addDoctorCommand(cmd)
	addCompletionCommand(cmd)

	// Replace default help command with one that includes agent-focused topic subcommands
	addHelpCommand(cmd)

	return cmd
}

// Execute runs the root command. This is the main entry point for the CLI.
func Execute() error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("start does not support Windows")
	}
	return NewRootCmd().Execute()
}

// checkHelpArg checks if the first argument is "help" and shows help if so.
// Returns true if help was shown, false otherwise.
// Use this at the top of RunE on commands that use noArgsOrHelp as their Args validator.
func checkHelpArg(cmd *cobra.Command, args []string) (bool, error) {
	if len(args) > 0 && args[0] == "help" {
		return true, cmd.Help()
	}
	return false, nil
}

// unknownCommandError returns a formatted error for unknown subcommands.
func unknownCommandError(cmdPath, arg string) error {
	return fmt.Errorf("unknown command %q for %q\nRun '%s --help' for usage", arg, cmdPath, cmdPath)
}

// noArgsOrHelp is like cobra.NoArgs but allows "help" as a single argument.
// When combined with checkHelpArg in RunE, it enables "cmd help" as an alias
// for "cmd --help" on leaf commands that take no positional arguments.
func noArgsOrHelp(cmd *cobra.Command, args []string) error {
	if len(args) == 1 && args[0] == "help" {
		return nil
	}
	return cobra.NoArgs(cmd, args)
}

// isTerminal reports whether r is connected to a terminal.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
