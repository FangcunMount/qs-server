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
	var po AnalyticsScanWatermarkPO
	err := r.WithContext(ctx).
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
	if watermark == nil {
		return nil
	}
	po := scanWatermarkFromDomain(watermark)
	return r.WithContext(ctx).Clauses(clause.OnConflict{
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

// ListReportGeneratedFacts scans interpreted assessments as report facts.
func (r *StatisticsRepository) ListReportGeneratedFacts(
	ctx context.Context,
	orgID int64,
	sinceID uint64,
	sinceTime time.Time,
	limit int,
) ([]domainStatistics.ReportGeneratedFact, error) {
	if limit <= 0 {
		return nil, nil
	}
	query := r.WithContext(ctx).
		Model(&evaluationInfra.AssessmentPO{}).
		Select("id, org_id, testee_id, answer_sheet_id, interpreted_at, created_at").
		Where("org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL", orgID)
	if !sinceTime.IsZero() {
		query = query.Where("(id > ? OR interpreted_at > ?)", sinceID, sinceTime)
	}
	var rows []evaluationInfra.AssessmentPO
	if err := query.Order("id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	facts := make([]domainStatistics.ReportGeneratedFact, 0, len(rows))
	for _, row := range rows {
		occurredAt := row.CreatedAt
		if row.InterpretedAt != nil {
			occurredAt = *row.InterpretedAt
		}
		facts = append(facts, domainStatistics.ReportGeneratedFact{
			OrgID:        row.OrgID,
			TesteeID:     row.TesteeID,
			AssessmentID: row.ID.Uint64(),
			ReportID:     row.ID.Uint64(),
			OccurredAt:   occurredAt,
		})
	}
	return facts, nil
}
