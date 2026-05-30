package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/registry"
	"golang.org/x/term"
)

// IsSilentError returns true if the error should not be printed to stderr.
// Used by main.go to suppress output for errors that only set the exit code.
//
// The chain is walked with errors.As, mirroring ExitCodeFromError: main.go
// classifies the same returned error with both helpers, so silence detection
// must stay consistent with exit-code derivation even when a silenced error is
// wrapped further up the chain.
func IsSilentError(err error) bool {
	type silent interface {
		Silent() bool
	}
	var s silent
	if errors.As(err, &s) {
		return s.Silent()
	}
	return false
}

// silenced wraps err so main.go suppresses its own "Error:" stderr line — the
// command has already reported the condition in its own words — while the
// exit-code mapper still derives the process code from the wrapped chain.
// Returns nil for nil.
func silenced(err error) error {
	if err == nil {
		return nil
	}
	return silentErr{err: err}
}

type silentErr struct{ err error }

func (e silentErr) Error() string { return e.err.Error() }
func (e silentErr) Unwrap() error { return e.err }
func (e silentErr) Silent() bool  { return true }

// Build-time variables set via ldflags
var (
	cliVersion = "dev"
	commit     = "unknown"
	buildDate  = "unknown"
	repoURL    = "https://github.com/start-cli/start"
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

When stdin is piped, its content is used as the prompt text — equivalent
to passing it inline. For bare 'start', only required contexts are
included; for 'start prompt' it fills the missing [text] arg; for
'start task <name>' it fills the missing [instructions] arg. Persistent
flags (--agent, --role, --context, ...) are honoured as normal.

Examples:
  start                              Launch agent with default role and contexts
  start --role go-expert             Launch with a specific role
  echo "summarise this" | start      Send piped text as a one-shot prompt
  echo "..." | start task review     Pipe instructions to a task
  start task review/pre-commit       Run a predefined task
  start doctor                       Check installation and configuration`,
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

			// Resolve colour to a single settled state from --color plus the
			// environment, then drive fatih/color's global from it. An invalid
			// value is a usage error (exit 2).
			decorated, err := resolveColorMode(flags.Color, isTerminalWriter(cmd.OutOrStdout()))
			if err != nil {
				return err
			}
			color.NoColor = !decorated

			// Debug implies verbose
			if flags.Debug {
				flags.Verbose = true
			}
			return nil
		},
	}

	// Bind the production registry-client provider into the root context.
	// Cobra copies the root context onto the resolved subcommand before
	// running it, so every subcommand observes this provider; tests override
	// the bound provider before Execute to run offline.
	cmd.SetContext(WithProvider(context.Background(), registry.NewClient))

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
	cmd.PersistentFlags().StringVar(&flags.Color, "color", "auto", "Colour output: auto, always, never")
	cmd.PersistentFlags().BoolVarP(&flags.Local, "local", "l", false, "Target local config (./.start/) instead of global")
	cmd.PersistentFlags().BoolVar(&flags.NoRole, "no-role", false, "Skip role assignment")
	cmd.MarkFlagsMutuallyExclusive("role", "no-role")

	// Set RunE on root command for `start` execution
	cmd.RunE = runStart

	// Define command groups for help output
	cmd.AddGroup(
		&cobra.Group{ID: "modules", Title: "Modules:"},
		&cobra.Group{ID: "workflow", Title: "Workflow:"},
		&cobra.Group{ID: "utilities", Title: "Utilities:"},
	)

	// Add subcommands
	addDescribeCommand(cmd, flags)
	addGetCommand(cmd, flags)
	addPromptCommand(cmd)
	addTaskCommand(cmd)
	addInstallCommand(cmd)
	addListCommand(cmd)
	addUpdateCommand(cmd)
	addLibraryCommand(cmd)
	addConfigCommand(cmd, flags)
	addSearchCommand(cmd)
	addDoctorCommand(cmd)
	addCompletionCommand(cmd)

	// Replace default help command with one that includes agent-focused topic subcommands
	addHelpCommand(cmd)

	// Classify Cobra's own flag-parse and arg-count failures as usage errors
	// (exit 2). FlagErrorFunc is inherited by every subcommand; wrapUsageArgs
	// wraps each command's Args validator. Cobra's unknown-command error is
	// produced earlier, during Find, and is intentionally left at exit 1 to
	// match git/gh.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError(err)
	})
	wrapUsageArgs(cmd)

	return cmd
}

// wrapUsageArgs wraps every command's positional-argument validator so an
// arg-count failure carries the usage fault domain (exit 2) without changing
// its message. A nil Args validator (Cobra's ArbitraryArgs default) never
// errors, so it is left untouched.
func wrapUsageArgs(cmd *cobra.Command) {
	if cmd.Args != nil {
		inner := cmd.Args
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := inner(c, args); err != nil {
				return usageError(err)
			}
			return nil
		}
	}
	for _, sub := range cmd.Commands() {
		wrapUsageArgs(sub)
	}
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

// resolveColorMode collapses the --color value and the environment into a
// single decoration decision. Precedence (Requirement 2): NO_COLOR (set to any
// value) disables colour and wins even over --color=always; --color=never
// disables; --color=always forces on; --color=auto enables only when stdout is
// a TTY, with TERM=dumb disabling and FORCE_COLOR/CLICOLOR_FORCE forcing on
// when stdout is not a TTY. An out-of-set value is a usage error (exit 2).
func resolveColorMode(mode string, stdoutTTY bool) (decorated bool, err error) {
	switch mode {
	case "auto", "always", "never":
	default:
		return false, usageError(fmt.Errorf("invalid --color %q: must be one of auto, always, never", mode))
	}

	// NO_COLOR is absolute: it disables colour ahead of --color=always.
	if os.Getenv("NO_COLOR") != "" {
		return false, nil
	}

	switch mode {
	case "never":
		return false, nil
	case "always":
		return true, nil
	default: // auto
		if os.Getenv("TERM") == "dumb" {
			return false, nil
		}
		force := envTruthy("FORCE_COLOR") || envTruthy("CLICOLOR_FORCE")
		return stdoutTTY || force, nil
	}
}

// envTruthy reports whether a cross-ecosystem boolean env var is on. Any
// non-empty, non-falsy value is truthy, matching the de facto FORCE_COLOR /
// CLICOLOR_FORCE convention so the same value behaves identically across tools.
func envTruthy(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v != "" && v != "0" && v != "false" && v != "no"
}

// isTerminal reports whether r is connected to a terminal.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// isTerminalWriter reports whether w is connected to a terminal. Used for the
// stdout TTY check that drives --color=auto; in tests the writer is a buffer,
// so auto resolves to no colour unless FORCE_COLOR is set.
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// readPipedStdin returns the full contents of stdin when it is piped
// (not a TTY). Content is returned raw to preserve leading whitespace
// and trailing newlines, matching file-sourced prompts via
// orchestration.ReadFilePath. When stdin is a TTY, returns
// ("", false, nil) so callers can fall back to their interactive path.
//
// Callers decide their own empty-stdin policy. runStart treats a blank
// pipe as "no prompt given" and runs the normal start flow with default
// contexts, so `start </dev/null` behaves like bare `start`; runPrompt and
// runTask accept an empty pipe as a valid no-text invocation.
func readPipedStdin(stdin io.Reader) (text string, piped bool, err error) {
	if isTerminal(stdin) {
		return "", false, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", true, fmt.Errorf("reading stdin: %w", err)
	}
	return string(data), true, nil
}
