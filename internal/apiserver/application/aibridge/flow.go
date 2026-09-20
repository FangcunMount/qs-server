package aibridge

import (
	"context"
	"encoding/json"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type FlowGateway interface {
	ReadFlow(context.Context, DraftScope, string, string) (json.RawMessage, error)
}
type FlowAdministration struct{ Gateway FlowGateway }

func (s *FlowAdministration) Read(ctx context.Context, scope DraftScope, kind, id string) (json.RawMessage, error) {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 || !ValidPublicationID(id) || (kind != "solution" && kind != "publication") {
		return nil, ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityAuditInterpretation).Allowed {
		return nil, ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return nil, ErrManagementUnavailable
	}
	return s.Gateway.ReadFlow(ctx, scope, kind, id)
}
