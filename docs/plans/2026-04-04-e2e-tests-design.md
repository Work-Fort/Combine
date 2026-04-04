# E2E Test Suite Design

## Overview

End-to-end test program for Combine, structured as a separate Go module
following the Nexus/Sharkfin convention. Validates core Git functionality
across SSH and HTTP transports by running the actual `combine` binary as a
subprocess.

Primary purpose: serve as a safety net for the hexagonal architecture
migration. Tests pass against the current codebase, then again after migration
to verify nothing broke.

## Structure

```
tests/e2e/
  go.mod            # module github.com/Work-Fort/combine-e2e
  go.sum
  combine_test.go   # test suite
  harness/
    harness.go      # Daemon lifecycle, port allocation, Git/SSH helpers
```

Separate `go.mod` means the E2E tests have no import dependency on Combine's
internal packages. They interact only through the binary, Git protocol, and
HTTP — exactly like a real user.

## Harness

### TestMain

Builds the `combine` binary with `-race` into a temp directory before any
tests run. Binary path stored in a package-level variable. Temp dir cleaned
up after all tests complete.

```go
func TestMain(m *testing.M) {
    tmpDir, _ := os.MkdirTemp(".", ".e2e-bin-*")
    cmd := exec.Command("go", "build", "-race", "-o",
        filepath.Join(tmpDir, "combine"), "../../cmd/combine/")
    // ...
    code := m.Run()
    os.RemoveAll(tmpDir)
    os.Exit(code)
}
```

### StartDaemon

Spawns `combine serve` as a child process with full isolation:

- Creates a temp data directory per daemon
- Generates an ed25519 SSH keypair for the test admin
- Sets environment:
  - `COMBINE_DATA_PATH` → temp dir
  - `COMBINE_INITIAL_ADMIN_KEYS` → test public key
  - `COMBINE_SSH_LISTEN_ADDR` → `:0` or random free port
  - `COMBINE_HTTP_LISTEN_ADDR` → `:0` or random free port
- Captures stderr for race detection
- Polls `/readyz` over HTTP until ready (10s timeout)
- Returns `*Daemon` with SSH/HTTP addresses, data dir, cleanup

Cleanup via `t.Cleanup`: sends SIGTERM, waits up to 10s, checks stderr for
`DATA RACE`, removes temp dir.

### FreePort

Standard port-0-listen-close trick to get an available port.

### Git Helpers

Thin wrappers around `exec.Command("git", ...)` that set appropriate
environment variables and fail the test on error:

- `GitInit(t, dir)` — `git init` in a temp directory
- `GitAddCommit(t, dir, filename, content, message)` — write file, add, commit
- `GitCloneSSH(t, sshURL, keyPath, dir)` — clone with `GIT_SSH_COMMAND`
- `GitCloneHTTP(t, httpURL, dir)` — clone over HTTP
- `GitPush(t, dir, keyPath)` — push with SSH key
- `GitLog(t, dir)` — return commit log output
- `GitAddRemote(t, dir, name, url)` — add a remote

All SSH operations use `GIT_SSH_COMMAND` pointing to the test private key with
`-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null`.

### HTTP Client

Minimal — just `http.Get` against the health endpoints. No REST API client
needed (Combine doesn't have a management API yet).

## Test Scenarios

### TestHealth

Start daemon. `GET /readyz` → 200. `GET /livez` → 200.

### TestSSHPushCreatesRepo

Init local repo, add `hello.txt`, commit. Add SSH remote pointing to
`ssh://localhost:{port}/test-repo`. Push. Verify push succeeds (exit code 0).

### TestSSHClone

After pushing a repo, clone it from `ssh://localhost:{port}/test-repo` into
a fresh directory. Verify `hello.txt` exists with correct content.

### TestSSHPushUpdate

Clone repo, add `second.txt`, commit, push. Clone into another fresh
directory. Verify both `hello.txt` and `second.txt` present.

### TestHTTPClone

After pushing a repo over SSH, clone from
`http://localhost:{port}/test-repo.git`. Verify file content matches.

### TestHTTPCloneNonExistent

`git clone http://localhost:{port}/nonexistent.git` — should fail with a
non-zero exit code.

### TestSSHCloneNonExistent

`git clone ssh://localhost:{port}/nonexistent` — should fail.

### TestSSHPushUnauthorized

Generate a second SSH keypair that is NOT in the admin keys. Try to push
using that key. Should be rejected.

### TestLFSPushPull

Init repo, configure LFS tracking (`git lfs track "*.bin"`), add a binary
file, commit, push over SSH. Clone into a fresh directory (LFS pulls via
HTTP automatically). Verify the binary file content matches.

Note: this test depends on `git-lfs` being installed on the test machine.
Skip gracefully if not present.

## What's Not Tested

- HTTP push (requires user/token management which is stripped)
- User management (no admin CLI commands beyond migrate/sync-hooks)
- Webhooks (would need an HTTP callback receiver in the test harness)
- Mirror repos (requires external Git server)
- Git daemon protocol (scheduled for removal)

These can be added as the product evolves and gains a REST API.
