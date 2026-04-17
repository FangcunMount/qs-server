package main

import (
	"context"
	"fmt"
	"strings"

	actorMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	statisticsMySQL "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	"gorm.io/gorm"
)

type journeyHistoryRebuildStats struct {
	DeletedFootprints                   int64
	DeletedEpisodes                     int64
	EntryOpenedInserted                 int64
	IntakeConfirmedInserted             int64
	TesteeProfileCreatedInserted        int64
	CareEstablishedFromIntakeInserted   int64
	CareEstablishedFromRelationInserted int64
	CareTransferredInserted             int64
	EpisodesInserted                    int64
	EpisodesAttributedFromCreator       int64
	EpisodesAttributedFromIntake        int64
	EpisodesAttributedFromRelation      int64
	AnswerSheetSubmittedInserted        int64
	AssessmentCreatedInserted           int64
	ReportGeneratedInserted             int64
	BehaviorFootprintRows               int64
	AssessmentEpisodeRows               int64
}

func seedJourneyRebuildHistory(ctx context.Context, deps *dependencies) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.Config.Global.OrgID == 0 {
		return fmt.Errorf("global.orgId is required for journey_rebuild_history")
	}
	if strings.TrimSpace(deps.Config.Local.MySQLDSN) == "" {
		return fmt.Errorf("seeddata local.mysql_dsn is required for journey_rebuild_history")
	}

	mysqlDB, err := openLocalSeedMySQL(deps.Config.Local.MySQLDSN)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeLocalSeedMySQL(mysqlDB); closeErr != nil {
			deps.Logger.Warnw("Failed to close local mysql after journey rebuild history", "error", closeErr.Error())
		}
	}()

	deps.Logger.Infow("Journey rebuild history started",
		"org_id", deps.Config.Global.OrgID,
		"mode", "rebuild_behavior_footprint_and_assessment_episode_from_existing_history",
	)

	progress := newSeedProgressBar("journey_rebuild_history", 4)
	defer progress.Close()

	pendingCount, err := waitForAnalyticsProjectorIdle(ctx, mysqlDB, deps)
	if err != nil {
		return err
	}
	progress.Increment()

	sourceStats, err := countJourneyHistorySourceRows(ctx, mysqlDB, deps.Config.Global.OrgID)
	if err != nil {
		return err
	}
	deps.Logger.Infow("Journey rebuild history source snapshot",
		"org_id", deps.Config.Global.OrgID,
		"pending_event_count", pendingCount,
		"entry_resolve_logs", sourceStats.EntryOpenedInserted,
		"entry_intake_logs", sourceStats.IntakeConfirmedInserted,
		"relation_rows", sourceStats.CareEstablishedFromRelationInserted+sourceStats.CareTransferredInserted,
		"assessments", sourceStats.EpisodesInserted,
	)
	progress.Increment()

	stats, err := rebuildJourneyRuntimeTables(ctx, mysqlDB, deps.Config.Global.OrgID)
	if err != nil {
		return err
	}
	progress.Increment()

	progress.Complete()
	deps.Logger.Infow("Journey rebuild history completed",
		"org_id", deps.Config.Global.OrgID,
		"deleted_footprints", stats.DeletedFootprints,
		"deleted_episodes", stats.DeletedEpisodes,
		"entry_opened_inserted", stats.EntryOpenedInserted,
		"intake_confirmed_inserted", stats.IntakeConfirmedInserted,
		"testee_profile_created_inserted", stats.TesteeProfileCreatedInserted,
		"care_relationship_established_from_intake_inserted", stats.CareEstablishedFromIntakeInserted,
		"care_relationship_established_from_relation_inserted", stats.CareEstablishedFromRelationInserted,
		"care_relationship_transferred_inserted", stats.CareTransferredInserted,
		"episodes_inserted", stats.EpisodesInserted,
		"episodes_attributed_from_creator", stats.EpisodesAttributedFromCreator,
		"episodes_attributed_from_intake", stats.EpisodesAttributedFromIntake,
		"episodes_attributed_from_relation", stats.EpisodesAttributedFromRelation,
		"answersheet_submitted_inserted", stats.AnswerSheetSubmittedInserted,
		"assessment_created_inserted", stats.AssessmentCreatedInserted,
		"report_generated_inserted", stats.ReportGeneratedInserted,
		"behavior_footprint_rows", stats.BehaviorFootprintRows,
		"assessment_episode_rows", stats.AssessmentEpisodeRows,
		"next_step", "statistics_backfill",
	)
	return nil
}

func countJourneyHistorySourceRows(ctx context.Context, mysqlDB *gorm.DB, orgID int64) (journeyHistoryRebuildStats, error) {
	var stats journeyHistoryRebuildStats
	if err := mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AssessmentEntryResolveLogPO{}).TableName()).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&stats.EntryOpenedInserted).Error; err != nil {
		return stats, fmt.Errorf("count assessment_entry_resolve_log: %w", err)
	}
	if err := mysqlDB.WithContext(ctx).
		Table((statisticsMySQL.AssessmentEntryIntakeLogPO{}).TableName()).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&stats.IntakeConfirmedInserted).Error; err != nil {
		return stats, fmt.Errorf("count assessment_entry_intake_log: %w", err)
	}
	if err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Where("relation_type IN ?", journeyHistoryRebuildAccessGrantRelationTypes()).
		Where("(source_type IS NULL OR source_type <> ?)", "transfer").
		Count(&stats.CareEstablishedFromRelationInserted).Error; err != nil {
		return stats, fmt.Errorf("count clinician_relation established candidates: %w", err)
	}
	if err := mysqlDB.WithContext(ctx).
		Table((actorMySQL.ClinicianRelationPO{}).TableName()).
		Where("org_id = ? AND deleted_at IS NULL AND source_type = ?", orgID, "transfer").
		Where("relation_type IN ?", journeyHistoryRebuildAccessGrantRelationTypes()).
		Count(&stats.CareTransferredInserted).Error; err != nil {
		return stats, fmt.Errorf("count clinician_relation transfer candidates: %w", err)
	}
	if err := mysqlDB.WithContext(ctx).
		Table((evaluationMySQL.AssessmentPO{}).TableName()).
		Where("org_id = ? AND deleted_at IS NULL AND answer_sheet_id <> 0", orgID).
		Count(&stats.EpisodesInserted).Error; err != nil {
		return stats, fmt.Errorf("count assessment candidates: %w", err)
	}
	return stats, nil
}

func rebuildJourneyRuntimeTables(ctx context.Context, mysqlDB *gorm.DB, orgID int64) (journeyHistoryRebuildStats, error) {
	stats := journeyHistoryRebuildStats{}
	tables := journeyHistoryRebuildTables()
	statements := []struct {
		name string
		sql  string
		args []interface{}
		dest *int64
	}{
		{
			name: "delete behavior_footprint",
			sql:  fmt.Sprintf("DELETE FROM %s WHERE org_id = ?", tables.BehaviorFootprint),
			args: []interface{}{orgID},
			dest: &stats.DeletedFootprints,
		},
		{
			name: "delete assessment_episode",
			sql:  fmt.Sprintf("DELETE FROM %s WHERE org_id = ?", tables.AssessmentEpisode),
			args: []interface{}{orgID},
			dest: &stats.DeletedEpisodes,
		},
		{
			name: "insert entry_opened footprints",
			sql:  buildJourneyHistoryEntryOpenedSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.EntryOpenedInserted,
		},
		{
			name: "insert intake_confirmed footprints",
			sql:  buildJourneyHistoryIntakeConfirmedSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.IntakeConfirmedInserted,
		},
		{
			name: "insert testee_profile_created footprints",
			sql:  buildJourneyHistoryTesteeProfileCreatedSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.TesteeProfileCreatedInserted,
		},
		{
			name: "insert care_relationship_established footprints from intake",
			sql:  buildJourneyHistoryCareEstablishedFromIntakeSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.CareEstablishedFromIntakeInserted,
		},
		{
			name: "insert care_relationship_established footprints from relation",
			sql:  buildJourneyHistoryCareEstablishedFromRelationSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.CareEstablishedFromRelationInserted,
		},
		{
			name: "insert care_relationship_transferred footprints",
			sql:  buildJourneyHistoryCareTransferredSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.CareTransferredInserted,
		},
		{
			name: "insert assessment_episode",
			sql:  buildJourneyHistoryAssessmentEpisodesSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.EpisodesInserted,
		},
		{
			name: "attribute assessment_episode from creator relation",
			sql:  buildJourneyHistoryEpisodeAttributionFromCreatorSQL(tables),
			args: []interface{}{orgID, orgID},
			dest: &stats.EpisodesAttributedFromCreator,
		},
		{
			name: "attribute assessment_episode from intake log",
			sql:  buildJourneyHistoryEpisodeAttributionFromIntakeSQL(tables),
			args: []interface{}{orgID, orgID},
			dest: &stats.EpisodesAttributedFromIntake,
		},
		{
			name: "attribute assessment_episode from clinician relation",
			sql:  buildJourneyHistoryEpisodeAttributionFromRelationSQL(tables),
			args: []interface{}{orgID, orgID},
			dest: &stats.EpisodesAttributedFromRelation,
		},
		{
			name: "insert answersheet_submitted footprints",
			sql:  buildJourneyHistoryAnswerSheetSubmittedSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.AnswerSheetSubmittedInserted,
		},
		{
			name: "insert assessment_created footprints",
			sql:  buildJourneyHistoryAssessmentCreatedSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.AssessmentCreatedInserted,
		},
		{
			name: "insert report_generated footprints",
			sql:  buildJourneyHistoryReportGeneratedSQL(tables),
			args: []interface{}{orgID},
			dest: &stats.ReportGeneratedInserted,
		},
	}

	err := mysqlDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, stmt := range statements {
			result := tx.Exec(stmt.sql, stmt.args...)
			if result.Error != nil {
				return fmt.Errorf("%s: %w", stmt.name, result.Error)
			}
			if stmt.dest != nil {
				*stmt.dest = result.RowsAffected
			}
		}
		return nil
	})
	if err != nil {
		return stats, err
	}

	if err := mysqlDB.WithContext(ctx).
		Table(tables.BehaviorFootprint).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&stats.BehaviorFootprintRows).Error; err != nil {
		return stats, fmt.Errorf("count rebuilt behavior_footprint rows: %w", err)
	}
	if err := mysqlDB.WithContext(ctx).
		Table(tables.AssessmentEpisode).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&stats.AssessmentEpisodeRows).Error; err != nil {
		return stats, fmt.Errorf("count rebuilt assessment_episode rows: %w", err)
	}
	return stats, nil
}

type journeyHistoryRebuildTableNames struct {
	BehaviorFootprint          string
	AssessmentEpisode          string
	AssessmentEntryResolveLog  string
	AssessmentEntryIntakeLog   string
	ClinicianRelation          string
	Assessment                 string
}

func journeyHistoryRebuildTables() journeyHistoryRebuildTableNames {
	return journeyHistoryRebuildTableNames{
		BehaviorFootprint:         (statisticsMySQL.BehaviorFootprintPO{}).TableName(),
		AssessmentEpisode:         (statisticsMySQL.AssessmentEpisodePO{}).TableName(),
		AssessmentEntryResolveLog: (statisticsMySQL.AssessmentEntryResolveLogPO{}).TableName(),
		AssessmentEntryIntakeLog:  (statisticsMySQL.AssessmentEntryIntakeLogPO{}).TableName(),
		ClinicianRelation:         (actorMySQL.ClinicianRelationPO{}).TableName(),
		Assessment:                (evaluationMySQL.AssessmentPO{}).TableName(),
	}
}

func journeyHistoryRebuildAccessGrantRelationTypes() []string {
	return []string{"assigned", "primary", "attending", "collaborator"}
}

func journeyHistoryRebuildAccessGrantRelationTypesSQL() string {
	return "'assigned','primary','attending','collaborator'"
}

func buildJourneyHistoryEntryOpenedSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:entry_opened:resolve_log:', l.id),
  l.org_id,
  'assessment_entry',
  l.entry_id,
  'assessment_entry',
  l.entry_id,
  l.entry_id,
  l.clinician_id,
  0,
  0,
  0,
  0,
  0,
  'entry_opened',
  l.resolved_at,
  JSON_OBJECT('history_source', 'assessment_entry_resolve_log', 'history_id', l.id),
  NOW(3),
  NOW(3)
FROM %s l
WHERE l.org_id = ?
  AND l.deleted_at IS NULL
`, t.BehaviorFootprint, t.AssessmentEntryResolveLog)
}

func buildJourneyHistoryIntakeConfirmedSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:intake_confirmed:intake_log:', l.id),
  l.org_id,
  'testee',
  l.testee_id,
  'clinician',
  l.clinician_id,
  l.entry_id,
  l.clinician_id,
  0,
  l.testee_id,
  0,
  0,
  0,
  'intake_confirmed',
  l.intake_at,
  JSON_OBJECT('history_source', 'assessment_entry_intake_log', 'history_id', l.id),
  NOW(3),
  NOW(3)
FROM %s l
WHERE l.org_id = ?
  AND l.deleted_at IS NULL
`, t.BehaviorFootprint, t.AssessmentEntryIntakeLog)
}

func buildJourneyHistoryTesteeProfileCreatedSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:testee_profile_created:intake_log:', l.id),
  l.org_id,
  'testee',
  l.testee_id,
  'clinician',
  l.clinician_id,
  l.entry_id,
  l.clinician_id,
  0,
  l.testee_id,
  0,
  0,
  0,
  'testee_profile_created',
  l.intake_at,
  JSON_OBJECT('history_source', 'assessment_entry_intake_log', 'history_id', l.id),
  NOW(3),
  NOW(3)
FROM %s l
WHERE l.org_id = ?
  AND l.deleted_at IS NULL
  AND l.testee_created = 1
`, t.BehaviorFootprint, t.AssessmentEntryIntakeLog)
}

func buildJourneyHistoryCareEstablishedFromIntakeSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:care_relationship_established:intake_log:', l.id),
  l.org_id,
  'testee',
  l.testee_id,
  'clinician',
  l.clinician_id,
  l.entry_id,
  l.clinician_id,
  0,
  l.testee_id,
  0,
  0,
  0,
  'care_relationship_established',
  l.intake_at,
  JSON_OBJECT('history_source', 'assessment_entry_intake_log', 'history_id', l.id),
  NOW(3),
  NOW(3)
FROM %s l
WHERE l.org_id = ?
  AND l.deleted_at IS NULL
  AND l.assignment_created = 1
`, t.BehaviorFootprint, t.AssessmentEntryIntakeLog)
}

func buildJourneyHistoryCareEstablishedFromRelationSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:care_relationship_established:relation:', cr.id),
  cr.org_id,
  'testee',
  cr.testee_id,
  'clinician',
  cr.clinician_id,
  CASE WHEN cr.source_type = 'assessment_entry' THEN COALESCE(cr.source_id, 0) ELSE 0 END,
  cr.clinician_id,
  0,
  cr.testee_id,
  0,
  0,
  0,
  'care_relationship_established',
  cr.bound_at,
  JSON_OBJECT('history_source', 'clinician_relation', 'history_id', cr.id, 'relation_type', cr.relation_type, 'source_type', cr.source_type),
  NOW(3),
  NOW(3)
FROM %s cr
WHERE cr.org_id = ?
  AND cr.deleted_at IS NULL
  AND cr.relation_type IN (%s)
  AND (cr.source_type IS NULL OR (cr.source_type <> 'assessment_entry' AND cr.source_type <> 'transfer'))
`, t.BehaviorFootprint, t.ClinicianRelation, journeyHistoryRebuildAccessGrantRelationTypesSQL())
}

func buildJourneyHistoryCareTransferredSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:care_relationship_transferred:relation:', cr.id),
  cr.org_id,
  'testee',
  cr.testee_id,
  'clinician',
  cr.clinician_id,
  0,
  cr.clinician_id,
  COALESCE((
    SELECT prev.clinician_id
    FROM %s prev
    WHERE prev.org_id = cr.org_id
      AND prev.testee_id = cr.testee_id
      AND prev.deleted_at IS NULL
      AND prev.id <> cr.id
      AND prev.relation_type IN (%s)
      AND prev.bound_at <= cr.bound_at
    ORDER BY COALESCE(prev.unbound_at, prev.bound_at) DESC, prev.id DESC
    LIMIT 1
  ), 0),
  cr.testee_id,
  0,
  0,
  0,
  'care_relationship_transferred',
  cr.bound_at,
  JSON_OBJECT('history_source', 'clinician_relation', 'history_id', cr.id, 'relation_type', cr.relation_type, 'source_type', cr.source_type),
  NOW(3),
  NOW(3)
FROM %s cr
WHERE cr.org_id = ?
  AND cr.deleted_at IS NULL
  AND cr.source_type = 'transfer'
  AND cr.relation_type IN (%s)
`, t.BehaviorFootprint, t.ClinicianRelation, journeyHistoryRebuildAccessGrantRelationTypesSQL(), t.ClinicianRelation, journeyHistoryRebuildAccessGrantRelationTypesSQL())
}

func buildJourneyHistoryAssessmentEpisodesSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  episode_id, org_id, entry_id, clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, attributed_intake_at, submitted_at,
  assessment_created_at, report_generated_at, failed_at, status, failure_reason,
  created_at, updated_at
)
SELECT
  a.answer_sheet_id,
  a.org_id,
  NULL,
  NULL,
  a.testee_id,
  a.answer_sheet_id,
  a.id,
  CASE
    WHEN a.interpreted_at IS NOT NULL OR a.status = 'interpreted' THEN a.id
    ELSE NULL
  END,
  NULL,
  COALESCE(a.submitted_at, a.created_at),
  a.created_at,
  CASE
    WHEN a.interpreted_at IS NOT NULL OR a.status = 'interpreted' THEN a.interpreted_at
    ELSE NULL
  END,
  a.failed_at,
  CASE
    WHEN a.failed_at IS NOT NULL OR a.status = 'failed' THEN 'failed'
    WHEN a.interpreted_at IS NOT NULL OR a.status = 'interpreted' THEN 'completed'
    ELSE 'active'
  END,
  COALESCE(a.failure_reason, ''),
  NOW(3),
  NOW(3)
FROM %s a
WHERE a.org_id = ?
  AND a.deleted_at IS NULL
  AND a.answer_sheet_id <> 0
`, t.AssessmentEpisode, t.Assessment)
}

func buildJourneyHistoryEpisodeAttributionFromCreatorSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
UPDATE %s e
JOIN (
  SELECT ranked.answersheet_id, ranked.entry_id, ranked.clinician_id, ranked.bound_at
  FROM (
    SELECT
      a.answer_sheet_id AS answersheet_id,
      cr.source_id AS entry_id,
      cr.clinician_id,
      cr.bound_at,
      ROW_NUMBER() OVER (
        PARTITION BY a.answer_sheet_id
        ORDER BY cr.bound_at DESC, cr.id DESC
      ) AS rn
    FROM %s a
    JOIN %s cr
      ON cr.org_id = a.org_id
     AND cr.testee_id = a.testee_id
     AND cr.deleted_at IS NULL
     AND cr.source_type = 'assessment_entry'
     AND cr.relation_type = 'creator'
     AND cr.bound_at <= COALESCE(a.submitted_at, a.created_at)
     AND cr.bound_at >= DATE_SUB(COALESCE(a.submitted_at, a.created_at), INTERVAL 30 DAY)
    WHERE a.org_id = ?
      AND a.deleted_at IS NULL
      AND a.answer_sheet_id <> 0
  ) ranked
  WHERE ranked.rn = 1
) matched
  ON matched.answersheet_id = e.answersheet_id
SET
  e.entry_id = matched.entry_id,
  e.clinician_id = matched.clinician_id,
  e.attributed_intake_at = matched.bound_at,
  e.updated_at = NOW(3)
WHERE e.org_id = ?
  AND e.deleted_at IS NULL
`, t.AssessmentEpisode, t.Assessment, t.ClinicianRelation)
}

func buildJourneyHistoryEpisodeAttributionFromIntakeSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
UPDATE %s e
JOIN (
  SELECT ranked.answersheet_id, ranked.entry_id, ranked.clinician_id, ranked.intake_at
  FROM (
    SELECT
      a.answer_sheet_id AS answersheet_id,
      l.entry_id,
      l.clinician_id,
      l.intake_at,
      ROW_NUMBER() OVER (
        PARTITION BY a.answer_sheet_id
        ORDER BY l.intake_at DESC, l.id DESC
      ) AS rn
    FROM %s a
    JOIN %s l
      ON l.org_id = a.org_id
     AND l.testee_id = a.testee_id
     AND l.deleted_at IS NULL
     AND l.intake_at <= COALESCE(a.submitted_at, a.created_at)
     AND l.intake_at >= DATE_SUB(COALESCE(a.submitted_at, a.created_at), INTERVAL 30 DAY)
    WHERE a.org_id = ?
      AND a.deleted_at IS NULL
      AND a.answer_sheet_id <> 0
  ) ranked
  WHERE ranked.rn = 1
) matched
  ON matched.answersheet_id = e.answersheet_id
SET
  e.entry_id = COALESCE(e.entry_id, matched.entry_id),
  e.clinician_id = COALESCE(e.clinician_id, matched.clinician_id),
  e.attributed_intake_at = COALESCE(e.attributed_intake_at, matched.intake_at),
  e.updated_at = NOW(3)
WHERE e.org_id = ?
  AND e.deleted_at IS NULL
`, t.AssessmentEpisode, t.Assessment, t.AssessmentEntryIntakeLog)
}

func buildJourneyHistoryEpisodeAttributionFromRelationSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
UPDATE %s e
JOIN (
  SELECT ranked.answersheet_id, ranked.entry_id, ranked.clinician_id, ranked.bound_at
  FROM (
    SELECT
      a.answer_sheet_id AS answersheet_id,
      CASE WHEN cr.source_type = 'assessment_entry' THEN COALESCE(cr.source_id, 0) ELSE 0 END AS entry_id,
      cr.clinician_id,
      cr.bound_at,
      ROW_NUMBER() OVER (
        PARTITION BY a.answer_sheet_id
        ORDER BY cr.bound_at DESC, cr.id DESC
      ) AS rn
    FROM %s a
    JOIN %s cr
      ON cr.org_id = a.org_id
     AND cr.testee_id = a.testee_id
     AND cr.deleted_at IS NULL
     AND cr.relation_type IN (%s)
     AND cr.bound_at <= COALESCE(a.submitted_at, a.created_at)
     AND (cr.unbound_at IS NULL OR cr.unbound_at >= COALESCE(a.submitted_at, a.created_at))
    WHERE a.org_id = ?
      AND a.deleted_at IS NULL
      AND a.answer_sheet_id <> 0
  ) ranked
  WHERE ranked.rn = 1
) matched
  ON matched.answersheet_id = e.answersheet_id
SET
  e.entry_id = CASE
    WHEN e.entry_id IS NULL AND matched.entry_id <> 0 THEN matched.entry_id
    ELSE e.entry_id
  END,
  e.clinician_id = COALESCE(e.clinician_id, matched.clinician_id),
  e.attributed_intake_at = COALESCE(e.attributed_intake_at, matched.bound_at),
  e.updated_at = NOW(3)
WHERE e.org_id = ?
  AND e.deleted_at IS NULL
`, t.AssessmentEpisode, t.Assessment, t.ClinicianRelation, journeyHistoryRebuildAccessGrantRelationTypesSQL())
}

func buildJourneyHistoryAnswerSheetSubmittedSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:answersheet_submitted:episode:', e.answersheet_id),
  e.org_id,
  'answersheet',
  e.answersheet_id,
  'testee',
  e.testee_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  e.testee_id,
  e.answersheet_id,
  COALESCE(e.assessment_id, 0),
  COALESCE(e.report_id, 0),
  'answersheet_submitted',
  e.submitted_at,
  JSON_OBJECT('history_source', 'assessment_episode', 'history_id', e.episode_id),
  NOW(3),
  NOW(3)
FROM %s e
WHERE e.org_id = ?
  AND e.deleted_at IS NULL
`, t.BehaviorFootprint, t.AssessmentEpisode)
}

func buildJourneyHistoryAssessmentCreatedSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:assessment_created:episode:', e.assessment_id),
  e.org_id,
  'assessment',
  e.assessment_id,
  'testee',
  e.testee_id,
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  e.testee_id,
  e.answersheet_id,
  COALESCE(e.assessment_id, 0),
  COALESCE(e.report_id, 0),
  'assessment_created',
  e.assessment_created_at,
  JSON_OBJECT('history_source', 'assessment_episode', 'history_id', e.episode_id),
  NOW(3),
  NOW(3)
FROM %s e
WHERE e.org_id = ?
  AND e.deleted_at IS NULL
  AND e.assessment_id IS NOT NULL
  AND e.assessment_created_at IS NOT NULL
`, t.BehaviorFootprint, t.AssessmentEpisode)
}

func buildJourneyHistoryReportGeneratedSQL(t journeyHistoryRebuildTableNames) string {
	return fmt.Sprintf(`
INSERT INTO %s (
  id, org_id, subject_type, subject_id, actor_type, actor_id,
  entry_id, clinician_id, source_clinician_id, testee_id,
  answersheet_id, assessment_id, report_id, event_name, occurred_at,
  properties_json, created_at, updated_at
)
SELECT
  CONCAT('history:report_generated:episode:', e.report_id),
  e.org_id,
  'assessment',
  COALESCE(e.assessment_id, 0),
  'assessment',
  COALESCE(e.assessment_id, 0),
  COALESCE(e.entry_id, 0),
  COALESCE(e.clinician_id, 0),
  0,
  e.testee_id,
  e.answersheet_id,
  COALESCE(e.assessment_id, 0),
  COALESCE(e.report_id, 0),
  'report_generated',
  e.report_generated_at,
  JSON_OBJECT('history_source', 'assessment_episode', 'history_id', e.episode_id),
  NOW(3),
  NOW(3)
FROM %s e
WHERE e.org_id = ?
  AND e.deleted_at IS NULL
  AND e.report_generated_at IS NOT NULL
  AND e.report_id IS NOT NULL
`, t.BehaviorFootprint, t.AssessmentEpisode)
}
