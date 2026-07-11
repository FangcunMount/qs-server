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
	// ErrInvalidClaim indicates malformed claim ownership or lease timing.
	ErrInvalidClaim = errors.New("invalid evaluation run claim")
)

// ID identifies one Evaluation execution attempt in an Assessment lifecycle.
type ID string

func (id ID) String() string { return string(id) }

// EvaluationRun records one durable, claimable scoring attempt.
type EvaluationRun struct {
	RunID            ID
	AssessmentID     uint64
	Attempt          Attempt
	Failure          *Failure
	TraceID          string
	InputSnapshotRef string
	ClaimToken       string
	LeaseExpiresAt   *time.Time
	StartedAt        time.Time
	FinishedAt       *time.Time
}

// Claim assigns exclusive execution ownership until leaseExpiresAt. A pending
// attempt becomes running; an expired running attempt can be reclaimed without
// creating a new attempt.
func (r *EvaluationRun) Claim(token string, now, leaseExpiresAt time.Time) error {
	if r == nil || token == "" || now.IsZero() || !leaseExpiresAt.After(now) {
		return ErrInvalidClaim
	}
	if r.Attempt.Status == StatusPending {
		if err := r.Start(now); err != nil {
			return err
		}
	} else if r.Attempt.Status != StatusRunning {
		return fmt.Errorf("%w: claim from %s", ErrInvalidTransition, r.Attempt.Status)
	}
	r.ClaimToken = token
	lease := leaseExpiresAt
	r.LeaseExpiresAt = &lease
	return nil
}

// HasActiveLease reports whether another worker still owns this attempt.
func (r EvaluationRun) HasActiveLease(now time.Time) bool {
	return r.Attempt.Status == StatusRunning && r.ClaimToken != "" && r.LeaseExpiresAt != nil && r.LeaseExpiresAt.After(now)
}

// NewEvaluationRun creates the first in-memory attempt for an Assessment.
func NewEvaluationRun(assessmentID uint64) EvaluationRun {
	return NewEvaluationRunWithAttempt(assessmentID, 1)
}

// NewEvaluationRunWithAttempt creates a pending run for a specific attempt number.
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

// NextEvaluationRun creates the next attempt after a retryable failure.
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

// Start transitions a pending run to running.
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

// Succeed records a terminal successful attempt and releases its lease.
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
	r.LeaseExpiresAt = nil
	return nil
}

// Fail records a terminal failed attempt and releases its lease.
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
	r.LeaseExpiresAt = nil
	return nil
}

// Retryable reports whether a failed attempt may be retried.
func (r EvaluationRun) Retryable() bool {
	return r.Failure != nil && r.Failure.Retryable
}

func (r EvaluationRun) String() string {
	return fmt.Sprintf("run=%s assessment=%d attempt=%d status=%s", r.RunID, r.AssessmentID, r.Attempt.Number, r.Attempt.Status)
}
