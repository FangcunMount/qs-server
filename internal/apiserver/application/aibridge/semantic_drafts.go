package aibridge

import (
	"context"
	"encoding/json"
	"strconv"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type SemanticDraftGateway interface {
	ReadSemanticDraft(context.Context, DraftScope, string, string, int64) (json.RawMessage, error)
	WriteSemanticDraft(context.Context, DraftScope, string, json.RawMessage) (json.RawMessage, error)
}
type SemanticDraftAdministration struct{ Gateway SemanticDraftGateway }

func authorizeConfiguration(ctx context.Context, scope DraftScope, write bool) error {
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
	return nil
}
func (s *SemanticDraftAdministration) Read(ctx context.Context, scope DraftScope, operation, id, revision string) (json.RawMessage, error) {
	if err := authorizeConfiguration(ctx, scope, false); err != nil {
		return nil, err
	}
	if s == nil || s.Gateway == nil {
		return nil, ErrManagementUnavailable
	}
	if !ValidPublicationID(id) {
		return nil, ErrInvalid
	}
	var version int64
	if revision != "" {
		var err error
		version, err = strconv.ParseInt(revision, 10, 64)
		if err != nil || version < 0 {
			return nil, ErrInvalid
		}
	}
	switch operation {
	case "get", "receipt":
	case "validate":
		if version <= 0 {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	return s.Gateway.ReadSemanticDraft(ctx, scope, operation, id, version)
}
func (s *SemanticDraftAdministration) Write(ctx context.Context, scope DraftScope, operation, id string, body json.RawMessage) (json.RawMessage, error) {
	if err := authorizeConfiguration(ctx, scope, true); err != nil {
		return nil, err
	}
	if s == nil || s.Gateway == nil {
		return nil, ErrManagementUnavailable
	}
	if !ValidPublicationID(id) || len(body) > 240*1024 || !json.Valid(body) {
		return nil, ErrInvalid
	}
	var command struct {
		DraftID          string `json:"draft_id"`
		CommandID        string `json:"command_id"`
		Reason           string `json:"reason"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if json.Unmarshal(body, &command) != nil || command.DraftID != id || !(PromptDraftCommand{CommandID: command.CommandID, Reason: command.Reason}).Valid() {
		return nil, ErrInvalid
	}
	switch operation {
	case "create":
	case "revise", "freeze":
		if command.ExpectedRevision <= 0 {
			return nil, ErrInvalid
		}
	default:
		return nil, ErrInvalid
	}
	return s.Gateway.WriteSemanticDraft(ctx, scope, operation, body)
}
