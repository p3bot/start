package cli

import (
	"testing"

	"github.com/fatih/color"
	"github.com/start-cli/start/internal/orchestration"
)

// TestFlagsMarkdownStyleSettlesOnce pins the lazy-but-once contract: the style
// is computed on first access and cached, so a later decoration flip cannot
// change it. Off-TTY (the test process) settle never probes and returns dark,
// which lets this run without a terminal.
func TestFlagsMarkdownStyleSettlesOnce(t *testing.T) {
	restoreNoColor(t)
	color.NoColor = true // decoration off → dark, no background probe

	f := &Flags{}
	if got := f.MarkdownStyle(); got != markdownStyleDark {
		t.Fatalf("first call = %q, want %q", got, markdownStyleDark)
	}

	// Flip decoration on after the first settle. A cached value is unchanged; a
	// per-call re-derivation would still be dark here (off-TTY), so the flip
	// alone cannot prove caching — but it guards against a future regression
	// where the value is recomputed from changed state.
	color.NoColor = false
	if got := f.MarkdownStyle(); got != markdownStyleDark {
		t.Errorf("second call = %q, want cached %q", got, markdownStyleDark)
	}
}

func TestShouldStyleMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      markdownSource
		filename string
		want     bool
	}{
		{"prompt is styled", sourcePrompt, "", true},
		{"command is never styled", sourceCommand, "", false},
		{"md file is styled", sourceFile, "notes.md", true},
		{"markdown file is styled", sourceFile, "README.markdown", true},
		{"uppercase MD extension is styled", sourceFile, "GUIDE.MD", true},
		{"txt file is not styled", sourceFile, "notes.txt", false},
		{"extensionless file is not styled", sourceFile, "Makefile", false},
		{"empty filename is not styled", sourceFile, "", false},
		{"path with md extension is styled", sourceFile, "/home/u/docs/a.md", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldStyleMarkdown(tt.src, tt.filename); got != tt.want {
				t.Errorf("shouldStyleMarkdown(%v, %q) = %v, want %v", tt.src, tt.filename, got, tt.want)
			}
		})
	}
}

func TestUTDSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields orchestration.UTDFields
		want   markdownSource
	}{
		{"file wins", orchestration.UTDFields{File: "a.md"}, sourceFile},
		{"prompt when no file", orchestration.UTDFields{Prompt: "hi"}, sourcePrompt},
		{"command when neither", orchestration.UTDFields{Command: "echo hi"}, sourceCommand},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := utdSource(tt.fields); got != tt.want {
				t.Errorf("utdSource(%+v) = %v, want %v", tt.fields, got, tt.want)
			}
		})
	}
}
