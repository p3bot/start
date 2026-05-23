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
	internalcue "github.com/start-cli/start/internal/cue"
)

// quotedPlaceholderPattern detects placeholders that are incorrectly wrapped in quotes.
// Since escapeForShell wraps all placeholder values in single quotes, templates should NOT
// include quotes around any placeholder.
var quotedPlaceholderPattern = regexp.MustCompile(`['"]{{\.(?:bin|model|role|role_file|prompt|datetime)}}['"]`)

// singleBracePlaceholderPattern detects placeholders using {name} syntax instead of {{.name}}.
// This is a common mistake when users expect simple substitution syntax.
var singleBracePlaceholderPattern = regexp.MustCompile(`\{(bin|model|role|role_file|prompt|datetime)\}`)

// Agent represents an agent configuration.
type Agent struct {
	Name         string
	Bin          string
	Command      string
	DefaultModel string
	Models       map[string]string
	Description  string
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
}

// CommandData holds data for command template substitution.
// Uses lowercase keys to match CUE field naming conventions.
type CommandData map[string]string

// Executor handles agent command execution.
type Executor struct {
	workingDir string
}

// NewExecutor creates a new agent executor.
func NewExecutor(workingDir string) *Executor {
	return &Executor{workingDir: workingDir}
}

// ValidateCommandTemplate checks for common template errors.
// Returns an error if the template contains quoted placeholders like '{{.prompt}}'
// since escapeForShell already wraps values in single quotes.
// Also detects {placeholder} syntax which should be {{.placeholder}}.
func ValidateCommandTemplate(tmpl string) error {
	// Check for single-brace placeholders like {prompt} instead of {{.prompt}}
	if match := singleBracePlaceholderPattern.FindStringSubmatch(tmpl); match != nil {
		placeholder := match[1]
		return fmt.Errorf(`template uses {%s} but Go templates require {{.%s}}

Update your command template:

  Before: %s
  After:  %s`, placeholder, placeholder, tmpl, singleBracePlaceholderPattern.ReplaceAllString(tmpl, "{{.$1}}"))
	}

	// Check for quoted placeholders
	if match := quotedPlaceholderPattern.FindString(tmpl); match != "" {
		// Extract the placeholder name from the match
		placeholder := strings.TrimPrefix(match, "'{{.")
		placeholder = strings.TrimPrefix(placeholder, "\"{{.")
		placeholder = strings.TrimSuffix(placeholder, "}}'")
		placeholder = strings.TrimSuffix(placeholder, "}}\"")

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
	// Validate template for common errors
	if err := ValidateCommandTemplate(cfg.Agent.Command); err != nil {
		return "", err
	}

	// Resolve model name to actual model string
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
		return "", fmt.Errorf(`agent 'bin' field is empty

Check your agent's 'bin' field`)
	}
	if _, err := exec.LookPath(bin); err != nil {
		return "", fmt.Errorf(`binary %q not found

  Error: %s

Check your agent's 'bin' field or ensure the executable is in PATH`, cfg.Agent.Bin, err)
	}

	roleFile, err := ExpandTilde(cfg.RoleFile)
	if err != nil {
		return "", fmt.Errorf("expanding role file path %q: %w", cfg.RoleFile, err)
	}

	// Build template data with lowercase keys to match CUE conventions.
	// All values are shell-escaped and wrapped in single quotes for safety.
	data := CommandData{
		"bin":       escapeForShell(bin),
		"model":     escapeForShell(model),
		"role":      escapeForShell(cfg.Role),
		"role_file": escapeForShell(roleFile),
		"prompt":    escapeForShell(cfg.Prompt),
		"datetime":  escapeForShell(time.Now().Format(time.RFC3339)),
	}

	// Parse and execute command template
	tmpl, err := template.New("command").Parse(cfg.Agent.Command)
	if err != nil {
		return "", fmt.Errorf("parsing command template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing command template: %w", err)
	}

	cmdStr := buf.String()

	// Validate that the command starts with an executable
	if err := validateCommandExecutable(cmdStr, cfg.Agent.Command); err != nil {
		return "", err
	}

	return cmdStr, nil
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

	// Skip leading environment variable assignments (VAR=value patterns).
	// These are valid shell syntax: VAR1=x VAR2=y command args...
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
	// Strip surrounding quotes (the token might be quoted)
	stripped := strings.Trim(token, "'\"")

	// Look for = sign
	eqIdx := strings.Index(stripped, "=")
	if eqIdx <= 0 {
		return false
	}

	// Get the variable name (part before =)
	varName := stripped[:eqIdx]

	// Validate it's a proper env var name
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
	// Find shell
	shell, err := exec.LookPath("bash")
	if err != nil {
		shell, err = exec.LookPath("sh")
		if err != nil {
			return fmt.Errorf("no shell available")
		}
	}

	// Set working directory.
	// Note: os.Chdir mutates process-global state. This is safe here because
	// syscall.Exec below replaces the process. If Exec fails, the working
	// directory remains changed with no rollback. This function must only be
	// called as the final action before process replacement.
	if cfg.WorkingDir != "" {
		if err := os.Chdir(cfg.WorkingDir); err != nil {
			return fmt.Errorf("changing directory: %w", err)
		}
	}

	// Unix-only: syscall.Exec replaces the current process with the agent.
	// This is intentional - no wrapper overhead, clean process model.
	args := []string{shell, "-c", cmdStr}
	env := os.Environ()

	return syscall.Exec(shell, args, env)
}

// ExecuteWithoutReplace runs the agent command without process replacement.
// Useful for testing or when process replacement is not desired.
func (e *Executor) ExecuteWithoutReplace(cfg ExecuteConfig) (string, error) {
	cmdStr, err := e.BuildCommand(cfg)
	if err != nil {
		return "", err
	}

	// Find shell
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
//
// Example: "hello 'world'" becomes "'hello '\"'\"'world'\"'\"”"
//
// Note: Environment variables (e.g., $HOME) are NOT expanded. Use literal values
// in prompts or the command field for dynamic content.
func escapeForShell(s string) string {
	// Escape single quotes: replace ' with '"'"'
	// This ends the single-quoted string, adds a double-quoted literal quote,
	// then restarts the single-quoted string.
	escaped := strings.ReplaceAll(s, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// ExtractAgent extracts agent configuration from CUE value.
func ExtractAgent(cfg cue.Value, name string) (Agent, error) {
	agentVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyAgents)).LookupPath(cue.MakePath(cue.Str(name)))
	if !agentVal.Exists() {
		return Agent{}, fmt.Errorf("agent %q not found", name)
	}

	return extractAgentFields(agentVal, name), nil
}

// extractAgentFields extracts agent fields from a resolved CUE value.
func extractAgentFields(agentVal cue.Value, name string) Agent {
	var agent Agent
	agent.Name = name

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

	if models := agentVal.LookupPath(cue.ParsePath("models")); models.Exists() {
		agent.Models = make(map[string]string)
		iter, err := models.Fields()
		if err == nil {
			for iter.Next() {
				modelName := iter.Selector().Unquoted()
				modelVal := iter.Value()

				// Try direct string first (simple format: models: { sonnet: "model-id" })
				if s, err := modelVal.String(); err == nil {
					agent.Models[modelName] = s
					continue
				}

				// Try nested id field (object format: models: { sonnet: { id: "model-id" } })
				if idVal := modelVal.LookupPath(cue.ParsePath("id")); idVal.Exists() {
					if s, err := idVal.String(); err == nil {
						agent.Models[modelName] = s
					}
				}
			}
		}
	}

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
