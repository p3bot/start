package orchestration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"text/template"
	"time"

	"cuelang.org/go/cue"
	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/fault"
)

// quotedPlaceholderPattern detects placeholders that are incorrectly wrapped in quotes.
// escapeForShell wraps non-empty values in single quotes, so templates should NOT
// include quotes around any placeholder.
var quotedPlaceholderPattern = regexp.MustCompile(`['"]{{\.(?:bin|model|role|role_file|prompt|datetime|permission|effort|output|resume|print)}}['"]`)

// singleBracePlaceholderPattern detects placeholders using {name} syntax instead of {{.name}}.
// This is a common mistake when users expect simple substitution syntax.
var singleBracePlaceholderPattern = regexp.MustCompile(`\{(bin|model|role|role_file|prompt|datetime|permission|effort|output|resume|print)\}`)

// rawFragmentPlaceholders are inserted as pre-assembled argv, not as one shell-quoted word.
var rawFragmentPlaceholders = map[string]bool{
	"permission": true,
	"effort":     true,
	"output":     true,
	"resume":     true,
	"print":      true,
}

// Agent represents an agent configuration.
type Agent struct {
	Name         string
	Agentdex     string // catalog id; empty means CUE-only bin and models
	Bin          string
	Command      string
	DefaultModel string
	Models       map[string]string
	Description  string
	Flags        *FlagTable
}

// ExecuteConfig holds the configuration for agent execution.
type ExecuteConfig struct {
	Agent      Agent
	Model      string
	Role       string
	RoleFile   string
	Prompt     string
	PromptFile string
	WorkingDir string
	DryRun     bool
	Launch     LaunchFlags
}

// CommandData holds data for command template substitution.
// Keys are lowercase to match CUE field naming conventions.
type CommandData map[string]string

// processReplacer replaces the current process image with the given command,
// matching syscall.Exec semantics: on success it never returns, so callers
// cannot resume after a successful replacement.
type processReplacer func(argv0 string, argv []string, envv []string) error

// defaultProcessReplacer performs the real process replacement via syscall.Exec.
// Production builds compile only this default, so interactive launches replace
// the running process with the agent exactly as before. Test builds swap it
// through SetDefaultProcessReplacer (from a package's TestMain) so that no test
// ever replaces its own process and masks the suite's failure tally.
var defaultProcessReplacer processReplacer = func(argv0 string, argv []string, envv []string) error {
	// Unix-only: syscall.Exec replaces the current process with the agent.
	// This is intentional - no wrapper overhead, clean process model.
	return syscall.Exec(argv0, argv, envv)
}

// SetDefaultProcessReplacer swaps the package-level process replacer that
// NewExecutor injects into every Executor, returning the previous value. It
// exists so a sibling package's TestMain — which never runs this package's
// TestMain, and so otherwise links the production default — can install a
// guard that fails loudly instead of replacing the test process. It must not
// be called from production code.
func SetDefaultProcessReplacer(r processReplacer) processReplacer {
	prev := defaultProcessReplacer
	defaultProcessReplacer = r
	return prev
}

// Executor handles agent command execution.
type Executor struct {
	workingDir string
	// replaceProcess is the injected process-replacement seam. It is captured
	// at construction from defaultProcessReplacer so production keeps the real
	// syscall.Exec behaviour while tests run with a non-replacing guard.
	replaceProcess processReplacer
}

// NewExecutor creates a new agent executor.
func NewExecutor(workingDir string) *Executor {
	return &Executor{workingDir: workingDir, replaceProcess: defaultProcessReplacer}
}

// ValidateCommandTemplate checks for common template errors.
// Returns an error if the template contains quoted placeholders like '{{.prompt}}'
// since escapeForShell already wraps non-empty values in single quotes.
// Also detects {placeholder} syntax which should be {{.placeholder}}.
func ValidateCommandTemplate(tmpl string) error {
	if match := singleBracePlaceholderPattern.FindStringSubmatch(tmpl); match != nil {
		placeholder := match[1]
		return fmt.Errorf(`template uses {%s} but Go templates require {{.%s}}

Update your command template:

  Before: %s
  After:  %s`, placeholder, placeholder, tmpl, singleBracePlaceholderPattern.ReplaceAllString(tmpl, "{{.$1}}"))
	}

	if match := quotedPlaceholderPattern.FindString(tmpl); match != "" {
		placeholder := strings.TrimPrefix(match, "'{{.")
		placeholder = strings.TrimPrefix(placeholder, "\"{{.")
		placeholder = strings.TrimSuffix(placeholder, "}}'")
		placeholder = strings.TrimSuffix(placeholder, "}}\"")

		if rawFragmentPlaceholders[placeholder] {
			return fmt.Errorf(`template contains quoted placeholder %s

%s is inserted raw, including its own spaces.
Remove the surrounding quotes from your command template:

  Before: '%s'
  After:  %s`, match, "{{."+placeholder+"}}", "{{."+placeholder+"}}", "{{."+placeholder+"}}")
		}

		return fmt.Errorf(`template contains quoted placeholder %s

Placeholders are automatically shell-escaped and quoted.
Remove the surrounding quotes from your command template:

  Before: --%s '%s'
  After:  --%s %s`, match, placeholder, "{{."+placeholder+"}}", placeholder, "{{."+placeholder+"}}")
	}
	return nil
}

// BuildCommand builds the agent command from template and config.
func (e *Executor) BuildCommand(cfg ExecuteConfig) (string, error) {
	// Translate before LookPath or process start so a bad flag is usage,
	// not a missing binary and not a launched command.
	frags, err := cfg.Agent.CommandFragments(cfg.Launch, cfg.Prompt, FragmentLaunch)
	if err != nil {
		return "", err
	}

	if err := ValidateCommandTemplate(cfg.Agent.Command); err != nil {
		return "", err
	}

	model := cfg.Model
	if model == "" {
		model = cfg.Agent.DefaultModel
	}
	if model != "" {
		if resolved, ok := cfg.Agent.Models[model]; ok {
			model = resolved
		}
	}

	// Expand ~ in path fields before shell-escaping, since single-quoted strings
	// in shell don't expand tilde. Fail early with a clear message if home dir
	// is unavailable rather than passing an unexpanded path to the agent.
	bin, err := ExpandTilde(cfg.Agent.Bin)
	if err != nil {
		return "", fmt.Errorf("expanding bin path %q: %w", cfg.Agent.Bin, err)
	}

	// Validate the binary directly on the raw (pre-escape) path.
	// This avoids strings.Fields misparsing shell-quoted tokens when the
	// bin path contains spaces (e.g. "/my tools/claude").
	if bin == "" {
		return "", emptyBinError(cfg.Agent)
	}
	if _, err := exec.LookPath(bin); err != nil {
		// Binary absent from PATH is an environment the user must fix (78),
		// not a missing config resource (3) — note the deliberately different
		// domain from the "agent not found" config-absent case in ExtractAgent.
		return "", missingBinError(cfg.Agent, err)
	}

	roleFile, err := ExpandTilde(cfg.RoleFile)
	if err != nil {
		return "", fmt.Errorf("expanding role file path %q: %w", cfg.RoleFile, err)
	}

	data := CommandData{
		"bin":        escapeForShell(bin),
		"model":      escapeForShell(model),
		"role":       escapeForShell(cfg.Role),
		"role_file":  escapeForShell(roleFile),
		"prompt":     escapeForShell(cfg.Prompt),
		"datetime":   escapeForShell(time.Now().Format(time.RFC3339)),
		"permission": frags.Permission,
		"effort":     frags.Effort,
		"output":     frags.Output,
		"resume":     frags.Resume,
		"print":      frags.Print,
	}

	cmdStr, err := executeCommandTemplate(cfg.Agent.Command, data)
	if err != nil {
		return "", err
	}

	if err := validateCommandExecutable(cmdStr, cfg.Agent.Command); err != nil {
		return "", err
	}

	return cmdStr, nil
}

func executeCommandTemplate(tmpl string, data CommandData) (string, error) {
	t, err := template.New("command").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing command template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing command template: %w", err)
	}
	return buf.String(), nil
}

// FillAgentCommandForDisplay evaluates an agent command as a Go template with
// the same emptiness rules as launch, without shell-quoting. Runtime keys stay
// as the literal placeholders so get/describe still show them. Flag fragments
// use the same table as launch; {{.prompt}} stays visible and {{.resume}} is
// filled only inside an id fragment.
func FillAgentCommandForDisplay(command, bin, model string, agent Agent, launch LaunchFlags) (string, error) {
	frags, err := agent.CommandFragments(launch, "", FragmentDisplay)
	if err != nil {
		return "", err
	}
	return executeCommandTemplate(command, CommandData{
		"bin":        bin,
		"model":      model,
		"role":       "{{.role}}",
		"role_file":  "{{.role_file}}",
		"prompt":     "{{.prompt}}",
		"datetime":   "{{.datetime}}",
		"permission": frags.Permission,
		"effort":     frags.Effort,
		"output":     frags.Output,
		"resume":     frags.Resume,
		"print":      frags.Print,
	})
}

// validateCommandExecutable checks that the first token of the built command
// is a valid executable (either in PATH or a direct path).
// Skips leading environment variable assignments (VAR=value patterns).
func validateCommandExecutable(cmdStr, template string) error {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return fmt.Errorf(`command template produced empty command

  Template: %s

Check your agent's 'command' field`, template)
	}

	// Valid shell syntax: VAR1=x VAR2=y command args...
	cmdIndex := 0
	for cmdIndex < len(fields) && isEnvVarAssignment(fields[cmdIndex]) {
		cmdIndex++
	}

	if cmdIndex >= len(fields) {
		return fmt.Errorf(`command template produced only environment variables, no command

  Template: %s

Check your agent's 'command' field - it must include an executable`, template)
	}

	return nil
}

// isEnvVarAssignment checks if a token looks like an environment variable assignment.
// Valid patterns: VAR=value, VAR='value', VAR="value", VAR=
// The variable name must be a valid shell identifier.
func isEnvVarAssignment(token string) bool {
	stripped := strings.Trim(token, "'\"")

	eqIdx := strings.Index(stripped, "=")
	if eqIdx <= 0 {
		return false
	}

	varName := stripped[:eqIdx]
	return isValidEnvVarName(varName)
}

// isValidEnvVarName checks if a string is a valid shell environment variable name.
// Must start with letter or underscore, contain only alphanumeric and underscore.
func isValidEnvVarName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if i == 0 {
			if !isLetter && c != '_' {
				return false
			}
		} else {
			if !isLetter && (c < '0' || c > '9') && c != '_' {
				return false
			}
		}
	}
	return true
}

// Execute builds and runs the agent command, replacing the current process.
func (e *Executor) Execute(cfg ExecuteConfig) error {
	cmdStr, err := e.BuildCommand(cfg)
	if err != nil {
		return err
	}
	return e.ExecuteCommand(cmdStr, cfg)
}

// ExecuteCommand runs a pre-built command string, replacing the current process.
// Use this when the command has already been built and validated.
func (e *Executor) ExecuteCommand(cmdStr string, cfg ExecuteConfig) error {
	shell, err := exec.LookPath("bash")
	if err != nil {
		shell, err = exec.LookPath("sh")
		if err != nil {
			return fmt.Errorf("no shell available")
		}
	}

	// os.Chdir mutates process-global state with no rollback if Exec fails, so this
	// must only run as the final action before syscall.Exec replaces the process.
	if cfg.WorkingDir != "" {
		if err := os.Chdir(cfg.WorkingDir); err != nil {
			return fmt.Errorf("changing directory: %w", err)
		}
	}

	// Replacement goes through the injected seam so production replaces the
	// process via syscall.Exec while test builds substitute a guard. The
	// argument shape mirrors syscall.Exec: shell, [shell, -c, cmd], env.
	args := []string{shell, "-c", cmdStr}
	env := os.Environ()

	return e.replaceProcess(shell, args, env)
}

// ExecuteWithoutReplace runs the agent command without process replacement.
// Useful for testing or when process replacement is not desired.
func (e *Executor) ExecuteWithoutReplace(cfg ExecuteConfig) (string, error) {
	cmdStr, err := e.BuildCommand(cfg)
	if err != nil {
		return "", err
	}

	shell, err := exec.LookPath("bash")
	if err != nil {
		shell, err = exec.LookPath("sh")
		if err != nil {
			return "", fmt.Errorf("no shell available")
		}
	}

	cmd := exec.Command(shell, "-c", cmdStr)
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("command failed: %w\nstderr: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// escapeForShell prepares a string for safe use in shell commands.
// It wraps the value in single quotes and escapes internal single quotes,
// preventing shell command injection. The returned value is already quoted -
// templates should use {{.prompt}} directly, NOT '{{.prompt}}'.
// Empty stays empty so {{if .placeholder}} is false; quoting '' is truthy.
//
// Example: hello 'world' becomes 'hello '"'"'world'"'"'
//
// Note: Environment variables (e.g., $HOME) are NOT expanded. Use literal values
// in prompts or the command field for dynamic content.
func escapeForShell(s string) string {
	if s == "" {
		return ""
	}
	// ' -> '"'"': close the single quote, add a double-quoted literal quote, reopen.
	escaped := strings.ReplaceAll(s, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

func emptyBinError(agent Agent) error {
	if agent.Agentdex != "" {
		return fault.UserConfig(fmt.Errorf(`agent binary is empty (agentdex %q)

Install the catalogued CLI or remove the join key; joined recipes have no bin field`, agent.Agentdex))
	}
	return fault.UserConfig(fmt.Errorf(`agent 'bin' field is empty

Check your agent's 'bin' field`))
}

func missingBinError(agent Agent, err error) error {
	if agent.Agentdex != "" {
		name := agent.Bin
		if name == "" {
			name = agent.Agentdex
		}
		return fault.UserConfig(fmt.Errorf(`binary %q not found

  Error: %s

Install %s or remove the agentdex join key %q; joined recipes have no bin field`, agent.Bin, err, name, agent.Agentdex))
	}
	return fault.UserConfig(fmt.Errorf(`binary %q not found

  Error: %s

Check your agent's 'bin' field or ensure the executable is in PATH`, agent.Bin, err))
}

// ExtractAgent extracts agent configuration from CUE value.
func ExtractAgent(cfg cue.Value, name string) (Agent, error) {
	agentVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyAgents)).LookupPath(cue.MakePath(cue.Str(name)))
	if !agentVal.Exists() {
		// Config-absent: the named agent is a missing resource (3), distinct
		// from a configured agent whose binary is absent from PATH (78 above).
		return Agent{}, fault.NotFound(fmt.Errorf("agent %q not found", name))
	}

	return extractAgentFields(agentVal, name), nil
}

// AgentFromValue extracts agent fields from a resolved CUE value (config entry
// or module body) without looking up the agents map.
func AgentFromValue(agentVal cue.Value, name string) Agent {
	return extractAgentFields(agentVal, name)
}

// extractAgentFields extracts agent fields from a resolved CUE value.
func extractAgentFields(agentVal cue.Value, name string) Agent {
	var agent Agent
	agent.Name = name

	if ad := agentVal.LookupPath(cue.ParsePath("agentdex")); ad.Exists() {
		agent.Agentdex, _ = ad.String()
	}
	if bin := agentVal.LookupPath(cue.ParsePath("bin")); bin.Exists() {
		agent.Bin, _ = bin.String()
	}
	if cmd := agentVal.LookupPath(cue.ParsePath("command")); cmd.Exists() {
		agent.Command, _ = cmd.String()
	}
	if dm := agentVal.LookupPath(cue.ParsePath("default_model")); dm.Exists() {
		agent.DefaultModel, _ = dm.String()
	}
	if desc := agentVal.LookupPath(cue.ParsePath("description")); desc.Exists() {
		agent.Description, _ = desc.String()
	}

	agent.Models = internalcue.AgentModels(agentVal)
	agent.Flags = decodeAgentFlagTable(agentVal)

	return agent
}

// GenerateDryRunCommand generates the command.txt content for dry-run.
func GenerateDryRunCommand(agent Agent, model, roleName string, contexts []string, workingDir string, cmdStr string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Agent: %s\n", agent.Name)
	fmt.Fprintf(&sb, "# Model: %s\n", model)
	fmt.Fprintf(&sb, "# Role: %s\n", roleName)
	fmt.Fprintf(&sb, "# Contexts: %s\n", strings.Join(contexts, ", "))
	fmt.Fprintf(&sb, "# Working Directory: %s\n", workingDir)
	fmt.Fprintf(&sb, "# Generated: %s\n", time.Now().Format(time.RFC3339))
	sb.WriteString("\n")
	sb.WriteString(cmdStr)

	return sb.String()
}
