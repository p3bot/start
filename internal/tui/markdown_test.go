package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// withNoColor sets color.NoColor for the duration of a test and restores it.
func withNoColor(t *testing.T, v bool) {
	t.Helper()
	prev := color.NoColor
	color.NoColor = v
	t.Cleanup(func() { color.NoColor = prev })
}

func TestRenderMarkdownRawPassthroughWhenDecorationOff(t *testing.T) {
	withNoColor(t, true)

	const md = "# Heading\n\nSome *body* text.\n"
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, md, "dark"); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if got := buf.String(); got != md {
		t.Errorf("raw passthrough altered content:\n got %q\nwant %q", got, md)
	}
}

func TestRenderMarkdownEmptyInputProducesNoArtefacts(t *testing.T) {
	// Even with decoration on, empty input must produce no styled output.
	withNoColor(t, false)

	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, "", "dark"); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty input produced output: %q", buf.String())
	}
}

func TestRenderMarkdownStylesWhenDecorationOn(t *testing.T) {
	withNoColor(t, false)

	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, "# Heading\n", "dark"); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if !strings.Contains(buf.String(), "Heading") {
		t.Errorf("rendered output missing heading text: %q", buf.String())
	}
	// The --color=always | pipe case: dst is a non-TTY buffer, so the profile
	// would default to NoTTY and strip colour. Forcing a coloured profile must
	// preserve ANSI escapes.
	if !strings.Contains(buf.String(), "\x1b[") {
		t.Errorf("expected ANSI escapes in styled output, got %q", buf.String())
	}
}

func TestRenderMarkdownFallsBackOnInvalidStyle(t *testing.T) {
	withNoColor(t, false)

	const md = "# Heading\n"
	var buf bytes.Buffer
	if err := RenderMarkdown(&buf, md, "no-such-style"); err != nil {
		t.Fatalf("RenderMarkdown: %v", err)
	}
	if got := buf.String(); got != md {
		t.Errorf("invalid style should fall back to raw:\n got %q\nwant %q", got, md)
	}
}

func TestMarkdownWidthFallback(t *testing.T) {
	t.Parallel()
	// A non-*os.File writer has no terminal width; fall back to 80.
	if got := markdownWidth(&bytes.Buffer{}); got != markdownWidthFallback {
		t.Errorf("markdownWidth fallback = %d, want %d", got, markdownWidthFallback)
	}
}
