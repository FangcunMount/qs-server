package aibridge

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (c *EvaluationClient) PrepareEvaluation(ctx context.Context, scope app.DraftScope, query app.EvaluationPlanQuery) (app.EvaluationPlan, error) {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 || !query.Valid() {
		return app.EvaluationPlan{}, app.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request := &pb.EvaluationPlanQuery{Scope: draftScope(scope), Suite: frozenRef(query.Suite), GenerationRoute: frozenRef(query.GenerationRoute), SemanticRoute: frozenRef(query.SemanticRoute)}
	response, err := c.RPC.Prepare(ctx, request, grpc.MaxCallRecvMsgSize(32*1024))
	if err != nil {
		return app.EvaluationPlan{}, err
	}
	return evaluationPlan(response, query)
}

func evaluationPlan(response *pb.EvaluationPlan, query app.EvaluationPlanQuery) (app.EvaluationPlan, error) {
	if response == nil || response.SchemaVersion != "qs-ai-evaluation-plan/v1" || proto.Size(response) > 32*1024 || !utf8.ValidString(response.PlanJson) || !json.Valid([]byte(response.PlanJson)) {
		return app.EvaluationPlan{}, app.ErrConflict
	}
	var result app.EvaluationPlan
	decoder := json.NewDecoder(bytes.NewBufferString(response.PlanJson))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || !result.Release.Valid() || result.Release.Suite != query.Suite || result.Release.GenerationRoute != query.GenerationRoute || result.Release.SemanticRoute != query.SemanticRoute {
		return app.EvaluationPlan{}, app.ErrConflict
	}
	raw, _ := json.Marshal(result.Release)
	var refs map[string]app.FrozenEvaluationRef
	if json.Unmarshal(raw, &refs) != nil || len(refs) != 11 || result.ReleaseFingerprint != releaseDigest(refs) || !planPolicy(result.ExecutionPolicyJSON, result.Release.ExecutionPolicy) || !planPolicy(result.GatePolicyJSON, result.Release.GatePolicy) {
		return app.EvaluationPlan{}, app.ErrConflict
	}
	// Check response/document consistency only; AI owns the execution and approval rules.
	var execution struct {
		Slots struct {
			Cases      int64 `json:"required_generation_cases"`
			Candidates int64 `json:"required_candidates_per_case"`
			Preflight  int64 `json:"required_preflight_cases"`
		} `json:"slot_policy"`
		Generation struct {
			Maximum int64 `json:"max_executions_per_run"`
		} `json:"generation_budget"`
		Semantic struct {
			Maximum int64 `json:"max_executions_per_run"`
		} `json:"semantic_budget"`
	}
	if json.Unmarshal([]byte(result.ExecutionPolicyJSON), &execution) != nil {
		return app.EvaluationPlan{}, app.ErrConflict
	}
	for _, count := range []int64{result.GenerationCaseCount, result.CandidatesPerCase, result.CandidateCount, result.PreflightCaseCount, result.MaxGenerationInvocations, result.MaxSemanticInvocations} {
		if count <= 0 || count > 9007199254740991 {
			return app.EvaluationPlan{}, app.ErrConflict
		}
	}
	if result.CandidateCount/result.GenerationCaseCount != result.CandidatesPerCase || result.CandidateCount%result.GenerationCaseCount != 0 ||
		result.GenerationCaseCount != execution.Slots.Cases || result.CandidatesPerCase != execution.Slots.Candidates || result.PreflightCaseCount != execution.Slots.Preflight || result.MaxGenerationInvocations != execution.Generation.Maximum || result.MaxSemanticInvocations != execution.Semantic.Maximum {
		return app.EvaluationPlan{}, app.ErrConflict
	}
	return result, nil
}

func planPolicy(raw string, reference app.FrozenEvaluationRef) bool {
	var document struct {
		ID      string `json:"policy_id"`
		Version string `json:"version"`
	}
	return utf8.ValidString(raw) && json.Unmarshal([]byte(raw), &document) == nil && document.ID == reference.ID && document.Version == reference.Version && digest([]byte(raw)) == reference.Fingerprint
}
