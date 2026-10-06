package aibridge

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type EvaluationReviewCorrection struct {
	CommandID                  string `json:"command_id"`
	ExpectedVersion            int64  `json:"expected_version"`
	CandidateID                string `json:"candidate_id"`
	Role                       string `json:"role"`
	PreviousReviewFingerprint  string `json:"previous_review_fingerprint"`
	CandidateOutputFingerprint string `json:"candidate_output_fingerprint"`
	Decision                   string `json:"decision"`
	Reason                     string `json:"reason"`
	Confirm                    bool   `json:"confirm"`
}

type EvaluationReviewCorrections interface {
	CorrectEvaluationReview(context.Context, EvaluationScope, EvaluationReviewCorrection) (EvaluationState, error)
}

func (s *EvaluationAdministration) CorrectReview(ctx context.Context, scope EvaluationScope, command EvaluationReviewCorrection) (EvaluationState, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	id, err := uuid.Parse(command.CommandID)
	command.Reason = strings.TrimSpace(command.Reason)
	if err != nil || id == uuid.Nil || id.String() != command.CommandID || command.ExpectedVersion < 1 || !command.Confirm ||
		!frozenID.MatchString(command.CandidateID) || !frozenFingerprint.MatchString(command.PreviousReviewFingerprint) ||
		!frozenFingerprint.MatchString(command.CandidateOutputFingerprint) || !reviewText(command.Reason, 1000) ||
		(command.Role != "assessment_semantics" && command.Role != "safety_product") ||
		(command.Decision != "approve" && command.Decision != "reject") {
		return EvaluationState{}, ErrInvalid
	}
	gateway, ok := s.Gateway.(EvaluationReviewCorrections)
	if !ok {
		return EvaluationState{}, ErrManagementUnavailable
	}
	// AI checks original-reviewer ownership and frozen evidence inside the CAS transaction.
	return gateway.CorrectEvaluationReview(ctx, scope, command)
}
