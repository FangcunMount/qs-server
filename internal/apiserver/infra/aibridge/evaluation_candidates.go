package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

var candidateIdentity = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._/-]{0,127}$`)

func (c *EvaluationClient) ListEvaluationCandidates(ctx context.Context, scope app.EvaluationScope) (app.EvaluationCandidateIndex, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.ListCandidates(ctx, query(scope))
	if err != nil {
		return app.EvaluationCandidateIndex{}, err
	}
	if response == nil || response.RunId != scope.RunID || response.Version < 1 || len(response.Candidates) > 35 {
		return app.EvaluationCandidateIndex{}, app.ErrConflict
	}
	result := app.EvaluationCandidateIndex{RunID: response.RunId, Version: response.Version, Candidates: make([]app.EvaluationCandidateSummary, 0, len(response.Candidates))}
	seen := make(map[string]bool)
	for _, item := range response.Candidates {
		if item == nil || !candidateIdentity.MatchString(item.CandidateId) || !candidateIdentity.MatchString(item.CaseId) || item.SlotOrdinal < 1 || item.SlotOrdinal > 5 || seen[item.CandidateId] {
			return app.EvaluationCandidateIndex{}, app.ErrConflict
		}
		seen[item.CandidateId] = true
		result.Candidates = append(result.Candidates, app.EvaluationCandidateSummary{CandidateID: item.CandidateId, CaseID: item.CaseId, SlotOrdinal: item.SlotOrdinal})
	}
	return result, nil
}

func (c *EvaluationClient) GetEvaluationCandidate(ctx context.Context, scope app.EvaluationScope, requested app.CandidateQuery) (app.EvaluationCandidateEvidence, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.GetCandidate(ctx, &pb.EvaluationCandidateQuery{Scope: query(scope), CandidateId: requested.CandidateID, ExpectedVersion: requested.ExpectedVersion})
	if err != nil {
		return app.EvaluationCandidateEvidence{}, err
	}
	if response == nil || response.RunId != scope.RunID || response.Version != requested.ExpectedVersion || response.CandidateId != requested.CandidateID ||
		len(response.NormalizedOutput)+len(response.SemanticOutput)+len(response.EvidenceJson) > 2*1024*1024 ||
		!utf8.Valid(response.NormalizedOutput) || !utf8.Valid(response.SemanticOutput) || !utf8.ValidString(response.EvidenceJson) ||
		!json.Valid(response.NormalizedOutput) || !json.Valid(response.SemanticOutput) {
		return app.EvaluationCandidateEvidence{}, app.ErrConflict
	}
	var identity struct {
		Candidate struct {
			ID          string `json:"id"`
			Fingerprint string `json:"normalized_output_fingerprint"`
		} `json:"candidate"`
		Semantic struct {
			Fingerprint string `json:"output_fingerprint"`
		} `json:"semantic"`
	}
	fingerprint := func(raw []byte) string { hash := sha256.Sum256(raw); return "sha256:" + hex.EncodeToString(hash[:]) }
	if json.Unmarshal([]byte(response.EvidenceJson), &identity) != nil || identity.Candidate.ID != requested.CandidateID ||
		identity.Candidate.Fingerprint != fingerprint(response.NormalizedOutput) || identity.Semantic.Fingerprint != fingerprint(response.SemanticOutput) {
		return app.EvaluationCandidateEvidence{}, app.ErrConflict
	}
	return app.EvaluationCandidateEvidence{RunID: response.RunId, Version: response.Version, CandidateID: response.CandidateId,
		NormalizedOutput: string(response.NormalizedOutput), SemanticOutput: string(response.SemanticOutput), Evidence: json.RawMessage(response.EvidenceJson)}, nil
}
