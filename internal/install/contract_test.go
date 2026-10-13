package install

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerContract(t *testing.T) {
	for _, target := range []string{"all", "claude", "codex", "tools"} {
		t.Run(target, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "stage")
			executable := filepath.Join(t.TempDir(), "paperclip")
			if err := os.WriteFile(executable, []byte("test binary"), 0o755); err != nil {
				t.Fatal(err)
			}
			options := Options{Operation: "plan", Target: target, InstallRoot: root, Executable: executable}
			plan, err := Execute(options)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("plan wrote to disk: %v", err)
			}
			options.Operation = "install"
			installed, err := Execute(options)
			if err != nil {
				t.Fatal(err)
			}
			for name, result := range installed.Targets {
				if len(result.Files) != len(plan.Targets[name].Files) {
					t.Fatalf("plan/install mismatch for %s", name)
				}
				for index, file := range result.Files {
					if file.Path != plan.Targets[name].Files[index].Path || !filepath.IsLocal(mustRelative(t, root, file.Path)) {
						t.Fatalf("unexpected path: %s", file.Path)
					}
					body, err := os.ReadFile(file.Path)
					if err != nil {
						t.Fatal(err)
					}
					hash := sha256.Sum256(body)
					if file.SHA256 != hex.EncodeToString(hash[:]) {
						t.Fatalf("incorrect hash: %s", file.Path)
					}
				}
				if name == "claude" || name == "codex" {
					body, err := os.ReadFile(filepath.Join(root, "."+name, "skills", "paperclip:skill-feedback", "SKILL.md"))
					if err != nil || string(body) != feedbackSkill {
						t.Fatalf("missing skill-feedback payload for %s: %v", name, err)
					}
				}
			}
			foreign := filepath.Join(root, "keep.txt")
			if err := os.WriteFile(foreign, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			options.Operation = "uninstall"
			removed, err := Execute(options)
			if err != nil || len(removed.Notices) != 0 {
				t.Fatalf("uninstall: %+v %v", removed, err)
			}
			for _, result := range installed.Targets {
				for _, file := range result.Files {
					if _, err := os.Stat(file.Path); !os.IsNotExist(err) {
						t.Fatalf("owned file survived uninstall: %s %v", file.Path, err)
					}
				}
			}
			if _, err := os.Stat(foreign); err != nil {
				t.Fatalf("uninstall touched unrelated file: %v", err)
			}
		})
	}
}

func mustRelative(t *testing.T, root, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return relative
}
