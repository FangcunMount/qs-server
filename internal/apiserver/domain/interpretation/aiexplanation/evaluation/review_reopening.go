package evaluation

import (
	"fmt"
	"reflect"
	"slices"
	"time"
)

// ReviewReopening preserves the complete previous finalization before a new
// review round. Provider evidence and release identity are shared and immutable.
type ReviewReopening struct {
	Reviews         []CandidateHumanReview `json:"reviews"`
	Gate            EvidenceGateResult     `json:"gate"`
	FinalizedAt     time.Time              `json:"finalized_at"`
	TransitionCount int                    `json:"transition_count"`
	CandidateIDs    []string               `json:"candidate_ids"`
	Actor           string                 `json:"actor"`
	Reason          string                 `json:"reason"`
	ReopenedAt      time.Time              `json:"reopened_at"`
}

// ReopenReviewCandidateIDs deliberately excludes infrastructure, score, case,
// deterministic and human rejection failures. Reopening never grants approval.
func (e PromptEvaluationEvidenceV2) ReopenReviewCandidateIDs() []string {
	if e.Status != EvidenceStatusRejected || e.GateResult == nil || e.GatePolicy.Version != "v2" || len(e.ReviewReopenings) >= 3 {
		return nil
	}
	for _, gate := range []string{"G1", "G2", "G3", "G5"} {
		if !e.GateResult.GatePasses[gate] {
			return nil
		}
	}
	if e.GateResult.GatePasses["G4"] || len(e.GateResult.Reasons) == 0 {
		return nil
	}
	for _, reason := range e.GateResult.Reasons {
		if reason.Gate != "G4" || reason.Code != "candidate_hard_assertion_failed" {
			return nil
		}
	}
	var ids []string
	for _, slot := range e.Slots {
		if slot.Candidate == nil {
			continue
		}
		assertions, _ := e.effectiveCandidateAssertions(*slot.Candidate)
		failed := false
		for _, a := range assertions {
			if !a.Hard || a.Status == AssertionPassed {
				continue
			}
			if a.Status != AssertionFailed || a.Type != "forbidden_claims_absent" || a.Scope != AssertionScopeDefault {
				return nil
			}
			found := false
			for _, j := range e.SemanticExecutions {
				if j.ID == slot.Candidate.AcceptedSemanticExecutionID && j.Result != nil && a.Evaluator == "semantic-"+j.Result.EvaluatorVersion {
					found = true
				}
			}
			if !found {
				return nil
			}
			failed = true
		}
		if failed {
			ids = append(ids, slot.Candidate.ID)
		}
	}
	return ids
}

func (e *PromptEvaluationEvidenceV2) ReopenReview(actor, reason string, at time.Time) error {
	if e == nil {
		return fmt.Errorf("%w: missing evidence", ErrSemanticAdjudication)
	}
	if err := e.Validate(); err != nil {
		return err
	}
	ids := e.ReopenReviewCandidateIDs()
	if len(ids) == 0 || !evidenceEntityIDPattern.MatchString(actor) || !validateEvidenceText(reason, 1000) || at.IsZero() || e.Audit.FinalizedAt == nil || at.Before(*e.Audit.FinalizedAt) {
		return fmt.Errorf("%w: run is not eligible for audited review reopening", ErrSemanticAdjudication)
	}
	next := e.Clone()
	history := ReviewReopening{Reviews: next.HumanReviews, Gate: *next.GateResult, FinalizedAt: *next.Audit.FinalizedAt, TransitionCount: len(next.StateTransitions), CandidateIDs: ids, Actor: actor, Reason: reason, ReopenedAt: at}
	next.ReviewReopenings = append(next.ReviewReopenings, history)
	next.HumanReviews = nil
	for _, r := range e.HumanReviews {
		if !slices.Contains(ids, r.CandidateID) {
			next.HumanReviews = append(next.HumanReviews, r)
		}
	}
	next.GateResult = nil
	next.Audit.FinalizedAt = nil
	from := EvidenceStatusRejected
	next.Status = EvidenceStatusAwaitingReview
	next.StateTransitions = append(next.StateTransitions, EvidenceStateTransition{From: &from, To: next.Status, CauseCode: "semantic_review_reopened", Actor: actor, Reason: reason, TransitionedAt: at})
	next.version++
	next = next.Clone()
	if err := next.Validate(); err != nil {
		return err
	}
	*e = next
	return nil
}

func (e PromptEvaluationEvidenceV2) validateReviewReopenings() error {
	if len(e.ReviewReopenings) > 3 {
		return fmt.Errorf("%w: too many review rounds", ErrSemanticAdjudication)
	}
	count := 0
	for _, tr := range e.StateTransitions {
		if tr.From != nil && *tr.From == EvidenceStatusRejected {
			count++
		}
	}
	if count != len(e.ReviewReopenings) {
		return fmt.Errorf("%w: reopening audit required", ErrSemanticAdjudication)
	}
	for i, h := range e.ReviewReopenings {
		if h.TransitionCount < 1 || h.TransitionCount >= len(e.StateTransitions) {
			return fmt.Errorf("%w: invalid history boundary", ErrSemanticAdjudication)
		}
		tr := e.StateTransitions[h.TransitionCount]
		if tr.From == nil || *tr.From != EvidenceStatusRejected || tr.To != EvidenceStatusAwaitingReview || tr.CauseCode != "semantic_review_reopened" || tr.Actor != h.Actor || tr.Reason != h.Reason || !validateEvidenceText(h.Reason, 1000) || !tr.TransitionedAt.Equal(h.ReopenedAt) || h.ReopenedAt.Before(h.FinalizedAt) {
			return fmt.Errorf("%w: reopening audit mismatch", ErrSemanticAdjudication)
		}
		old := e.Clone()
		old.Status = EvidenceStatusRejected
		old.StateTransitions = old.StateTransitions[:h.TransitionCount]
		old.ReviewReopenings = old.ReviewReopenings[:i]
		old.HumanReviews = h.Reviews
		old.GateResult = &h.Gate
		old.Audit.FinalizedAt = copyTime(h.FinalizedAt)
		old.Audit.CanceledAt = nil
		if !old.StateTransitions[len(old.StateTransitions)-1].TransitionedAt.Equal(h.FinalizedAt) {
			return fmt.Errorf("%w: historical finalization time mismatch", ErrSemanticAdjudication)
		}
		if err := old.Validate(); err != nil {
			return fmt.Errorf("historical review: %w", err)
		}
		if !reflect.DeepEqual(old.ReopenReviewCandidateIDs(), h.CandidateIDs) {
			return fmt.Errorf("%w: reopening candidates mismatch", ErrSemanticAdjudication)
		}
	}
	if len(e.ReviewReopenings) > 0 {
		h := e.ReviewReopenings[len(e.ReviewReopenings)-1]
		for _, r := range e.HumanReviews {
			if slices.Contains(h.CandidateIDs, r.CandidateID) && r.ReviewedAt.Before(h.ReopenedAt) {
				return fmt.Errorf("%w: new signature predates reopening", ErrSemanticAdjudication)
			}
		}
		for _, r := range h.Reviews {
			if !slices.Contains(h.CandidateIDs, r.CandidateID) && !slices.ContainsFunc(e.HumanReviews, func(v CandidateHumanReview) bool { return reflect.DeepEqual(r, v) }) {
				return fmt.Errorf("%w: unaffected review changed", ErrSemanticAdjudication)
			}
		}
	}
	return nil
}

// FinalizeChecked binds the user's explicit expected outcome to the CAS version.
func (e *PromptEvaluationEvidenceV2) FinalizeChecked(actor, reason string, version int64, passed bool, at time.Time) error {
	if e == nil || e.Version() != version {
		return fmt.Errorf("%w: evidence changed; refresh preview", ErrConflict)
	}
	if !validateEvidenceText(reason, 1000) {
		return fmt.Errorf("%w: finalization reason required", ErrSemanticAdjudication)
	}
	gate, err := e.EvaluateGate(at)
	if err != nil {
		return err
	}
	if gate.Passed != passed {
		return fmt.Errorf("%w: gate outcome changed; refresh preview", ErrConflict)
	}
	next := e.Clone()
	if err := next.Finalize(actor, "human_review_finalized", at); err != nil {
		return err
	}
	next.StateTransitions[len(next.StateTransitions)-1].Reason = reason
	if err := next.Validate(); err != nil {
		return err
	}
	*e = next
	return nil
}
