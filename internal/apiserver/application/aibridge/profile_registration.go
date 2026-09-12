package aibridge

import (
	"context"
	"encoding/json"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"unicode/utf8"
)

type RegisterProfile struct {
	PromptDraftCommand
	Source          PromptDraftSource `json:"source"`
	DefinitionJSON  string            `json:"definition_json"`
	Prompt          PromptDraftSource `json:"prompt"`
	GenerationRoute PromptDraftSource `json:"generation_route"`
}

func (c RegisterProfile) Valid() bool {
	return c.PromptDraftCommand.Valid() && c.Source.Valid() && c.Prompt.Valid() && c.GenerationRoute.Valid() &&
		len(c.DefinitionJSON) > 0 && len(c.DefinitionJSON) <= 128*1024 && utf8.ValidString(c.DefinitionJSON) && json.Valid([]byte(c.DefinitionJSON))
}

type RegisteredManifest struct {
	Profile         PromptDraftSource `json:"profile"`
	Prompt          PromptDraftSource `json:"prompt"`
	GenerationRoute PromptDraftSource `json:"generation_route"`
	InputSchema     PromptDraftSource `json:"input_schema"`
	OutputSchema    PromptDraftSource `json:"output_schema"`
}
type ProfileRegistrationReceipt struct {
	Scope        DraftScope         `json:"scope"`
	Command      RegisterProfile    `json:"command"`
	Manifest     RegisteredManifest `json:"manifest"`
	RegisteredAt string             `json:"registered_at"`
}
type ProfileGateway interface {
	RegisterProfile(context.Context, DraftScope, RegisterProfile) (ProfileRegistrationReceipt, error)
	GetProfileReceipt(context.Context, DraftScope, string) (ProfileRegistrationReceipt, error)
}
type ProfileAdministration struct{ Gateway ProfileGateway }

func (s *ProfileAdministration) authorize(ctx context.Context, scope DraftScope, capability authz.Capability) error {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, capability).Allowed {
		return ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return ErrManagementUnavailable
	}
	return nil
}
func (s *ProfileAdministration) Register(ctx context.Context, scope DraftScope, command RegisterProfile) (ProfileRegistrationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return ProfileRegistrationReceipt{}, err
	}
	if !command.Valid() {
		return ProfileRegistrationReceipt{}, ErrInvalid
	}
	return s.Gateway.RegisterProfile(ctx, scope, command)
}
func (s *ProfileAdministration) GetReceipt(ctx context.Context, scope DraftScope, id string) (ProfileRegistrationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return ProfileRegistrationReceipt{}, err
	}
	if !ValidPublicationID(id) {
		return ProfileRegistrationReceipt{}, ErrInvalid
	}
	return s.Gateway.GetProfileReceipt(ctx, scope, id)
}
