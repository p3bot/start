package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/fatih/color"
	"golang.org/x/term"
)

var (
	ColorError     = color.New(color.FgRed)
	ColorWarning   = color.New(color.FgYellow)
	ColorSuccess   = color.New(color.FgGreen)
	ColorHeader    = color.New(color.FgGreen)
	ColorSeparator = color.New(color.FgMagenta)
	ColorDim       = color.New(color.Faint)
	ColorCyan      = color.New(color.FgCyan)
	ColorBlue      = color.New(color.FgBlue)
	ColorHiYellow  = color.New(color.FgHiYellow)

	// Asset category colours
	ColorAgents    = color.New(color.FgBlue)
	ColorRoles     = color.New(color.FgGreen)
	ColorContexts  = color.New(color.FgCyan)
	ColorTasks     = color.New(color.FgHiYellow)
	ColorSettings  = color.New(color.FgMagenta)
	ColorPrompts   = color.New(color.Faint)
	ColorPaths     = color.New(color.FgHiCyan)
	ColorInstalled = color.New(color.FgHiGreen)
	ColorRegistry  = color.New(color.FgYellow)
)

// CategoryColor returns the colour for an asset category.
// Matching is case-insensitive.
func CategoryColor(category string) *color.Color {
	switch strings.ToLower(category) {
	case "agents":
		return ColorAgents
	case "roles":
		return ColorRoles
	case "contexts":
		return ColorContexts
	case "tasks":
		return ColorTasks
	case "settings":
		return ColorSettings
	default:
		return ColorDim
	}
}

// Annotate returns text wrapped in cyan parentheses with dim content: (text)
func Annotate(format string, a ...any) string {
	text := fmt.Sprintf(format, a...)
	return ColorCyan.Sprint("(") + ColorDim.Sprint(text) + ColorCyan.Sprint(")")
}

// Bracket returns text wrapped in cyan square brackets with dim content: [text]
func Bracket(format string, a ...any) string {
	text := fmt.Sprintf(format, a...)
	return ColorCyan.Sprint("[") + ColorDim.Sprint(text) + ColorCyan.Sprint("]")
}

// Progress provides an in-place progress indicator using carriage return.
// Each Update overwrites the previous line. Done clears it.
// When the writer is not a terminal, Update and Done are no-ops.
type Progress struct {
	w     io.Writer
	tty   bool // true when w is a terminal; guards carriage-return writes
	width int  // length of the last written line, for clearing
}

// NewProgress creates a progress writer.
// Carriage-return progress output is suppressed when w is not a terminal or quiet is true.
func NewProgress(w io.Writer, quiet bool) *Progress {
	tty := false
	if !quiet {
		if f, ok := w.(*os.File); ok {
			tty = term.IsTerminal(int(f.Fd()))
		}
	}
	return &Progress{w: w, tty: tty}
}

// Update writes a progress message, overwriting the previous line.
// No-op when the writer is not a terminal.
func (p *Progress) Update(format string, a ...any) {
	if !p.tty {
		return
	}
	msg := fmt.Sprintf(format, a...)
	msgWidth := utf8.RuneCountInString(msg)
	padding := ""
	if msgWidth < p.width {
		padding = strings.Repeat(" ", p.width-msgWidth)
	}
	p.width = msgWidth
	_, _ = fmt.Fprintf(p.w, "\r%s%s", msg, padding)
}

// Done clears the progress line and resets.
// No-op when the writer is not a terminal.
func (p *Progress) Done() {
	if !p.tty || p.width == 0 {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", p.width))
	p.width = 0
}
