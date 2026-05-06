package config

import "github.com/start-cli/start/test/testdata/schemas"

// Invalid: timeout must be >= 1
tasks: "bad": schemas.#Task & {
	command: "echo test"
	prompt:  "Test"
	timeout: 0
}
