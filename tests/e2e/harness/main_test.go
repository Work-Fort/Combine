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
