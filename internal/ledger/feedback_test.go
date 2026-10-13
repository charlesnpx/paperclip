package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFeedbackHomeIsIndependentAndDoesNotCreateFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	t.Setenv("PAPERCLIP_HOME", "invalid-relative-path")
	t.Setenv("PAPERCUT_HOME", "also-relative")
	t.Setenv("PAPERCLIP_FEEDBACK_HOME", root)
	repository, err := NewFeedback()
	if err != nil || repository.Path() != filepath.Join(root, "PAPERCLIP.md") {
		t.Fatalf("feedback home: %+v %v", repository, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("constructing repository created files: %v", err)
	}
	t.Setenv("PAPERCLIP_FEEDBACK_HOME", "relative")
	if _, err := NewFeedback(); err == nil {
		t.Fatal("relative feedback home should fail")
	}
	t.Setenv("PAPERCLIP_FEEDBACK_HOME", "")
	repository, err = NewFeedback()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil || repository.Path() != filepath.Join(home, "paperclip", "skill-feedback", "PAPERCLIP.md") {
		t.Fatalf("unexpected default: %s %v", repository.Path(), err)
	}
}
