package config

import "github.com/p3bot/start/test/testdata/schemas"

// Full-featured agent with all optional fields
agents: "claude": schemas.#Agent & {
	bin:         "claude"
	command:     "{{.bin}} --model {{.model}} --append-system-prompt {{.role}} {{.prompt}}"
	description: "Claude Code by Anthropic"
	default_model: "sonnet"
	models: {
		haiku:  "claude-3-5-haiku-20241022"
		sonnet: "claude-sonnet-4-20250514"
		opus:   "claude-opus-4-20250514"
	}
	tags: ["anthropic", "claude", "ai"]
}
