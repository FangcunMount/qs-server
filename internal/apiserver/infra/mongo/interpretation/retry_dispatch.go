package interpretation

import (
	"context"
	"errors"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type retryDispatchAuthority struct {
	db          *mongo.Database
	runs        *RunRepository
	generations *GenerationRepository
}

func NewRetryDispatchAuthority(db *mongo.Database, opts ...base.BaseRepositoryOptions) (port.AuthorityValidator, error) {
	if db == nil {
		return nil, transaction.ErrBinding
	}
	return &retryDispatchAuthority{db: db, runs: NewRunRepositoryWithoutSchema(db, opts...), generations: NewGenerationRepositoryWithoutSchema(db, opts...)}, nil
}

func (a *retryDispatchAuthority) ValidateDispatchAuthority(ctx context.Context, f model.Frozen) error {
	if !f.Valid() || f.Authority().Owner != "qs.mongo" || f.Authority().Kind != model.Interpretation {
		return transaction.ErrBinding
	}
	session := mongo.SessionFromContext(ctx)
	if session == nil {
		return transaction.ErrTransactionRequired
	}
	if session.Client() != a.db.Client() {
		return transaction.ErrBinding
	}
	bridge, ok := session.(mongo.XSession) //nolint:staticcheck // Pinned original active-session bridge.
	if !ok || bridge.ClientSession() == nil || !bridge.ClientSession().TransactionRunning() {
		return transaction.ErrTransactionRequired
	}
	generationID, err := strconv.ParseUint(f.Authority().SubjectID, 10, 64)
	if err != nil {
		return err
	}
	runID, err := strconv.ParseUint(f.Authority().RunID, 10, 64)
	if err != nil {
		return err
	}
	var g ReportGenerationPO
	if err := a.generations.FindOne(ctx, bson.M{"domain_id": generationID}, &g); err != nil {
		return err
	}
	if g.LatestRunID != runID || g.OutcomeID != f.Authority().OutcomeID || g.Status != string(generation.StatusFailed) {
		return transaction.ErrStaleExecution
	}
	var po InterpretationRunPO
	if err := a.runs.FindOne(ctx, bson.M{"domain_id": runID, "generation_id": generationID}, &po); err != nil {
		return err
	}
	run, err := a.runs.mapper.RunToDomain(&po)
	if err != nil {
		return err
	}
	if run.Status() != domainrun.StatusFailed || run.Attempt() != f.Authority().ExpectedAttempt {
		return transaction.ErrStaleExecution
	}
	if err := f.MatchDecision(run.RetryDecision(), f.DueAt().Truncate(time.Millisecond)); err != nil {
		return err
	}
	// Read-only snapshot validation is insufficient: these conditional writes
	// compete with new generation claims and changed/paused Run decisions.
	result, err := a.generations.UpdateOne(ctx, bson.M{"domain_id": generationID, "latest_run_id": runID, "version": g.Version}, bson.M{"$inc": bson.M{"retry_dispatch_guard": int64(1)}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return transaction.ErrStaleExecution
	}
	result, err = a.runs.UpdateOne(ctx, bson.M{"domain_id": runID, "generation_id": generationID, "status": po.Status, "retry_disposition": po.RetryDisposition, "retry_event_id": po.RetryEventID, "next_attempt_at": po.NextAttemptAt}, bson.M{"$inc": bson.M{"retry_dispatch_guard": int64(1)}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return transaction.ErrStaleExecution
	}
	if proof := run.RetryEventEvidence(); proof == nil || proof.Reference == nil || proof.EventID != f.Message().Input().ID || *proof.Reference != standard.ReferenceFromMessage(f.Message()) {
		return errors.Join(transaction.ErrBinding, model.ErrConflict)
	}
	return nil
}
