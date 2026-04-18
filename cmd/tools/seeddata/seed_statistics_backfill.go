package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	evaluationMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	statisticsMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"gorm.io/gorm"
)

const (
	statisticsBackfillWaitTimeout  = 20 * time.Second
	statisticsBackfillPollInterval = 500 * time.Millisecond
)

func seedStatisticsBackfill(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for statistics_backfill")
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for statistics_backfill")
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after statistics_backfill", "error", closeErr.Error())
		}
	}()

	deps.Logger.Infow("Statistics backfill started",
		"org_id", deps.Config.Global.OrgID,
		"mode", "rebuild_analytics_projection_from_behavior_footprint_and_assessment_episode",
	)
	progress := newSeedProgressBar("statistics_backfill", 3)
	defer progress.Close()

	pendingCount, err := waitForAnalyticsProjectorIdle(ctx, mysqlDB, deps)
	if err != nil {
		return err
	}
	progress.Increment()

	if err := rebuildAnalyticsProjectionTables(ctx, mysqlDB); err != nil {
		return err
	}
	progress.Increment()

	if err := deps.APIClient.NotifyRepairComplete(ctx, RepairCompleteRequest{
		RepairKind: "statistics_backfill",
		OrgIDs:     []int64{deps.Config.Global.OrgID},
	}); err != nil {
		return err
	}
	progress.Increment()
	progress.Complete()

	orgRows, clinicianRows, entryRows, err := countAnalyticsProjectionRows(ctx, mysqlDB)
	if err != nil {
		return err
	}
	deps.Logger.Infow("Statistics backfill completed",
		"org_id", deps.Config.Global.OrgID,
		"pending_event_count", pendingCount,
		"org_projection_rows", orgRows,
		"clinician_projection_rows", clinicianRows,
		"entry_projection_rows", entryRows,
	)
	return nil
}

func waitForAnalyticsProjectorIdle(ctx context.Context, mysqlDB *gorm.DB, deps *dependencies) (int64, error) {
	deadline := time.Now().Add(statisticsBackfillWaitTimeout)
	for {
		processingCount, pendingCount, err := loadAnalyticsProjectorState(ctx, mysqlDB)
		if err != nil {
			return 0, err
		}
		if processingCount == 0 {
			if pendingCount > 0 {
				deps.Logger.Warnw("Analytics pending events still exist during statistics_backfill; projection rebuild will use current committed footprints/episodes",
					"pending_event_count", pendingCount,
				)
			}
			return pendingCount, nil
		}
		if time.Now().After(deadline) {
			deps.Logger.Warnw("Timed out waiting for analytics projector to become idle; proceeding with current committed footprints/episodes",
				"processing_checkpoint_count", processingCount,
				"pending_event_count", pendingCount,
				"timeout", statisticsBackfillWaitTimeout.String(),
			)
			return pendingCount, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(statisticsBackfillPollInterval):
		}
	}
}

func loadAnalyticsProjectorState(ctx context.Context, mysqlDB *gorm.DB) (processingCount int64, pendingCount int64, err error) {
	if err = mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AnalyticsProjectorCheckpointPO{}).TableName()).
		Where("status = ? AND deleted_at IS NULL", statisticsMySQL.AnalyticsProjectorCheckpointStatusProcessing).
		Count(&processingCount).Error; err != nil {
		return 0, 0, fmt.Errorf("count analytics projector processing checkpoints: %w", err)
	}
	if err = mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AnalyticsPendingEventPO{}).TableName()).
		Where("deleted_at IS NULL").
		Count(&pendingCount).Error; err != nil {
		return 0, 0, fmt.Errorf("count analytics pending events: %w", err)
	}
	return processingCount, pendingCount, nil
}

func rebuildAnalyticsProjectionTables(ctx context.Context, mysqlDB *gorm.DB) error {
	orgTable := (statisticsMySQL.AnalyticsProjectionOrgDailyPO{}).TableName()
	clinicianTable := (statisticsMySQL.AnalyticsProjectionClinicianDailyPO{}).TableName()
	entryTable := (statisticsMySQL.AnalyticsProjectionEntryDailyPO{}).TableName()
	footprintTable := (statisticsMySQL.BehaviorFootprintPO{}).TableName()
	episodeTable := (statisticsMySQL.AssessmentEpisodePO{}).TableName()
	assessmentTable := (evaluationMySQL.AssessmentPO{}).TableName()

	orgInsert := fmt.Sprintf(`
INSERT INTO %s (
  org_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count,
  created_at, updated_at
)
SELECT
  agg.org_id,
  agg.stat_date,
  SUM(agg.entry_opened_count),
  SUM(agg.intake_confirmed_count),
  SUM(agg.testee_profile_created_count),
  SUM(agg.care_relationship_established_count),
  SUM(agg.care_relationship_transferred_count),
  SUM(agg.answersheet_submitted_count),
  SUM(agg.assessment_created_count),
  SUM(agg.report_generated_count),
  SUM(agg.episode_completed_count),
  SUM(agg.episode_failed_count),
  NOW(3),
  NOW(3)
FROM (
  SELECT org_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count
  FROM %s WHERE deleted_at IS NULL AND event_name = 'entry_opened'
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'intake_confirmed'
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'testee_profile_created'
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'care_relationship_established'
  UNION ALL
  SELECT org_id, DATE(occurred_at), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'care_relationship_transferred'
  UNION ALL
  SELECT org_id, DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL
  UNION ALL
  SELECT org_id, DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND assessment_created_at IS NOT NULL
  UNION ALL
  SELECT org_id, DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM %s WHERE deleted_at IS NULL AND report_generated_at IS NOT NULL
  UNION ALL
  SELECT org_id, DATE(failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM %s WHERE deleted_at IS NULL AND failed_at IS NOT NULL
) agg
GROUP BY agg.org_id, agg.stat_date
`, orgTable, footprintTable, footprintTable, footprintTable, footprintTable, footprintTable, episodeTable, episodeTable, episodeTable, assessmentTable)

	clinicianInsert := fmt.Sprintf(`
INSERT INTO %s (
  org_id, clinician_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count,
  created_at, updated_at
)
SELECT
  agg.org_id,
  agg.clinician_id,
  agg.stat_date,
  SUM(agg.entry_opened_count),
  SUM(agg.intake_confirmed_count),
  SUM(agg.testee_profile_created_count),
  SUM(agg.care_relationship_established_count),
  SUM(agg.care_relationship_transferred_count),
  SUM(agg.answersheet_submitted_count),
  SUM(agg.assessment_created_count),
  SUM(agg.report_generated_count),
  SUM(agg.episode_completed_count),
  SUM(agg.episode_failed_count),
  NOW(3),
  NOW(3)
FROM (
  SELECT org_id, clinician_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count
  FROM %s WHERE deleted_at IS NULL AND event_name = 'entry_opened' AND clinician_id <> 0
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'intake_confirmed' AND clinician_id <> 0
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'testee_profile_created' AND clinician_id <> 0
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'care_relationship_established' AND clinician_id <> 0
  UNION ALL
  SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'care_relationship_transferred' AND clinician_id <> 0
  UNION ALL
  SELECT org_id, clinician_id, DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND clinician_id IS NOT NULL
  UNION ALL
  SELECT org_id, clinician_id, DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND clinician_id IS NOT NULL AND assessment_created_at IS NOT NULL
  UNION ALL
  SELECT org_id, clinician_id, DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM %s WHERE deleted_at IS NULL AND clinician_id IS NOT NULL AND report_generated_at IS NOT NULL
  UNION ALL
  SELECT e.org_id, e.clinician_id, DATE(a.failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM %s e
  JOIN %s a ON a.answer_sheet_id = e.answersheet_id AND a.deleted_at IS NULL
  WHERE e.deleted_at IS NULL AND e.clinician_id IS NOT NULL AND a.failed_at IS NOT NULL
) agg
GROUP BY agg.org_id, agg.clinician_id, agg.stat_date
`, clinicianTable, footprintTable, footprintTable, footprintTable, footprintTable, footprintTable, episodeTable, episodeTable, episodeTable, episodeTable, assessmentTable)

	entryInsert := fmt.Sprintf(`
INSERT INTO %s (
  org_id, entry_id, clinician_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count,
  created_at, updated_at
)
SELECT
  agg.org_id,
  agg.entry_id,
  agg.clinician_id,
  agg.stat_date,
  SUM(agg.entry_opened_count),
  SUM(agg.intake_confirmed_count),
  SUM(agg.testee_profile_created_count),
  SUM(agg.care_relationship_established_count),
  SUM(agg.care_relationship_transferred_count),
  SUM(agg.answersheet_submitted_count),
  SUM(agg.assessment_created_count),
  SUM(agg.report_generated_count),
  SUM(agg.episode_completed_count),
  SUM(agg.episode_failed_count),
  NOW(3),
  NOW(3)
FROM (
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count
  FROM %s WHERE deleted_at IS NULL AND event_name = 'entry_opened' AND entry_id <> 0
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'intake_confirmed' AND entry_id <> 0
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'testee_profile_created' AND entry_id <> 0
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND event_name = 'care_relationship_established' AND entry_id <> 0
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND entry_id IS NOT NULL
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM %s WHERE deleted_at IS NULL AND entry_id IS NOT NULL AND assessment_created_at IS NOT NULL
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM %s WHERE deleted_at IS NULL AND entry_id IS NOT NULL AND report_generated_at IS NOT NULL
  UNION ALL
  SELECT e.org_id, e.entry_id, COALESCE(e.clinician_id, 0), DATE(a.failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM %s e
  JOIN %s a ON a.answer_sheet_id = e.answersheet_id AND a.deleted_at IS NULL
  WHERE e.deleted_at IS NULL AND e.entry_id IS NOT NULL AND a.failed_at IS NOT NULL
) agg
GROUP BY agg.org_id, agg.entry_id, agg.clinician_id, agg.stat_date
`, entryTable, footprintTable, footprintTable, footprintTable, footprintTable, episodeTable, episodeTable, episodeTable, episodeTable, assessmentTable)

	return mysqlDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{orgTable, clinicianTable, entryTable} {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
			}
		}
		for _, stmt := range []string{orgInsert, clinicianInsert, entryInsert} {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("rebuild analytics projection: %w", err)
			}
		}
		return nil
	})
}

func countAnalyticsProjectionRows(ctx context.Context, mysqlDB *gorm.DB) (orgRows, clinicianRows, entryRows int64, err error) {
	if err = mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AnalyticsProjectionOrgDailyPO{}).TableName()).
		Where("deleted_at IS NULL").
		Count(&orgRows).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("count analytics_projection_org_daily: %w", err)
	}
	if err = mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AnalyticsProjectionClinicianDailyPO{}).TableName()).
		Where("deleted_at IS NULL").
		Count(&clinicianRows).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("count analytics_projection_clinician_daily: %w", err)
	}
	if err = mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AnalyticsProjectionEntryDailyPO{}).TableName()).
		Where("deleted_at IS NULL").
		Count(&entryRows).Error; err != nil {
		return 0, 0, 0, fmt.Errorf("count analytics_projection_entry_daily: %w", err)
	}
	return orgRows, clinicianRows, entryRows, nil
}
