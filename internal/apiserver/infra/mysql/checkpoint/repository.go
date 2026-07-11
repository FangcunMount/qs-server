package checkpoint

import (
	"context"
	"fmt"
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func NewRunRepository(db *gorm.DB) evaluationrun.Repository {
	return NewRepository(db)
}

var (
	_ evaluationrun.Repository = (*Repository)(nil)
	_ evalrun.CheckpointSeam   = (*Repository)(nil)
)

func (r *Repository) Save(ctx context.Context, run evalrun.EvaluationRun) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("runtime checkpoint repository is not configured")
	}
	po := runToPO(run)
	var existing RuntimeCheckpointPO
	db := checkpointDB(ctx, r.db)
	err := db.
		Where("scope = ? AND resource_id = ? AND attempt_no = ? AND deleted_at IS NULL",
			scopeEvaluationRun, po.ResourceID, po.AttemptNo).
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return db.Create(po).Error
	}
	if err != nil {
		return err
	}
	po.ID = existing.ID
	return db.Save(po).Error
}

func checkpointDB(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := mysql.TxFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

func (r *Repository) FindLatestByAssessmentID(ctx context.Context, assessmentID uint64) (*evalrun.EvaluationRun, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("runtime checkpoint repository is not configured")
	}
	var po RuntimeCheckpointPO
	err := r.db.WithContext(ctx).
		Where("scope = ? AND assessment_id = ? AND deleted_at IS NULL", scopeEvaluationRun, assessmentID).
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

func (r *Repository) ListByAssessmentID(ctx context.Context, assessmentID uint64, limit int) ([]evalrun.EvaluationRun, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []RuntimeCheckpointPO
	err := r.db.WithContext(ctx).
		Where("scope = ? AND assessment_id = ? AND deleted_at IS NULL", scopeEvaluationRun, assessmentID).
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

func (r *Repository) ListRetryableFailed(ctx context.Context, params evaluationrun.ListRetryableFailedParams) (*evaluationrun.ListRetryableFailedResult, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("runtime checkpoint repository is not configured")
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 50
	}
	query := r.db.WithContext(ctx).
		Table("runtime_checkpoint AS rc").
		Select("rc.*, a.org_id").
		Joins("INNER JOIN assessment AS a ON a.id = rc.assessment_id").
		Where("rc.scope = ? AND rc.status = ? AND rc.retryable = ? AND rc.deleted_at IS NULL",
			scopeEvaluationRun, evalrun.StatusFailed.String(), true).
		Where("a.org_id = ?", params.OrgID)
	if params.Cursor > 0 {
		query = query.Where("rc.id < ?", params.Cursor)
	}
	query = query.Order("rc.id DESC").Limit(limit + 1)

	type retryableFailedRow struct {
		RuntimeCheckpointPO
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
			Run:   runFromPO(row.RuntimeCheckpointPO),
			OrgID: row.OrgID,
		})
	}
	return &evaluationrun.ListRetryableFailedResult{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

func (r *Repository) Begin(scope evalrun.CheckpointScope, resourceID string, attemptNo int) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if resourceID == "" {
		return false, nil
	}
	now := time.Now()
	po := &RuntimeCheckpointPO{
		Scope:      string(scope),
		ResourceID: resourceID,
		AttemptNo:  uint(attemptNo),
		Status:     evalrun.StatusRunning.String(),
		StartedAt:  now,
	}
	if scope == evalrun.CheckpointScopeEvaluationRun {
		assessmentID, err := parseAssessmentIDFromRunID(resourceID, attemptNo)
		if err == nil {
			po.AssessmentID = &assessmentID
		}
	}
	if err := r.db.Create(po).Error; err == nil {
		return true, nil
	}
	var existing RuntimeCheckpointPO
	if err := r.db.
		Where("scope = ? AND resource_id = ? AND attempt_no = ? AND deleted_at IS NULL", po.Scope, po.ResourceID, po.AttemptNo).
		First(&existing).Error; err != nil {
		return false, err
	}
	return false, nil
}

func (r *Repository) Complete(
	scope evalrun.CheckpointScope,
	resourceID string,
	attemptNo int,
	status string,
	retryable bool,
	errCode, errMsg string,
) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if resourceID == "" {
		return nil
	}
	updates := map[string]interface{}{
		"status":    status,
		"retryable": retryable,
	}
	now := time.Now()
	updates["finished_at"] = now
	if errCode != "" {
		updates["error_code"] = errCode
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	return r.db.Model(&RuntimeCheckpointPO{}).
		Where("scope = ? AND resource_id = ? AND attempt_no = ? AND deleted_at IS NULL",
			string(scope), resourceID, attemptNo).
		Updates(updates).Error
}

func (r *Repository) TryBeginAnalyticsProjectorCheckpoint(ctx context.Context, eventID, eventType string) (string, error) {
	if r == nil || r.db == nil {
		return "", fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if eventID == "" {
		return "", nil
	}
	now := time.Now()
	eventTypeCopy := eventType
	po := &RuntimeCheckpointPO{
		Scope:      scopeAnalyticsProjector,
		ResourceID: eventID,
		AttemptNo:  1,
		EventType:  &eventTypeCopy,
		Status:     evalrun.UnifiedStatusForAnalytics(evalrun.AnalyticsProjectorCheckpointStatusProcessing),
		StartedAt:  now,
	}
	if err := r.db.WithContext(ctx).Create(po).Error; err == nil {
		return "", nil
	}
	var existing RuntimeCheckpointPO
	if err := r.db.WithContext(ctx).
		Where("scope = ? AND resource_id = ? AND attempt_no = ? AND deleted_at IS NULL",
			scopeAnalyticsProjector, eventID, 1).
		First(&existing).Error; err != nil {
		return "", err
	}
	return evalrun.AnalyticsStatusFromUnified(existing.Status), nil
}

func (r *Repository) MarkAnalyticsProjectorCheckpointStatus(ctx context.Context, eventID, status string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if eventID == "" {
		return nil
	}
	unified := evalrun.UnifiedStatusForAnalytics(status)
	updates := map[string]interface{}{
		"status":      unified,
		"finished_at": time.Now(),
	}
	return r.db.WithContext(ctx).
		Model(&RuntimeCheckpointPO{}).
		Where("scope = ? AND resource_id = ? AND attempt_no = ? AND deleted_at IS NULL",
			scopeAnalyticsProjector, eventID, 1).
		Updates(updates).Error
}

func runToPO(run evalrun.EvaluationRun) *RuntimeCheckpointPO {
	assessmentID := run.AssessmentID
	po := &RuntimeCheckpointPO{
		Scope:        scopeEvaluationRun,
		ResourceID:   run.RunID.String(),
		AttemptNo:    uint(run.Attempt.Number),
		AssessmentID: &assessmentID,
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

func runFromPO(po RuntimeCheckpointPO) evalrun.EvaluationRun {
	run := evalrun.EvaluationRun{
		RunID:        evalrun.ID(po.ResourceID),
		AssessmentID: derefUint64(po.AssessmentID),
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

func parseAssessmentIDFromRunID(resourceID string, _ int) (uint64, error) {
	var assessmentID uint64
	var attempt int
	if _, err := fmt.Sscanf(resourceID, "%d:%d", &assessmentID, &attempt); err != nil {
		return 0, fmt.Errorf("parse run id %q: %w", resourceID, err)
	}
	return assessmentID, nil
}

func derefUint64(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}
