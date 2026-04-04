# E2E Test Suite Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build an end-to-end test suite that validates core Git functionality across SSH and HTTP transports, serving as a safety net for the hexagonal architecture migration.

**Architecture:** Separate Go module at `tests/e2e/` with a harness package that builds the `combine` binary, spawns it as a subprocess with isolated data directories, and exercises Git operations via `exec.Command("git", ...)`. Follows the Nexus/Sharkfin E2E convention.

**Tech Stack:** Go standard testing, exec.Command for git/ssh, net/http for health checks, crypto/ed25519 for test SSH keys

---

### Task 1: Scaffold the E2E module and harness

**Files:**
- Create: `tests/e2e/go.mod`
- Create: `tests/e2e/harness/harness.go`
- Create: `tests/e2e/combine_test.go`

**Step 1: Create the module**

```bash
mkdir -p tests/e2e/harness
```

Create `tests/e2e/go.mod`:
```
module github.com/Work-Fort/combine-e2e

go 1.24.2
```

**Step 2: Write the harness**

Create `tests/e2e/harness/harness.go` with:

1. `FreePort() (string, error)` — listen on `:0`, return `"127.0.0.1:{port}"`.

2. `GenerateSSHKey(dir string) (pubKeyPath, privKeyPath string, err error)` —
   generate ed25519 keypair using `crypto/ed25519`, write OpenSSH-format
   private key and `authorized_keys`-format public key to files in `dir`.

3. `Daemon` struct:
   ```go
   type Daemon struct {
       SSHAddr    string    // "127.0.0.1:port"
       HTTPAddr   string    // "127.0.0.1:port"
       DataDir    string    // temp data directory
       PrivKeyPath string   // path to test SSH private key
       cmd        *exec.Cmd
       stderrBuf  *bytes.Buffer
   }
   ```

4. `StartDaemon(t *testing.T, binary string, opts ...DaemonOption) *Daemon`:
   - Create temp dir for data (`t.TempDir()`)
   - Call `GenerateSSHKey` in temp dir
   - Read public key file content
   - Get two free ports (SSH + HTTP)
   - Set environment variables:
     ```
     COMBINE_DATA_PATH=<tempdir>
     COMBINE_INITIAL_ADMIN_KEYS=<pubkey content>
     COMBINE_SSH_LISTEN_ADDR=127.0.0.1:<sshPort>
     COMBINE_HTTP_LISTEN_ADDR=127.0.0.1:<httpPort>
     COMBINE_GIT_LISTEN_ADDR=  (empty string to disable git daemon)
     COMBINE_GIT_ENABLED=false
     COMBINE_STATS_LISTEN_ADDR=127.0.0.1:0
     COMBINE_TESTRUN=true
     ```
   - Start `combine serve` as subprocess, capture stderr
   - Poll `http://{httpAddr}/readyz` every 100ms, up to 15s timeout
   - Register `t.Cleanup` that sends SIGTERM, waits 10s, checks stderr for
     `DATA RACE`
   - Return `*Daemon`

5. `DaemonOption` functional options:
   ```go
   type DaemonOption func(*daemonConfig)
   type daemonConfig struct {
       extraEnv []string
   }
   ```

6. Git helper functions (all take `*testing.T` and `t.Fatal` on error):
   ```go
   func GitInit(t *testing.T, dir string)
   func GitAddCommit(t *testing.T, dir, filename, content, message string)
   func GitAddRemote(t *testing.T, dir, name, url string)
   func GitPush(t *testing.T, dir string, privKeyPath string, args ...string)
   func GitCloneSSH(t *testing.T, url, privKeyPath, destDir string)
   func GitCloneHTTP(t *testing.T, url, destDir string)
   func GitLog(t *testing.T, dir string) string
   ```

   All SSH operations use:
   ```go
   cmd.Env = append(os.Environ(),
       fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", privKeyPath),
   )
   ```

**Step 3: Write TestMain and a placeholder test**

Create `tests/e2e/combine_test.go`:
```go
package e2e

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
)

var combineBin string

func TestMain(m *testing.M) {
    // Find project root (two levels up from tests/e2e/)
    projectRoot, err := filepath.Abs("../..")
    if err != nil {
        panic(err)
    }

    // Build binary with race detector
    tmpDir, err := os.MkdirTemp(projectRoot, ".e2e-bin-*")
    if err != nil {
        panic(err)
    }

    combineBin = filepath.Join(tmpDir, "combine")
    cmd := exec.Command("go", "build", "-race", "-o", combineBin, "./cmd/combine/")
    cmd.Dir = projectRoot
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        os.RemoveAll(tmpDir)
        panic("failed to build combine: " + err.Error())
    }

    code := m.Run()
    os.RemoveAll(tmpDir)
    os.Exit(code)
}
```

**Step 4: Verify the module builds**

```bash
cd tests/e2e && go build ./...
```

**Step 5: Commit**

```bash
git add tests/e2e/
git commit -m "feat: scaffold E2E test module with harness"
```

---

### Task 2: TestHealth

**Files:**
- Modify: `tests/e2e/combine_test.go`

**Step 1: Write the test**

```go
func TestHealth(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    // Test liveness
    resp, err := http.Get("http://" + d.HTTPAddr + "/livez")
    if err != nil {
        t.Fatalf("livez request failed: %v", err)
    }
    resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Errorf("livez status = %d, want 200", resp.StatusCode)
    }

    // Test readiness
    resp, err = http.Get("http://" + d.HTTPAddr + "/readyz")
    if err != nil {
        t.Fatalf("readyz request failed: %v", err)
    }
    resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        t.Errorf("readyz status = %d, want 200", resp.StatusCode)
    }
}
```

**Step 2: Run the test**

```bash
cd tests/e2e && go test -v -run TestHealth -timeout 60s
```

Expected: PASS — daemon starts, health endpoints return 200.

**Step 3: Commit**

```bash
git add tests/e2e/combine_test.go
git commit -m "test: add TestHealth E2E test"
```

---

### Task 3: TestSSHPushCreatesRepo and TestSSHClone

**Files:**
- Modify: `tests/e2e/combine_test.go`

**Step 1: Write the tests**

```go
func TestSSHPushCreatesRepo(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    // Init a local repo and add a file
    localDir := t.TempDir()
    harness.GitInit(t, localDir)
    harness.GitAddCommit(t, localDir, "hello.txt", "hello world\n", "initial commit")

    // Add SSH remote and push
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/test-repo", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, localDir, "origin", sshURL)
    harness.GitPush(t, localDir, d.PrivKeyPath, "origin", "main")

    // Verify repo directory was created on disk
    repoPath := filepath.Join(d.DataDir, "repos", "test-repo.git")
    if _, err := os.Stat(repoPath); os.IsNotExist(err) {
        t.Fatalf("repo directory not created at %s", repoPath)
    }
}

func TestSSHClone(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    // Push a repo first
    localDir := t.TempDir()
    harness.GitInit(t, localDir)
    harness.GitAddCommit(t, localDir, "hello.txt", "hello world\n", "initial commit")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/clone-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, localDir, "origin", sshURL)
    harness.GitPush(t, localDir, d.PrivKeyPath, "origin", "main")

    // Clone it back
    cloneDir := t.TempDir()
    harness.GitCloneSSH(t, sshURL, d.PrivKeyPath, cloneDir)

    // Verify file content
    content, err := os.ReadFile(filepath.Join(cloneDir, "hello.txt"))
    if err != nil {
        t.Fatalf("read cloned file: %v", err)
    }
    if string(content) != "hello world\n" {
        t.Errorf("cloned content = %q, want %q", string(content), "hello world\n")
    }
}

// sshPort extracts the port from "127.0.0.1:port"
func sshPort(addr string) string {
    _, port, _ := net.SplitHostPort(addr)
    return port
}
```

**Step 2: Run**

```bash
cd tests/e2e && go test -v -run "TestSSHPush|TestSSHClone" -timeout 60s
```

**Step 3: Commit**

```bash
git commit -am "test: add TestSSHPushCreatesRepo and TestSSHClone"
```

---

### Task 4: TestSSHPushUpdate

**Files:**
- Modify: `tests/e2e/combine_test.go`

**Step 1: Write the test**

```go
func TestSSHPushUpdate(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    // Push initial repo
    localDir := t.TempDir()
    harness.GitInit(t, localDir)
    harness.GitAddCommit(t, localDir, "first.txt", "first file\n", "first commit")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/update-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, localDir, "origin", sshURL)
    harness.GitPush(t, localDir, d.PrivKeyPath, "origin", "main")

    // Add second file and push update
    harness.GitAddCommit(t, localDir, "second.txt", "second file\n", "second commit")
    harness.GitPush(t, localDir, d.PrivKeyPath, "origin", "main")

    // Fresh clone and verify both files
    cloneDir := t.TempDir()
    harness.GitCloneSSH(t, sshURL, d.PrivKeyPath, cloneDir)

    for _, f := range []struct{ name, want string }{
        {"first.txt", "first file\n"},
        {"second.txt", "second file\n"},
    } {
        content, err := os.ReadFile(filepath.Join(cloneDir, f.name))
        if err != nil {
            t.Fatalf("read %s: %v", f.name, err)
        }
        if string(content) != f.want {
            t.Errorf("%s content = %q, want %q", f.name, string(content), f.want)
        }
    }

    // Verify commit count
    logOutput := harness.GitLog(t, cloneDir)
    if count := strings.Count(logOutput, "commit "); count != 2 {
        t.Errorf("commit count = %d, want 2", count)
    }
}
```

**Step 2: Run**

```bash
cd tests/e2e && go test -v -run TestSSHPushUpdate -timeout 60s
```

**Step 3: Commit**

```bash
git commit -am "test: add TestSSHPushUpdate"
```

---

### Task 5: TestHTTPClone and error cases

**Files:**
- Modify: `tests/e2e/combine_test.go`

**Step 1: Write the tests**

```go
func TestHTTPClone(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    // Push a repo over SSH first
    localDir := t.TempDir()
    harness.GitInit(t, localDir)
    harness.GitAddCommit(t, localDir, "http-test.txt", "via http\n", "initial commit")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/http-clone-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, localDir, "origin", sshURL)
    harness.GitPush(t, localDir, d.PrivKeyPath, "origin", "main")

    // Clone over HTTP
    httpURL := fmt.Sprintf("http://%s/http-clone-test.git", d.HTTPAddr)
    cloneDir := t.TempDir()
    harness.GitCloneHTTP(t, httpURL, cloneDir)

    // Verify
    content, err := os.ReadFile(filepath.Join(cloneDir, "http-test.txt"))
    if err != nil {
        t.Fatalf("read cloned file: %v", err)
    }
    if string(content) != "via http\n" {
        t.Errorf("content = %q, want %q", string(content), "via http\n")
    }
}

func TestHTTPCloneNonExistent(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    cloneDir := t.TempDir()
    cmd := exec.Command("git", "clone",
        fmt.Sprintf("http://%s/nonexistent.git", d.HTTPAddr),
        cloneDir+"/repo")
    out, err := cmd.CombinedOutput()
    if err == nil {
        t.Fatal("expected clone of nonexistent repo to fail")
    }
    t.Logf("expected failure output: %s", string(out))
}

func TestSSHCloneNonExistent(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    cloneDir := t.TempDir()
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/nonexistent", sshPort(d.SSHAddr))
    cmd := exec.Command("git", "clone", sshURL, cloneDir+"/repo")
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
            d.PrivKeyPath),
    )
    out, err := cmd.CombinedOutput()
    if err == nil {
        t.Fatal("expected clone of nonexistent repo to fail")
    }
    t.Logf("expected failure output: %s", string(out))
}
```

**Step 2: Run**

```bash
cd tests/e2e && go test -v -run "TestHTTPClone|TestSSHCloneNon" -timeout 60s
```

**Step 3: Commit**

```bash
git commit -am "test: add TestHTTPClone and non-existent repo error tests"
```

---

### Task 6: TestSSHPushUnauthorized

**Files:**
- Modify: `tests/e2e/combine_test.go`

**Step 1: Write the test**

```go
func TestSSHPushUnauthorized(t *testing.T) {
    d := harness.StartDaemon(t, combineBin)

    // Generate a DIFFERENT SSH key (not the admin key)
    unknownKeyDir := t.TempDir()
    _, unknownPrivKey, err := harness.GenerateSSHKey(unknownKeyDir)
    if err != nil {
        t.Fatalf("generate unknown key: %v", err)
    }

    // Try to push with the unknown key
    localDir := t.TempDir()
    harness.GitInit(t, localDir)
    harness.GitAddCommit(t, localDir, "unauthorized.txt", "should fail\n", "commit")
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/unauth-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, localDir, "origin", sshURL)

    // This should fail
    cmd := exec.Command("git", "push", "origin", "main")
    cmd.Dir = localDir
    cmd.Env = append(os.Environ(),
        fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
            unknownPrivKey),
    )
    out, err := cmd.CombinedOutput()
    if err == nil {
        t.Fatal("expected push with unauthorized key to fail")
    }
    t.Logf("expected failure output: %s", string(out))
}
```

**Step 2: Run**

```bash
cd tests/e2e && go test -v -run TestSSHPushUnauthorized -timeout 60s
```

**Step 3: Commit**

```bash
git commit -am "test: add TestSSHPushUnauthorized"
```

---

### Task 7: TestLFSPushPull

**Files:**
- Modify: `tests/e2e/combine_test.go`

**Step 1: Write the test**

This test depends on `git-lfs` being installed. Skip if not available.

```go
func TestLFSPushPull(t *testing.T) {
    // Skip if git-lfs is not installed
    if _, err := exec.LookPath("git-lfs"); err != nil {
        t.Skip("git-lfs not installed, skipping LFS test")
    }

    d := harness.StartDaemon(t, combineBin)

    // Init repo with LFS tracking
    localDir := t.TempDir()
    harness.GitInit(t, localDir)

    // Install LFS in the repo
    cmd := exec.Command("git", "lfs", "install", "--local")
    cmd.Dir = localDir
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("git lfs install: %v\n%s", err, out)
    }

    // Track *.bin files with LFS
    cmd = exec.Command("git", "lfs", "track", "*.bin")
    cmd.Dir = localDir
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("git lfs track: %v\n%s", err, out)
    }

    // Add .gitattributes and a binary file
    harness.GitAddCommit(t, localDir, ".gitattributes", "",  "") // stage what lfs track created
    // Write a binary file large enough to be LFS-tracked
    binContent := make([]byte, 1024)
    for i := range binContent {
        binContent[i] = byte(i % 256)
    }
    os.WriteFile(filepath.Join(localDir, "data.bin"), binContent, 0644)
    harness.GitAddCommit(t, localDir, "data.bin", "", "add LFS tracked binary")

    // Push over SSH
    sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/lfs-test", sshPort(d.SSHAddr))
    harness.GitAddRemote(t, localDir, "origin", sshURL)
    harness.GitPush(t, localDir, d.PrivKeyPath, "origin", "main")

    // Clone into fresh directory (LFS should auto-pull via HTTP)
    cloneDir := t.TempDir()
    harness.GitCloneSSH(t, sshURL, d.PrivKeyPath, cloneDir)

    // Verify binary content matches
    clonedContent, err := os.ReadFile(filepath.Join(cloneDir, "data.bin"))
    if err != nil {
        t.Fatalf("read cloned binary: %v", err)
    }
    if !bytes.Equal(clonedContent, binContent) {
        t.Errorf("LFS binary content mismatch: got %d bytes, want %d bytes",
            len(clonedContent), len(binContent))
    }
}
```

Note: The `GitAddCommit` helper may need adjustment for LFS — the `.gitattributes`
file is already created by `git lfs track`, so the helper should handle the case
where the file already exists (just `git add` + `git commit` without writing).
The implementing agent should handle this by either:
- Making `GitAddCommit` skip the write if content is empty
- Or using raw git commands in this test

**Step 2: Run**

```bash
cd tests/e2e && go test -v -run TestLFSPushPull -timeout 120s
```

LFS tests may take longer due to the LFS transfer protocol negotiation.

**Step 3: Commit**

```bash
git commit -am "test: add TestLFSPushPull"
```

---

### Task 8: Run full suite and verify

**Step 1: Run all E2E tests**

```bash
cd tests/e2e && go test -v -race -timeout 180s
```

All 9 tests should pass. Expected output:
```
=== RUN   TestHealth
--- PASS: TestHealth
=== RUN   TestSSHPushCreatesRepo
--- PASS: TestSSHPushCreatesRepo
=== RUN   TestSSHClone
--- PASS: TestSSHClone
=== RUN   TestSSHPushUpdate
--- PASS: TestSSHPushUpdate
=== RUN   TestHTTPClone
--- PASS: TestHTTPClone
=== RUN   TestHTTPCloneNonExistent
--- PASS: TestHTTPCloneNonExistent
=== RUN   TestSSHCloneNonExistent
--- PASS: TestSSHCloneNonExistent
=== RUN   TestSSHPushUnauthorized
--- PASS: TestSSHPushUnauthorized
=== RUN   TestLFSPushPull
--- PASS: TestLFSPushPull (or SKIP if git-lfs not installed)
PASS
```

**Step 2: Fix any failures**

If tests fail, debug and fix. Common issues:
- SSH key format: ensure the private key is in OpenSSH format (not PEM)
- Port conflicts: ensure `FreePort` actually returns unused ports
- Git config: may need `git config user.email` and `user.name` in test repos
- Race conditions: daemon may need longer startup time

**Step 3: Commit any fixes**

```bash
git commit -am "test: fix E2E test issues"
```

---

### Task 9: Update remaining-work.md

**Files:**
- Modify: `docs/remaining-work.md`

**Step 1: Mark E2E items as complete**

Update the E2E Test Suite section to mark all items with `[x]`.

**Step 2: Commit**

```bash
git add docs/remaining-work.md
git commit -m "docs: mark E2E test suite as complete"
```
