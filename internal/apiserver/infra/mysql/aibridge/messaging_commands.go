package aibridge

import (
	"context"
	"database/sql"
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
	body := &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: r.RequestID, Actor: &pb.Actor{OrgId: r.Actor.OrgID, SubjectId: r.Actor.SubjectID}, TesteeId: r.TesteeID, AssessmentIds: r.AssessmentIDs, Goal: r.Goal}}}
	for _, e := range r.Evidence {
		item := &pb.EvidenceItem{AssessmentId: e.AssessmentID, TesteeId: e.TesteeID, ReportId: e.ReportID, SourceVersion: e.SourceVersion}
		for _, f := range e.Facts {
			item.Facts = append(item.Facts, &pb.Fact{Ref: f.Ref, Value: f.Value})
		}
		body.GetStart().Evidence = append(body.GetStart().Evidence, item)
	}
	return s.stageCommand(ctx, pb.MessagingKind_START, r.RequestID, r.RequestID, app.OperationScope{OrganizationID: r.Actor.OrgID, SubjectID: r.Actor.SubjectID, ResourceID: r.RequestID}, body, func(tx *sql.Tx) error { return persistStart(ctx, tx, r, raw, hash) })
}

func (s *MessagingCommandStore) StageChange(ctx context.Context, requestID string, r app.Change) error {
	body := &pb.MessagingBody{Value: &pb.MessagingBody_Change{Change: &pb.ChangeCommand{CommandId: r.CommandID, SessionId: r.SessionID, Actor: &pb.Actor{OrgId: r.Actor.OrgID, SubjectId: r.Actor.SubjectID}, Action: r.Action, ExpectedVersion: r.ExpectedVersion, QuestionId: r.QuestionID, Answer: r.Answer, Skip: r.Skip}}}
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
