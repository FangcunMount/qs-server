package aibridge

import (
	"context"
	"strconv"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/google/uuid"
)

// RuntimeCommandSubmitter commits a host operation and Outbox before returning.
// It cannot return an AI decision or authorize execution by Broker confirmation.
type RuntimeCommandSubmitter interface {
	SubmitParticipantRetry(context.Context, DraftScope, string, ParticipantRetry) error
	SubmitEvaluationStart(context.Context, EvaluationScope, string, EvaluationStart) error
	SubmitEvaluationCancel(context.Context, EvaluationScope, string, EvaluationCancel) error
}

func validMQCommandID(id string) bool { return validID(id) && id != uuid.Nil.String() }

func (s *ParticipantAdministration) SubmitRetry(ctx context.Context, scope DraftScope, sessionID string, command ParticipantRetry) error {
	if err := s.authorize(ctx, scope); err != nil {
		return err
	}
	command, err := normalizeParticipantRetry(sessionID, command)
	if err != nil {
		return err
	}
	if s.Messages == nil {
		return ErrManagementUnavailable
	}
	return s.Messages.SubmitParticipantRetry(ctx, scope, sessionID, command)
}
func (s *EvaluationAdministration) SubmitStart(ctx context.Context, scope EvaluationScope, command EvaluationStart) error {
	if err := s.authorize(ctx, scope); err != nil {
		return err
	}
	command, err := normalizeEvaluationStart(command)
	if err != nil {
		return err
	}
	if !validMQCommandID(command.CommandID) {
		return ErrInvalid
	}
	if s.Messages == nil {
		return ErrManagementUnavailable
	}
	return s.Messages.SubmitEvaluationStart(ctx, scope, command.CommandID, command)
}
func (s *EvaluationAdministration) SubmitCancel(ctx context.Context, scope EvaluationScope, command EvaluationCancel) error {
	if err := s.authorize(ctx, scope); err != nil {
		return err
	}
	command, err := normalizeEvaluationCancel(command)
	if err != nil {
		return err
	}
	if !validMQCommandID(command.CommandID) {
		return ErrInvalid
	}
	if s.Messages == nil {
		return ErrManagementUnavailable
	}
	return s.Messages.SubmitEvaluationCancel(ctx, scope, command.CommandID, command)
}

type MessagingOperationReader interface {
	ReadOperation(context.Context, OperationScope, string) (MessagingOperation, error)
}
type OperationAdministration struct{ Store MessagingOperationReader }

func (s *OperationAdministration) Get(ctx context.Context, scope DraftScope, id string) (MessagingOperation, error) {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 || !validMQCommandID(id) {
		return MessagingOperation{}, ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityOrgAdmin).Allowed {
		return MessagingOperation{}, ErrGovernanceDenied
	}
	if s == nil || s.Store == nil {
		return MessagingOperation{}, ErrManagementUnavailable
	}
	return s.Store.ReadOperation(ctx, OperationScope{OrganizationID: strconv.FormatInt(scope.OrganizationID, 10), SubjectID: strconv.FormatInt(scope.OperatorUserID, 10)}, id)
}
