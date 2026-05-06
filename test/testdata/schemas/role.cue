package schemas

// #Role defines the schema for AI agent roles (system prompts).
// Roles define what the AI agent is and how it should behave.
//
// Note: Roles are identified by their map key (e.g., roles["code-reviewer"]).
// There is no 'name' field - the key IS the name.
//
// Roles use the UTD pattern to build the system prompt from
// static files, dynamic command output, and template text.
#Role: {
	// Embed UTD pattern (file, command, prompt, shell, timeout)
	#UTD

	// Metadata
	description?: string
	tags?: [...string]
}
