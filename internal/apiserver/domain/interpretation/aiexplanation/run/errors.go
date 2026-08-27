package run

import "errors"

var (
	ErrNotFound           = errors.New("AI explanation run not found")
	ErrAlreadyExists      = errors.New("AI explanation run already exists")
	ErrConflict           = errors.New("AI explanation run state conflict")
	ErrUnsafeLeaseReclaim = errors.New("AI explanation provider invocation cannot be safely reclaimed")
	ErrRecoveryNotAllowed = errors.New("AI explanation run is not eligible for lease recovery wake-up")
	ErrRetryNotAllowed    = errors.New("AI explanation run is not eligible for a governed retry")
)
