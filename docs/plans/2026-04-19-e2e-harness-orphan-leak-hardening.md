---
type: plan
step: "1"
title: "combine e2e harness — orphan-leak hardening"
status: pending
assessment_status: complete
provenance:
  source: roadmap
  issue_id: null
  roadmap_step: null
dates:
  created: "2026-04-19"
  approved: null
  completed: null
related_plans: []
---

# Combine E2E Harness — Orphan-Leak Hardening

**Goal:** Stop the e2e harness from leaking orphan processes when the
`combine serve` subprocess exits before its stderr buffer drains.
The current `tests/e2e/harness/harness.go` wires `cmd.Stderr = &stderr`
(a `bytes.Buffer`) at line 117-118, which makes `exec.Cmd` open an OS
pipe and a copy goroutine. The cleanup at line 154-170 sends SIGTERM
to the daemon PID only — leaked descendants (git hooks, future SSH
session children) inherit the pipe write end and keep `cmd.Wait()`
blocked until the workflow timeout fires.

**Canonical fix** (see `/home/kazw/Work/WorkFort/skills/lead/go-service-architecture/references/architecture-reference.md` — section
"Orphan-Process Hardening (Required)"):

1. **`Setpgid: true`** in `cmd.SysProcAttr`.
2. **`*os.File` for stdout/stderr**, not `bytes.Buffer`.
3. **Negative-pid kill** (`syscall.Kill(-pgid, sig)`).
4. **`cmd.WaitDelay = 10 * time.Second`** safety net.

All four parts are load-bearing.

**Repo specifics.** Combine spawns `combine serve` once per test from
inside a `t.Cleanup` closure that lives inside `StartDaemon`. The
harness has no nested module Go version concerns — `tests/e2e/go.mod`
is on Go 1.24.2 so `cmd.WaitDelay` (Go 1.20+) is available. The
spawn block doesn't have a separate stop method; the inline closure
needs the same four-part treatment. We also extract a small
`stopDaemon` helper because the cleanup is non-trivial and needs the
process-group pgid in two places.

**Tech stack:** Go 1.24.2 (e2e nested module), `os/exec`, `syscall`.
No new dependencies.

**Commands:** Combine has no mise task runner. Run e2e tests with
`go test -count=1 -race ./...` from `tests/e2e/` (the existing pattern;
see `combine_test.go` and `mcp_bridge_test.go`). The package builds
the binary itself in `TestMain`, so no separate build step is required.

---

## Prerequisites

- `tests/e2e/go.mod` (Go 1.24.2) is unchanged.
- `combine` binary is built in `TestMain` per the existing harness
  setup; this plan does not change that.
- The harness package has no other consumers — only `tests/e2e/`.

---

## Conventions

- Run all e2e tests with `cd tests/e2e && go test -race -count=1 ./...`.
  Targeted tests with `go test -run TestX -count=1 ./...`.
- Commit after each task with the multi-line conventional-commits
  HEREDOC and the Co-Authored-By trailer below.

```bash
git add <files>
git commit -m "$(cat <<'EOF'
<type>(<scope>): <description>

<body explaining why, not what>

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task Breakdown

### Task 1: Add a `TestMain` to the harness package and write the failing leak-detection test

**Files:**
- Create: `tests/e2e/harness/main_test.go` — builds the combine
  binary once for the harness package and exposes its path via
  `COMBINE_BINARY`.
- Create: `tests/e2e/harness/daemon_leak_test.go` — the failing
  leak-detection test.

**Step 1: Add `TestMain` to the harness package**

The parent `tests/e2e/` package builds `combineBin` in its own
`TestMain` (see `tests/e2e/combine_test.go:22-47`). The harness
sub-package has no equivalent, so any test that lives in
`package harness` cannot find the binary today. Mirror the parent's
pattern with a harness-package-local `TestMain` that builds the
binary into a temp dir, sets `COMBINE_BINARY`, runs the package's
tests, and removes the temp dir on exit.

```go
// SPDX-License-Identifier: Apache-2.0
package harness

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	projectRoot, err := filepath.Abs("../../..")
	if err != nil {
		panic("abs: " + err.Error())
	}

	tmpDir, err := os.MkdirTemp(projectRoot, ".harness-bin-*")
	if err != nil {
		panic("mktemp: " + err.Error())
	}

	bin := filepath.Join(tmpDir, "combine")
	cmd := exec.Command("go", "build", "-race", "-o", bin, "./cmd/combine/")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		panic("build failed: " + err.Error())
	}

	if err := os.Setenv("COMBINE_BINARY", bin); err != nil {
		os.RemoveAll(tmpDir)
		panic("setenv: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}
```

This mirrors the parent package's `TestMain` exactly and keeps the
harness package self-contained — running
`go test ./tests/e2e/harness/...` builds the binary itself; nothing
external has to know to set `COMBINE_BINARY`.

**Step 2: Write the leak test**

The test starts a daemon, registers an ESRCH assertion as the LAST
`t.Cleanup` (so it runs FIRST in LIFO order — i.e. AFTER `StartDaemon`'s
own cleanup which runs LATER, no — see note below), then asserts the
group is empty after the harness's cleanup has run.

Approach: `StartDaemon` registers its own `t.Cleanup` to stop the
daemon (LIFO). To make the leak assertion run AFTER the harness's
cleanup, use a `t.Cleanup` registered in `t.Run`'s subtest scope so
its lifetime is decoupled — but the simpler trick is to call the
harness cleanup explicitly via `t.Run` ordering control. The
cleanest pattern: register the assertion via `t.Cleanup` AFTER
calling `StartDaemon`. `t.Cleanup` runs LIFO, so the assertion
registered second (the leak check) runs FIRST — which is the wrong
order. The right pattern is: capture pgid before the harness
cleanup, then assert in a final cleanup that runs LAST.

A `t.Cleanup` registered EARLIER runs LATER (LIFO). So register the
ESRCH assertion BEFORE calling `StartDaemon` — it will run after the
harness's own cleanup pops off. That gives us deterministic ordering
without any wrapper:

```go
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
```

This avoids a `fakeT` wrapper entirely. `t.Cleanup` ordering is
deterministic LIFO: the ESRCH assertion is registered first, so it
runs last — after `StartDaemon`'s own `t.Cleanup` has stopped the
daemon and reaped the group.

**Step 3: Run the test to verify it fails**

Run from `combine/lead/`:

```
cd tests/e2e && go test -run TestDaemonStop_KillsProcessGroup -count=1 ./harness/...
```

`TestMain` in the harness package builds the binary and sets
`COMBINE_BINARY` automatically. Expected: FAIL with `daemon pgid =
<harness_pgid>, want <daemon_pid> (Setpgid not set)`.

**Step 4: Commit the failing test plus TestMain**

```bash
git add tests/e2e/harness/main_test.go tests/e2e/harness/daemon_leak_test.go
git commit -m "$(cat <<'EOF'
test(e2e): add harness TestMain and failing leak test

Adds a TestMain to the harness sub-package that builds combine into
a temp dir and exposes the binary path via COMBINE_BINARY, mirroring
the parent e2e package's TestMain. Adds TestDaemonStop_KillsProcessGroup
which asserts the daemon spawns into its own process group and that
the harness cleanup empties the group. Currently fails because
StartDaemon does not set Setpgid; the next task fixes the harness.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Apply the four-part canonical fix to `StartDaemon`

**Depends on:** Task 1

**Files:**
- Modify: `tests/e2e/harness/harness.go` — `Daemon` struct (line
  72-81), `StartDaemon` (line 84-173), and a new `stopDaemon` helper.

**Step 1: Replace the `Daemon` struct's stderr field**

The current struct (lines 72-81) holds `stderr *bytes.Buffer`. Replace
that field with `stderrFile *os.File`. `bytes` import stays for the
read-back DATA RACE check.

```go
// Daemon represents a running combine server.
type Daemon struct {
	SSHAddr     string
	HTTPAddr    string
	DataDir     string
	PrivKeyPath string
	SignJWT     func(id, username, displayName, userType string) string
	cmd         *exec.Cmd
	stderrFile  *os.File // *os.File (not bytes.Buffer) — see hardening notes
	jwksStop    func()
}
```

**Step 2: Rewrite the spawn block**

In `StartDaemon`, replace lines 117-122 (the `var stderr bytes.Buffer`,
`cmd.Stderr = &stderr`, and `cmd.Start` block) with:

```go
	stderrFile, err := os.CreateTemp("", "combine-e2e-stderr-*")
	if err != nil {
		t.Fatalf("create stderr temp file: %v", err)
	}
	// *os.File (not bytes.Buffer) so exec.Cmd does not create a copy
	// goroutine; Setpgid puts the daemon and any descendants in a
	// fresh process group; WaitDelay force-closes any inherited fds
	// after the daemon exits. See the orphan-process hardening
	// section of go-service-architecture.
	cmd.Stdout = stderrFile
	cmd.Stderr = stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 10 * time.Second

	if err := cmd.Start(); err != nil {
		stderrFile.Close()
		os.Remove(stderrFile.Name())
		t.Fatalf("start daemon: %v", err)
	}
```

`bytes` and `io` imports may now be unused — leave `bytes` (we'll
need it for `bytes.Contains` below) and remove `io` if no other code
in the file references it. Quick check: `grep -n "io\." harness.go`.

**Step 3: Update the `Daemon` literal**

Replace lines 124-133 (the `d := &Daemon{...}` literal) with:

```go
	d := &Daemon{
		SSHAddr:     sshAddr,
		HTTPAddr:    httpAddr,
		DataDir:     dataDir,
		PrivKeyPath: privKeyPath,
		SignJWT:     signJWT,
		cmd:         cmd,
		stderrFile:  stderrFile,
		jwksStop:    jwksStop,
	}
```

**Step 4: Update the failure-path kill (`cmd.Process.Kill()` at line 149)**

Currently:

```go
	cmd.Process.Kill()
	cmd.Wait()
	t.Fatalf("daemon not ready after 15s, stderr:\n%s", stderr.String())
```

Replace with:

```go
	pgid := cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	cmd.Wait()
	stderrBytes, _ := os.ReadFile(stderrFile.Name())
	stderrFile.Close()
	os.Remove(stderrFile.Name())
	t.Fatalf("daemon not ready after 15s, stderr:\n%s", stderrBytes)
```

This path runs before the `Daemon` struct is constructed, so all
locals (`cmd`, `stderrFile`, `dataDir` from the existing
`harness.go:87`) are still in scope. No `d.xdgDir` reference here —
the struct does not exist yet.

**Step 5: Rewrite the cleanup closure (lines 154-170)**

Replace the whole `t.Cleanup(func() { ... })` block with an extracted
helper call:

```go
	t.Cleanup(func() { stopDaemon(t, d) })

	return d
}

// stopDaemon signals the daemon's process group with SIGTERM, waits
// up to 10s, then SIGKILLs the group. Reads the stderr temp file,
// reports DATA RACE if present, and removes the temp file.
func stopDaemon(t *testing.T, d *Daemon) {
	t.Helper()
	if d.cmd.Process != nil {
		pgid := d.cmd.Process.Pid
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- d.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			<-done
		}
	}
	if d.jwksStop != nil {
		d.jwksStop()
	}
	var stderrBytes []byte
	if d.stderrFile != nil {
		stderrBytes, _ = os.ReadFile(d.stderrFile.Name())
		d.stderrFile.Close()
		os.Remove(d.stderrFile.Name())
	}
	if bytes.Contains(stderrBytes, []byte("DATA RACE")) {
		t.Errorf("data race detected in daemon stderr:\n%s", stderrBytes)
	}
}
```

The `bytes` import stays in active use; remove `io` if nothing else
references it.

**Step 6: Run the leak test to verify it passes**

```
cd tests/e2e && go test -run TestDaemonStop_KillsProcessGroup \
  -count=1 ./harness/...
```

The harness `TestMain` (added in Task 1 Step 1) builds the binary and
sets `COMBINE_BINARY`. Expected: PASS. The daemon pgid equals its
PID, and the group is empty after cleanup.

**Step 7: Run the full e2e suite to verify no regression**

Run from `tests/e2e/`:

```
go test -race -count=1 ./...
```

Expected: PASS. Existing combine tests still see the daemon start,
git push/pull works, MCP bridge tests pass.

**Step 8: Commit**

```bash
git add tests/e2e/harness/harness.go
git commit -m "$(cat <<'EOF'
fix(e2e): harden daemon harness against orphan-process leaks

Spawn the combine daemon into its own process group (Setpgid),
capture stdout+stderr to an *os.File instead of a bytes.Buffer
(eliminates the copy goroutine that holds pipe fds), signal the
whole group on cleanup (kill(-pgid, ...)), and set WaitDelay so
cmd.Wait force-closes any inherited fd after the daemon exits.
Extract the cleanup into stopDaemon so the negative-pid kill and
stderr read-back live in one place.

Without this, a daemon that buffers output through a pipe — or any
descendant that inherits the write end — can leave the harness
blocked in cmd.Wait until the CI workflow timeout fires.

Implements the canonical e2e-harness orphan-leak hardening pattern
documented in skills/lead/go-service-architecture/references/architecture-reference.md
(section "Orphan-Process Hardening (Required)").

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Verify cleanup is bounded under simulated test failure

**Depends on:** Task 2

**Files:**
- (Temporary, reverted) inject `t.Fatal` into `combine_test.go`'s
  cheapest test.

**Step 1: Confirm working tree is clean**

Run `git status`. Expected: clean. The next step injects a temporary
edit; a clean tree before this step ensures revert is unambiguous.

**Step 2: Inject a forced failure**

Pick the simplest test in `combine_test.go` and add
`t.Fatal("synthetic failure to verify cleanup bound")` immediately
after `StartDaemon` returns. Do not commit this change. Optionally
`git stash push -k -m "synthetic-failure"` then `git stash pop` so
the diff is recoverable if the timing run is interrupted.

**Step 3: Time the e2e run**

Run from `combine/lead/tests/e2e/`:

```
time go test -race -count=1 -run TestSyntheticTarget ./...
```

(Use the actual test name; substitute as appropriate.) Expected:

- The synthetic test FAILs.
- Total wall clock under 30 seconds. With the fix, teardown takes
  ~1 second (SIGTERM + clean exit). Without the fix, this hangs on
  `cmd.Wait` until interrupted.

If the run exceeds 30 seconds, inspect with `ps -o pid,pgid,cmd
-p $(pgrep -f combine.*serve)` and re-check `Setpgid`, the
negative-pid kill, and `WaitDelay`.

**Step 4: Revert the synthetic failure**

`git checkout -- combine_test.go` to restore. Run `git status` and
confirm the working tree is clean.

**Step 5: Final regression run**

```
cd tests/e2e && go test -race -count=1 ./...
```

Expected: PASS, all tests green.

No commit for this task — verification only.

---

## Verification Checklist

After all tasks complete:

- [ ] `cd tests/e2e && go test -race -count=1 ./...` passes.
- [ ] `TestDaemonStop_KillsProcessGroup` (in
  `tests/e2e/harness/daemon_leak_test.go`) passes; reverting the
  `Setpgid` line in `harness.go` makes it fail with the expected
  message.
- [ ] `Daemon.stderr` is `*os.File`, not `*bytes.Buffer`.
- [ ] `cmd.SysProcAttr.Setpgid == true`, `cmd.WaitDelay == 10s`,
  `cmd.Stdout`/`cmd.Stderr` both `*os.File`.
- [ ] `stopDaemon` and the not-ready failure path use
  `syscall.Kill(-pgid, sig)`, never `cmd.Process.Signal`/
  `cmd.Process.Kill`.
- [ ] DATA RACE check still runs against the read-back stderr.
- [ ] `time go test ...` with an injected `t.Fatal` returns in
  under 30 seconds (Task 3 spot check).

## Out of Scope

- Adding mise to combine. The repo deliberately uses raw `go test`;
  this plan honours that.
- Changes outside `tests/e2e/harness/harness.go` and the new test.
- Auditing every git/SSH child path inside the daemon for orphan
  potential. Setpgid + group-kill covers descendants regardless.
