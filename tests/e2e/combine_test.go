package e2e

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Work-Fort/combine-e2e/harness"
)

var combineBin string

func TestMain(m *testing.M) {
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		panic("abs: " + err.Error())
	}

	tmpDir, err := os.MkdirTemp(projectRoot, ".e2e-bin-*")
	if err != nil {
		panic("mktemp: " + err.Error())
	}

	combineBin = filepath.Join(tmpDir, "combine")

	cmd := exec.Command("go", "build", "-race", "-o", combineBin, "./cmd/combine/")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		panic("build failed: " + err.Error())
	}

	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func sshPort(addr string) string {
	_, port, _ := net.SplitHostPort(addr)
	return port
}

func TestHealth(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	for _, path := range []string{"/readyz", "/livez"} {
		resp, err := http.Get(fmt.Sprintf("http://%s%s", d.HTTPAddr, path))
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d, want 200", path, resp.StatusCode)
		}
	}
}

func TestSSHPushCreatesRepo(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	repoDir := t.TempDir()
	harness.GitInit(t, repoDir)
	harness.GitAddCommit(t, repoDir, "hello.txt", "hello world\n", "initial commit")
	harness.GitAddRemote(t, repoDir, "origin",
		fmt.Sprintf("ssh://127.0.0.1:%s/test-repo", sshPort(d.SSHAddr)))
	harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

	repoPath := filepath.Join(d.DataDir, "repos", "test-repo.git")
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		t.Fatalf("repo dir %s does not exist after push", repoPath)
	}
}

func TestSSHClone(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	// Push a repo
	repoDir := t.TempDir()
	harness.GitInit(t, repoDir)
	harness.GitAddCommit(t, repoDir, "hello.txt", "hello world\n", "initial commit")
	harness.GitAddRemote(t, repoDir, "origin",
		fmt.Sprintf("ssh://127.0.0.1:%s/ssh-clone-test", sshPort(d.SSHAddr)))
	harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

	// Clone it back
	cloneDir := filepath.Join(t.TempDir(), "clone")
	harness.GitCloneSSH(t,
		fmt.Sprintf("ssh://127.0.0.1:%s/ssh-clone-test", sshPort(d.SSHAddr)),
		d.PrivKeyPath, cloneDir)

	content, err := os.ReadFile(filepath.Join(cloneDir, "hello.txt"))
	if err != nil {
		t.Fatalf("read hello.txt: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Errorf("hello.txt content = %q, want %q", content, "hello world\n")
	}
}

func TestSSHPushUpdate(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	repoDir := t.TempDir()
	harness.GitInit(t, repoDir)
	harness.GitAddCommit(t, repoDir, "first.txt", "first\n", "first commit")
	harness.GitAddRemote(t, repoDir, "origin",
		fmt.Sprintf("ssh://127.0.0.1:%s/push-update-test", sshPort(d.SSHAddr)))
	harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

	harness.GitAddCommit(t, repoDir, "second.txt", "second\n", "second commit")
	harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

	// Clone fresh and verify
	cloneDir := filepath.Join(t.TempDir(), "clone")
	harness.GitCloneSSH(t,
		fmt.Sprintf("ssh://127.0.0.1:%s/push-update-test", sshPort(d.SSHAddr)),
		d.PrivKeyPath, cloneDir)

	for _, f := range []string{"first.txt", "second.txt"} {
		if _, err := os.Stat(filepath.Join(cloneDir, f)); os.IsNotExist(err) {
			t.Errorf("file %s not found in clone", f)
		}
	}

	log := harness.GitLog(t, cloneDir)
	if count := strings.Count(log, "commit "); count != 2 {
		t.Errorf("expected 2 commits in log, got %d:\n%s", count, log)
	}
}

func TestHTTPClone(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	// Push over SSH
	repoDir := t.TempDir()
	harness.GitInit(t, repoDir)
	harness.GitAddCommit(t, repoDir, "hello.txt", "hello world\n", "initial commit")
	harness.GitAddRemote(t, repoDir, "origin",
		fmt.Sprintf("ssh://127.0.0.1:%s/http-clone-test", sshPort(d.SSHAddr)))
	harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

	// Clone over HTTP
	cloneDir := filepath.Join(t.TempDir(), "clone")
	harness.GitCloneHTTP(t,
		fmt.Sprintf("http://%s/http-clone-test.git", d.HTTPAddr), cloneDir)

	content, err := os.ReadFile(filepath.Join(cloneDir, "hello.txt"))
	if err != nil {
		t.Fatalf("read hello.txt: %v", err)
	}
	if string(content) != "hello world\n" {
		t.Errorf("hello.txt content = %q, want %q", content, "hello world\n")
	}
}

func TestHTTPCloneNonExistent(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	cloneDir := filepath.Join(t.TempDir(), "clone")
	harness.GitCloneHTTPExpectFail(t,
		fmt.Sprintf("http://%s/nonexistent.git", d.HTTPAddr), cloneDir)
}

func TestSSHCloneNonExistent(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	cloneDir := filepath.Join(t.TempDir(), "clone")
	harness.GitCloneSSHExpectFail(t,
		fmt.Sprintf("ssh://127.0.0.1:%s/nonexistent", sshPort(d.SSHAddr)),
		d.PrivKeyPath, cloneDir)
}

func TestSSHPushUnauthorized(t *testing.T) {
	d := harness.StartDaemon(t, combineBin)

	// Generate a second key that is NOT an admin key
	keyDir := t.TempDir()
	_, badKeyPath, err := harness.GenerateSSHKey(keyDir)
	if err != nil {
		t.Fatalf("generate bad key: %v", err)
	}

	repoDir := t.TempDir()
	harness.GitInit(t, repoDir)
	harness.GitAddCommit(t, repoDir, "hello.txt", "hello\n", "initial")
	harness.GitAddRemote(t, repoDir, "origin",
		fmt.Sprintf("ssh://127.0.0.1:%s/unauth-test", sshPort(d.SSHAddr)))
	harness.GitPushExpectFail(t, repoDir, badKeyPath, "origin", "main")
}

func TestLFSPushPull(t *testing.T) {
	// Check if git-lfs is installed
	if _, err := exec.LookPath("git-lfs"); err != nil {
		t.Skip("git-lfs not installed")
	}

	d := harness.StartDaemon(t, combineBin)

	repoDir := t.TempDir()
	harness.GitInit(t, repoDir)

	// Setup LFS
	cmd := exec.Command("git", "lfs", "install", "--local")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git lfs install: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "lfs", "track", "*.bin")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git lfs track: %v\n%s", err, out)
	}

	harness.GitAddCommit(t, repoDir, ".gitattributes", "", "add lfs tracking")

	// Write a 1KB binary file
	binData := make([]byte, 1024)
	if _, err := rand.Read(binData); err != nil {
		t.Fatalf("generate random data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "data.bin"), binData, 0644); err != nil {
		t.Fatalf("write data.bin: %v", err)
	}
	harness.GitAddCommit(t, repoDir, "data.bin", "", "add binary file")

	// Push
	sshURL := fmt.Sprintf("ssh://127.0.0.1:%s/lfs-test", sshPort(d.SSHAddr))
	harness.GitAddRemote(t, repoDir, "origin", sshURL)
	harness.GitPush(t, repoDir, d.PrivKeyPath, "origin", "main")

	// Clone into fresh dir
	cloneDir := filepath.Join(t.TempDir(), "clone")
	harness.GitCloneSSH(t, sshURL, d.PrivKeyPath, cloneDir)

	clonedData, err := os.ReadFile(filepath.Join(cloneDir, "data.bin"))
	if err != nil {
		t.Fatalf("read cloned data.bin: %v", err)
	}
	if !bytes.Equal(clonedData, binData) {
		t.Errorf("cloned data.bin does not match original (got %d bytes, want %d)", len(clonedData), len(binData))
	}
}
