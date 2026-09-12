package aibridge

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type FreezePromptDraft struct {
	PromptDraftCommand
	ExpectedRevision int64 `json:"expected_revision"`
}

func (c FreezePromptDraft) Valid() bool {
	return c.PromptDraftCommand.Valid() && c.ExpectedRevision > 0
}

type FrozenPromptCommand struct {
	DraftID          string `json:"draft_id"`
	CommandID        string `json:"command_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

// FrozenPromptReceipt records syntax validation and asset identity, not quality approval.
type FrozenPromptReceipt struct {
	Scope            DraftScope          `json:"scope"`
	Command          FrozenPromptCommand `json:"command"`
	Asset            PromptDraftSource   `json:"asset"`
	SnapshotSHA256   string              `json:"snapshot_sha256"`
	ValidatorVersion string              `json:"validator_version"`
	FrozenAt         string              `json:"frozen_at"`
}

func (s *PromptDraftAdministration) Freeze(ctx context.Context, scope DraftScope, id string, c FreezePromptDraft) (FrozenPromptReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return FrozenPromptReceipt{}, err
	}
	if !ValidPublicationID(id) || !c.Valid() {
		return FrozenPromptReceipt{}, ErrInvalid
	}
	return s.Gateway.FreezePromptDraft(ctx, scope, id, c)
}
func (s *PromptDraftAdministration) GetFreezeReceipt(ctx context.Context, scope DraftScope, id string) (FrozenPromptReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return FrozenPromptReceipt{}, err
	}
	if !ValidPublicationID(id) {
		return FrozenPromptReceipt{}, ErrInvalid
	}
	return s.Gateway.GetPromptFreezeReceipt(ctx, scope, id)
}
