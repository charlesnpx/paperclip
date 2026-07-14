package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDelegateInstallIgnoresInvalidPapercutHome(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "run", "./cmd/papercut", "delegate-install", "--plan", "--target", "codex", "--json")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PAPERCUT_HOME=relative")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("delegate install failed: %v\n%s", err, string(out))
	}
	if !strings.Contains(string(out), `"operation":"plan"`) {
		t.Fatalf("unexpected output: %s", string(out))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
