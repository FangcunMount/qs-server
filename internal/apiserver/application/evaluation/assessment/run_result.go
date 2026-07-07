package assessment

import "time"

// AssessmentRunResult is the protected query view of one evaluation run.
type AssessmentRunResult struct {
	RunID        string
	AssessmentID uint64
	AttemptNo    int
	Status       string
	Retryable    bool
	ErrorCode    string
	ErrorMessage string
	StartedAt    time.Time
	FinishedAt   *time.Time
	TraceID      string
}

// AssessmentRunListResult lists evaluation runs for one assessment.
type AssessmentRunListResult struct {
	Items []*AssessmentRunResult
}
