// Package evaluationfailure publishes immediate failure by transaction while
// persisting new future retry authorizations separately from publish Outbox.
package evaluationfailure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	schedulemodel "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	schedulestore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationtransaction"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/reliable-messaging/message"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

type Committer struct {
	boundary  eventport.Committer
	resolver  catalog.TopicResolver
	writer    assessment.Repository
	runWriter evaluationrun.Repository
	primary   assessment.Repository
	runs      evaluationrun.ExactReader
	latest    evaluationrun.Repository
	schedules *schedulestore.Store
}

var _ port.FailureCommitter = (*Committer)(nil)

func New(db *gorm.DB, owner *boundary.Committer, resolver catalog.TopicResolver, writer assessment.Repository, runWriter evaluationrun.Repository) (*Committer, error) {
	if owner == nil || !owner.UsesPrimary(db) || resolver == nil || writer == nil || runWriter == nil {
		return nil, transaction.ErrBinding
	}
	schedules, err := schedulestore.NewWithPlanReader(db, owner.Binding(), owner.Artifact)
	if err != nil {
		return nil, err
	}
	return &Committer{schedules: schedules, boundary: owner, resolver: resolver, writer: writer, runWriter: runWriter, primary: repository.NewAssessmentRepository(db), runs: checkpoint.NewExactRunReader(db), latest: checkpoint.NewRunRepository(db)}, nil
}

func (c *Committer) CommitFailure(ctx context.Context, q port.FailureRequest) (a *assessment.Assessment, r *evalrun.EvaluationRun, commitErr error) {
	defer func() {
		if commitErr != nil {
			commitErr = errors.Join(eventport.ErrUnresolved, commitErr)
		}
	}()
	request, err := freeze(q, c.resolver)
	if err != nil {
		return nil, nil, err
	}
	reservation, err := c.boundary.Reserve(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	f, err := c.readExecution(reservation.Execution)
	if err != nil {
		return nil, nil, err
	}
	if err := reservation.Execution.Plan.MatchRequest("qs.mysql", fmt.Sprintf("org:%d", f.Assessment.OrgID), request.Key, request.InputHash); err != nil {
		return nil, nil, err
	}
	if !reservation.Acquired {
		return c.recover(ctx, reservation.Execution, f)
	}
	raw, err := c.boundary.Artifact(reservation.Execution.Plan)
	if err != nil || !bytes.Equal(raw, request.Artifact) {
		return nil, nil, transaction.ErrInvalidPlan
	}
	prepared := repository.NewAssessmentMapper().ToDomain(f.Assessment)
	run := evalrun.Reconstruct(f.Run)
	// Future authorization is frozen before the root and is not a half branch.
	// Old winning V1 artifacts retain their original Outbox responsibility.
	var future message.Message
	var retryPlan schedulemodel.Frozen
	if f.Future != nil && f.Version == 2 {
		retryPlan, err = f.retryPlan()
		if err != nil {
			return nil, nil, err
		}
	}
	if f.Future != nil && f.Version == 1 {
		future, err = message.New(*f.Future)
		if err != nil {
			return nil, nil, err
		}
	}
	result, err := c.boundary.Commit(ctx, reservation.Execution, func(txCtx context.Context) error {
		if retryPlan.Valid() {
			winner, err := c.schedules.SaveFrozen(txCtx, retryPlan)
			if err != nil {
				return err
			}
			if winner.Digest() != retryPlan.Digest() {
				return schedulemodel.ErrConflict
			}
		}
		if err := c.writer.Save(txCtx, prepared); err != nil {
			return err
		}
		if err := c.runWriter.SaveClaimed(txCtx, run); err != nil {
			return err
		}
		if f.Future != nil && f.Version == 1 {
			tx, err := hostmysql.RequireTx(txCtx)
			if err != nil {
				return err
			}
			appender, err := sdkmysql.BindGORM(tx)
			if err != nil {
				return err
			}
			return appender.Append(txCtx, future, f.Due)
		}
		return nil
	})
	if result.Local != transaction.LocalCommitted {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, eventport.ErrUnresolved
	}
	prepared.ClearEvents()
	return prepared, &run, nil
}

func (c *Committer) readExecution(e transaction.Execution) (frozenFailure, error) {
	raw, err := c.boundary.Artifact(e.Plan)
	if err != nil {
		return frozenFailure{}, err
	}
	f, err := read(raw)
	if err != nil {
		return f, err
	}
	in := e.Plan.Input()
	hash, err := f.inputHash()
	if err != nil || in.Owner != "qs.mysql" || in.Scope != fmt.Sprintf("org:%d", f.Assessment.OrgID) || in.IdempotencyKey != failureKey(f.Run.RunID) || in.InputHash != hash || len(in.Branches) != 1 {
		return f, transaction.ErrInvalidPlan
	}
	b := in.Branches[0]
	consumer := b.Message.Input()
	consumer.Payload = bytes.Clone(b.Wire)
	m, err := message.New(consumer)
	if err != nil {
		return f, err
	}
	ref := standard.ReferenceFromMessage(m)
	envelope, err := standard.VerifyReference(consumer, ref.Fingerprint, ref)
	if err != nil {
		return f, err
	}
	var body assessment.EvaluationFailedData
	if json.Unmarshal(envelope.Data, &body) != nil || consumer.EventType != assessment.EventTypeFailed || envelope.AggregateType != assessment.AggregateType || envelope.AggregateID != f.Assessment.ID.String() || body.OrgID != f.Assessment.OrgID || body.AssessmentID != int64(f.Assessment.ID.Uint64()) || body.TesteeID != f.Assessment.TesteeID || body.Reason != *f.Assessment.FailureReason || !body.FailedAt.Equal(*f.Assessment.FailedAt) {
		return f, transaction.ErrInvalidPlan
	}
	return f, nil
}

func (c *Committer) recover(ctx context.Context, e transaction.Execution, f frozenFailure) (*assessment.Assessment, *evalrun.EvaluationRun, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if c.boundary.Confirm(ctx, e) != transaction.LocalCommitted {
		return nil, nil, eventport.ErrUnresolved
	}
	if f.Future != nil && f.Version == 2 {
		plan, err := f.retryPlan()
		if err != nil {
			return nil, nil, eventport.ErrUnresolved
		}
		record, err := c.schedules.Find(ctx, plan)
		if err != nil || record.Validate() != nil || record.Plan.Digest() != plan.Digest() {
			return nil, nil, errors.Join(eventport.ErrUnresolved, err)
		}
	}
	a, err := c.primary.FindByID(ctx, f.Assessment.ID)
	if err != nil {
		return nil, nil, err
	}
	run, err := c.runs.FindRunByID(ctx, f.Run.RunID)
	if err != nil {
		return nil, nil, err
	}
	if a == nil || !a.Status().IsFailed() || a.FailedAt() == nil || !a.FailedAt().Equal(*f.Assessment.FailedAt) || a.FailureReason() == nil || *a.FailureReason() != *f.Assessment.FailureReason || run == nil || !sameRun(snapshot(*run), f.Run) {
		return nil, nil, eventport.ErrUnresolved
	}
	a.ClearEvents()
	return a, run, nil
}

func sameRun(a, b evalrun.ReconstructInput) bool {
	canonical := func(in evalrun.ReconstructInput) ([]byte, error) {
		x := snapshot(evalrun.Reconstruct(in))
		x.StartedAt = x.StartedAt.UTC().Truncate(time.Millisecond)
		for _, p := range []**time.Time{&x.FinishedAt, &x.LeaseExpiresAt} {
			if *p != nil {
				v := (*p).UTC().Truncate(time.Millisecond)
				*p = &v
			}
		}
		if x.RetryDecision != nil && x.RetryDecision.NextAttemptAt != nil {
			v := x.RetryDecision.NextAttemptAt.UTC().Truncate(time.Millisecond)
			x.RetryDecision.NextAttemptAt = &v
		}
		return json.Marshal(x)
	}
	first, err := canonical(a)
	if err != nil {
		return false
	}
	second, err := canonical(b)
	return err == nil && bytes.Equal(first, second)
}

func (c *Committer) RecoverPending(ctx context.Context, a *assessment.Assessment) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if a == nil {
		return true, eventport.ErrUnresolved
	}
	run, err := c.latest.FindLatestByAssessmentID(ctx, a.ID().Uint64())
	if err != nil {
		return true, errors.Join(eventport.ErrUnresolved, err)
	}
	// A committed failed Run already carries its original retry decision. Its
	// next attempt stays governed by the existing claim/authorization protocol.
	if run == nil || run.Attempt().Status != evalrun.StatusRunning {
		return false, nil
	}
	r, err := c.boundary.Find(ctx, fmt.Sprintf("org:%d", a.OrgID()), failureKey(run.ID()))
	if errors.Is(err, transaction.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return true, errors.Join(eventport.ErrUnresolved, err)
	}
	f, err := c.readExecution(r.Execution)
	if err != nil || f.Run.RunID != run.ID() || f.Assessment.ID != a.ID() {
		return true, eventport.ErrUnresolved
	}
	stored, _, err := c.recover(ctx, r.Execution, f)
	if err != nil {
		return true, err
	}
	*a = *stored
	return true, nil
}
