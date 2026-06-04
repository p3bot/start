package orchestration

import (
	"fmt"
	"strings"
)

// ComposeSegments resolves each argument as a file path or literal and joins the
// resolved segments with exactly one blank line between adjacent non-empty
// segments. With exactly one argument it returns that resolved segment verbatim,
// bypassing the drop-empty step, so single-argument behaviour is byte-identical
// to the previous single-argument handling. fileNoun names the segment kind in
// read errors (for example "prompt file" or "instructions file").
func ComposeSegments(args []string, fileNoun string) (string, error) {
	segs := make([]string, len(args))
	for i, arg := range args {
		if IsFilePath(arg) {
			content, err := ReadFilePath(arg)
			if err != nil {
				return "", fmt.Errorf("reading %s %q: %w", fileNoun, arg, err)
			}
			segs[i] = content
			continue
		}
		segs[i] = arg
	}

	if len(args) == 1 {
		return segs[0], nil
	}
	return joinSegments(segs), nil
}

// joinSegments applies the seam rule to already-resolved segments: segments that
// are empty after stripping newline characters are dropped, then the remainder
// is joined with exactly one blank line between adjacent segments. Only newline
// characters (\n) are added or removed at a seam; spaces and tabs are never
// touched, so a CRLF segment keeps its \r before the blank line.
// Zero remaining segments yield the empty string, one remaining yields that
// segment verbatim, and two or more are seam-joined.
func joinSegments(segs []string) string {
	kept := make([]string, 0, len(segs))
	for _, s := range segs {
		if strings.Trim(s, "\n") == "" {
			continue
		}
		kept = append(kept, s)
	}

	switch len(kept) {
	case 0:
		return ""
	case 1:
		return kept[0]
	}

	last := len(kept) - 1
	for i, s := range kept {
		switch i {
		case 0:
			kept[i] = strings.TrimRight(s, "\n")
		case last:
			kept[i] = strings.TrimLeft(s, "\n")
		default:
			kept[i] = strings.Trim(s, "\n")
		}
	}
	return strings.Join(kept, "\n\n")
}
