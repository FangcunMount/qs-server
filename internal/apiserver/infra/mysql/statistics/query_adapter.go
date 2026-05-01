package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"gorm.io/gorm"
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
	stats.Window = r.planTaskWindow(ctx, orgID, planID)
	stats.Trend = domainStatistics.PlanTaskTrend{
		TaskCreated:   r.planTaskTrend(ctx, orgID, planID, "task_created_count"),
		TaskOpened:    r.planTaskTrend(ctx, orgID, planID, "task_opened_count"),
		TaskCompleted: r.planTaskTrend(ctx, orgID, planID, "task_completed_count"),
		TaskExpired:   r.planTaskTrend(ctx, orgID, planID, "task_expired_count"),
	}
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

func defaultPlanTaskTrendRange() (time.Time, time.Time) {
	now := time.Now().In(time.Local)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return todayStart.AddDate(0, 0, -30), todayStart.AddDate(0, 0, 1)
}
