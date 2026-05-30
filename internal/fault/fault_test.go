package fault

import (
	"errors"
	"fmt"
	"testing"
)

// TestTag_PreservesMessage verifies a tagged error delegates Error() to the
// wrapped error, so producers keep their existing user-facing text (e.g.
// `role "x" not found`) rather than appending the sentinel's own words.
func TestTag_PreservesMessage(t *testing.T) {
	t.Parallel()
	const msg = `role "x" not found`
	if got := NotFound(errors.New(msg)).Error(); got != msg {
		t.Errorf("Error() = %q, want %q", got, msg)
	}
}

// TestTag_ClassifiableViaIs verifies each constructor attaches exactly its own
// domain sentinel and no other, so the exit-code mapper's errors.Is branches
// cannot cross-classify.
func TestTag_ClassifiableViaIs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		domain error
		others []error
	}{
		{"not found", NotFound(errors.New("x")), ErrNotFound, []error{ErrUsage, ErrUserConfig}},
		{"usage", Usage(errors.New("x")), ErrUsage, []error{ErrNotFound, ErrUserConfig}},
		{"user config", UserConfig(errors.New("x")), ErrUserConfig, []error{ErrNotFound, ErrUsage}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if !errors.Is(tt.err, tt.domain) {
				t.Errorf("errors.Is(%v, its domain) = false, want true", tt.err)
			}
			for _, other := range tt.others {
				if errors.Is(tt.err, other) {
					t.Errorf("%v should not classify as %v", tt.err, other)
				}
			}
		})
	}
}

// TestTag_PreservesOriginalChain verifies Unwrap reports both the original
// chain and the domain sentinel, so a sentinel wrapped inside the tagged error
// (e.g. fs.ErrPermission) stays reachable via errors.Is alongside the domain.
func TestTag_PreservesOriginalChain(t *testing.T) {
	t.Parallel()
	inner := errors.New("inner cause")
	tagged := UserConfig(fmt.Errorf("loading config: %w", inner))

	if !errors.Is(tagged, inner) {
		t.Error("original chain should remain reachable through the tag")
	}
	if !errors.Is(tagged, ErrUserConfig) {
		t.Error("domain sentinel should be reachable through the tag")
	}
}

// TestTag_NilReturnsNil verifies the constructors are nil-safe, so call sites
// can tag unconditionally without converting a nil error into a non-nil one.
func TestTag_NilReturnsNil(t *testing.T) {
	t.Parallel()
	for _, ctor := range []struct {
		name string
		fn   func(error) error
	}{
		{"NotFound", NotFound},
		{"Usage", Usage},
		{"UserConfig", UserConfig},
	} {
		if err := ctor.fn(nil); err != nil {
			t.Errorf("%s(nil) = %v, want nil", ctor.name, err)
		}
	}
}
