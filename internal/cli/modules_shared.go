package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/cache"
	"github.com/start-cli/start/internal/registry"
	"github.com/start-cli/start/internal/tui"
)

// fetchIndex centralises the registry-client + fetch + cache-write trio shared
// by install and update. The cache write is best-effort, logged at debug only.
func fetchIndex(ctx context.Context, cmd *cobra.Command, prog *tui.Progress, message string) (*registry.Index, registry.Client, error) {
	client, err := getProvider(cmd)()
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
