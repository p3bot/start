package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/config"
	"github.com/start-cli/start/internal/doctor"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
	"golang.org/x/mod/semver"
)

// validateModuleStatus is the outcome of a per-module check.
type validateModuleStatus int

const (
	validateModulePass validateModuleStatus = iota
	validateModuleFail
)

// validateModuleResult holds the outcome for a single module.
type validateModuleResult struct {
	name    string               // module name within category (e.g. "claude", "review/architecture")
	version string               // version from the index entry
	status  validateModuleStatus // overall module status
	issues  []string             // descriptions of any problems found
}

// validateCatResult holds results for one category.
type validateCatResult struct {
	name    string
	modules []validateModuleResult
}

// ValidateResult is the top-level JSON output for doctor validate.
type ValidateResult struct {
	Index      ValidateIndexResult      `json:"index"`
	Categories []ValidateCategoryResult `json:"categories"`
	Stats      ValidateStatsResult      `json:"stats"`
}

// ValidateIndexResult holds the index validation checks for JSON output.
type ValidateIndexResult struct {
	Checks []ValidateCheckResult `json:"checks"`
}

// ValidateCheckResult mirrors doctor.CheckResult for JSON output.
type ValidateCheckResult struct {
	Status  string `json:"status"`
	Label   string `json:"label"`
	Message string `json:"message,omitempty"`
}

// ValidateCategoryResult holds per-category module results for JSON output.
type ValidateCategoryResult struct {
	Name    string                 `json:"name"`
	Modules []ValidateModuleResult `json:"modules"`
}

// ValidateModuleResult holds per-module validation results for JSON output.
type ValidateModuleResult struct {
	Name    string   `json:"name"`
	Version string   `json:"version,omitempty"`
	Status  string   `json:"status"`
	Issues  []string `json:"issues,omitempty"`
}

// ValidateStatsResult holds summary statistics for JSON output.
type ValidateStatsResult struct {
	Checked int `json:"checked"`
	Pass    int `json:"pass"`
	Fail    int `json:"fail"`
}

// validateError is a silent exit-code-1 error for validation failures.
type validateError struct{}

func (e *validateError) Error() string { return "validation issues found" }
func (e *validateError) Silent() bool  { return true }

// addDoctorValidateCommand registers the validate subcommand under doctor.
func addDoctorValidateCommand(parent *cobra.Command) {
	cmd := &cobra.Command{
		Use:     "validate",
		Aliases: []string{"verify", "check"},
		Short:   "Validate index and module version consistency",
		Long: `Check that git tags, CUE registry published versions, and index version
fields are consistent with each other.

Clones the modules repository to cache and checks each module for:
  - Version drift between index, registry, and git tags
  - Modules in the filesystem with no index entry
  - Content changes since the last published tag

Exit codes:
  0 - All checks passed
  1 - Issues found`,
		Args: noArgsOrHelp,
		RunE: runDoctorValidate,
	}
	cmd.Flags().Bool("yes", false, "Confirm intent to run network checks")
	_ = cmd.Flags().MarkHidden("yes")
	cmd.Flags().Bool("json", false, "Output as JSON")
	parent.AddCommand(cmd)
}

// runDoctorValidate executes the validate command.
func runDoctorValidate(cmd *cobra.Command, args []string) error {
	if shown, err := checkHelpArg(cmd, args); shown || err != nil {
		return err
	}
	ctx := context.Background()
	flags := getFlags(cmd)
	w := cmd.OutOrStdout()
	prog := tui.NewProgress(cmd.ErrOrStderr(), flags.Quiet)
	defer prog.Done() // safety net: clears any active progress line on early return

	// Gate: --yes is required to prevent casual traffic against public infrastructure.
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "The 'start doctor validate' command is a maintainer tool for checking")
		fmt.Fprintln(w, "consistency between git tags, the CUE registry, and the modules index.")
		fmt.Fprintln(w, "It makes significant network requests against public infrastructure.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Running it will:")
		fmt.Fprintln(w, "  - Clone or pull the modules repository from GitHub")
		fmt.Fprintln(w, "  - Fetch all git tags from origin")
		fmt.Fprintln(w, "  - Query the CUE registry for each published module")
		fmt.Fprintln(w)
		tui.ColorHiYellow.Fprintln(w, "A freely accessible public registry is a shared resource — don't be that person.")
		fmt.Fprintln(w)
		tui.ColorDim.Fprintln(w, "Run with --yes to proceed.")
		return nil
	}

	// Prerequisite 1: git in PATH
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git not found in PATH: install git and retry")
	}

	// Prerequisite 2: read library_index setting
	indexPath := registry.EffectiveIndexPath(resolveLibraryIndexPath())

	// Prerequisite 3: derive git repo URL
	cloneURL, err := validateDeriveRepoURL(indexPath)
	if err != nil {
		return err
	}

	// Prerequisite 4: check network reachability
	prog.Update("Checking network...")
	if err := validateCheckNetwork(cloneURL); err != nil {
		return err
	}

	// Prerequisite 5: clone or pull
	cacheDir, err := validateCacheDir(cloneURL)
	if err != nil {
		return fmt.Errorf("resolving cache directory: %w", err)
	}
	prog.Update("Syncing repository...")
	if err := validateEnsureRepo(cloneURL, cacheDir); err != nil {
		return err
	}

	// Prerequisite 6: verify repo state
	if err := validateVerifyRepo(cacheDir); err != nil {
		return err
	}

	// Prerequisite 7: fetch tags
	prog.Update("Fetching tags...")
	if err := validateFetchTags(cacheDir); err != nil {
		return err
	}

	// Collect all git tags for later checks
	tags, err := validateListTags(cacheDir)
	if err != nil {
		return fmt.Errorf("listing git tags: %w", err)
	}

	// Create registry client
	client, err := getProvider(cmd)()
	if err != nil {
		return fmt.Errorf("creating registry client: %w", err)
	}

	jsonFlag, _ := cmd.Flags().GetBool("json")

	// Section 1: Index validation
	prog.Update("Fetching index...")
	indexSection, idx, fatal := validateIndex(ctx, client, indexPath, tags)
	prog.Done() // clear before printing section output

	if !jsonFlag {
		printValidateIndexSection(w, indexSection)
	}
	if fatal {
		if jsonFlag {
			if err := outputValidateJSON(w, indexSection, nil); err != nil {
				return err
			}
		}
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		return &validateError{}
	}
	if idx == nil {
		if jsonFlag {
			return outputValidateJSON(w, indexSection, nil)
		}
		return nil
	}

	// Section 2: Mismatch checks with progress
	// Note: total counts only indexed modules. Filesystem orphans discovered during
	// validateModules are appended after the progress counter reaches 100%, so the
	// final displayed module count may exceed total if orphans exist.
	total := indexEntryCount(idx)
	done := 0
	onModule := func() {
		done++
		pct := done * 100 / total
		prog.Update("Checking modules %d/%d (%d%%)", done, total, pct)
	}
	cats := validateModules(ctx, client, idx, tags, cacheDir, onModule)
	prog.Done() // clear before printing section output

	if jsonFlag {
		hasFailure := validateHasFailure(cats)
		if err := outputValidateJSON(w, indexSection, cats); err != nil {
			return err
		}
		if hasFailure {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			return &validateError{}
		}
		return nil
	}

	printValidateModules(w, cats, flags.Verbose)

	// Section 3: Statistics
	hasFailure := printValidateStats(w, cats)
	if hasFailure {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		return &validateError{}
	}
	return nil
}

// validateDeriveRepoURL converts an index module path to a GitHub HTTPS repo URL.
// e.g. "github.com/start-cli/library/index@v1" → "https://github.com/start-cli/library"
// Returns an error if the path does not end with the "/index" subpath convention.
func validateDeriveRepoURL(indexModulePath string) (string, error) {
	path := indexModulePath
	// Strip version suffix
	if idx := strings.LastIndex(path, "@"); idx != -1 {
		path = path[:idx]
	}
	// Require and strip /index suffix
	if !strings.HasSuffix(path, "/index") {
		return "", fmt.Errorf("doctor validate requires an index path ending with /index (got %q); custom subpaths are not supported", path)
	}
	path = strings.TrimSuffix(path, "/index")
	return "https://" + path, nil
}

// validateCheckNetwork confirms the git repo host is reachable and the repo exists.
func validateCheckNetwork(repoURL string) error {
	client := &http.Client{Timeout: 8 * time.Second}

	resp, err := client.Head(repoURL)
	if err != nil {
		return fmt.Errorf("network check failed: cannot reach %s: %w", repoURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("network check failed: %s returned HTTP %d", repoURL, resp.StatusCode)
	}

	return nil
}

// validateCacheDir returns the path for the modules git clone cache.
// The directory name is derived from the repo URL so different index sources
// get separate cache directories.
func validateCacheDir(repoURL string) (string, error) {
	paths, err := config.ResolvePaths("")
	if err != nil {
		return "", err
	}
	return filepath.Join(paths.Global, "cache", validateCacheDirName(repoURL)), nil
}

// validateCacheDirName derives a filesystem-safe cache directory name from a repo URL.
// e.g. "https://github.com/start-cli/library" → "start-cli-library"
func validateCacheDirName(repoURL string) string {
	// Strip scheme
	path := strings.TrimPrefix(repoURL, "https://")
	path = strings.TrimPrefix(path, "http://")
	// Split on "/" and take the last two segments (owner + repo)
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "-" + parts[len(parts)-1]
	}
	if len(parts) == 1 && parts[0] != "" {
		return parts[0]
	}
	return "modules-cache"
}

// defaultLibraryBranch is the expected default branch of the modules repository.
// Custom modules repositories on a different default branch are not supported.
const defaultLibraryBranch = "main"

// validateEnsureRepo clones the repo if absent, otherwise fetches and resets to
// origin/main. Using fetch+reset instead of pull avoids divergent branch errors
// since the cache is not a working repository.
// If the cache directory exists but has no .git (e.g. a stale failed clone),
// it is removed and re-cloned, provided it is safely scoped under cacheParent.
func validateEnsureRepo(repoURL, cacheDir string) error {
	if _, err := os.Stat(filepath.Join(cacheDir, ".git")); os.IsNotExist(err) {
		// Guard: only remove if cacheDir is under its expected parent.
		// This prevents path traversal from a malformed cache directory path.
		// filepath.Rel(parent, cacheDir) always yields the final path component
		// (never a multi-segment result) because parent == filepath.Dir(cacheDir),
		// so exact matches on ".." and "." are sufficient here.
		parent := filepath.Dir(cacheDir)
		if rel, err := filepath.Rel(parent, cacheDir); err != nil || rel == ".." || rel == "." {
			return fmt.Errorf("refusing to remove unsafe cache path: %s", cacheDir)
		}
		// Remove any stale directory before cloning (no-op if it doesn't exist).
		if err := os.RemoveAll(cacheDir); err != nil {
			return fmt.Errorf("clearing stale cache directory: %w", err)
		}
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("creating cache directory: %w", err)
		}
		out, err := exec.Command("git", "clone", repoURL, cacheDir).CombinedOutput()
		if err != nil {
			return fmt.Errorf("cloning %s into %s: %s", repoURL, cacheDir, strings.TrimSpace(string(out)))
		}
		return nil
	}
	// Already cloned — reset to match remote default branch.
	// Using fetch + reset avoids merge strategy issues (divergent branches)
	// that occur with "git pull" on a validation cache.
	if out, err := exec.Command("git", "-C", cacheDir, "checkout", defaultLibraryBranch).CombinedOutput(); err != nil {
		return fmt.Errorf("checking out %s in %s: %s", defaultLibraryBranch, cacheDir, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", cacheDir, "fetch", "origin", defaultLibraryBranch).CombinedOutput(); err != nil {
		return fmt.Errorf("fetching %s from %s (cache: %s): %s", defaultLibraryBranch, repoURL, cacheDir, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("git", "-C", cacheDir, "reset", "--hard", "origin/"+defaultLibraryBranch).CombinedOutput(); err != nil {
		return fmt.Errorf("resetting to origin/%s in %s: %s", defaultLibraryBranch, cacheDir, strings.TrimSpace(string(out)))
	}
	return nil
}

// validateVerifyRepo checks that the clone is on main, clean, and up to date.
func validateVerifyRepo(cacheDir string) error {
	// Must be on main
	out, err := exec.Command("git", "-C", cacheDir, "branch", "--show-current").Output()
	if err != nil {
		return fmt.Errorf("checking current branch: %w", err)
	}
	if branch := strings.TrimSpace(string(out)); branch != defaultLibraryBranch {
		return fmt.Errorf("expected branch %s, got %q", defaultLibraryBranch, branch)
	}

	// Must have no uncommitted changes
	out, err = exec.Command("git", "-C", cacheDir, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("checking repo status: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("repository has uncommitted changes")
	}

	return nil
}

// validateFetchTags fetches all remote tags into the local clone.
// Using an explicit refspec with --force ensures new and updated tags are
// always written, which --tags alone may miss for recently added refs.
func validateFetchTags(cacheDir string) error {
	out, err := exec.Command("git", "-C", cacheDir, "fetch", "--force", "origin", "refs/tags/*:refs/tags/*").CombinedOutput()
	if err != nil {
		return fmt.Errorf("fetching tags from %s (cache: %s): %s", "origin", cacheDir, strings.TrimSpace(string(out)))
	}
	return nil
}

// validateListTags returns all git tags from the local clone.
func validateListTags(cacheDir string) ([]string, error) {
	out, err := exec.Command("git", "-C", cacheDir, "tag", "--list").Output()
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	var tags []string
	for t := range strings.SplitSeq(string(out), "\n") {
		if t := strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	return tags, nil
}

// validateTagVersions returns all semver versions from tags with the given prefix,
// sorted ascending. e.g. prefix "agents/claude/" → ["v0.0.1", "v0.0.2", "v0.1.0"]
func validateTagVersions(tags []string, prefix string) []string {
	var versions []string
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			v := tag[len(prefix):]
			if semver.IsValid(v) {
				versions = append(versions, v)
			}
		}
	}
	slices.SortFunc(versions, semver.Compare)
	return versions
}

// validateLatestTagVersion returns the highest semver from tags with the given prefix.
func validateLatestTagVersion(tags []string, prefix string) string {
	versions := validateTagVersions(tags, prefix)
	if len(versions) == 0 {
		return ""
	}
	return versions[len(versions)-1]
}

// validateGitTagPrefix returns the git tag prefix for a module.
// e.g. ("agents", "claude") → "agents/claude/"
// e.g. ("tasks", "review/architecture") → "tasks/review/architecture/"
func validateGitTagPrefix(category, name string) string {
	return category + "/" + name + "/"
}

// validateIndex validates the index module and returns a doctor section for reporting.
// Returns the section, the loaded index (nil if fatal), and whether the status is fatal.
func validateIndex(ctx context.Context, client registry.Client, indexPath string, tags []string) (doctor.SectionResult, *registry.Index, bool) {
	section := doctor.SectionResult{Name: "Index"}

	// Check for version mismatch: if the configured path has a specific pinned version,
	// verify it exists by listing available versions.
	if err := validateCheckIndexVersionExists(ctx, client, indexPath); err != nil {
		section.Results = append(section.Results, doctor.CheckResult{
			Status:  doctor.StatusFail,
			Label:   "Version mismatch",
			Message: err.Error(),
		})
		return section, nil, true
	}

	// Resolve to latest canonical version
	resolvedPath, err := client.ResolveLatestVersion(ctx, indexPath)
	if err != nil {
		section.Results = append(section.Results, doctor.CheckResult{
			Status:  doctor.StatusFail,
			Label:   "Unreachable",
			Message: "cannot resolve index version from registry",
		})
		return section, nil, true
	}

	// Fetch the module
	result, err := client.Fetch(ctx, resolvedPath)
	if err != nil {
		section.Results = append(section.Results, doctor.CheckResult{
			Status:  doctor.StatusFail,
			Label:   "Unreachable",
			Message: fmt.Sprintf("cannot fetch index module: %v", err),
		})
		return section, nil, true
	}

	// Load and decode the index
	idx, err := registry.LoadIndex(result.SourceDir, client.Registry())
	if err != nil {
		section.Results = append(section.Results, doctor.CheckResult{
			Status:  doctor.StatusFail,
			Label:   "Corrupt",
			Message: fmt.Sprintf("index failed to load: %v", err),
		})
		return section, nil, true
	}

	// Extract resolved version string (e.g. "v0.1.8")
	resolvedVersion := indexVersionFromPath(resolvedPath)

	// Check staleness: compare published version vs latest git tag
	latestTag := validateLatestTagVersion(tags, "index/")
	if latestTag != "" && resolvedVersion != "" {
		if semver.Compare(resolvedVersion, latestTag) < 0 {
			section.Results = append(section.Results, doctor.CheckResult{
				Status:  doctor.StatusWarn,
				Label:   "Stale",
				Message: fmt.Sprintf("registry has %s but latest git tag is %s", resolvedVersion, latestTag),
			})
			// Not fatal — continue to mismatch checks
			if total := indexEntryCount(idx); total == 0 {
				section.Results = append(section.Results, doctor.CheckResult{
					Status:  doctor.StatusWarn,
					Label:   "Empty",
					Message: "index contains no entries",
				})
				return section, nil, false // empty but not fatal
			}
			return section, idx, false
		}
	}

	// Check empty
	if total := indexEntryCount(idx); total == 0 {
		section.Results = append(section.Results, doctor.CheckResult{
			Status:  doctor.StatusWarn,
			Label:   "Empty",
			Message: "index contains no entries",
		})
		return section, nil, false
	}

	section.Results = append(section.Results, doctor.CheckResult{
		Status:  doctor.StatusPass,
		Label:   "Valid",
		Message: resolvedVersion,
	})
	return section, idx, false
}

// validateCheckIndexVersionExists returns an error if the configured index path pins
// a specific version that does not exist in the registry.
func validateCheckIndexVersionExists(ctx context.Context, client registry.Client, indexPath string) error {
	// Only check if the path includes a canonical version (not just @v0)
	atIdx := strings.LastIndex(indexPath, "@")
	if atIdx == -1 {
		return nil
	}
	ver := indexPath[atIdx+1:]
	if semver.Canonical(ver) != ver {
		// Major version only (e.g. "v0") or non-canonical — no specific version to validate
		return nil
	}

	// List available versions and confirm the pinned one exists
	versions, err := client.ModuleVersions(ctx, indexPath)
	if err != nil {
		// Can't determine — let the subsequent resolve/fetch handle it
		return nil
	}
	if slices.Contains(versions, ver) {
		return nil
	}
	return fmt.Errorf("version %s not found in registry (available: %s)", ver, strings.Join(versions, ", "))
}

// indexVersionFromPath extracts the canonical version string from a resolved module path.
// Returns "" for major-only versions (e.g. "@v1") or invalid inputs.
// e.g. "github.com/start-cli/library/index@v1.0.1" → "v1.0.1"
func indexVersionFromPath(resolvedPath string) string {
	if idx := strings.LastIndex(resolvedPath, "@"); idx != -1 {
		v := resolvedPath[idx+1:]
		if semver.Canonical(v) == v {
			return v
		}
	}
	return ""
}

// indexEntryCount returns the total number of entries across all index categories.
func indexEntryCount(idx *registry.Index) int {
	return len(idx.Agents) + len(idx.Roles) + len(idx.Contexts) + len(idx.Tasks)
}

// validateModules runs the five mismatch checks for every module in the index
// and discovers filesystem-only modules. onModule is called after each module check.
func validateModules(ctx context.Context, client registry.Client, idx *registry.Index, tags []string, cacheDir string, onModule func()) []validateCatResult {
	categories := []struct {
		name    string
		entries map[string]registry.IndexEntry
	}{
		{"agents", idx.Agents},
		{"roles", idx.Roles},
		{"contexts", idx.Contexts},
		{"tasks", idx.Tasks},
	}

	var results []validateCatResult
	for _, cat := range categories {
		catResult := validateCatResult{name: cat.name}

		// Collect names from the index for sorted output
		names := make([]string, 0, len(cat.entries))
		for n := range cat.entries {
			names = append(names, n)
		}
		sort.Strings(names)

		// Check each indexed module
		for _, name := range names {
			entry := cat.entries[name]
			m := validateOneModule(ctx, client, cat.name, name, entry, tags, cacheDir)
			catResult.modules = append(catResult.modules, m)
			if onModule != nil {
				onModule()
			}
		}

		// Check 4: find filesystem modules not in the index
		fsModules := validateFindFSModules(cat.name, cacheDir)
		for _, fsName := range fsModules {
			if _, inIndex := cat.entries[fsName]; !inIndex {
				catResult.modules = append(catResult.modules, validateModuleResult{
					name:   fsName,
					status: validateModuleFail,
					issues: []string{"module directory exists in repository but has no index entry — remove the directory or add it to the index"},
				})
			}
		}

		results = append(results, catResult)
	}
	return results
}

// validateOneModule runs checks 1–3 and the staleness check for a single indexed module.
// Check 4 (filesystem orphan detection) is performed by the caller, validateModules.
func validateOneModule(ctx context.Context, client registry.Client, category, name string, entry registry.IndexEntry, tags []string, cacheDir string) validateModuleResult {
	m := validateModuleResult{
		name:    name,
		version: entry.Version,
		status:  validateModulePass,
	}

	tagPrefix := validateGitTagPrefix(category, name)
	tagVersions := validateTagVersions(tags, tagPrefix)
	tagVersionSet := make(map[string]bool, len(tagVersions))
	for _, v := range tagVersions {
		tagVersionSet[v] = true
	}

	// Get published versions from registry
	publishedVersions, err := client.ModuleVersions(ctx, entry.Module)
	if err != nil {
		m.issues = append(m.issues, fmt.Sprintf("cannot query registry: %v", err))
		m.status = validateModuleFail
		return m
	}
	slices.SortFunc(publishedVersions, semver.Compare)

	publishedSet := make(map[string]bool, len(publishedVersions))
	for _, v := range publishedVersions {
		publishedSet[v] = true
	}

	latestPublished := ""
	if len(publishedVersions) > 0 {
		latestPublished = publishedVersions[len(publishedVersions)-1]
	}

	// Extract the module's major version from its path (e.g. "...@v1" → "v1").
	// The registry only returns versions for this major version, so git tags
	// from prior major versions (e.g. v0.x.x tags after upgrading to @v1) are
	// not relevant to checks against the registry.
	moduleMajor := ""
	if idx := strings.LastIndex(entry.Module, "@"); idx >= 0 {
		moduleMajor = entry.Module[idx+1:]
	}

	// Check 1: index version does not match latest published version
	if entry.Version != "" && latestPublished != "" && entry.Version != latestPublished {
		m.issues = append(m.issues, fmt.Sprintf("index version %s does not match latest published %s", entry.Version, latestPublished))
	}

	// Check 2: latest published version has no corresponding git tag
	if latestPublished != "" && !tagVersionSet[latestPublished] {
		m.issues = append(m.issues, fmt.Sprintf("published version %s has no git tag %s", latestPublished, tagPrefix+latestPublished))
	}

	// Check 3: git tags exist with no corresponding published version.
	// Only check tags whose major version matches the module's current major
	// version. Old tags from prior major versions are expected to be absent
	// from the registry for the current module path.
	for _, tv := range tagVersions {
		if moduleMajor != "" && semver.Major(tv) != moduleMajor {
			continue
		}
		if !publishedSet[tv] {
			m.issues = append(m.issues, fmt.Sprintf("git tag %s%s was never published to registry", tagPrefix, tv))
		}
	}

	// Staleness check: content changed since the latest tagged and published version
	latestTag := ""
	if len(tagVersions) > 0 {
		latestTag = tagVersions[len(tagVersions)-1]
	}
	if latestTag != "" && publishedSet[latestTag] {
		stale, err := validateIsStale(cacheDir, tagPrefix+latestTag, category+"/"+name)
		if err != nil {
			m.issues = append(m.issues, fmt.Sprintf("staleness check failed: %v", err))
		} else if stale {
			m.issues = append(m.issues, fmt.Sprintf("content changed since %s%s — publish a new version", tagPrefix, latestTag))
		}
	}

	if len(m.issues) > 0 {
		m.status = validateModuleFail
	}
	return m
}

// validateFindFSModules returns the names of modules found in the filesystem
// for the given category directory. A module is identified by the presence of cue.mod/.
func validateFindFSModules(category, cacheDir string) []string {
	catDir := filepath.Join(cacheDir, category)
	var modules []string
	_ = validateWalkModules(catDir, "", func(relPath string) {
		modules = append(modules, relPath)
	})
	sort.Strings(modules)
	return modules
}

// validateWalkModules recursively walks dir, calling fn with the path of each
// directory that contains a cue.mod/ subdirectory. relBase is prepended to paths.
func validateWalkModules(dir, relBase string, fn func(string)) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		fullPath := filepath.Join(dir, name)
		relPath := name
		if relBase != "" {
			relPath = relBase + "/" + name
		}
		// A directory with cue.mod/ is a CUE module
		if _, err := os.Stat(filepath.Join(fullPath, "cue.mod")); err == nil {
			fn(relPath)
			continue // don't recurse into module directories
		}
		// Recurse into non-module directories (e.g. tasks/review/ containing many tasks)
		_ = validateWalkModules(fullPath, relPath, fn)
	}
	return nil
}

// validateIsStale returns true if the module content has changed since the given git tag.
// Returns an error if the git command fails (e.g. missing tag, corrupt repo).
func validateIsStale(cacheDir, tag, relPath string) (bool, error) {
	out, err := exec.Command("git", "-C", cacheDir, "diff", "--name-only", tag+"..HEAD", "--", relPath).Output()
	if err != nil {
		return false, fmt.Errorf("git diff failed for tag %s: %w", tag, err)
	}
	return strings.TrimSpace(string(out)) != "", nil
}

// outputValidateJSON marshals the full validate result as JSON.
func outputValidateJSON(w io.Writer, indexSection doctor.SectionResult, cats []validateCatResult) error {
	result := ValidateResult{
		Index: ValidateIndexResult{
			Checks: make([]ValidateCheckResult, 0, len(indexSection.Results)),
		},
		Categories: make([]ValidateCategoryResult, 0),
	}

	for _, cr := range indexSection.Results {
		result.Index.Checks = append(result.Index.Checks, ValidateCheckResult{
			Status:  cr.Status.String(),
			Label:   cr.Label,
			Message: cr.Message,
		})
	}

	for _, cat := range cats {
		catResult := ValidateCategoryResult{
			Name:    cat.name,
			Modules: make([]ValidateModuleResult, 0, len(cat.modules)),
		}
		for _, m := range cat.modules {
			status := "pass"
			if m.status == validateModuleFail {
				status = "fail"
			}
			catResult.Modules = append(catResult.Modules, ValidateModuleResult{
				Name:    m.name,
				Version: m.version,
				Status:  status,
				Issues:  m.issues,
			})
			result.Stats.Checked++
			if m.status == validateModulePass {
				result.Stats.Pass++
			} else {
				result.Stats.Fail++
			}
		}
		result.Categories = append(result.Categories, catResult)
	}

	if err := writeJSON(w, result); err != nil {
		return fmt.Errorf("marshalling validate results: %w", err)
	}
	return nil
}

// validateHasFailure returns true if any module has a failure status.
func validateHasFailure(cats []validateCatResult) bool {
	for _, cat := range cats {
		for _, m := range cat.modules {
			if m.status == validateModuleFail {
				return true
			}
		}
	}
	return false
}

// printValidateIndexSection prints the index validation result (Section 1).
// Uses the doctor CheckResult types for status icons but without the full doctor
// reporter header or summary — this is the focused doctor validate output,
// not the full doctor report.
func printValidateIndexSection(w io.Writer, section doctor.SectionResult) {
	tui.ColorHeader.Fprintln(w, section.Name)
	for _, result := range section.Results {
		fmt.Fprint(w, "  ")
		switch result.Status {
		case doctor.StatusPass:
			tui.ColorSuccess.Fprint(w, "✓")
		case doctor.StatusFail:
			tui.ColorError.Fprint(w, "✗")
		case doctor.StatusWarn:
			tui.ColorWarning.Fprint(w, "⚠")
		default:
			fmt.Fprint(w, "-")
		}
		if result.Message != "" {
			fmt.Fprintf(w, " %s - ", result.Label)
			tui.ColorDim.Fprint(w, result.Message)
		} else {
			fmt.Fprintf(w, " %s", result.Label)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

// printValidateModules prints Section 2 output.
func printValidateModules(w io.Writer, cats []validateCatResult, verbose bool) {
	for _, cat := range cats {
		total := len(cat.modules)
		fail := 0
		for _, m := range cat.modules {
			if m.status == validateModuleFail {
				fail++
			}
		}

		catColor := tui.CategoryColor(cat.name)

		if verbose {
			// Verbose: list every module with its status
			catColor.Fprintf(w, "%s", cat.name)
			fmt.Fprintf(w, " %s\n", tui.Annotate("%d", total))
			for _, m := range cat.modules {
				if m.status == validateModulePass {
					tui.ColorSuccess.Fprint(w, "  ✓")
				} else {
					tui.ColorError.Fprint(w, "  ✗")
				}
				fmt.Fprintf(w, " %-20s", m.name)
				if m.version != "" {
					tui.ColorDim.Fprintf(w, " %s", m.version)
				}
				fmt.Fprintln(w)
				for _, issue := range m.issues {
					tui.ColorDim.Fprintf(w, "      %s\n", issue)
				}
			}
		} else {
			// Default: one summary line per category
			pass := total - fail
			catColor.Fprintf(w, "%-10s", cat.name)
			if fail == 0 {
				tui.ColorSuccess.Fprintf(w, " %d/%d OK\n", pass, total)
			} else {
				tui.ColorError.Fprintf(w, " %d/%d FAIL\n", pass, total)
				// List failing modules below
				for _, m := range cat.modules {
					if m.status != validateModuleFail {
						continue
					}
					tui.ColorError.Fprint(w, "  ✗")
					fmt.Fprintf(w, " %-20s", m.name)
					if m.version != "" {
						tui.ColorDim.Fprintf(w, " %s", m.version)
					}
					fmt.Fprintln(w)
					for _, issue := range m.issues {
						tui.ColorDim.Fprintf(w, "      %s\n", issue)
					}
				}
			}
		}
		fmt.Fprintln(w)
	}
}

// printValidateStats prints Section 3 (always shown). Returns true if there are failures.
func printValidateStats(w io.Writer, cats []validateCatResult) bool {
	var checked, pass, fail int
	for _, cat := range cats {
		for _, m := range cat.modules {
			checked++
			switch m.status {
			case validateModulePass:
				pass++
			case validateModuleFail:
				fail++
			}
		}
	}

	fmt.Fprintf(w, "Checked: ")
	tui.ColorDim.Fprintf(w, "%d modules", checked)
	fmt.Fprint(w, "  Pass: ")
	tui.ColorSuccess.Fprintf(w, "%d", pass)
	fmt.Fprint(w, "  Fail: ")
	if fail > 0 {
		tui.ColorError.Fprintf(w, "%d", fail)
	} else {
		tui.ColorDim.Fprintf(w, "%d", fail)
	}
	fmt.Fprintln(w)

	return fail > 0
}
