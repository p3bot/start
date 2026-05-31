package registry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelang.org/go/mod/modregistry"
)

func TestClassifyFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantKind FetchKind
		wantOK   bool
	}{
		{"module not found", modregistry.ErrNotFound, FetchNotFound, true},
		{"name unknown", ociregistry.ErrNameUnknown, FetchNotFound, true},
		{"wrapped not found", fmt.Errorf("fetch: %w", modregistry.ErrNotFound), FetchNotFound, true},
		{"string lookalike does not classify", errors.New("module not found"), 0, false},
		{"too many requests", ociregistry.ErrTooManyRequests, FetchTransient, true},
		{"deadline", context.DeadlineExceeded, FetchTransient, true},
		{"net error", &net.DNSError{Err: "no such host", IsNotFound: true}, FetchTransient, true},
		{"404 http", newHTTPError(404), FetchNotFound, true},
		{"500 http", newHTTPError(500), FetchTransient, true},
		{"503 http", newHTTPError(503), FetchTransient, true},
		{"429 http", newHTTPError(429), FetchTransient, true},
		{"opaque string degrades", errors.New("copying zip: unexpected EOF"), 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kind, ok := classifyFetch(tt.err)
			if ok != tt.wantOK {
				t.Fatalf("classifyFetch ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && kind != tt.wantKind {
				t.Errorf("classifyFetch kind = %v, want %v", kind, tt.wantKind)
			}
		})
	}
}

func newHTTPError(status int) error {
	return ociregistry.NewHTTPError(errors.New("http failure"), status, nil, nil)
}

// TestFetchError_Error verifies that the message reports the attempt count only
// for retry-exhausted failures (Attempts > 0).
func TestFetchError_Error(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  *FetchError
		want string
	}{
		{
			name: "retry-exhausted reports attempts",
			err:  &FetchError{Op: "fetch", Path: "mod@v0", Attempts: 3, Err: errors.New("connection refused")},
			want: "fetch mod@v0 after 3 attempts: connection refused",
		},
		{
			name: "no attempt count when unset",
			err:  &FetchError{Op: "resolve", Path: "mod@v0", Err: errors.New("no versions found")},
			want: "resolve mod@v0: no versions found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFetchError_KindPreservedThroughWrap verifies the typed Kind survives %w
// wrapping so the mapper's errors.As recovers it downstream.
func TestFetchError_KindPreservedThroughWrap(t *testing.T) {
	t.Parallel()
	base := &FetchError{Kind: FetchTransient, Op: "fetch", Path: "x", Err: errors.New("boom")}
	wrapped := fmt.Errorf("fetching index: %w", base)

	var fe *FetchError
	if !errors.As(wrapped, &fe) {
		t.Fatal("errors.As failed to recover FetchError through %w wrap")
	}
	if fe.Kind != FetchTransient {
		t.Errorf("kind = %v, want FetchTransient", fe.Kind)
	}
}
