// Package fault defines the cross-cutting error sentinels that the CLI's
// exit-code mapper classifies. They live here, in a dependency-free leaf
// package, because the conditions they mark are produced across internal/cli,
// internal/modules, and internal/orchestration — no single consumer package
// can own a sentinel every layer needs to wrap at its source.
//
// Each sentinel names a fault domain, not a message. Producers wrap their
// own contextual error with %w (e.g. fmt.Errorf("role %q: %w", name,
// fault.ErrNotFound)); the mapper branches with errors.Is, never on text.
package fault

import "errors"

var (
	// ErrNotFound marks a missing resource: a config item, module, or agent
	// the caller named that does not exist. Maps to exit 3.
	ErrNotFound = errors.New("not found")

	// ErrUsage marks a caller mistake the user can fix and retry: a bad flag
	// value, a too-short query, a missing terminal for an interactive flow.
	// Maps to exit 2.
	ErrUsage = errors.New("usage error")

	// ErrUserConfig marks an environment fault the user must correct that is
	// not a missing resource: invalid user CUE, or an agent binary absent
	// from PATH. Maps to exit 78. Distinct from ErrNotFound so a typo'd
	// module name (retry with a different name) never collapses into a
	// broken-environment signal (fix the environment).
	ErrUserConfig = errors.New("invalid configuration")
)

// tagged attaches a fault-domain sentinel without altering the message:
// Error() delegates to the wrapped error, while Unwrap reports both the
// original chain and the domain so errors.Is(result, domain) holds. Producers
// keep their existing messages yet stay classifiable.
type tagged struct {
	err    error
	domain error
}

func (t tagged) Error() string   { return t.err.Error() }
func (t tagged) Unwrap() []error { return []error{t.err, t.domain} }

// NotFound tags err as a missing-resource fault (exit 3). Returns nil for nil.
func NotFound(err error) error { return tag(err, ErrNotFound) }

// Usage tags err as a caller usage fault (exit 2). Returns nil for nil.
func Usage(err error) error { return tag(err, ErrUsage) }

// UserConfig tags err as an invalid-configuration fault (exit 78). Returns nil
// for nil.
func UserConfig(err error) error { return tag(err, ErrUserConfig) }

func tag(err, domain error) error {
	if err == nil {
		return nil
	}
	return tagged{err: err, domain: domain}
}
