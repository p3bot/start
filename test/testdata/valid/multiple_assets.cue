package config

import "github.com/p3bot/start/test/testdata/schemas"

// Multiple agents in one file
agents: {
	"claude": schemas.#Agent & {
		bin:         "claude"
		command:     "{{.bin}} --model {{.model}} {{.prompt}}"
		description: "Claude by Anthropic"
		default_model: "sonnet"
		models: {
			sonnet: "claude-sonnet-4-20250514"
			opus:   "claude-opus-4-20250514"
		}
	}
	"gemini": schemas.#Agent & {
		bin:         "gemini"
		command:     "{{.bin}} --model {{.model}} {{.prompt}}"
		description: "Google Gemini"
		default_model: "pro"
		models: {
			flash: "gemini-2.0-flash"
			pro:   "gemini-2.0-pro"
		}
	}
}

// Multiple contexts
contexts: {
	"environment": schemas.#Context & {
		required: true
		file:     "~/context/ENVIRONMENT.md"
		prompt:   "Environment: {{.file_contents}}"
	}
	"project": schemas.#Context & {
		default: true
		file:    "./PROJECT.md"
		prompt:  "Project: {{.file_contents}}"
	}
}

// Multiple roles
roles: {
	"assistant": schemas.#Role & {
		prompt: "You are a helpful assistant."
	}
	"reviewer": schemas.#Role & {
		prompt: "You are an expert code reviewer."
	}
}

// Multiple tasks
tasks: {
	"review": schemas.#Task & {
		role:    "reviewer"
		command: "git diff --staged"
		prompt:  "Review:\n{{.command_output}}"
	}
	"explain": schemas.#Task & {
		role:   "assistant"
		prompt: "Explain: {{.instructions}}"
	}
}
