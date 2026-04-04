package harness

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// FreePort returns a free TCP address on localhost.
func FreePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("failed to get free port: %v", err))
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

// GenerateSSHKey generates an ed25519 SSH key pair in dir.
func GenerateSSHKey(dir string) (pubKeyPath, privKeyPath string, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate key: %w", err)
	}

	signer, err := ssh.NewSignerFromKey(privKey)
	if err != nil {
		return "", "", fmt.Errorf("new signer: %w", err)
	}

	// Marshal private key in OpenSSH format
	privPEM, err := ssh.MarshalPrivateKey(privKey, "")
	if err != nil {
		return "", "", fmt.Errorf("marshal private key: %w", err)
	}

	privKeyPath = filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(privKeyPath, pem.EncodeToMemory(privPEM), 0600); err != nil {
		return "", "", fmt.Errorf("write private key: %w", err)
	}

	// Marshal public key in authorized_keys format
	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return "", "", fmt.Errorf("new public key: %w", err)
	}
	_ = signer // used indirectly via sshPub

	pubKeyPath = filepath.Join(dir, "id_ed25519.pub")
	if err := os.WriteFile(pubKeyPath, ssh.MarshalAuthorizedKey(sshPub), 0644); err != nil {
		return "", "", fmt.Errorf("write public key: %w", err)
	}

	return pubKeyPath, privKeyPath, nil
}

// Daemon represents a running combine server.
type Daemon struct {
	SSHAddr     string
	HTTPAddr    string
	DataDir     string
	PrivKeyPath string
	cmd         *exec.Cmd
	stderr      *bytes.Buffer
}

// StartDaemon starts a combine server and waits for it to become ready.
func StartDaemon(t *testing.T, binary string) *Daemon {
	t.Helper()

	dataDir := t.TempDir()

	pubKeyPath, privKeyPath, err := GenerateSSHKey(dataDir)
	if err != nil {
		t.Fatalf("generate ssh key: %v", err)
	}

	pubKeyBytes, err := os.ReadFile(pubKeyPath)
	if err != nil {
		t.Fatalf("read public key: %v", err)
	}

	sshAddr := FreePort()
	httpAddr := FreePort()

	cmd := exec.Command(binary, "serve")
	cmd.Env = append(os.Environ(),
		"COMBINE_DATA_PATH="+dataDir,
		"COMBINE_INITIAL_ADMIN_KEYS="+strings.TrimSpace(string(pubKeyBytes)),
		"COMBINE_SSH_LISTEN_ADDR="+sshAddr,
		"COMBINE_HTTP_LISTEN_ADDR="+httpAddr,
		"COMBINE_STATS_ENABLED=false",
		"COMBINE_TESTRUN=true",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}

	d := &Daemon{
		SSHAddr:     sshAddr,
		HTTPAddr:    httpAddr,
		DataDir:     dataDir,
		PrivKeyPath: privKeyPath,
		cmd:         cmd,
		stderr:      &stderr,
	}

	// Poll for readiness
	readyURL := fmt.Sprintf("http://%s/v1/health", httpAddr)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(readyURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				goto ready
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	// Not ready - kill and fail
	cmd.Process.Kill()
	cmd.Wait()
	t.Fatalf("daemon not ready after 15s, stderr:\n%s", stderr.String())

ready:
	t.Cleanup(func() {
		cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			<-done
		}
		if strings.Contains(stderr.String(), "DATA RACE") {
			t.Errorf("data race detected in daemon stderr:\n%s", stderr.String())
		}
	})

	return d
}

// GitInit initializes a git repository in dir.
func GitInit(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, nil, "init")
	runGit(t, dir, nil, "config", "user.email", "test@test.com")
	runGit(t, dir, nil, "config", "user.name", "Test")
	runGit(t, dir, nil, "checkout", "-b", "main")
}

// GitAddCommit adds a file and commits.
func GitAddCommit(t *testing.T, dir, filename, content, message string) {
	t.Helper()
	if content != "" {
		path := filepath.Join(dir, filename)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	runGit(t, dir, nil, "add", ".")
	runGit(t, dir, nil, "commit", "-m", message)
}

// GitAddRemote adds a remote to the repo.
func GitAddRemote(t *testing.T, dir, name, url string) {
	t.Helper()
	runGit(t, dir, nil, "remote", "add", name, url)
}

// GitPush pushes via SSH with the given key.
func GitPush(t *testing.T, dir, privKeyPath string, args ...string) {
	t.Helper()
	env := sshEnv(privKeyPath)
	a := append([]string{"push"}, args...)
	runGit(t, dir, env, a...)
}

// GitPushExpectFail pushes via SSH and expects failure.
func GitPushExpectFail(t *testing.T, dir, privKeyPath string, args ...string) {
	t.Helper()
	env := sshEnv(privKeyPath)
	a := append([]string{"push"}, args...)
	runGitExpectFail(t, dir, env, a...)
}

// GitCloneSSH clones a repo over SSH.
func GitCloneSSH(t *testing.T, url, privKeyPath, destDir string) {
	t.Helper()
	env := sshEnv(privKeyPath)
	cmd := exec.Command("git", "clone", url, destDir)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone ssh failed: %v\n%s", err, out)
	}
}

// GitCloneSSHExpectFail clones over SSH and expects failure.
func GitCloneSSHExpectFail(t *testing.T, url, privKeyPath, destDir string) {
	t.Helper()
	env := sshEnv(privKeyPath)
	cmd := exec.Command("git", "clone", url, destDir)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected git clone ssh to fail, but it succeeded:\n%s", out)
	}
}

// GitCloneHTTP clones a repo over HTTP.
func GitCloneHTTP(t *testing.T, url, destDir string) {
	t.Helper()
	cmd := exec.Command("git", "clone", url, destDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git clone http failed: %v\n%s", err, out)
	}
}

// GitCloneHTTPExpectFail clones over HTTP and expects failure.
func GitCloneHTTPExpectFail(t *testing.T, url, destDir string) {
	t.Helper()
	cmd := exec.Command("git", "clone", url, destDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected git clone http to fail, but it succeeded:\n%s", out)
	}
}

// GitLog returns the git log output.
func GitLog(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "log")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	return string(out)
}

func sshEnv(privKeyPath string) []string {
	return []string{
		fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null", privKeyPath),
	}
}

func runGit(t *testing.T, dir string, extraEnv []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func runGitExpectFail(t *testing.T, dir string, extraEnv []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if extraEnv != nil {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected git %v to fail, but it succeeded:\n%s", args, out)
	}
}
