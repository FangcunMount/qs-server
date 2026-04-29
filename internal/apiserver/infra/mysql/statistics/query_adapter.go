package statistics

import (
	"context"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func (r *StatisticsRepository) LoadSystemStatistics(ctx context.Context, orgID int64) (*domainStatistics.SystemStatistics, bool, error) {
	po, err := r.GetAccumulatedStatistics(ctx, orgID, domainStatistics.StatisticTypeSystem, "system")
	if err != nil || po == nil {
		return nil, false, err
	}
	stats := r.accumulatedPOToSystemStatistics(ctx, po, orgID)
	if err := r.fillRealtimeTodayFields(ctx, orgID, stats); err != nil {
		return nil, false, err
	}
	return stats, true, nil
}

func (r *StatisticsRepository) LoadQuestionnaireStatistics(ctx context.Context, orgID int64, questionnaireCode string) (*domainStatistics.QuestionnaireStatistics, bool, error) {
	po, err := r.GetAccumulatedStatistics(ctx, orgID, domainStatistics.StatisticTypeQuestionnaire, questionnaireCode)
	if err != nil || po == nil {
		return nil, false, err
	}

	stats := r.accumulatedPOToQuestionnaireStatistics(po, orgID, questionnaireCode)
	stats.DailyTrend = r.dailyTrend(ctx, orgID, domainStatistics.StatisticTypeQuestionnaire, questionnaireCode)
	if len(stats.OriginDistribution) == 0 {
		originDistribution, originErr := r.originDistribution(ctx, orgID, questionnaireCode)
		if originErr != nil {
			return nil, false, originErr
		}
		stats.OriginDistribution = originDistribution
	}
	return stats, true, nil
}

func (r *StatisticsRepository) LoadPlanStatistics(ctx context.Context, orgID int64, planID uint64) (*domainStatistics.PlanStatistics, bool, error) {
	po, err := r.GetPlanStatistics(ctx, orgID, planID)
	if err != nil || po == nil {
		return nil, false, err
	}
	return r.planPOToPlanStatistics(ctx, po), true, nil
}

func (r *StatisticsRepository) accumulatedPOToSystemStatistics(ctx context.Context, po *StatisticsAccumulatedPO, orgID int64) *domainStatistics.SystemStatistics {
	result := &domainStatistics.SystemStatistics{
		OrgID:                        orgID,
		AssessmentCount:              po.TotalSubmissions,
		AssessmentStatusDistribution: make(map[string]int64),
		AssessmentTrend:              []domainStatistics.DailyCount{},
	}
	if po.Distribution != nil {
		if statusDist, ok := po.Distribution["status"].(map[string]interface{}); ok {
			for key, value := range statusDist {
				if count, ok := value.(float64); ok {
					result.AssessmentStatusDistribution[key] = int64(count)
				}
			}
		}
		if questionnaireCount, ok := po.Distribution["questionnaire_count"].(float64); ok {
			result.QuestionnaireCount = int64(questionnaireCount)
		}
		if answerSheetCount, ok := po.Distribution["answer_sheet_count"].(float64); ok {
			result.AnswerSheetCount = int64(answerSheetCount)
		}
		if testeeCount, ok := po.Distribution["testee_count"].(float64); ok {
			result.TesteeCount = int64(testeeCount)
		}
		if todayNewAssessments, ok := po.Distribution["today_new_assessments"].(float64); ok {
			result.TodayNewAssessments = int64(todayNewAssessments)
		}
		if todayNewAnswerSheets, ok := po.Distribution["today_new_answer_sheets"].(float64); ok {
			result.TodayNewAnswerSheets = int64(todayNewAnswerSheets)
		}
		if todayNewTestees, ok := po.Distribution["today_new_testees"].(float64); ok {
			result.TodayNewTestees = int64(todayNewTestees)
		}
	}
	result.AssessmentTrend = r.dailyTrend(ctx, orgID, domainStatistics.StatisticTypeSystem, "system")
	return result
}

func (r *StatisticsRepository) accumulatedPOToQuestionnaireStatistics(po *StatisticsAccumulatedPO, orgID int64, questionnaireCode string) *domainStatistics.QuestionnaireStatistics {
	result := &domainStatistics.QuestionnaireStatistics{
		OrgID:              orgID,
		QuestionnaireCode:  questionnaireCode,
		TotalSubmissions:   po.TotalSubmissions,
		TotalCompletions:   po.TotalCompletions,
		Last7DaysCount:     po.Last7dSubmissions,
		Last15DaysCount:    po.Last15dSubmissions,
		Last30DaysCount:    po.Last30dSubmissions,
		OriginDistribution: make(map[string]int64),
		DailyTrend:         []domainStatistics.DailyCount{},
	}
	result.CompletionRate = domainStatistics.NewAggregator().CalculateCompletionRate(po.TotalSubmissions, po.TotalCompletions)
	if po.Distribution != nil {
		if originDist, ok := po.Distribution["origin"].(map[string]interface{}); ok {
			for key, value := range originDist {
				if count, ok := value.(float64); ok {
					result.OriginDistribution[key] = int64(count)
				}
			}
		}
	}
	return result
}

func (r *StatisticsRepository) planPOToPlanStatistics(ctx context.Context, po *StatisticsPlanPO) *domainStatistics.PlanStatistics {
	result := &domainStatistics.PlanStatistics{
		OrgID:           po.OrgID,
		PlanID:          po.PlanID,
		TotalTasks:      po.TotalTasks,
		CompletedTasks:  po.CompletedTasks,
		PendingTasks:    po.PendingTasks,
		ExpiredTasks:    po.ExpiredTasks,
		EnrolledTestees: po.EnrolledTestees,
		ActiveTestees:   po.ActiveTestees,
	}
	result.CompletionRate = domainStatistics.NewAggregator().CalculateCompletionRate(po.TotalTasks, po.CompletedTasks)
	if r != nil && r.db != nil {
		result.Window = r.planTaskWindow(ctx, po.OrgID, po.PlanID)
		result.Trend = domainStatistics.PlanTaskTrend{
			TaskCreated:   r.planTaskTrend(ctx, po.OrgID, po.PlanID, "task_created_count"),
			TaskOpened:    r.planTaskTrend(ctx, po.OrgID, po.PlanID, "task_opened_count"),
			TaskCompleted: r.planTaskTrend(ctx, po.OrgID, po.PlanID, "task_completed_count"),
			TaskExpired:   r.planTaskTrend(ctx, po.OrgID, po.PlanID, "task_expired_count"),
		}
	}
	return result
}

func (r *StatisticsRepository) planTaskWindow(ctx context.Context, orgID int64, planID uint64) domainStatistics.PlanTaskWindow {
	from, to := defaultPlanTaskTrendRange()
	var row struct {
		TaskCreatedCount   int64
		TaskOpenedCount    int64
		TaskCompletedCount int64
		TaskExpiredCount   int64
		EnrolledTestees    int64
		ActiveTestees      int64
	}
	if err := r.db.WithContext(ctx).
		Table("analytics_plan_task_daily").
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
	row.EnrolledTestees = r.countPlanTaskDistinctTestees(ctx, orgID, planID, "created_at", "", from, to)
	row.ActiveTestees = r.countPlanTaskDistinctTestees(ctx, orgID, planID, "completed_at", "completed", from, to)
	return domainStatistics.PlanTaskWindow{
		TaskCreatedCount:   row.TaskCreatedCount,
		TaskOpenedCount:    row.TaskOpenedCount,
		TaskCompletedCount: row.TaskCompletedCount,
		TaskExpiredCount:   row.TaskExpiredCount,
		EnrolledTestees:    row.EnrolledTestees,
		ActiveTestees:      row.ActiveTestees,
	}
}

func (r *StatisticsRepository) countPlanTaskDistinctTestees(ctx context.Context, orgID int64, planID uint64, timeField, status string, from, to time.Time) int64 {
	var row struct {
		Count int64
	}
	query := r.db.WithContext(ctx).
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
	var rows []struct {
		StatDate time.Time
		Count    int64
	}
	if err := r.db.WithContext(ctx).
		Table("analytics_plan_task_daily").
		Select("stat_date, COALESCE("+field+", 0) AS count").
		Where("org_id = ? AND plan_id = ? AND stat_date >= ? AND stat_date < ? AND deleted_at IS NULL", orgID, planID, from, to).
		Order("stat_date ASC").
		Scan(&rows).Error; err != nil {
		return []domainStatistics.DailyCount{}
	}
	result := make([]domainStatistics.DailyCount, 0, len(rows))
	for _, row := range rows {
		result = append(result, domainStatistics.DailyCount{Date: row.StatDate, Count: row.Count})
	}
	return result
}

func defaultPlanTaskTrendRange() (time.Time, time.Time) {
	now := time.Now().In(time.Local)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return todayStart.AddDate(0, 0, -30), todayStart.AddDate(0, 0, 1)
}
