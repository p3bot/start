package cue

// CUE configuration keys, centralised to prevent typos.
const (
	KeyAgents   = "agents"
	KeyRoles    = "roles"
	KeyContexts = "contexts"
	KeyTasks    = "tasks"
	KeySkills   = "skills"
	KeySettings = "settings"

	// Agent sub-field keys for the `models` alias map.
	KeyModels = "models"
	KeyID     = "id"

	// KeyPriority ranks a role for automatic selection; lower is tried first.
	KeyPriority = "priority"

	// KeyMinMembers is the smallest roster a task needs. start stores it and
	// does not read it at launch.
	KeyMinMembers = "min_members"

	// KeySessionLocator is an agent's list of strings locating a session.
	// start stores it and does not read it at launch.
	KeySessionLocator = "session_locator"
)

// ConfigFiles maps each category key to its CUE config filename.
var ConfigFiles = map[string]string{
	KeyAgents:   "agents.cue",
	KeyRoles:    "roles.cue",
	KeyContexts: "contexts.cue",
	KeyTasks:    "tasks.cue",
	KeySkills:   "skills.cue",
	KeySettings: "settings.cue",
}
