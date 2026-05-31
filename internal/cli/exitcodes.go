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
	ExitSuccess    = 0
	ExitFailure    = 1 // general/internal failure; do not retry blindly
	ExitUsage      = 2 // bad arguments or input; fix and retry
	ExitNotFound   = 3
	ExitPermission = 4
	ExitConflict   = 5  // resource state prevents the operation
	ExitTransient  = 75 // retry with backoff (sysexits EX_TEMPFAIL)
	ExitConfig     = 78 // configuration/environment error (sysexits EX_CONFIG)
)

// ExitError forces a specific process exit code for the error it carries.
// The mapper checks for it first, so any call site can pin a code directly
// when the fault-domain sentinels in internal/fault do not express it.
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
// fault domain, branching only on error type and never on message text.
//
// Order matters where chains overlap: a permission failure reading user CUE
// is checked before the config-fault sentinel so it surfaces as 4 (fix
// permissions) rather than 78, even when tagged as a user-config fault.
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

	// Permission denied is its own domain (fix perms), distinct from invalid
	// config content; checked ahead of ErrUserConfig.
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

	// Schema-validation failures are user-config faults (78).
	var validationErr *internalcue.ValidationError
	if errors.As(err, &validationErr) {
		return ExitConfig
	}

	// A fetch that exceeds its deadline is transient — a retry could clear it.
	if errors.Is(err, context.DeadlineExceeded) {
		return ExitTransient
	}

	return ExitFailure
}

// usageError tags err as a usage fault (exit 2).
func usageError(err error) error { return fault.Usage(err) }

// notFoundError tags err as a missing-resource fault (exit 3).
func notFoundError(err error) error { return fault.NotFound(err) }
