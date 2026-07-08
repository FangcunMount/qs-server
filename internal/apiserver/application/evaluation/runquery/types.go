package runquery

import (
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
)

// RunResult 是application 读模型 用于 一个评估执行 尝试。
type RunResult struct {
	RunID            string
	AssessmentID     uint64
	AttemptNo        int
	Status           string
	Retryable        bool
	ErrorCode        string
	ErrorMessage     string
	StartedAt        time.Time
	FinishedAt       *time.Time
	TraceID          string
	InputSnapshotRef string
}

// RunListResult 是有界 list of runs 用于 一个assessment。
type RunListResult struct {
	Items []*RunResult
}

// RetryableFailedRunResult includes org scope 用于 operating 查询。
type RetryableFailedRunResult struct {
	RunResult
	OrgID int64
}

// RetryableFailedListResult 是cursor page of 可重试 失败 runs。
type RetryableFailedListResult struct {
	Items      []*RetryableFailedRunResult
	NextCursor uint64
}

func runResultFromDomain(run evalrun.EvaluationRun) *RunResult {
	result := &RunResult{
		RunID:            run.RunID.String(),
		AssessmentID:     run.AssessmentID,
		AttemptNo:        run.Attempt.Number,
		Status:           run.Attempt.Status.String(),
		Retryable:        run.Retryable(),
		StartedAt:        run.StartedAt,
		FinishedAt:       run.FinishedAt,
		TraceID:          run.TraceID,
		InputSnapshotRef: run.InputSnapshotRef,
	}
	if run.Failure != nil {
		result.ErrorCode = run.Failure.Kind.String()
		result.ErrorMessage = run.Failure.Message
		result.Retryable = run.Failure.Retryable
	}
	return result
}
