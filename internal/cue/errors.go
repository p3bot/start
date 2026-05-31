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
func FormatError(err error) error {
	if err == nil {
		return nil
	}

	cueErrs := errors.Errors(err)
	if len(cueErrs) == 0 {
		return err
	}

	first := cueErrs[0]
	pos := first.Position()

	// Msg() yields format+args without the file:line prefix that Error() adds.
	format, args := first.Msg()
	var message string
	if format != "" && len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
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

	first := FormatError(cueErrs[0]).Error()
	return first + " (and " + strconv.Itoa(len(cueErrs)-1) + " more errors)"
}

// FormatErrorWithContext converts a CUE error into a ValidationError with a
// source snippet read from the error's file.
func FormatErrorWithContext(err error) *ValidationError {
	if err == nil {
		return nil
	}

	baseErr := FormatError(err)
	if baseErr == nil {
		return nil
	}

	ve, ok := baseErr.(*ValidationError)
	if !ok {
		return &ValidationError{Message: err.Error()}
	}

	if ve.Filename != "" && ve.Line > 0 {
		ve.Context = generateSourceContext(ve.Filename, ve.Line, ve.Column)
	}

	return ve
}

// generateSourceContext shows 2 lines either side of the error line, with line
// numbers and a caret under the error column.
func generateSourceContext(filename string, line, column int) string {
	file, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	const contextLines = 2
	startLine := max(line-contextLines, 1)
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

	maxLineNum := lines[len(lines)-1].num
	lineNumWidth := len(strconv.Itoa(maxLineNum))

	var sb strings.Builder
	for _, l := range lines {
		numStr := strconv.Itoa(l.num)
		padding := strings.Repeat(" ", lineNumWidth-len(numStr))
		sb.WriteString("    " + padding + numStr + " | " + l.text + "\n")

		if l.num == line && column > 0 {
			pointerPadding := strings.Repeat(" ", lineNumWidth+7+column-1) // 7 = "    " + " | "
			sb.WriteString(pointerPadding + "^\n")
		}
	}

	return sb.String()
}
