// SPDX-License-Identifier: Apache-2.0
package harness

import (
	"errors"
	"os"
	"syscall"
	"testing"
)

func TestDaemonStop_KillsProcessGroup(t *testing.T) {
	binary := os.Getenv("COMBINE_BINARY")
	if binary == "" {
		t.Skip("COMBINE_BINARY not set; run via TestMain in tests/e2e/harness/main_test.go")
	}

	// Capture pid+pgid via a closure that StartDaemon's cleanup can
	// fill, then register the ESRCH assertion BEFORE StartDaemon so
	// it runs AFTER the harness cleanup in LIFO order.
	var pid, pgid int
	t.Cleanup(func() {
		if pid == 0 {
			return // StartDaemon failed; nothing to assert
		}
		// After the harness cleanup ran, signalling the group with
		// sig 0 must report no such process.
		if err := syscall.Kill(-pgid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Fatalf("kill(-%d, 0) = %v, want ESRCH (group still has live members)", pgid, err)
		}
	})

	d := StartDaemon(t, binary)
	pid = d.cmd.Process.Pid

	// pgid must equal pid because StartDaemon sets Setpgid.
	var err error
	pgid, err = syscall.Getpgid(pid)
	if err != nil {
		t.Fatalf("Getpgid(%d): %v", pid, err)
	}
	if pgid != pid {
		t.Fatalf("daemon pgid = %d, want %d (Setpgid not set)", pgid, pid)
	}
	// Defence against the (vanishingly rare) case where the test
	// process itself is in a group whose id equals the daemon PID —
	// pgid == pid would pass spuriously.
	if pgid == os.Getpid() {
		t.Fatalf("daemon pgid (%d) equals harness pid; daemon inherited harness group", pgid)
	}
	// Use errors.Is in the cleanup above (not direct ==) because
	// syscall.Errno implements the errors.Is contract and errors.Is
	// is the idiomatic Go choice.
}
