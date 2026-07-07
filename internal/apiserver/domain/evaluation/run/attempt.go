package run

// Status tracks execution-phase progress for one evaluation attempt.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

func (s Status) String() string { return string(s) }

// Attempt records one evaluation execution try.
type Attempt struct {
	Number int
	Status Status
}

// NewAttempt creates the first execution attempt.
func NewAttempt() Attempt {
	return Attempt{Number: 1, Status: StatusPending}
}
