package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// MessagingSeal is a pure, locally configured callback. It is invoked once for a
// new operation, never for a duplicate command or a network retry.
type MessagingSeal func(kind pb.MessagingKind, id, aggregate, org, originalTime string, body *pb.MessagingBody) (*app.PreparedMessaging, error)

// MessagingCommandStore reuses the original request/projection store and changes
// only command staging. Its host chooses this store before admitting MQ traffic;
// it creates no legacy command row, fallback delivery or second scheduler.
type MessagingCommandStore struct {
	*Store
	Messaging    *MessagingStore
	Seal         MessagingSeal
	Transactions *TransactionCommands
}

func (s *MessagingCommandStore) StageStart(ctx context.Context, r app.Start) error {
	body := startMessagingBody(r)
	return s.stageCommand(ctx, pb.MessagingKind_START, r.RequestID, r.RequestID, app.OperationScope{OrganizationID: r.Actor.OrgID, SubjectID: r.Actor.SubjectID, ResourceID: r.RequestID}, body)
}

func (s *MessagingCommandStore) StageChange(ctx context.Context, requestID string, r app.Change) error {
	body := changeMessagingBody(r)
	return s.stageCommand(ctx, pb.MessagingKind_CHANGE, r.CommandID, requestID, app.OperationScope{OrganizationID: r.Actor.OrgID, SubjectID: r.Actor.SubjectID, ResourceID: r.SessionID}, body)
}

func (s *MessagingCommandStore) stageCommand(ctx context.Context, kind pb.MessagingKind, id, aggregate string, scope app.OperationScope, body *pb.MessagingBody) error {
	if s == nil || s.Store == nil || s.DB == nil || s.Messaging == nil || s.Seal == nil || s.Transactions == nil {
		return app.ErrManagementUnavailable
	}
	command, err := app.NewTransactionCommand(kind, id, aggregate, scope, body)
	if err != nil {
		return err
	}
	err = s.Transactions.Submit(ctx, command)
	if errors.Is(err, transaction.ErrInputConflict) {
		return errors.Join(app.ErrConflict, err)
	}
	return err
}

// SubmitParticipantRetry preserves the original request aggregate. The host
// authenticates the operator before calling; qs-ai decides current eligibility
// and rechecks participant access in its original admission transaction.
func (s *MessagingCommandStore) SubmitParticipantRetry(ctx context.Context, scope app.DraftScope, sessionID string, command app.ParticipantRetry) error {
	if s == nil || s.Store == nil || s.DB == nil || s.Transactions == nil {
		return app.ErrManagementUnavailable
	}
	if s.Transactions != nil {
		if _, nested := hostmysql.TxFromContext(ctx); nested {
			return transaction.ErrTransactionRequired
		}
	}
	var requestID string
	err := s.DB.QueryRowContext(ctx, "SELECT request_id FROM ai_bridge_requests WHERE session_id=? AND organization_id=?", sessionID, scope.OrganizationID).Scan(&requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrNotFound
	}
	if err != nil {
		return err
	}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_ParticipantRetry{ParticipantRetry: &pb.ParticipantRetryCommand{
		Scope: &pb.PublicationScope{OrganizationId: scope.OrganizationID, OperatorUserId: scope.OperatorUserID}, SessionId: sessionID, CommandId: command.CommandID, ExpectedRunId: command.ExpectedRunID, ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm, ExpectedProviderInvocations: command.ExpectedProviderInvocations, AcceptResultUnknownRisk: command.AcceptResultUnknownRisk,
	}}}
	return s.stageCommand(ctx, pb.MessagingKind_PARTICIPANT_RETRY, command.CommandID, requestID, app.OperationScope{OrganizationID: strconv.FormatInt(scope.OrganizationID, 10), SubjectID: strconv.FormatInt(scope.OperatorUserID, 10), ResourceID: sessionID}, body)
}

func (s *MessagingCommandStore) SubmitEvaluationStart(ctx context.Context, scope app.EvaluationScope, commandID string, command app.EvaluationStart) error {
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationStart{EvaluationStart: &pb.EvaluationStartCommand{
		Scope: &pb.EvaluationQuery{OrganizationId: scope.OrganizationID, OperatorUserId: scope.OperatorUserID, RunId: scope.RunID}, ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm,
	}}}
	return s.stageCommand(ctx, pb.MessagingKind_EVALUATION_START, commandID, scope.RunID, app.OperationScope{OrganizationID: strconv.FormatInt(scope.OrganizationID, 10), SubjectID: strconv.FormatInt(scope.OperatorUserID, 10), ResourceID: scope.RunID}, body)
}

func (s *MessagingCommandStore) SubmitEvaluationCancel(ctx context.Context, scope app.EvaluationScope, commandID string, command app.EvaluationCancel) error {
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationCancel{EvaluationCancel: &pb.EvaluationCancelCommand{
		Scope: &pb.EvaluationQuery{OrganizationId: scope.OrganizationID, OperatorUserId: scope.OperatorUserID, RunId: scope.RunID}, ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm, Discard: command.Discard,
	}}}
	return s.stageCommand(ctx, pb.MessagingKind_EVALUATION_CANCEL, commandID, scope.RunID, app.OperationScope{OrganizationID: strconv.FormatInt(scope.OrganizationID, 10), SubjectID: strconv.FormatInt(scope.OperatorUserID, 10), ResourceID: scope.RunID}, body)
}

// Current operation readers never interpret retained publication rows as modern
// business evidence. The original host must supply its transaction adapter.
func (s *MessagingCommandStore) ReadOperation(ctx context.Context, scope app.OperationScope, id string) (app.MessagingOperation, error) {
	return s.readCurrentOperation(ctx, scope, "", id)
}
func (s *MessagingCommandStore) ReadRequestOperation(ctx context.Context, scope app.OperationScope, requestID, id string) (app.MessagingOperation, error) {
	return s.readCurrentOperation(ctx, scope, requestID, id)
}
func (s *MessagingCommandStore) readCurrentOperation(ctx context.Context, scope app.OperationScope, requestID, id string) (app.MessagingOperation, error) {
	if s == nil || s.Transactions == nil {
		return app.MessagingOperation{}, app.ErrManagementUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	operation, found, err := s.Transactions.operation(ctx, scope, requestID, id)
	if err != nil {
		return app.MessagingOperation{}, err
	}
	if !found {
		return app.MessagingOperation{}, app.ErrNotFound
	}
	return operation, nil
}

func startMessagingBody(r app.Start) *pb.MessagingBody {
	body := &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: r.RequestID, Actor: &pb.Actor{OrgId: r.Actor.OrgID, SubjectId: r.Actor.SubjectID}, TesteeId: r.TesteeID, AssessmentIds: r.AssessmentIDs, Goal: r.Goal}}}
	for _, e := range r.Evidence {
		item := &pb.EvidenceItem{AssessmentId: e.AssessmentID, TesteeId: e.TesteeID, ReportId: e.ReportID, SourceVersion: e.SourceVersion}
		for _, f := range e.Facts {
			item.Facts = append(item.Facts, &pb.Fact{Ref: f.Ref, Value: f.Value})
		}
		body.GetStart().Evidence = append(body.GetStart().Evidence, item)
	}
	return body
}

func changeMessagingBody(r app.Change) *pb.MessagingBody {
	body := &pb.MessagingBody{Value: &pb.MessagingBody_Change{Change: &pb.ChangeCommand{CommandId: r.CommandID, SessionId: r.SessionID, Actor: &pb.Actor{OrgId: r.Actor.OrgID, SubjectId: r.Actor.SubjectID}, Action: r.Action, ExpectedVersion: r.ExpectedVersion, QuestionId: r.QuestionID, Answer: r.Answer, Skip: r.Skip}}}
	return body
}
