package run

import (
	"fmt"
	"strconv"
	"time"
)

// ID identifies one evaluation run within an assessment lifecycle.
type ID string

func (id ID) String() string { return string(id) }

// EvaluationRun records one evaluation execution attempt for an assessment.
type EvaluationRun struct {
	RunID        ID
	AssessmentID uint64
	Attempt      Attempt
	Failure      *Failure
	TraceID      string
	StartedAt    time.Time
	FinishedAt   *time.Time
}

// NewEvaluationRun creates the first in-memory run for an assessment execution.
func NewEvaluationRun(assessmentID uint64) EvaluationRun {
	return EvaluationRun{
		RunID:        ID(strconv.FormatUint(assessmentID, 10) + ":1"),
		AssessmentID: assessmentID,
		Attempt:      NewAttempt(),
	}
}

// Start marks the run as actively executing.
func (r *EvaluationRun) Start(now time.Time) {
	if r == nil {
		return
	}
	r.Attempt.Status = StatusRunning
	r.StartedAt = now
}

// Succeed marks the run as completed successfully.
func (r *EvaluationRun) Succeed(now time.Time) {
	if r == nil {
		return
	}
	r.Attempt.Status = StatusSucceeded
	r.FinishedAt = &now
}

// Fail marks the run as failed with retry metadata.
func (r *EvaluationRun) Fail(now time.Time, failure Failure) {
	if r == nil {
		return
	}
	r.Attempt.Status = StatusFailed
	r.Failure = &failure
	r.FinishedAt = &now
}

// Retryable reports whether the latest failure can be retried.
func (r EvaluationRun) Retryable() bool {
	return r.Failure != nil && r.Failure.Retryable
}

func (r EvaluationRun) String() string {
	return fmt.Sprintf("run=%s assessment=%d attempt=%d status=%s", r.RunID, r.AssessmentID, r.Attempt.Number, r.Attempt.Status)
}
