package config

import "github.com/start-cli/start/test/testdata/schemas"

// Invalid: timeout must be int, not string
tasks: "bad": schemas.#Task & {
	command: "echo test"
	prompt:  "Test"
	timeout: "thirty"
}
