package aibridge

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// EvaluationUnknownExecution contains original call metadata; counts do not confirm billing.
type EvaluationUnknownExecution struct {
	ExecutionID          string `json:"execution_id"`
	InvocationID         string `json:"invocation_id"`
	Kind                 string `json:"kind"`
	CaseID               string `json:"case_id"`
	SlotOrdinal          int32  `json:"slot_ordinal"`
	CandidateID          string `json:"candidate_id"`
	ExecutionOrdinal     int32  `json:"execution_ordinal"`
	StartedAt            string `json:"started_at"`
	FinishedAt           string `json:"finished_at"`
	ProviderCallCount    int32  `json:"provider_call_count"`
	FailureStage         string `json:"failure_stage"`
	FailureCode          string `json:"failure_code"`
	TargetExecutionCount int32  `json:"target_execution_count"`
	TargetExecutionLimit int32  `json:"target_execution_limit"`
	StageExecutionCount  int32  `json:"stage_execution_count"`
	StageExecutionLimit  int32  `json:"stage_execution_limit"`
	ReplacementAllowed   bool   `json:"replacement_allowed"`
}
type EvaluationUnknownIndex struct {
	RunID                        string                       `json:"run_id"`
	Version                      int64                        `json:"version"`
	ReleaseFingerprint           string                       `json:"release_fingerprint"`
	Status                       string                       `json:"status"`
	UnresolvedResultUnknownCount int32                        `json:"unresolved_result_unknown_count"`
	CanResolve                   bool                         `json:"can_resolve"`
	Executions                   []EvaluationUnknownExecution `json:"executions"`
}

func (s *EvaluationAdministration) ListUnknowns(ctx context.Context, scope EvaluationScope, expectedVersion int64) (EvaluationUnknownIndex, error) {
	if err := s.authorizeCapability(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return EvaluationUnknownIndex{}, err
	}
	if expectedVersion < 1 {
		return EvaluationUnknownIndex{}, ErrInvalid
	}
	return s.Gateway.ListEvaluationUnknowns(ctx, scope, expectedVersion)
}
