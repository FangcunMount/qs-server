package plan

import (
	"context"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// taskPersistence keeps a task terminal transition and its Enrollment close check
// inside one MySQL transaction. Tests and legacy assembly may omit the optional
// dependencies; production V2 assembly always supplies both.
type taskPersistence struct {
	tasks       domainplan.AssessmentTaskRepository
	enrollments domainplan.EnrollmentRepository
	tx          apptransaction.Runner
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
