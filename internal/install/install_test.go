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
	exe := filepath.Join(root, "papercut")
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
	claudeFiles := result.Targets["claude"].Files
	if len(claudeFiles) != 2 {
		t.Fatalf("claude files = %d", len(claudeFiles))
	}
	if claudeFiles[0].Path != filepath.Join(root, ".claude", "skills", "paperclip", "SKILL.md") {
		t.Fatalf("claude skill path = %q", claudeFiles[0].Path)
	}
	if claudeFiles[1].Path != filepath.Join(root, ".claude", "rules", "paperclip.md") {
		t.Fatalf("claude rule path = %q", claudeFiles[1].Path)
	}
	codexFiles := result.Targets["codex"].Files
	if len(codexFiles) != 1 {
		t.Fatalf("codex files = %d", len(codexFiles))
	}
	if codexFiles[0].Path != filepath.Join(root, ".codex", "skills", "paperclip", "SKILL.md") {
		t.Fatalf("codex skill path = %q", codexFiles[0].Path)
	}
	toolFiles := result.Targets["tools"].Files
	if len(toolFiles) != 1 {
		t.Fatalf("tools files = %d", len(toolFiles))
	}
	if toolFiles[0].Path != filepath.Join(root, ".local", "bin", "papercut") {
		t.Fatalf("tool path = %q", toolFiles[0].Path)
	}
	if _, err := os.Stat(filepath.Join(root, ".local")); !os.IsNotExist(err) {
		t.Fatalf("plan should not create staging path, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex")); !os.IsNotExist(err) {
		t.Fatalf("plan should not create codex path, stat err=%v", err)
	}
	if len(result.Notices) != 1 || !strings.Contains(result.Notices[0], "add this text") {
		t.Fatalf("expected codex notice, got %#v", result.Notices)
	}
}

func TestCodexInstallTargetWritesExplicitUseSkill(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "papercut")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Execute(Options{Operation: "install", Target: "codex", InstallRoot: root, Executable: exe})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Targets) != 1 || len(result.Targets["codex"].Files) != 1 {
		t.Fatalf("unexpected targets: %+v", result.Targets)
	}
	path := filepath.Join(root, ".codex", "skills", "paperclip", "SKILL.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "allow_implicit_invocation: false") {
		t.Fatalf("codex skill is missing explicit invocation policy:\n%s", string(body))
	}
	if result.Targets["codex"].Files[0].SHA256 == "" {
		t.Fatalf("install result should include sha: %+v", result.Targets["codex"].Files[0])
	}
}

func TestCodexUninstallDoesNotEmitInstallNotice(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "papercut")
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

func TestClaudeInstallTargetWritesSkillAndRule(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "papercut")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := Execute(Options{Operation: "install", Target: "claude", InstallRoot: root, Executable: exe})
	if err != nil {
		t.Fatal(err)
	}
	files := result.Targets["claude"].Files
	if len(files) != 2 {
		t.Fatalf("claude files = %d", len(files))
	}
	for _, path := range []string{
		filepath.Join(root, ".claude", "skills", "paperclip", "SKILL.md"),
		filepath.Join(root, ".claude", "rules", "paperclip.md"),
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "Paperclip") && !strings.Contains(string(body), "paperclip") {
			t.Fatalf("unexpected claude payload at %s:\n%s", path, string(body))
		}
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
	if result.Operation != "plan" || len(result.Targets["codex"].Files) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestAtomicFileWriteUsesVisibleTempAndReplacesMode(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "papercut")
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
		if entry.Name() != "papercut" {
			t.Fatalf("unexpected leftover temp entry: %s", entry.Name())
		}
		if len(entry.Name()) > 0 && entry.Name()[0] == '.' {
			t.Fatalf("hidden temp entry left behind: %s", entry.Name())
		}
	}
}

func TestConflictingOperationFlagsAreRejected(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "papercut")
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
	exe := filepath.Join(root, "papercut")
	if err := os.WriteFile(exe, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"--plan=false", "--target", "codex", "--json"}, &stdout, &stderr, exe)
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
	code = Run([]string{"--plan=false", "--install", "--target", "codex", "--json"}, &stdout, &stderr, exe)
	if code != 0 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"--plan=False", "--install", "--target", "codex", "--json"}, &stdout, &stderr, exe)
	if code != 0 {
		t.Fatalf("False variant code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	code = Run([]string{"--plan=0", "--install", "--target", "codex", "--json"}, &stdout, &stderr, exe)
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
