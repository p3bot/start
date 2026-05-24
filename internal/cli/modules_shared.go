package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
)

// fetchIndex creates a registry client, fetches the registry index, and writes
// the resolved version to the cache. The supplied progress reporter shows
// message while the network call is in flight. The cache write is best-effort:
// any error is logged at debug level only.
//
// Centralises the registry-client + fetch + cache-write trio shared by
// install and update. Each command's post-fetch logic stays at its call site
// because the commands use the index differently and consolidating further
// would obscure call-site UX.
//
// library uses Fetch + ResolveLatestVersion directly (it needs the source
// dir, not a parsed index), and list fetches conditionally inside
// checkForUpdates with graceful failure — neither fits this helper.
func fetchIndex(ctx context.Context, cmd *cobra.Command, prog *tui.Progress, message string) (*registry.Index, *registry.Client, error) {
	client, err := registry.NewClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating registry client: %w", err)
	}
	prog.Update("%s", message)
	index, indexVersion, err := client.FetchIndex(ctx, resolveLibraryIndexPath())
	if err != nil {
		return nil, nil, fmt.Errorf("fetching index: %w", err)
	}
	if err := cache.WriteIndex(indexVersion); err != nil {
		debugf(cmd.ErrOrStderr(), getFlags(cmd), dbgCache, "cache write failed: %v", err)
	}
	return index, client, nil
}
