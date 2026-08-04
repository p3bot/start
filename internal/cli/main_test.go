package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/p3bot/start/internal/orchestration"
)

// TestMain installs the execution guard for the cli test binary. The cli tests
// reach execution through orchestration.Executor, but go test runs only this
// package's TestMain — orchestration is linked in its production configuration
// with the real syscall.Exec default intact. So the guard is installed through
// orchestration's exported seam setter rather than by relying on
// orchestration's own TestMain (which does not run here).
//
// The guard makes it structurally impossible for a cli test to replace its own
// process and mask the suite's failure tally: any test that reaches the
// interactive launch path panics with a diagnostic instead of calling
// syscall.Exec.
func TestMain(m *testing.M) {
	orchestration.SetDefaultProcessReplacer(guardProcessReplacer)
	// Keep resolver-backed surfaces (get, describe, start, task) offline by
	// default: the cross-category exact tier now consults the registry, which
	// would otherwise pull the real index over the network in every content and
	// scope test. Tests that need an index inject one through the index source
	// (newResolverWithIndex).
	offlineRegistryForTests = true
	os.Exit(m.Run())
}

// guardProcessReplacer panics unconditionally rather than replace the process.
// Panic is deliberate: it aborts the test binary with a non-zero exit and a
// stack trace at the offending call site, and — unlike a returned error — it
// cannot be ignored by a caller that drops the return value. It fires only on
// a defect (a test reaching real execution), which must never appear in a
// passing suite.
func guardProcessReplacer(argv0 string, argv []string, _ []string) error {
	panic(fmt.Sprintf("test reached the process-replacing execution path: %s %v\n"+
		"No test may replace its own process; use dry-run, ExecuteWithoutReplace, "+
		"or a recorder seam. If this asserts the project-04 hit-model, skip it.",
		argv0, argv))
}
