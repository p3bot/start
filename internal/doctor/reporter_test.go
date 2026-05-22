package doctor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// TestPrintResult_FixRendersForStatusNotFound asserts that the human-readable
// reporter surfaces the Fix line for StatusNotFound results. Previously the
// guard only covered StatusFail and StatusWarn, which silently swallowed
// install hints attached to missing-extract reports.
func TestPrintResult_FixRendersForStatusNotFound(t *testing.T) {
	// Disable colour so the captured output is plain text we can match against.
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	var buf bytes.Buffer
	r := NewReporter(&buf, false, false)
	r.printResult(CheckResult{
		Status:  StatusNotFound,
		Label:   "myrole",
		Message: "module not extracted",
		Fix:     "Run 'start modules install myrole' to fetch the module",
	}, false)

	out := buf.String()
	if !strings.Contains(out, "myrole") {
		t.Errorf("output = %q, want it to include the label", out)
	}
	if !strings.Contains(out, "Fix: Run 'start modules install myrole'") {
		t.Errorf("output = %q, want the Fix line to be rendered", out)
	}
}

// TestPrintResult_NoFixForStatusPass confirms the guard still suppresses
// Fix lines for passing results (where a Fix would be nonsensical).
func TestPrintResult_NoFixForStatusPass(t *testing.T) {
	prev := color.NoColor
	color.NoColor = true
	t.Cleanup(func() { color.NoColor = prev })

	var buf bytes.Buffer
	r := NewReporter(&buf, false, false)
	r.printResult(CheckResult{
		Status:  StatusPass,
		Label:   "ok",
		Message: "all good",
		Fix:     "should never render",
	}, false)

	if strings.Contains(buf.String(), "should never render") {
		t.Errorf("output = %q, Fix line should not render for StatusPass", buf.String())
	}
}
