package doctor

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/start-cli/start/internal/tui"
)

// fprintDim writes text with dim styling and cyan parenthetical delimiters.
// Text is dim, ( and ) are cyan. Byte iteration is safe here as delimiters are ASCII.
func fprintDim(w io.Writer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '(' || s[i] == ')' {
			if i > start {
				_, _ = tui.ColorDim.Fprint(w, s[start:i])
			}
			_, _ = tui.ColorCyan.Fprintf(w, "%c", s[i])
			start = i + 1
		}
	}
	if start < len(s) {
		_, _ = tui.ColorDim.Fprint(w, s[start:])
	}
}

// Reporter handles output formatting for doctor results.
type Reporter struct {
	w       io.Writer
	verbose bool
	quiet   bool
}

// NewReporter creates a new reporter.
func NewReporter(w io.Writer, verbose, quiet bool) *Reporter {
	return &Reporter{
		w:       w,
		verbose: verbose,
		quiet:   quiet,
	}
}

// Print outputs the complete report.
func (r *Reporter) Print(report Report) {
	if r.quiet {
		r.printQuiet(report)
		return
	}

	r.printHeader()

	for _, section := range report.Sections {
		r.printSection(section)
	}

	r.printSummary(report)
}

// printHeader prints the doctor header.
func (r *Reporter) printHeader() {
	_, _ = fmt.Fprintln(r.w)
	_, _ = tui.ColorHeader.Fprintln(r.w, "start doctor")
	_, _ = tui.ColorSeparator.Fprintln(r.w, strings.Repeat("═", 59))
	_, _ = fmt.Fprintln(r.w)
}

// printSection prints a single section.
func (r *Reporter) printSection(section SectionResult) {
	// Section header
	sc := tui.CategoryColor(section.Name)
	if sc != nil {
		_, _ = sc.Fprintf(r.w, "%s", section.Name)
	} else {
		_, _ = fmt.Fprint(r.w, section.Name)
	}
	if section.Summary != "" {
		_, _ = fmt.Fprint(r.w, " ")
		fprintDim(r.w, "("+section.Summary+")")
	}
	_, _ = fmt.Fprintln(r.w)

	// Section results
	for _, result := range section.Results {
		r.printResult(result, section.NoIcons)
	}

	_, _ = fmt.Fprintln(r.w)
}

// statusColor returns the colour for a status.
func statusColor(s Status) *color.Color {
	switch s {
	case StatusPass:
		return tui.ColorSuccess
	case StatusFail:
		return tui.ColorError
	case StatusWarn:
		return tui.ColorWarning
	default:
		return tui.ColorDim
	}
}

// printResult prints a single check result.
func (r *Reporter) printResult(result CheckResult, noIcons bool) {
	indent := strings.Repeat("  ", result.Indent+1)

	// Format based on content and icon mode
	if noIcons || result.NoIcon {
		// No icons - used for info-only sections like Version, Repository
		// and per-result headers like config directory names
		if result.Message == "" {
			_, _ = fmt.Fprintf(r.w, "%s%s\n", indent, result.Label)
		} else {
			_, _ = fmt.Fprintf(r.w, "%s%-10s ", indent, result.Label+":")
			fprintDim(r.w, result.Message)
			_, _ = fmt.Fprintln(r.w)
		}
		return
	}

	sc := statusColor(result.Status)
	symbol := result.Status.Symbol()

	// Format based on content
	_, _ = fmt.Fprint(r.w, indent)
	_, _ = sc.Fprintf(r.w, "%s", symbol)
	if result.Message == "" {
		_, _ = fmt.Fprintf(r.w, " %s\n", result.Label)
	} else {
		_, _ = fmt.Fprintf(r.w, " %s - ", result.Label)
		fprintDim(r.w, result.Message)
		_, _ = fmt.Fprintln(r.w)
	}

	// Print fix suggestion if present and the result is actionable.
	// IsIssue treats NotFound as actionable only when Fix is non-empty,
	// so the explicit Fix != "" guard here covers Fail/Warn alone.
	if result.Fix != "" && result.IsIssue() {
		fixIndent := strings.Repeat("  ", result.Indent+2)
		_, _ = fmt.Fprint(r.w, fixIndent)
		fprintDim(r.w, "Fix: "+result.Fix)
		_, _ = fmt.Fprintln(r.w)
	}

	// Print details in verbose mode
	if r.verbose && len(result.Details) > 0 {
		detailIndent := strings.Repeat("  ", result.Indent+2)
		for _, detail := range result.Details {
			_, _ = tui.ColorDim.Fprintf(r.w, "%s%s\n", detailIndent, detail)
		}
	}
}

// printSummary prints the summary section.
func (r *Reporter) printSummary(report Report) {
	_, _ = tui.ColorHeader.Fprintln(r.w, "Summary")
	_, _ = tui.ColorSeparator.Fprintln(r.w, strings.Repeat("─", 59))

	errCount := report.ErrorCount()
	warnings := report.WarnCount()
	missing := report.MissingCount()

	if errCount == 0 && warnings == 0 && missing == 0 {
		_, _ = tui.ColorSuccess.Fprintln(r.w, "  No issues found")
		_, _ = fmt.Fprintln(r.w)
		return
	}

	// Count summary. Sep tracks whether the next segment needs a leading
	// ", " so segments compose cleanly regardless of which are non-zero.
	_, _ = fmt.Fprint(r.w, "  ")
	sep := ""
	if errCount > 0 {
		label := "error"
		if errCount > 1 {
			label = "errors"
		}
		_, _ = tui.ColorError.Fprintf(r.w, "%d %s", errCount, label)
		sep = ", "
	}
	if warnings > 0 {
		_, _ = fmt.Fprint(r.w, sep)
		label := "warning"
		if warnings > 1 {
			label = "warnings"
		}
		_, _ = tui.ColorWarning.Fprintf(r.w, "%d %s", warnings, label)
		sep = ", "
	}
	if missing > 0 {
		_, _ = fmt.Fprint(r.w, sep)
		label := "missing"
		_, _ = tui.ColorDim.Fprintf(r.w, "%d %s", missing, label)
	}
	_, _ = fmt.Fprintln(r.w, " found")
	_, _ = fmt.Fprintln(r.w)

	// List issues
	issues := report.Issues()
	if len(issues) > 0 {
		_, _ = fmt.Fprintln(r.w, "Issues:")
		for _, issue := range issues {
			sc := statusColor(issue.Status)
			symbol := issue.Status.Symbol()
			_, _ = fmt.Fprint(r.w, "  ")
			_, _ = sc.Fprintf(r.w, "%s", symbol)
			if issue.Message != "" {
				_, _ = fmt.Fprintf(r.w, " %s: ", issue.Label)
				fprintDim(r.w, issue.Message)
				_, _ = fmt.Fprintln(r.w)
			} else {
				_, _ = fmt.Fprintf(r.w, " %s\n", issue.Label)
			}
		}
	}
}

// printQuiet prints minimal output for quiet mode.
func (r *Reporter) printQuiet(report Report) {
	issues := report.Issues()
	for _, issue := range issues {
		sc := statusColor(issue.Status)
		prefix := "Warning"
		switch issue.Status {
		case StatusFail:
			prefix = "Error"
		case StatusNotFound:
			prefix = "Missing"
		}

		_, _ = sc.Fprintf(r.w, "%s: ", prefix)
		if issue.Message != "" {
			_, _ = fmt.Fprintf(r.w, "%s: %s\n", issue.Label, issue.Message)
		} else {
			_, _ = fmt.Fprintf(r.w, "%s\n", issue.Label)
		}
	}
}
