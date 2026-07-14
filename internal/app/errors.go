package app

import "errors"

var (
	ErrUsage    = errors.New("usage error")
	ErrConflict = errors.New("conflict")
	ErrPolicy   = errors.New("policy denied")
)
