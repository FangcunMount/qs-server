package assessment

import "time"

// AssessmentRunResult 是protected 查询 视图 of 一个评估执行。
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

// AssessmentRunListResult 列出评估执行 用于 一个assessment。
type AssessmentRunListResult struct {
	Items []*AssessmentRunResult
}
