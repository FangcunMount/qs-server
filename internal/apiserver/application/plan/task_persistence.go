package plan

import (
	"context"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
)

// taskPersistence keeps a task terminal transition and its Enrollment close check
// inside one MySQL transaction. Tests and legacy assembly may omit the optional
// dependencies; production V2 assembly always supplies both.
type taskPersistence struct {
	tasks       domainplan.AssessmentTaskRepository
	enrollments domainplan.EnrollmentRepository
	tx          apptransaction.Runner
	recorder    stageport.Recorder
}

type historicalTaskStagePayload struct {
	TaskID       string `json:"task_id"`
	EnrollmentID string `json:"enrollment_id"`
	AssessmentID string `json:"assessment_id,omitempty"`
}

func (p taskPersistence) save(ctx context.Context, task *domainplan.AssessmentTask, checkEnrollment bool) error {
	write := func(txCtx context.Context) error {
		if err := p.tasks.Save(txCtx, task); err != nil {
			return err
		}
		if !checkEnrollment || p.enrollments == nil || task.GetEnrollmentID().IsZero() {
			return p.recordHistorical(ctx, txCtx, task)
		}
		closedAt := time.Now()
		if completedAt := task.GetCompletedAt(); completedAt != nil {
			closedAt = *completedAt
		}
		if _, err := p.enrollments.CloseIfAllTasksTerminal(txCtx, task.GetEnrollmentID(), closedAt); err != nil {
			return err
		}
		return p.recordHistorical(ctx, txCtx, task)
	}
	return p.withinTransaction(ctx, write)
}

func (p taskPersistence) withinTransaction(ctx context.Context, write func(context.Context) error) error {
	if _, active := mysql.TxFromContext(ctx); active || p.tx == nil {
		return write(ctx)
	}
	return p.tx.WithinTransaction(ctx, write)
}

func (p taskPersistence) recordHistorical(_ context.Context, txCtx context.Context, task *domainplan.AssessmentTask) error {
	if p.recorder == nil || task == nil {
		return nil
	}
	stage := ""
	businessAt := time.Time{}
	if completedAt := task.GetCompletedAt(); completedAt != nil {
		stage, businessAt = stageport.StageTaskComplete, *completedAt
	} else if openedAt := task.GetOpenAt(); openedAt != nil {
		stage, businessAt = stageport.StageTaskOpen, *openedAt
	}
	if stage == "" {
		return nil
	}
	assessmentID := ""
	if value := task.GetAssessmentID(); value != nil {
		assessmentID = value.String()
	}
	_, err := stageport.CompleteStage(txCtx, p.recorder, stageport.Completion{Stage: stage, BusinessAt: businessAt, ResourceType: "plan_task", ResourceID: task.GetID().String(), Payload: historicalTaskStagePayload{
		TaskID: task.GetID().String(), EnrollmentID: task.GetEnrollmentID().String(), AssessmentID: assessmentID,
	}})
	return err
}
