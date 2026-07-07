package runquery

import (
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

// RunResult is the application read model for one evaluation run attempt.
type RunResult struct {
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

// RunListResult is a bounded list of runs for one assessment.
type RunListResult struct {
	Items []*RunResult
}

// RetryableFailedRunResult includes org scope for operating queries.
type RetryableFailedRunResult struct {
	RunResult
	OrgID int64
}

// RetryableFailedListResult is a cursor page of retryable failed runs.
type RetryableFailedListResult struct {
	Items      []*RetryableFailedRunResult
	NextCursor uint64
}

func runResultFromDomain(run evalrun.EvaluationRun) *RunResult {
	result := &RunResult{
		RunID:        run.RunID.String(),
		AssessmentID: run.AssessmentID,
		AttemptNo:    run.Attempt.Number,
		Status:       run.Attempt.Status.String(),
		Retryable:    run.Retryable(),
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
		TraceID:      run.TraceID,
	}
	if run.Failure != nil {
		result.ErrorCode = run.Failure.Kind.String()
		result.ErrorMessage = run.Failure.Message
		result.Retryable = run.Failure.Retryable
	}
	return result
}
