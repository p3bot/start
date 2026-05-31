package cue

// CUE configuration keys, centralised to prevent typos.
const (
	KeyAgents   = "agents"
	KeyRoles    = "roles"
	KeyContexts = "contexts"
	KeyTasks    = "tasks"
	KeySettings = "settings"
)

// ConfigFiles maps each category key to its CUE config filename.
var ConfigFiles = map[string]string{
	KeyAgents:   "agents.cue",
	KeyRoles:    "roles.cue",
	KeyContexts: "contexts.cue",
	KeyTasks:    "tasks.cue",
	KeySettings: "settings.cue",
}
