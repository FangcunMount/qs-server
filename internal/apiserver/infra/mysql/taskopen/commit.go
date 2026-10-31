package taskopen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	boundary "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/tasktransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"gorm.io/gorm"
)

type SnapshotWriter interface {
	SaveOpenedSnapshot(context.Context, *repository.AssessmentTaskPO, time.Time, uint64) error
}

// Committer borrows the shared qs.mysql owner; the host retains runtime and
// database ownership. Constructing it does not install schema or connect MQ.
type Committer struct {
	boundary eventport.Committer
	writer   SnapshotWriter
	primary  domain.AssessmentTaskRepository
}

var _ port.Opener = (*Committer)(nil)

func New(db *gorm.DB, owner *boundary.Committer, resolver catalog.TopicResolver, writer SnapshotWriter, opts ...hostmysql.BaseRepositoryOptions) (*Committer, error) {
	if owner == nil || !owner.UsesPrimary(db) || resolver == nil || writer == nil {
		return nil, transaction.ErrBinding
	}
	topic, ok := resolver.GetTopicForEvent(domain.EventTypeTaskOpenedReminderRequested)
	if !ok || !owner.HasTopic(topic) {
		return nil, transaction.ErrBinding
	}
	return &Committer{boundary: owner, writer: writer, primary: repository.NewTaskRepository(db, opts...)}, nil
}

func (c *Committer) readExecution(e transaction.Execution) (frozenOpening, error) {
	raw, err := c.boundary.Artifact(e.Plan)
	if err != nil {
		return frozenOpening{}, err
	}
	f, err := read(raw)
	if err != nil {
		return f, err
	}
	in := e.Plan.Input()
	hash, err := f.inputHash()
	if err != nil || in.Owner != "qs.mysql" || in.Scope != fmt.Sprintf("org:%d", f.Task.OrgID) || in.IdempotencyKey != openingKey(f.Task.ID.Uint64(), f.Task.ScheduleRevision) || in.InputHash != hash || len(in.Branches) != 1 {
		return f, transaction.ErrInvalidPlan
	}
	b := in.Branches[0]
	consumer := b.Message.Input()
	consumer.Payload = bytes.Clone(b.Wire)
	m, err := message.New(consumer)
	if err != nil {
		return f, err
	}
	ref := eventmessage.ReferenceFromMessage(m)
	envelope, err := eventmessage.VerifyReference(consumer, ref.Fingerprint, ref)
	if err != nil {
		return f, err
	}
	if consumer.EventType != domain.EventTypeTaskOpenedReminderRequested || envelope.AggregateType != domain.AggregateTypeTask || envelope.AggregateID != f.Task.ID.String() || !bytes.Equal(envelope.Data, f.Reminder) {
		return f, transaction.ErrBinding
	}
	return f, nil
}

func (c *Committer) CommitOpened(ctx context.Context, q port.OpenRequest) (*domain.AssessmentTask, error) {
	if _, nested := hostmysql.TxFromContext(ctx); nested {
		return nil, transaction.ErrTransactionRequired
	}
	request, err := freeze(q.Candidate, middleware.GetUserIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	r, err := c.boundary.Reserve(ctx, request)
	if err != nil {
		return nil, errors.Join(eventport.ErrUnresolved, err)
	}
	f, err := c.readExecution(r.Execution)
	if err != nil {
		return nil, errors.Join(eventport.ErrUnresolved, err)
	}
	if err := r.Execution.Plan.MatchRequest("qs.mysql", fmt.Sprintf("org:%d", f.Task.OrgID), request.Key, request.InputHash); err != nil {
		return nil, err
	}
	if !r.Acquired {
		return c.recover(ctx, r.Execution, f)
	}
	raw, err := c.boundary.Artifact(r.Execution.Plan)
	if err != nil || !bytes.Equal(raw, request.Artifact) {
		return nil, transaction.ErrInvalidPlan
	}
	prepared := repository.NewTaskMapper().ToDomain(&f.Task)
	result, err := c.boundary.Commit(ctx, r.Execution, func(local context.Context) error {
		if q.Guard != nil {
			if err := q.Guard(local, prepared); err != nil {
				return err
			}
		}
		return c.writer.SaveOpenedSnapshot(local, &f.Task, f.UpdatedAt, f.UpdatedBy)
	})
	if result.Local != transaction.LocalCommitted {
		return nil, errors.Join(eventport.ErrUnresolved, err)
	}
	prepared.ClearEvents()
	return prepared, nil
}

func (c *Committer) recover(ctx context.Context, e transaction.Execution, f frozenOpening) (*domain.AssessmentTask, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if c.boundary.Confirm(ctx, e) != transaction.LocalCommitted {
		return nil, eventport.ErrUnresolved
	}
	current, err := c.primary.FindByID(ctx, domain.AssessmentTaskID(f.Task.ID))
	if err != nil {
		return nil, errors.Join(eventport.ErrUnresolved, err)
	}
	if current == nil || current.GetOrgID() != f.Task.OrgID || current.GetScheduleRevision() != f.Task.ScheduleRevision || current.GetOpenAt() == nil || !current.GetOpenAt().Equal(*f.Task.OpenAt) || current.GetEntryURL() != f.Task.EntryURL {
		return nil, eventport.ErrUnresolved
	}
	po := repository.NewTaskMapper().ToPO(current)
	hash, err := (frozenOpening{Task: *po}).inputHash()
	want, other := f.inputHash()
	if err != nil || other != nil || hash != want || !sameOptionalClock(po.ExpireAt, f.Task.ExpireAt) {
		return nil, eventport.ErrUnresolved
	}
	current.ClearEvents()
	return current, nil
}

func sameOptionalClock(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (c *Committer) RecoverOpened(ctx context.Context, task *domain.AssessmentTask) (*domain.AssessmentTask, bool, error) {
	if task == nil || task.GetID().IsZero() || task.GetOrgID() <= 0 {
		return nil, true, transaction.ErrInvalidPlan
	}
	if _, nested := hostmysql.TxFromContext(ctx); nested {
		return nil, true, transaction.ErrTransactionRequired
	}
	r, err := c.boundary.Find(ctx, fmt.Sprintf("org:%d", task.GetOrgID()), openingKey(task.GetID().Uint64(), task.GetScheduleRevision()))
	if errors.Is(err, transaction.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, errors.Join(eventport.ErrUnresolved, err)
	}
	f, err := c.readExecution(r.Execution)
	if err != nil {
		return nil, true, errors.Join(eventport.ErrUnresolved, err)
	}
	if f.Task.ID != task.GetID() || f.Task.OrgID != task.GetOrgID() || f.Task.ScheduleRevision != task.GetScheduleRevision() {
		return nil, true, transaction.ErrBinding
	}
	result, err := c.recover(ctx, r.Execution, f)
	return result, true, err
}
