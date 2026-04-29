package statistics

import (
	"context"
	"time"

	"gorm.io/gorm"
)

func (r *StatisticsRepository) rebuildAccessDailyProjections(ctx context.Context, tx *gorm.DB, orgID int64, startDate, endDate time.Time) error {
	if err := deleteProjectionWindow(ctx, tx, "analytics_access_org_daily", orgID, startDate, endDate); err != nil {
		return err
	}
	if err := deleteProjectionWindow(ctx, tx, "analytics_access_clinician_daily", orgID, startDate, endDate); err != nil {
		return err
	}
	if err := deleteProjectionWindow(ctx, tx, "analytics_access_entry_daily", orgID, startDate, endDate); err != nil {
		return err
	}

	if err := tx.WithContext(ctx).Exec(accessOrgDailyInsertSQL, orgID,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(accessClinicianDailyInsertSQL,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(accessEntryDailyInsertSQL,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error
}

func (r *StatisticsRepository) rebuildAssessmentServiceDailyProjections(ctx context.Context, tx *gorm.DB, orgID int64, startDate, endDate time.Time) error {
	for _, table := range []string{
		"analytics_assessment_service_org_daily",
		"analytics_assessment_service_clinician_daily",
		"analytics_assessment_service_entry_daily",
		"analytics_assessment_service_content_daily",
	} {
		if err := deleteProjectionWindow(ctx, tx, table, orgID, startDate, endDate); err != nil {
			return err
		}
	}

	if err := tx.WithContext(ctx).Exec(assessmentServiceOrgDailyInsertSQL, orgID,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(assessmentServiceClinicianDailyInsertSQL,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(assessmentServiceEntryDailyInsertSQL,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(assessmentServiceContentDailyInsertSQL,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error
}

func (r *StatisticsRepository) rebuildPlanTaskDailyProjection(ctx context.Context, tx *gorm.DB, orgID int64) error {
	if err := tx.WithContext(ctx).Exec("DELETE FROM analytics_plan_task_daily WHERE org_id = ?", orgID).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(planTaskDailyInsertSQL, orgID, orgID, orgID, orgID).Error
}

func (r *StatisticsRepository) rebuildOrganizationSnapshot(ctx context.Context, tx *gorm.DB, orgID int64, snapshotAt time.Time) error {
	if err := tx.WithContext(ctx).Exec("DELETE FROM analytics_organization_snapshot WHERE org_id = ?", orgID).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(organizationSnapshotInsertSQL,
		orgID,
		orgID,
		orgID,
		orgID, snapshotAt,
		orgID,
		orgID,
		orgID,
		orgID,
		orgID,
		orgID,
		snapshotAt,
	).Error
}

func (r *StatisticsRepository) rebuildPlanTaskWindowSnapshots(ctx context.Context, tx *gorm.DB, orgID int64, now time.Time) error {
	if err := tx.WithContext(ctx).Exec("DELETE FROM analytics_plan_task_window_snapshot WHERE org_id = ?", orgID).Error; err != nil {
		return err
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	windows := []struct {
		preset string
		from   time.Time
		to     time.Time
	}{
		{preset: "today", from: todayStart, to: todayStart},
		{preset: "7d", from: todayStart.AddDate(0, 0, -6), to: todayStart},
		{preset: "30d", from: todayStart.AddDate(0, 0, -29), to: todayStart},
	}
	snapshotAt := now
	for _, window := range windows {
		if err := tx.WithContext(ctx).Exec(planTaskWindowSnapshotInsertSQL,
			orgID,
			window.preset,
			window.from,
			window.to,
			orgID, window.from, window.to,
			orgID, window.from, window.to,
			orgID, window.from, window.to,
			orgID, window.from, window.to,
			orgID, window.from, window.to,
			orgID, window.from, window.to,
			snapshotAt,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

func deleteProjectionWindow(ctx context.Context, tx *gorm.DB, table string, orgID int64, startDate, endDate time.Time) error {
	return tx.WithContext(ctx).
		Exec("DELETE FROM "+table+" WHERE org_id = ? AND stat_date >= ? AND stat_date < ?", orgID, startDate, endDate).
		Error
}

const organizationSnapshotInsertSQL = `
INSERT INTO analytics_organization_snapshot (
  org_id, testee_count, clinician_count, active_entry_count, assessment_count,
  report_count, dimension_clinician_count, dimension_entry_count, dimension_content_count, snapshot_at
)
SELECT
  ? AS org_id,
  (SELECT COUNT(*) FROM testee WHERE org_id = ? AND deleted_at IS NULL) AS testee_count,
  (SELECT COUNT(*) FROM clinician WHERE org_id = ? AND is_active = 1 AND deleted_at IS NULL) AS clinician_count,
  (SELECT COUNT(*) FROM assessment_entry WHERE org_id = ? AND is_active = 1 AND deleted_at IS NULL AND (expires_at IS NULL OR expires_at > ?)) AS active_entry_count,
  (SELECT COUNT(*) FROM assessment WHERE org_id = ? AND deleted_at IS NULL) AS assessment_count,
  (SELECT COUNT(*) FROM assessment WHERE org_id = ? AND interpreted_at IS NOT NULL AND deleted_at IS NULL) AS report_count,
  (SELECT COUNT(*) FROM clinician WHERE org_id = ? AND deleted_at IS NULL) AS dimension_clinician_count,
  (SELECT COUNT(*) FROM assessment_entry WHERE org_id = ? AND deleted_at IS NULL) AS dimension_entry_count,
  (
    SELECT COUNT(*)
    FROM (
      SELECT DISTINCT
        CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END AS content_type,
        COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS content_code
      FROM assessment
      WHERE org_id = ? AND deleted_at IS NULL
        AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
    ) content
  ) AS dimension_content_count,
  ? AS snapshot_at`

const planTaskWindowSnapshotInsertSQL = `
INSERT INTO analytics_plan_task_window_snapshot (
  org_id, preset, window_start, window_end, task_created_count, task_opened_count,
  task_completed_count, task_expired_count, enrolled_testees, active_testees, snapshot_at
)
SELECT
  ? AS org_id,
  ? AS preset,
  ? AS window_start,
  ? AS window_end,
  COALESCE((SELECT SUM(task_created_count) FROM analytics_plan_task_daily WHERE org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL), 0) AS task_created_count,
  COALESCE((SELECT SUM(task_opened_count) FROM analytics_plan_task_daily WHERE org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL), 0) AS task_opened_count,
  COALESCE((SELECT SUM(task_completed_count) FROM analytics_plan_task_daily WHERE org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL), 0) AS task_completed_count,
  COALESCE((SELECT SUM(task_expired_count) FROM analytics_plan_task_daily WHERE org_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL), 0) AS task_expired_count,
  (
    SELECT COUNT(DISTINCT t.testee_id)
    FROM assessment_task t
    JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
    WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.created_at >= ? AND t.created_at < ?
  ) AS enrolled_testees,
  (
    SELECT COUNT(DISTINCT t.testee_id)
    FROM assessment_task t
    JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
    WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.status = 'completed' AND t.completed_at >= ? AND t.completed_at < ?
  ) AS active_testees,
  ? AS snapshot_at`

const accessOrgDailyInsertSQL = `
INSERT INTO analytics_access_org_daily (
  org_id, stat_date, entry_opened_count, intake_confirmed_count,
  testee_created_count, care_relationship_established_count
)
SELECT
  ? AS org_id,
  raw.stat_date,
  SUM(raw.entry_opened_count),
  SUM(raw.intake_confirmed_count),
  SUM(raw.testee_created_count),
  SUM(raw.care_relationship_established_count)
FROM (
  SELECT DATE(resolved_at) AS stat_date, COUNT(*) AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_created_count, 0 AS care_relationship_established_count
  FROM assessment_entry_resolve_log
  WHERE org_id = ? AND deleted_at IS NULL AND resolved_at >= ? AND resolved_at < ?
  GROUP BY DATE(resolved_at)
  UNION ALL
  SELECT DATE(intake_at) AS stat_date, 0, COUNT(*), 0, 0
  FROM assessment_entry_intake_log
  WHERE org_id = ? AND deleted_at IS NULL AND intake_at >= ? AND intake_at < ?
  GROUP BY DATE(intake_at)
  UNION ALL
  SELECT DATE(created_at) AS stat_date, 0, 0, COUNT(*), 0
  FROM testee
  WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?
  GROUP BY DATE(created_at)
  UNION ALL
  SELECT DATE(bound_at) AS stat_date, 0, 0, 0, COUNT(DISTINCT testee_id)
  FROM clinician_relation
  WHERE org_id = ? AND deleted_at IS NULL AND bound_at >= ? AND bound_at < ?
  GROUP BY DATE(bound_at)
) raw
GROUP BY raw.stat_date`

const accessClinicianDailyInsertSQL = `
INSERT INTO analytics_access_clinician_daily (
  org_id, clinician_id, stat_date, entry_opened_count, intake_confirmed_count,
  testee_created_count, care_relationship_established_count
)
SELECT
  raw.org_id,
  raw.clinician_id,
  raw.stat_date,
  SUM(raw.entry_opened_count),
  SUM(raw.intake_confirmed_count),
  SUM(raw.testee_created_count),
  SUM(raw.care_relationship_established_count)
FROM (
  SELECT org_id, clinician_id, DATE(resolved_at) AS stat_date, COUNT(*) AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_created_count, 0 AS care_relationship_established_count
  FROM assessment_entry_resolve_log
  WHERE org_id = ? AND deleted_at IS NULL AND clinician_id <> 0 AND resolved_at >= ? AND resolved_at < ?
  GROUP BY org_id, clinician_id, DATE(resolved_at)
  UNION ALL
  SELECT org_id, clinician_id, DATE(intake_at) AS stat_date, 0, COUNT(*), 0, 0
  FROM assessment_entry_intake_log
  WHERE org_id = ? AND deleted_at IS NULL AND clinician_id <> 0 AND intake_at >= ? AND intake_at < ?
  GROUP BY org_id, clinician_id, DATE(intake_at)
  UNION ALL
  SELECT org_id, clinician_id, DATE(bound_at) AS stat_date, 0, 0, 0, COUNT(DISTINCT testee_id)
  FROM clinician_relation
  WHERE org_id = ? AND deleted_at IS NULL AND clinician_id <> 0 AND bound_at >= ? AND bound_at < ?
  GROUP BY org_id, clinician_id, DATE(bound_at)
) raw
GROUP BY raw.org_id, raw.clinician_id, raw.stat_date`

const accessEntryDailyInsertSQL = `
INSERT INTO analytics_access_entry_daily (
  org_id, entry_id, clinician_id, stat_date, entry_opened_count, intake_confirmed_count,
  testee_created_count, care_relationship_established_count
)
SELECT
  raw.org_id,
  raw.entry_id,
  MAX(raw.clinician_id),
  raw.stat_date,
  SUM(raw.entry_opened_count),
  SUM(raw.intake_confirmed_count),
  SUM(raw.testee_created_count),
  SUM(raw.care_relationship_established_count)
FROM (
  SELECT org_id, entry_id, clinician_id, DATE(resolved_at) AS stat_date, COUNT(*) AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_created_count, 0 AS care_relationship_established_count
  FROM assessment_entry_resolve_log
  WHERE org_id = ? AND deleted_at IS NULL AND entry_id <> 0 AND resolved_at >= ? AND resolved_at < ?
  GROUP BY org_id, entry_id, clinician_id, DATE(resolved_at)
  UNION ALL
  SELECT org_id, entry_id, clinician_id, DATE(intake_at) AS stat_date, 0, COUNT(*), 0, 0
  FROM assessment_entry_intake_log
  WHERE org_id = ? AND deleted_at IS NULL AND entry_id <> 0 AND intake_at >= ? AND intake_at < ?
  GROUP BY org_id, entry_id, clinician_id, DATE(intake_at)
  UNION ALL
  SELECT org_id, source_id AS entry_id, clinician_id, DATE(bound_at) AS stat_date, 0, 0, 0, COUNT(DISTINCT testee_id)
  FROM clinician_relation
  WHERE org_id = ? AND deleted_at IS NULL AND source_id IS NOT NULL AND source_id <> 0 AND bound_at >= ? AND bound_at < ?
  GROUP BY org_id, source_id, clinician_id, DATE(bound_at)
) raw
GROUP BY raw.org_id, raw.entry_id, raw.stat_date`

const assessmentServiceOrgDailyInsertSQL = `
INSERT INTO analytics_assessment_service_org_daily (
  org_id, stat_date, answersheet_submitted_count, assessment_created_count,
  report_generated_count, assessment_failed_count
)
SELECT
  ? AS org_id,
  raw.stat_date,
  SUM(raw.answersheet_submitted_count),
  SUM(raw.assessment_created_count),
  SUM(raw.report_generated_count),
  SUM(raw.assessment_failed_count)
FROM (
  SELECT DATE(submitted_at) AS stat_date, COUNT(*) AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS assessment_failed_count
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND submitted_at IS NOT NULL AND submitted_at >= ? AND submitted_at < ?
  GROUP BY DATE(submitted_at)
  UNION ALL
  SELECT DATE(created_at) AS stat_date, 0, COUNT(*), 0, 0
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?
  GROUP BY DATE(created_at)
  UNION ALL
  SELECT DATE(interpreted_at) AS stat_date, 0, 0, COUNT(*), 0
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at >= ? AND interpreted_at < ?
  GROUP BY DATE(interpreted_at)
  UNION ALL
  SELECT DATE(failed_at) AS stat_date, 0, 0, 0, COUNT(*)
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND failed_at IS NOT NULL AND failed_at >= ? AND failed_at < ?
  GROUP BY DATE(failed_at)
) raw
GROUP BY raw.stat_date`

const assessmentServiceClinicianDailyInsertSQL = `
INSERT INTO analytics_assessment_service_clinician_daily (
  org_id, clinician_id, stat_date, answersheet_submitted_count, assessment_created_count,
  report_generated_count, assessment_failed_count
)
SELECT
  raw.org_id,
  raw.clinician_id,
  raw.stat_date,
  SUM(raw.answersheet_submitted_count),
  SUM(raw.assessment_created_count),
  SUM(raw.report_generated_count),
  SUM(raw.assessment_failed_count)
FROM (
  SELECT a.org_id, e.clinician_id, DATE(a.submitted_at) AS stat_date, COUNT(*) AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS assessment_failed_count
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.clinician_id IS NOT NULL AND e.clinician_id <> 0 AND a.submitted_at IS NOT NULL AND a.submitted_at >= ? AND a.submitted_at < ?
  GROUP BY a.org_id, e.clinician_id, DATE(a.submitted_at)
  UNION ALL
  SELECT a.org_id, e.clinician_id, DATE(a.created_at) AS stat_date, 0, COUNT(*), 0, 0
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.clinician_id IS NOT NULL AND e.clinician_id <> 0 AND a.created_at >= ? AND a.created_at < ?
  GROUP BY a.org_id, e.clinician_id, DATE(a.created_at)
  UNION ALL
  SELECT a.org_id, e.clinician_id, DATE(a.interpreted_at) AS stat_date, 0, 0, COUNT(*), 0
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.clinician_id IS NOT NULL AND e.clinician_id <> 0 AND a.interpreted_at IS NOT NULL AND a.interpreted_at >= ? AND a.interpreted_at < ?
  GROUP BY a.org_id, e.clinician_id, DATE(a.interpreted_at)
  UNION ALL
  SELECT a.org_id, e.clinician_id, DATE(a.failed_at) AS stat_date, 0, 0, 0, COUNT(*)
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.clinician_id IS NOT NULL AND e.clinician_id <> 0 AND a.failed_at IS NOT NULL AND a.failed_at >= ? AND a.failed_at < ?
  GROUP BY a.org_id, e.clinician_id, DATE(a.failed_at)
) raw
GROUP BY raw.org_id, raw.clinician_id, raw.stat_date`

const assessmentServiceEntryDailyInsertSQL = `
INSERT INTO analytics_assessment_service_entry_daily (
  org_id, entry_id, clinician_id, stat_date, answersheet_submitted_count, assessment_created_count,
  report_generated_count, assessment_failed_count
)
SELECT
  raw.org_id,
  raw.entry_id,
  MAX(raw.clinician_id),
  raw.stat_date,
  SUM(raw.answersheet_submitted_count),
  SUM(raw.assessment_created_count),
  SUM(raw.report_generated_count),
  SUM(raw.assessment_failed_count)
FROM (
  SELECT a.org_id, e.entry_id, COALESCE(e.clinician_id, 0) AS clinician_id, DATE(a.submitted_at) AS stat_date, COUNT(*) AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS assessment_failed_count
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.entry_id IS NOT NULL AND e.entry_id <> 0 AND a.submitted_at IS NOT NULL AND a.submitted_at >= ? AND a.submitted_at < ?
  GROUP BY a.org_id, e.entry_id, e.clinician_id, DATE(a.submitted_at)
  UNION ALL
  SELECT a.org_id, e.entry_id, COALESCE(e.clinician_id, 0) AS clinician_id, DATE(a.created_at) AS stat_date, 0, COUNT(*), 0, 0
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.entry_id IS NOT NULL AND e.entry_id <> 0 AND a.created_at >= ? AND a.created_at < ?
  GROUP BY a.org_id, e.entry_id, e.clinician_id, DATE(a.created_at)
  UNION ALL
  SELECT a.org_id, e.entry_id, COALESCE(e.clinician_id, 0) AS clinician_id, DATE(a.interpreted_at) AS stat_date, 0, 0, COUNT(*), 0
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.entry_id IS NOT NULL AND e.entry_id <> 0 AND a.interpreted_at IS NOT NULL AND a.interpreted_at >= ? AND a.interpreted_at < ?
  GROUP BY a.org_id, e.entry_id, e.clinician_id, DATE(a.interpreted_at)
  UNION ALL
  SELECT a.org_id, e.entry_id, COALESCE(e.clinician_id, 0) AS clinician_id, DATE(a.failed_at) AS stat_date, 0, 0, 0, COUNT(*)
  FROM assessment a
  JOIN assessment_episode e ON e.org_id = a.org_id AND e.answersheet_id = a.answer_sheet_id AND e.deleted_at IS NULL
  WHERE a.org_id = ? AND a.deleted_at IS NULL AND e.entry_id IS NOT NULL AND e.entry_id <> 0 AND a.failed_at IS NOT NULL AND a.failed_at >= ? AND a.failed_at < ?
  GROUP BY a.org_id, e.entry_id, e.clinician_id, DATE(a.failed_at)
) raw
GROUP BY raw.org_id, raw.entry_id, raw.stat_date`

const assessmentServiceContentDailyInsertSQL = `
INSERT INTO analytics_assessment_service_content_daily (
  org_id, content_type, content_code, stat_date, answersheet_submitted_count, assessment_created_count,
  report_generated_count, assessment_failed_count
)
SELECT
  raw.org_id,
  raw.content_type,
  raw.content_code,
  raw.stat_date,
  SUM(raw.answersheet_submitted_count),
  SUM(raw.assessment_created_count),
  SUM(raw.report_generated_count),
  SUM(raw.assessment_failed_count)
FROM (
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END AS content_type, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS content_code, DATE(submitted_at) AS stat_date, COUNT(*) AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS assessment_failed_count
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND submitted_at IS NOT NULL AND submitted_at >= ? AND submitted_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, DATE(submitted_at)
  UNION ALL
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END AS content_type, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS content_code, DATE(created_at) AS stat_date, 0, COUNT(*), 0, 0
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, DATE(created_at)
  UNION ALL
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END AS content_type, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS content_code, DATE(interpreted_at) AS stat_date, 0, 0, COUNT(*), 0
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at >= ? AND interpreted_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, DATE(interpreted_at)
  UNION ALL
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END AS content_type, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS content_code, DATE(failed_at) AS stat_date, 0, 0, 0, COUNT(*)
  FROM assessment
  WHERE org_id = ? AND deleted_at IS NULL AND failed_at IS NOT NULL AND failed_at >= ? AND failed_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, DATE(failed_at)
) raw
GROUP BY raw.org_id, raw.content_type, raw.content_code, raw.stat_date`

const planTaskDailyInsertSQL = `
INSERT INTO analytics_plan_task_daily (
  org_id, plan_id, stat_date, task_created_count, task_opened_count,
  task_completed_count, task_expired_count, enrolled_testees, active_testees
)
SELECT
  raw.org_id,
  raw.plan_id,
  raw.stat_date,
  SUM(raw.task_created_count),
  SUM(raw.task_opened_count),
  SUM(raw.task_completed_count),
  SUM(raw.task_expired_count),
  COUNT(DISTINCT raw.enrolled_testee_id),
  COUNT(DISTINCT raw.active_testee_id)
FROM (
  SELECT t.org_id, t.plan_id, DATE(t.created_at) AS stat_date, 1 AS task_created_count, 0 AS task_opened_count, 0 AS task_completed_count, 0 AS task_expired_count, t.testee_id AS enrolled_testee_id, NULL AS active_testee_id
  FROM assessment_task t
  JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL
  UNION ALL
  SELECT t.org_id, t.plan_id, DATE(t.open_at) AS stat_date, 0, 1, 0, 0, NULL, NULL
  FROM assessment_task t
  JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.open_at IS NOT NULL
  UNION ALL
  SELECT t.org_id, t.plan_id, DATE(t.completed_at) AS stat_date, 0, 0, 1, 0, NULL, t.testee_id
  FROM assessment_task t
  JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.completed_at IS NOT NULL
  UNION ALL
  SELECT t.org_id, t.plan_id, DATE(t.expire_at) AS stat_date, 0, 0, 0, 1, NULL, NULL
  FROM assessment_task t
  JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.expire_at IS NOT NULL AND t.status COLLATE utf8mb4_unicode_ci = 'expired' COLLATE utf8mb4_unicode_ci
) raw
GROUP BY raw.org_id, raw.plan_id, raw.stat_date`
