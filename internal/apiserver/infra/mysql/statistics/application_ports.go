package statistics

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	actorInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	evaluationInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	planInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
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

func (r *StatisticsRepository) GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteePeriodicStatisticsResponse, error) {
	if err := r.ensureTesteeExists(ctx, orgID, testeeID); err != nil {
		return nil, err
	}
	tasks, err := r.loadPeriodicTasks(ctx, orgID, testeeID)
	if err != nil {
		return nil, err
	}
	projectMap, assessmentIDs := groupPeriodicTasksByPlan(tasks)
	assessmentNames, err := r.loadAssessmentNames(ctx, orgID, assessmentIDs)
	if err != nil {
		return nil, err
	}
	return buildPeriodicStatsResponse(projectMap, assessmentNames), nil
}

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

	return tx.Exec(
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
	).Error
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
	return tx.Create(systemRow).Error
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
	if len(planRows) == 0 {
		return nil
	}
	return tx.CreateInBatches(planRows, 200).Error
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

func (r *StatisticsRepository) ensureTesteeExists(ctx context.Context, orgID int64, testeeID uint64) error {
	var testee actorInfra.TesteePO
	if err := r.WithContext(ctx).
		Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, testeeID).
		First(&testee).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return err
	}
	return nil
}

func (r *StatisticsRepository) loadPeriodicTasks(ctx context.Context, orgID int64, testeeID uint64) ([]planInfra.AssessmentTaskPO, error) {
	var tasks []planInfra.AssessmentTaskPO
	if err := r.WithContext(ctx).
		Table("assessment_task t").
		Joins("JOIN assessment_plan p ON p.id = t.plan_id AND p.deleted_at IS NULL").
		Where("t.org_id = ? AND t.testee_id = ? AND t.deleted_at IS NULL", orgID, testeeID).
		Order("t.plan_id ASC, t.seq ASC, t.planned_at ASC, t.id ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func groupPeriodicTasksByPlan(tasks []planInfra.AssessmentTaskPO) (map[string][]planInfra.AssessmentTaskPO, []uint64) {
	projectMap := make(map[string][]planInfra.AssessmentTaskPO)
	assessmentIDs := make([]uint64, 0)
	for _, item := range tasks {
		planID := strconv.FormatUint(item.PlanID, 10)
		projectMap[planID] = append(projectMap[planID], item)
		if item.AssessmentID != nil {
			assessmentIDs = append(assessmentIDs, *item.AssessmentID)
		}
	}
	return projectMap, assessmentIDs
}

func (r *StatisticsRepository) loadAssessmentNames(ctx context.Context, orgID int64, assessmentIDs []uint64) (map[uint64]string, error) {
	assessmentNames := make(map[uint64]string, len(assessmentIDs))
	if len(assessmentIDs) == 0 {
		return assessmentNames, nil
	}
	var assessments []evaluationInfra.AssessmentPO
	if err := r.WithContext(ctx).
		Select("id, medical_scale_name").
		Where("org_id = ? AND id IN ? AND deleted_at IS NULL", orgID, assessmentIDs).
		Find(&assessments).Error; err != nil {
		return nil, err
	}
	for _, item := range assessments {
		if item.MedicalScaleName != nil && strings.TrimSpace(*item.MedicalScaleName) != "" {
			assessmentNames[item.ID.Uint64()] = strings.TrimSpace(*item.MedicalScaleName)
		}
	}
	return assessmentNames, nil
}

func buildPeriodicStatsResponse(projectMap map[string][]planInfra.AssessmentTaskPO, assessmentNames map[uint64]string) *domainStatistics.TesteePeriodicStatisticsResponse {
	projects := make([]domainStatistics.TesteePeriodicProjectStatistics, 0, len(projectMap))
	activeProjects := 0
	for planID, items := range projectMap {
		project, hasActiveTask := buildPeriodicProjectStatistics(planID, items, assessmentNames)
		if hasActiveTask {
			activeProjects++
		}
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ProjectID < projects[j].ProjectID
	})
	return &domainStatistics.TesteePeriodicStatisticsResponse{
		Projects:       projects,
		TotalProjects:  len(projects),
		ActiveProjects: activeProjects,
	}
}

func buildPeriodicProjectStatistics(planID string, items []planInfra.AssessmentTaskPO, assessmentNames map[uint64]string) (domainStatistics.TesteePeriodicProjectStatistics, bool) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Seq == items[j].Seq {
			if items[i].PlannedAt.Equal(items[j].PlannedAt) {
				return items[i].ID < items[j].ID
			}
			return items[i].PlannedAt.Before(items[j].PlannedAt)
		}
		return items[i].Seq < items[j].Seq
	})

	completed := 0
	currentWeek := 0
	hasActiveTask := false
	scaleName := ""
	tasks := make([]domainStatistics.TesteePeriodicTaskStatistics, 0, len(items))
	var startDate *time.Time
	var endDate *time.Time

	for _, item := range items {
		task := buildPeriodicTaskStatistics(item)
		tasks = append(tasks, task)
		if task.Status == "completed" {
			completed++
		} else if currentWeek == 0 {
			currentWeek = item.Seq
		}
		if item.Status == "pending" || item.Status == "opened" {
			hasActiveTask = true
		}
		scaleName = pickPeriodicScaleName(scaleName, item, assessmentNames)
		startDate, endDate = expandPeriodicWindow(startDate, endDate, item)
	}

	totalWeeks := len(items)
	if totalWeeks == 0 {
		currentWeek = 0
	} else if currentWeek == 0 {
		currentWeek = totalWeeks
	}
	if scaleName == "" {
		scaleName = "未命名量表"
	}

	return domainStatistics.TesteePeriodicProjectStatistics{
		ProjectID:      planID,
		ProjectName:    scaleName,
		ScaleName:      scaleName,
		TotalWeeks:     totalWeeks,
		CompletedWeeks: completed,
		CompletionRate: calculatePeriodicCompletionRate(completed, totalWeeks),
		CurrentWeek:    currentWeek,
		Tasks:          tasks,
		StartDate:      startDate,
		EndDate:        endDate,
	}, hasActiveTask
}

func buildPeriodicTaskStatistics(item planInfra.AssessmentTaskPO) domainStatistics.TesteePeriodicTaskStatistics {
	task := domainStatistics.TesteePeriodicTaskStatistics{
		Week:      item.Seq,
		Status:    periodicTaskStatus(item.Status),
		PlannedAt: cloneTime(item.PlannedAt),
		DueDate:   cloneTimePtr(item.ExpireAt),
	}
	if item.CompletedAt != nil {
		task.CompletedAt = cloneTimePtr(item.CompletedAt)
	}
	if item.AssessmentID != nil {
		assessmentID := strconv.FormatUint(*item.AssessmentID, 10)
		task.AssessmentID = &assessmentID
	}
	return task
}

func pickPeriodicScaleName(current string, item planInfra.AssessmentTaskPO, assessmentNames map[uint64]string) string {
	if current != "" {
		return current
	}
	if item.AssessmentID != nil && assessmentNames[*item.AssessmentID] != "" {
		return assessmentNames[*item.AssessmentID]
	}
	return strings.TrimSpace(item.ScaleCode)
}

func expandPeriodicWindow(startDate, endDate *time.Time, item planInfra.AssessmentTaskPO) (*time.Time, *time.Time) {
	if startDate == nil || item.PlannedAt.Before(*startDate) {
		startDate = cloneTime(item.PlannedAt)
	}
	if item.ExpireAt != nil {
		if endDate == nil || item.ExpireAt.After(*endDate) {
			endDate = cloneTimePtr(item.ExpireAt)
		}
		return startDate, endDate
	}
	if endDate == nil || item.PlannedAt.After(*endDate) {
		endDate = cloneTime(item.PlannedAt)
	}
	return startDate, endDate
}

func calculatePeriodicCompletionRate(completed, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(completed) / float64(total) * 100
}

func periodicTaskStatus(status string) string {
	switch status {
	case "completed":
		return "completed"
	case "expired":
		return "overdue"
	case "canceled":
		return "canceled"
	default:
		return "pending"
	}
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

func daysAgo(days int) *time.Time {
	t := time.Now().AddDate(0, 0, -days)
	return &t
}

func currentDayBounds(now time.Time) (time.Time, time.Time) {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 1)
}

func cloneTime(value time.Time) *time.Time {
	t := value
	return &t
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	t := *value
	return &t
}
