package aibridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// PromptDraftLifecycle is a read snapshot, not permission to issue a later edit.
type PromptDraftLifecycle struct {
	SchemaVersion string               `json:"schema_version"`
	Draft         PromptDraftState     `json:"draft"`
	Status        string               `json:"status"`
	Frozen        *PromptFrozenVersion `json:"frozen,omitempty"`
}

type PromptFrozenVersion struct {
	Asset    PromptDraftSource `json:"asset"`
	Revision int64             `json:"revision"`
	FrozenAt string            `json:"frozen_at"`
}

func (s *PromptDraftAdministration) GetLifecycle(ctx context.Context, scope DraftScope, id string) (PromptDraftLifecycle, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return PromptDraftLifecycle{}, err
	}
	if !ValidPublicationID(id) {
		return PromptDraftLifecycle{}, ErrInvalid
	}
	return s.Gateway.GetPromptDraftLifecycle(ctx, scope, id)
}
