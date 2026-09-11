package cli

import (
	"context"
	"strings"

	"cuelang.org/go/cue"
	"github.com/p3bot/agentdex"
	internalcue "github.com/p3bot/start/internal/cue"
	"github.com/p3bot/start/internal/modules"
	"github.com/p3bot/start/internal/orchestration"
)

// prepareLaunchAgent rewrites --agent / default_agent so the liveness union
// and resolveAgent interpret the same identifier. resolveAgent still rewrites
// itself; a second pass is a no-op.
func (r *resolver) prepareLaunchAgent(name string) string {
	if name == "" {
		return name
	}
	return rewriteLaunchAgent(name, r.cfg.Value, r.workingDir, r.catalogOpts...)
}

// rewriteLaunchAgent applies launch-only identifier rewrites: leftover-name
// aliases, then prefix-qualify a slash-less catalog id as agents:<id>. Applied
// only to --agent and settings.default_agent. An exact installed match of the
// original name is left unchanged so a leftover claude/interactive still
// launches from CUE. Unknown or unresolvable catalog ids stay bare so substring
// fallback still runs.
func rewriteLaunchAgent(name string, cfg cue.Value, workingDir string, opts ...agentdex.Option) string {
	if name == "" || orchestration.IsLocator(name) {
		return name
	}
	if exactInstalledAgent(cfg, launchAgentIdent(name)) {
		return name
	}
	name = applyLaunchAgentAlias(name)
	if strings.Contains(name, ":") || strings.Contains(name, "/") {
		return name
	}
	if orchestration.KnownCatalogID(context.Background(), name, workingDir, opts...) {
		return "agents:" + name
	}
	return name
}

func launchAgentIdent(name string) string {
	if cat, rest, ok := strings.Cut(name, ":"); ok && cat == "agents" {
		return rest
	}
	return name
}

// applyLaunchAgentAlias rewrites the finite leftover-name table. claude →
// claude-code and claude/<variant> → claude-code/<variant>. Keep this table
// small enough to delete once leftover installs are gone.
func applyLaunchAgentAlias(name string) string {
	prefix := ""
	ident := name
	if cat, rest, ok := strings.Cut(name, ":"); ok && cat == "agents" {
		prefix = "agents:"
		ident = rest
	}
	lower := strings.ToLower(ident)
	switch {
	case lower == "claude":
		return prefix + "claude-code"
	case strings.HasPrefix(lower, "claude/"):
		return prefix + "claude-code/" + strings.ToLower(ident[len("claude/"):])
	}
	return name
}

func exactInstalledAgent(cfg cue.Value, ident string) bool {
	if ident == "" {
		return false
	}
	agents := cfg.LookupPath(cue.ParsePath(internalcue.KeyAgents))
	if !agents.Exists() {
		return false
	}
	iter, err := agents.Fields()
	if err != nil {
		return false
	}
	for iter.Next() {
		if modules.NameMatches(ident, iter.Selector().Unquoted(), modules.ModeExact) {
			return true
		}
	}
	return false
}
