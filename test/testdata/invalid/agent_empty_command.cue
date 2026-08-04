package config

import "github.com/p3bot/start/test/testdata/schemas"

// Invalid: command must not be empty
agents: "bad": schemas.#Agent & {
	command: ""
}
