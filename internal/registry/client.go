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

// Client fetches CUE modules from the registry with retry logic.
type Client struct {
	registry modconfig.Registry
	retries  int
	baseWait time.Duration
}

// NewClient creates a registry client using CUE's standard configuration.
// It respects CUE_REGISTRY environment variable and cue login authentication.
func NewClient() (*Client, error) {
	reg, err := modconfig.NewRegistry(nil)
	if err != nil {
		return nil, fmt.Errorf("creating registry client: %w", err)
	}
	return &Client{
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
func (c *Client) Registry() modconfig.Registry {
	return c.registry
}

// Fetch downloads a module from the registry with retry logic.
// The module path should include version, e.g., "github.com/user/repo/path@v0".
func (c *Client) Fetch(ctx context.Context, modulePath string) (FetchResult, error) {
	mv, err := module.ParseVersion(modulePath)
	if err != nil {
		return FetchResult{}, fmt.Errorf("parsing module path %q: %w", modulePath, err)
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
		lastErr = err
	}

	return FetchResult{}, fmt.Errorf("fetching module %s after %d attempts: %w", modulePath, c.retries, lastErr)
}

// ModuleVersions returns available versions for a module path.
func (c *Client) ModuleVersions(ctx context.Context, modulePath string) ([]string, error) {
	return c.registry.ModuleVersions(ctx, modulePath)
}

// ResolveLatestVersion resolves a module path with major version (e.g., @v0) to
// the latest canonical version (e.g., @v0.0.1).
func (c *Client) ResolveLatestVersion(ctx context.Context, modulePath string) (string, error) {
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
		return "", fmt.Errorf("getting versions for %s: %w", modulePath, err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no versions found for %s", modulePath)
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
