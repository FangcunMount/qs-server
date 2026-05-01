package statistics

import (
	"context"
	"time"

	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
	"gorm.io/gorm"
)

func (r *StatisticsRepository) RebuildDailyStatistics(ctx context.Context, orgID int64, startDate, endDate time.Time) error {
	tx, err := gormuow.RequireTx(ctx)
	if err != nil {
		return err
	}
	if err := deleteDailyWindow(ctx, tx, "statistics_journey_daily", orgID, startDate, endDate); err != nil {
		return err
	}
	if err := deleteDailyWindow(ctx, tx, "statistics_content_daily", orgID, startDate, endDate); err != nil {
		return err
	}
	if err := r.rebuildJourneyProjectionDaily(ctx, tx, orgID, startDate, endDate); err != nil {
		return err
	}
	if err := r.rebuildAccessFunnelDaily(ctx, tx, orgID, startDate, endDate); err != nil {
		return err
	}
	if err := r.rebuildAssessmentServiceDaily(ctx, tx, orgID, startDate, endDate); err != nil {
		return err
	}
	return r.rebuildContentDaily(ctx, tx, orgID, startDate, endDate)
}

func (r *StatisticsRepository) RebuildAccumulatedStatistics(ctx context.Context, orgID int64, _ time.Time) error {
	tx, err := gormuow.RequireTx(ctx)
	if err != nil {
		return err
	}
	return r.rebuildOrgSnapshot(ctx, tx, orgID, time.Now().In(time.Local))
}

func (r *StatisticsRepository) RebuildPlanStatistics(ctx context.Context, orgID int64) error {
	tx, err := gormuow.RequireTx(ctx)
	if err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec("DELETE FROM statistics_plan_daily WHERE org_id = ?", orgID).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(planDailyInsertSQL, orgID, orgID, orgID, orgID).Error
}

func (r *StatisticsRepository) rebuildJourneyProjectionDaily(ctx context.Context, tx *gorm.DB, orgID int64, startDate, endDate time.Time) error {
	if err := tx.WithContext(ctx).Exec(journeyProjectionOrgInsertSQL,
		orgID, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate,
		orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Exec(journeyProjectionClinicianInsertSQL,
		orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate,
		orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(journeyProjectionEntryInsertSQL,
		orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate,
		orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate, orgID, startDate, endDate,
	).Error
}

func (r *StatisticsRepository) rebuildAccessFunnelDaily(ctx context.Context, tx *gorm.DB, orgID int64, startDate, endDate time.Time) error {
	return tx.WithContext(ctx).Exec(accessFunnelOrgInsertSQL, orgID,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error
}

func (r *StatisticsRepository) rebuildAssessmentServiceDaily(ctx context.Context, tx *gorm.DB, orgID int64, startDate, endDate time.Time) error {
	return tx.WithContext(ctx).Exec(assessmentServiceOrgInsertSQL, orgID,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error
}

func (r *StatisticsRepository) rebuildContentDaily(ctx context.Context, tx *gorm.DB, orgID int64, startDate, endDate time.Time) error {
	return tx.WithContext(ctx).Exec(contentDailyInsertSQL,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error
}

func (r *StatisticsRepository) rebuildOrgSnapshot(ctx context.Context, tx *gorm.DB, orgID int64, snapshotAt time.Time) error {
	if err := tx.WithContext(ctx).Exec("DELETE FROM statistics_org_snapshot WHERE org_id = ?", orgID).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Exec(orgSnapshotInsertSQL,
		orgID,
		orgID,
		orgID,
		orgID, snapshotAt,
		orgID,
		orgID,
		orgID,
		orgID,
		orgID,
		snapshotAt,
	).Error
}

func deleteDailyWindow(ctx context.Context, tx *gorm.DB, table string, orgID int64, startDate, endDate time.Time) error {
	return tx.WithContext(ctx).
		Exec("DELETE FROM "+table+" WHERE org_id = ? AND stat_date >= ? AND stat_date < ?", orgID, startDate, endDate).
		Error
}

const journeyProjectionOrgInsertSQL = `
INSERT INTO statistics_journey_daily (
  org_id, subject_type, subject_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count, assessment_failed_count
)
SELECT
  ? AS org_id, 'org' AS subject_type, 0 AS subject_id, raw.stat_date,
  SUM(raw.entry_opened_count), SUM(raw.intake_confirmed_count), SUM(raw.testee_profile_created_count),
  SUM(raw.care_relationship_established_count), SUM(raw.care_relationship_transferred_count),
  SUM(raw.answersheet_submitted_count), SUM(raw.assessment_created_count), SUM(raw.report_generated_count),
  SUM(raw.episode_completed_count), SUM(raw.episode_failed_count), SUM(raw.assessment_failed_count)
FROM (
  SELECT DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count, 0 AS assessment_failed_count
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'entry_opened' AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'intake_confirmed' AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'testee_profile_created' AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'care_relationship_established' AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT DATE(occurred_at), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'care_relationship_transferred' AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND submitted_at >= ? AND submitted_at < ?
  UNION ALL SELECT DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND assessment_created_at IS NOT NULL AND assessment_created_at >= ? AND assessment_created_at < ?
  UNION ALL SELECT DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND report_generated_at IS NOT NULL AND report_generated_at >= ? AND report_generated_at < ?
  UNION ALL SELECT DATE(failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND failed_at IS NOT NULL AND failed_at >= ? AND failed_at < ?
) raw
GROUP BY raw.stat_date
ON DUPLICATE KEY UPDATE
  entry_opened_count = VALUES(entry_opened_count),
  intake_confirmed_count = VALUES(intake_confirmed_count),
  testee_profile_created_count = VALUES(testee_profile_created_count),
  care_relationship_established_count = VALUES(care_relationship_established_count),
  care_relationship_transferred_count = VALUES(care_relationship_transferred_count),
  answersheet_submitted_count = VALUES(answersheet_submitted_count),
  assessment_created_count = VALUES(assessment_created_count),
  report_generated_count = VALUES(report_generated_count),
  episode_completed_count = VALUES(episode_completed_count),
  episode_failed_count = VALUES(episode_failed_count),
  assessment_failed_count = VALUES(assessment_failed_count),
  updated_at = NOW(3)`

const journeyProjectionClinicianInsertSQL = `
INSERT INTO statistics_journey_daily (
  org_id, subject_type, subject_id, clinician_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, care_relationship_transferred_count,
  answersheet_submitted_count, assessment_created_count, report_generated_count,
  episode_completed_count, episode_failed_count, assessment_failed_count
)
SELECT
  raw.org_id, 'clinician', raw.clinician_id, raw.clinician_id, raw.stat_date,
  SUM(raw.entry_opened_count), SUM(raw.intake_confirmed_count), SUM(raw.testee_profile_created_count),
  SUM(raw.care_relationship_established_count), SUM(raw.care_relationship_transferred_count),
  SUM(raw.answersheet_submitted_count), SUM(raw.assessment_created_count), SUM(raw.report_generated_count),
  SUM(raw.episode_completed_count), SUM(raw.episode_failed_count), SUM(raw.assessment_failed_count)
FROM (
  SELECT org_id, clinician_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS care_relationship_transferred_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count, 0 AS assessment_failed_count
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'entry_opened' AND clinician_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'intake_confirmed' AND clinician_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'testee_profile_created' AND clinician_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'care_relationship_established' AND clinician_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(occurred_at), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'care_relationship_transferred' AND clinician_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(submitted_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND submitted_at >= ? AND submitted_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(assessment_created_at), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND assessment_created_at IS NOT NULL AND assessment_created_at >= ? AND assessment_created_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND report_generated_at IS NOT NULL AND report_generated_at >= ? AND report_generated_at < ?
  UNION ALL SELECT org_id, clinician_id, DATE(failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND clinician_id IS NOT NULL AND clinician_id <> 0 AND failed_at IS NOT NULL AND failed_at >= ? AND failed_at < ?
) raw
GROUP BY raw.org_id, raw.clinician_id, raw.stat_date
ON DUPLICATE KEY UPDATE
  clinician_id = VALUES(clinician_id),
  entry_opened_count = VALUES(entry_opened_count),
  intake_confirmed_count = VALUES(intake_confirmed_count),
  testee_profile_created_count = VALUES(testee_profile_created_count),
  care_relationship_established_count = VALUES(care_relationship_established_count),
  care_relationship_transferred_count = VALUES(care_relationship_transferred_count),
  answersheet_submitted_count = VALUES(answersheet_submitted_count),
  assessment_created_count = VALUES(assessment_created_count),
  report_generated_count = VALUES(report_generated_count),
  episode_completed_count = VALUES(episode_completed_count),
  episode_failed_count = VALUES(episode_failed_count),
  assessment_failed_count = VALUES(assessment_failed_count),
  updated_at = NOW(3)`

const journeyProjectionEntryInsertSQL = `
INSERT INTO statistics_journey_daily (
  org_id, subject_type, subject_id, clinician_id, entry_id, stat_date,
  entry_opened_count, intake_confirmed_count, testee_profile_created_count,
  care_relationship_established_count, answersheet_submitted_count,
  assessment_created_count, report_generated_count, episode_completed_count,
  episode_failed_count, assessment_failed_count
)
SELECT
  raw.org_id, 'entry', raw.entry_id, MAX(raw.clinician_id), raw.entry_id, raw.stat_date,
  SUM(raw.entry_opened_count), SUM(raw.intake_confirmed_count), SUM(raw.testee_profile_created_count),
  SUM(raw.care_relationship_established_count), SUM(raw.answersheet_submitted_count),
  SUM(raw.assessment_created_count), SUM(raw.report_generated_count), SUM(raw.episode_completed_count),
  SUM(raw.episode_failed_count), SUM(raw.assessment_failed_count)
FROM (
  SELECT org_id, entry_id, clinician_id, DATE(occurred_at) AS stat_date, 1 AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_profile_created_count, 0 AS care_relationship_established_count, 0 AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS episode_completed_count, 0 AS episode_failed_count, 0 AS assessment_failed_count
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'entry_opened' AND entry_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'intake_confirmed' AND entry_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'testee_profile_created' AND entry_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, entry_id, clinician_id, DATE(occurred_at), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM behavior_footprint WHERE org_id = ? AND deleted_at IS NULL AND event_name = 'care_relationship_established' AND entry_id <> 0 AND occurred_at >= ? AND occurred_at < ?
  UNION ALL SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(submitted_at), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND entry_id IS NOT NULL AND entry_id <> 0 AND submitted_at >= ? AND submitted_at < ?
  UNION ALL SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(assessment_created_at), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND entry_id IS NOT NULL AND entry_id <> 0 AND assessment_created_at IS NOT NULL AND assessment_created_at >= ? AND assessment_created_at < ?
  UNION ALL SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(report_generated_at), 0, 0, 0, 0, 0, 0, 1, 1, 0, 0
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND entry_id IS NOT NULL AND entry_id <> 0 AND report_generated_at IS NOT NULL AND report_generated_at >= ? AND report_generated_at < ?
  UNION ALL SELECT org_id, entry_id, COALESCE(clinician_id, 0), DATE(failed_at), 0, 0, 0, 0, 0, 0, 0, 0, 1, 1
  FROM assessment_episode WHERE org_id = ? AND deleted_at IS NULL AND entry_id IS NOT NULL AND entry_id <> 0 AND failed_at IS NOT NULL AND failed_at >= ? AND failed_at < ?
) raw
GROUP BY raw.org_id, raw.entry_id, raw.stat_date
ON DUPLICATE KEY UPDATE
  clinician_id = VALUES(clinician_id),
  entry_id = VALUES(entry_id),
  entry_opened_count = VALUES(entry_opened_count),
  intake_confirmed_count = VALUES(intake_confirmed_count),
  testee_profile_created_count = VALUES(testee_profile_created_count),
  care_relationship_established_count = VALUES(care_relationship_established_count),
  answersheet_submitted_count = VALUES(answersheet_submitted_count),
  assessment_created_count = VALUES(assessment_created_count),
  report_generated_count = VALUES(report_generated_count),
  episode_completed_count = VALUES(episode_completed_count),
  episode_failed_count = VALUES(episode_failed_count),
  assessment_failed_count = VALUES(assessment_failed_count),
  updated_at = NOW(3)`

const accessFunnelOrgInsertSQL = `
INSERT INTO statistics_journey_daily (
  org_id, subject_type, subject_id, stat_date,
  access_entry_opened_count, access_intake_confirmed_count,
  access_testee_created_count, access_care_relationship_established_count
)
SELECT
  ? AS org_id, 'org', 0, raw.stat_date,
  SUM(raw.entry_opened_count), SUM(raw.intake_confirmed_count),
  SUM(raw.testee_created_count), SUM(raw.care_relationship_established_count)
FROM (
  SELECT DATE(resolved_at) AS stat_date, COUNT(*) AS entry_opened_count, 0 AS intake_confirmed_count, 0 AS testee_created_count, 0 AS care_relationship_established_count
  FROM assessment_entry_resolve_log WHERE org_id = ? AND deleted_at IS NULL AND resolved_at >= ? AND resolved_at < ? GROUP BY DATE(resolved_at)
  UNION ALL SELECT DATE(intake_at), 0, COUNT(*), 0, 0
  FROM assessment_entry_intake_log WHERE org_id = ? AND deleted_at IS NULL AND intake_at >= ? AND intake_at < ? GROUP BY DATE(intake_at)
  UNION ALL SELECT DATE(created_at), 0, 0, COUNT(*), 0
  FROM testee WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ? GROUP BY DATE(created_at)
  UNION ALL SELECT DATE(bound_at), 0, 0, 0, COUNT(DISTINCT testee_id)
  FROM clinician_relation WHERE org_id = ? AND deleted_at IS NULL AND bound_at >= ? AND bound_at < ? GROUP BY DATE(bound_at)
) raw
GROUP BY raw.stat_date
ON DUPLICATE KEY UPDATE
  access_entry_opened_count = VALUES(access_entry_opened_count),
  access_intake_confirmed_count = VALUES(access_intake_confirmed_count),
  access_testee_created_count = VALUES(access_testee_created_count),
  access_care_relationship_established_count = VALUES(access_care_relationship_established_count),
  updated_at = NOW(3)`

const assessmentServiceOrgInsertSQL = `
INSERT INTO statistics_journey_daily (
  org_id, subject_type, subject_id, stat_date,
  service_answersheet_submitted_count, service_assessment_created_count,
  service_report_generated_count, service_assessment_failed_count
)
SELECT
  ? AS org_id, 'org', 0, raw.stat_date,
  SUM(raw.answersheet_submitted_count), SUM(raw.assessment_created_count),
  SUM(raw.report_generated_count), SUM(raw.assessment_failed_count)
FROM (
  SELECT DATE(submitted_at) AS stat_date, COUNT(*) AS answersheet_submitted_count, 0 AS assessment_created_count, 0 AS report_generated_count, 0 AS assessment_failed_count
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND submitted_at IS NOT NULL AND submitted_at >= ? AND submitted_at < ? GROUP BY DATE(submitted_at)
  UNION ALL SELECT DATE(created_at), 0, COUNT(*), 0, 0
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ? GROUP BY DATE(created_at)
  UNION ALL SELECT DATE(interpreted_at), 0, 0, COUNT(*), 0
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at >= ? AND interpreted_at < ? GROUP BY DATE(interpreted_at)
  UNION ALL SELECT DATE(failed_at), 0, 0, 0, COUNT(*)
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND failed_at IS NOT NULL AND failed_at >= ? AND failed_at < ? GROUP BY DATE(failed_at)
) raw
GROUP BY raw.stat_date
ON DUPLICATE KEY UPDATE
  service_answersheet_submitted_count = VALUES(service_answersheet_submitted_count),
  service_assessment_created_count = VALUES(service_assessment_created_count),
  service_report_generated_count = VALUES(service_report_generated_count),
  service_assessment_failed_count = VALUES(service_assessment_failed_count),
  updated_at = NOW(3)`

const contentDailyInsertSQL = `
INSERT INTO statistics_content_daily (
  org_id, content_type, content_code, origin_type, stat_date,
  submission_count, completion_count, answersheet_submitted_count,
  assessment_created_count, report_generated_count, assessment_failed_count
)
SELECT
  raw.org_id, raw.content_type, raw.content_code, raw.origin_type, raw.stat_date,
  SUM(raw.submission_count), SUM(raw.completion_count), SUM(raw.answersheet_submitted_count),
  SUM(raw.assessment_created_count), SUM(raw.report_generated_count), SUM(raw.assessment_failed_count)
FROM (
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END AS content_type, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) AS content_code, COALESCE(origin_type, '') AS origin_type, DATE(created_at) AS stat_date, COUNT(*) AS submission_count, 0 AS completion_count, 0 AS answersheet_submitted_count, COUNT(*) AS assessment_created_count, 0 AS report_generated_count, 0 AS assessment_failed_count
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, origin_type, DATE(created_at)
  UNION ALL
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code), COALESCE(origin_type, ''), DATE(interpreted_at), 0, COUNT(*), 0, 0, COUNT(*), 0
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at >= ? AND interpreted_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, origin_type, DATE(interpreted_at)
  UNION ALL
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code), COALESCE(origin_type, ''), DATE(submitted_at), 0, 0, COUNT(*), 0, 0, 0
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND submitted_at IS NOT NULL AND submitted_at >= ? AND submitted_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, origin_type, DATE(submitted_at)
  UNION ALL
  SELECT org_id, CASE WHEN COALESCE(medical_scale_code, '') <> '' THEN 'scale' ELSE 'questionnaire' END, COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code), COALESCE(origin_type, ''), DATE(failed_at), 0, 0, 0, 0, 0, COUNT(*)
  FROM assessment WHERE org_id = ? AND deleted_at IS NULL AND failed_at IS NOT NULL AND failed_at >= ? AND failed_at < ? AND COALESCE(NULLIF(medical_scale_code, ''), questionnaire_code) <> ''
  GROUP BY org_id, content_type, content_code, origin_type, DATE(failed_at)
) raw
GROUP BY raw.org_id, raw.content_type, raw.content_code, raw.origin_type, raw.stat_date
ON DUPLICATE KEY UPDATE
  submission_count = VALUES(submission_count),
  completion_count = VALUES(completion_count),
  answersheet_submitted_count = VALUES(answersheet_submitted_count),
  assessment_created_count = VALUES(assessment_created_count),
  report_generated_count = VALUES(report_generated_count),
  assessment_failed_count = VALUES(assessment_failed_count),
  updated_at = NOW(3)`

const planDailyInsertSQL = `
INSERT INTO statistics_plan_daily (
  org_id, plan_id, stat_date, task_created_count, task_opened_count,
  task_completed_count, task_expired_count, enrolled_testees, active_testees
)
SELECT
  raw.org_id, raw.plan_id, raw.stat_date,
  SUM(raw.task_created_count), SUM(raw.task_opened_count),
  SUM(raw.task_completed_count), SUM(raw.task_expired_count),
  COUNT(DISTINCT raw.enrolled_testee_id), COUNT(DISTINCT raw.active_testee_id)
FROM (
  SELECT t.org_id, t.plan_id, DATE(t.created_at) AS stat_date, 1 AS task_created_count, 0 AS task_opened_count, 0 AS task_completed_count, 0 AS task_expired_count, t.testee_id AS enrolled_testee_id, NULL AS active_testee_id
  FROM assessment_task t JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL
  UNION ALL
  SELECT t.org_id, t.plan_id, DATE(t.open_at), 0, 1, 0, 0, NULL, NULL
  FROM assessment_task t JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.open_at IS NOT NULL
  UNION ALL
  SELECT t.org_id, t.plan_id, DATE(t.completed_at), 0, 0, 1, 0, NULL, t.testee_id
  FROM assessment_task t JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.completed_at IS NOT NULL
  UNION ALL
  SELECT t.org_id, t.plan_id, DATE(t.expire_at), 0, 0, 0, 1, NULL, NULL
  FROM assessment_task t JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
  WHERE t.org_id = ? AND t.deleted_at IS NULL AND t.expire_at IS NOT NULL AND t.status COLLATE utf8mb4_unicode_ci = 'expired' COLLATE utf8mb4_unicode_ci
) raw
GROUP BY raw.org_id, raw.plan_id, raw.stat_date`

const orgSnapshotInsertSQL = `
INSERT INTO statistics_org_snapshot (
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
