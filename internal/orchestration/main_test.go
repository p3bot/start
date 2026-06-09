package orchestration

import (
	"fmt"
	"os"
	"testing"
)

// TestMain installs the execution guard for this package's test binary by
// swapping orchestration's own process-replacement default. The guard makes it
// structurally impossible for a test to replace its own process (and so force
// exit 0, masking the suite's failure tally): any test path that reaches the
// interactive launch path panics with a diagnostic instead of calling
// syscall.Exec.
func TestMain(m *testing.M) {
	SetDefaultProcessReplacer(guardProcessReplacer)
	os.Exit(m.Run())
}

// guardProcessReplacer panics unconditionally rather than replace the process.
// Panic is deliberate: it aborts the test binary with a non-zero exit and a
// stack trace attributed to the offending test at the call site, and — unlike
// returning an error — it cannot be ignored by a caller that drops the return
// value. It fires only on a defect (a test reaching real execution), which
// must never appear in a passing suite.
func guardProcessReplacer(argv0 string, argv []string, _ []string) error {
	panic(fmt.Sprintf("test reached the process-replacing execution path: %s %v\n"+
		"No test may replace its own process; use dry-run, ExecuteWithoutReplace, "+
		"or a recorder seam. If this asserts the project-04 hit-model, skip it.",
		argv0, argv))
}
