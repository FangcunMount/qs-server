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

func (r *runRepository) ListByAssessmentID(ctx context.Context, assessmentID uint64, limit int) ([]evalrun.EvaluationRun, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("evaluation run repository is not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []EvaluationRunPO
	err := r.db.WithContext(ctx).
		Where("assessment_id = ?", assessmentID).
		Order("attempt_no DESC, id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	runs := make([]evalrun.EvaluationRun, 0, len(rows))
	for _, po := range rows {
		runs = append(runs, runFromPO(po))
	}
	return runs, nil
}

func (r *runRepository) ListRetryableFailed(ctx context.Context, params evaluationrun.ListRetryableFailedParams) (*evaluationrun.ListRetryableFailedResult, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("evaluation run repository is not configured")
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	query := r.db.WithContext(ctx).
		Table("evaluation_run AS er").
		Select("er.*, a.org_id").
		Joins("INNER JOIN assessment AS a ON a.id = er.assessment_id").
		Where("er.status = ? AND er.retryable = ?", evalrun.StatusFailed.String(), true).
		Where("a.org_id = ?", params.OrgID)
	if params.Cursor > 0 {
		query = query.Where("er.id < ?", params.Cursor)
	}
	query = query.Order("er.id DESC").Limit(limit + 1)

	type retryableFailedRow struct {
		EvaluationRunPO
		OrgID int64 `gorm:"column:org_id"`
	}
	var rows []retryableFailedRow
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}

	nextCursor := uint64(0)
	if len(rows) > limit {
		nextCursor = rows[limit-1].ID
		rows = rows[:limit]
	}
	items := make([]evaluationrun.RetryableFailedRun, 0, len(rows))
	for _, row := range rows {
		items = append(items, evaluationrun.RetryableFailedRun{
			Run:   runFromPO(row.EvaluationRunPO),
			OrgID: row.OrgID,
		})
	}
	return &evaluationrun.ListRetryableFailedResult{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
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
	if run.InputSnapshotRef != "" {
		inputSnapshotRef := run.InputSnapshotRef
		po.InputSnapshotRef = &inputSnapshotRef
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
	if po.InputSnapshotRef != nil {
		run.InputSnapshotRef = *po.InputSnapshotRef
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
