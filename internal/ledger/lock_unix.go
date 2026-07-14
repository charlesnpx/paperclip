//go:build darwin || linux

package ledger

import (
	"errors"
	"os"
	"syscall"
	"time"
)

type fileLock struct {
	file *os.File
	path string
}

func acquireLock(path string, timeout time.Duration) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return &fileLock{file: file, path: path}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, err
		}
		if timeout <= 0 || time.Now().After(deadline) {
			_ = file.Close()
			return nil, LockTimeoutError{Path: path}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *fileLock) release() error {
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
