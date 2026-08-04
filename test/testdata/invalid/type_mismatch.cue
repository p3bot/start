package config

import "github.com/p3bot/start/test/testdata/schemas"

// Invalid: timeout must be int, not string
tasks: "bad": schemas.#Task & {
	command: "echo test"
	prompt:  "Test"
	timeout: "thirty"
}
