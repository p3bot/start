package config

import "github.com/start-cli/start/test/testdata/schemas"

// Role with file-based content
roles: "general-assistant": schemas.#Role & {
	description: "General purpose AI assistant"
	file:        "~/.config/start/roles/general.md"
	tags: ["general", "assistant"]
}
