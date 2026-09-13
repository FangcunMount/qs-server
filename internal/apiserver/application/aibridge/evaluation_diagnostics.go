package aibridge

import (
	"context"
	"encoding/json"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type ExecutionQuery struct {
	ExpectedVersion int64
	Cursor          string
	Limit           int
}

type ExecutionSummary struct {
	ExecutionID           string          `json:"execution_id"`
	InvocationID          string          `json:"invocation_id"`
	Kind                  string          `json:"kind"`
	CaseID                string          `json:"case_id"`
	SlotOrdinal           int32           `json:"slot_ordinal"`
	ExecutionOrdinal      int32           `json:"execution_ordinal"`
	Status                string          `json:"status"`
	RawOutputBytes        int64           `json:"raw_output_bytes"`
	NormalizedOutputBytes int64           `json:"normalized_output_bytes"`
	Evidence              json.RawMessage `json:"evidence" swaggertype:"object"`
}

type ExecutionPage struct {
	RunID      string             `json:"run_id"`
	Version    int64              `json:"version"`
	Executions []ExecutionSummary `json:"executions"`
	NextCursor string             `json:"next_cursor"`
}

type ExecutionOutput struct {
	RunID            string           `json:"run_id"`
	Version          int64            `json:"version"`
	Execution        ExecutionSummary `json:"execution"`
	RawOutput        []byte           `json:"raw_output" swaggertype:"string" format:"byte"`
	NormalizedOutput []byte           `json:"normalized_output" swaggertype:"string" format:"byte"`
	RawSHA256        string           `json:"raw_sha256"`
	NormalizedSHA256 string           `json:"normalized_sha256"`
}

// Optional read capability keeps unrelated evaluation commands independent.
type EvaluationDiagnosticsGateway interface {
	ListEvaluationExecutions(context.Context, EvaluationScope, ExecutionQuery) (ExecutionPage, error)
	GetEvaluationExecutionOutput(context.Context, EvaluationScope, int64, string) (ExecutionOutput, error)
}

func (s *EvaluationAdministration) diagnostics(ctx context.Context, scope EvaluationScope) (EvaluationDiagnosticsGateway, error) {
	if err := s.authorizeCapability(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return nil, err
	}
	gateway, ok := s.Gateway.(EvaluationDiagnosticsGateway)
	if !ok {
		return nil, ErrManagementUnavailable
	}
	return gateway, nil
}

func (s *EvaluationAdministration) ListExecutions(ctx context.Context, scope EvaluationScope, query ExecutionQuery) (ExecutionPage, error) {
	gateway, err := s.diagnostics(ctx, scope)
	if err != nil {
		return ExecutionPage{}, err
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.ExpectedVersion < 1 || query.Limit < 1 || query.Limit > 50 || (query.Cursor != "" && !frozenID.MatchString(query.Cursor)) {
		return ExecutionPage{}, ErrInvalid
	}
	return gateway.ListEvaluationExecutions(ctx, scope, query)
}

func (s *EvaluationAdministration) GetExecutionOutput(ctx context.Context, scope EvaluationScope, version int64, executionID string) (ExecutionOutput, error) {
	gateway, err := s.diagnostics(ctx, scope)
	if err != nil {
		return ExecutionOutput{}, err
	}
	if version < 1 || !frozenID.MatchString(executionID) {
		return ExecutionOutput{}, ErrInvalid
	}
	return gateway.GetEvaluationExecutionOutput(ctx, scope, version, executionID)
}
