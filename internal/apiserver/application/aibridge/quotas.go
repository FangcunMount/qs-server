package aibridge

import (
	"context"
	"encoding/json"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"strconv"
)

type QuotaGateway interface {
	ReadQuota(context.Context, DraftScope, string, string, int64) (json.RawMessage, error)
	WriteQuota(context.Context, DraftScope, string, json.RawMessage) (json.RawMessage, error)
}
type QuotaAdministration struct{ Gateway QuotaGateway }

func (s *QuotaAdministration) authorize(ctx context.Context, scope DraftScope, write bool) error {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	capability := authz.CapabilityAuditInterpretation
	if write {
		capability = authz.CapabilityOrgAdmin
	}
	if !ok || !authz.DecideCapability(snapshot, capability).Allowed {
		return ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return ErrManagementUnavailable
	}
	return nil
}
func (s *QuotaAdministration) Read(ctx context.Context, scope DraftScope, operation, id, cursor string) (json.RawMessage, error) {
	if err := s.authorize(ctx, scope, false); err != nil {
		return nil, err
	}
	var before int64
	switch operation {
	case "get":
	case "receipt":
		if !ValidPublicationID(id) {
			return nil, ErrInvalid
		}
	case "history":
		if cursor != "" {
			value, err := strconv.ParseInt(cursor, 10, 64)
			if err != nil || value < 0 {
				return nil, ErrInvalid
			}
			before = value
		}
	default:
		return nil, ErrInvalid
	}
	return s.Gateway.ReadQuota(ctx, scope, operation, id, before)
}
func (s *QuotaAdministration) Write(ctx context.Context, scope DraftScope, operation string, body json.RawMessage) (json.RawMessage, error) {
	if err := s.authorize(ctx, scope, true); err != nil {
		return nil, err
	}
	if len(body) > 15*1024 || !json.Valid(body) {
		return nil, ErrInvalid
	}
	var audit struct {
		CommandID        string `json:"command_id"`
		Reason           string `json:"reason"`
		ExpectedRevision *int64 `json:"expected_revision"`
	}
	if json.Unmarshal(body, &audit) != nil || !ValidPublicationID(audit.CommandID) || !(PromptDraftCommand{CommandID: audit.CommandID, Reason: audit.Reason}).Valid() || audit.ExpectedRevision == nil || *audit.ExpectedRevision < 0 {
		return nil, ErrInvalid
	}
	if operation != "update" && operation != "rollback" {
		return nil, ErrInvalid
	}
	return s.Gateway.WriteQuota(ctx, scope, operation, body)
}
