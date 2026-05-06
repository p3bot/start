// Local configuration - project overrides
// This simulates ./.start/

// Add a new agent (deep merge - both claude from global and gemini coexist)
agents: {
	gemini: {
		command:       "gemini --model {{.model}} {{.prompt}}"
		bin:           "gemini"
		description:   "Google Gemini"
		default_model: "pro"
		models: {
			flash: "gemini-2.0-flash"
			pro:   "gemini-2.0-pro"
		}
	}
}

// Add project-specific context (deep merge - both environment and project coexist)
contexts: {
	project: {
		default: true
		file:    "./PROJECT.md"
		prompt:  "Project: {{.file_contents}}"
	}
}

// Add project-specific role (deep merge - both assistant and reviewer coexist)
roles: {
	reviewer: {
		description: "Code reviewer for this project"
		prompt:      "You are an expert code reviewer."
	}
}

// Override settings (deep merge - same keys get replaced)
settings: {
	default_agent: "gemini"
}
