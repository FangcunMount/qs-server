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
	runID            ID
	assessmentID     uint64
	attempt          Attempt
	failure          *Failure
	traceID          string
	inputSnapshotRef string
	claimToken       string
	leaseExpiresAt   *time.Time
	startedAt        time.Time
	finishedAt       *time.Time
}

type ClaimInput struct {
	Token          string
	TraceID        string
	ClaimedAt      time.Time
	LeaseExpiresAt time.Time
}

type ReconstructInput struct {
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

func Reconstruct(input ReconstructInput) EvaluationRun {
	return EvaluationRun{
		runID: input.RunID, assessmentID: input.AssessmentID, attempt: input.Attempt,
		failure: cloneFailure(input.Failure), traceID: input.TraceID,
		inputSnapshotRef: input.InputSnapshotRef, claimToken: input.ClaimToken,
		leaseExpiresAt: cloneTime(input.LeaseExpiresAt), startedAt: input.StartedAt,
		finishedAt: cloneTime(input.FinishedAt),
	}
}

func (r EvaluationRun) ID() ID                     { return r.runID }
func (r EvaluationRun) AssessmentID() uint64       { return r.assessmentID }
func (r EvaluationRun) Attempt() Attempt           { return r.attempt }
func (r EvaluationRun) Failure() *Failure          { return cloneFailure(r.failure) }
func (r EvaluationRun) TraceID() string            { return r.traceID }
func (r EvaluationRun) InputSnapshotRef() string   { return r.inputSnapshotRef }
func (r EvaluationRun) ClaimToken() string         { return r.claimToken }
func (r EvaluationRun) LeaseExpiresAt() *time.Time { return cloneTime(r.leaseExpiresAt) }
func (r EvaluationRun) StartedAt() time.Time       { return r.startedAt }
func (r EvaluationRun) FinishedAt() *time.Time     { return cloneTime(r.finishedAt) }

// Claim assigns exclusive execution ownership until leaseExpiresAt. A pending
// attempt becomes running; an expired running attempt can be reclaimed without
// creating a new attempt.
func (r *EvaluationRun) Claim(input ClaimInput) error {
	if r == nil || input.Token == "" || input.ClaimedAt.IsZero() || !input.LeaseExpiresAt.After(input.ClaimedAt) {
		return ErrInvalidClaim
	}
	if r.attempt.Status == StatusPending {
		if err := r.Start(input.ClaimedAt); err != nil {
			return err
		}
	} else if r.attempt.Status != StatusRunning {
		return fmt.Errorf("%w: claim from %s", ErrInvalidTransition, r.attempt.Status)
	}
	r.claimToken = input.Token
	r.traceID = input.TraceID
	lease := input.LeaseExpiresAt
	r.leaseExpiresAt = &lease
	return nil
}

// HasActiveLease reports whether another worker still owns this attempt.
func (r EvaluationRun) HasActiveLease(now time.Time) bool {
	return r.attempt.Status == StatusRunning && r.claimToken != "" && r.leaseExpiresAt != nil && r.leaseExpiresAt.After(now)
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
		runID:        ID(strconv.FormatUint(assessmentID, 10) + ":" + strconv.Itoa(attemptNo)),
		assessmentID: assessmentID,
		attempt:      Attempt{Number: attemptNo, Status: StatusPending},
	}
}

// NextEvaluationRun creates the next attempt after a retryable failure.
func NextEvaluationRun(latest EvaluationRun) EvaluationRun {
	return NewEvaluationRunWithAttempt(latest.assessmentID, latest.attempt.Number+1)
}

// AttachInputSnapshot records the stable audit reference for a running attempt.
func (r *EvaluationRun) AttachInputSnapshot(ref string) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.attempt.Status != StatusRunning {
		return fmt.Errorf("%w: attach input snapshot from %s", ErrInvalidTransition, r.attempt.Status)
	}
	if ref == "" || r.inputSnapshotRef == ref {
		return nil
	}
	if r.inputSnapshotRef != "" {
		return fmt.Errorf("%w: existing=%q incoming=%q", ErrInputSnapshotConflict, r.inputSnapshotRef, ref)
	}
	r.inputSnapshotRef = ref
	return nil
}

// Start transitions a pending run to running.
func (r *EvaluationRun) Start(now time.Time) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.attempt.Status != StatusPending {
		return fmt.Errorf("%w: start from %s", ErrInvalidTransition, r.attempt.Status)
	}
	r.attempt.Status = StatusRunning
	r.startedAt = now
	return nil
}

// Succeed records a terminal successful attempt and releases its lease.
func (r *EvaluationRun) Succeed(now time.Time) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.attempt.Status != StatusRunning {
		return fmt.Errorf("%w: succeed from %s", ErrInvalidTransition, r.attempt.Status)
	}
	r.attempt.Status = StatusSucceeded
	r.failure = nil
	r.finishedAt = &now
	r.leaseExpiresAt = nil
	return nil
}

// Fail records a terminal failed attempt and releases its lease.
func (r *EvaluationRun) Fail(now time.Time, failure Failure) error {
	if r == nil {
		return fmt.Errorf("%w: nil run", ErrInvalidTransition)
	}
	if r.attempt.Status != StatusRunning {
		return fmt.Errorf("%w: fail from %s", ErrInvalidTransition, r.attempt.Status)
	}
	r.attempt.Status = StatusFailed
	r.failure = cloneFailure(&failure)
	r.finishedAt = &now
	r.leaseExpiresAt = nil
	return nil
}

// Retryable reports whether a failed attempt may be retried.
func (r EvaluationRun) Retryable() bool {
	return r.failure != nil && r.failure.Retryable
}

func (r EvaluationRun) String() string {
	return fmt.Sprintf("run=%s assessment=%d attempt=%d status=%s", r.runID, r.assessmentID, r.attempt.Number, r.attempt.Status)
}

func cloneFailure(value *Failure) *Failure {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
