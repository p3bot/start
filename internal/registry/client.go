// Package registry handles fetching CUE modules from the CUE Central Registry.
package registry

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"cuelang.org/go/mod/modconfig"
	"cuelang.org/go/mod/module"
	"golang.org/x/mod/semver"
)

// Client is the registry surface consumed across the codebase. The interface
// keeps the natural name so call sites read unchanged; the concrete fetcher is
// the unexported client below. Defining it here (rather than where consumed)
// is deliberate: consumers span internal/cli, internal/modules, and
// internal/orchestration, so the "interface where consumed" rule cannot place
// it in a single consumer package.
type Client interface {
	FetchIndex(ctx context.Context, indexPath string) (*Index, string, error)
	Fetch(ctx context.Context, modulePath string) (FetchResult, error)
	ModuleVersions(ctx context.Context, modulePath string) ([]string, error)
	ResolveLatestVersion(ctx context.Context, modulePath string) (string, error)
	Registry() modconfig.Registry
}

// client fetches CUE modules from the registry with retry logic.
type client struct {
	registry modconfig.Registry
	retries  int
	baseWait time.Duration
}

// NewClient creates a registry client using CUE's standard configuration.
// It respects CUE_REGISTRY environment variable and cue login authentication.
func NewClient() (Client, error) {
	reg, err := modconfig.NewRegistry(nil)
	if err != nil {
		return nil, fmt.Errorf("creating registry client: %w", err)
	}
	return &client{
		registry: reg,
		retries:  3,
		baseWait: time.Second,
	}, nil
}

// FetchResult contains the result of fetching a module.
type FetchResult struct {
	// SourceDir is the filesystem path to the fetched module.
	SourceDir string
}

// Registry returns the underlying modconfig.Registry for use with cue/load.
func (c *client) Registry() modconfig.Registry {
	return c.registry
}

// Fetch downloads a module from the registry with retry logic.
// The module path should include version, e.g., "github.com/user/repo/path@v0".
//
// The retry loop spends attempts only on genuinely transient failures. A
// not-found condition discovered inside Fetch short-circuits and returns a
// FetchError the mapper classifies as exit 3, so a typo'd name never burns
// retries or surfaces as transient. A failure upstream flattens to an opaque
// string (classifyFetch returns ok == false) is returned unwrapped and
// unretried, degrading to the general exit code rather than a false 75.
func (c *client) Fetch(ctx context.Context, modulePath string) (FetchResult, error) {
	mv, err := module.ParseVersion(modulePath)
	if err != nil {
		// A local parse failure is a caller mistake, knowable before any
		// network call — the only reachable registry usage (2) condition.
		return FetchResult{}, &FetchError{Kind: FetchUsage, Op: "parsing module path", Path: modulePath, Err: err}
	}

	var lastErr error
	for attempt := 0; attempt < c.retries; attempt++ {
		if attempt > 0 {
			wait := c.baseWait * time.Duration(1<<(attempt-1)) // exponential backoff
			select {
			case <-ctx.Done():
				return FetchResult{}, ctx.Err()
			case <-time.After(wait):
			}
		}

		loc, err := c.registry.Fetch(ctx, mv)
		if err == nil {
			// Get the OS path from the SourceLoc
			dir, err := sourceLocToPath(loc)
			if err != nil {
				return FetchResult{}, fmt.Errorf("resolving source location for %s: %w", modulePath, err)
			}
			return FetchResult{SourceDir: dir}, nil
		}

		kind, ok := classifyFetch(err)
		if !ok {
			// Unclassifiable upstream error: do not retry, do not mislabel as
			// transient. Falls through to the general exit code.
			return FetchResult{}, fmt.Errorf("fetching module %s: %w", modulePath, err)
		}
		if kind != FetchTransient {
			// Permanent (not-found): retrying cannot clear it.
			return FetchResult{}, &FetchError{Kind: kind, Op: "fetch", Path: modulePath, Err: err}
		}
		lastErr = err
	}

	return FetchResult{}, &FetchError{Kind: FetchTransient, Op: "fetch", Path: modulePath, Attempts: c.retries, Err: lastErr}
}

// ModuleVersions returns available versions for a module path.
func (c *client) ModuleVersions(ctx context.Context, modulePath string) ([]string, error) {
	return c.registry.ModuleVersions(ctx, modulePath)
}

// ResolveLatestVersion resolves a module path with major version (e.g., @v0) to
// the latest canonical version (e.g., @v0.0.1).
func (c *client) ResolveLatestVersion(ctx context.Context, modulePath string) (string, error) {
	// Parse the module path to extract base path and major version
	mv, err := module.ParseVersion(modulePath)
	if err == nil && mv.Version() != "" {
		// Already has a version, check if it's canonical
		v := mv.Version()
		if semver.Canonical(v) == v {
			return modulePath, nil
		}
	}

	// Get available versions
	versions, err := c.ModuleVersions(ctx, modulePath)
	if err != nil {
		// Classify the network failure so version resolution carries the same
		// transient-vs-permanent distinction as Fetch; an unclassifiable error
		// is returned unwrapped and degrades to the general exit code.
		if kind, ok := classifyFetch(err); ok {
			return "", &FetchError{Kind: kind, Op: "resolve", Path: modulePath, Err: err}
		}
		return "", fmt.Errorf("getting versions for %s: %w", modulePath, err)
	}
	if len(versions) == 0 {
		// No published versions: the module does not exist at any version.
		return "", &FetchError{Kind: FetchNotFound, Op: "resolve", Path: modulePath, Err: fmt.Errorf("no versions found")}
	}

	// Sort versions by semver to find the latest
	slices.SortFunc(versions, semver.Compare)
	latestVersion := versions[len(versions)-1]

	// Replace the version in the module path
	// Module path format: path@version
	atIdx := strings.LastIndex(modulePath, "@")
	if atIdx == -1 {
		return "", fmt.Errorf("invalid module path %s: no version", modulePath)
	}

	return modulePath[:atIdx+1] + latestVersion, nil
}

// sourceLocToPath extracts the OS filesystem path from a module.SourceLoc.
func sourceLocToPath(loc module.SourceLoc) (string, error) {
	// SourceLoc.FS may implement OSRootFS which provides the OS path.
	type osRootFS interface {
		OSRoot() string
	}
	if ofs, ok := loc.FS.(osRootFS); ok {
		return ofs.OSRoot(), nil
	}
	return "", fmt.Errorf("source location does not provide OS path")
}
