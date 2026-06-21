package cli

import (
	"sort"

	"cuelang.org/go/cue/ast"
	"github.com/start-cli/start/internal/modules"
)

// upsertAgent writes agent into its category file through the shared AST layer,
// creating the file with the managed header when absent and preserving sibling
// entries and comments. The same applies to upsertRole/upsertContext/upsertTask.
func upsertAgent(path string, a AgentConfig) error {
	return modules.UpsertConfigModule(path, "agents", a.Name, agentEntry(a))
}

func upsertRole(path string, r RoleConfig) error {
	return modules.UpsertConfigModule(path, "roles", r.Name, roleEntry(r))
}

func upsertContext(path string, c ContextConfig) error {
	return modules.UpsertConfigModule(path, "contexts", c.Name, contextEntry(c))
}

func upsertTask(path string, t TaskConfig) error {
	return modules.UpsertConfigModule(path, "tasks", t.Name, taskEntry(t))
}

func agentEntry(a AgentConfig) *ast.StructLit {
	fields := map[string]ast.Expr{}
	putString(fields, "description", a.Description)
	putStringList(fields, "tags", a.Tags)
	putStringList(fields, "uses", a.Uses)
	putString(fields, "bin", a.Bin)
	putString(fields, "command", a.Command)
	putString(fields, "default_model", a.DefaultModel)
	if len(a.Models) > 0 {
		fields["models"] = modelsExpr(a.Models)
	}
	return assembleEntry("agents", a.Origin, fields)
}

func roleEntry(r RoleConfig) *ast.StructLit {
	fields := map[string]ast.Expr{}
	putString(fields, "description", r.Description)
	putStringList(fields, "tags", r.Tags)
	putStringList(fields, "uses", r.Uses)
	putString(fields, "file", r.File)
	putString(fields, "command", r.Command)
	putPrompt(fields, r.Prompt)
	putBool(fields, "optional", r.Optional)
	return assembleEntry("roles", r.Origin, fields)
}

func contextEntry(c ContextConfig) *ast.StructLit {
	fields := map[string]ast.Expr{}
	putString(fields, "description", c.Description)
	putStringList(fields, "tags", c.Tags)
	putStringList(fields, "uses", c.Uses)
	putString(fields, "file", c.File)
	putString(fields, "command", c.Command)
	putPrompt(fields, c.Prompt)
	putBool(fields, "required", c.Required)
	putBool(fields, "default", c.Default)
	return assembleEntry("contexts", c.Origin, fields)
}

func taskEntry(t TaskConfig) *ast.StructLit {
	fields := map[string]ast.Expr{}
	putString(fields, "description", t.Description)
	putStringList(fields, "tags", t.Tags)
	putStringList(fields, "uses", t.Uses)
	putString(fields, "role", t.Role)
	putString(fields, "file", t.File)
	putString(fields, "command", t.Command)
	putPrompt(fields, t.Prompt)
	return assembleEntry("tasks", t.Origin, fields)
}

// assembleEntry builds an entry struct, emitting origin first (when present) then
// each populated field in the shared per-category order, so output stays
// byte-identical to install for scalar and list fields. fields holds only the
// non-empty values; the caller's putX helpers omit zero values.
func assembleEntry(category, origin string, fields map[string]ast.Expr) *ast.StructLit {
	s := &ast.StructLit{}
	if origin != "" {
		s.Elts = append(s.Elts, &ast.Field{
			Label: ast.NewIdent("origin"),
			Value: ast.NewString(origin),
		})
	}
	for _, name := range modules.CategoryFieldOrder(category) {
		if expr, ok := fields[name]; ok {
			s.Elts = append(s.Elts, &ast.Field{
				Label: ast.NewIdent(name),
				Value: expr,
			})
		}
	}
	return s
}

func putString(m map[string]ast.Expr, name, val string) {
	if val != "" {
		m[name] = ast.NewString(val)
	}
}

func putStringList(m map[string]ast.Expr, name string, items []string) {
	if len(items) == 0 {
		return
	}
	exprs := make([]ast.Expr, len(items))
	for i, item := range items {
		exprs[i] = ast.NewString(item)
	}
	m[name] = ast.NewList(exprs...)
}

func putBool(m map[string]ast.Expr, name string, val bool) {
	if val {
		m[name] = ast.NewBool(true)
	}
}

func putPrompt(m map[string]ast.Expr, prompt string) {
	if prompt != "" {
		m["prompt"] = modules.PromptExpr(prompt)
	}
}

// modelsExpr renders an agent's models map. Aliases are emitted in sorted order
// for determinism, since the source is an unordered Go map; this is the one field
// where add/edit output cannot match install's source-order emission.
func modelsExpr(models map[string]string) ast.Expr {
	aliases := make([]string, 0, len(models))
	for alias := range models {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	inner := &ast.StructLit{}
	for _, alias := range aliases {
		inner.Elts = append(inner.Elts, &ast.Field{
			Label: ast.NewStringLabel(alias),
			Value: ast.NewString(models[alias]),
		})
	}
	return inner
}
