package cli

import (
	"context"
	"io"

	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/modules"
	"github.com/start-cli/start/internal/registry"
)

// decideCachedIndex is the single cache-gating decision shared by ensureIndex
// (resolution) and resolveDisplayIndexVersion (display commands). When wantLive
// is false and the cache is fresh (< DefaultMaxAge) and its version belongs to
// the same module as effectivePath, the cached canonical version is reused
// offline. Otherwise the caller must resolve the index live.
//
// It performs no network I/O and no cache write: the live resolve, the
// best-effort cache write, and any "fetching" notice belong to each consumer's
// wrapper. effectivePath must already be run through registry.EffectiveIndexPath.
func decideCachedIndex(effectivePath string, wantLive bool) (version string, useCache bool) {
	if wantLive {
		return "", false
	}
	cached, err := cache.ReadIndex()
	if err != nil || !cached.IsFresh(cache.DefaultMaxAge) ||
		modules.ModuleFromOrigin(cached.Version) != modules.ModuleFromOrigin(effectivePath) {
		return "", false
	}
	return cached.Version, true
}

// resolveDisplayIndexVersion resolves the canonical index version for a
// read-only display command (list --verbose, search, library) under the
// cache-gating rule. With a fresh cache it returns the cached version and makes
// no registry metadata request; otherwise it resolves the latest version live
// and writes the cache best-effort. The persistent --refresh flag (flags.Refresh)
// forces the live resolve regardless of cache freshness.
//
// The returned version is canonical, so a follow-up FetchIndex or Fetch
// re-resolves nothing. Callers fetch from it what they each need: list and
// search parse the *Index via FetchIndex; library reads SourceDir via Fetch.
func resolveDisplayIndexVersion(ctx context.Context, client registry.Client, configuredPath string, stderr io.Writer, flags *Flags) (string, error) {
	effectivePath := registry.EffectiveIndexPath(configuredPath)
	if version, useCache := decideCachedIndex(effectivePath, flags.Refresh); useCache {
		debugf(stderr, flags, dbgCache, "using cached index version: %s", version)
		return version, nil
	}

	resolved, err := client.ResolveLatestVersion(ctx, effectivePath)
	if err != nil {
		return "", err
	}
	if err := cache.WriteIndex(resolved); err != nil {
		debugf(stderr, flags, dbgCache, "cache write failed: %v", err)
	}
	return resolved, nil
}
