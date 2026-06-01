package cli

import (
	"path/filepath"
	"strings"
)

// Glamour standard style names recorded as the settled per-invocation style.
const (
	markdownStyleDark  = "dark"
	markdownStyleLight = "light"
)

// markdownSource identifies which resolved source produced a body, so the
// styling predicate can decide whether the body is Markdown to be rendered.
type markdownSource int

const (
	sourceFile markdownSource = iota
	sourcePrompt
	sourceCommand
)

// shouldStyleMarkdown reports whether a resolved body should be Markdown-styled.
// The body is styled iff it is a rendered prompt, or a file whose extension is
// .md or .markdown. Command-sourced output is never styled; filename is
// consulted only for file sources. Agent command templates never reach this
// predicate — getAgent emits them raw without consulting it.
func shouldStyleMarkdown(src markdownSource, filename string) bool {
	switch src {
	case sourcePrompt:
		return true
	case sourceFile:
		return isMarkdownFile(filename)
	default:
		return false
	}
}

// isMarkdownFile reports whether filename has a Markdown extension.
func isMarkdownFile(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".md", ".markdown":
		return true
	default:
		return false
	}
}
