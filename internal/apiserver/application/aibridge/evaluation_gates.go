package aibridge

import (
	"context"
	"encoding/json"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type EvaluationGatePreview struct {
	RunID              string          `json:"run_id"`
	Version            int64           `json:"version"`
	ReleaseFingerprint string          `json:"release_fingerprint"`
	GateResult         json.RawMessage `json:"gate_result" swaggertype:"object"`
}

func (s *EvaluationAdministration) PreviewGates(ctx context.Context, scope EvaluationScope, expectedVersion int64) (EvaluationGatePreview, error) {
	if err := s.authorizeCapability(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return EvaluationGatePreview{}, err
	}
	if expectedVersion < 1 {
		return EvaluationGatePreview{}, ErrInvalid
	}
	return s.Gateway.PreviewEvaluationGates(ctx, scope, expectedVersion)
}
