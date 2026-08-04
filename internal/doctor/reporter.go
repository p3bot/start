package doctor

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/p3bot/start/internal/tui"
)

// fprintDim writes text dim with cyan parentheses. Byte iteration is safe
// since the delimiters are ASCII.
func fprintDim(w io.Writer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '(' || s[i] == ')' {
			if i > start {
				tui.ColorDim.Fprint(w, s[start:i])
			}
			tui.ColorCyan.Fprintf(w, "%c", s[i])
			start = i + 1
		}
	}
	if start < len(s) {
		tui.ColorDim.Fprint(w, s[start:])
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

func (r *Reporter) printHeader() {
	fmt.Fprintln(r.w)
	tui.ColorHeader.Fprintln(r.w, "start doctor")
	tui.ColorSeparator.Fprintln(r.w, strings.Repeat("═", 59))
	fmt.Fprintln(r.w)
}

func (r *Reporter) printSection(section SectionResult) {
	sc := tui.CategoryColor(section.Name)
	if sc != nil {
		sc.Fprintf(r.w, "%s", section.Name)
	} else {
		fmt.Fprint(r.w, section.Name)
	}
	if section.Summary != "" {
		fmt.Fprint(r.w, " ")
		fprintDim(r.w, "("+section.Summary+")")
	}
	fmt.Fprintln(r.w)

	for _, result := range section.Results {
		r.printResult(result, section.NoIcons)
	}

	fmt.Fprintln(r.w)
}

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

func (r *Reporter) printResult(result CheckResult, noIcons bool) {
	indent := strings.Repeat("  ", result.Indent+1)

	if noIcons || result.NoIcon {
		if result.Message == "" {
			fmt.Fprintf(r.w, "%s%s\n", indent, result.Label)
		} else {
			fmt.Fprintf(r.w, "%s%-10s ", indent, result.Label+":")
			fprintDim(r.w, result.Message)
			fmt.Fprintln(r.w)
		}
		return
	}

	sc := statusColor(result.Status)
	symbol := result.Status.Symbol()

	fmt.Fprint(r.w, indent)
	sc.Fprintf(r.w, "%s", symbol)
	if result.Message == "" {
		fmt.Fprintf(r.w, " %s\n", result.Label)
	} else {
		fmt.Fprintf(r.w, " %s - ", result.Label)
		fprintDim(r.w, result.Message)
		fmt.Fprintln(r.w)
	}

	if result.Fix != "" && result.IsIssue() {
		fixIndent := strings.Repeat("  ", result.Indent+2)
		fmt.Fprint(r.w, fixIndent)
		fprintDim(r.w, "Fix: "+result.Fix)
		fmt.Fprintln(r.w)
	}

	if r.verbose && len(result.Details) > 0 {
		detailIndent := strings.Repeat("  ", result.Indent+2)
		for _, detail := range result.Details {
			tui.ColorDim.Fprintf(r.w, "%s%s\n", detailIndent, detail)
		}
	}
}

func (r *Reporter) printSummary(report Report) {
	tui.ColorHeader.Fprintln(r.w, "Summary")
	tui.ColorSeparator.Fprintln(r.w, strings.Repeat("─", 59))

	errCount := report.ErrorCount()
	warnings := report.WarnCount()
	missing := report.MissingCount()

	if errCount == 0 && warnings == 0 && missing == 0 {
		tui.ColorSuccess.Fprintln(r.w, "  No issues found")
		fmt.Fprintln(r.w)
		return
	}

	// sep prefixes each non-first segment so they compose regardless of which counts are non-zero.
	fmt.Fprint(r.w, "  ")
	sep := ""
	if errCount > 0 {
		label := "error"
		if errCount > 1 {
			label = "errors"
		}
		tui.ColorError.Fprintf(r.w, "%d %s", errCount, label)
		sep = ", "
	}
	if warnings > 0 {
		fmt.Fprint(r.w, sep)
		label := "warning"
		if warnings > 1 {
			label = "warnings"
		}
		tui.ColorWarning.Fprintf(r.w, "%d %s", warnings, label)
		sep = ", "
	}
	if missing > 0 {
		fmt.Fprint(r.w, sep)
		label := "missing"
		tui.ColorDim.Fprintf(r.w, "%d %s", missing, label)
	}
	fmt.Fprintln(r.w, " found")
	fmt.Fprintln(r.w)

	issues := report.Issues()
	if len(issues) > 0 {
		fmt.Fprintln(r.w, "Issues:")
		for _, issue := range issues {
			sc := statusColor(issue.Status)
			symbol := issue.Status.Symbol()
			fmt.Fprint(r.w, "  ")
			sc.Fprintf(r.w, "%s", symbol)
			if issue.Message != "" {
				fmt.Fprintf(r.w, " %s: ", issue.Label)
				fprintDim(r.w, issue.Message)
				fmt.Fprintln(r.w)
			} else {
				fmt.Fprintf(r.w, " %s\n", issue.Label)
			}
		}
	}
}

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

		sc.Fprintf(r.w, "%s: ", prefix)
		if issue.Message != "" {
			fmt.Fprintf(r.w, "%s: %s\n", issue.Label, issue.Message)
		} else {
			fmt.Fprintf(r.w, "%s\n", issue.Label)
		}
	}
}
