package evaluation

import (
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const RecheckProviderInvocationsV1 = 2

type RecheckStatus string

const (
	RecheckStatusQueued        RecheckStatus = "queued"
	RecheckStatusDispatching   RecheckStatus = "dispatching"
	RecheckStatusCompleted     RecheckStatus = "completed"
	RecheckStatusFailed        RecheckStatus = "failed"
	RecheckStatusResultUnknown RecheckStatus = "result_unknown"
)

func (s RecheckStatus) IsValid() bool {
	switch s {
	case RecheckStatusQueued, RecheckStatusDispatching, RecheckStatusCompleted, RecheckStatusFailed, RecheckStatusResultUnknown:
		return true
	default:
		return false
	}
}

func (s RecheckStatus) IsTerminal() bool {
	return s == RecheckStatusCompleted || s == RecheckStatusFailed || s == RecheckStatusResultUnknown
}

// PromptEvaluationRecheck is diagnostic evidence for one source attempt. It
// is deliberately separate from PromptEvaluationRun so a recheck cannot
// rewrite release evidence, human-review progress or a published gate.
type PromptEvaluationRecheck struct {
	id            meta.ID
	sourceRunID   meta.ID
	sourceCaseID  string
	sourceAttempt int
	release       ReleaseIdentity
	status        RecheckStatus
	version       int64
	execution     *AttemptExecution
	result        *AttemptRecord
	requestedOrg  int64
	requestedBy   string
	reason        string
	createdAt     time.Time
	finishedAt    *time.Time
}

func NewPromptEvaluationRecheck(
	id, sourceRunID meta.ID,
	sourceCaseID string,
	sourceAttempt int,
	release ReleaseIdentity,
	requestedOrg int64,
	requestedBy, reason string,
	createdAt time.Time,
) (*PromptEvaluationRecheck, error) {
	sourceCaseID, requestedBy, reason = strings.TrimSpace(sourceCaseID), strings.TrimSpace(requestedBy), strings.TrimSpace(reason)
	if id.IsZero() || sourceRunID.IsZero() || sourceCaseID == "" || sourceAttempt < 1 || requestedOrg <= 0 ||
		requestedBy == "" || len(requestedBy) > 256 || reason == "" || len(reason) > 1000 || createdAt.IsZero() {
		return nil, fmt.Errorf("AI explanation attempt recheck identity and audit are required")
	}
	if err := release.Validate(); err != nil {
		return nil, err
	}
	if !release.IsGenerationCase(sourceCaseID) || sourceAttempt > release.RepetitionsPerCase {
		return nil, fmt.Errorf("AI explanation attempt recheck target is outside the candidate release suite")
	}
	return &PromptEvaluationRecheck{
		id: id, sourceRunID: sourceRunID, sourceCaseID: sourceCaseID, sourceAttempt: sourceAttempt,
		release: cloneRelease(release), status: RecheckStatusQueued, version: 1,
		requestedOrg: requestedOrg, requestedBy: requestedBy, reason: reason, createdAt: createdAt,
	}, nil
}

func (r *PromptEvaluationRecheck) BeginDispatch(owner string, at time.Time, leaseDuration time.Duration) error {
	owner = strings.TrimSpace(owner)
	if r == nil || r.status != RecheckStatusQueued || owner == "" || len(owner) > 256 || at.IsZero() ||
		at.Before(r.createdAt) || leaseDuration <= 0 {
		return fmt.Errorf("queued AI explanation attempt recheck and dispatch lease are required")
	}
	dispatchAt := at
	r.execution = &AttemptExecution{
		CaseID: r.sourceCaseID, Attempt: r.sourceAttempt, Owner: owner,
		InvocationID: "ai-prompt-evaluation-recheck:" + r.id.String(), Phase: AttemptExecutionDispatching,
		ClaimedAt: at, LeaseExpiresAt: at.Add(leaseDuration), DispatchStartedAt: &dispatchAt,
	}
	if err := r.execution.Validate(); err != nil {
		return err
	}
	r.status = RecheckStatusDispatching
	r.version++
	return nil
}

func (r *PromptEvaluationRecheck) Complete(owner string, result AttemptRecord) error {
	owner = strings.TrimSpace(owner)
	if r == nil || r.status != RecheckStatusDispatching || r.execution == nil || owner == "" || owner != r.execution.Owner ||
		result.CaseID != r.sourceCaseID || result.Attempt != r.sourceAttempt || result.Stage != AttemptStageGeneration ||
		result.StartedAt.Before(*r.execution.DispatchStartedAt) {
		return fmt.Errorf("dispatching AI explanation attempt recheck and matching evidence are required")
	}
	if err := result.Validate(); err != nil {
		return err
	}
	if result.ProviderReceipt != nil && (result.ProviderReceipt.Provider != r.release.Provider.ResolvedProvider ||
		result.ProviderReceipt.Model != r.release.Provider.ResolvedModel) {
		return fmt.Errorf("AI explanation attempt recheck Provider receipt does not match the candidate release")
	}
	if result.Semantic != nil && (result.Semantic.EvaluatorVersion != r.release.SemanticEvaluator.Version ||
		result.Semantic.ProviderReceipt.Provider != r.release.SemanticEvaluator.Provider.ResolvedProvider ||
		result.Semantic.ProviderReceipt.Model != r.release.SemanticEvaluator.Provider.ResolvedModel) {
		return fmt.Errorf("AI explanation attempt recheck semantic receipt does not match the candidate release")
	}
	if result.SemanticExecution != nil {
		if result.SemanticExecution.EvaluatorVersion != r.release.SemanticEvaluator.Version {
			return fmt.Errorf("AI explanation attempt recheck semantic execution does not match the candidate release")
		}
		receipt := result.SemanticExecution.ProviderReceipt
		failure := result.SemanticExecution.Failure
		if receipt != nil && (failure == nil || failure.Code != SemanticReceiptInvalid) &&
			(receipt.Provider != r.release.SemanticEvaluator.Provider.ResolvedProvider || receipt.Model != r.release.SemanticEvaluator.Provider.ResolvedModel) {
			return fmt.Errorf("AI explanation attempt recheck semantic execution receipt does not match the candidate release")
		}
	}
	r.result = cloneAttemptPtr(&result)
	r.execution = nil
	finishedAt := result.FinishedAt
	r.finishedAt = &finishedAt
	switch {
	case result.Failure == nil:
		r.status = RecheckStatusCompleted
	case result.Failure.ResultUnknown:
		r.status = RecheckStatusResultUnknown
	default:
		r.status = RecheckStatusFailed
	}
	r.version++
	return nil
}

type PromptEvaluationRecheckPersistedInput struct {
	ID            meta.ID
	SourceRunID   meta.ID
	SourceCaseID  string
	SourceAttempt int
	Release       ReleaseIdentity
	Status        RecheckStatus
	Version       int64
	Execution     *AttemptExecution
	Result        *AttemptRecord
	RequestedOrg  int64
	RequestedBy   string
	Reason        string
	CreatedAt     time.Time
	FinishedAt    *time.Time
}

func RestorePromptEvaluationRecheck(input PromptEvaluationRecheckPersistedInput) (*PromptEvaluationRecheck, error) {
	value, err := NewPromptEvaluationRecheck(
		input.ID, input.SourceRunID, input.SourceCaseID, input.SourceAttempt, input.Release,
		input.RequestedOrg, input.RequestedBy, input.Reason, input.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if input.Version < 1 || !input.Status.IsValid() {
		return nil, fmt.Errorf("AI explanation attempt recheck persistence state is invalid")
	}
	value.status = input.Status
	value.version = input.Version
	value.execution = cloneAttemptExecution(input.Execution)
	value.result = cloneAttemptPtr(input.Result)
	value.finishedAt = copyTimePtr(input.FinishedAt)
	if err := value.validateLifecycle(); err != nil {
		return nil, err
	}
	return value, nil
}

func (r *PromptEvaluationRecheck) validateLifecycle() error {
	if r == nil {
		return fmt.Errorf("AI explanation attempt recheck is required")
	}
	switch r.status {
	case RecheckStatusQueued:
		if r.execution != nil || r.result != nil || r.finishedAt != nil {
			return fmt.Errorf("queued AI explanation attempt recheck lifecycle is invalid")
		}
	case RecheckStatusDispatching:
		if r.execution == nil || r.execution.Validate() != nil || r.execution.Phase != AttemptExecutionDispatching ||
			r.execution.CaseID != r.sourceCaseID || r.execution.Attempt != r.sourceAttempt || r.result != nil || r.finishedAt != nil {
			return fmt.Errorf("dispatching AI explanation attempt recheck lifecycle is invalid")
		}
	case RecheckStatusCompleted, RecheckStatusFailed, RecheckStatusResultUnknown:
		if r.execution != nil || r.result == nil || r.finishedAt == nil || !r.finishedAt.Equal(r.result.FinishedAt) ||
			r.result.Validate() != nil || r.result.CaseID != r.sourceCaseID || r.result.Attempt != r.sourceAttempt {
			return fmt.Errorf("terminal AI explanation attempt recheck lifecycle is invalid")
		}
		expected := RecheckStatusCompleted
		if r.result.Failure != nil {
			expected = RecheckStatusFailed
			if r.result.Failure.ResultUnknown {
				expected = RecheckStatusResultUnknown
			}
		}
		if r.status != expected {
			return fmt.Errorf("AI explanation attempt recheck terminal status is inconsistent")
		}
	}
	return nil
}

func (r *PromptEvaluationRecheck) ID() meta.ID              { return r.id }
func (r *PromptEvaluationRecheck) SourceRunID() meta.ID     { return r.sourceRunID }
func (r *PromptEvaluationRecheck) SourceCaseID() string     { return r.sourceCaseID }
func (r *PromptEvaluationRecheck) SourceAttempt() int       { return r.sourceAttempt }
func (r *PromptEvaluationRecheck) Release() ReleaseIdentity { return cloneRelease(r.release) }
func (r *PromptEvaluationRecheck) Status() RecheckStatus    { return r.status }
func (r *PromptEvaluationRecheck) Version() int64           { return r.version }
func (r *PromptEvaluationRecheck) RequestedOrgID() int64    { return r.requestedOrg }
func (r *PromptEvaluationRecheck) RequestedBy() string      { return r.requestedBy }
func (r *PromptEvaluationRecheck) Reason() string           { return r.reason }
func (r *PromptEvaluationRecheck) CreatedAt() time.Time     { return r.createdAt }
func (r *PromptEvaluationRecheck) FinishedAt() *time.Time   { return copyTimePtr(r.finishedAt) }
func (r *PromptEvaluationRecheck) Execution() *AttemptExecution {
	return cloneAttemptExecution(r.execution)
}
func (r *PromptEvaluationRecheck) Result() *AttemptRecord { return cloneAttemptPtr(r.result) }

func cloneAttemptPtr(value *AttemptRecord) *AttemptRecord {
	if value == nil {
		return nil
	}
	cloned := cloneAttempt(*value)
	return &cloned
}
