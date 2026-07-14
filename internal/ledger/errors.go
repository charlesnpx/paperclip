package ledger

import (
	"errors"
	"fmt"
)

type MalformedError struct {
	Block   int
	Offset  int
	Message string
}

func (e MalformedError) Error() string {
	if e.Block > 0 {
		return fmt.Sprintf("malformed ledger at block %d byte %d: %s", e.Block, e.Offset, e.Message)
	}
	return fmt.Sprintf("malformed ledger at byte %d: %s", e.Offset, e.Message)
}

type LockTimeoutError struct {
	Path string
}

func (e LockTimeoutError) Error() string {
	return "lock timeout: " + e.Path
}

func IsMalformed(err error) bool {
	var malformed MalformedError
	return errors.As(err, &malformed)
}

func IsLockTimeout(err error) bool {
	var lockTimeout LockTimeoutError
	return errors.As(err, &lockTimeout)
}
