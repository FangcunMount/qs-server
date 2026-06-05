package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"gorm.io/gorm"
)

const (
	planTaskCompletedStatus = "completed"
	planTaskExpiredStatus   = "expired"
	planTaskCanceledStatus  = "canceled"
)

func (r *StatisticsRepository) LoadSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool, error) {
	var snapshot StatisticsOrgSnapshotPO
	if err := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		First(&snapshot).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, false, nil
		}
		return nil, false, err
	}

	stats := &domainStatistics.SystemStatistics{
		OrgID:                        orgID,
		TesteeCount:                  snapshot.TesteeCount,
		AssessmentCount:              snapshot.AssessmentCount,
		AssessmentStatusDistribution: map[string]int64{},
		AssessmentTrend:              r.systemDailyTrend(ctx, orgID),
	}
	if err := r.fillAssessmentStatusDistribution(ctx, orgID, stats.AssessmentStatusDistribution); err != nil {
		return nil, false, err
	}
	if err := r.fillRealtimeTodayFields(ctx, orgID, stats); err != nil {
		return nil, false, err
	}
	return stats, true, nil
}

func (r *StatisticsRepository) LoadQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, bool, error) {
	totals, found, err := r.questionnaireContentTotals(ctx, orgID, questionnaireCode)
	if err != nil || !found {
		return nil, found, err
	}

	stats := r.contentTotalsToQuestionnaireStatistics(totals, orgID, questionnaireCode)
	stats.DailyTrend = r.questionnaireDailyTrend(ctx, orgID, questionnaireCode)
	originDistribution, err := r.questionnaireOriginDistributionFromContent(ctx, orgID, questionnaireCode)
	if err != nil {
		return nil, false, err
	}
	stats.OriginDistribution = originDistribution
	return stats, true, nil
}

func (r *StatisticsRepository) LoadPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, bool, error) {
	exists, err := r.planExists(ctx, orgID, planID)
	if err != nil || !exists {
		return nil, exists, err
	}
	stats, err := r.planStatisticsFromTasks(ctx, orgID, planID)
	if err != nil {
		return nil, false, err
	}
	activityWindow := r.planTaskWindow(ctx, orgID, planID)
	activityTrend := domainStatistics.PlanTaskTrend{
		TaskCreated:   r.planTaskTrend(ctx, orgID, planID, "task_created_count"),
		TaskOpened:    r.planTaskTrend(ctx, orgID, planID, "task_opened_count"),
		TaskCompleted: r.planTaskTrend(ctx, orgID, planID, "task_completed_count"),
		TaskExpired:   r.planTaskTrend(ctx, orgID, planID, "task_expired_count"),
	}
	stats.Activity = domainStatistics.PlanTaskActivityStatistics{
		Window: activityWindow,
		Trend:  activityTrend,
	}
	stats.Fulfillment = domainStatistics.PlanTaskFulfillmentStatistics{
		Window: r.planTaskFulfillmentWindow(ctx, orgID, planID),
		Trend:  r.planTaskFulfillmentTrend(ctx, orgID, planID),
	}
	stats.Window = activityWindow
	stats.Trend = activityTrend
	return stats, true, nil
}

type questionnaireContentTotals struct {
	TotalSubmissions   int64
	TotalCompletions   int64
	Last7dSubmissions  int64
	Last15dSubmissions int64
	Last30dSubmissions int64
}

func (r *StatisticsRepository) questionnaireContentTotals(ctx context.Context, orgID int64, questionnaireCode string) (questionnaireContentTotals, bool, error) {
	now := time.Now().In(time.Local)
	todayStart, _ := currentDayBounds(now)
	last7d := todayStart.AddDate(0, 0, -7)
	last15d := todayStart.AddDate(0, 0, -15)
	last30d := todayStart.AddDate(0, 0, -30)

	var row questionnaireContentTotals
	var count int64
	if err := r.WithContext(ctx).
		Model(&StatisticsContentDailyPO{}).
		Where("org_id = ? AND content_type = ? AND content_code = ? AND deleted_at IS NULL", orgID, StatisticsContentTypeQuestionnaire, questionnaireCode).
		Count(&count).Error; err != nil {
		return row, false, err
	}
	if count == 0 {
		return row, false, nil
	}
	if err := r.WithContext(ctx).
		Model(&StatisticsContentDailyPO{}).
		Select(`
			COALESCE(SUM(submission_count), 0) AS total_submissions,
			COALESCE(SUM(completion_count), 0) AS total_completions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last7d_submissions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last15d_submissions,
			COALESCE(SUM(CASE WHEN stat_date >= ? THEN submission_count ELSE 0 END), 0) AS last30d_submissions
		`, last7d, last15d, last30d).
		Where("org_id = ? AND content_type = ? AND content_code = ? AND deleted_at IS NULL", orgID, StatisticsContentTypeQuestionnaire, questionnaireCode).
		Scan(&row).Error; err != nil {
		return row, false, err
	}
	return row, true, nil
}

func (r *StatisticsRepository) contentTotalsToQuestionnaireStatistics(totals questionnaireContentTotals, orgID int64, questionnaireCode string) *domainStatistics.QuestionnaireStatistics {
	stats := &domainStatistics.QuestionnaireStatistics{
		OrgID:              orgID,
		QuestionnaireCode:  questionnaireCode,
		TotalSubmissions:   totals.TotalSubmissions,
		TotalCompletions:   totals.TotalCompletions,
		Last7DaysCount:     totals.Last7dSubmissions,
		Last15DaysCount:    totals.Last15dSubmissions,
		Last30DaysCount:    totals.Last30dSubmissions,
		OriginDistribution: map[string]int64{},
		DailyTrend:         []domainStatistics.DailyCount{},
	}
	stats.CompletionRate = domainStatistics.NewAggregator().CalculateCompletionRate(totals.TotalSubmissions, totals.TotalCompletions)
	return stats
}

func (r *StatisticsRepository) questionnaireOriginDistributionFromContent(ctx context.Context, orgID int64, questionnaireCode string) (map[string]int64, error) {
	var rows []struct {
		OriginType string
		Count      int64
	}
	if err := r.WithContext(ctx).
		Model(&StatisticsContentDailyPO{}).
		Select("origin_type, COALESCE(SUM(submission_count), 0) AS count").
		Where("org_id = ? AND content_type = ? AND content_code = ? AND deleted_at IS NULL", orgID, StatisticsContentTypeQuestionnaire, questionnaireCode).
		Group("origin_type").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	distribution := make(map[string]int64, len(rows))
	for _, row := range rows {
		distribution[row.OriginType] = row.Count
	}
	return distribution, nil
}

func (r *StatisticsRepository) questionnaireDailyTrend(ctx context.Context, orgID int64, questionnaireCode string) []domainStatistics.DailyCount {
	endDate := time.Now().In(time.Local)
	startDate := endDate.AddDate(0, 0, -30)
	endExclusive := dateOnly(endDate).AddDate(0, 0, 1)
	var rows []dailyCountRow
	if err := r.WithContext(ctx).
		Model(&StatisticsContentDailyPO{}).
		Select("stat_date, COALESCE(SUM(submission_count), 0) AS count").
		Where("org_id = ? AND content_type = ? AND content_code = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID, StatisticsContentTypeQuestionnaire, questionnaireCode, dateOnly(startDate), endExclusive).
		Group("stat_date").
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return []domainStatistics.DailyCount{}
	}
	return buildDailyCounts(rows)
}

func (r *StatisticsRepository) systemDailyTrend(ctx context.Context, orgID int64) []domainStatistics.DailyCount {
	endDate := time.Now().In(time.Local)
	startDate := endDate.AddDate(0, 0, -30)
	endExclusive := dateOnly(endDate).AddDate(0, 0, 1)
	var rows []dailyCountRow
	if err := r.WithContext(ctx).
		Model(&StatisticsJourneyDailyPO{}).
		Select("stat_date, COALESCE(SUM(service_assessment_created_count), 0) AS count").
		Where("org_id = ? AND subject_type = ? AND subject_id = 0 AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL",
			orgID, StatisticsJourneySubjectOrg, dateOnly(startDate), endExclusive).
		Group("stat_date").
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return []domainStatistics.DailyCount{}
	}
	return buildDailyCounts(rows)
}

type dailyCountRow struct {
	StatDate time.Time
	Count    int64
}

func buildDailyCounts(rows []dailyCountRow) []domainStatistics.DailyCount {
	result := make([]domainStatistics.DailyCount, 0, len(rows))
	for _, row := range rows {
		result = append(result, domainStatistics.DailyCount{Date: row.StatDate, Count: row.Count})
	}
	return result
}

func (r *StatisticsRepository) fillAssessmentStatusDistribution(ctx context.Context, orgID int64, dist map[string]int64) error {
	var rows []struct {
		Status string
		Count  int64
	}
	if err := r.WithContext(ctx).
		Table("assessment").
		Select("status, COUNT(*) AS count").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Group("status").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		dist[row.Status] = row.Count
	}
	return nil
}

func (r *StatisticsRepository) planExists(ctx context.Context, orgID int64, planID uint64) (bool, error) {
	var count int64
	if err := r.WithContext(ctx).
		Table("assessment_plan").
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, planID).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

type planTaskTotals struct {
	TotalTasks      int64
	CompletedTasks  int64
	PendingTasks    int64
	ExpiredTasks    int64
	EnrolledTestees int64
	ActiveTestees   int64
}

func (r *StatisticsRepository) planStatisticsFromTasks(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, error) {
	var row planTaskTotals
	if err := r.WithContext(ctx).
		Table("assessment_task").
		Select(`
			COUNT(*) AS total_tasks,
			COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0) AS completed_tasks,
			COALESCE(SUM(CASE WHEN status IN ('pending', 'opened') THEN 1 ELSE 0 END), 0) AS pending_tasks,
			COALESCE(SUM(CASE WHEN status = 'expired' THEN 1 ELSE 0 END), 0) AS expired_tasks,
			COUNT(DISTINCT testee_id) AS enrolled_testees,
			COUNT(DISTINCT CASE WHEN status = 'completed' THEN testee_id END) AS active_testees
		`).
		Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", orgID, planID).
		Scan(&row).Error; err != nil {
		return nil, err
	}
	return planStatisticsFromTaskTotals(orgID, planID, row), nil
}

func planStatisticsFromTaskTotals(orgID int64, planID uint64, row planTaskTotals) *domainStatistics.PlanStatistics {
	stats := &domainStatistics.PlanStatistics{
		OrgID:           orgID,
		PlanID:          planID,
		TotalTasks:      row.TotalTasks,
		CompletedTasks:  row.CompletedTasks,
		PendingTasks:    row.PendingTasks,
		ExpiredTasks:    row.ExpiredTasks,
		EnrolledTestees: row.EnrolledTestees,
		ActiveTestees:   row.ActiveTestees,
	}
	stats.CompletionRate = domainStatistics.NewAggregator().CalculateCompletionRate(row.TotalTasks, row.CompletedTasks)
	return stats
}

func (r *StatisticsRepository) planTaskWindow(ctx context.Context, orgID int64, planID uint64) domainStatistics.PlanTaskWindow {
	from, to := defaultPlanTaskTrendRange()
	var row struct {
		TaskCreatedCount   int64
		TaskOpenedCount    int64
		TaskCompletedCount int64
		TaskExpiredCount   int64
	}
	if err := r.WithContext(ctx).
		Model(&StatisticsPlanDailyPO{}).
		Select(`
			COALESCE(SUM(task_created_count), 0) AS task_created_count,
			COALESCE(SUM(task_opened_count), 0) AS task_opened_count,
			COALESCE(SUM(task_completed_count), 0) AS task_completed_count,
			COALESCE(SUM(task_expired_count), 0) AS task_expired_count
		`).
		Where("org_id = ? AND plan_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, planID, from, to).
		Scan(&row).Error; err != nil {
		return domainStatistics.PlanTaskWindow{}
	}
	return domainStatistics.PlanTaskWindow{
		TaskCreatedCount:   row.TaskCreatedCount,
		TaskOpenedCount:    row.TaskOpenedCount,
		TaskCompletedCount: row.TaskCompletedCount,
		TaskExpiredCount:   row.TaskExpiredCount,
		EnrolledTestees:    r.countPlanTaskDistinctTestees(ctx, orgID, planID, "created_at", "", from, to),
		ActiveTestees:      r.countPlanTaskDistinctTestees(ctx, orgID, planID, "completed_at", "completed", from, to),
	}
}

func (r *StatisticsRepository) countPlanTaskDistinctTestees(ctx context.Context, orgID int64, planID uint64, timeField, status string, from, to time.Time) int64 {
	var row struct {
		Count int64
	}
	query := r.WithContext(ctx).
		Table("assessment_task t").
		Joins("JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL").
		Select("COUNT(DISTINCT t.testee_id) AS count").
		Where("t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL", orgID, planID).
		Where("t."+timeField+" >= ? AND t."+timeField+" < ?", from, to)
	if status != "" {
		query = query.Where("t.status = ?", status)
	}
	if err := query.Scan(&row).Error; err != nil {
		return 0
	}
	return row.Count
}

func (r *StatisticsRepository) planTaskTrend(ctx context.Context, orgID int64, planID uint64, field string) []domainStatistics.DailyCount {
	from, to := defaultPlanTaskTrendRange()
	var rows []dailyCountRow
	if err := r.WithContext(ctx).
		Model(&StatisticsPlanDailyPO{}).
		Select("stat_date, COALESCE("+field+", 0) AS count").
		Where("org_id = ? AND plan_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, planID, from, to).
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return []domainStatistics.DailyCount{}
	}
	return buildDailyCounts(rows)
}

func (r *StatisticsRepository) planTaskActivityWindowFromTasks(ctx context.Context, orgID int64, planID uint64) domainStatistics.PlanTaskWindow {
	from, to := defaultPlanTaskTrendRange()
	var row struct {
		TaskCreatedCount   int64
		TaskOpenedCount    int64
		TaskCompletedCount int64
		TaskExpiredCount   int64
		EnrolledTestees    int64
		ActiveTestees      int64
	}
	if err := r.WithContext(ctx).
		Table("assessment_task t").
		Joins("JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL").
		Select(`
			COALESCE(SUM(CASE WHEN t.created_at >= ? AND t.created_at < ? THEN 1 ELSE 0 END), 0) AS task_created_count,
			COALESCE(SUM(CASE WHEN t.open_at IS NOT NULL AND t.open_at >= ? AND t.open_at < ? THEN 1 ELSE 0 END), 0) AS task_opened_count,
			COALESCE(SUM(CASE WHEN t.completed_at IS NOT NULL AND t.completed_at >= ? AND t.completed_at < ? AND t.status = ? THEN 1 ELSE 0 END), 0) AS task_completed_count,
			COALESCE(SUM(CASE WHEN t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ? AND t.status = ? THEN 1 ELSE 0 END), 0) AS task_expired_count,
			COUNT(DISTINCT CASE WHEN t.created_at >= ? AND t.created_at < ? THEN t.testee_id END) AS enrolled_testees,
			COUNT(DISTINCT CASE WHEN t.completed_at IS NOT NULL AND t.completed_at >= ? AND t.completed_at < ? AND t.status = ? THEN t.testee_id END) AS active_testees
		`,
			from, to,
			from, to,
			from, to, planTaskCompletedStatus,
			from, to, planTaskExpiredStatus,
			from, to,
			from, to, planTaskCompletedStatus,
		).
		Where("t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL", orgID, planID).
		Scan(&row).Error; err != nil {
		return domainStatistics.PlanTaskWindow{}
	}
	return domainStatistics.PlanTaskWindow{
		TaskCreatedCount:   row.TaskCreatedCount,
		TaskOpenedCount:    row.TaskOpenedCount,
		TaskCompletedCount: row.TaskCompletedCount,
		TaskExpiredCount:   row.TaskExpiredCount,
		EnrolledTestees:    row.EnrolledTestees,
		ActiveTestees:      row.ActiveTestees,
	}
}

func (r *StatisticsRepository) planTaskActivityTrendFromTasks(ctx context.Context, orgID int64, planID uint64) domainStatistics.PlanTaskTrend {
	from, to := defaultPlanTaskTrendRange()
	type row struct {
		StatDate           time.Time
		TaskCreatedCount   int64
		TaskOpenedCount    int64
		TaskCompletedCount int64
		TaskExpiredCount   int64
	}
	var rows []row
	if err := r.WithContext(ctx).Raw(`
		SELECT
			raw.stat_date,
			COALESCE(SUM(raw.task_created_count), 0) AS task_created_count,
			COALESCE(SUM(raw.task_opened_count), 0) AS task_opened_count,
			COALESCE(SUM(raw.task_completed_count), 0) AS task_completed_count,
			COALESCE(SUM(raw.task_expired_count), 0) AS task_expired_count
		FROM (
			SELECT DATE(t.created_at) AS stat_date, COUNT(*) AS task_created_count, 0 AS task_opened_count, 0 AS task_completed_count, 0 AS task_expired_count
			FROM assessment_task t
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL AND t.created_at >= ? AND t.created_at < ?
			GROUP BY DATE(t.created_at)
			UNION ALL
			SELECT DATE(t.open_at) AS stat_date, 0, COUNT(*), 0, 0
			FROM assessment_task t
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL AND t.open_at IS NOT NULL AND t.open_at >= ? AND t.open_at < ?
			GROUP BY DATE(t.open_at)
			UNION ALL
			SELECT DATE(t.completed_at) AS stat_date, 0, 0, COUNT(*), 0
			FROM assessment_task t
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL AND t.status = ? AND t.completed_at IS NOT NULL AND t.completed_at >= ? AND t.completed_at < ?
			GROUP BY DATE(t.completed_at)
			UNION ALL
			SELECT DATE(t.expire_at) AS stat_date, 0, 0, 0, COUNT(*)
			FROM assessment_task t
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL AND t.status = ? AND t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ?
			GROUP BY DATE(t.expire_at)
		) raw
		GROUP BY raw.stat_date
		ORDER BY raw.stat_date ASC`,
		orgID, planID, from, to,
		orgID, planID, from, to,
		orgID, planID, planTaskCompletedStatus, from, to,
		orgID, planID, planTaskExpiredStatus, from, to,
	).Scan(&rows).Error; err != nil {
		return domainStatistics.PlanTaskTrend{}
	}
	trend := domainStatistics.PlanTaskTrend{}
	for _, item := range rows {
		trend.TaskCreated = append(trend.TaskCreated, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskCreatedCount})
		trend.TaskOpened = append(trend.TaskOpened, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskOpenedCount})
		trend.TaskCompleted = append(trend.TaskCompleted, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskCompletedCount})
		trend.TaskExpired = append(trend.TaskExpired, domainStatistics.DailyCount{Date: item.StatDate, Count: item.TaskExpiredCount})
	}
	return trend
}

func (r *StatisticsRepository) planTaskFulfillmentWindow(ctx context.Context, orgID int64, planID uint64) domainStatistics.PlanTaskFulfillmentWindow {
	from, to := defaultPlanTaskTrendRange()
	var row struct {
		PlannedTaskCount     int64
		DueTaskCount         int64
		CompletedTaskCount   int64
		OnTimeCompletedCount int64
		OverdueTaskCount     int64
	}
	if err := r.WithContext(ctx).
		Table("assessment_task t").
		Joins("JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL").
		Select(`
			COALESCE(SUM(CASE WHEN t.planned_at >= ? AND t.planned_at < ? THEN 1 ELSE 0 END), 0) AS planned_task_count,
			COALESCE(SUM(CASE WHEN t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ? THEN 1 ELSE 0 END), 0) AS due_task_count,
			COALESCE(SUM(CASE WHEN t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ? AND t.status = ? THEN 1 ELSE 0 END), 0) AS completed_task_count,
			COALESCE(SUM(CASE WHEN t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ? AND t.status = ? AND t.completed_at IS NOT NULL AND t.completed_at <= t.expire_at THEN 1 ELSE 0 END), 0) AS on_time_completed_count,
			COALESCE(SUM(CASE WHEN t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ? AND (t.status = ? OR (t.completed_at IS NOT NULL AND t.completed_at > t.expire_at) OR (t.status <> ? AND t.expire_at < ?)) THEN 1 ELSE 0 END), 0) AS overdue_task_count
		`,
			from, to,
			from, to,
			from, to, planTaskCompletedStatus,
			from, to, planTaskCompletedStatus,
			from, to, planTaskExpiredStatus, planTaskCompletedStatus, time.Now(),
		).
		Where("t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL AND t.status <> ?", orgID, planID, planTaskCanceledStatus).
		Scan(&row).Error; err != nil {
		return domainStatistics.PlanTaskFulfillmentWindow{}
	}
	return planTaskFulfillmentWindowFromCounts(
		row.PlannedTaskCount,
		row.DueTaskCount,
		row.CompletedTaskCount,
		row.OnTimeCompletedCount,
		row.OverdueTaskCount,
	)
}

func (r *StatisticsRepository) planTaskFulfillmentTrend(ctx context.Context, orgID int64, planID uint64) domainStatistics.PlanTaskFulfillmentTrend {
	from, to := defaultPlanTaskTrendRange()
	type row struct {
		StatDate           time.Time
		PlannedTaskCount   int64
		DueTaskCount       int64
		CompletedTaskCount int64
		OverdueTaskCount   int64
	}
	var rows []row
	if err := r.WithContext(ctx).Raw(`
		SELECT
			raw.stat_date,
			COALESCE(SUM(raw.planned_task_count), 0) AS planned_task_count,
			COALESCE(SUM(raw.due_task_count), 0) AS due_task_count,
			COALESCE(SUM(raw.completed_task_count), 0) AS completed_task_count,
			COALESCE(SUM(raw.overdue_task_count), 0) AS overdue_task_count
		FROM (
			SELECT DATE(t.planned_at) AS stat_date, COUNT(*) AS planned_task_count, 0 AS due_task_count, 0 AS completed_task_count, 0 AS overdue_task_count
			FROM assessment_task t
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL AND t.status <> ? AND t.planned_at >= ? AND t.planned_at < ?
			GROUP BY DATE(t.planned_at)
			UNION ALL
			SELECT DATE(t.expire_at) AS stat_date, 0 AS planned_task_count, COUNT(*) AS due_task_count,
				SUM(CASE WHEN t.status = ? THEN 1 ELSE 0 END) AS completed_task_count,
				SUM(CASE WHEN t.status = ? OR (t.completed_at IS NOT NULL AND t.completed_at > t.expire_at) OR (t.status <> ? AND t.expire_at < ?) THEN 1 ELSE 0 END) AS overdue_task_count
			FROM assessment_task t
			JOIN assessment_plan p ON p.org_id = t.org_id AND p.id = t.plan_id AND p.deleted_at IS NULL
			WHERE t.org_id = ? AND t.plan_id = ? AND t.deleted_at IS NULL AND t.status <> ? AND t.expire_at IS NOT NULL AND t.expire_at >= ? AND t.expire_at < ?
			GROUP BY DATE(t.expire_at)
		) raw
		GROUP BY raw.stat_date
		ORDER BY raw.stat_date ASC`,
		orgID, planID, planTaskCanceledStatus, from, to,
		planTaskCompletedStatus, planTaskExpiredStatus, planTaskCompletedStatus, time.Now(),
		orgID, planID, planTaskCanceledStatus, from, to,
	).Scan(&rows).Error; err != nil {
		return domainStatistics.PlanTaskFulfillmentTrend{}
	}
	trend := domainStatistics.PlanTaskFulfillmentTrend{}
	for _, item := range rows {
		trend.Planned = append(trend.Planned, domainStatistics.DailyCount{Date: item.StatDate, Count: item.PlannedTaskCount})
		trend.Due = append(trend.Due, domainStatistics.DailyCount{Date: item.StatDate, Count: item.DueTaskCount})
		trend.Completed = append(trend.Completed, domainStatistics.DailyCount{Date: item.StatDate, Count: item.CompletedTaskCount})
		trend.Overdue = append(trend.Overdue, domainStatistics.DailyCount{Date: item.StatDate, Count: item.OverdueTaskCount})
	}
	return trend
}

func planTaskFulfillmentWindowFromCounts(planned, due, completed, onTimeCompleted, overdue int64) domainStatistics.PlanTaskFulfillmentWindow {
	aggregator := domainStatistics.NewAggregator()
	return domainStatistics.PlanTaskFulfillmentWindow{
		PlannedTaskCount:     planned,
		DueTaskCount:         due,
		CompletedTaskCount:   completed,
		OnTimeCompletedCount: onTimeCompleted,
		OverdueTaskCount:     overdue,
		CompletionRate:       aggregator.CalculateCompletionRate(due, completed),
		OnTimeCompletionRate: aggregator.CalculateCompletionRate(due, onTimeCompleted),
	}
}

func defaultPlanTaskTrendRange() (time.Time, time.Time) {
	now := time.Now().In(time.Local)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return todayStart.AddDate(0, 0, -30), todayStart.AddDate(0, 0, 1)
}
