package evaluation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

var ErrSemanticAdjudication = errors.New("invalid semantic adjudication")

// SemanticAdjudicationPolicyV1 identifies explicit dual-role adjudication.
// It never replaces frozen Provider evidence.
const SemanticAdjudicationPolicyV1 = "semantic-contradiction-dual-review/v1"

// SemanticContradictionReview is signed by the containing human review's
// authenticated reviewer, role and timestamp. Presence means confirmation of
// a contradictory failed verdict, not a generic quality waiver.
type SemanticContradictionReview struct {
	PolicyVersion     string                    `json:"policy_version"`
	ExecutionID       string                    `json:"execution_id"`
	OutputFingerprint aiexplanation.Fingerprint `json:"output_fingerprint"`
	AssertionOrdinal  int                       `json:"assertion_ordinal"`
	OriginalDetail    string                    `json:"original_detail"`
	CandidateExcerpt  string                    `json:"candidate_excerpt"`
	Reason            string                    `json:"reason"`
}

type SemanticAdjudicationRecord struct {
	PolicyVersion     string                    `json:"policy_version"`
	CandidateID       string                    `json:"candidate_id"`
	ExecutionID       string                    `json:"execution_id"`
	OutputFingerprint aiexplanation.Fingerprint `json:"output_fingerprint"`
	AssertionType     string                    `json:"assertion_type"`
	AssertionOrdinal  int                       `json:"assertion_ordinal"`
	OriginalStatus    AssertionStatus           `json:"original_status"`
	EffectiveStatus   AssertionStatus           `json:"effective_status"`
	Reviewers         []string                  `json:"reviewers"`
}

func (e PromptEvaluationEvidenceV2) validateSemanticReviews() error {
	for _, review := range e.HumanReviews {
		r := review.SemanticReview
		if r == nil {
			continue
		}
		if e.GatePolicy.Version != "v2" || r.PolicyVersion != SemanticAdjudicationPolicyV1 || review.Decision != ReviewDecisionApprove ||
			r.AssertionOrdinal < 1 || !validateEvidenceText(r.Reason, 1000) || !validateEvidenceText(r.CandidateExcerpt, 1000) || !validateEvidenceText(r.OriginalDetail, 2000) {
			return fmt.Errorf("%w: semantic contradiction review policy, approval and evidence are required", ErrSemanticAdjudication)
		}
		var candidate *Candidate
		for _, slot := range e.Slots {
			if slot.Candidate != nil && slot.Candidate.ID == review.CandidateID {
				candidate = slot.Candidate
				break
			}
		}
		if candidate == nil || r.ExecutionID != candidate.AcceptedSemanticExecutionID {
			return fmt.Errorf("%w: semantic review must bind the accepted execution", ErrSemanticAdjudication)
		}
		var execution *SemanticEvaluationExecution
		for i := range e.SemanticExecutions {
			if e.SemanticExecutions[i].ID == r.ExecutionID {
				execution = &e.SemanticExecutions[i]
				break
			}
		}
		if execution == nil || execution.Result == nil || r.OutputFingerprint != execution.Result.OutputFingerprint || execution.FinishedAt == nil || review.ReviewedAt.Before(*execution.FinishedAt) {
			return fmt.Errorf("%w: semantic review output fingerprint or timestamp mismatch", ErrSemanticAdjudication)
		}
		matched := false
		for _, a := range candidate.Assertions {
			if a.Type == "forbidden_claims_absent" && a.Scope == AssertionScopeDefault && a.Ordinal == r.AssertionOrdinal && a.Status == AssertionFailed && a.Detail == r.OriginalDetail && a.Evaluator == "semantic-"+execution.Result.EvaluatorVersion {
				matched = true
			}
		}
		sourceMatched := false
		for _, a := range execution.Result.Decisions {
			if a.Type == "forbidden_claims_absent" && a.Scope == AssertionScopeDefault && a.Ordinal == r.AssertionOrdinal && a.Status == AssertionFailed && a.Detail == r.OriginalDetail {
				sourceMatched = true
			}
		}
		if !matched || !sourceMatched {
			return fmt.Errorf("%w: only an original failed semantic forbidden-claims assertion can be reviewed", ErrSemanticAdjudication)
		}
		excerptMatched := false
		for _, g := range e.GenerationExecutions {
			if g.ID == candidate.GenerationExecutionID && strings.Contains(string(g.NormalizedOutput), r.CandidateExcerpt) {
				excerptMatched = true
			}
		}
		if !excerptMatched {
			return fmt.Errorf("%w: semantic review excerpt must occur in the frozen candidate output", ErrSemanticAdjudication)
		}
	}
	return nil
}

func (e PromptEvaluationEvidenceV2) effectiveCandidateAssertions(candidate Candidate) ([]AssertionReceipt, *SemanticAdjudicationRecord) {
	var assessment, safety *CandidateHumanReview
	for i := range e.HumanReviews {
		r := &e.HumanReviews[i]
		if r.CandidateID != candidate.ID || r.Decision != ReviewDecisionApprove || r.SemanticReview == nil {
			continue
		}
		switch r.Role {
		case ReviewRoleAssessmentSemantics:
			assessment = r
		case ReviewRoleSafetyProduct:
			safety = r
		}
	}
	if assessment == nil || safety == nil || assessment.Reviewer == safety.Reviewer {
		return candidate.Assertions, nil
	}
	a, b := assessment.SemanticReview, safety.SemanticReview
	if a.PolicyVersion != b.PolicyVersion || a.ExecutionID != b.ExecutionID || a.OutputFingerprint != b.OutputFingerprint || a.AssertionOrdinal != b.AssertionOrdinal || a.OriginalDetail != b.OriginalDetail {
		return candidate.Assertions, nil
	}
	effective := append([]AssertionReceipt(nil), candidate.Assertions...)
	for i := range effective {
		r := &effective[i]
		if r.Type == "forbidden_claims_absent" && r.Scope == AssertionScopeDefault && r.Ordinal == a.AssertionOrdinal && r.Status == AssertionFailed && r.Detail == a.OriginalDetail && strings.HasPrefix(r.Evaluator, "semantic-") {
			r.Status = AssertionPassed
		}
	}
	return effective, &SemanticAdjudicationRecord{PolicyVersion: a.PolicyVersion, CandidateID: candidate.ID, ExecutionID: a.ExecutionID, OutputFingerprint: a.OutputFingerprint, AssertionType: "forbidden_claims_absent", AssertionOrdinal: a.AssertionOrdinal, OriginalStatus: AssertionFailed, EffectiveStatus: AssertionPassed, Reviewers: []string{assessment.Reviewer, safety.Reviewer}}
}
