package plan

import (
	"context"
	"fmt"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
)

// taskPersistence keeps a task terminal transition and its Enrollment close check
// inside one MySQL transaction. All task services require both dependencies.
type taskPersistence struct {
	tasks          domainplan.AssessmentTaskRepository
	enrollments    domainplan.EnrollmentRepository
	tx             apptransaction.Runner
	openingOptions TaskOpeningOptions
}

// saveOpened persists the winning pending->opened transition and its reminder
// intent in the same MySQL transaction. An outer caller-owned transaction keeps
// both writes together; its normal Relay scan recovers if no wake is registered.
func (p taskPersistence) saveOpened(ctx context.Context, task *domainplan.AssessmentTask) (bool, error) {
	if !p.openingOptions.ReminderEnabled {
		return false, p.save(ctx, task, false)
	}
	if err := p.openingOptions.validate(); err != nil {
		return false, err
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
		return p.openingOptions.ReminderOutbox.Stager.Stage(txCtx, staged...)
	}); err != nil {
		return false, err
	}
	task.ClearEvents()
	if _, active := mysql.TxFromContext(ctx); !active && p.openingOptions.ReminderOutbox.PostCommit != nil {
		p.openingOptions.ReminderOutbox.PostCommit.AfterCommit(ctx, staged, time.Now())
	}
	return true, nil
}

func (p taskPersistence) save(ctx context.Context, task *domainplan.AssessmentTask, checkEnrollment bool) error {
	write := func(txCtx context.Context) error {
		if err := p.tasks.Save(txCtx, task); err != nil {
			return err
		}
		if !checkEnrollment || task.GetEnrollmentID().IsZero() {
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
	if _, active := mysql.TxFromContext(ctx); active {
		return write(ctx)
	}
	return p.tx.WithinTransaction(ctx, write)
}
