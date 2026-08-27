// Package run owns one provider execution attempt under an AI explanation
// generation. Reliability attempts are not agent turns or conversational memory.
package run

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusRunning, StatusSucceeded, StatusFailed:
		return true
	default:
		return false
	}
}

// InvocationPhase makes the external side-effect boundary explicit. A lease
// can be reclaimed without provider idempotency only before dispatch begins.
type InvocationPhase string

const (
	InvocationPhaseNone             InvocationPhase = "none"
	InvocationPhasePrepared         InvocationPhase = "prepared"
	InvocationPhaseDispatching      InvocationPhase = "dispatching"
	InvocationPhaseResponseReceived InvocationPhase = "response_received"
	InvocationPhaseResultUnknown    InvocationPhase = "result_unknown"
)

func (p InvocationPhase) IsValid() bool {
	switch p {
	case InvocationPhaseNone, InvocationPhasePrepared, InvocationPhaseDispatching, InvocationPhaseResponseReceived, InvocationPhaseResultUnknown:
		return true
	default:
		return false
	}
}

type FailureKind string

const (
	FailureKindInput             FailureKind = "input"
	FailureKindProfile           FailureKind = "profile"
	FailureKindPrompt            FailureKind = "prompt"
	FailureKindProviderTransport FailureKind = "provider_transport"
	FailureKindProviderRateLimit FailureKind = "provider_rate_limit"
	FailureKindProviderTimeout   FailureKind = "provider_timeout"
	FailureKindProviderRefusal   FailureKind = "provider_refusal"
	FailureKindOutputValidation  FailureKind = "output_validation"
	FailureKindSafety            FailureKind = "safety"
)

func (k FailureKind) IsValid() bool {
	switch k {
	case FailureKindInput, FailureKindProfile, FailureKindPrompt,
		FailureKindProviderTransport, FailureKindProviderRateLimit,
		FailureKindProviderTimeout, FailureKindProviderRefusal,
		FailureKindOutputValidation, FailureKindSafety:
		return true
	default:
		return false
	}
}

type Failure struct {
	Kind        FailureKind
	Code        string
	SafeMessage string
	Retryable   bool
}

func (f Failure) Validate() error {
	if !f.Kind.IsValid() || strings.TrimSpace(f.Code) == "" || strings.TrimSpace(f.SafeMessage) == "" {
		return fmt.Errorf("AI explanation failure kind, code and safe message are required")
	}
	return nil
}

type ClaimRecord struct {
	ReclaimedAt time.Time
	TraceID     string
}

// RecoveryWakeup is the durable proof that a scheduler observed one exact
// expired execution lease. It authorizes only re-delivery of the same Run; it
// never authorizes a new business attempt or an additional Provider budget.
type RecoveryWakeup struct {
	EventID                string
	ExpectedLeaseExpiresAt time.Time
	InvocationPhase        InvocationPhase
	RequestedAt            time.Time
}

func (w RecoveryWakeup) Validate() error {
	if strings.TrimSpace(w.EventID) == "" || len(strings.TrimSpace(w.EventID)) > 768 ||
		w.ExpectedLeaseExpiresAt.IsZero() || !w.InvocationPhase.IsValid() ||
		w.InvocationPhase == InvocationPhaseNone || w.InvocationPhase == InvocationPhaseResponseReceived ||
		w.InvocationPhase == InvocationPhaseResultUnknown || w.RequestedAt.IsZero() ||
		w.RequestedAt.Before(w.ExpectedLeaseExpiresAt) {
		return fmt.Errorf("AI explanation recovery wake-up is invalid")
	}
	return nil
}

func (w RecoveryWakeup) Same(other RecoveryWakeup) bool {
	return w.EventID == other.EventID && w.ExpectedLeaseExpiresAt.Equal(other.ExpectedLeaseExpiresAt) &&
		w.InvocationPhase == other.InvocationPhase
}

// RetryAuthorization is an immutable, one-shot authorization for the next
// attempt. It contains only operational audit facts; participant input and
// Provider output never enter the wake-up contract.
type RetryAuthorization struct {
	ExpectedAttempt           int
	NextAttempt               int
	Origin                    retrygovernance.AttemptOrigin
	RequestID                 string
	EventID                   string
	Actor                     string
	Reason                    string
	AcceptedResultUnknownRisk bool
	AuthorizedAt              time.Time
}

func (a RetryAuthorization) Validate() error {
	if a.ExpectedAttempt < 1 || a.NextAttempt != a.ExpectedAttempt+1 || a.Origin != retrygovernance.AttemptOriginManual ||
		strings.TrimSpace(a.RequestID) == "" || len(strings.TrimSpace(a.RequestID)) > 256 ||
		strings.TrimSpace(a.EventID) == "" || len(strings.TrimSpace(a.EventID)) > 768 ||
		strings.TrimSpace(a.Actor) == "" || len(strings.TrimSpace(a.Actor)) > 256 ||
		strings.TrimSpace(a.Reason) == "" || len(strings.TrimSpace(a.Reason)) > 1000 || a.AuthorizedAt.IsZero() {
		return fmt.Errorf("AI explanation retry authorization is invalid")
	}
	return nil
}

// SameAction compares the stable governance decision. AuthorizedAt is audit
// metadata assigned by the winning request and does not participate in client
// idempotency replay matching.
func (a RetryAuthorization) SameAction(other RetryAuthorization) bool {
	return a.ExpectedAttempt == other.ExpectedAttempt && a.NextAttempt == other.NextAttempt && a.Origin == other.Origin &&
		a.RequestID == other.RequestID && a.EventID == other.EventID && a.Actor == other.Actor && a.Reason == other.Reason &&
		a.AcceptedResultUnknownRisk == other.AcceptedResultUnknownRisk
}

// AIExplanationRun is one provider invocation attempt. InvocationID is stable
// across safe lease recovery and is supplied to provider adapters as their
// idempotency identity when supported.
type AIExplanationRun struct {
	id                 meta.ID
	generationID       meta.ID
	attempt            int
	status             Status
	failure            *Failure
	traceID            string
	startedAt          *time.Time
	leaseExpiresAt     *time.Time
	finishedAt         *time.Time
	origin             retrygovernance.AttemptOrigin
	invocationID       string
	invocationPhase    InvocationPhase
	dispatchStartedAt  *time.Time
	receipt            *aiexplanation.ProviderReceipt
	retryAuthorization *RetryAuthorization
	recoveryWakeup     *RecoveryWakeup
	claimHistory       []ClaimRecord
	recoveryCount      int
	lastReclaimedAt    *time.Time
}

func NewPending(id, generationID meta.ID, attempt int, origin retrygovernance.AttemptOrigin) (*AIExplanationRun, error) {
	if id.IsZero() || generationID.IsZero() || attempt < 1 {
		return nil, fmt.Errorf("AI explanation run identity and positive attempt are required")
	}
	if !origin.IsValid() {
		return nil, fmt.Errorf("AI explanation attempt origin is invalid")
	}
	return &AIExplanationRun{
		id: id, generationID: generationID, attempt: attempt, status: StatusPending,
		origin: origin, invocationPhase: InvocationPhaseNone,
	}, nil
}

func Next(id meta.ID, latest *AIExplanationRun, origin retrygovernance.AttemptOrigin) (*AIExplanationRun, error) {
	if latest == nil || latest.status != StatusFailed {
		return nil, fmt.Errorf("next AI explanation run requires a failed latest run")
	}
	return NewPending(id, latest.generationID, latest.attempt+1, origin)
}

type RestoreInput struct {
	ID                 meta.ID
	GenerationID       meta.ID
	Attempt            int
	Status             Status
	Failure            *Failure
	TraceID            string
	StartedAt          *time.Time
	LeaseExpiresAt     *time.Time
	FinishedAt         *time.Time
	Origin             retrygovernance.AttemptOrigin
	InvocationID       string
	InvocationPhase    InvocationPhase
	DispatchStartedAt  *time.Time
	Receipt            *aiexplanation.ProviderReceipt
	RetryAuthorization *RetryAuthorization
	RecoveryWakeup     *RecoveryWakeup
	ClaimHistory       []ClaimRecord
	RecoveryCount      int
	LastReclaimedAt    *time.Time
}

func Restore(input RestoreInput) (*AIExplanationRun, error) {
	r, err := NewPending(input.ID, input.GenerationID, input.Attempt, input.Origin)
	if err != nil {
		return nil, err
	}
	if !input.Status.IsValid() || !input.InvocationPhase.IsValid() {
		return nil, fmt.Errorf("AI explanation run persistence status is invalid")
	}
	if err := validatePersistedState(input); err != nil {
		return nil, err
	}
	r.status = input.Status
	r.traceID = input.TraceID
	r.startedAt = copyTimePtr(input.StartedAt)
	r.leaseExpiresAt = copyTimePtr(input.LeaseExpiresAt)
	r.finishedAt = copyTimePtr(input.FinishedAt)
	r.invocationID = input.InvocationID
	r.invocationPhase = input.InvocationPhase
	r.dispatchStartedAt = copyTimePtr(input.DispatchStartedAt)
	r.claimHistory = copyClaims(input.ClaimHistory)
	r.recoveryCount = input.RecoveryCount
	r.lastReclaimedAt = copyTimePtr(input.LastReclaimedAt)
	if input.Failure != nil {
		failure := *input.Failure
		r.failure = &failure
	}
	if input.Receipt != nil {
		receipt := *input.Receipt
		r.receipt = &receipt
	}
	if input.RetryAuthorization != nil {
		authorization := *input.RetryAuthorization
		r.retryAuthorization = &authorization
	}
	if input.RecoveryWakeup != nil {
		wakeup := *input.RecoveryWakeup
		r.recoveryWakeup = &wakeup
	}
	return r, nil
}

func validatePersistedState(input RestoreInput) error {
	switch input.Status {
	case StatusPending:
		if input.StartedAt != nil || input.LeaseExpiresAt != nil || input.FinishedAt != nil || input.Failure != nil || input.InvocationID != "" || input.InvocationPhase != InvocationPhaseNone {
			return fmt.Errorf("pending AI explanation run has execution state")
		}
	case StatusRunning:
		if input.StartedAt == nil || input.FinishedAt != nil || input.Failure != nil || strings.TrimSpace(input.InvocationID) == "" || input.InvocationPhase == InvocationPhaseNone {
			return fmt.Errorf("running AI explanation run state is invalid")
		}
	case StatusSucceeded:
		if input.StartedAt == nil || input.LeaseExpiresAt != nil || input.FinishedAt == nil || input.Failure != nil || input.InvocationPhase != InvocationPhaseResponseReceived || input.Receipt == nil {
			return fmt.Errorf("succeeded AI explanation run state is invalid")
		}
	case StatusFailed:
		if input.StartedAt == nil || input.LeaseExpiresAt != nil || input.FinishedAt == nil || input.Failure == nil {
			return fmt.Errorf("failed AI explanation run state is invalid")
		}
	}
	if input.Failure != nil {
		if err := input.Failure.Validate(); err != nil {
			return err
		}
	}
	if input.Receipt != nil {
		if err := input.Receipt.Validate(); err != nil {
			return err
		}
		if input.Receipt.InvocationID != input.InvocationID {
			return fmt.Errorf("AI explanation provider receipt invocation mismatch")
		}
	}
	if input.RetryAuthorization != nil {
		if input.Status != StatusFailed || input.Failure == nil || input.RetryAuthorization.ExpectedAttempt != input.Attempt {
			return fmt.Errorf("AI explanation retry authorization requires its failed attempt")
		}
		if err := input.RetryAuthorization.Validate(); err != nil {
			return err
		}
		if input.Attempt >= retrygovernance.HardMaxBusinessAttempts {
			return ErrRetryNotAllowed
		}
		if !input.Failure.Retryable && (input.Failure.Code != "provider_result_unknown" || !input.RetryAuthorization.AcceptedResultUnknownRisk) {
			return ErrRetryNotAllowed
		}
	}
	if input.RecoveryWakeup != nil {
		if err := input.RecoveryWakeup.Validate(); err != nil {
			return err
		}
		if input.Status != StatusRunning || input.LeaseExpiresAt == nil ||
			!input.LeaseExpiresAt.Equal(input.RecoveryWakeup.ExpectedLeaseExpiresAt) ||
			input.InvocationPhase != input.RecoveryWakeup.InvocationPhase {
			return ErrRecoveryNotAllowed
		}
	}
	switch input.InvocationPhase {
	case InvocationPhaseNone:
		if input.Status != StatusPending || input.DispatchStartedAt != nil || input.Receipt != nil {
			return fmt.Errorf("empty AI explanation invocation phase is invalid")
		}
	case InvocationPhasePrepared:
		if input.DispatchStartedAt != nil || input.Receipt != nil {
			return fmt.Errorf("prepared AI explanation invocation state is invalid")
		}
	case InvocationPhaseDispatching, InvocationPhaseResultUnknown:
		if input.DispatchStartedAt == nil || input.Receipt != nil {
			return fmt.Errorf("dispatched AI explanation invocation state is invalid")
		}
	case InvocationPhaseResponseReceived:
		if input.DispatchStartedAt == nil || input.Receipt == nil {
			return fmt.Errorf("received AI explanation invocation state is invalid")
		}
	}
	if input.DispatchStartedAt != nil && input.StartedAt == nil {
		return fmt.Errorf("AI explanation provider dispatch requires a started run")
	}
	if input.StartedAt != nil && input.FinishedAt != nil && input.FinishedAt.Before(*input.StartedAt) {
		return fmt.Errorf("AI explanation run finished before it started")
	}
	return nil
}

func (r *AIExplanationRun) StartWithLease(at time.Time, traceID string, leaseExpiresAt time.Time, invocationID string) error {
	if r == nil || r.status != StatusPending {
		return fmt.Errorf("pending AI explanation run is required")
	}
	if at.IsZero() || !leaseExpiresAt.After(at) || strings.TrimSpace(traceID) == "" || strings.TrimSpace(invocationID) == "" {
		return fmt.Errorf("AI explanation run lease, trace and invocation are required")
	}
	r.status = StatusRunning
	r.traceID = strings.TrimSpace(traceID)
	r.startedAt = copyTime(at)
	r.leaseExpiresAt = copyTime(leaseExpiresAt)
	r.invocationID = strings.TrimSpace(invocationID)
	r.invocationPhase = InvocationPhasePrepared
	return nil
}

// BeginProviderDispatch must be persisted before the external request is sent.
func (r *AIExplanationRun) BeginProviderDispatch(at time.Time) error {
	if r == nil || r.status != StatusRunning || r.invocationPhase != InvocationPhasePrepared || at.IsZero() {
		return fmt.Errorf("prepared running AI explanation invocation is required")
	}
	r.invocationPhase = InvocationPhaseDispatching
	r.dispatchStartedAt = copyTime(at)
	return nil
}

func (r *AIExplanationRun) RecordProviderResponse(receipt aiexplanation.ProviderReceipt) error {
	if r == nil || r.status != StatusRunning || r.invocationPhase != InvocationPhaseDispatching {
		return fmt.Errorf("dispatched running AI explanation invocation is required")
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.InvocationID != r.invocationID {
		return fmt.Errorf("AI explanation provider receipt invocation mismatch")
	}
	r.invocationPhase = InvocationPhaseResponseReceived
	r.receipt = &receipt
	return nil
}

func (r *AIExplanationRun) MarkProviderResultUnknown() error {
	if r == nil || r.status != StatusRunning || r.invocationPhase != InvocationPhaseDispatching {
		return fmt.Errorf("dispatched running AI explanation invocation is required")
	}
	r.invocationPhase = InvocationPhaseResultUnknown
	return nil
}

func (r *AIExplanationRun) Succeed(at time.Time) error {
	if r == nil || r.status != StatusRunning || r.invocationPhase != InvocationPhaseResponseReceived || r.receipt == nil || at.IsZero() {
		return fmt.Errorf("validated AI explanation provider response is required")
	}
	r.status = StatusSucceeded
	r.leaseExpiresAt = nil
	r.finishedAt = copyTime(at)
	r.recoveryWakeup = nil
	return nil
}

func (r *AIExplanationRun) Fail(at time.Time, failure Failure) error {
	if r == nil || r.status != StatusRunning || at.IsZero() {
		return fmt.Errorf("running AI explanation run and failure time are required")
	}
	if err := failure.Validate(); err != nil {
		return err
	}
	r.status = StatusFailed
	r.failure = &failure
	r.leaseExpiresAt = nil
	r.finishedAt = copyTime(at)
	r.recoveryWakeup = nil
	return nil
}

// AuthorizeManualRetry grants exactly one new attempt. Retryable failures may
// be retried normally. provider_result_unknown additionally requires explicit
// duplicate-call/cost risk acceptance; other terminal failures stay closed.
func (r *AIExplanationRun) AuthorizeManualRetry(authorization RetryAuthorization) error {
	if r == nil || r.status != StatusFailed || r.failure == nil || r.attempt >= retrygovernance.HardMaxBusinessAttempts {
		return ErrRetryNotAllowed
	}
	if err := authorization.Validate(); err != nil {
		return err
	}
	if authorization.ExpectedAttempt != r.attempt {
		return ErrRetryNotAllowed
	}
	if !r.failure.Retryable && (r.failure.Code != "provider_result_unknown" || !authorization.AcceptedResultUnknownRisk) {
		return ErrRetryNotAllowed
	}
	if r.retryAuthorization != nil {
		if r.retryAuthorization.SameAction(authorization) {
			return nil
		}
		return ErrConflict
	}
	copy := authorization
	copy.RequestID = strings.TrimSpace(copy.RequestID)
	copy.EventID = strings.TrimSpace(copy.EventID)
	copy.Actor = strings.TrimSpace(copy.Actor)
	copy.Reason = strings.TrimSpace(copy.Reason)
	r.retryAuthorization = &copy
	return nil
}

// ScheduleRecoveryWakeup attaches one outbox identity to the exact expired
// lease observed by the scheduler. An old candidate cannot schedule work for a
// newer lease or a different invocation phase.
func (r *AIExplanationRun) ScheduleRecoveryWakeup(wakeup RecoveryWakeup) (bool, error) {
	if r == nil || r.status != StatusRunning || r.leaseExpiresAt == nil {
		return false, ErrRecoveryNotAllowed
	}
	if err := wakeup.Validate(); err != nil {
		return false, err
	}
	if !r.leaseExpiresAt.Equal(wakeup.ExpectedLeaseExpiresAt) || r.invocationPhase != wakeup.InvocationPhase {
		return false, ErrRecoveryNotAllowed
	}
	if r.recoveryWakeup != nil {
		if r.recoveryWakeup.Same(wakeup) {
			return false, nil
		}
		return false, ErrConflict
	}
	copy := wakeup
	copy.EventID = strings.TrimSpace(copy.EventID)
	r.recoveryWakeup = &copy
	return true, nil
}

// ReclaimExpiredLease rejects post-dispatch recovery unless the resolved
// provider route guarantees idempotent redispatch for the stable InvocationID.
func (r *AIExplanationRun) ReclaimExpiredLease(at time.Time, traceID string, leaseExpiresAt time.Time, allowIdempotentRedispatch bool) error {
	if r == nil || r.status != StatusRunning || r.leaseExpiresAt == nil || r.leaseExpiresAt.After(at) {
		return fmt.Errorf("expired running AI explanation lease is required")
	}
	if at.IsZero() || !leaseExpiresAt.After(at) || strings.TrimSpace(traceID) == "" {
		return fmt.Errorf("AI explanation recovery lease is invalid")
	}
	if r.invocationPhase != InvocationPhasePrepared && !allowIdempotentRedispatch {
		return ErrUnsafeLeaseReclaim
	}
	r.traceID = strings.TrimSpace(traceID)
	r.leaseExpiresAt = copyTime(leaseExpiresAt)
	r.recoveryCount++
	r.lastReclaimedAt = copyTime(at)
	r.claimHistory = append(copyClaims(r.claimHistory), ClaimRecord{ReclaimedAt: at, TraceID: r.traceID})
	r.recoveryWakeup = nil
	return nil
}

func (r *AIExplanationRun) ID() meta.ID                           { return r.id }
func (r *AIExplanationRun) GenerationID() meta.ID                 { return r.generationID }
func (r *AIExplanationRun) Attempt() int                          { return r.attempt }
func (r *AIExplanationRun) Status() Status                        { return r.status }
func (r *AIExplanationRun) TraceID() string                       { return r.traceID }
func (r *AIExplanationRun) Origin() retrygovernance.AttemptOrigin { return r.origin }
func (r *AIExplanationRun) InvocationID() string                  { return r.invocationID }
func (r *AIExplanationRun) InvocationPhase() InvocationPhase      { return r.invocationPhase }
func (r *AIExplanationRun) StartedAt() *time.Time                 { return copyTimePtr(r.startedAt) }
func (r *AIExplanationRun) LeaseExpiresAt() *time.Time            { return copyTimePtr(r.leaseExpiresAt) }
func (r *AIExplanationRun) FinishedAt() *time.Time                { return copyTimePtr(r.finishedAt) }
func (r *AIExplanationRun) DispatchStartedAt() *time.Time         { return copyTimePtr(r.dispatchStartedAt) }
func (r *AIExplanationRun) RecoveryCount() int                    { return r.recoveryCount }
func (r *AIExplanationRun) LastReclaimedAt() *time.Time           { return copyTimePtr(r.lastReclaimedAt) }
func (r *AIExplanationRun) ClaimHistory() []ClaimRecord           { return copyClaims(r.claimHistory) }

func (r *AIExplanationRun) RecoveryWakeup() *RecoveryWakeup {
	if r == nil || r.recoveryWakeup == nil {
		return nil
	}
	wakeup := *r.recoveryWakeup
	return &wakeup
}

func (r *AIExplanationRun) Failure() *Failure {
	if r == nil || r.failure == nil {
		return nil
	}
	failure := *r.failure
	return &failure
}

func (r *AIExplanationRun) ProviderReceipt() *aiexplanation.ProviderReceipt {
	if r == nil || r.receipt == nil {
		return nil
	}
	receipt := *r.receipt
	return &receipt
}

func (r *AIExplanationRun) RetryAuthorization() *RetryAuthorization {
	if r == nil || r.retryAuthorization == nil {
		return nil
	}
	copy := *r.retryAuthorization
	return &copy
}

func copyTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return copyTime(*value)
}

func copyClaims(values []ClaimRecord) []ClaimRecord {
	return append([]ClaimRecord(nil), values...)
}
