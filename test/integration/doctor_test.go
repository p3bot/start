//go:build integration

package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/doctor"
)

// setupDoctorTestConfig creates a temporary directory with a valid CUE config
// suitable for doctor testing.
func setupDoctorTestConfig(t *testing.T) (string, config.Paths) {
	t.Helper()
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Create a valid config with echo agent (available on all systems)
	configContent := `
agents: {
	echo: {
		bin: "echo"
		command: "{{.bin}} '{{.prompt}}'"
		default_model: "default"
		models: {
			default: "echo-model"
		}
	}
}

roles: {
	assistant: {
		prompt: "You are a helpful assistant."
	}
}

contexts: {
	env: {
		required: true
		prompt: "Environment context"
	}
}

settings: {
	default_agent: "echo"
}
`
	configFile := filepath.Join(configDir, "settings.cue")
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	paths := config.Paths{
		Global:       filepath.Join(tmpDir, "global"), // Non-existent global
		Local:        configDir,
		GlobalExists: false,
		LocalExists:  true,
	}

	return tmpDir, paths
}

// Note: Tests below use os.Chdir (process-global state). Do not add t.Parallel()
// to any test that calls os.Chdir — it will cause data races on the working directory.

func TestDoctor_ValidConfig_AllPass(t *testing.T) {
	tmpDir, paths := setupDoctorTestConfig(t)

	// Change to temp directory
	chdir(t, tmpDir)

	// Build the report manually (simulating what the CLI does)
	report := runDoctorChecks(t, paths)

	// Check that we have sections
	if len(report.Sections) == 0 {
		t.Fatal("report should have sections")
	}

	// Configuration check should pass
	var hasConfigSection bool
	for _, section := range report.Sections {
		if section.Name == "Configuration" {
			hasConfigSection = true
			hasValidation := false
			for _, result := range section.Results {
				if result.Label == "Validation" && result.Status == doctor.StatusPass {
					hasValidation = true
				}
			}
			if !hasValidation {
				t.Error("configuration validation should pass")
			}
		}
	}
	if !hasConfigSection {
		t.Error("report should have Configuration section")
	}

	// Agent check should pass (echo is available)
	var hasAgentSection bool
	for _, section := range report.Sections {
		if section.Name == "Agents" {
			hasAgentSection = true
			hasEcho := false
			for _, result := range section.Results {
				if result.Label == "echo" && result.Status == doctor.StatusPass {
					hasEcho = true
				}
			}
			if !hasEcho {
				t.Error("echo agent should be found")
			}
		}
	}
	if !hasAgentSection {
		t.Error("report should have Agents section")
	}
}

func TestDoctor_InvalidCUESyntax_ReportsError(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Write invalid CUE syntax
	invalidCUE := `this is not valid { cue syntax {{{{`
	if err := os.WriteFile(filepath.Join(configDir, "bad.cue"), []byte(invalidCUE), 0644); err != nil {
		t.Fatalf("writing invalid config: %v", err)
	}

	paths := config.Paths{
		Global:       filepath.Join(tmpDir, "global"),
		Local:        configDir,
		GlobalExists: false,
		LocalExists:  true,
	}

	// Run configuration check
	section := doctor.CheckConfiguration(paths)

	// Should have a failure
	hasFail := false
	for _, result := range section.Results {
		if result.Status == doctor.StatusFail {
			hasFail = true
		}
	}

	if !hasFail {
		t.Error("invalid CUE syntax should result in failure")
	}
}

func TestDoctor_MissingAgentBinary_ReportsError(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Config with non-existent binary
	configContent := `
agents: {
	nonexistent: {
		bin: "this-binary-does-not-exist-12345"
		command: "{{.bin}}"
		default_model: "default"
		models: { default: "model" }
	}
}
settings: { default_agent: "nonexistent" }
`
	if err := os.WriteFile(filepath.Join(configDir, "agents.cue"), []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Load config
	loader := internalcue.NewLoader()
	cfgValue, err := loader.LoadSingle(configDir)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	// Run agent check
	section := doctor.CheckAgents(cfgValue)

	// Should have a failure for missing binary
	hasFail := false
	var failLabel string
	for _, r := range section.Results {
		if r.Status == doctor.StatusFail {
			hasFail = true
			failLabel = r.Label
		}
	}

	if !hasFail {
		t.Error("missing agent binary should result in failure")
	}
	if failLabel != "nonexistent" {
		t.Errorf("failure should be for 'nonexistent' agent, got %q", failLabel)
	}
}

func TestDoctor_MissingContextFile_ReportsIssue(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Config with context referencing non-existent file
	configContent := `
contexts: {
	missing: {
		required: true
		file: "/this/file/does/not/exist.md"
	}
	optional_missing: {
		required: false
		file: "/another/missing/file.md"
	}
}
`
	if err := os.WriteFile(filepath.Join(configDir, "contexts.cue"), []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Load config
	loader := internalcue.NewLoader()
	cfgValue, err := loader.LoadSingle(configDir)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	// Run context check
	section := doctor.CheckContexts(cfgValue)

	// Should have failures/warnings for missing files
	var hasFail, hasWarn bool
	for _, r := range section.Results {
		if r.Status == doctor.StatusFail && r.Label == "missing" {
			hasFail = true
		}
		if r.Status == doctor.StatusWarn && r.Label == "optional_missing" {
			hasWarn = true
		}
	}

	if !hasFail {
		t.Error("required missing context file should result in failure")
	}
	if !hasWarn {
		t.Error("optional missing context file should result in warning")
	}
}

func TestDoctor_MissingRoleFile_ReportsError(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".start")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	// Config with role referencing non-existent file
	configContent := `
roles: {
	missing_role: {
		file: "/this/role/does/not/exist.md"
	}
}
`
	if err := os.WriteFile(filepath.Join(configDir, "roles.cue"), []byte(configContent), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	// Load config
	loader := internalcue.NewLoader()
	cfgValue, err := loader.LoadSingle(configDir)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	// Run role check
	section := doctor.CheckRoles(cfgValue)

	// Should have a failure for missing file
	hasFail := false
	for _, r := range section.Results {
		if r.Status == doctor.StatusFail && r.Label == "missing_role" {
			hasFail = true
		}
	}

	if !hasFail {
		t.Error("missing role file should result in failure")
	}
}

func TestDoctor_QuietMode_NoOutputOnSuccess(t *testing.T) {
	// Create a healthy report
	report := doctor.Report{
		Sections: []doctor.SectionResult{
			{
				Name: "Test",
				Results: []doctor.CheckResult{
					{Status: doctor.StatusPass, Label: "item", Message: "ok"},
				},
			},
		},
	}

	var buf bytes.Buffer
	reporter := doctor.NewReporter(&buf, false, true) // quiet mode
	reporter.Print(report)

	if buf.String() != "" {
		t.Errorf("quiet mode with no issues should produce no output, got: %q", buf.String())
	}
}

func TestDoctor_QuietMode_OutputOnFailure(t *testing.T) {
	// Create a report with issues
	report := doctor.Report{
		Sections: []doctor.SectionResult{
			{
				Name: "Test",
				Results: []doctor.CheckResult{
					{Status: doctor.StatusFail, Label: "agent", Message: "NOT FOUND"},
					{Status: doctor.StatusWarn, Label: "context", Message: "missing"},
				},
			},
		},
	}

	var buf bytes.Buffer
	reporter := doctor.NewReporter(&buf, false, true) // quiet mode
	reporter.Print(report)

	output := buf.String()
	if !strings.Contains(output, "Error: agent: NOT FOUND") {
		t.Errorf("quiet mode should show errors, got: %q", output)
	}
	if !strings.Contains(output, "Warning: context: missing") {
		t.Errorf("quiet mode should show warnings, got: %q", output)
	}
}

func TestDoctor_ExitCode_HealthyReturnsZero(t *testing.T) {
	report := doctor.Report{
		Sections: []doctor.SectionResult{
			{
				Name: "Test",
				Results: []doctor.CheckResult{
					{Status: doctor.StatusPass, Label: "ok"},
					{Status: doctor.StatusInfo, Label: "info"},
				},
			},
		},
	}

	if report.HasIssues() {
		t.Error("healthy report should not have issues (exit code 0)")
	}
}

func TestDoctor_ExitCode_WarningReturnsOne(t *testing.T) {
	report := doctor.Report{
		Sections: []doctor.SectionResult{
			{
				Name: "Test",
				Results: []doctor.CheckResult{
					{Status: doctor.StatusPass, Label: "ok"},
					{Status: doctor.StatusWarn, Label: "warning"},
				},
			},
		},
	}

	if !report.HasIssues() {
		t.Error("report with warning should have issues (exit code 1)")
	}
}

func TestDoctor_ExitCode_ErrorReturnsOne(t *testing.T) {
	report := doctor.Report{
		Sections: []doctor.SectionResult{
			{
				Name: "Test",
				Results: []doctor.CheckResult{
					{Status: doctor.StatusPass, Label: "ok"},
					{Status: doctor.StatusFail, Label: "error"},
				},
			},
		},
	}

	if !report.HasIssues() {
		t.Error("report with error should have issues (exit code 1)")
	}
}

func TestDoctor_FullReport_Integration(t *testing.T) {
	tmpDir, paths := setupDoctorTestConfig(t)

	// Change to temp directory
	chdir(t, tmpDir)

	// Build full report
	report := runDoctorChecks(t, paths)

	// Output the report
	var buf bytes.Buffer
	reporter := doctor.NewReporter(&buf, false, false)
	reporter.Print(report)

	output := buf.String()

	// Verify expected sections are present
	expectedSections := []string{
		"Repository",
		"Version",
		"Configuration",
		"Agents",
		"Contexts",
		"Roles",
		"Environment",
		"Summary",
	}

	for _, section := range expectedSections {
		if !strings.Contains(output, section) {
			t.Errorf("output should contain %q section", section)
		}
	}

	// Verify header
	if !strings.Contains(output, "start doctor") {
		t.Error("output should contain 'start doctor' header")
	}

	// Verify unicode separator
	if !strings.Contains(output, "═") {
		t.Error("output should contain header separator")
	}
}

func TestDoctor_VerboseMode_ShowsDetails(t *testing.T) {
	// Create a report with details
	report := doctor.Report{
		Sections: []doctor.SectionResult{
			{
				Name: "Test",
				Results: []doctor.CheckResult{
					{
						Status:  doctor.StatusPass,
						Label:   "item",
						Message: "ok",
						Details: []string{"detail line 1", "detail line 2"},
					},
				},
			},
		},
	}

	// Normal mode - no details
	var normalBuf bytes.Buffer
	normalReporter := doctor.NewReporter(&normalBuf, false, false)
	normalReporter.Print(report)

	if strings.Contains(normalBuf.String(), "detail line 1") {
		t.Error("normal mode should not show details")
	}

	// Verbose mode - shows details
	var verboseBuf bytes.Buffer
	verboseReporter := doctor.NewReporter(&verboseBuf, true, false)
	verboseReporter.Print(report)

	if !strings.Contains(verboseBuf.String(), "detail line 1") {
		t.Error("verbose mode should show details")
	}
	if !strings.Contains(verboseBuf.String(), "detail line 2") {
		t.Error("verbose mode should show all detail lines")
	}
}

func TestDoctor_Environment_WritableDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("creating config dir: %v", err)
	}

	paths := config.Paths{
		Global:       configDir,
		Local:        filepath.Join(tmpDir, "local"),
		GlobalExists: true,
		LocalExists:  false,
	}

	section := doctor.CheckEnvironment(paths)

	// Config directory should be writable
	hasWritable := false
	for _, r := range section.Results {
		if r.Label == "Config directory" && r.Status == doctor.StatusPass {
			hasWritable = true
		}
	}

	if !hasWritable {
		t.Error("config directory should be marked as writable")
	}
}

func TestDoctor_SummarySection_CountsIssues(t *testing.T) {
	report := doctor.Report{
		Sections: []doctor.SectionResult{
			{
				Name: "Test1",
				Results: []doctor.CheckResult{
					{Status: doctor.StatusFail, Label: "err1"},
					{Status: doctor.StatusFail, Label: "err2"},
				},
			},
			{
				Name: "Test2",
				Results: []doctor.CheckResult{
					{Status: doctor.StatusWarn, Label: "warn1"},
					{Status: doctor.StatusPass, Label: "ok"},
				},
			},
		},
	}

	var buf bytes.Buffer
	reporter := doctor.NewReporter(&buf, false, false)
	reporter.Print(report)

	output := buf.String()

	// Should show correct counts
	if !strings.Contains(output, "2 errors") {
		t.Error("summary should show '2 errors'")
	}
	if !strings.Contains(output, "1 warning") {
		t.Error("summary should show '1 warning'")
	}
}

// runDoctorChecks runs all doctor checks and builds a report.
func runDoctorChecks(t *testing.T, paths config.Paths) doctor.Report {
	t.Helper()
	var report doctor.Report

	// Intro section
	report.Sections = append(report.Sections, doctor.CheckIntro())

	// Version section
	buildInfo := doctor.DefaultBuildInfo()
	report.Sections = append(report.Sections, doctor.CheckVersion(buildInfo))

	// Configuration section
	report.Sections = append(report.Sections, doctor.CheckConfiguration(paths))

	// Load config for remaining checks
	var cfgLoaded bool
	var cfgResult internalcue.LoadResult

	if paths.AnyExists() {
		loader := internalcue.NewLoader()
		dirs := paths.ForScope(config.ScopeMerged)
		if len(dirs) > 0 {
			var err error
			cfgResult, err = loader.Load(dirs)
			if err == nil {
				cfgLoaded = true
			}
		}
	}

	// Agent checks
	if cfgLoaded {
		report.Sections = append(report.Sections, doctor.CheckAgents(cfgResult.Value))
	} else {
		report.Sections = append(report.Sections, doctor.SectionResult{
			Name: "Agents",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "no valid config"},
			},
		})
	}

	// Role checks
	if cfgLoaded {
		report.Sections = append(report.Sections, doctor.CheckRoles(cfgResult.Value))
	} else {
		report.Sections = append(report.Sections, doctor.SectionResult{
			Name: "Roles",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "no valid config"},
			},
		})
	}

	// Context checks
	if cfgLoaded {
		report.Sections = append(report.Sections, doctor.CheckContexts(cfgResult.Value))
	} else {
		report.Sections = append(report.Sections, doctor.SectionResult{
			Name: "Contexts",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "no valid config"},
			},
		})
	}

	// Environment checks
	report.Sections = append(report.Sections, doctor.CheckEnvironment(paths))

	return report
}
