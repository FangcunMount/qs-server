package run

import (
	"fmt"
	"strconv"
	"time"
)

// ID 标识一个评估执行 在 测评生命周期。
type ID string

func (id ID) String() string { return string(id) }

// EvaluationRun 记录一个评估执行 尝试 用于 assessment。
type EvaluationRun struct {
	RunID            ID
	AssessmentID     uint64
	Attempt          Attempt
	Failure          *Failure
	TraceID          string
	InputSnapshotRef string
	StartedAt        time.Time
	FinishedAt       *time.Time
}

// NewEvaluationRun 创建首个 in-memory run 用于 测评执行。
func NewEvaluationRun(assessmentID uint64) EvaluationRun {
	return NewEvaluationRunWithAttempt(assessmentID, 1)
}

// NewEvaluationRunWithAttempt 创建run 用于 特定 尝试序号。
func NewEvaluationRunWithAttempt(assessmentID uint64, attemptNo int) EvaluationRun {
	if attemptNo < 1 {
		attemptNo = 1
	}
	return EvaluationRun{
		RunID:        ID(strconv.FormatUint(assessmentID, 10) + ":" + strconv.Itoa(attemptNo)),
		AssessmentID: assessmentID,
		Attempt:      Attempt{Number: attemptNo, Status: StatusPending},
	}
}

// NextEvaluationRun 创建下一个 尝试 在之后 失败 可重试 run。
func NextEvaluationRun(latest EvaluationRun) EvaluationRun {
	return NewEvaluationRunWithAttempt(latest.AssessmentID, latest.Attempt.Number+1)
}

// AttachInputSnapshot records a stable audit reference to the resolved input snapshot.
func (r *EvaluationRun) AttachInputSnapshot(ref string) {
	if r == nil {
		return
	}
	r.InputSnapshotRef = ref
}

// Start 标记run 作为 活跃ly executing。
func (r *EvaluationRun) Start(now time.Time) {
	if r == nil {
		return
	}
	r.Attempt.Status = StatusRunning
	r.StartedAt = now
}

// Succeed 标记run 作为 completed 成功ly。
func (r *EvaluationRun) Succeed(now time.Time) {
	if r == nil {
		return
	}
	r.Attempt.Status = StatusSucceeded
	r.FinishedAt = &now
}

// Fail 标记run 作为 失败 使用 重试元数据。
func (r *EvaluationRun) Fail(now time.Time, failure Failure) {
	if r == nil {
		return
	}
	r.Attempt.Status = StatusFailed
	r.Failure = &failure
	r.FinishedAt = &now
}

// Retryable 报告是否 最新 失败 can be retried。
func (r EvaluationRun) Retryable() bool {
	return r.Failure != nil && r.Failure.Retryable
}

func (r EvaluationRun) String() string {
	return fmt.Sprintf("run=%s assessment=%d attempt=%d status=%s", r.RunID, r.AssessmentID, r.Attempt.Number, r.Attempt.Status)
}
