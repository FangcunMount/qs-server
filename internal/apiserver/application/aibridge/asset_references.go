package aibridge

import (
	"context"
	"encoding/json"
)

type PolicyReferencesQuery struct {
	Kind      string              `json:"kind"`
	Reference FrozenEvaluationRef `json:"reference"`
	UsageKind string              `json:"usage_kind"`
	Limit     int                 `json:"limit"`
	Cursor    string              `json:"cursor"`
}

func (q PolicyReferencesQuery) Valid() bool {
	return (q.Kind == "execution_policy" || q.Kind == "gate_policy") && q.Reference.Valid() && (q.UsageKind == "suite" || q.UsageKind == "evaluation" || q.UsageKind == "publication") && q.Limit >= 1 && q.Limit <= 50 && len(q.Cursor) <= 4096
}

type PolicyReferencesGateway interface {
	PolicyReferences(context.Context, DraftScope, PolicyReferencesQuery) (json.RawMessage, error)
}

func (s *AssetCatalogAdministration) References(ctx context.Context, scope DraftScope, q PolicyReferencesQuery) (json.RawMessage, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return nil, err
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if !q.Valid() {
		return nil, ErrInvalid
	}
	gateway, ok := s.Gateway.(PolicyReferencesGateway)
	if !ok {
		return nil, ErrManagementUnavailable
	}
	return gateway.PolicyReferences(ctx, scope, q)
}
