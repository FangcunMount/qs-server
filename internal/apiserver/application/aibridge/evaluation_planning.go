package aibridge

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type EvaluationPlanQuery struct {
	Suite           FrozenEvaluationRef `json:"suite"`
	GenerationRoute FrozenEvaluationRef `json:"generation_route"`
	SemanticRoute   FrozenEvaluationRef `json:"semantic_route"`
}

func (q EvaluationPlanQuery) Valid() bool {
	return q.Suite.Valid() && q.GenerationRoute.Valid() && q.SemanticRoute.Valid()
}

// This is a read-only projection of AI-owned assets and policy limits, not permission to execute.
type EvaluationPlan struct {
	Release                  EvaluationRelease `json:"release"`
	ReleaseFingerprint       string            `json:"release_fingerprint"`
	GenerationCaseCount      int64             `json:"generation_case_count"`
	CandidatesPerCase        int64             `json:"candidates_per_case"`
	CandidateCount           int64             `json:"candidate_count"`
	PreflightCaseCount       int64             `json:"preflight_case_count"`
	MaxGenerationInvocations int64             `json:"max_generation_invocations"`
	MaxSemanticInvocations   int64             `json:"max_semantic_invocations"`
	ExecutionPolicyJSON      string            `json:"execution_policy_json"`
	GatePolicyJSON           string            `json:"gate_policy_json"`
}

func (s *EvaluationAdministration) Prepare(ctx context.Context, scope DraftScope, query EvaluationPlanQuery) (EvaluationPlan, error) {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 || !query.Valid() {
		return EvaluationPlan{}, ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityAuditInterpretation).Allowed {
		return EvaluationPlan{}, ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return EvaluationPlan{}, ErrManagementUnavailable
	}
	return s.Gateway.PrepareEvaluation(ctx, scope, query)
}
