package cli

import (
	"context"
	"fmt"
	"time"

	"cuelang.org/go/cue"
	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/assets"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/config"
	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/doctor"
	"github.com/start-cli/start/internal/registry"
)

// addDoctorCommand adds the doctor command to the parent command.
func addDoctorCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "doctor",
		GroupID: "utilities",
		Short:   "Diagnose start installation and configuration",
		Long: `Check start installation, configuration, and environment for issues.
Reports warnings and suggestions for any problems found.

Checks performed:
  - Version and build information
  - Configuration file validation (CUE syntax)
  - Schema validation (fetched from registry)
  - Agent binary availability
  - Context and role file existence
  - Environment (directory permissions)

Exit codes:
  0 - All checks passed
  1 - Issues found`,
		RunE: runDoctor,
	}

	cmd.Flags().Bool("json", false, "Output as JSON")
	parent.AddCommand(cmd)
}

// runDoctor executes the doctor command.
func runDoctor(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	report, err := prepareDoctor()
	if err != nil {
		return err
	}

	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		if err := writeJSON(cmd.OutOrStdout(), report); err != nil {
			return fmt.Errorf("marshalling doctor report: %w", err)
		}
		if report.HasIssues() {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			return errDoctorIssuesFound
		}
		return nil
	}

	flags := getFlags(cmd)
	reporter := doctor.NewReporter(cmd.OutOrStdout(), flags.Verbose, flags.Quiet)
	reporter.Print(report)

	if report.HasIssues() {
		// Return a silent error to set exit code 1
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		return errDoctorIssuesFound
	}

	return nil
}

// errDoctorIssuesFound is returned when doctor finds issues.
// It implements SilentError so main.go skips printing it.
var errDoctorIssuesFound = &doctorError{}

type doctorError struct{}

func (e *doctorError) Error() string { return "issues found" }
func (e *doctorError) Silent() bool  { return true }

// prepareDoctor runs all checks and builds the report.
func prepareDoctor() (doctor.Report, error) {
	var report doctor.Report

	// Intro section
	report.Sections = append(report.Sections, doctor.CheckIntro())

	// Version section
	indexPath := resolveAssetsIndexPath()
	buildInfo := doctor.BuildInfo{
		Version:      cliVersion,
		Commit:       commit,
		BuildDate:    buildDate,
		GoVersion:    doctor.DefaultBuildInfo().GoVersion,
		Platform:     doctor.DefaultBuildInfo().Platform,
		IndexVersion: resolveIndexVersion(indexPath),
		IndexPath:    indexPath,
	}
	report.Sections = append(report.Sections, doctor.CheckVersion(buildInfo))

	// Cache section
	report.Sections = append(report.Sections, doctor.CheckCache())

	// Configuration section
	paths, err := config.ResolvePaths("")
	if err != nil {
		return report, err
	}
	report.Sections = append(report.Sections, doctor.CheckConfiguration(paths))

	// Schema validation section
	report.Sections = append(report.Sections, fetchAndValidateSchemas(paths))

	// Load config for remaining checks (if possible)
	var cfgLoaded bool
	var cfgResult internalcue.LoadResult

	if paths.AnyExists() {
		loader := internalcue.NewLoader()
		dirs := paths.ForScope(config.ScopeMerged)
		if len(dirs) > 0 {
			cfgResult, err = loader.Load(dirs)
			if err == nil {
				cfgLoaded = true
			}
		}
	}

	// Settings checks always run (reads settings.cue directly via config paths)
	var settingsCfg cue.Value
	if cfgLoaded {
		settingsCfg = cfgResult.Value
	}
	report.Sections = append(report.Sections, doctor.CheckSettings(paths, settingsCfg))

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

	// Task checks
	if cfgLoaded {
		report.Sections = append(report.Sections, doctor.CheckTasks(cfgResult.Value))
	} else {
		report.Sections = append(report.Sections, doctor.SectionResult{
			Name: "Tasks",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "no valid config"},
			},
		})
	}

	// Environment checks
	report.Sections = append(report.Sections, doctor.CheckEnvironment(paths))

	return report, nil
}

// fetchAndValidateSchemas fetches schemas from the registry and validates config files.
func fetchAndValidateSchemas(paths config.Paths) doctor.SectionResult {
	if !paths.AnyExists() {
		return doctor.SectionResult{
			Name: "Schema Validation",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "no config directories"},
			},
		}
	}

	client, err := registry.NewClient()
	if err != nil {
		return doctor.SectionResult{
			Name: "Schema Validation",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "registry unavailable"},
			},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resolvedPath, err := client.ResolveLatestVersion(ctx, registry.SchemaModulePath)
	if err != nil {
		return doctor.SectionResult{
			Name: "Schema Validation",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "cannot resolve schema version"},
			},
		}
	}

	result, err := client.Fetch(ctx, resolvedPath)
	if err != nil {
		return doctor.SectionResult{
			Name: "Schema Validation",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: "cannot fetch schemas"},
			},
		}
	}

	schemas, err := doctor.LoadSchemas(result.SourceDir, client.Registry())
	if err != nil {
		return doctor.SectionResult{
			Name: "Schema Validation",
			Results: []doctor.CheckResult{
				{Status: doctor.StatusInfo, Label: "Skipped", Message: fmt.Sprintf("cannot load schemas: %v", err)},
			},
		}
	}

	return doctor.CheckSchemaValidation(paths, schemas)
}

// resolveIndexVersion returns the latest index version string (e.g., "v0.3.2").
// Reads from cache first; falls back to a registry network call if cache is missing.
func resolveIndexVersion(indexPath string) string {
	// Try cache first to avoid a network call.
	cached, err := cache.ReadIndex()
	if err == nil && cached.Version != "" {
		return assets.VersionFromOrigin(cached.Version)
	}

	// Fall back to registry query.
	client, err := registry.NewClient()
	if err != nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resolved, err := client.ResolveLatestVersion(ctx, registry.EffectiveIndexPath(indexPath))
	if err != nil {
		return ""
	}

	return assets.VersionFromOrigin(resolved)
}
