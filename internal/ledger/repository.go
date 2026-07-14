package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charlesnpx/paperclip/internal/domain"
)

type Repository struct {
	path         string
	beforeRename func(tmpPath string) error
}

func NewDefault() (*Repository, error) {
	home := os.Getenv("PAPERCUT_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		home = filepath.Join(userHome, "Papercuts")
	} else if !filepath.IsAbs(home) {
		return nil, errors.New("PAPERCUT_HOME must be absolute")
	}
	return New(filepath.Join(home, "PAPERCUTS.md")), nil
}

func New(path string) *Repository {
	return &Repository{path: path}
}

func NewWithHook(path string, beforeRename func(string) error) *Repository {
	return &Repository{path: path, beforeRename: beforeRename}
}

func (r *Repository) Path() string {
	return r.path
}

func (r *Repository) Read(timeout time.Duration) (domain.Snapshot, error) {
	var snapshot domain.Snapshot
	err := r.withLock(timeout, func() error {
		body, _, err := r.readExisting()
		if err != nil {
			return err
		}
		parsed, err := Parse(body)
		if err != nil {
			return err
		}
		snapshot = parsed
		return nil
	})
	return snapshot, err
}

func (r *Repository) Commit(timeout time.Duration, plan func(domain.Snapshot) ([]domain.Event, domain.CommitResult, error)) (domain.CommitResult, error) {
	var result domain.CommitResult
	err := r.withLock(timeout, func() error {
		existing, existingMode, err := r.readExisting()
		if err != nil {
			return err
		}
		snapshot, err := Parse(existing)
		if err != nil {
			return err
		}
		events, plannedResult, err := plan(snapshot)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			result = plannedResult
			return nil
		}
		allEvents := append(snapshot.Events(), cloneEvents(events)...)
		if _, err := domain.ApplyEvents(allEvents); err != nil {
			return MalformedError{Offset: 0, Message: "planned events failed validation: " + err.Error()}
		}
		next, err := AppendBlocks(existing, events)
		if err != nil {
			return err
		}
		if err := r.atomicWrite(next, existingMode); err != nil {
			return err
		}
		plannedResult.EventsAppended = len(events)
		for _, event := range events {
			plannedResult.EventIDs = append(plannedResult.EventIDs, event.EventID)
		}
		result = plannedResult
		return nil
	})
	return result, err
}

func (r *Repository) withLock(timeout time.Duration, fn func() error) error {
	if err := ensureDir(filepath.Dir(r.path)); err != nil {
		return err
	}
	lock, err := acquireLock(r.path+".lock", timeout)
	if err != nil {
		return err
	}
	defer func() { _ = lock.release() }()
	return fn()
}

func (r *Repository) readExisting() ([]byte, os.FileMode, error) {
	info, err := os.Lstat(r.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0o600, nil
		}
		return nil, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, 0, fmt.Errorf("ledger path is a symlink: %s", r.path)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("ledger path is not a regular file: %s", r.path)
	}
	body, err := os.ReadFile(r.path)
	if err != nil {
		return nil, 0, err
	}
	mode := info.Mode().Perm() & 0o600
	if mode == 0 {
		mode = 0o600
	}
	return append([]byte(nil), body...), mode, nil
}

func (r *Repository) atomicWrite(body []byte, mode os.FileMode) error {
	if mode == 0 {
		mode = 0o600
	}
	dir := filepath.Dir(r.path)
	tmp := filepath.Join(dir, "papercut-"+randomHex(8)+".tmp")
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if r.beforeRename != nil {
		if err := r.beforeRename(tmp); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return err
	}
	cleanup = false
	return syncDir(dir)
}

func ensureDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.MkdirAll(dir, 0o700)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("ledger parent is not a directory: %s", dir)
	}
	mode := info.Mode().Perm()
	if mode&0o077 != 0 {
		return fmt.Errorf("ledger parent directory must not be group or other accessible: %s", dir)
	}
	return nil
}

func syncDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func cloneEvents(events []domain.Event) []domain.Event {
	out := make([]domain.Event, len(events))
	for i, event := range events {
		out[i] = event.Clone()
	}
	return out
}

func randomHex(bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
