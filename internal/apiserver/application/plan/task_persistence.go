package plan

import (
	"context"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
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
		_, err := p.enrollments.CloseIfAllTasksTerminal(txCtx, task.GetEnrollmentID(), time.Now())
		return err
	}
	if p.tx == nil {
		return write(ctx)
	}
	return p.tx.WithinTransaction(ctx, write)
}
