package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	evaluationInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LoadScanWatermark returns the watermark for a source/org pair.
func (r *StatisticsRepository) LoadScanWatermark(ctx context.Context, orgID int64, sourceName string) (*domainStatistics.ScanWatermark, error) {
	ctx, release, acquireErr := r.acquire(ctx)
	if acquireErr != nil {
		return nil, acquireErr
	}
	defer release()

	var po AnalyticsScanWatermarkPO
	err := r.withContext(ctx).
		Where("source_name = ? AND org_id = ?", sourceName, orgID).
		First(&po).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return scanWatermarkToDomain(&po), nil
}

// SaveScanWatermark upserts scan progress for a source/org pair.
func (r *StatisticsRepository) SaveScanWatermark(ctx context.Context, watermark *domainStatistics.ScanWatermark) error {
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	if watermark == nil {
		return nil
	}
	po := scanWatermarkFromDomain(watermark)
	return r.withContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source_name"}, {Name: "org_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"last_seen_id",
			"last_seen_time",
			"scan_window_start",
			"scan_window_end",
			"status",
			"last_error",
			"updated_at",
		}),
	}).Create(po).Error
}

// ListAssessmentCreatedFacts scans created assessments as assessment-created facts.
func (r *StatisticsRepository) ListAssessmentCreatedFacts(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
) ([]domainStatistics.AssessmentCreatedFact, error) {
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	if limit <= 0 {
		return nil, nil
	}
	query := r.withContext(ctx).
		Model(&evaluationInfra.AssessmentPO{}).
		Select("id, org_id, testee_id, answer_sheet_id, created_at").
		Where("org_id = ? AND deleted_at IS NULL", orgID)
	if !sinceTime.IsZero() {
		query = query.Where("(id > ? OR created_at > ?)", sinceID, sinceTime)
	}
	var rows []evaluationInfra.AssessmentPO
	if err := query.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	facts := make([]domainStatistics.AssessmentCreatedFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, domainStatistics.AssessmentCreatedFact{
			OrgID:         row.OrgID,
			TesteeID:      row.TesteeID,
			AnswerSheetID: row.AnswerSheetID,
			AssessmentID:  row.ID.Uint64(),
			OccurredAt:    row.CreatedAt,
		})
	}
	return facts, nil
}

// ListEntryResolveFacts scans assessment entry resolve logs.
func (r *StatisticsRepository) ListEntryResolveFacts(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
) ([]domainStatistics.EntryResolveFact, error) {
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	if limit <= 0 {
		return nil, nil
	}
	query := r.withContext(ctx).
		Model(&AssessmentEntryResolveLogPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID)
	if !sinceTime.IsZero() {
		query = query.Where("(id > ? OR resolved_at > ?)", sinceID, sinceTime)
	}
	var rows []AssessmentEntryResolveLogPO
	if err := query.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	facts := make([]domainStatistics.EntryResolveFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, domainStatistics.EntryResolveFact{
			OrgID:       row.OrgID,
			ClinicianID: row.ClinicianID,
			EntryID:     row.EntryID,
			LogID:       row.ID,
			OccurredAt:  row.ResolvedAt,
		})
	}
	return facts, nil
}

// ListEntryIntakeFacts scans assessment entry intake logs.
func (r *StatisticsRepository) ListEntryIntakeFacts(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
) ([]domainStatistics.EntryIntakeFact, error) {
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	if limit <= 0 {
		return nil, nil
	}
	query := r.withContext(ctx).
		Model(&AssessmentEntryIntakeLogPO{}).
		Where("org_id = ? AND deleted_at IS NULL", orgID)
	if !sinceTime.IsZero() {
		query = query.Where("(id > ? OR intake_at > ?)", sinceID, sinceTime)
	}
	var rows []AssessmentEntryIntakeLogPO
	if err := query.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	facts := make([]domainStatistics.EntryIntakeFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, domainStatistics.EntryIntakeFact{
			OrgID:             row.OrgID,
			ClinicianID:       row.ClinicianID,
			EntryID:           row.EntryID,
			TesteeID:          row.TesteeID,
			LogID:             row.ID,
			TesteeCreated:     row.TesteeCreated,
			AssignmentCreated: row.AssignmentCreated,
			OccurredAt:        row.IntakeAt,
		})
	}
	return facts, nil
}
