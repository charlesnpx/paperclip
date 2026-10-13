package install

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanAllReportsDelegatedTargetsWithoutWrites(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "paperclip")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Execute(Options{Operation: "plan", Target: "all", InstallRoot: root, Executable: exe})
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != 1 || result.Name != "paperclip" || result.Kind != "delegated" || result.Operation != "plan" {
		t.Fatalf("unexpected result: %+v", result)
	}
	for _, target := range []string{"claude", "codex"} {
		files := result.Targets[target].Files
		want := []string{
			filepath.Join(root, "."+target, "skills", "paperclip", "SKILL.md"),
			filepath.Join(root, "."+target, "skills", "paperclip:skill-feedback", "SKILL.md"),
		}
		if len(files) != len(want) || files[0].Path != want[0] || files[1].Path != want[1] {
			t.Fatalf("%s files = %+v", target, files)
		}
	}
	toolFiles := result.Targets["tools"].Files
	if len(toolFiles) != 1 {
		t.Fatalf("tools files = %d", len(toolFiles))
	}
	if toolFiles[0].Path != filepath.Join(root, ".local", "bin", "paperclip") {
		t.Fatalf("tool path = %q", toolFiles[0].Path)
	}
	if _, err := os.Stat(filepath.Join(root, ".local")); !os.IsNotExist(err) {
		t.Fatalf("plan should not create staging path, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("plan should not create codex path, stat err=%v", err)
	}
	if len(result.Notices) != 0 {
		t.Fatalf("notices = %#v", result.Notices)
	}
}

func TestSkillsAreImplicitAndWriteOnly(t *testing.T) {
	for _, body := range []string{paperclipSkill, feedbackSkill} {
		if strings.Contains(body, "allow_implicit_invocation") {
			t.Fatalf("skills must be available for implicit use:\n%s", body)
		}
		if !strings.Contains(body, "unless the user") {
			t.Fatalf("skills must forbid reading the ledger unless asked:\n%s", body)
		}
	}
	if !strings.Contains(paperclipSkill, "paperclip add") || !strings.Contains(feedbackSkill, "paperclip feedback add") {
		t.Fatal("skills must document their capture command")
	}
}

func TestCodexUninstallDoesNotEmitInstallNotice(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "paperclip")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".codex", "skills", "paperclip", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Execute(Options{Operation: "uninstall", Target: "codex", InstallRoot: root, Executable: exe})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Notices) != 0 {
		t.Fatalf("uninstall notices = %#v", result.Notices)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected skill file removed, stat err=%v", err)
	}
}

func TestWrapperWorksOutsideRepository(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(filepath.Dir(filepath.Dir(wd)), "install-skill.sh")
	cmd := exec.Command(script, "--plan", "--target", "codex", "--json")
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed: %v\n%s", err, string(out))
	}
	var result Result
	if err := json.Unmarshal(bytes.TrimSpace(out), &result); err != nil {
		t.Fatalf("decode output: %v\n%s", err, string(out))
	}
	if result.Operation != "plan" || len(result.Targets["codex"].Files) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAtomicFileWriteUsesVisibleTempAndReplacesMode(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "paperclip")
	if err := os.WriteFile(dst, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(dst, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "new" {
		t.Fatalf("body = %q", string(body))
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "paperclip" {
			t.Fatalf("unexpected leftover temp entry: %s", entry.Name())
		}
		if len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			t.Fatalf("hidden temp entry left behind: %s", entry.Name())
		}
	}
}

func TestConflictingOperationFlagsAreRejected(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "paperclip")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--plan", "--install", "--target", "tools", "--json", "--install-root", root}, &stdout, &stderr, exe)
	if code != 2 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".local")); !os.IsNotExist(err) {
		t.Fatalf("conflicting plan/install should not write, stat err=%v", err)
	}
}

func TestFalseOperationFlagDoesNotSelectOperation(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "paperclip")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--plan=false", "--target", "codex", "--json", "--install-root", root}, &stdout, &stderr, exe)
	if code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result Result
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if result.Operation != "install" {
		t.Fatalf("operation = %q", result.Operation)
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"--plan=false", "--install", "--target", "codex", "--json", "--install-root", root}, &stdout, &stderr, exe)
	if code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"--plan=False", "--install", "--target", "codex", "--json", "--install-root", root}, &stdout, &stderr, exe)
	if code != 0 {
		t.Fatalf("False variant code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"--plan=0", "--install", "--target", "codex", "--json", "--install-root", root}, &stdout, &stderr, exe)
	if code != 0 {
		t.Fatalf("0 variant code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"--plan=garbage", "--target", "codex", "--json"}, &stdout, &stderr, exe)
	if code != 2 {
		t.Fatalf("garbage code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}
