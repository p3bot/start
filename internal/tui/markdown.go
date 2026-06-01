package tui

import (
	"io"
	"os"
	"strings"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/fatih/color"
	"golang.org/x/term"
)

// markdownWidthFallback is the word-wrap width used when the destination's
// terminal width cannot be determined.
const markdownWidthFallback = 80

// RenderMarkdown writes md to dst with terminal Markdown styling, gating on the
// settled color.NoColor decoration state. When decoration is off (or md is
// empty) it writes md through unchanged, preserving the pipe-clean contract.
// When on it renders with the supplied glamour style ("dark" or "light"),
// word-wrapping to dst's terminal width with an 80-column fallback. Any glamour
// construction or render error falls back to writing md raw.
//
// Style is settled once per invocation by the caller and passed in; this helper
// does no terminal background probing, so it stays pure and unit-testable.
func RenderMarkdown(dst io.Writer, md, style string) error {
	if color.NoColor || md == "" {
		_, err := io.WriteString(dst, md)
		return err
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(markdownWidth(dst)),
	)
	if err != nil {
		_, werr := io.WriteString(dst, md)
		return werr
	}

	rendered, err := r.Render(md)
	if err != nil {
		_, werr := io.WriteString(dst, md)
		return werr
	}
	// glamour wraps the document in leading/trailing blank lines; collapse them
	// to a single trailing newline so styled output gains no stray blank lines.
	rendered = strings.Trim(rendered, "\n") + "\n"

	// colorprofile downsamples the TrueColor output glamour v2 emits to dst's
	// capability. A non-TTY dst resolves to NoTTY, which strips all colour; when
	// we reach this point decoration is on (color.NoColor is false), so force a
	// coloured profile to keep "--color=always | pipe" coloured.
	cw := colorprofile.NewWriter(dst, os.Environ())
	if cw.Profile == colorprofile.NoTTY {
		cw.Profile = colorprofile.ANSI256
	}
	_, err = io.WriteString(cw, rendered)
	return err
}

// markdownWidth reports the terminal width of dst, falling back to 80 columns
// when dst is not a terminal or its width cannot be read.
func markdownWidth(dst io.Writer) int {
	f, ok := dst.(*os.File)
	if !ok {
		return markdownWidthFallback
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		return markdownWidthFallback
	}
	return width
}
