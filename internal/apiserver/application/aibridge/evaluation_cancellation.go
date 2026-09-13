package aibridge

import (
	"context"
	"math"
	"strings"
)

type EvaluationCancel struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
	Confirm         bool   `json:"confirm"`
	Discard         *bool  `json:"discard"`
}

type EvaluationCancellationReceipt struct {
	SchemaVersion      string `json:"schema_version"`
	RunID              string `json:"run_id"`
	SourceVersion      int64  `json:"source_version"`
	Version            int64  `json:"version"`
	SourceStatus       string `json:"source_status"`
	Status             string `json:"status"`
	ReleaseFingerprint string `json:"release_fingerprint"`
	Actor              string `json:"actor"`
	Reason             string `json:"reason"`
	Discard            *bool  `json:"discard"`
	CanceledAt         string `json:"canceled_at"`
	ExecutionID        string `json:"execution_id"`
	InvocationID       string `json:"invocation_id"`
}

func (s *EvaluationAdministration) Cancel(ctx context.Context, scope EvaluationScope, command EvaluationCancel) (EvaluationState, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ExpectedVersion < 1 || command.ExpectedVersion == math.MaxInt64 || !command.Confirm || command.Discard == nil || command.Reason == "" || len(command.Reason) > 1000 || strings.ContainsAny(command.Reason, "<>") {
		return EvaluationState{}, ErrInvalid
	}
	return s.Gateway.CancelEvaluation(ctx, scope, command)
}
