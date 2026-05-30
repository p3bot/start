package cli

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/start-cli/start/internal/registry"
)

// providerKey is the context key for the registry-client provider.
type providerKey struct{}

// clientProvider returns a registry client. It is invoked once per registry
// interaction (a fresh client per call), preserving the call count of direct
// registry.NewClient() use. Production binds registry.NewClient; tests bind a
// stub. Stored on the command context, mirroring how Flags are threaded, so
// parallel tests do not race on shared global state.
type clientProvider func() (registry.Client, error)

// WithProvider returns a context carrying the registry-client provider.
func WithProvider(ctx context.Context, provider clientProvider) context.Context {
	return context.WithValue(ctx, providerKey{}, provider)
}

// getProvider retrieves the registry-client provider from the command context,
// falling back to the production registry.NewClient when none is bound.
func getProvider(cmd *cobra.Command) clientProvider {
	if p, ok := cmd.Context().Value(providerKey{}).(clientProvider); ok {
		return p
	}
	return registry.NewClient
}
