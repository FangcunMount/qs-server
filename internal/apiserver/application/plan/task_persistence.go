package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// taskPersistence keeps a task terminal transition and its Enrollment close check
// inside one MySQL transaction. Tests and legacy assembly may omit the optional
// dependencies; production V2 assembly always supplies both.
type taskPersistence struct {
	tasks        domainplan.AssessmentTaskRepository
	enrollments  domainplan.EnrollmentRepository
	tx           apptransaction.Runner
	openedOutbox appEventing.ProfileBinding
}

// saveOpened persists the winning pending->opened transition and its reminder
// intent in the same MySQL transaction. An outer caller-owned transaction keeps
// both writes together; its normal Relay scan recovers if no wake is registered.
func (p taskPersistence) saveOpened(ctx context.Context, task *domainplan.AssessmentTask) (bool, error) {
	if p.openedOutbox.Stager == nil {
		return false, p.save(ctx, task, false)
	}
	if _, active := mysql.TxFromContext(ctx); !active && p.tx == nil {
		return false, fmt.Errorf("task opened outbox requires a MySQL transaction runner")
	}
	if task == nil || len(task.Events()) != 1 {
		return false, fmt.Errorf("task opened outbox requires exactly one opening event")
	}
	opened, ok := task.Events()[0].(domainplan.TaskOpenedEvent)
	if !ok || opened.EventType() != domainplan.EventTypeTaskOpened || opened.Data.OrgID != task.GetOrgID() {
		return false, fmt.Errorf("task opened outbox requires the original scoped opening event")
	}
	reminder := domainplan.NewTaskOpenedReminderRequestedEvent(opened, task.GetScheduleRevision())
	staged := []event.DomainEvent{reminder}
	if err := p.withinTransaction(ctx, func(txCtx context.Context) error {
		if err := p.tasks.Save(txCtx, task); err != nil {
			return err
		}
		return p.openedOutbox.Stager.Stage(txCtx, staged...)
	}); err != nil {
		return false, err
	}
	task.ClearEvents()
	if _, active := mysql.TxFromContext(ctx); !active && p.openedOutbox.PostCommit != nil {
		p.openedOutbox.PostCommit.AfterCommit(ctx, staged, time.Now())
	}
	return true, nil
}

func (p taskPersistence) save(ctx context.Context, task *domainplan.AssessmentTask, checkEnrollment bool) error {
	write := func(txCtx context.Context) error {
		if err := p.tasks.Save(txCtx, task); err != nil {
			return err
		}
		if !checkEnrollment || p.enrollments == nil || task.GetEnrollmentID().IsZero() {
			return nil
		}
		closedAt := time.Now()
		if completedAt := task.GetCompletedAt(); completedAt != nil {
			closedAt = *completedAt
		} else if expiredAt := task.GetExpiredAt(); expiredAt != nil {
			closedAt = *expiredAt
		} else if canceledAt := task.GetCanceledAt(); canceledAt != nil {
			closedAt = *canceledAt
		}
		if _, err := p.enrollments.CloseIfAllTasksTerminal(txCtx, task.GetEnrollmentID(), closedAt); err != nil {
			return err
		}
		return nil
	}
	return p.withinTransaction(ctx, write)
}

func (p taskPersistence) withinTransaction(ctx context.Context, write func(context.Context) error) error {
	if _, active := mysql.TxFromContext(ctx); active || p.tx == nil {
		return write(ctx)
	}
	return p.tx.WithinTransaction(ctx, write)
}
