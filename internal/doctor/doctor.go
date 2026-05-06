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
	Label   string   `json:"label"`             // Short label (e.g., "claude", "agents.cue")
	Message string   `json:"message"`           // Detail message (e.g., "/usr/local/bin/claude", "NOT FOUND")
	Fix     string   `json:"fix,omitempty"`     // Suggested fix action
	Details []string `json:"details,omitempty"` // Additional details for verbose mode
	Indent  int      `json:"-"`                 // Additional indentation level (0 = normal, 1+ = nested)
	NoIcon  bool     `json:"-"`                 // Suppress status icon for this result
}

// SectionResult holds the results for a check section.
type SectionResult struct {
	Name    string        `json:"name"`
	Results []CheckResult `json:"results"`
	Summary string        `json:"summary,omitempty"` // Optional summary (e.g., "2 configured")
	NoIcons bool          `json:"-"`                 // If true, don't show status icons (for info-only sections)
}

// Report holds the complete diagnostic report.
type Report struct {
	Sections []SectionResult `json:"sections"`
}

// HasIssues returns true if the report contains any failures or warnings.
func (r Report) HasIssues() bool {
	for _, s := range r.Sections {
		for _, c := range s.Results {
			if c.Status == StatusFail || c.Status == StatusWarn {
				return true
			}
		}
	}
	return false
}

// Issues returns all check results that are failures or warnings.
func (r Report) Issues() []CheckResult {
	var issues []CheckResult
	for _, s := range r.Sections {
		for _, c := range s.Results {
			if c.Status == StatusFail || c.Status == StatusWarn {
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

// BuildInfo holds version and build information.
type BuildInfo struct {
	Version      string
	Commit       string
	BuildDate    string
	GoVersion    string
	Platform     string
	IndexVersion string // Registry index version (empty if unavailable)
	IndexPath    string // Configured assets_index path (empty if using built-in default)
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
const RepoURL = "https://github.com/grantcarthew/start"

// IssuesURL is the issues URL for the project.
const IssuesURL = "https://github.com/start-cli/start/issues"
