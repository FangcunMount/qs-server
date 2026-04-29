package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

func (r *StatisticsRepository) RebuildDailyStatistics(ctx context.Context, orgID int64, startDate, endDate time.Time) error {
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	if err := tx.Exec(
		`DELETE FROM statistics_daily
		  WHERE org_id = ? AND statistic_type IN ('questionnaire', 'system')
		    AND stat_date >= ? AND stat_date < ?`,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}

	if err := tx.Exec(
		`INSERT INTO statistics_daily (
			org_id, statistic_type, statistic_key, stat_date, submission_count, completion_count
		)
		SELECT ?, 'questionnaire', agg.statistic_key, agg.stat_date, agg.submission_count, agg.completion_count
		FROM (
			SELECT raw.statistic_key, raw.stat_date,
			       SUM(raw.submission_count) AS submission_count,
			       SUM(raw.completion_count) AS completion_count
			FROM (
				SELECT questionnaire_code AS statistic_key,
				       DATE(created_at) AS stat_date,
				       1 AS submission_count,
				       0 AS completion_count
				FROM assessment
				WHERE org_id = ? AND deleted_at IS NULL
				  AND questionnaire_code <> ''
				  AND created_at >= ? AND created_at < ?
				UNION ALL
				SELECT questionnaire_code AS statistic_key,
				       DATE(interpreted_at) AS stat_date,
				       0 AS submission_count,
				       1 AS completion_count
				FROM assessment
				WHERE org_id = ? AND deleted_at IS NULL
				  AND questionnaire_code <> ''
				  AND interpreted_at IS NOT NULL
				  AND interpreted_at >= ? AND interpreted_at < ?
			) raw
			GROUP BY raw.statistic_key, raw.stat_date
		) agg`,
		orgID,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}

	if err := tx.Exec(
		`INSERT INTO statistics_daily (
			org_id, statistic_type, statistic_key, stat_date, submission_count, completion_count
		)
		SELECT ?, 'system', 'system', agg.stat_date, agg.submission_count, agg.completion_count
		FROM (
			SELECT raw.stat_date,
			       SUM(raw.submission_count) AS submission_count,
			       SUM(raw.completion_count) AS completion_count
			FROM (
				SELECT DATE(created_at) AS stat_date,
				       1 AS submission_count,
				       0 AS completion_count
				FROM assessment
				WHERE org_id = ? AND deleted_at IS NULL
				  AND created_at >= ? AND created_at < ?
				UNION ALL
				SELECT DATE(interpreted_at) AS stat_date,
				       0 AS submission_count,
				       1 AS completion_count
				FROM assessment
				WHERE org_id = ? AND deleted_at IS NULL
				  AND interpreted_at IS NOT NULL
				  AND interpreted_at >= ? AND interpreted_at < ?
			) raw
			GROUP BY raw.stat_date
		) agg`,
		orgID,
		orgID, startDate, endDate,
		orgID, startDate, endDate,
	).Error; err != nil {
		return err
	}

	if err := r.rebuildAccessDailyProjections(ctx, tx, orgID, startDate, endDate); err != nil {
		return err
	}
	return r.rebuildAssessmentServiceDailyProjections(ctx, tx, orgID, startDate, endDate)
}

func (r *StatisticsRepository) RebuildAccumulatedStatistics(ctx context.Context, orgID int64, todayStart time.Time) error {
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	if err := tx.Exec(
		`DELETE FROM statistics_accumulated
		  WHERE org_id = ? AND statistic_type IN ('questionnaire', 'system')`,
		orgID,
	).Error; err != nil {
		return err
	}

	questionnaireRows, err := r.buildQuestionnaireAccumulatedRows(ctx, tx, orgID, todayStart)
	if err != nil {
		return err
	}
	if len(questionnaireRows) > 0 {
		if err := tx.CreateInBatches(questionnaireRows, 200).Error; err != nil {
			return err
		}
	}

	systemRow, err := r.buildSystemAccumulatedRow(ctx, tx, orgID, todayStart)
	if err != nil {
		return err
	}
	if err := tx.Create(systemRow).Error; err != nil {
		return err
	}
	return r.rebuildOrganizationSnapshot(ctx, tx, orgID, time.Now().In(time.Local))
}

func (r *StatisticsRepository) RebuildPlanStatistics(ctx context.Context, orgID int64) error {
	tx, err := mysql.RequireTx(ctx)
	if err != nil {
		return err
	}
	if err := tx.Exec("DELETE FROM statistics_plan WHERE org_id = ?", orgID).Error; err != nil {
		return err
	}

	var planRows []*StatisticsPlanPO
	if err := tx.Raw(
		`SELECT
			p.org_id AS org_id,
			p.id AS plan_id,
			COUNT(t.id) AS total_tasks,
			COALESCE(SUM(CASE WHEN t.status = 'completed' THEN 1 ELSE 0 END), 0) AS completed_tasks,
			COALESCE(SUM(CASE WHEN t.status IN ('pending', 'opened') THEN 1 ELSE 0 END), 0) AS pending_tasks,
			COALESCE(SUM(CASE WHEN t.status = 'expired' THEN 1 ELSE 0 END), 0) AS expired_tasks,
			COUNT(DISTINCT t.testee_id) AS enrolled_testees,
			COUNT(DISTINCT CASE WHEN t.status = 'completed' THEN t.testee_id END) AS active_testees
		FROM assessment_plan p
		LEFT JOIN assessment_task t
		  ON t.org_id = p.org_id
		 AND t.plan_id = p.id
		 AND t.deleted_at IS NULL
		WHERE p.org_id = ? AND p.deleted_at IS NULL
		GROUP BY p.org_id, p.id`,
		orgID,
	).Scan(&planRows).Error; err != nil {
		return err
	}
	if len(planRows) > 0 {
		if err := tx.CreateInBatches(planRows, 200).Error; err != nil {
			return err
		}
	}
	if err := r.rebuildPlanTaskDailyProjection(ctx, tx, orgID); err != nil {
		return err
	}
	return r.rebuildPlanTaskWindowSnapshots(ctx, tx, orgID, time.Now().In(time.Local))
}

func (r *StatisticsRepository) buildQuestionnaireAccumulatedRows(ctx context.Context, tx *gorm.DB, orgID int64, todayStart time.Time) ([]*StatisticsAccumulatedPO, error) {
	last7d := todayStart.AddDate(0, 0, -7)
	last15d := todayStart.AddDate(0, 0, -15)
	last30d := todayStart.AddDate(0, 0, -30)

	var aggregates []struct {
		StatisticKey       string
		TotalSubmissions   int64
		TotalCompletions   int64
		Last7dSubmissions  int64
		Last15dSubmissions int64
		Last30dSubmissions int64
	}
	if err := tx.WithContext(ctx).Raw(
		`SELECT
			statistic_key,
			COALESCE(SUM(submission_count), 0) AS total_submissions,
			COALESCE(SUM(completion_count), 0) AS total_completions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last7d_submissions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last15d_submissions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last30d_submissions
		FROM statistics_daily
		WHERE org_id = ? AND statistic_type = 'questionnaire'
		GROUP BY statistic_key`,
		last7d, last15d, last30d, orgID,
	).Scan(&aggregates).Error; err != nil {
		return nil, err
	}

	originDistribution := make(map[string]JSONField)
	var originRows []struct {
		QuestionnaireCode string
		OriginType        string
		Count             int64
	}
	if err := tx.WithContext(ctx).Raw(
		`SELECT questionnaire_code, origin_type, COUNT(*) AS count
		FROM assessment
		WHERE org_id = ? AND deleted_at IS NULL
		  AND questionnaire_code <> ''
		  AND created_at < ?
		GROUP BY questionnaire_code, origin_type`,
		orgID, todayStart,
	).Scan(&originRows).Error; err != nil {
		return nil, err
	}
	for _, row := range originRows {
		if originDistribution[row.QuestionnaireCode] == nil {
			originDistribution[row.QuestionnaireCode] = JSONField{}
		}
		originDistribution[row.QuestionnaireCode][row.OriginType] = row.Count
	}

	timeBounds := make(map[string]struct {
		FirstOccurredAt *time.Time
		LastOccurredAt  *time.Time
	})
	var timeRows []struct {
		QuestionnaireCode string
		FirstOccurredAt   *time.Time
		LastOccurredAt    *time.Time
	}
	if err := tx.WithContext(ctx).Raw(
		`SELECT questionnaire_code, MIN(created_at) AS first_occurred_at, MAX(created_at) AS last_occurred_at
		FROM assessment
		WHERE org_id = ? AND deleted_at IS NULL
		  AND questionnaire_code <> ''
		  AND created_at < ?
		GROUP BY questionnaire_code`,
		orgID, todayStart,
	).Scan(&timeRows).Error; err != nil {
		return nil, err
	}
	for _, row := range timeRows {
		timeBounds[row.QuestionnaireCode] = struct {
			FirstOccurredAt *time.Time
			LastOccurredAt  *time.Time
		}{FirstOccurredAt: row.FirstOccurredAt, LastOccurredAt: row.LastOccurredAt}
	}

	result := make([]*StatisticsAccumulatedPO, 0, len(aggregates))
	for _, aggregate := range aggregates {
		distribution := JSONField{}
		if origin := originDistribution[aggregate.StatisticKey]; len(origin) > 0 {
			distribution["origin"] = origin
		}
		bounds := timeBounds[aggregate.StatisticKey]
		result = append(result, &StatisticsAccumulatedPO{
			OrgID:              orgID,
			StatisticType:      "questionnaire",
			StatisticKey:       aggregate.StatisticKey,
			TotalSubmissions:   aggregate.TotalSubmissions,
			TotalCompletions:   aggregate.TotalCompletions,
			Last7dSubmissions:  aggregate.Last7dSubmissions,
			Last15dSubmissions: aggregate.Last15dSubmissions,
			Last30dSubmissions: aggregate.Last30dSubmissions,
			Distribution:       distribution,
			FirstOccurredAt:    bounds.FirstOccurredAt,
			LastOccurredAt:     bounds.LastOccurredAt,
		})
	}
	return result, nil
}

func (r *StatisticsRepository) buildSystemAccumulatedRow(ctx context.Context, tx *gorm.DB, orgID int64, todayStart time.Time) (*StatisticsAccumulatedPO, error) {
	var assessmentCount int64
	if err := tx.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Count(&assessmentCount).Error; err != nil {
		return nil, err
	}
	var completionCount int64
	if err := tx.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL AND interpreted_at IS NOT NULL AND interpreted_at < ?", orgID, todayStart).
		Count(&completionCount).Error; err != nil {
		return nil, err
	}
	var testeeCount int64
	if err := tx.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Count(&testeeCount).Error; err != nil {
		return nil, err
	}

	statusDistribution := JSONField{}
	var statusRows []struct {
		Status string
		Count  int64
	}
	if err := tx.WithContext(ctx).
		Table("assessment").
		Select("status, COUNT(*) AS count").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Group("status").
		Scan(&statusRows).Error; err != nil {
		return nil, err
	}
	for _, row := range statusRows {
		statusDistribution[row.Status] = row.Count
	}

	var timeInfo struct {
		FirstOccurredAt *time.Time
		LastOccurredAt  *time.Time
	}
	if err := tx.WithContext(ctx).
		Table("assessment").
		Select("MIN(created_at) AS first_occurred_at, MAX(created_at) AS last_occurred_at").
		Where("org_id = ? AND deleted_at IS NULL AND created_at < ?", orgID, todayStart).
		Scan(&timeInfo).Error; err != nil {
		return nil, err
	}

	return &StatisticsAccumulatedPO{
		OrgID:            orgID,
		StatisticType:    "system",
		StatisticKey:     "system",
		TotalSubmissions: assessmentCount,
		TotalCompletions: completionCount,
		Distribution: JSONField{
			"status":       statusDistribution,
			"testee_count": testeeCount,
		},
		FirstOccurredAt: timeInfo.FirstOccurredAt,
		LastOccurredAt:  timeInfo.LastOccurredAt,
	}, nil
}
