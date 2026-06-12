package orchestration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJoinSegments(t *testing.T) {
	tests := []struct {
		name string
		segs []string
		want string
	}{
		{name: "zero segments", segs: nil, want: ""},
		{name: "single non-empty verbatim", segs: []string{"only\n\n"}, want: "only\n\n"},
		{name: "single empty dropped to empty", segs: []string{""}, want: ""},
		{name: "single all-newline dropped to empty", segs: []string{"\n\n"}, want: ""},
		{name: "two plain", segs: []string{"a", "b"}, want: "a\n\nb"},
		{name: "left trailing newlines collapsed", segs: []string{"a\n\n\n", "b"}, want: "a\n\nb"},
		{name: "right leading newlines collapsed", segs: []string{"a", "\n\n\nb"}, want: "a\n\nb"},
		{name: "both sides newlines collapsed", segs: []string{"a\n\n", "\n\nb"}, want: "a\n\nb"},
		{name: "first leading newlines kept", segs: []string{"\n\na", "b"}, want: "\n\na\n\nb"},
		{name: "last trailing newlines kept", segs: []string{"a", "b\n\n"}, want: "a\n\nb\n\n"},
		{name: "trailing spaces preserved at seam", segs: []string{"foo  \n", "bar"}, want: "foo  \n\nbar"},
		{name: "leading spaces preserved at seam", segs: []string{"a", "   bar"}, want: "a\n\n   bar"},
		{name: "empty middle dropped", segs: []string{"a", "", "b"}, want: "a\n\nb"},
		{name: "all-newline middle dropped", segs: []string{"a", "\n\n", "b"}, want: "a\n\nb"},
		{name: "spaces-only middle kept", segs: []string{"a", "   \n", "b"}, want: "a\n\n   \n\nb"},
		{name: "three segments", segs: []string{"a\n", "b\n", "c\n"}, want: "a\n\nb\n\nc\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinSegments(tt.segs); got != tt.want {
				t.Errorf("joinSegments(%q) = %q, want %q", tt.segs, got, tt.want)
			}
		})
	}
}

func TestComposeSegments_Resolution(t *testing.T) {
	dir := t.TempDir()
	fileA := filepath.Join(dir, "a.md")
	if err := os.WriteFile(fileA, []byte("file A body\n"), 0600); err != nil {
		t.Fatalf("writing a.md: %v", err)
	}
	fileB := filepath.Join(dir, "b.md")
	if err := os.WriteFile(fileB, []byte("file B body"), 0600); err != nil {
		t.Fatalf("writing b.md: %v", err)
	}

	got, err := ComposeSegments([]string{fileA, "inline text", fileB}, "prompt file")
	if err != nil {
		t.Fatalf("ComposeSegments error: %v", err)
	}
	want := "file A body\n\ninline text\n\nfile B body"
	if got != want {
		t.Errorf("ComposeSegments = %q, want %q", got, want)
	}
}

func TestComposeSegments_SingleArgumentVerbatim(t *testing.T) {
	dir := t.TempDir()
	allNewline := filepath.Join(dir, "blank.md")
	if err := os.WriteFile(allNewline, []byte("\n\n"), 0600); err != nil {
		t.Fatalf("writing blank.md: %v", err)
	}

	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "literal with trailing newlines", arg: "just text\n\n", want: "just text\n\n"},
		{name: "empty literal", arg: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComposeSegments([]string{tt.arg}, "prompt file")
			if err != nil {
				t.Fatalf("ComposeSegments error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ComposeSegments([%q]) = %q, want %q", tt.arg, got, tt.want)
			}
		})
	}

	// A lone all-newline file passes through unchanged rather than dropping,
	// keeping single-argument behaviour byte-identical to the previous handling.
	got, err := ComposeSegments([]string{allNewline}, "prompt file")
	if err != nil {
		t.Fatalf("ComposeSegments error: %v", err)
	}
	if got != "\n\n" {
		t.Errorf("ComposeSegments lone all-newline file = %q, want %q", got, "\n\n")
	}
}

func TestComposeSegments_ReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.md")

	tests := []struct {
		name        string
		segmentNoun string
		wantPre     string
	}{
		{name: "prompt noun", segmentNoun: "prompt", wantPre: `reading prompt "`},
		{name: "instructions noun", segmentNoun: "instructions", wantPre: `reading instructions "`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ComposeSegments([]string{missing}, tt.segmentNoun)
			if err == nil {
				t.Fatalf("expected error for missing file %q", missing)
			}
			if !strings.HasPrefix(err.Error(), tt.wantPre) {
				t.Errorf("error = %q, want prefix %q", err.Error(), tt.wantPre)
			}
			if !strings.Contains(err.Error(), missing) {
				t.Errorf("error = %q, want it to name %q", err.Error(), missing)
			}
		})
	}
}
