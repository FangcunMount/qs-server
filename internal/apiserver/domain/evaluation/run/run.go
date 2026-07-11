package run

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

var (
	// ErrInvalidTransition indicates a terminal or otherwise invalid EvaluationRun transition.
	ErrInvalidTransition = errors.New("invalid evaluation run transition")
	// ErrInputSnapshotConflict indicates that a run is being associated with two input snapshots.
	ErrInputSnapshotConflict = errors.New("evaluation run input snapshot conflict")
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

// AttachInputSnapshot records the stable audit reference for a running attempt.
func (r *EvaluationRun) AttachInputSnapshot(ref string) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.Attempt.Status != StatusRunning {
		return fmt.Errorf("%w: attach input snapshot from %s", ErrInvalidTransition, r.Attempt.Status)
	}
	if ref == "" || r.InputSnapshotRef == ref {
		return nil
	}
	if r.InputSnapshotRef != "" {
		return fmt.Errorf("%w: existing=%q incoming=%q", ErrInputSnapshotConflict, r.InputSnapshotRef, ref)
	}
	r.InputSnapshotRef = ref
	return nil
}

// Start 标记run 作为 活跃ly executing。
func (r *EvaluationRun) Start(now time.Time) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.Attempt.Status != StatusPending {
		return fmt.Errorf("%w: start from %s", ErrInvalidTransition, r.Attempt.Status)
	}
	r.Attempt.Status = StatusRunning
	r.StartedAt = now
	return nil
}

// Succeed 标记run 作为 completed 成功ly。
func (r *EvaluationRun) Succeed(now time.Time) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.Attempt.Status != StatusRunning {
		return fmt.Errorf("%w: succeed from %s", ErrInvalidTransition, r.Attempt.Status)
	}
	r.Attempt.Status = StatusSucceeded
	r.Failure = nil
	r.FinishedAt = &now
	return nil
}

// Fail 标记run 作为 失败 使用 重试元数据。
func (r *EvaluationRun) Fail(now time.Time, failure Failure) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.Attempt.Status != StatusRunning {
		return fmt.Errorf("%w: fail from %s", ErrInvalidTransition, r.Attempt.Status)
	}
	r.Attempt.Status = StatusFailed
	r.Failure = &failure
	r.FinishedAt = &now
	return nil
}

// Retryable 报告是否 最新 失败 can be retried。
func (r EvaluationRun) Retryable() bool {
	return r.Failure != nil && r.Failure.Retryable
}

func (r EvaluationRun) String() string {
	return fmt.Sprintf("run=%s assessment=%d attempt=%d status=%s", r.RunID, r.AssessmentID, r.Attempt.Number, r.Attempt.Status)
}
