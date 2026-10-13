//go:build linux

package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func TestGetIDPromptsForMissingNameOnTerminal(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)
	slave, master := openPty(t)
	if _, err := master.Write([]byte("located\n")); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := execGet(slave, "--id")
	if err != nil {
		t.Fatalf("prompted get --id: %v\nstderr: %s", err, stderr)
	}
	if stdout != "agents:located\n" {
		t.Errorf("stdout = %q, want the resolved id", stdout)
	}
	if !strings.Contains(stderr, "Enter a search query:") {
		t.Errorf("stderr missing the name prompt:\n%s", stderr)
	}
	if strings.Contains(stderr, "Select") || strings.Contains(stdout, "Select") {
		t.Errorf("missing-name prompt became a selection menu\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestGetIDAmbiguousOnTerminalDoesNotMenu(t *testing.T) {
	dir := isolateGetHome(t)
	writeGetSettings(t, dir, fieldConfig)
	slave, master := openPty(t)

	stdout, stderr, err := execGetUntil(t, slave, master, false, "--id", "helper")
	if err == nil {
		t.Fatal("expected ambiguity error on a terminal")
	}
	for _, want := range []string{"roles:helper", "tasks:helper", "category-qualified name"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error missing %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "run interactively") {
		t.Errorf("error tells the caller to run interactively: %v", err)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if strings.Contains(stderr, "Select") || strings.Contains(stderr, "Found") {
		t.Errorf("stderr opened a menu:\n%s", stderr)
	}

	stdout, stderr, err = execGetUntil(t, slave, master, true, "helper")
	if err == nil {
		t.Fatal("expected the selection menu to read stdin")
	}
	if !strings.Contains(err.Error(), "reading input") {
		t.Fatalf("menu should read a selection, got %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "Select") || !strings.Contains(stderr, "roles:helper") {
		t.Errorf("menu missing from stderr:\n%s", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

// execGetUntil runs get. When unblock is set, a call that is still reading
// stdin after a short wait has its pty master closed so the read returns.
// A call that ignores stdin must return while the master is still open.
func execGetUntil(t *testing.T, slave, master *os.File, unblock bool, args ...string) (string, string, error) {
	t.Helper()
	type result struct {
		stdout string
		stderr string
		err    error
	}
	done := make(chan result, 1)
	go func() {
		stdout, stderr, err := execGet(slave, args...)
		done <- result{stdout, stderr, err}
	}()
	// A menu blocks in ReadString, so this wait is the whole menu case. The
	// id case returns as soon as resolution finishes and must not hit it.
	wait := 2 * time.Second
	if unblock {
		wait = 300 * time.Millisecond
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case res := <-done:
		if unblock {
			t.Fatal("get returned without reading a selection")
		}
		return res.stdout, res.stderr, res.err
	case <-timer.C:
		if !unblock {
			_ = master.Close()
			res := <-done
			t.Fatalf("get --id blocked on terminal input\nstderr:\n%s\nerr: %v", res.stderr, res.err)
		}
		_ = master.Close()
		res := <-done
		return res.stdout, res.stderr, res.err
	}
}

func execGet(in io.Reader, args ...string) (string, string, error) {
	cmd := NewRootCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(in)
	cmd.SetArgs(append([]string{"get"}, args...))
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func openPty(t *testing.T) (slave, master *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("opening ptmx: %v", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	// Linux grants the slave on open of /dev/ptmx. TIOCSPTLCK 0 unlocks it.
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	if !term.IsTerminal(int(slave.Fd())) {
		t.Fatal("pty slave is not a terminal")
	}
	return slave, master
}
