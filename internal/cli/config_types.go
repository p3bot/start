package cli

import (
	"errors"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"github.com/p3bot/start/internal/config"
	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/modules"
)

// AgentConfig represents an agent configuration for editing.
type AgentConfig struct {
	Name           string            `json:"name"`
	Agentdex       string            `json:"agentdex,omitempty"`
	Bin            string            `json:"bin,omitempty"`
	Command        string            `json:"command,omitempty"`
	DefaultModel   string            `json:"defaultModel,omitempty"`
	Description    string            `json:"description,omitempty"`
	Models         map[string]string `json:"models,omitempty"`
	Tags           []string          `json:"tags,omitempty"`
	Uses           []string          `json:"uses,omitempty"` // Colon-form addresses of modules pulled in via `start get`
	Flags          ast.Expr          `json:"-"`              // flags table, written back unchanged by config edit
	SessionLocator []string          `json:"sessionLocator,omitempty"`
	Source         string            `json:"source"`           // "global" or "local"
	Origin         string            `json:"origin,omitempty"` // Registry module path when installed from registry

	unkept error // A stored managed field this struct cannot hold; writing the agent back would drop it
}

// writable reports whether writing a's managed fields back keeps their stored values.
func (a AgentConfig) writable() error { return keptForRewrite("agent", a.Name, a.unkept) }

// decodeAgentValue populates an AgentConfig from a per-item CUE value (Name and
// Source are left for the caller). A flags table is kept as an expression so a
// later edit writes the same table back. A table that cannot be rewritten, or a
// session_locator that is not a list of strings, is returned as an error
// alongside the rest of the agent.
func decodeAgentValue(val cue.Value) (AgentConfig, error) {
	var agent AgentConfig

	if v := val.LookupPath(cue.ParsePath("agentdex")); v.Exists() {
		agent.Agentdex, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("bin")); v.Exists() {
		agent.Bin, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("command")); v.Exists() {
		agent.Command, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("default_model")); v.Exists() {
		agent.DefaultModel, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("description")); v.Exists() {
		agent.Description, _ = v.String()
	}

	agent.Tags = extractStringList(val, "tags")
	agent.Uses = extractStringList(val, "uses")

	agent.Models = internalcue.AgentModels(val)

	if v := val.LookupPath(cue.ParsePath("origin")); v.Exists() {
		agent.Origin, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("flags")); v.Exists() {
		expr, err := modules.FormatFieldExpr(v)
		if err != nil {
			return agent, fmt.Errorf("flags: %w", err)
		}
		agent.Flags = expr
	}
	locator, err := internalcue.StringList(val, internalcue.KeySessionLocator)
	if err != nil {
		return agent, fmt.Errorf("%s: %w", internalcue.KeySessionLocator, err)
	}
	agent.SessionLocator = locator

	return agent, nil
}

func loadAgentsForScope(scope config.Scope) (map[string]AgentConfig, []string, error) {
	return loadForScope(scope, loadAgentsFromDir, func(a *AgentConfig, s string) { a.Source = s })
}

func loadAgentsFromDir(dir string) (map[string]AgentConfig, []string, error) {
	agents := make(map[string]AgentConfig)
	var order []string

	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(dir)
	if err != nil {
		if errors.Is(err, internalcue.ErrNoCUEFiles) {
			return agents, order, nil
		}
		return agents, order, err
	}

	agentsVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyAgents))
	if !agentsVal.Exists() {
		return agents, order, nil
	}

	iter, err := agentsVal.Fields()
	if err != nil {
		return nil, nil, fmt.Errorf("iterating agents: %w", err)
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		agent, err := decodeAgentValue(iter.Value())
		agent.unkept = err
		agent.Name = name
		agents[name] = agent
		order = append(order, name)
	}

	return agents, order, nil
}

func getDefaultAgentFromConfig(cfg cue.Value) string {
	val := cfg.LookupPath(cue.ParsePath(internalcue.KeySettings + ".default_agent"))
	if !val.Exists() {
		return ""
	}
	s, err := val.String()
	if err != nil {
		return ""
	}
	return s
}

// RoleConfig represents a role configuration for editing.
type RoleConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	File        string   `json:"file,omitempty"`
	Command     string   `json:"command,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Uses        []string `json:"uses,omitempty"`     // Colon-form addresses of modules pulled in via `start get`
	Priority    int      `json:"priority,omitempty"` // Automatic-selection rank, lowest first; 0 means never selected automatically
	Source      string   `json:"source"`             // "global" or "local"
	Origin      string   `json:"origin,omitempty"`   // Registry module path when installed from registry

	unkept error // A stored managed field this struct cannot hold; writing the role back would drop it
}

// writable reports whether writing r's managed fields back keeps their stored values.
func (r RoleConfig) writable() error { return keptForRewrite("role", r.Name, r.unkept) }

// decodeRoleValue populates a RoleConfig from a per-item CUE value (Name and
// Source are left for the caller). A priority that is not a whole number of 1
// or more is returned as an error alongside the rest of the role.
func decodeRoleValue(val cue.Value) (RoleConfig, error) {
	var role RoleConfig

	if v := val.LookupPath(cue.ParsePath("description")); v.Exists() {
		role.Description, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("file")); v.Exists() {
		role.File, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("command")); v.Exists() {
		role.Command, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("prompt")); v.Exists() {
		role.Prompt, _ = v.String()
	}

	role.Tags = extractStringList(val, "tags")
	role.Uses = extractStringList(val, "uses")

	if v := val.LookupPath(cue.ParsePath("origin")); v.Exists() {
		role.Origin, _ = v.String()
	}
	priority, err := internalcue.PositiveInt(val, internalcue.KeyPriority)
	if err != nil {
		return role, fmt.Errorf("%s: %w", internalcue.KeyPriority, err)
	}
	role.Priority = priority

	return role, nil
}

func loadRolesForScope(scope config.Scope) (map[string]RoleConfig, []string, error) {
	return loadForScope(scope, loadRolesFromDir, func(r *RoleConfig, s string) { r.Source = s })
}

func loadRolesFromDir(dir string) (map[string]RoleConfig, []string, error) {
	roles := make(map[string]RoleConfig)
	var order []string

	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(dir)
	if err != nil {
		if errors.Is(err, internalcue.ErrNoCUEFiles) {
			return roles, order, nil
		}
		return roles, order, err
	}

	rolesVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyRoles))
	if !rolesVal.Exists() {
		return roles, order, nil
	}

	iter, err := rolesVal.Fields()
	if err != nil {
		return nil, nil, fmt.Errorf("iterating roles: %w", err)
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		role, err := decodeRoleValue(iter.Value())
		role.unkept = err
		role.Name = name
		roles[name] = role
		order = append(order, name)
	}

	return roles, order, nil
}

// ContextConfig represents a context configuration for editing.
type ContextConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	File        string   `json:"file,omitempty"`
	Command     string   `json:"command,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Default     bool     `json:"default,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Uses        []string `json:"uses,omitempty"`   // Colon-form addresses of modules pulled in via `start get`
	Source      string   `json:"source"`           // "global" or "local"
	Origin      string   `json:"origin,omitempty"` // Registry module path when installed from registry
}

// decodeContextValue populates a ContextConfig from a per-item CUE value (Name
// and Source are left for the caller).
func decodeContextValue(val cue.Value) ContextConfig {
	var ctx ContextConfig

	if v := val.LookupPath(cue.ParsePath("description")); v.Exists() {
		ctx.Description, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("file")); v.Exists() {
		ctx.File, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("command")); v.Exists() {
		ctx.Command, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("prompt")); v.Exists() {
		ctx.Prompt, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("required")); v.Exists() {
		ctx.Required, _ = v.Bool()
	}
	if v := val.LookupPath(cue.ParsePath("default")); v.Exists() {
		ctx.Default, _ = v.Bool()
	}

	ctx.Tags = extractStringList(val, "tags")
	ctx.Uses = extractStringList(val, "uses")

	if v := val.LookupPath(cue.ParsePath("origin")); v.Exists() {
		ctx.Origin, _ = v.String()
	}

	return ctx
}

func loadContextsForScope(scope config.Scope) (map[string]ContextConfig, []string, error) {
	return loadForScope(scope, loadContextsFromDir, func(c *ContextConfig, s string) { c.Source = s })
}

func loadContextsFromDir(dir string) (map[string]ContextConfig, []string, error) {
	contexts := make(map[string]ContextConfig)
	var order []string

	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(dir)
	if err != nil {
		if errors.Is(err, internalcue.ErrNoCUEFiles) {
			return contexts, order, nil
		}
		return contexts, order, err
	}

	contextsVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyContexts))
	if !contextsVal.Exists() {
		return contexts, order, nil
	}

	iter, err := contextsVal.Fields()
	if err != nil {
		return nil, nil, fmt.Errorf("iterating contexts: %w", err)
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		ctx := decodeContextValue(iter.Value())
		ctx.Name = name
		contexts[name] = ctx
		order = append(order, name)
	}

	return contexts, order, nil
}

// TaskConfig represents a task configuration for editing.
type TaskConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	File        string   `json:"file,omitempty"`
	Command     string   `json:"command,omitempty"`
	Prompt      string   `json:"prompt,omitempty"`
	Role        string   `json:"role,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Uses        []string `json:"uses,omitempty"`       // Colon-form addresses of modules pulled in via `start get`
	MinMembers  int      `json:"minMembers,omitempty"` // Smallest roster the task needs; 0 when unset. Not read at launch
	Source      string   `json:"source"`               // "global" or "local"
	Origin      string   `json:"origin,omitempty"`     // Registry module path when installed from registry

	unkept error // A stored managed field this struct cannot hold; writing the task back would drop it
}

// writable reports whether writing t's managed fields back keeps their stored values.
func (t TaskConfig) writable() error { return keptForRewrite("task", t.Name, t.unkept) }

// decodeTaskValue populates a TaskConfig from a per-item CUE value (Name and
// Source are left for the caller). A min_members that is not a whole number of
// 1 or more is returned as an error alongside the rest of the task.
func decodeTaskValue(val cue.Value) (TaskConfig, error) {
	var task TaskConfig

	if v := val.LookupPath(cue.ParsePath("description")); v.Exists() {
		task.Description, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("file")); v.Exists() {
		task.File, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("command")); v.Exists() {
		task.Command, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("prompt")); v.Exists() {
		task.Prompt, _ = v.String()
	}
	if v := val.LookupPath(cue.ParsePath("role")); v.Exists() {
		task.Role, _ = v.String()
	}

	task.Tags = extractStringList(val, "tags")
	task.Uses = extractStringList(val, "uses")

	if v := val.LookupPath(cue.ParsePath("origin")); v.Exists() {
		task.Origin, _ = v.String()
	}
	minMembers, err := internalcue.PositiveInt(val, internalcue.KeyMinMembers)
	if err != nil {
		return task, fmt.Errorf("%s: %w", internalcue.KeyMinMembers, err)
	}
	task.MinMembers = minMembers

	return task, nil
}

func loadTasksForScope(scope config.Scope) (map[string]TaskConfig, []string, error) {
	return loadForScope(scope, loadTasksFromDir, func(t *TaskConfig, s string) { t.Source = s })
}

func loadTasksFromDir(dir string) (map[string]TaskConfig, []string, error) {
	tasks := make(map[string]TaskConfig)
	var order []string

	loader := internalcue.NewLoader()
	cfg, err := loader.LoadSingle(dir)
	if err != nil {
		if errors.Is(err, internalcue.ErrNoCUEFiles) {
			return tasks, order, nil
		}
		return tasks, order, err
	}

	tasksVal := cfg.LookupPath(cue.ParsePath(internalcue.KeyTasks))
	if !tasksVal.Exists() {
		return tasks, order, nil
	}

	iter, err := tasksVal.Fields()
	if err != nil {
		return nil, nil, fmt.Errorf("iterating tasks: %w", err)
	}

	for iter.Next() {
		name := iter.Selector().Unquoted()
		task, err := decodeTaskValue(iter.Value())
		task.unkept = err
		task.Name = name
		tasks[name] = task
		order = append(order, name)
	}

	return tasks, order, nil
}
