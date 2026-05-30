package cli

import (
	"context"
	"errors"
	"io/fs"

	internalcue "github.com/start-cli/start/internal/cue"
	"github.com/start-cli/start/internal/fault"
	"github.com/start-cli/start/internal/registry"
)

// Exit codes follow the semantic taxonomy documented under `start help
// schemas`. The values are anchored on sysexits.h (75 transient, 78 config)
// and Bash convention (2 usage) so agents and shells generalise across tools.
const (
	ExitSuccess    = 0  // success
	ExitFailure    = 1  // general/internal failure; do not retry blindly
	ExitUsage      = 2  // bad arguments or input; fix and retry
	ExitNotFound   = 3  // resource not found
	ExitPermission = 4  // filesystem permission denied
	ExitConflict   = 5  // resource state prevents the operation
	ExitTransient  = 75 // retry with backoff (sysexits EX_TEMPFAIL)
	ExitConfig     = 78 // configuration/environment error (sysexits EX_CONFIG)
)

// ExitError forces a specific process exit code for the error it carries. The
// mapper checks for it first, so any call site can pin a code directly when
// the fault-domain sentinels in internal/fault do not express the distinction.
// Code 5 (conflict) currently has no producer; the constant and this carrier
// exist for taxonomy completeness (see the project's Implementation Guidance).
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return "exit error"
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error { return e.Err }

// ExitCodeFromError derives the process exit code from a returned error by
// fault domain. It branches only on error type — sentinels matched with
// errors.Is and typed errors matched with errors.As — and never on message
// text. nil maps to success.
//
// Order matters where chains overlap: an ExitError pins its code outright; a
// permission failure reading user CUE is checked before the config-fault
// sentinel so it surfaces as 4 (fix permissions) rather than 78 (fix the CUE),
// even when the loader has tagged the same error as a user-config fault.
func ExitCodeFromError(err error) int {
	if err == nil {
		return ExitSuccess
	}

	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}

	var fetchErr *registry.FetchError
	if errors.As(err, &fetchErr) {
		switch fetchErr.Kind {
		case registry.FetchNotFound:
			return ExitNotFound
		case registry.FetchUsage:
			return ExitUsage
		default:
			return ExitTransient
		}
	}

	// Permission denied on a user-owned path is its own domain (fix perms),
	// distinct from invalid config content. Checked ahead of ErrUserConfig.
	if errors.Is(err, fs.ErrPermission) {
		return ExitPermission
	}

	switch {
	case errors.Is(err, fault.ErrUsage):
		return ExitUsage
	case errors.Is(err, fault.ErrNotFound):
		return ExitNotFound
	case errors.Is(err, fault.ErrUserConfig):
		return ExitConfig
	}

	// Schema-validation failures are user-config faults (78). The loader's
	// build error and validator's ValidationError are the same CUE error
	// type, so 78-vs-1 is decided by the source-level tagging above, not here;
	// ValidationError is a distinct exported type and maps cleanly.
	var validationErr *internalcue.ValidationError
	if errors.As(err, &validationErr) {
		return ExitConfig
	}

	// A fetch that exceeds its deadline is transient — a retry could clear it.
	// Reachable via the resolver's and doctor's fetch timeouts when the
	// underlying error is the bare context error rather than a FetchError.
	if errors.Is(err, context.DeadlineExceeded) {
		return ExitTransient
	}

	return ExitFailure
}

// usageError tags err as a usage fault (exit 2) while preserving its message.
// Convenience wrapper over fault.Usage for the cli package's many inline
// input-validation sites.
func usageError(err error) error { return fault.Usage(err) }

// notFoundError tags err as a missing-resource fault (exit 3).
func notFoundError(err error) error { return fault.NotFound(err) }
