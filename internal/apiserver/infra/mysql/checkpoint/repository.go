package checkpoint

import (
	"context"
	"errors"
	"fmt"
	"time"

	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	domainstatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
)

func (r *Repository) Claim(ctx context.Context, request evaluationrun.ClaimRequest) (evaluationrun.ClaimResult, error) {
	if r == nil || r.db == nil {
		return evaluationrun.ClaimResult{}, fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if request.AssessmentID == 0 || request.Token == "" || request.ClaimedAt.IsZero() || !request.LeaseUntil.After(request.ClaimedAt) {
		return evaluationrun.ClaimResult{}, fmt.Errorf("invalid evaluation run claim request")
	}
	result, err := r.claimOnce(ctx, request)
	if err == nil || !isDuplicateKey(err) {
		return result, err
	}
	// A concurrent first-attempt insert won the unique key. Re-read the new
	// latest row; its fresh lease will make this caller a duplicate skip.
	return r.claimOnce(ctx, request)
}

func (r *Repository) claimOnce(ctx context.Context, request evaluationrun.ClaimRequest) (evaluationrun.ClaimResult, error) {
	result := evaluationrun.ClaimResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var latest RuntimeCheckpointPO
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("scope = ? AND assessment_id = ? AND deleted_at IS NULL", scopeEvaluationRun, request.AssessmentID).
			Order("attempt_no DESC, id DESC").
			First(&latest).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			run := evalrun.NewEvaluationRun(request.AssessmentID)
			run.TraceID = request.TraceID
			if err := run.Claim(request.Token, request.ClaimedAt, request.LeaseUntil); err != nil {
				return err
			}
			if err := tx.Create(runToPO(run)).Error; err != nil {
				return err
			}
			result = evaluationrun.ClaimResult{Run: run, Claimed: true}
			return nil
		}
		if err != nil {
			return err
		}

		run := runFromPO(latest)
		switch run.Attempt.Status {
		case evalrun.StatusPending:
			// Claim the existing attempt.
		case evalrun.StatusRunning:
			if run.HasActiveLease(request.ClaimedAt) {
				result = evaluationrun.ClaimResult{Run: run}
				return nil
			}
		case evalrun.StatusFailed:
			if !run.Retryable() {
				result = evaluationrun.ClaimResult{Run: run}
				return nil
			}
			run = evalrun.NextEvaluationRun(run)
		case evalrun.StatusSucceeded:
			result = evaluationrun.ClaimResult{Run: run}
			return nil
		default:
			return fmt.Errorf("latest evaluation run %s has unknown status %q", run.RunID, run.Attempt.Status)
		}
		run.TraceID = request.TraceID
		if err := run.Claim(request.Token, request.ClaimedAt, request.LeaseUntil); err != nil {
			return err
		}
		po := runToPO(run)
		if po.ResourceID != latest.ResourceID || po.AttemptNo != latest.AttemptNo {
			if err := tx.Create(po).Error; err != nil {
				return err
			}
		} else {
			updates := claimUpdates(po)
			updated := tx.Model(&RuntimeCheckpointPO{}).
				Where("id = ? AND status = ? AND deleted_at IS NULL", latest.ID, latest.Status).
				Updates(updates)
			if updated.Error != nil {
				return updated.Error
			}
			if updated.RowsAffected != 1 {
				return evaluationrun.ErrClaimLost
			}
		}
		result = evaluationrun.ClaimResult{Run: run, Claimed: true}
		return nil
	})
	return result, err
}

func (r *Repository) SaveClaimed(ctx context.Context, run evalrun.EvaluationRun) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if run.ClaimToken == "" {
		return evaluationrun.ErrClaimLost
	}
	po := runToPO(run)
	updates := claimUpdates(po)
	updates["updated_at"] = time.Now()
	result := checkpointDB(ctx, r.db).Model(&RuntimeCheckpointPO{}).
		Where("scope = ? AND resource_id = ? AND attempt_no = ? AND claim_token = ? AND status = ? AND deleted_at IS NULL",
			scopeEvaluationRun, po.ResourceID, po.AttemptNo, run.ClaimToken, evalrun.StatusRunning.String()).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return evaluationrun.ErrClaimLost
	}
	return nil
}

func claimUpdates(po *RuntimeCheckpointPO) map[string]interface{} {
	return map[string]interface{}{
		"status":             po.Status,
		"started_at":         po.StartedAt,
		"finished_at":        po.FinishedAt,
		"error_code":         po.ErrorCode,
		"error_message":      po.ErrorMessage,
		"retryable":          po.Retryable,
		"trace_id":           po.TraceID,
		"input_snapshot_ref": po.InputSnapshotRef,
		"claim_token":        po.ClaimToken,
		"lease_expires_at":   po.LeaseExpiresAt,
	}
}

func isDuplicateKey(err error) bool {
	var mysqlErr *drivermysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
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
		Status:     analyticsStatusToCheckpoint(domainstatistics.AnalyticsProjectorCheckpointStatusProcessing),
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
	return analyticsStatusFromCheckpoint(existing.Status), nil
}

func (r *Repository) MarkAnalyticsProjectorCheckpointStatus(ctx context.Context, eventID, status string) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("runtime checkpoint repository is not configured")
	}
	if eventID == "" {
		return nil
	}
	unified := analyticsStatusToCheckpoint(status)
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

func analyticsStatusToCheckpoint(status string) string {
	switch status {
	case domainstatistics.AnalyticsProjectorCheckpointStatusProcessing:
		return evalrun.StatusRunning.String()
	case domainstatistics.AnalyticsProjectorCheckpointStatusCompleted:
		return evalrun.StatusSucceeded.String()
	case domainstatistics.AnalyticsProjectorCheckpointStatusPending:
		return evalrun.StatusPending.String()
	default:
		return status
	}
}

func analyticsStatusFromCheckpoint(status string) string {
	switch evalrun.Status(status) {
	case evalrun.StatusRunning:
		return domainstatistics.AnalyticsProjectorCheckpointStatusProcessing
	case evalrun.StatusSucceeded:
		return domainstatistics.AnalyticsProjectorCheckpointStatusCompleted
	case evalrun.StatusPending:
		return domainstatistics.AnalyticsProjectorCheckpointStatusPending
	default:
		return status
	}
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
	if run.ClaimToken != "" {
		claimToken := run.ClaimToken
		po.ClaimToken = &claimToken
	}
	po.LeaseExpiresAt = run.LeaseExpiresAt
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
	if po.ClaimToken != nil {
		run.ClaimToken = *po.ClaimToken
	}
	run.LeaseExpiresAt = po.LeaseExpiresAt
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

func derefUint64(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}
