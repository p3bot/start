// Test configuration using P-002 schema from CUE Central Registry
package config

import "github.com/grantcarthew/start-assets/schemas@v0"

// Agents - mirrors P-002 claude agent
agents: {
	claude: schemas.#Agent & {
		bin:           "claude"
		command:       "{{.bin}} --model {{.model}} --permission-mode default --append-system-prompt-file {{.role_file}} {{.prompt}}"
		description:   "Claude Code by Anthropic - agentic coding assistant"
		default_model: "sonnet"
		models: {
			haiku:  "haiku"
			sonnet: "sonnet"
			opus:   "opus"
		}
		tags: ["anthropic", "claude", "coding", "agent"]
	}
	gemini: schemas.#Agent & {
		bin:           "gemini"
		command:       "{{.bin}} --model {{.model}} {{.prompt}}"
		description:   "Google Gemini CLI"
		default_model: "flash"
		models: {
			flash:      "gemini-2.0-flash"
			"flash-lite": "gemini-2.0-flash-lite"
			pro:        "gemini-2.0-pro"
		}
		tags: ["google", "gemini"]
	}
}

// Contexts - mirrors P-002 contexts
contexts: {
	environment: schemas.#Context & {
		required: true
		file:     "~/context/ENVIRONMENT.md"
		prompt:   "User environment context: {{.file_contents}}"
		tags:     ["system", "environment"]
	}
	project: schemas.#Context & {
		default: true
		file:    "./PROJECT.md"
		prompt:  "Project documentation: {{.file_contents}}"
		tags:    ["project", "documentation"]
	}
	"git-status": schemas.#Context & {
		command: "git status --short"
		prompt:  "Current git status:\n{{.command_output}}"
		tags:    ["git", "status"]
	}
}

// Roles - mirrors P-002 golang roles
roles: {
	"golang-agent": schemas.#Role & {
		description: "Go programming language expert - autonomous agent mode"
		tags:        ["golang", "programming", "agent", "autonomous"]
		prompt: """
			# Role: Go Programming Language Expert

			You are an expert in Go programming language with deep knowledge of:
			- Go syntax, idioms, and best practices
			- Concurrency patterns (goroutines, channels, sync package)
			- Standard library and ecosystem
			- Performance optimization and profiling
			- Testing and benchmarking

			## Operating Mode: Autonomous Agent

			Work autonomously with minimal interruption. Make reasonable decisions
			and proceed with implementation. Only ask questions when truly blocked.
			"""
	}
	"golang-assistant": schemas.#Role & {
		description: "Go programming language expert - collaborative assistant mode"
		tags:        ["golang", "programming", "assistant", "collaborative"]
		prompt: """
			# Role: Go Programming Language Expert

			You are a helpful Go programming assistant who:
			- Explains concepts clearly
			- Asks clarifying questions
			- Provides multiple options when appropriate
			- Teaches while helping

			Be collaborative and conversational.
			"""
	}
}

// Tasks - mirrors P-002 golang tasks
tasks: {
	"code-review": schemas.#Task & {
		description: "Comprehensive Go code review"
		tags:        ["golang", "review", "code-quality"]
		role:        "golang-agent"
		command:     "git diff --staged"
		prompt: """
			Review the following Go code changes:

			```diff
			{{.command_output}}
			```

			Focus on:
			- Code correctness
			- Idiomatic Go patterns
			- Error handling
			- Performance considerations

			## Custom Instructions
			{{.instructions}}
			"""
	}
	debug: schemas.#Task & {
		description: "Systematic Go debugging assistance"
		tags:        ["golang", "debug", "troubleshooting"]
		role:        "golang-assistant"
		prompt: """
			Help debug the following Go issue:

			{{.instructions}}

			Approach systematically:
			1. Understand the expected vs actual behavior
			2. Identify potential causes
			3. Suggest debugging steps
			4. Provide solutions
			"""
	}
}
