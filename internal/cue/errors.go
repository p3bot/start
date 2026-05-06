package cue

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cuelang.org/go/cue/errors"
)

// FormatError converts a CUE error into a user-friendly error.
// It extracts position information and formats messages for clarity.
func FormatError(err error) error {
	if err == nil {
		return nil
	}

	// Try to extract CUE errors with position information
	cueErrs := errors.Errors(err)
	if len(cueErrs) == 0 {
		return err
	}

	// Format the first error with position info
	first := cueErrs[0]
	pos := first.Position()

	// Use Msg() to get the unformatted message without position prefix.
	// Msg() returns the format string and args separately, allowing us to
	// reconstruct the message without file:line prefixes.
	format, args := first.Msg()
	var message string
	if format != "" && len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	// Fallback to Error() if Msg() returns empty or unusable format
	if message == "" {
		message = first.Error()
	}

	ve := &ValidationError{
		Message: message,
	}

	if pos.IsValid() {
		ve.Filename = pos.Filename()
		ve.Line = pos.Line()
		ve.Column = pos.Column()
	}

	return ve
}

// FormatErrors formats multiple CUE errors into a single error message.
func FormatErrors(err error) []error {
	if err == nil {
		return nil
	}

	cueErrs := errors.Errors(err)
	if len(cueErrs) == 0 {
		return []error{err}
	}

	result := make([]error, 0, len(cueErrs))
	for _, e := range cueErrs {
		result = append(result, FormatError(e))
	}
	return result
}

// ErrorSummary returns a short summary of CUE errors.
func ErrorSummary(err error) string {
	if err == nil {
		return ""
	}

	cueErrs := errors.Errors(err)
	if len(cueErrs) == 0 {
		return err.Error()
	}

	if len(cueErrs) == 1 {
		return FormatError(cueErrs[0]).Error()
	}

	// Multiple errors: show first and count
	first := FormatError(cueErrs[0]).Error()
	return first + " (and " + strconv.Itoa(len(cueErrs)-1) + " more errors)"
}

// FormatErrorWithContext converts a CUE error into a ValidationError with source context.
// It reads the source file to provide a snippet around the error location.
func FormatErrorWithContext(err error) *ValidationError {
	if err == nil {
		return nil
	}

	// Get basic formatted error first
	baseErr := FormatError(err)
	if baseErr == nil {
		return nil
	}

	ve, ok := baseErr.(*ValidationError)
	if !ok {
		return &ValidationError{Message: err.Error()}
	}

	// Add source context if we have file and line info
	if ve.Filename != "" && ve.Line > 0 {
		ve.Context = generateSourceContext(ve.Filename, ve.Line, ve.Column)
	}

	return ve
}

// generateSourceContext reads a file and generates a context snippet around the given line.
// It shows 2 lines before and after the error line, with line numbers and a pointer to the column.
func generateSourceContext(filename string, line, column int) string {
	file, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	// Read relevant lines
	const contextLines = 2
	startLine := line - contextLines
	if startLine < 1 {
		startLine = 1
	}
	endLine := line + contextLines

	scanner := bufio.NewScanner(file)
	lineNum := 0
	var lines []struct {
		num  int
		text string
	}

	for scanner.Scan() {
		lineNum++
		if lineNum >= startLine && lineNum <= endLine {
			lines = append(lines, struct {
				num  int
				text string
			}{lineNum, scanner.Text()})
		}
		if lineNum > endLine {
			break
		}
	}

	if len(lines) == 0 {
		return ""
	}

	// Find max line number width for alignment
	maxLineNum := lines[len(lines)-1].num
	lineNumWidth := len(strconv.Itoa(maxLineNum))

	var sb strings.Builder
	for _, l := range lines {
		// Format: "  12 | code here"
		numStr := strconv.Itoa(l.num)
		padding := strings.Repeat(" ", lineNumWidth-len(numStr))
		sb.WriteString("    " + padding + numStr + " | " + l.text + "\n")

		// Add column pointer on the error line
		if l.num == line && column > 0 {
			// Create pointer line: "       ^"
			pointerPadding := strings.Repeat(" ", lineNumWidth+7+column-1) // 7 = "    " + " | "
			sb.WriteString(pointerPadding + "^\n")
		}
	}

	return sb.String()
}
