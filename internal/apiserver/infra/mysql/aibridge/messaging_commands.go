package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

// MessagingSeal is a pure, locally configured callback. It is invoked once for a
// new operation, never for a duplicate command or a network retry.
type MessagingSeal func(kind pb.MessagingKind, id, aggregate, org, originalTime string, body *pb.MessagingBody) (*app.PreparedMessaging, error)

// MessagingCommandStore reuses the original request/projection store and changes
// only command staging. Its host chooses this store before admitting MQ traffic;
// it creates no legacy command row, fallback delivery or second scheduler.
type MessagingCommandStore struct {
	*Store
	Messaging *MessagingStore
	Seal      MessagingSeal
}

func (s *MessagingCommandStore) StageStart(ctx context.Context, r app.Start) error {
	raw, hash, err := encode(r)
	if err != nil {
		return err
	}
	body := startMessagingBody(r)
	return s.stageCommand(ctx, pb.MessagingKind_START, r.RequestID, r.RequestID, app.OperationScope{OrganizationID: r.Actor.OrgID, SubjectID: r.Actor.SubjectID, ResourceID: r.RequestID}, body, func(tx *sql.Tx) error { return persistStart(ctx, tx, r, raw, hash) })
}

func (s *MessagingCommandStore) StageChange(ctx context.Context, requestID string, r app.Change) error {
	body := changeMessagingBody(r)
	return s.stageCommand(ctx, pb.MessagingKind_CHANGE, r.CommandID, requestID, app.OperationScope{OrganizationID: r.Actor.OrgID, SubjectID: r.Actor.SubjectID, ResourceID: r.SessionID}, body, func(tx *sql.Tx) error { return validateChange(ctx, tx, requestID, r) })
}

func (s *MessagingCommandStore) stageCommand(ctx context.Context, kind pb.MessagingKind, id, aggregate string, scope app.OperationScope, body *pb.MessagingBody, original func(*sql.Tx) error) error {
	if s == nil || s.Store == nil || s.DB == nil || s.Messaging == nil || s.Seal == nil {
		return app.ErrManagementUnavailable
	}
	envelope, raw, err := app.PrepareMessaging(kind, id, aggregate, "", body)
	if err != nil {
		return err
	}
	envelope.OriginalOccurredAt = time.Now().In(time.FixedZone("UTC+8", 8*60*60)).Format(time.RFC3339Nano)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Acquire the shared gate lock before any business write, then check immutable
	// identities. A closed gate still permits the original operation to be read
	// and returned; current authorization has already run in the host application.
	admission, err := (MessagingAdmission{}).Read(ctx, tx)
	if err != nil {
		return err
	}
	exists, err := s.Messaging.existingOperation(ctx, tx, envelope, scope)
	if err != nil {
		return err
	}
	if exists {
		return tx.Commit()
	}
	if admission.Closed {
		return app.ErrRuntimeAdmissionClosed
	}
	if original != nil {
		if err = original(tx); err != nil {
			return err
		}
	}
	_, err = s.Messaging.StageOperation(ctx, tx, envelope, raw, scope, func() (*app.PreparedMessaging, error) {
		return s.Seal(kind, id, aggregate, scope.OrganizationID, envelope.OriginalOccurredAt, body)
	})
	if err != nil {
		return err
	}
	return tx.Commit()
}

// No legacy scanner may claim MQ-owned commands even if a host accidentally
// invokes Service.Relay. Host lifecycle must choose the MQ step explicitly.
func (s *MessagingCommandStore) Pending(context.Context, int) ([]app.Command, error) {
	return nil, app.ErrManagementUnavailable
}

// SubmitParticipantRetry preserves the original request aggregate. The host
// authenticates the operator before calling; qs-ai decides current eligibility
// and rechecks participant access in its original admission transaction.
func (s *MessagingCommandStore) SubmitParticipantRetry(ctx context.Context, scope app.DraftScope, sessionID string, command app.ParticipantRetry) error {
	if s == nil || s.Store == nil || s.DB == nil {
		return app.ErrManagementUnavailable
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
	return s.stageCommand(ctx, pb.MessagingKind_PARTICIPANT_RETRY, command.CommandID, requestID, app.OperationScope{OrganizationID: strconv.FormatInt(scope.OrganizationID, 10), SubjectID: strconv.FormatInt(scope.OperatorUserID, 10), ResourceID: sessionID}, body, nil)
}

func (s *MessagingCommandStore) SubmitEvaluationStart(ctx context.Context, scope app.EvaluationScope, commandID string, command app.EvaluationStart) error {
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationStart{EvaluationStart: &pb.EvaluationStartCommand{
		Scope: &pb.EvaluationQuery{OrganizationId: scope.OrganizationID, OperatorUserId: scope.OperatorUserID, RunId: scope.RunID}, ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm,
	}}}
	return s.stageCommand(ctx, pb.MessagingKind_EVALUATION_START, commandID, scope.RunID, app.OperationScope{OrganizationID: strconv.FormatInt(scope.OrganizationID, 10), SubjectID: strconv.FormatInt(scope.OperatorUserID, 10), ResourceID: scope.RunID}, body, nil)
}

func (s *MessagingCommandStore) SubmitEvaluationCancel(ctx context.Context, scope app.EvaluationScope, commandID string, command app.EvaluationCancel) error {
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationCancel{EvaluationCancel: &pb.EvaluationCancelCommand{
		Scope: &pb.EvaluationQuery{OrganizationId: scope.OrganizationID, OperatorUserId: scope.OperatorUserID, RunId: scope.RunID}, ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm, Discard: command.Discard,
	}}}
	return s.stageCommand(ctx, pb.MessagingKind_EVALUATION_CANCEL, commandID, scope.RunID, app.OperationScope{OrganizationID: strconv.FormatInt(scope.OrganizationID, 10), SubjectID: strconv.FormatInt(scope.OperatorUserID, 10), ResourceID: scope.RunID}, body, nil)
}

func (s *MessagingCommandStore) ReadOperation(ctx context.Context, scope app.OperationScope, id string) (app.MessagingOperation, error) {
	if s == nil || s.Store == nil || s.DB == nil || s.Messaging == nil {
		return app.MessagingOperation{}, app.ErrManagementUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return app.MessagingOperation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	return s.Messaging.Operation(ctx, tx, scope, id)
}

func (s *MessagingCommandStore) ReadRequestOperation(ctx context.Context, scope app.OperationScope, requestID, id string) (app.MessagingOperation, error) {
	if s == nil || s.DB == nil || s.Messaging == nil {
		return app.MessagingOperation{}, app.ErrManagementUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return app.MessagingOperation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var found string
	err = tx.QueryRowContext(ctx, `SELECT message_id FROM ai_messaging_outbox WHERE producer='qs-server' AND destination='qs-ai' AND message_id=? AND aggregate_key=? AND kind IN (?,?)`, id, requestID, pb.MessagingKind_START, pb.MessagingKind_CHANGE).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return app.MessagingOperation{}, app.ErrNotFound
	}
	if err != nil {
		return app.MessagingOperation{}, err
	}
	return s.Messaging.Operation(ctx, tx, scope, id)
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
