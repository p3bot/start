package config

import "github.com/p3bot/start/test/testdata/schemas"

// Required context - always included in every command
contexts: "environment": schemas.#Context & {
	required:    true
	description: "Local environment information"
	file:        "~/context/ENVIRONMENT.md"
	prompt:      "Read {{.file}} for environment context."
	tags: ["environment", "system"]
}
