package evaluation

import (
	"context"
	"fmt"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"gorm.io/gorm"
)

type runRepository struct {
	db *gorm.DB
}

// NewRunRepository creates an evaluation run repository.
func NewRunRepository(db *gorm.DB) evaluationrun.Repository {
	return &runRepository{db: db}
}

func (r *runRepository) Save(ctx context.Context, run evalrun.EvaluationRun) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("evaluation run repository is not configured")
	}
	po := runToPO(run)
	var existing EvaluationRunPO
	err := r.db.WithContext(ctx).Where("run_id = ?", po.RunID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.db.WithContext(ctx).Create(po).Error
	}
	if err != nil {
		return err
	}
	po.ID = existing.ID
	return r.db.WithContext(ctx).Save(po).Error
}

func (r *runRepository) FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("evaluation run repository is not configured")
	}
	var po EvaluationRunPO
	err := r.db.WithContext(ctx).
		Where("assessment_id = ?", assessmentID).
		Order("attempt_no DESC, id DESC").
		First(&po).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	run := runFromPO(po)
	return &run, nil
}

func runToPO(run evalrun.EvaluationRun) *EvaluationRunPO {
	po := &EvaluationRunPO{
		RunID:        run.RunID.String(),
		AssessmentID: run.AssessmentID,
		AttemptNo:    uint(run.Attempt.Number),
		Status:       run.Attempt.Status.String(),
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
		Retryable:    run.Retryable(),
	}
	if run.TraceID != "" {
		traceID := run.TraceID
		po.TraceID = &traceID
	}
	if run.Failure != nil {
		code := run.Failure.Kind.String()
		message := run.Failure.Message
		po.ErrorCode = &code
		po.ErrorMessage = &message
		po.Retryable = run.Failure.Retryable
	}
	return po
}

func runFromPO(po EvaluationRunPO) evalrun.EvaluationRun {
	run := evalrun.EvaluationRun{
		RunID:        evalrun.ID(po.RunID),
		AssessmentID: po.AssessmentID,
		Attempt: evalrun.Attempt{
			Number: int(po.AttemptNo),
			Status: evalrun.Status(po.Status),
		},
		StartedAt:  po.StartedAt,
		FinishedAt: po.FinishedAt,
	}
	if po.TraceID != nil {
		run.TraceID = *po.TraceID
	}
	if po.ErrorCode != nil || po.ErrorMessage != nil {
		failure := evalrun.Failure{Retryable: po.Retryable}
		if po.ErrorCode != nil {
			failure.Kind = evalrun.FailureKind(*po.ErrorCode)
		}
		if po.ErrorMessage != nil {
			failure.Message = *po.ErrorMessage
		}
		run.Failure = &failure
	}
	return run
}

var _ evaluationrun.Repository = (*runRepository)(nil)
