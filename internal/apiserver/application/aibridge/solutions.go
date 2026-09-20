package aibridge

import (
	"context"
	"encoding/json"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// Solution content belongs to qs-ai; QS supplies trusted scope and enforces capabilities.
type SolutionGateway interface {
	ReadSolution(context.Context, DraftScope, string, string, string) (json.RawMessage, error)
	WriteSolution(context.Context, DraftScope, string, string, json.RawMessage) (json.RawMessage, error)
}
type SolutionAdministration struct{ Gateway SolutionGateway }

func (s *SolutionAdministration) authorize(ctx context.Context, scope DraftScope, write bool) error {
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
func (s *SolutionAdministration) Read(ctx context.Context, scope DraftScope, operation, id, cursor string) (json.RawMessage, error) {
	if err := s.authorize(ctx, scope, false); err != nil {
		return nil, err
	}
	switch operation {
	case "get", "receipt":
		if !ValidPublicationID(id) {
			return nil, ErrInvalid
		}
	case "list":
		if cursor != "" && !ValidPublicationID(cursor) {
			return nil, ErrInvalid
		}
	case "models":
	default:
		return nil, ErrInvalid
	}
	return s.Gateway.ReadSolution(ctx, scope, operation, id, cursor)
}
func (s *SolutionAdministration) Write(ctx context.Context, scope DraftScope, operation, id string, body json.RawMessage) (json.RawMessage, error) {
	if err := s.authorize(ctx, scope, true); err != nil {
		return nil, err
	}
	if !ValidPublicationID(id) || len(body) > 256*1024 || !json.Valid(body) {
		return nil, ErrInvalid
	}
	var audit struct {
		CommandID        string `json:"command_id"`
		Reason           string `json:"reason"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if json.Unmarshal(body, &audit) != nil || !ValidPublicationID(audit.CommandID) || !(PromptDraftCommand{CommandID: audit.CommandID, Reason: audit.Reason}).Valid() {
		return nil, ErrInvalid
	}
	switch operation {
	case "create":
	case "save", "prepare":
		if audit.ExpectedRevision <= 0 {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	return s.Gateway.WriteSolution(ctx, scope, operation, id, body)
}
