package run

// Status tracks execution-phase progress for one Evaluation attempt.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

func (s Status) String() string { return string(s) }

// Attempt records one Evaluation execution attempt.
type Attempt struct {
	Number int
	Status Status
}
