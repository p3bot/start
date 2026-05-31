package registry

import (
	"context"
	"errors"
	"fmt"
	"net"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelang.org/go/mod/modregistry"
)

// FetchKind classifies a registry failure by fault domain so the CLI's exit-code
// mapper can branch on the typed value. Classified here while the upstream signal
// is still intact, then carried in the error value.
type FetchKind int

const (
	// FetchTransient is a retryable network/server failure (connection refused,
	// DNS, 5xx/429, rate limit, deadline). The CLI maps it to exit 75.
	FetchTransient FetchKind = iota

	// FetchNotFound is a missing module or version (ErrNotFound, ErrNameUnknown,
	// 404). The CLI maps it to exit 3; the Fetch retry loop short-circuits on it.
	FetchNotFound

	// FetchUsage is a caller-side mistake found before any network call (an
	// unparseable module path). The CLI maps it to exit 2.
	FetchUsage
)

// FetchError carries a registry failure together with its fault-domain
// classification. Returned wrapped with %w so the mapper can errors.As it and
// read Kind; the upstream error stays reachable through Unwrap.
type FetchError struct {
	Kind     FetchKind
	Op       string // the operation that failed, e.g. "fetch", "resolve", "parse"
	Path     string // the module path involved
	Attempts int    // retries spent before giving up; 0 when not retry-exhausted
	Err      error  // the wrapped upstream error
}

func (e *FetchError) Error() string {
	if e.Attempts > 1 {
		return fmt.Sprintf("%s %s after %d attempts: %v", e.Op, e.Path, e.Attempts, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
}

func (e *FetchError) Unwrap() error { return e.Err }

// classifyFetch maps an upstream registry error to a FetchKind plus a bool
// reporting whether it was classifiable. An unclassifiable error (ok == false)
// is neither retried nor wrapped, falling through to the general exit code (1)
// to cover cases upstream flattens to an opaque string (e.g. modcache drops).
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
