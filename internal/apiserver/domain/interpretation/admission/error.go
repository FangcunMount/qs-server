package admission

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when admission evidence does not exist.
var ErrNotFound = errors.New("interpretation admission failure not found")

// RejectedError is returned by automation when admission fails closed.
// It is distinct from Run failure and must not create Generation.
type RejectedError struct {
	Failure *Failure
}

func (e *RejectedError) Error() string {
	if e == nil || e.Failure == nil {
		return "interpretation admission rejected"
	}
	return fmt.Sprintf("interpretation admission rejected: kind=%s code=%s", e.Failure.Kind(), e.Failure.Code())
}

func RejectedFrom(err error) (*RejectedError, bool) {
	var rejected *RejectedError
	if errors.As(err, &rejected) {
		return rejected, true
	}
	return nil, false
}
