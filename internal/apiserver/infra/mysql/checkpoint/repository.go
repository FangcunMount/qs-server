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
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
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
	_ evaluationrun.Repository         = (*Repository)(nil)
	_ evaluationrun.RetryAuthorizer    = (*Repository)(nil)
	_ evaluationrun.ExpiredLeaseReader = (*Repository)(nil)
)

func (r *Repository) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]evaluationrun.ExpiredLease, error) {
	if r == nil || r.db == nil || limit <= 0 {
		return nil, nil
	}
	var rows []RuntimeCheckpointPO
	if err := r.db.WithContext(ctx).
		Where("scope = ? AND status = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ? AND deleted_at IS NULL", scopeEvaluationRun, evalrun.StatusRunning.String(), now).
		Order("lease_expires_at ASC, id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]evaluationrun.ExpiredLease, 0, len(rows))
	for _, row := range rows {
		if row.AssessmentID != nil {
			result = append(result, evaluationrun.ExpiredLease{AssessmentID: *row.AssessmentID, RunID: evalrun.ID(row.ResourceID)})
		}
	}
	return result, nil
}

func (r *Repository) AuthorizeRetry(ctx context.Context, request evaluationrun.RetryAuthorizationRequest) (*evalrun.EvaluationRun, error) {
	if r == nil || r.db == nil || request.AssessmentID == 0 || request.ExpectedAttempt < 1 {
		return nil, fmt.Errorf("invalid evaluation retry authorization")
	}
	db := checkpointDB(ctx, r.db)
	var po RuntimeCheckpointPO
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("scope = ? AND assessment_id = ? AND deleted_at IS NULL", scopeEvaluationRun, request.AssessmentID).
		Order("attempt_no DESC, id DESC").First(&po).Error; err != nil {
		return nil, err
	}
	run := runFromPO(po)
	if run.Attempt().Number != request.ExpectedAttempt {
		return nil, evaluationrun.ErrClaimLost
	}
	if err := run.AuthorizeOneRetry(request.Origin, request.RequestID, request.EventID, request.AuthorizedAt); err != nil {
		return nil, err
	}
	updated := runToPO(run)
	result := db.Model(&RuntimeCheckpointPO{}).
		Where("id = ? AND attempt_no = ? AND retry_disposition = ? AND deleted_at IS NULL", po.ID, request.ExpectedAttempt, po.RetryDisposition).
		Updates(map[string]interface{}{
			"retry_disposition": updated.RetryDisposition, "next_attempt_at": updated.NextAttemptAt,
			"retry_event_id": updated.RetryEventID, "action_request_id": updated.ActionRequestID,
			"updated_at": request.AuthorizedAt,
		})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, evaluationrun.ErrClaimLost
	}
	return &run, nil
}

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
			if err := run.Claim(evalrun.ClaimInput{Token: request.Token, TraceID: request.TraceID, ClaimedAt: request.ClaimedAt, LeaseExpiresAt: request.LeaseUntil}); err != nil {
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
		attempt := run.Attempt()
		switch attempt.Status {
		case evalrun.StatusPending:
			// Claim the existing attempt.
		case evalrun.StatusRunning:
			if run.HasActiveLease(request.ClaimedAt) {
				result = evaluationrun.ClaimResult{Run: run}
				return nil
			}
		case evalrun.StatusFailed:
			decision := run.RetryDecision()
			if (!run.Retryable() && request.Origin != retrygovernance.AttemptOriginForce) || decision == nil || decision.Disposition != retrygovernance.DispositionAutomatic ||
				decision.NextAttemptAt == nil || decision.NextAttemptAt.After(request.ClaimedAt) ||
				request.ExpectedAttempt != attempt.Number || request.RetryEventID == "" || request.RetryEventID != decision.RetryEventID ||
				!request.Origin.IsValid() || request.Origin == retrygovernance.AttemptOriginInitial || request.Origin == retrygovernance.AttemptOriginLeaseRecovery ||
				request.ActionRequestID != decision.ActionRequestID {
				result = evaluationrun.ClaimResult{Run: run}
				return nil
			}
			run = evalrun.NextEvaluationRunWithOrigin(run, request.Origin)
		case evalrun.StatusSucceeded:
			result = evaluationrun.ClaimResult{Run: run}
			return nil
		default:
			return fmt.Errorf("latest evaluation run %s has unknown status %q", run.ID(), attempt.Status)
		}
		if err := run.Claim(evalrun.ClaimInput{Token: request.Token, TraceID: request.TraceID, ClaimedAt: request.ClaimedAt, LeaseExpiresAt: request.LeaseUntil}); err != nil {
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
	if run.ClaimToken() == "" {
		return evaluationrun.ErrClaimLost
	}
	po := runToPO(run)
	updates := claimUpdates(po)
	updates["updated_at"] = time.Now()
	result := checkpointDB(ctx, r.db).Model(&RuntimeCheckpointPO{}).
		Where("scope = ? AND resource_id = ? AND attempt_no = ? AND claim_token = ? AND status = ? AND deleted_at IS NULL",
			scopeEvaluationRun, po.ResourceID, po.AttemptNo, run.ClaimToken(), evalrun.StatusRunning.String()).
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
		"status":               po.Status,
		"started_at":           po.StartedAt,
		"finished_at":          po.FinishedAt,
		"error_code":           po.ErrorCode,
		"error_message":        po.ErrorMessage,
		"retryable":            po.Retryable,
		"attempt_origin":       po.AttemptOrigin,
		"retry_disposition":    po.RetryDisposition,
		"next_attempt_at":      po.NextAttemptAt,
		"policy_max_attempts":  po.PolicyMaxAttempts,
		"retry_policy_version": po.RetryPolicyVersion,
		"retry_event_id":       po.RetryEventID,
		"action_request_id":    po.ActionRequestID,
		"trace_id":             po.TraceID,
		"input_snapshot_ref":   po.InputSnapshotRef,
		"claim_token":          po.ClaimToken,
		"lease_expires_at":     po.LeaseExpiresAt,
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
		Joins(`INNER JOIN (
			SELECT assessment_id, MAX(attempt_no) AS latest_attempt
			FROM runtime_checkpoint
			WHERE scope = ? AND deleted_at IS NULL
			GROUP BY assessment_id
		) AS latest ON latest.assessment_id = rc.assessment_id AND latest.latest_attempt = rc.attempt_no`, scopeEvaluationRun).
		Where("rc.scope = ? AND rc.status = ? AND rc.retryable = ? AND rc.deleted_at IS NULL",
			scopeEvaluationRun, evalrun.StatusFailed.String(), true).
		Where("a.status = ?", "failed").
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
	assessmentID := run.AssessmentID()
	attempt := run.Attempt()
	po := &RuntimeCheckpointPO{
		Scope:        scopeEvaluationRun,
		ResourceID:   run.ID().String(),
		AttemptNo:    uint(attempt.Number),
		AssessmentID: &assessmentID,
		Status:       attempt.Status.String(),
		StartedAt:    run.StartedAt(),
		FinishedAt:   run.FinishedAt(),
		Retryable:    run.Retryable(),
	}
	if run.TraceID() != "" {
		traceID := run.TraceID()
		po.TraceID = &traceID
	}
	if run.InputSnapshotRef() != "" {
		inputSnapshotRef := run.InputSnapshotRef()
		po.InputSnapshotRef = &inputSnapshotRef
	}
	if run.ClaimToken() != "" {
		claimToken := run.ClaimToken()
		po.ClaimToken = &claimToken
	}
	po.LeaseExpiresAt = run.LeaseExpiresAt()
	origin := string(run.Origin())
	po.AttemptOrigin = &origin
	if decision := run.RetryDecision(); decision != nil {
		disposition := string(decision.Disposition)
		maxAttempts := uint(decision.MaxAutomaticAttempts)
		po.RetryDisposition = &disposition
		po.NextAttemptAt = decision.NextAttemptAt
		po.PolicyMaxAttempts = &maxAttempts
		if decision.PolicyVersion != "" {
			version := decision.PolicyVersion
			po.RetryPolicyVersion = &version
		}
		if decision.RetryEventID != "" {
			eventID := decision.RetryEventID
			po.RetryEventID = &eventID
		}
		if decision.ActionRequestID != "" {
			requestID := decision.ActionRequestID
			po.ActionRequestID = &requestID
		}
	}
	if failure := run.Failure(); failure != nil {
		code := failure.Kind.String()
		message := failure.Message
		po.ErrorCode = &code
		po.ErrorMessage = &message
		po.Retryable = failure.Retryable
	}
	return po
}

func runFromPO(po RuntimeCheckpointPO) evalrun.EvaluationRun {
	input := evalrun.ReconstructInput{
		RunID: evalrun.ID(po.ResourceID), AssessmentID: derefUint64(po.AssessmentID),
		Attempt:   evalrun.Attempt{Number: int(po.AttemptNo), Status: evalrun.Status(po.Status)},
		StartedAt: po.StartedAt, FinishedAt: po.FinishedAt, LeaseExpiresAt: po.LeaseExpiresAt,
	}
	if po.TraceID != nil {
		input.TraceID = *po.TraceID
	}
	if po.InputSnapshotRef != nil {
		input.InputSnapshotRef = *po.InputSnapshotRef
	}
	if po.ClaimToken != nil {
		input.ClaimToken = *po.ClaimToken
	}
	if po.AttemptOrigin != nil {
		input.Origin = retrygovernance.AttemptOrigin(*po.AttemptOrigin)
	}
	if po.ErrorCode != nil || po.ErrorMessage != nil {
		failure := evalrun.Failure{Retryable: po.Retryable}
		if po.ErrorCode != nil {
			failure.Kind = evalrun.FailureKind(*po.ErrorCode)
		}
		if po.ErrorMessage != nil {
			failure.Message = *po.ErrorMessage
		}
		input.Failure = &failure
		if po.RetryDisposition != nil {
			decision := retrygovernance.Decision{Disposition: retrygovernance.Disposition(*po.RetryDisposition), Attempt: int(po.AttemptNo)}
			if po.PolicyMaxAttempts != nil {
				decision.MaxAutomaticAttempts = int(*po.PolicyMaxAttempts)
			}
			decision.RemainingAutomaticAttempts = decision.MaxAutomaticAttempts - decision.Attempt
			if decision.RemainingAutomaticAttempts < 0 {
				decision.RemainingAutomaticAttempts = 0
			}
			decision.NextAttemptAt = po.NextAttemptAt
			if po.RetryPolicyVersion != nil {
				decision.PolicyVersion = *po.RetryPolicyVersion
			}
			if po.RetryEventID != nil {
				decision.RetryEventID = *po.RetryEventID
			}
			if po.ActionRequestID != nil {
				decision.ActionRequestID = *po.ActionRequestID
			}
			input.RetryDecision = &decision
		} else {
			decisionAt := po.UpdatedAt
			if po.FinishedAt != nil {
				decisionAt = *po.FinishedAt
			}
			decision := retrygovernance.BusinessPolicy().DecideFailure(failure.Retryable, int(po.AttemptNo), decisionAt)
			input.RetryDecision = &decision
		}
	}
	return evalrun.Reconstruct(input)
}

func derefUint64(v *uint64) uint64 {
	if v == nil {
		return 0
	}
	return *v
}
