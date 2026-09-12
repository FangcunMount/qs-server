package aibridge

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

// DraftScope is populated only from the protected QS context.
type DraftScope struct{ OrganizationID, OperatorUserID int64 }
type PromptDraftSource struct {
	Identity      string `json:"identity"`
	Version       string `json:"version"`
	Fingerprint   string `json:"fingerprint"`
	ContentSHA256 string `json:"content_sha256"`
}

func (s PromptDraftSource) Valid() bool {
	return strings.TrimSpace(s.Identity) != "" && len(s.Identity) <= 255 && utf8.ValidString(s.Identity) && frozenVersion.MatchString(s.Version) && frozenFingerprint.MatchString(s.Fingerprint) && frozenFingerprint.MatchString("sha256:"+s.ContentSHA256)
}

type PromptDraftContent struct {
	SystemMessage       string   `json:"system_message"`
	TaskTemplate        string   `json:"task_template"`
	DataPreamble        string   `json:"data_preamble"`
	AllowedPlaceholders []string `json:"allowed_placeholders"`
}

func (c PromptDraftContent) Valid() bool {
	for _, v := range []string{c.SystemMessage, c.TaskTemplate, c.DataPreamble} {
		if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
			return false
		}
	}
	if len(c.AllowedPlaceholders) > 64 {
		return false
	}
	for _, v := range c.AllowedPlaceholders {
		if !utf8.ValidString(v) || utf8.RuneCountInString(v) > 128 {
			return false
		}
	}
	// AI owns the canonical serialized size and template validation. Bound the
	// transport here without rejecting its Unicode/HTML serialization choices.
	raw, err := json.Marshal(c)
	return err == nil && len(raw) <= 256*1024
}
func (c PromptDraftContent) Equal(other PromptDraftContent) bool {
	return c.SystemMessage == other.SystemMessage && c.TaskTemplate == other.TaskTemplate && c.DataPreamble == other.DataPreamble && slices.Equal(c.AllowedPlaceholders, other.AllowedPlaceholders)
}

type PromptDraftCommand struct {
	CommandID string `json:"command_id"`
	Reason    string `json:"reason"`
}

func (c PromptDraftCommand) Valid() bool {
	return ValidPublicationID(c.CommandID) && strings.TrimSpace(c.Reason) != "" && len(c.Reason) <= 1000 && utf8.ValidString(c.Reason) && !strings.ContainsAny(c.Reason, "<>\x00")
}

type CreatePromptDraft struct {
	PromptDraftCommand
	Source        PromptDraftSource `json:"source"`
	TemplateID    string            `json:"template_id"`
	TargetVersion string            `json:"target_version"`
}

func (c CreatePromptDraft) Valid() bool {
	return c.PromptDraftCommand.Valid() && c.Source.Valid() && frozenVersion.MatchString(c.TemplateID) && frozenVersion.MatchString(c.TargetVersion) && (c.TemplateID != c.Source.Identity || c.TargetVersion != c.Source.Version)
}

type RevisePromptDraft struct {
	PromptDraftCommand
	ExpectedRevision int64               `json:"expected_revision"`
	Content          *PromptDraftContent `json:"content"`
}

func (c RevisePromptDraft) Valid() bool {
	return c.PromptDraftCommand.Valid() && c.ExpectedRevision > 0 && c.ExpectedRevision < 9223372036854775807 && c.Content != nil && c.Content.Valid()
}

type PromptDraftState struct {
	DraftID        string             `json:"draft_id"`
	OrganizationID int64              `json:"organization_id"`
	TemplateID     string             `json:"template_id"`
	TargetVersion  string             `json:"target_version"`
	Source         PromptDraftSource  `json:"source"`
	Revision       int64              `json:"revision"`
	Content        PromptDraftContent `json:"content"`
	CommandID      string             `json:"command_id"`
	OperatorUserID int64              `json:"operator_user_id"`
	Reason         string             `json:"reason"`
	SavedAt        string             `json:"saved_at"`
}
type PromptDraftGateway interface {
	CreatePromptDraft(context.Context, DraftScope, string, CreatePromptDraft) (PromptDraftState, error)
	RevisePromptDraft(context.Context, DraftScope, string, RevisePromptDraft) (PromptDraftState, error)
	GetPromptDraft(context.Context, DraftScope, string, *int64) (PromptDraftState, error)
	GetPromptDraftReceipt(context.Context, DraftScope, string) (PromptDraftState, error)
}
type PromptDraftAdministration struct{ Gateway PromptDraftGateway }

func (s *PromptDraftAdministration) authorize(ctx context.Context, scope DraftScope, capability authz.Capability) error {
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
func (s *PromptDraftAdministration) Create(ctx context.Context, scope DraftScope, id string, c CreatePromptDraft) (PromptDraftState, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return PromptDraftState{}, err
	}
	if !ValidPublicationID(id) || !c.Valid() {
		return PromptDraftState{}, ErrInvalid
	}
	return s.Gateway.CreatePromptDraft(ctx, scope, id, c)
}
func (s *PromptDraftAdministration) Revise(ctx context.Context, scope DraftScope, id string, c RevisePromptDraft) (PromptDraftState, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return PromptDraftState{}, err
	}
	if !ValidPublicationID(id) || !c.Valid() {
		return PromptDraftState{}, ErrInvalid
	}
	return s.Gateway.RevisePromptDraft(ctx, scope, id, c)
}
func (s *PromptDraftAdministration) Get(ctx context.Context, scope DraftScope, id string, revision *int64) (PromptDraftState, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return PromptDraftState{}, err
	}
	if !ValidPublicationID(id) || revision != nil && *revision < 1 {
		return PromptDraftState{}, ErrInvalid
	}
	return s.Gateway.GetPromptDraft(ctx, scope, id, revision)
}
func (s *PromptDraftAdministration) GetReceipt(ctx context.Context, scope DraftScope, id string) (PromptDraftState, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return PromptDraftState{}, err
	}
	if !ValidPublicationID(id) {
		return PromptDraftState{}, ErrInvalid
	}
	return s.Gateway.GetPromptDraftReceipt(ctx, scope, id)
}
