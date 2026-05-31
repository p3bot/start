// Package doctor provides health check diagnostics for start configuration.
package doctor

import (
	"encoding/json"
	"io"
	"runtime"
)

// Status represents the result status of a check.
type Status int

const (
	StatusPass Status = iota
	StatusWarn
	StatusFail
	StatusInfo
	StatusNotFound
)

// String returns the string representation of a Status.
func (s Status) String() string {
	switch s {
	case StatusPass:
		return "pass"
	case StatusWarn:
		return "warn"
	case StatusFail:
		return "fail"
	case StatusInfo:
		return "info"
	case StatusNotFound:
		return "notfound"
	default:
		return "unknown"
	}
}

// MarshalJSON implements json.Marshaler for Status, emitting the string representation.
func (s Status) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Symbol returns the display symbol for a Status.
func (s Status) Symbol() string {
	switch s {
	case StatusPass:
		return "✓"
	case StatusWarn:
		return "⚠"
	case StatusFail:
		return "✗"
	case StatusInfo:
		return "-"
	case StatusNotFound:
		return "○"
	default:
		return "?"
	}
}

// CheckResult holds the result of a single check item.
type CheckResult struct {
	Status  Status   `json:"status"`
	Label   string   `json:"label"`
	Message string   `json:"message"`
	Fix     string   `json:"fix,omitempty"`
	Details []string `json:"details,omitempty"`
	Indent  int      `json:"-"`
	NoIcon  bool     `json:"-"`
}

// SectionResult holds the results for a check section.
type SectionResult struct {
	Name    string        `json:"name"`
	Results []CheckResult `json:"results"`
	Summary string        `json:"summary,omitempty"`
	NoIcons bool          `json:"-"`
}

// Report holds the complete diagnostic report.
type Report struct {
	Sections []SectionResult `json:"sections"`
}

// IsIssue reports whether a check result is an actionable issue.
// Failures and warnings always are. NotFound results are issues only when
// they carry a Fix string — bare NotFound (e.g. an optional local file
// the user has not created) stays informational and must not force a
// non-zero exit code.
func (c CheckResult) IsIssue() bool {
	switch c.Status {
	case StatusFail, StatusWarn:
		return true
	case StatusNotFound:
		return c.Fix != ""
	default:
		return false
	}
}

// HasIssues returns true if the report contains any actionable issues.
func (r Report) HasIssues() bool {
	for _, s := range r.Sections {
		for _, c := range s.Results {
			if c.IsIssue() {
				return true
			}
		}
	}
	return false
}

// Issues returns all actionable check results (see CheckResult.IsIssue).
func (r Report) Issues() []CheckResult {
	var issues []CheckResult
	for _, s := range r.Sections {
		for _, c := range s.Results {
			if c.IsIssue() {
				issues = append(issues, c)
			}
		}
	}
	return issues
}

// ErrorCount returns the number of failures in the report.
func (r Report) ErrorCount() int {
	count := 0
	for _, s := range r.Sections {
		for _, c := range s.Results {
			if c.Status == StatusFail {
				count++
			}
		}
	}
	return count
}

// WarnCount returns the number of warnings in the report.
func (r Report) WarnCount() int {
	count := 0
	for _, s := range r.Sections {
		for _, c := range s.Results {
			if c.Status == StatusWarn {
				count++
			}
		}
	}
	return count
}

// MissingCount returns the number of actionable missing-artifact results
// (StatusNotFound with a non-empty Fix). Bare NotFound results are
// excluded so optional local files do not inflate the count.
func (r Report) MissingCount() int {
	count := 0
	for _, s := range r.Sections {
		for _, c := range s.Results {
			if c.Status == StatusNotFound && c.Fix != "" {
				count++
			}
		}
	}
	return count
}

// BuildInfo holds version and build information.
type BuildInfo struct {
	Version      string
	Commit       string
	BuildDate    string
	GoVersion    string
	Platform     string
	IndexVersion string // Registry index version (empty if unavailable)
	IndexPath    string // Configured library_index path (empty if using built-in default)
}

// DefaultBuildInfo returns build info with runtime defaults.
func DefaultBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   "dev",
		Commit:    "unknown",
		BuildDate: "unknown",
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
}

// Options configures the doctor run.
type Options struct {
	Verbose bool
	Quiet   bool
	Stdout  io.Writer
	Stderr  io.Writer
}

// RepoURL is the repository URL for the project.
const RepoURL = "https://github.com/start-cli/start"

// IssuesURL is the issues URL for the project.
const IssuesURL = "https://github.com/start-cli/start/issues"
