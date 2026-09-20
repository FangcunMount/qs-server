package aibridge

import (
	"context"
	"encoding/json"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type RegisterSuite struct {
	CaseEditsJSON               string               `json:"case_edits_json,omitempty"`
	SemanticPrompt              *FrozenEvaluationRef `json:"semantic_prompt,omitempty"`
	SemanticOwnerOrganizationID int64                `json:"semantic_owner_organization_id,omitempty"`
	PromptDraftCommand
	Source          FrozenEvaluationRef `json:"source"`
	SuiteID         string              `json:"suite_id"`
	SuiteVersion    string              `json:"suite_version"`
	Profile         PromptDraftSource   `json:"profile"`
	Prompt          PromptDraftSource   `json:"prompt"`
	GenerationRoute PromptDraftSource   `json:"generation_route"`
}

func (c RegisterSuite) Valid() bool {
	if len(c.CaseEditsJSON) > 8192 || (c.CaseEditsJSON != "" && !json.Valid([]byte(c.CaseEditsJSON))) || c.SemanticOwnerOrganizationID < 0 || (c.SemanticPrompt == nil && c.SemanticOwnerOrganizationID != 0) || (c.SemanticPrompt != nil && !c.SemanticPrompt.Valid()) {
		return false
	}
	return c.PromptDraftCommand.Valid() && c.Source.Valid() && frozenID.MatchString(c.SuiteID) && frozenVersion.MatchString(c.SuiteVersion) && c.Profile.Valid() && c.Prompt.Valid() && c.GenerationRoute.Valid()
}

func (c RegisterSuite) Equal(other RegisterSuite) bool {
	if (c.SemanticPrompt == nil) != (other.SemanticPrompt == nil) {
		return false
	}
	if c.SemanticPrompt != nil && *c.SemanticPrompt != *other.SemanticPrompt {
		return false
	}
	c.SemanticPrompt, other.SemanticPrompt = nil, nil
	return c == other
}

type SuiteRegistrationReceipt struct {
	Suite        FrozenEvaluationRef `json:"suite"`
	Scope        DraftScope          `json:"scope"`
	Command      RegisterSuite       `json:"command"`
	Manifest     RegisteredManifest  `json:"manifest"`
	RegisteredAt string              `json:"registered_at"`
}
type SuiteGateway interface {
	RegisterSuite(context.Context, DraftScope, RegisterSuite) (SuiteRegistrationReceipt, error)
	GetSuiteReceipt(context.Context, DraftScope, string) (SuiteRegistrationReceipt, error)
}
type SuiteAdministration struct{ Gateway SuiteGateway }

func (s *SuiteAdministration) authorize(ctx context.Context, scope DraftScope, capability authz.Capability) error {
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
func (s *SuiteAdministration) Register(ctx context.Context, scope DraftScope, command RegisterSuite) (SuiteRegistrationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return SuiteRegistrationReceipt{}, err
	}
	if !command.Valid() {
		return SuiteRegistrationReceipt{}, ErrInvalid
	}
	return s.Gateway.RegisterSuite(ctx, scope, command)
}
func (s *SuiteAdministration) GetReceipt(ctx context.Context, scope DraftScope, id string) (SuiteRegistrationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return SuiteRegistrationReceipt{}, err
	}
	if !ValidPublicationID(id) {
		return SuiteRegistrationReceipt{}, ErrInvalid
	}
	return s.Gateway.GetSuiteReceipt(ctx, scope, id)
}
