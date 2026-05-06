// Global configuration - base settings
// This simulates ~/.config/start/

agents: {
	"claude": {
		command:       "claude --model {{.model}} {{.prompt}}"
		bin:           "claude"
		description:   "Claude by Anthropic"
		default_model: "sonnet"
		models: {
			sonnet: "claude-sonnet-4-20250514"
			opus:   "claude-opus-4-20250514"
		}
	}
}

contexts: {
	"environment": {
		required: true
		file:     "~/context/ENVIRONMENT.md"
		prompt:   "Environment: {{.file_contents}}"
	}
}

roles: {
	"assistant": {
		description: "General assistant"
		prompt:      "You are a helpful assistant."
	}
}

// Global default settings
settings: {
	default_agent: "claude"
	shell:         "/bin/bash"
	timeout:       120
}
