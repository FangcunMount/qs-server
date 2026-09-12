package aibridge

import (
	"context"
	"strings"
)

type SemanticContradictionReview struct {
	PolicyVersion     string `json:"policy_version"`
	ExecutionID       string `json:"execution_id"`
	OutputFingerprint string `json:"output_fingerprint"`
	AssertionOrdinal  int32  `json:"assertion_ordinal"`
	OriginalDetail    string `json:"original_detail"`
	CandidateExcerpt  string `json:"candidate_excerpt"`
	Reason            string `json:"reason"`
}

type CandidateReviewItem struct {
	CandidateID    string                       `json:"candidate_id"`
	Decision       string                       `json:"decision"`
	Reason         string                       `json:"reason"`
	SemanticReview *SemanticContradictionReview `json:"semantic_review,omitempty"`
}

type EvaluationReview struct {
	ExpectedVersion int64                 `json:"expected_version"`
	Role            string                `json:"role"`
	Reviews         []CandidateReviewItem `json:"reviews"`
}

func reviewText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(value) <= limit && !strings.ContainsAny(value, "<>")
}

func (s *EvaluationAdministration) Review(ctx context.Context, scope EvaluationScope, command EvaluationReview) (EvaluationState, error) {
	// Both existing QS review roles require current OrgAdmin authorization.
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	if command.ExpectedVersion < 1 || len(command.Reviews) < 1 || len(command.Reviews) > 35 ||
		(command.Role != "assessment_semantics" && command.Role != "safety_product") {
		return EvaluationState{}, ErrInvalid
	}
	command.Reviews = append([]CandidateReviewItem(nil), command.Reviews...)
	seen := make(map[string]bool, len(command.Reviews))
	for i := range command.Reviews {
		item := &command.Reviews[i]
		item.CandidateID, item.Reason = strings.TrimSpace(item.CandidateID), strings.TrimSpace(item.Reason)
		if !frozenID.MatchString(item.CandidateID) || seen[item.CandidateID] || !reviewText(item.Reason, 1000) ||
			(item.Decision != "approve" && item.Decision != "reject") {
			return EvaluationState{}, ErrInvalid
		}
		seen[item.CandidateID] = true
		if item.SemanticReview != nil {
			value := *item.SemanticReview
			value.Reason = strings.TrimSpace(value.Reason)
			if item.Decision != "approve" || value.PolicyVersion != "semantic-contradiction-dual-review/v1" ||
				!frozenID.MatchString(value.ExecutionID) || !frozenFingerprint.MatchString(value.OutputFingerprint) || value.AssertionOrdinal < 1 ||
				!reviewText(value.OriginalDetail, 2000) || !reviewText(value.CandidateExcerpt, 1000) || !reviewText(value.Reason, 1000) {
				return EvaluationState{}, ErrInvalid
			}
			item.SemanticReview = &value
		}
	}
	return s.Gateway.ReviewEvaluation(ctx, scope, command)
}
