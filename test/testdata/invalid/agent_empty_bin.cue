package config

import "github.com/p3bot/start/test/testdata/schemas"

// Invalid: bin must not be empty if provided
agents: "bad": schemas.#Agent & {
	command: "test"
	bin:     ""
}
