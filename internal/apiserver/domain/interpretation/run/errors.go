package run

import "errors"

var (
	ErrNotFound             = errors.New("interpretation run not found")
	ErrAlreadyExists        = errors.New("interpretation run already exists")
	ErrInvalidRetrySchedule = errors.New("invalid interpretation retry schedule")
)
