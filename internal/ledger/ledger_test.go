package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charlesnpx/paperclip/internal/domain"
)

func TestNewDefaultUsesPaperclipDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PAPERCLIP_HOME", "")
	t.Setenv("PAPERCUT_HOME", "")

	repo, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "paperclip", "PAPERCLIP.md")
	if repo.Path() != want {
		t.Fatalf("default path = %q, want %q", repo.Path(), want)
	}
}

func TestNewDefaultUsesPaperclipHomeBeforeDeprecatedPapercutHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PAPERCLIP_HOME", filepath.Join(home, "new"))
	t.Setenv("PAPERCUT_HOME", filepath.Join(home, "old"))

	repo, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "new", "PAPERCLIP.md")
	if repo.Path() != want {
		t.Fatalf("default path = %q, want %q", repo.Path(), want)
	}
}

func TestNewDefaultAcceptsDeprecatedPapercutHomeFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PAPERCLIP_HOME", "")
	t.Setenv("PAPERCUT_HOME", filepath.Join(home, "old"))

	repo, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "old", "PAPERCLIP.md")
	if repo.Path() != want {
		t.Fatalf("default path = %q, want %q", repo.Path(), want)
	}
}

func TestCommitWritesAtomicallyWithPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paperclip", "PAPERCLIP.md")
	repo := New(path)
	event := eventForTest(t, "evt_1", "obs_1")

	result, err := repo.Commit(time.Second, func(snapshot domain.Snapshot) ([]domain.Event, domain.CommitResult, error) {
		if len(snapshot.Events()) != 0 {
			t.Fatal("expected empty snapshot")
		}
		return []domain.Event{event}, domain.CommitResult{ObservationID: "obs_1", Created: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsAppended != 1 {
		t.Fatalf("events appended = %d", result.EventsAppended)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("directory is group/other accessible: %o", dirInfo.Mode().Perm())
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), Header) || strings.Count(string(body), "```json") != 1 {
		t.Fatalf("unexpected ledger body:\n%s", string(body))
	}
}

func TestExistingPublicLedgerParentIsRejectedWithoutChmod(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "paperclip")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "PAPERCLIP.md")
	_, err := New(path).Read(time.Second)
	if err == nil || !strings.Contains(err.Error(), "must not be group or other accessible") {
		t.Fatalf("expected permission rejection, got %v", err)
	}
	info, statErr := os.Stat(dir)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("existing parent mode was changed to %o", info.Mode().Perm())
	}
}

func TestExistingPrivateLedgerParentIsAccepted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "paperclip")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "PAPERCLIP.md")
	if _, err := New(path).Read(time.Second); err != nil {
		t.Fatalf("read should accept private parent: %v", err)
	}
}

func TestMalformedLedgerFailsClosedWithoutWriting(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "paperclip")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "PAPERCLIP.md")
	original := []byte("# wrong\n```json\n{\"secret\":\"token=ghp_abcdefghijklmnopqrstuvwxyz123456\"}\n```\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	repo := New(path)
	_, err := repo.Commit(time.Second, func(snapshot domain.Snapshot) ([]domain.Event, domain.CommitResult, error) {
		t.Fatal("planner should not run for malformed ledger")
		return nil, domain.CommitResult{}, nil
	})
	if err == nil || !IsMalformed(err) {
		t.Fatalf("expected malformed error, got %v", err)
	}
	if strings.Contains(err.Error(), "ghp_") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("malformed error leaked payload: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("malformed ledger was modified")
	}
}

func TestMalformedLedgerDecoderErrorDoesNotEchoPayload(t *testing.T) {
	body := Header + "```json\n{\"schema\":\"paperclip.event.v1\",\"event_id\":\"evt_1\",\"occurred_at\":\"2026-07-14T12:00:00Z\",\"type\":\"observation-opened\",\"subject_id\":\"obs_1\",\"context\":{\"repo_id\":\"none\"},\"payload\":{},\"authorization=Basic dXNlcjpwYXNz\":\"secret\"}\n```\n"
	_, err := Parse([]byte(body))
	if err == nil || !IsMalformed(err) {
		t.Fatalf("expected malformed error, got %v", err)
	}
	if strings.Contains(err.Error(), "authorization") || strings.Contains(err.Error(), "dXNlcjpwYXNz") {
		t.Fatalf("malformed error leaked payload: %v", err)
	}
}

func TestAppendBlocksInsertsNewlineAfterClosingFenceAtEOF(t *testing.T) {
	first := eventForTest(t, "evt_1", "obs_1")
	body, err := AppendBlocks(nil, []domain.Event{first})
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.TrimRight(string(body), "\n"))
	second := eventForTestWithExpected(t, "evt_2", "obs_2", "expected two")
	next, err := AppendBlocks(body, []domain.Event{second})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(next), "``````json") {
		t.Fatalf("fences were concatenated:\n%s", string(next))
	}
	if !strings.Contains(string(next), "```\n```json\n") {
		t.Fatalf("expected newline between fences:\n%s", string(next))
	}
	if _, err := Parse(next); err != nil {
		t.Fatalf("parse appended ledger: %v", err)
	}
}

func TestCrashBeforeRenameLeavesOriginalLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paperclip", "PAPERCLIP.md")
	repo := NewWithHook(path, func(string) error {
		return errors.New("simulated crash before rename")
	})
	_, err := repo.Commit(time.Second, func(snapshot domain.Snapshot) ([]domain.Event, domain.CommitResult, error) {
		return []domain.Event{eventForTest(t, "evt_1", "obs_1")}, domain.CommitResult{ObservationID: "obs_1"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "simulated crash") {
		t.Fatalf("expected simulated crash, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ledger should not exist after failed first commit, stat err=%v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temp file was not cleaned up: %s", entry.Name())
		}
	}
}

func TestLockTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paperclip", "PAPERCLIP.md")
	if err := ensureDir(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireLock(path+".lock", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()
	repo := New(path)
	_, err = repo.Read(20 * time.Millisecond)
	if err == nil || !IsLockTimeout(err) {
		t.Fatalf("expected lock timeout, got %v", err)
	}
}

func TestRejectsSymlinkLedgerPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "paperclip")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte(Header), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "PAPERCLIP.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := New(link).Read(time.Second)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

func TestPreservesStricterExistingFilePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "paperclip")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "PAPERCLIP.md")
	if err := os.WriteFile(path, []byte(Header), 0o400); err != nil {
		t.Fatal(err)
	}
	repo := New(path)
	_, err := repo.Commit(time.Second, func(snapshot domain.Snapshot) ([]domain.Event, domain.CommitResult, error) {
		return []domain.Event{eventForTest(t, "evt_1", "obs_1")}, domain.CommitResult{ObservationID: "obs_1"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o400 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func eventForTest(t *testing.T, eventID string, obsID string) domain.Event {
	t.Helper()
	return eventForTestWithExpected(t, eventID, obsID, "expected")
}

func eventForTestWithExpected(t *testing.T, eventID string, obsID string, expected string) domain.Event {
	t.Helper()
	normalized := domain.NormalizedObservation{
		Expected: expected,
		Observed: "observed",
		Impact:   "impact",
		Locus:    domain.LocusRepo,
		Severity: domain.SeverityMedium,
		RepoID:   "none",
	}
	event, err := domain.NewEvent(eventID, time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC), domain.EventObservationOpened, obsID, "", domain.Context{RepoID: "none"}, domain.ObservationOpenedPayload{
		Expected:    expected,
		Observed:    "observed",
		Impact:      "impact",
		Locus:       domain.LocusRepo,
		Severity:    domain.SeverityMedium,
		Fingerprint: domain.Fingerprint(normalized),
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}
