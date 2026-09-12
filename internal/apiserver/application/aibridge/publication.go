package aibridge

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/google/uuid"
)

// PublicationScope is supplied by QS protected context, never by request JSON.
type PublicationScope struct{ OrganizationID, OperatorUserID int64 }
type PublicationSelector struct {
	Audience     string  `json:"audience"`
	ModelKind    string  `json:"model_kind"`
	DecisionKind string  `json:"decision_kind"`
	ModelCode    *string `json:"model_code,omitempty"`
	ModelVersion *string `json:"model_version,omitempty"`
}

func (s PublicationSelector) Valid() bool {
	return s.Audience == "participant" && s.ModelKind == "scale" && s.DecisionKind == "score_range" &&
		(s.ModelCode == nil || (strings.TrimSpace(*s.ModelCode) != "" && len(*s.ModelCode) <= 255 && utf8.ValidString(*s.ModelCode))) &&
		(s.ModelVersion == nil || (s.ModelCode != nil && frozenVersion.MatchString(*s.ModelVersion)))
}
func (s PublicationSelector) Equal(other PublicationSelector) bool {
	same := func(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
	return s.Audience == other.Audience && s.ModelKind == other.ModelKind && s.DecisionKind == other.DecisionKind && same(s.ModelCode, other.ModelCode) && same(s.ModelVersion, other.ModelVersion)
}
func ValidPublicationID(raw string) bool {
	id, err := uuid.Parse(raw)
	return err == nil && id != uuid.Nil && id.String() == raw
}

type PublicationExpectation struct {
	Selector            PublicationSelector `json:"selector"`
	Version             *int64              `json:"version"`
	ActivePublicationID string              `json:"active_publication_id"`
}
type PublicationCommand struct {
	CommandID string                  `json:"command_id"`
	Expected  *PublicationExpectation `json:"expected"`
	Reason    string                  `json:"reason"`
	Confirm   bool                    `json:"confirm"`
}

func (c PublicationCommand) Valid() bool {
	return ValidPublicationID(c.CommandID) && c.Expected != nil && c.Expected.Selector.Valid() &&
		c.Expected.Version != nil && *c.Expected.Version >= 0 &&
		(c.Expected.ActivePublicationID == "" || ValidPublicationID(c.Expected.ActivePublicationID)) &&
		c.Confirm && strings.TrimSpace(c.Reason) != "" && len(c.Reason) <= 1000 && utf8.ValidString(c.Reason) && !strings.ContainsAny(c.Reason, "<>")
}

type PublishConfiguration struct {
	PublicationCommand
	RunID              string `json:"run_id"`
	RunVersion         int64  `json:"run_version"`
	ReleaseFingerprint string `json:"release_fingerprint"`
}
type RollbackPublication struct {
	PublicationCommand
	TargetPublicationID string `json:"target_publication_id"`
}
type PublicationState struct {
	Selector            PublicationSelector `json:"selector"`
	Version             int64               `json:"version"`
	ActivePublicationID string              `json:"active_publication_id"`
	Publication         json.RawMessage     `json:"publication,omitempty" swaggertype:"object"`
	ChangedAt           string              `json:"changed_at"`
}
type PublicationReceipt struct {
	CommandID string           `json:"command_id"`
	Previous  PublicationState `json:"previous"`
	Current   PublicationState `json:"current"`
	Action    string           `json:"action"`
	Actor     string           `json:"actor"`
	Reason    string           `json:"reason"`
	ChangedAt string           `json:"changed_at"`
}
type PublicationGateway interface {
	PublishConfiguration(context.Context, PublicationScope, PublishConfiguration) (PublicationReceipt, error)
	RollbackPublication(context.Context, PublicationScope, RollbackPublication) (PublicationReceipt, error)
	DisablePublication(context.Context, PublicationScope, PublicationCommand) (PublicationReceipt, error)
	GetPublication(context.Context, PublicationScope, PublicationSelector) (PublicationState, error)
	GetPublicationReceipt(context.Context, PublicationScope, string) (PublicationReceipt, error)
}
type PublicationAdministration struct{ Gateway PublicationGateway }

func (s *PublicationAdministration) authorize(ctx context.Context, scope PublicationScope, capability authz.Capability) error {
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
func (s *PublicationAdministration) Publish(ctx context.Context, scope PublicationScope, command PublishConfiguration) (PublicationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return PublicationReceipt{}, err
	}
	if !command.Valid() || !ValidPublicationID(command.RunID) || command.RunVersion < 1 || !frozenFingerprint.MatchString(command.ReleaseFingerprint) {
		return PublicationReceipt{}, ErrInvalid
	}
	return s.Gateway.PublishConfiguration(ctx, scope, command)
}
func (s *PublicationAdministration) Rollback(ctx context.Context, scope PublicationScope, command RollbackPublication) (PublicationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return PublicationReceipt{}, err
	}
	if !command.Valid() || !ValidPublicationID(command.TargetPublicationID) {
		return PublicationReceipt{}, ErrInvalid
	}
	return s.Gateway.RollbackPublication(ctx, scope, command)
}
func (s *PublicationAdministration) Disable(ctx context.Context, scope PublicationScope, command PublicationCommand) (PublicationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityOrgAdmin); err != nil {
		return PublicationReceipt{}, err
	}
	if !command.Valid() {
		return PublicationReceipt{}, ErrInvalid
	}
	return s.Gateway.DisablePublication(ctx, scope, command)
}
func (s *PublicationAdministration) Get(ctx context.Context, scope PublicationScope, selector PublicationSelector) (PublicationState, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return PublicationState{}, err
	}
	if !selector.Valid() {
		return PublicationState{}, ErrInvalid
	}
	return s.Gateway.GetPublication(ctx, scope, selector)
}
func (s *PublicationAdministration) GetReceipt(ctx context.Context, scope PublicationScope, commandID string) (PublicationReceipt, error) {
	if err := s.authorize(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return PublicationReceipt{}, err
	}
	if !ValidPublicationID(commandID) {
		return PublicationReceipt{}, ErrInvalid
	}
	return s.Gateway.GetPublicationReceipt(ctx, scope, commandID)
}
