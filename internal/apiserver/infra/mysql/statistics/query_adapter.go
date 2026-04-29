package statistics

import (
	"context"

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
	return r.planPOToPlanStatistics(po), true, nil
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

func (r *StatisticsRepository) planPOToPlanStatistics(po *StatisticsPlanPO) *domainStatistics.PlanStatistics {
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
	return result
}
