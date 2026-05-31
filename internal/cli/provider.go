package cli

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/registry"
)

type providerKey struct{}

// clientProvider returns a fresh registry client per call (preserving the call
// count of direct registry.NewClient use). Production binds registry.NewClient;
// tests bind a stub. Stored on the command context so parallel tests do not race.
type clientProvider func() (registry.Client, error)

// WithProvider returns a context carrying the registry-client provider.
func WithProvider(ctx context.Context, provider clientProvider) context.Context {
	return context.WithValue(ctx, providerKey{}, provider)
}

// getProvider falls back to the production registry.NewClient when none is bound.
func getProvider(cmd *cobra.Command) clientProvider {
	if p, ok := cmd.Context().Value(providerKey{}).(clientProvider); ok {
		return p
	}
	return registry.NewClient
}
