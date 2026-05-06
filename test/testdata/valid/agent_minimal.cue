package config

import "github.com/start-cli/start/test/testdata/schemas"

// Minimal valid agent - only command is required
agents: "simple": schemas.#Agent & {
	command: "my-ai-tool {{.prompt}}"
}
