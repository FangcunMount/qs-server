package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func (r *StatisticsRepository) BuildRealtimeSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, error) {
	result := &domainStatistics.SystemStatistics{
		OrgID:                        orgID,
		AssessmentStatusDistribution: make(map[string]int64),
		AssessmentTrend:              []domainStatistics.DailyCount{},
	}

	if err := r.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&result.AssessmentCount).Error; err != nil {
		return nil, err
	}
	if err := r.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Count(&result.TesteeCount).Error; err != nil {
		return nil, err
	}

	result.QuestionnaireCount = 0
	result.AnswerSheetCount = 0

	var statusCounts []struct {
		Status string
		Count  int64
	}
	if err := r.WithContext(ctx).
		Table("assessment").
		Select("status, COUNT(*) as count").
		Where("org_id = ? AND deleted_at IS NULL", orgID).
		Group("status").
		Scan(&statusCounts).Error; err != nil {
		return nil, err
	}
	for _, row := range statusCounts {
		result.AssessmentStatusDistribution[row.Status] = row.Count
	}

	if err := r.fillRealtimeTodayFields(ctx, orgID, result); err != nil {
		return nil, err
	}
	result.AssessmentTrend = r.dailyTrend(ctx, orgID, domainStatistics.StatisticTypeSystem, "system")
	return result, nil
}

func (r *StatisticsRepository) BuildRealtimeQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, error) {
	aggregator := domainStatistics.NewAggregator()
	result := &domainStatistics.QuestionnaireStatistics{
		OrgID:              orgID,
		QuestionnaireCode:  questionnaireCode,
		OriginDistribution: make(map[string]int64),
		DailyTrend:         []domainStatistics.DailyCount{},
	}

	totalSubmissions, err := r.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, nil, "")
	if err != nil {
		return nil, err
	}
	totalCompletions, err := r.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, nil, "interpreted")
	if err != nil {
		return nil, err
	}
	result.TotalSubmissions = totalSubmissions
	result.TotalCompletions = totalCompletions
	result.CompletionRate = aggregator.CalculateCompletionRate(totalSubmissions, totalCompletions)

	last7dCount, err := r.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, daysAgo(7), "")
	if err != nil {
		return nil, err
	}
	last15dCount, err := r.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, daysAgo(15), "")
	if err != nil {
		return nil, err
	}
	last30dCount, err := r.countQuestionnaireAssessments(ctx, orgID, questionnaireCode, daysAgo(30), "")
	if err != nil {
		return nil, err
	}
	result.Last7DaysCount = last7dCount
	result.Last15DaysCount = last15dCount
	result.Last30DaysCount = last30dCount

	originDistribution, err := r.originDistribution(ctx, orgID, questionnaireCode)
	if err != nil {
		return nil, err
	}
	result.OriginDistribution = originDistribution
	result.DailyTrend = r.dailyTrend(ctx, orgID, domainStatistics.StatisticTypeQuestionnaire, questionnaireCode)
	return result, nil
}

func (r *StatisticsRepository) BuildRealtimeTesteeStatistics(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteeStatistics, error) {
	result := &domainStatistics.TesteeStatistics{
		OrgID:            orgID,
		TesteeID:         testeeID,
		RiskDistribution: make(map[string]int64),
	}

	if err := r.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Count(&result.TotalAssessments).Error; err != nil {
		return nil, err
	}
	if err := r.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND testee_id = ? AND status = 'interpreted' AND deleted_at IS NULL", orgID, testeeID).
		Count(&result.CompletedAssessments).Error; err != nil {
		return nil, err
	}
	result.PendingAssessments = result.TotalAssessments - result.CompletedAssessments

	var riskCounts []struct {
		RiskLevel string
		Count     int64
	}
	if err := r.WithContext(ctx).
		Table("assessment").
		Select("risk_level, COUNT(*) as count").
		Where("org_id = ? AND testee_id = ? AND risk_level IS NOT NULL AND deleted_at IS NULL", orgID, testeeID).
		Group("risk_level").
		Scan(&riskCounts).Error; err != nil {
		return nil, err
	}
	for _, row := range riskCounts {
		if row.RiskLevel != "" {
			result.RiskDistribution[row.RiskLevel] = row.Count
		}
	}

	var timeInfo struct {
		FirstAssessmentDate *time.Time
		LastAssessmentDate  *time.Time
	}
	if err := r.WithContext(ctx).
		Table("assessment").
		Select("MIN(created_at) as first_assessment_date, MAX(interpreted_at) as last_assessment_date").
		Where("org_id = ? AND testee_id = ? AND deleted_at IS NULL", orgID, testeeID).
		Scan(&timeInfo).Error; err == nil {
		result.FirstAssessmentDate = timeInfo.FirstAssessmentDate
		result.LastAssessmentDate = timeInfo.LastAssessmentDate
	}
	return result, nil
}

func (r *StatisticsRepository) BuildRealtimePlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, error) {
	result := &domainStatistics.PlanStatistics{OrgID: orgID, PlanID: planID}

	var taskStats struct {
		TotalTasks     int64
		CompletedTasks int64
		PendingTasks   int64
		ExpiredTasks   int64
	}
	if err := r.WithContext(ctx).
		Table("assessment_task").
		Select(`
			COUNT(*) as total_tasks,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_tasks,
			SUM(CASE WHEN status IN ('pending', 'opened') THEN 1 ELSE 0 END) as pending_tasks,
			SUM(CASE WHEN status = 'expired' THEN 1 ELSE 0 END) as expired_tasks
		`).
		Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", orgID, planID).
		Scan(&taskStats).Error; err != nil {
		return nil, err
	}
	result.TotalTasks = taskStats.TotalTasks
	result.CompletedTasks = taskStats.CompletedTasks
	result.PendingTasks = taskStats.PendingTasks
	result.ExpiredTasks = taskStats.ExpiredTasks
	result.CompletionRate = domainStatistics.NewAggregator().CalculateCompletionRate(taskStats.TotalTasks, taskStats.CompletedTasks)

	var testeeStats struct {
		EnrolledTestees int64
		ActiveTestees   int64
	}
	if err := r.WithContext(ctx).
		Table("assessment_task").
		Select(`
			COUNT(DISTINCT testee_id) as enrolled_testees,
			COUNT(DISTINCT CASE WHEN status = 'completed' THEN testee_id END) as active_testees
		`).
		Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", orgID, planID).
		Scan(&testeeStats).Error; err != nil {
		return nil, err
	}
	result.EnrolledTestees = testeeStats.EnrolledTestees
	result.ActiveTestees = testeeStats.ActiveTestees
	return result, nil
}

func (r *StatisticsRepository) fillRealtimeTodayFields(ctx context.Context, orgID int64, result *domainStatistics.SystemStatistics) error {
	todayStart, tomorrowStart := currentDayBounds(time.Now())

	if err := r.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?", orgID, todayStart, tomorrowStart).
		Count(&result.TodayNewAssessments).Error; err != nil {
		return err
	}
	if err := r.WithContext(ctx).
		Table("testee").
		Where("org_id = ? AND deleted_at IS NULL AND created_at >= ? AND created_at < ?", orgID, todayStart, tomorrowStart).
		Count(&result.TodayNewTestees).Error; err != nil {
		return err
	}
	result.TodayNewAnswerSheets = 0
	return nil
}

func (r *StatisticsRepository) dailyTrend(ctx context.Context, orgID int64, statType domainStatistics.StatisticType, statKey string) []domainStatistics.DailyCount {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)
	dailyPOs, err := r.GetDailyStatistics(ctx, orgID, statType, statKey, startDate, endDate)
	if err != nil || len(dailyPOs) == 0 {
		return []domainStatistics.DailyCount{}
	}
	trend := make([]domainStatistics.DailyCount, 0, len(dailyPOs))
	for _, po := range dailyPOs {
		trend = append(trend, domainStatistics.DailyCount{Date: po.StatDate, Count: po.SubmissionCount})
	}
	return trend
}

func (r *StatisticsRepository) countQuestionnaireAssessments(ctx context.Context, orgID int64, questionnaireCode string, createdAfter *time.Time, status string) (int64, error) {
	query := r.WithContext(ctx).
		Table("assessment").
		Where("org_id = ? AND questionnaire_code = ? AND deleted_at IS NULL", orgID, questionnaireCode)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if createdAfter != nil {
		query = query.Where("created_at >= ?", *createdAfter)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (r *StatisticsRepository) originDistribution(ctx context.Context, orgID int64, questionnaireCode string) (map[string]int64, error) {
	var originCounts []struct {
		OriginType string
		Count      int64
	}
	if err := r.WithContext(ctx).
		Table("assessment").
		Select("origin_type, COUNT(*) as count").
		Where("org_id = ? AND questionnaire_code = ? AND deleted_at IS NULL", orgID, questionnaireCode).
		Group("origin_type").
		Scan(&originCounts).Error; err != nil {
		return nil, err
	}
	distribution := make(map[string]int64)
	for _, row := range originCounts {
		distribution[row.OriginType] = row.Count
	}
	return distribution, nil
}

func daysAgo(days int) *time.Time {
	t := time.Now().AddDate(0, 0, -days)
	return &t
}

func currentDayBounds(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 1)
}
