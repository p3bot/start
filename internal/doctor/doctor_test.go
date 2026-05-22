package doctor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestStatus_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status Status
		want   string
	}{
		{StatusPass, "pass"},
		{StatusWarn, "warn"},
		{StatusFail, "fail"},
		{StatusInfo, "info"},
		{StatusNotFound, "notfound"},
		{Status(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("Status.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatus_Symbol(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status Status
		want   string
	}{
		{StatusPass, "✓"},
		{StatusWarn, "⚠"},
		{StatusFail, "✗"},
		{StatusInfo, "-"},
		{StatusNotFound, "○"},
		{Status(99), "?"},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			if got := tt.status.Symbol(); got != tt.want {
				t.Errorf("Status.Symbol() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatus_MarshalJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status Status
		want   string
	}{
		{StatusPass, `"pass"`},
		{StatusWarn, `"warn"`},
		{StatusFail, `"fail"`},
		{StatusInfo, `"info"`},
		{StatusNotFound, `"notfound"`},
		{Status(99), `"unknown"`},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			data, err := json.Marshal(tt.status)
			if err != nil {
				t.Fatalf("json.Marshal(Status) error = %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("json.Marshal(Status(%d)) = %s, want %s", tt.status, data, tt.want)
			}
		})
	}
}

func TestStatus_MarshalJSON_InStruct(t *testing.T) {
	t.Parallel()
	result := CheckResult{
		Status:  StatusPass,
		Label:   "test",
		Message: "ok",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal(CheckResult) error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}

	if decoded["status"] != "pass" {
		t.Errorf("status = %v, want %q", decoded["status"], "pass")
	}
}

func TestReport_HasIssues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		sections []SectionResult
		want     bool
	}{
		{
			name:     "empty report",
			sections: nil,
			want:     false,
		},
		{
			name: "all pass",
			sections: []SectionResult{
				{Results: []CheckResult{{Status: StatusPass}}},
			},
			want: false,
		},
		{
			name: "has warning",
			sections: []SectionResult{
				{Results: []CheckResult{{Status: StatusWarn}}},
			},
			want: true,
		},
		{
			name: "has failure",
			sections: []SectionResult{
				{Results: []CheckResult{{Status: StatusFail}}},
			},
			want: true,
		},
		{
			name: "info only",
			sections: []SectionResult{
				{Results: []CheckResult{{Status: StatusInfo}}},
			},
			want: false,
		},
		{
			// Bare NotFound (no Fix) covers cwd/home-prefixed roles and
			// contexts that legitimately may not exist; these must not
			// cause a non-zero exit code.
			name: "not found only without fix",
			sections: []SectionResult{
				{Results: []CheckResult{{Status: StatusNotFound}}},
			},
			want: false,
		},
		{
			// NotFound with a Fix is actionable (e.g. a missing @module/
			// extract whose origin field declared explicit intent). The
			// Fix is the recovery path, so the result must surface as an
			// issue rather than be silently swallowed.
			name: "not found with fix",
			sections: []SectionResult{
				{Results: []CheckResult{{Status: StatusNotFound, Fix: "run install"}}},
			},
			want: true,
		},
		{
			name: "not found alongside fail",
			sections: []SectionResult{
				{Results: []CheckResult{
					{Status: StatusNotFound},
					{Status: StatusFail},
				}},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Report{Sections: tt.sections}
			if got := r.HasIssues(); got != tt.want {
				t.Errorf("Report.HasIssues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReport_ErrorCount(t *testing.T) {
	t.Parallel()
	r := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{
				{Status: StatusPass},
				{Status: StatusFail},
				{Status: StatusFail},
			}},
			{Results: []CheckResult{
				{Status: StatusWarn},
				{Status: StatusFail},
			}},
		},
	}

	if got := r.ErrorCount(); got != 3 {
		t.Errorf("Report.ErrorCount() = %d, want 3", got)
	}
}

func TestReport_WarnCount(t *testing.T) {
	t.Parallel()
	r := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{
				{Status: StatusPass},
				{Status: StatusWarn},
				{Status: StatusWarn},
			}},
			{Results: []CheckResult{
				{Status: StatusFail},
				{Status: StatusWarn},
			}},
		},
	}

	if got := r.WarnCount(); got != 3 {
		t.Errorf("Report.WarnCount() = %d, want 3", got)
	}
}

func TestReport_Issues(t *testing.T) {
	t.Parallel()
	r := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{
				{Status: StatusPass, Label: "pass1"},
				{Status: StatusFail, Label: "fail1"},
				{Status: StatusNotFound, Label: "bare1"},
			}},
			{Results: []CheckResult{
				{Status: StatusWarn, Label: "warn1"},
				{Status: StatusInfo, Label: "info1"},
				{Status: StatusNotFound, Label: "actionable1", Fix: "install"},
			}},
		},
	}

	issues := r.Issues()
	if len(issues) != 3 {
		t.Fatalf("Report.Issues() returned %d items, want 3", len(issues))
	}

	labels := []string{issues[0].Label, issues[1].Label, issues[2].Label}
	want := []string{"fail1", "warn1", "actionable1"}
	for i := range want {
		if labels[i] != want[i] {
			t.Errorf("Report.Issues() labels = %v, want %v", labels, want)
			break
		}
	}
}

// TestReport_MissingCount confirms the new counter ignores bare NotFound
// results and counts only those with a Fix string attached.
func TestReport_MissingCount(t *testing.T) {
	t.Parallel()
	r := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{
				{Status: StatusNotFound, Label: "bare"},
				{Status: StatusNotFound, Label: "actionable1", Fix: "install"},
				{Status: StatusFail, Label: "fail"},
			}},
			{Results: []CheckResult{
				{Status: StatusNotFound, Label: "actionable2", Fix: "install"},
			}},
		},
	}

	if got := r.MissingCount(); got != 2 {
		t.Errorf("Report.MissingCount() = %d, want 2", got)
	}
}

// TestCheckResult_IsIssue covers the predicate that drives every report
// helper and reporter path. The behavioural contract here is the single
// source of truth for what counts as actionable.
func TestCheckResult_IsIssue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		r    CheckResult
		want bool
	}{
		{"pass", CheckResult{Status: StatusPass}, false},
		{"info", CheckResult{Status: StatusInfo}, false},
		{"warn", CheckResult{Status: StatusWarn}, true},
		{"fail", CheckResult{Status: StatusFail}, true},
		{"notfound bare", CheckResult{Status: StatusNotFound}, false},
		{"notfound with fix", CheckResult{Status: StatusNotFound, Fix: "x"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.IsIssue(); got != tt.want {
				t.Errorf("IsIssue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultBuildInfo(t *testing.T) {
	t.Parallel()
	info := DefaultBuildInfo()

	if info.Version != "dev" {
		t.Errorf("DefaultBuildInfo().Version = %q, want %q", info.Version, "dev")
	}
	if info.Commit != "unknown" {
		t.Errorf("DefaultBuildInfo().Commit = %q, want %q", info.Commit, "unknown")
	}
	if info.GoVersion == "" {
		t.Error("DefaultBuildInfo().GoVersion is empty")
	}
	if info.Platform == "" {
		t.Error("DefaultBuildInfo().Platform is empty")
	}
}

func TestReporter_Print_Quiet_NoIssues(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reporter := NewReporter(&buf, false, true) // quiet mode

	report := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{{Status: StatusPass}}},
		},
	}

	reporter.Print(report)

	if buf.String() != "" {
		t.Errorf("Quiet mode with no issues should produce no output, got: %q", buf.String())
	}
}

func TestReporter_Print_Quiet_WithIssues(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reporter := NewReporter(&buf, false, true) // quiet mode

	report := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{
				{Status: StatusFail, Label: "test", Message: "failed"},
				{Status: StatusWarn, Label: "warn", Message: "warning"},
				{Status: StatusNotFound, Label: "mod", Message: "missing", Fix: "install"},
				{Status: StatusNotFound, Label: "bare", Message: "optional"},
			}},
		},
	}

	reporter.Print(report)

	output := buf.String()
	if !strings.Contains(output, "Error: test: failed") {
		t.Errorf("Quiet mode should show error, got: %q", output)
	}
	if !strings.Contains(output, "Warning: warn: warning") {
		t.Errorf("Quiet mode should show warning, got: %q", output)
	}
	if !strings.Contains(output, "Missing: mod: missing") {
		t.Errorf("Quiet mode should show actionable not-found with Missing prefix, got: %q", output)
	}
	if strings.Contains(output, "bare") {
		t.Errorf("Quiet mode must not surface bare not-found (no Fix), got: %q", output)
	}
}

// TestReporter_Print_SummaryCountsMissing asserts the summary line lists
// the actionable not-found count alongside errors and warnings. Bare
// not-found results (no Fix) must not show up in the count — they remain
// informational.
func TestReporter_Print_SummaryCountsMissing(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reporter := NewReporter(&buf, false, false)

	report := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{
				{Status: StatusFail, Label: "f"},
				{Status: StatusWarn, Label: "w"},
				{Status: StatusNotFound, Label: "actionable", Fix: "install"},
				{Status: StatusNotFound, Label: "bare"},
			}},
		},
	}

	reporter.Print(report)

	output := buf.String()
	if !strings.Contains(output, "1 error") {
		t.Errorf("summary should report 1 error, got: %q", output)
	}
	if !strings.Contains(output, "1 warning") {
		t.Errorf("summary should report 1 warning, got: %q", output)
	}
	if !strings.Contains(output, "1 missing") {
		t.Errorf("summary should report 1 missing, got: %q", output)
	}
	// Ensure the segments compose with comma separators.
	if !strings.Contains(output, "1 error, 1 warning, 1 missing") {
		t.Errorf("summary segments should compose with ', ' separators, got: %q", output)
	}
	if strings.Contains(output, "2 missing") {
		t.Errorf("bare not-found must not inflate missing count, got: %q", output)
	}
}

// TestReporter_Print_SummaryOnlyMissing covers the edge case where the
// only issues are actionable not-founds. The summary line must still
// surface them rather than falling through to "No issues found".
func TestReporter_Print_SummaryOnlyMissing(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reporter := NewReporter(&buf, false, false)

	report := Report{
		Sections: []SectionResult{
			{Results: []CheckResult{
				{Status: StatusNotFound, Label: "mod", Fix: "install"},
			}},
		},
	}

	reporter.Print(report)

	output := buf.String()
	if strings.Contains(output, "No issues found") {
		t.Errorf("missing-only report must not be reported as healthy, got: %q", output)
	}
	if !strings.Contains(output, "1 missing found") {
		t.Errorf("missing-only summary should read '1 missing found', got: %q", output)
	}
}

func TestReporter_Print_Normal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reporter := NewReporter(&buf, false, false)

	report := Report{
		Sections: []SectionResult{
			{
				Name:    "Test Section",
				Summary: "2 items",
				Results: []CheckResult{
					{Status: StatusPass, Label: "item1", Message: "ok"},
					{Status: StatusFail, Label: "item2", Message: "bad"},
				},
			},
		},
	}

	reporter.Print(report)

	output := buf.String()
	if !strings.Contains(output, "start doctor") {
		t.Error("Output should contain header")
	}
	if !strings.Contains(output, "═") {
		t.Error("Output should contain unicode header line")
	}
	if !strings.Contains(output, "Test Section (2 items)") {
		t.Error("Output should contain section header with summary")
	}
	if !strings.Contains(output, "✓ item1") {
		t.Error("Output should contain pass symbol")
	}
	if !strings.Contains(output, "✗ item2") {
		t.Error("Output should contain fail symbol")
	}
	if !strings.Contains(output, "Summary") {
		t.Error("Output should contain summary section")
	}
}

func TestReporter_Print_NoIcons(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reporter := NewReporter(&buf, false, false)

	report := Report{
		Sections: []SectionResult{
			{
				Name:    "Version",
				NoIcons: true,
				Results: []CheckResult{
					{Status: StatusInfo, Label: "start dev"},
					{Status: StatusInfo, Label: "Commit", Message: "abc123"},
				},
			},
		},
	}

	reporter.Print(report)

	output := buf.String()
	if strings.Contains(output, "- start dev") {
		t.Error("NoIcons section should not show status symbols")
	}
	if !strings.Contains(output, "  start dev") {
		t.Error("NoIcons section should show label without symbol")
	}
	if !strings.Contains(output, "Commit:") {
		t.Error("NoIcons section should show label with colon for key-value")
	}
}

func TestReporter_Print_IndentAndNoIcon(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	reporter := NewReporter(&buf, false, false)

	report := Report{
		Sections: []SectionResult{
			{
				Name: "Configuration",
				Results: []CheckResult{
					{Status: StatusInfo, Label: "Global (~/.config/start)", NoIcon: true},
					{Status: StatusPass, Label: "agents.cue", Indent: 1},
					{Status: StatusPass, Label: "settings.cue", Indent: 1},
				},
			},
		},
	}

	reporter.Print(report)

	output := buf.String()
	// Header should appear without a status symbol
	if strings.Contains(output, "- Global") || strings.Contains(output, "✓ Global") {
		t.Error("NoIcon result should not show status symbol before label")
	}
	if !strings.Contains(output, "  Global (~/.config/start)") {
		t.Errorf("NoIcon result should show label at base indent, got:\n%s", output)
	}
	// Indented results should have extra indentation (4 spaces + symbol)
	if !strings.Contains(output, "    ✓ agents.cue") {
		t.Errorf("Indent=1 result should have 4-space indent, got:\n%s", output)
	}
	if !strings.Contains(output, "    ✓ settings.cue") {
		t.Errorf("Indent=1 result should have 4-space indent, got:\n%s", output)
	}
}
