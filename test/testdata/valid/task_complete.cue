package config

import "github.com/start-cli/start/test/testdata/schemas"

// Complete task with all UTD fields and references
tasks: "code-review": schemas.#Task & {
	description: "Review code changes before committing"
	role:        "code-reviewer"
	agent:       "claude"
	command:     "git diff --staged"
	prompt: """
		Review these code changes:

		{{.command_output}}

		Special focus: {{.instructions}}

		Check for:
		- Code quality
		- Security issues
		- Best practices
		"""
	timeout: 30
	tags: ["review", "git", "quality"]
}
