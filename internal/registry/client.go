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

// Client is the registry surface consumed across the codebase. It lives here
// rather than in a consumer package because consumers span internal/cli,
// internal/modules, and internal/orchestration.
type Client interface {
	FetchIndex(ctx context.Context, indexPath string) (*Index, string, error)
	Fetch(ctx context.Context, modulePath string) (FetchResult, error)
	ModuleVersions(ctx context.Context, modulePath string) ([]string, error)
	ResolveLatestVersion(ctx context.Context, modulePath string) (string, error)
	Registry() modconfig.Registry
}

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
	SourceDir string // filesystem path to the fetched module
}

// Registry returns the underlying modconfig.Registry for use with cue/load.
func (c *client) Registry() modconfig.Registry {
	return c.registry
}

// Fetch downloads a module from the registry with retry logic.
// The module path should include version, e.g., "github.com/user/repo/path@v0".
//
// Only transient failures are retried; not-found short-circuits (exit 3) and
// unclassifiable errors return unwrapped to degrade to the general exit code.
func (c *client) Fetch(ctx context.Context, modulePath string) (FetchResult, error) {
	mv, err := module.ParseVersion(modulePath)
	if err != nil {
		// Parse failure is a caller mistake knowable before any network call.
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
			dir, err := sourceLocToPath(loc)
			if err != nil {
				return FetchResult{}, fmt.Errorf("resolving source location for %s: %w", modulePath, err)
			}
			return FetchResult{SourceDir: dir}, nil
		}

		kind, ok := classifyFetch(err)
		if !ok {
			// Unclassifiable: do not retry or mislabel as transient.
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
	mv, err := module.ParseVersion(modulePath)
	if err == nil && mv.Version() != "" {
		v := mv.Version()
		if semver.Canonical(v) == v {
			return modulePath, nil
		}
	}

	versions, err := c.ModuleVersions(ctx, modulePath)
	if err != nil {
		// Classify so resolution carries Fetch's transient-vs-permanent distinction.
		if kind, ok := classifyFetch(err); ok {
			return "", &FetchError{Kind: kind, Op: "resolve", Path: modulePath, Err: err}
		}
		return "", fmt.Errorf("getting versions for %s: %w", modulePath, err)
	}
	if len(versions) == 0 {
		return "", &FetchError{Kind: FetchNotFound, Op: "resolve", Path: modulePath, Err: fmt.Errorf("no versions found")}
	}

	slices.SortFunc(versions, semver.Compare)
	latestVersion := versions[len(versions)-1]

	atIdx := strings.LastIndex(modulePath, "@")
	if atIdx == -1 {
		return "", fmt.Errorf("invalid module path %s: no version", modulePath)
	}

	return modulePath[:atIdx+1] + latestVersion, nil
}

func sourceLocToPath(loc module.SourceLoc) (string, error) {
	type osRootFS interface {
		OSRoot() string
	}
	if ofs, ok := loc.FS.(osRootFS); ok {
		return ofs.OSRoot(), nil
	}
	return "", fmt.Errorf("source location does not provide OS path")
}
