package aibridge

import (
	"context"
	"encoding/json"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type CandidateQuery struct {
	CandidateID     string
	ExpectedVersion int64
}
type EvaluationCandidateSummary struct {
	CandidateID string `json:"candidate_id"`
	CaseID      string `json:"case_id"`
	SlotOrdinal int32  `json:"slot_ordinal"`
}
type EvaluationCandidateIndex struct {
	RunID      string                       `json:"run_id"`
	Version    int64                        `json:"version"`
	Candidates []EvaluationCandidateSummary `json:"candidates"`
}
type EvaluationCandidateEvidence struct {
	RunID       string `json:"run_id"`
	Version     int64  `json:"version"`
	CandidateID string `json:"candidate_id"`
	// Strings retain the original normalized bytes after JSON decoding, including whitespace.
	NormalizedOutput string          `json:"normalized_output"`
	SemanticOutput   string          `json:"semantic_output"`
	Evidence         json.RawMessage `json:"evidence" swaggertype:"object"`
}

func (s *EvaluationAdministration) ListCandidates(ctx context.Context, scope EvaluationScope) (EvaluationCandidateIndex, error) {
	if err := s.authorizeCapability(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return EvaluationCandidateIndex{}, err
	}
	return s.Gateway.ListEvaluationCandidates(ctx, scope)
}
func (s *EvaluationAdministration) GetCandidate(ctx context.Context, scope EvaluationScope, query CandidateQuery) (EvaluationCandidateEvidence, error) {
	if err := s.authorizeCapability(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return EvaluationCandidateEvidence{}, err
	}
	if !frozenID.MatchString(query.CandidateID) || query.ExpectedVersion < 1 {
		return EvaluationCandidateEvidence{}, ErrInvalid
	}
	return s.Gateway.GetEvaluationCandidate(ctx, scope, query)
}
