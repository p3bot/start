package config

import "github.com/start-cli/start/test/testdata/schemas"

// Invalid: tags must be lowercase kebab-case
contexts: "bad": schemas.#Context & {
	file: "./test.md"
	tags: ["INVALID_TAG", "Also Bad"]
}
