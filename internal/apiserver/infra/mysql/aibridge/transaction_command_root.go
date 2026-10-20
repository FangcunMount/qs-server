package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// commandRoot is called only after all SDK half messages are prepared. It owns
// this one original SQL root; the archive guard borrows it. No Seal or Broker I/O
// occurs while it holds business locks.
func (s *TransactionArchive) commandRoot(ctx context.Context, e transaction.Execution) (transaction.LocalOutcome, error) {
	c, a, err := app.ReadTransactionCommand(e.Plan)
	if err != nil || !e.Valid() {
		return transaction.LocalUnknown, errors.Join(app.ErrMessagingContract, err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return transaction.LocalUnknown, err
	}
	defer func() { _ = tx.Rollback() }()
	guard, err := s.Bind(ctx, tx, e) // first operation: fences the same SDK aborter
	if err == nil {
		err = guardCommandSequence(ctx, tx, c, a.Sequence)
	}
	if err == nil {
		err = writeCommandFacts(ctx, tx, c)
	}
	if err == nil {
		err = insertCommandOperation(ctx, tx, c, a)
	}
	if err == nil {
		err = guard.Put(ctx, c.ID())
	}
	if err == nil {
		err = guard.CommitWitness(ctx)
	}
	if err != nil {
		// A callback error alone proves nothing. First finish the actual root
		// rollback, then compete for and persist the exact execution abortion.
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return transaction.LocalUnknown, errors.Join(err, rollbackErr)
		}
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		aborted, abortErr := s.store.Abort(abortCtx, e)
		if abortErr == nil && aborted {
			return transaction.LocalAborted, err
		}
		return transaction.LocalUnknown, errors.Join(err, abortErr)
	}
	if err = tx.Commit(); err != nil {
		// The server may have committed. Only a fresh exact witness can resolve
		// this; neither cancellation nor a failed second Rollback can.
		return transaction.LocalUnknown, err
	}
	return transaction.LocalCommitted, nil
}

func insertCommandOperation(ctx context.Context, tx *sql.Tx, c app.TransactionCommand, a app.FrozenMessagingArchive) error {
	e, scope := a.Envelope, c.Scope()
	if _, err := tx.ExecContext(ctx, `INSERT INTO ai_messaging_operations(command_id,kind,body_sha256,organization_id,subject_id,resource_id,aggregate_key,aggregate_sequence,created_at) VALUES(?,?,?,?,?,?,?,?,UTC_TIMESTAMP(6))`, c.ID(), e.Kind, e.BodySha256, scope.OrganizationID, scope.SubjectID, scope.ResourceID, c.Aggregate(), a.Sequence); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE ai_messaging_aggregates SET next_sequence=next_sequence+1 WHERE aggregate_key=? AND next_sequence=?", c.Aggregate(), a.Sequence)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return app.ErrConflict
	}
	return nil
}

// Reconstruct business input from the durable reservation winner, never the
// losing HTTP request or a mutable protobuf shared with a caller.
func writeCommandFacts(ctx context.Context, tx *sql.Tx, c app.TransactionCommand) error {
	body, err := c.Body()
	if err != nil {
		return err
	}
	switch {
	case body.GetStart() != nil:
		r := originalStart(body.GetStart())
		raw, hash, err := encode(r)
		if err != nil {
			return err
		}
		return persistStart(ctx, tx, r, raw, hash)
	case body.GetChange() != nil:
		v := body.GetChange()
		return validateChange(ctx, tx, c.Aggregate(), app.Change{CommandID: v.CommandId, SessionID: v.SessionId, Actor: app.Actor{OrgID: v.GetActor().GetOrgId(), SubjectID: v.GetActor().GetSubjectId()}, Action: v.Action, ExpectedVersion: v.ExpectedVersion, QuestionID: v.QuestionId, Answer: v.Answer, Skip: v.Skip})
	default:
		// Administrative commands were authorized before reservation; the AI
		// admission root retains its current eligibility checks.
		return nil
	}
}

func originalStart(v *pb.StartCommand) app.Start {
	r := app.Start{RequestID: v.RequestId, Actor: app.Actor{OrgID: v.GetActor().GetOrgId(), SubjectID: v.GetActor().GetSubjectId()}, TesteeID: v.TesteeId, AssessmentIDs: append([]string(nil), v.AssessmentIds...), Goal: v.Goal}
	for _, item := range v.Evidence {
		e := app.EvidenceItem{AssessmentID: item.GetAssessmentId(), TesteeID: item.GetTesteeId(), ReportID: item.GetReportId(), SourceVersion: item.GetSourceVersion()}
		for _, fact := range item.GetFacts() {
			e.Facts = append(e.Facts, app.Fact{Ref: fact.GetRef(), Value: fact.GetValue()})
		}
		r.Evidence = append(r.Evidence, e)
	}
	return r
}

func (s *TransactionArchive) confirmCommand(ctx context.Context, e transaction.Execution) transaction.LocalOutcome {
	if _, _, err := app.ReadTransactionCommand(e.Plan); err != nil {
		return transaction.LocalUnknown
	}
	var facts []*transaction.Evidence
	for _, ref := range e.References() {
		fact, err := s.store.Evidence(ctx, ref)
		if err != nil {
			return transaction.LocalUnknown
		}
		facts = append(facts, fact)
	}
	return transaction.DecideLocal(e.References(), facts)
}
