package registry

import (
	"context"
	"errors"
	"fmt"
	"net"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelang.org/go/mod/modregistry"
)

// FetchKind classifies a registry failure by fault domain so the CLI's
// exit-code mapper can branch on the typed value rather than re-deriving the
// cause downstream. The distinction is produced here, at the boundary where
// the upstream signal is still intact, and carried in the error value.
type FetchKind int

const (
	// FetchTransient is a network/server failure a retry could clear:
	// connection refused, DNS, a 5xx/429 response, rate limiting, or a
	// deadline. The CLI maps it to exit 75.
	FetchTransient FetchKind = iota

	// FetchNotFound is a module or version that does not exist:
	// modregistry.ErrNotFound, ociregistry.ErrNameUnknown, or a 404. The CLI
	// maps it to exit 3. A typo'd name must never present as transient, so
	// the Fetch retry loop short-circuits on this rather than retrying.
	FetchNotFound

	// FetchUsage is a caller-side mistake found before any network call —
	// a module path string that does not parse. The CLI maps it to exit 2.
	FetchUsage
)

// FetchError carries a registry failure together with its fault-domain
// classification. Producers in this package return it (wrapped with %w) so the
// mapper can errors.As it and read Kind; the underlying upstream error stays
// reachable through Unwrap for diagnostics and further errors.Is checks.
type FetchError struct {
	Kind     FetchKind
	Op       string // the operation that failed, e.g. "fetch", "resolve", "parse"
	Path     string // the module path involved
	Attempts int    // retries spent before giving up; 0 when not a retry-exhausted failure
	Err      error  // the wrapped upstream error
}

func (e *FetchError) Error() string {
	if e.Attempts > 1 {
		return fmt.Sprintf("%s %s after %d attempts: %v", e.Op, e.Path, e.Attempts, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
}

func (e *FetchError) Unwrap() error { return e.Err }

// classifyFetch maps an upstream registry error to a FetchKind, plus a bool
// reporting whether the error was classifiable at all. An unclassifiable error
// (ok == false) is neither retried nor wrapped as a FetchError: it falls
// through to the general exit code (1), per Requirement 5's treatment of the
// modcache mid-stream-drop case that upstream flattens to an opaque string.
func classifyFetch(err error) (FetchKind, bool) {
	if errors.Is(err, modregistry.ErrNotFound) || errors.Is(err, ociregistry.ErrNameUnknown) {
		return FetchNotFound, true
	}

	var httpErr ociregistry.HTTPError
	if errors.As(err, &httpErr) {
		switch code := httpErr.StatusCode(); {
		case code == 404:
			return FetchNotFound, true
		case code == 429 || (code >= 500 && code <= 599):
			return FetchTransient, true
		}
	}

	if errors.Is(err, ociregistry.ErrTooManyRequests) {
		return FetchTransient, true
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return FetchTransient, true
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return FetchTransient, true
	}

	return 0, false
}
