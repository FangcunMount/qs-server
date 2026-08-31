package evaluation

import (
	"fmt"
	"strings"
	"time"
)

type EvidenceExecutionKind string

const (
	EvidenceExecutionGeneration EvidenceExecutionKind = "generation"
	EvidenceExecutionSemantic   EvidenceExecutionKind = "semantic"
)

func (k EvidenceExecutionKind) IsValid() bool {
	return k == EvidenceExecutionGeneration || k == EvidenceExecutionSemantic
}

// EvidenceExecutionCheckpoint is the only mutable in-flight checkpoint for a
// v2 Run. Terminal execution evidence remains append-only in the aggregate.
type EvidenceExecutionCheckpoint struct {
	ID                string
	Kind              EvidenceExecutionKind
	CaseID            string
	SlotOrdinal       int
	CandidateID       string
	ExecutionOrdinal  int
	Owner             string
	InvocationID      string
	Phase             AttemptExecutionPhase
	ClaimedAt         time.Time
	LeaseExpiresAt    time.Time
	DispatchStartedAt *time.Time
}

func (c EvidenceExecutionCheckpoint) Validate() error {
	if !evidenceEntityIDPattern.MatchString(c.ID) || !c.Kind.IsValid() ||
		!evidenceEntityIDPattern.MatchString(c.CaseID) || c.SlotOrdinal < 1 || c.SlotOrdinal > RequiredRepetitionsPerCase ||
		c.ExecutionOrdinal < 1 || c.ExecutionOrdinal > 2 || !evidenceEntityIDPattern.MatchString(c.Owner) ||
		!evidenceEntityIDPattern.MatchString(c.InvocationID) || c.ClaimedAt.IsZero() || !c.LeaseExpiresAt.After(c.ClaimedAt) {
		return fmt.Errorf("AI explanation evidence execution checkpoint is invalid")
	}
	if c.Kind == EvidenceExecutionGeneration && c.CandidateID != "" ||
		c.Kind == EvidenceExecutionSemantic && !evidenceEntityIDPattern.MatchString(c.CandidateID) {
		return fmt.Errorf("AI explanation evidence execution checkpoint target is invalid")
	}
	switch c.Phase {
	case AttemptExecutionPrepared:
		if c.DispatchStartedAt != nil {
			return fmt.Errorf("prepared AI explanation evidence execution cannot have dispatch time")
		}
	case AttemptExecutionDispatching:
		if c.DispatchStartedAt == nil || c.DispatchStartedAt.Before(c.ClaimedAt) || c.DispatchStartedAt.After(c.LeaseExpiresAt) {
			return fmt.Errorf("dispatching AI explanation evidence execution requires a valid dispatch time")
		}
	default:
		return fmt.Errorf("AI explanation evidence execution phase is invalid")
	}
	return nil
}

func (c EvidenceExecutionCheckpoint) LeaseExpired(at time.Time) bool {
	return !at.IsZero() && !at.Before(c.LeaseExpiresAt)
}

type EvidenceNextActionKind string

const (
	EvidenceNextActionNone        EvidenceNextActionKind = "none"
	EvidenceNextActionPreflight   EvidenceNextActionKind = "preflight"
	EvidenceNextActionGeneration  EvidenceNextActionKind = "generation"
	EvidenceNextActionSemantic    EvidenceNextActionKind = "semantic"
	EvidenceNextActionAwaitReview EvidenceNextActionKind = "await_review"
	EvidenceNextActionBlock       EvidenceNextActionKind = "block"
)

type EvidenceNextAction struct {
	Kind             EvidenceNextActionKind
	CaseID           string
	SlotOrdinal      int
	CandidateID      string
	ExecutionOrdinal int
	Resume           bool
	CauseCode        string
}

// NextAction selects exactly one target in the frozen Slot order. It does not
// mutate evidence and never skips a failed or unresolved earlier Slot.
func (e PromptEvaluationEvidenceV2) NextAction() (EvidenceNextAction, error) {
	if err := e.Validate(); err != nil {
		return EvidenceNextAction{}, err
	}
	if e.Status == EvidenceStatusBlocked {
		return EvidenceNextAction{Kind: EvidenceNextActionBlock, CauseCode: "run_blocked"}, nil
	}
	if e.Status != EvidenceStatusCollecting {
		return EvidenceNextAction{Kind: EvidenceNextActionNone, CauseCode: "run_not_collecting"}, nil
	}
	if e.execution != nil {
		kind := EvidenceNextActionGeneration
		if e.execution.Kind == EvidenceExecutionSemantic {
			kind = EvidenceNextActionSemantic
		}
		return EvidenceNextAction{
			Kind: kind, CaseID: e.execution.CaseID, SlotOrdinal: e.execution.SlotOrdinal,
			CandidateID: e.execution.CandidateID, ExecutionOrdinal: e.execution.ExecutionOrdinal,
			Resume: true, CauseCode: "resume_active_execution",
		}, nil
	}
	if e.UnresolvedResultUnknownCount > 0 {
		return EvidenceNextAction{Kind: EvidenceNextActionBlock, CauseCode: "result_unknown_requires_review"}, nil
	}
	preflight := e.PreflightEvidence[0]
	switch preflight.Status {
	case PreflightEvidencePending:
		return EvidenceNextAction{Kind: EvidenceNextActionPreflight, CaseID: preflight.CaseID, CauseCode: "preflight_pending"}, nil
	case PreflightEvidenceFailed:
		return EvidenceNextAction{Kind: EvidenceNextActionBlock, CaseID: preflight.CaseID, CauseCode: "preflight_failed"}, nil
	}

	generationByID := make(map[string]CandidateGenerationExecution, len(e.GenerationExecutions))
	for _, execution := range e.GenerationExecutions {
		generationByID[execution.ID] = execution
	}
	semanticByID := make(map[string]SemanticEvaluationExecution, len(e.SemanticExecutions))
	for _, execution := range e.SemanticExecutions {
		semanticByID[execution.ID] = execution
	}
	for _, slot := range e.Slots {
		if slot.Candidate == nil {
			nextOrdinal := len(slot.GenerationExecutionIDs) + 1
			if len(slot.GenerationExecutionIDs) == 0 {
				return generationAction(slot, nextOrdinal, "candidate_missing"), nil
			}
			last := generationByID[slot.GenerationExecutionIDs[len(slot.GenerationExecutionIDs)-1]]
			if last.Status == ExecutionStatusResultUnknown && !e.replacementAuthorized(last.ID) {
				return blockedSlotAction(slot, "result_unknown_requires_review"), nil
			}
			if nextOrdinal > e.ExecutionPolicy.Generation.MaxExecutionsPerSlot {
				return blockedSlotAction(slot, "generation_budget_exhausted"), nil
			}
			if last.Failure == nil || !allowsGenerationRecovery(e.ExecutionPolicy.Recovery, *last.Failure, e.replacementAuthorized(last.ID)) {
				return blockedSlotAction(slot, "generation_recovery_not_allowed"), nil
			}
			return generationAction(slot, nextOrdinal, "generation_recovery_allowed"), nil
		}
		if slot.Candidate.ReviewReady {
			continue
		}
		nextOrdinal := len(slot.Candidate.SemanticExecutionIDs) + 1
		if len(slot.Candidate.SemanticExecutionIDs) == 0 {
			return semanticAction(slot, nextOrdinal, "semantic_evidence_missing"), nil
		}
		last := semanticByID[slot.Candidate.SemanticExecutionIDs[len(slot.Candidate.SemanticExecutionIDs)-1]]
		if last.Status == ExecutionStatusResultUnknown && !e.replacementAuthorized(last.ID) {
			return blockedSlotAction(slot, "result_unknown_requires_review"), nil
		}
		if nextOrdinal > e.ExecutionPolicy.Semantic.MaxExecutionsPerCandidate {
			return blockedSlotAction(slot, "semantic_budget_exhausted"), nil
		}
		if last.Failure == nil || !allowsSemanticRecovery(e.ExecutionPolicy.Recovery, *last.Failure, e.replacementAuthorized(last.ID)) {
			return blockedSlotAction(slot, "semantic_recovery_not_allowed"), nil
		}
		return semanticAction(slot, nextOrdinal, "semantic_recovery_allowed"), nil
	}
	return EvidenceNextAction{Kind: EvidenceNextActionAwaitReview, CauseCode: "candidate_evidence_complete"}, nil
}

func (e *PromptEvaluationEvidenceV2) BeginNextExecution(checkpoint EvidenceExecutionCheckpoint) error {
	if e == nil || e.Status != EvidenceStatusCollecting || e.execution != nil {
		return fmt.Errorf("collecting AI explanation evidence without an active execution is required")
	}
	if err := checkpoint.Validate(); err != nil || checkpoint.Phase != AttemptExecutionPrepared || checkpoint.ClaimedAt.Before(e.Audit.CreatedAt) {
		return fmt.Errorf("prepared AI explanation evidence execution checkpoint is invalid")
	}
	action, err := e.NextAction()
	if err != nil {
		return err
	}
	expectedKind := EvidenceExecutionGeneration
	if action.Kind == EvidenceNextActionSemantic {
		expectedKind = EvidenceExecutionSemantic
	} else if action.Kind != EvidenceNextActionGeneration {
		return fmt.Errorf("AI explanation evidence next action is not a Provider execution")
	}
	if checkpoint.Kind != expectedKind || checkpoint.CaseID != action.CaseID || checkpoint.SlotOrdinal != action.SlotOrdinal ||
		checkpoint.CandidateID != action.CandidateID || checkpoint.ExecutionOrdinal != action.ExecutionOrdinal {
		return fmt.Errorf("AI explanation evidence execution target is not the next frozen action")
	}
	cloned := checkpoint
	e.execution = &cloned
	e.version++
	return nil
}

func (e *PromptEvaluationEvidenceV2) MarkExecutionDispatching(owner string, at time.Time) error {
	owner = strings.TrimSpace(owner)
	if e == nil || e.Status != EvidenceStatusCollecting || e.execution == nil || e.execution.Phase != AttemptExecutionPrepared ||
		owner == "" || owner != e.execution.Owner || at.IsZero() || at.Before(e.execution.ClaimedAt) || at.After(e.execution.LeaseExpiresAt) {
		return fmt.Errorf("prepared AI explanation evidence execution and matching owner are required")
	}
	e.execution.Phase = AttemptExecutionDispatching
	e.execution.DispatchStartedAt = copyTime(at)
	e.version++
	return nil
}

func (e *PromptEvaluationEvidenceV2) ReleaseExpiredPreparation(at time.Time) error {
	if e == nil || e.Status != EvidenceStatusCollecting || e.execution == nil ||
		e.execution.Phase != AttemptExecutionPrepared || !e.execution.LeaseExpired(at) {
		return fmt.Errorf("expired prepared AI explanation evidence execution is required")
	}
	e.execution = nil
	e.version++
	return nil
}

func (e *PromptEvaluationEvidenceV2) CompletePreflight(value PreflightCaseEvidence) error {
	if e == nil || e.Status != EvidenceStatusCollecting || len(e.PreflightEvidence) != 1 ||
		e.PreflightEvidence[0].Status != PreflightEvidencePending || value.CaseID != e.PreflightEvidence[0].CaseID ||
		value.Status == PreflightEvidencePending || value.EvaluatedAt == nil || value.EvaluatedAt.Before(e.Audit.CreatedAt) {
		return fmt.Errorf("pending AI explanation preflight evidence is required")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	cloned := e.Clone()
	cloned.PreflightEvidence[0] = clonePreflightEvidence(value)
	if value.Status == PreflightEvidenceFailed {
		if err := cloned.Transition(EvidenceStatusBlocked, "preflight_failed", "system:runner", []string{value.CaseID}, *value.EvaluatedAt); err != nil {
			return err
		}
	} else {
		cloned.version++
		if err := cloned.Validate(); err != nil {
			return err
		}
	}
	*e = cloned
	return nil
}

// CompleteGenerationExecution atomically clears the dispatch checkpoint,
// appends terminal evidence and accepts the first contract-conformant output.
func (e *PromptEvaluationEvidenceV2) CompleteGenerationExecution(owner, candidateID string, assertions []AssertionReceipt, execution CandidateGenerationExecution) error {
	owner, candidateID = strings.TrimSpace(owner), strings.TrimSpace(candidateID)
	if e == nil || e.Status != EvidenceStatusCollecting || e.execution == nil || e.execution.Kind != EvidenceExecutionGeneration ||
		e.execution.Phase != AttemptExecutionDispatching || owner == "" || owner != e.execution.Owner {
		return fmt.Errorf("dispatching AI explanation generation checkpoint and matching owner are required")
	}
	if err := execution.Validate(); err != nil || execution.Status == ExecutionStatusPrepared || execution.Status == ExecutionStatusDispatching ||
		execution.ID != e.execution.ID || execution.CaseID != e.execution.CaseID || execution.SlotOrdinal != e.execution.SlotOrdinal ||
		execution.ExecutionOrdinal != e.execution.ExecutionOrdinal || execution.InvocationID != e.execution.InvocationID ||
		e.execution.DispatchStartedAt == nil || execution.StartedAt.Before(*e.execution.DispatchStartedAt) {
		return fmt.Errorf("terminal AI explanation generation evidence does not match its checkpoint")
	}
	if execution.Status == ExecutionStatusSucceeded {
		if !evidenceEntityIDPattern.MatchString(candidateID) || len(assertions) == 0 {
			return fmt.Errorf("successful AI explanation generation requires candidate evidence")
		}
		for _, assertion := range assertions {
			if err := assertion.Validate(); err != nil {
				return err
			}
		}
	} else if candidateID != "" || len(assertions) != 0 {
		return fmt.Errorf("failed AI explanation generation cannot create a candidate")
	}

	cloned := e.Clone()
	cloned.execution = nil
	slot := cloned.slot(execution.CaseID, execution.SlotOrdinal)
	if slot == nil || slot.Candidate != nil {
		return fmt.Errorf("AI explanation generation checkpoint no longer targets an open Slot")
	}
	cloned.GenerationExecutions = append(cloned.GenerationExecutions, cloneGenerationExecution(execution))
	slot.GenerationExecutionIDs = append(slot.GenerationExecutionIDs, execution.ID)
	if execution.Status == ExecutionStatusSucceeded {
		slot.Status = CandidateSlotAccepted
		slot.Candidate = &Candidate{
			ID: candidateID, GenerationExecutionID: execution.ID,
			NormalizedOutputFingerprint: execution.NormalizedOutputFingerprint,
			AcceptedAt:                  *execution.FinishedAt, Assertions: append([]AssertionReceipt(nil), assertions...),
		}
		cloned.version++
		if err := cloned.Validate(); err != nil {
			return err
		}
		*e = cloned
		return nil
	}

	causeCode := ""
	if execution.Status == ExecutionStatusResultUnknown {
		cloned.UnresolvedResultUnknownCount++
		causeCode = "result_unknown_requires_review"
	} else if len(slot.GenerationExecutionIDs) >= cloned.ExecutionPolicy.Generation.MaxExecutionsPerSlot {
		causeCode = "generation_budget_exhausted"
	} else if execution.Failure == nil || !allowsGenerationRecovery(cloned.ExecutionPolicy.Recovery, *execution.Failure, false) {
		causeCode = "generation_recovery_not_allowed"
	}
	if causeCode != "" {
		slot.Status = CandidateSlotBlocked
		if err := cloned.Transition(EvidenceStatusBlocked, causeCode, owner, []string{execution.ID}, *execution.FinishedAt); err != nil {
			return err
		}
	} else {
		cloned.version++
		if err := cloned.Validate(); err != nil {
			return err
		}
	}
	*e = cloned
	return nil
}

// CompleteSemanticExecution appends judge evidence to the same accepted
// Candidate. A failed judge execution can never replace generation evidence.
func (e *PromptEvaluationEvidenceV2) CompleteSemanticExecution(owner string, execution SemanticEvaluationExecution) error {
	owner = strings.TrimSpace(owner)
	if e == nil || e.Status != EvidenceStatusCollecting || e.execution == nil || e.execution.Kind != EvidenceExecutionSemantic ||
		e.execution.Phase != AttemptExecutionDispatching || owner == "" || owner != e.execution.Owner {
		return fmt.Errorf("dispatching AI explanation semantic checkpoint and matching owner are required")
	}
	if err := execution.Validate(); err != nil || execution.Status == ExecutionStatusPrepared || execution.Status == ExecutionStatusDispatching ||
		execution.ID != e.execution.ID || execution.CandidateID != e.execution.CandidateID ||
		execution.ExecutionOrdinal != e.execution.ExecutionOrdinal || execution.InvocationID != e.execution.InvocationID ||
		e.execution.DispatchStartedAt == nil || execution.StartedAt.Before(*e.execution.DispatchStartedAt) {
		return fmt.Errorf("terminal AI explanation semantic evidence does not match its checkpoint")
	}

	cloned := e.Clone()
	cloned.execution = nil
	slot := cloned.slot(e.execution.CaseID, e.execution.SlotOrdinal)
	if slot == nil || slot.Candidate == nil || slot.Candidate.ID != execution.CandidateID || slot.Candidate.ReviewReady {
		return fmt.Errorf("AI explanation semantic checkpoint no longer targets its accepted Candidate")
	}
	cloned.SemanticExecutions = append(cloned.SemanticExecutions, cloneSemanticExecution(execution))
	slot.Candidate.SemanticExecutionIDs = append(slot.Candidate.SemanticExecutionIDs, execution.ID)
	if execution.Status == ExecutionStatusSucceeded {
		for _, decision := range execution.Result.Decisions {
			matched := false
			for index := range slot.Candidate.Assertions {
				assertion := &slot.Candidate.Assertions[index]
				if assertion.Type != decision.Type || assertion.Scope != decision.Scope || assertion.Ordinal != decision.Ordinal ||
					assertion.Status != AssertionPendingSemantic {
					continue
				}
				assertion.Evaluator = "semantic-" + execution.Result.EvaluatorVersion
				assertion.Status = decision.Status
				assertion.Detail = decision.Detail
				matched = true
				break
			}
			if !matched {
				slot.Candidate.Assertions = append(slot.Candidate.Assertions, AssertionReceipt{
					Type: decision.Type, Scope: decision.Scope, Ordinal: decision.Ordinal,
					Evaluator: "semantic-" + execution.Result.EvaluatorVersion, Status: decision.Status, Detail: decision.Detail,
				})
			}
		}
		slot.Candidate.AcceptedSemanticExecutionID = execution.ID
		slot.Candidate.ReviewReady = true
		if err := cloned.Validate(); err != nil {
			return err
		}
		action, err := cloned.NextAction()
		if err != nil {
			return err
		}
		if action.Kind == EvidenceNextActionAwaitReview {
			if err := cloned.Transition(EvidenceStatusAwaitingReview, action.CauseCode, owner, nil, *execution.FinishedAt); err != nil {
				return err
			}
		} else {
			cloned.version++
		}
		*e = cloned
		return nil
	}

	causeCode := ""
	if execution.Status == ExecutionStatusResultUnknown {
		cloned.UnresolvedResultUnknownCount++
		causeCode = "result_unknown_requires_review"
	} else if len(slot.Candidate.SemanticExecutionIDs) >= cloned.ExecutionPolicy.Semantic.MaxExecutionsPerCandidate {
		causeCode = "semantic_budget_exhausted"
	} else if execution.Failure == nil || !allowsSemanticRecovery(cloned.ExecutionPolicy.Recovery, *execution.Failure, false) {
		causeCode = "semantic_recovery_not_allowed"
	}
	if causeCode != "" {
		if err := cloned.Transition(EvidenceStatusBlocked, causeCode, owner, []string{execution.ID}, *execution.FinishedAt); err != nil {
			return err
		}
	} else {
		cloned.version++
		if err := cloned.Validate(); err != nil {
			return err
		}
	}
	*e = cloned
	return nil
}

func (e *PromptEvaluationEvidenceV2) ResolveResultUnknown(value ResultUnknownResolution) error {
	if e == nil || e.Status != EvidenceStatusBlocked || e.UnresolvedResultUnknownCount < 1 {
		return fmt.Errorf("blocked AI explanation result-unknown evidence is required")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	for _, existing := range e.ResultUnknownResolutions {
		if existing.ExecutionID == value.ExecutionID {
			return fmt.Errorf("AI explanation result-unknown execution is already resolved")
		}
	}
	kind, ordinal, finishedAt, exists := e.resultUnknownExecution(value.ExecutionID)
	if !exists || value.ResolvedAt.Before(finishedAt) {
		return fmt.Errorf("AI explanation result-unknown resolution target is invalid")
	}
	if value.Decision == ResultUnknownAuthorizeReplacement {
		if kind == EvidenceExecutionGeneration && ordinal >= e.ExecutionPolicy.Generation.MaxExecutionsPerSlot ||
			kind == EvidenceExecutionSemantic && ordinal >= e.ExecutionPolicy.Semantic.MaxExecutionsPerCandidate {
			return fmt.Errorf("AI explanation result-unknown execution has no frozen recovery budget")
		}
	}
	cloned := e.Clone()
	cloned.ResultUnknownResolutions = append(cloned.ResultUnknownResolutions, value)
	cloned.UnresolvedResultUnknownCount--
	if value.Decision == ResultUnknownCancelRun {
		if err := cloned.Transition(EvidenceStatusCanceled, "result_unknown_run_canceled", value.Actor, []string{value.ExecutionID}, value.ResolvedAt); err != nil {
			return err
		}
	} else if cloned.UnresolvedResultUnknownCount == 0 {
		if err := cloned.Transition(EvidenceStatusCollecting, "manual_recovery_approved", value.Actor, []string{value.ExecutionID}, value.ResolvedAt); err != nil {
			return err
		}
	} else {
		cloned.version++
		if err := cloned.Validate(); err != nil {
			return err
		}
	}
	*e = cloned
	return nil
}

func (e *PromptEvaluationEvidenceV2) AddHumanReview(value CandidateHumanReview) error {
	if e == nil || e.Status != EvidenceStatusAwaitingReview || e.Audit.ClosedAt == nil || value.ReviewedAt.Before(*e.Audit.ClosedAt) {
		return fmt.Errorf("awaiting-review AI explanation evidence is required")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	cloned := e.Clone()
	cloned.HumanReviews = append(cloned.HumanReviews, value)
	cloned.version++
	if err := cloned.Validate(); err != nil {
		return err
	}
	*e = cloned
	return nil
}

func (e PromptEvaluationEvidenceV2) Version() int64 { return e.version }

func (e PromptEvaluationEvidenceV2) Execution() *EvidenceExecutionCheckpoint {
	return cloneEvidenceExecutionCheckpoint(e.execution)
}

// HasTerminalExecution proves that one exact durable address has already
// produced append-only terminal evidence. It is used to ACK redelivered events
// without invoking either Provider again.
func (e PromptEvaluationEvidenceV2) HasTerminalExecution(action EvidenceNextAction) bool {
	if action.Kind == EvidenceNextActionGeneration {
		for _, execution := range e.GenerationExecutions {
			if execution.CaseID == action.CaseID && execution.SlotOrdinal == action.SlotOrdinal &&
				execution.ExecutionOrdinal == action.ExecutionOrdinal {
				return true
			}
		}
		return false
	}
	if action.Kind == EvidenceNextActionSemantic {
		for _, execution := range e.SemanticExecutions {
			if execution.CandidateID == action.CandidateID && execution.ExecutionOrdinal == action.ExecutionOrdinal {
				return true
			}
		}
	}
	return false
}

// RestorePromptEvaluationEvidenceV2 restores the runtime-only CAS version and
// in-flight checkpoint around the immutable public evidence fields.
func RestorePromptEvaluationEvidenceV2(value PromptEvaluationEvidenceV2, version int64, execution *EvidenceExecutionCheckpoint) (*PromptEvaluationEvidenceV2, error) {
	restored := value.Clone()
	restored.version = version
	restored.execution = cloneEvidenceExecutionCheckpoint(execution)
	if err := restored.Validate(); err != nil {
		return nil, err
	}
	return &restored, nil
}

func (e PromptEvaluationEvidenceV2) LastModifiedAt() time.Time {
	latest := e.Audit.CreatedAt
	keepLatest := func(value time.Time) {
		if value.After(latest) {
			latest = value
		}
	}
	for _, transition := range e.StateTransitions {
		keepLatest(transition.TransitionedAt)
	}
	for _, preflight := range e.PreflightEvidence {
		if preflight.EvaluatedAt != nil {
			keepLatest(*preflight.EvaluatedAt)
		}
	}
	for _, execution := range e.GenerationExecutions {
		keepLatest(execution.StartedAt)
		if execution.FinishedAt != nil {
			keepLatest(*execution.FinishedAt)
		}
	}
	for _, execution := range e.SemanticExecutions {
		keepLatest(execution.StartedAt)
		if execution.FinishedAt != nil {
			keepLatest(*execution.FinishedAt)
		}
	}
	for _, review := range e.HumanReviews {
		keepLatest(review.ReviewedAt)
	}
	for _, resolution := range e.ResultUnknownResolutions {
		keepLatest(resolution.ResolvedAt)
	}
	if e.execution != nil {
		keepLatest(e.execution.ClaimedAt)
		if e.execution.DispatchStartedAt != nil {
			keepLatest(*e.execution.DispatchStartedAt)
		}
	}
	if e.GateResult != nil {
		keepLatest(e.GateResult.EvaluatedAt)
	}
	return latest
}

func (e PromptEvaluationEvidenceV2) validateExecutionCheckpoint() error {
	if e.execution == nil {
		return nil
	}
	if e.Status != EvidenceStatusCollecting || e.execution.ClaimedAt.Before(e.Audit.CreatedAt) {
		return fmt.Errorf("AI explanation evidence execution checkpoint requires a collecting Run")
	}
	if err := e.execution.Validate(); err != nil {
		return err
	}
	for _, execution := range e.GenerationExecutions {
		if execution.ID == e.execution.ID || execution.InvocationID == e.execution.InvocationID {
			return fmt.Errorf("AI explanation evidence generation checkpoint duplicates terminal evidence")
		}
	}
	for _, execution := range e.SemanticExecutions {
		if execution.ID == e.execution.ID || execution.InvocationID == e.execution.InvocationID {
			return fmt.Errorf("AI explanation evidence semantic checkpoint duplicates terminal evidence")
		}
	}
	withoutCheckpoint := e.Clone()
	withoutCheckpoint.execution = nil
	action, err := withoutCheckpoint.NextAction()
	if err != nil {
		return err
	}
	expectedKind := EvidenceExecutionGeneration
	if action.Kind == EvidenceNextActionSemantic {
		expectedKind = EvidenceExecutionSemantic
	} else if action.Kind != EvidenceNextActionGeneration {
		return fmt.Errorf("AI explanation evidence checkpoint does not match an executable next action")
	}
	if e.execution.Kind != expectedKind || e.execution.CaseID != action.CaseID || e.execution.SlotOrdinal != action.SlotOrdinal ||
		e.execution.CandidateID != action.CandidateID || e.execution.ExecutionOrdinal != action.ExecutionOrdinal {
		return fmt.Errorf("AI explanation evidence checkpoint is not the next frozen action")
	}
	return nil
}

func (e PromptEvaluationEvidenceV2) replacementAuthorized(executionID string) bool {
	for _, resolution := range e.ResultUnknownResolutions {
		if resolution.ExecutionID == executionID && resolution.Decision == ResultUnknownAuthorizeReplacement {
			return true
		}
	}
	return false
}

func (e PromptEvaluationEvidenceV2) resultUnknownExecution(executionID string) (EvidenceExecutionKind, int, time.Time, bool) {
	for _, execution := range e.GenerationExecutions {
		if execution.ID == executionID && execution.Status == ExecutionStatusResultUnknown && execution.FinishedAt != nil {
			return EvidenceExecutionGeneration, execution.ExecutionOrdinal, *execution.FinishedAt, true
		}
	}
	for _, execution := range e.SemanticExecutions {
		if execution.ID == executionID && execution.Status == ExecutionStatusResultUnknown && execution.FinishedAt != nil {
			return EvidenceExecutionSemantic, execution.ExecutionOrdinal, *execution.FinishedAt, true
		}
	}
	return "", 0, time.Time{}, false
}

func allowsGenerationRecovery(policy EvaluationRecoveryPolicy, failure ClassifiedFailure, manuallyAuthorized bool) bool {
	return manuallyAuthorized || failure.AllowsGenerationReplacement() ||
		failure.Disposition == FailureDispositionRetryGeneration && policy.AllowsAutomaticRetry(failure)
}

func allowsSemanticRecovery(policy EvaluationRecoveryPolicy, failure ClassifiedFailure, manuallyAuthorized bool) bool {
	return manuallyAuthorized || failure.AllowsSemanticRetry() && policy.AllowsAutomaticRetry(failure)
}

func generationAction(slot CandidateSlot, ordinal int, cause string) EvidenceNextAction {
	return EvidenceNextAction{Kind: EvidenceNextActionGeneration, CaseID: slot.CaseID, SlotOrdinal: slot.Ordinal, ExecutionOrdinal: ordinal, CauseCode: cause}
}

func semanticAction(slot CandidateSlot, ordinal int, cause string) EvidenceNextAction {
	return EvidenceNextAction{Kind: EvidenceNextActionSemantic, CaseID: slot.CaseID, SlotOrdinal: slot.Ordinal, CandidateID: slot.Candidate.ID, ExecutionOrdinal: ordinal, CauseCode: cause}
}

func blockedSlotAction(slot CandidateSlot, cause string) EvidenceNextAction {
	action := EvidenceNextAction{Kind: EvidenceNextActionBlock, CaseID: slot.CaseID, SlotOrdinal: slot.Ordinal, CauseCode: cause}
	if slot.Candidate != nil {
		action.CandidateID = slot.Candidate.ID
	}
	return action
}

func (e *PromptEvaluationEvidenceV2) slot(caseID string, ordinal int) *CandidateSlot {
	for index := range e.Slots {
		if e.Slots[index].CaseID == caseID && e.Slots[index].Ordinal == ordinal {
			return &e.Slots[index]
		}
	}
	return nil
}

func cloneGenerationExecution(value CandidateGenerationExecution) CandidateGenerationExecution {
	cloned := value
	cloned.FinishedAt = copyTimePtr(value.FinishedAt)
	cloned.RawOutput = append([]byte(nil), value.RawOutput...)
	cloned.NormalizedOutput = append([]byte(nil), value.NormalizedOutput...)
	if value.ProviderReceipt != nil {
		receipt := *value.ProviderReceipt
		cloned.ProviderReceipt = &receipt
	}
	if value.Failure != nil {
		failure := value.Failure.Clone()
		cloned.Failure = &failure
	}
	return cloned
}

func clonePreflightEvidence(value PreflightCaseEvidence) PreflightCaseEvidence {
	cloned := value
	cloned.EvaluatedAt = copyTimePtr(value.EvaluatedAt)
	cloned.Assertions = append([]AssertionReceipt(nil), value.Assertions...)
	return cloned
}

func cloneSemanticExecution(value SemanticEvaluationExecution) SemanticEvaluationExecution {
	cloned := value
	cloned.FinishedAt = copyTimePtr(value.FinishedAt)
	cloned.RawOutput = append([]byte(nil), value.RawOutput...)
	cloned.NormalizedOutput = append([]byte(nil), value.NormalizedOutput...)
	if value.ProviderReceipt != nil {
		receipt := *value.ProviderReceipt
		cloned.ProviderReceipt = &receipt
	}
	if value.Result != nil {
		result := *value.Result
		result.Decisions = append([]SemanticDecision(nil), value.Result.Decisions...)
		cloned.Result = &result
	}
	if value.Failure != nil {
		failure := value.Failure.Clone()
		cloned.Failure = &failure
	}
	return cloned
}

func cloneEvidenceExecutionCheckpoint(value *EvidenceExecutionCheckpoint) *EvidenceExecutionCheckpoint {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.DispatchStartedAt = copyTimePtr(value.DispatchStartedAt)
	return &cloned
}
