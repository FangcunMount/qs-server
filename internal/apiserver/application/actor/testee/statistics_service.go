package testee

import (
	"context"
	"sort"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// statisticsService 受试者统计服务实现
// 行为者：管理员、数据分析系统
// 职责：提供受试者的测评数据统计和分析能力
type statisticsService struct {
	testeeRepo       domain.Repository
	assessmentReader evaluationreadmodel.AssessmentReader
	scoreReader      evaluationreadmodel.ScoreReader
	reportReader     evaluationreadmodel.ReportReader
}

// NewStatisticsService 创建受试者统计服务。
//
// Deprecated: evaluation command repositories no longer expose read-model
// methods. Use NewStatisticsServiceWithReadModels when this legacy service is
// still needed.
func NewStatisticsService(
	testeeRepo domain.Repository,
	_ assessment.Repository,
	_ assessment.ScoreRepository,
	_ report.ReportRepository,
) TesteeStatisticsService {
	return &statisticsService{
		testeeRepo: testeeRepo,
	}
}

func NewStatisticsServiceWithReadModels(
	testeeRepo domain.Repository,
	assessmentReader evaluationreadmodel.AssessmentReader,
	scoreReader evaluationreadmodel.ScoreReader,
	reportReader evaluationreadmodel.ReportReader,
) TesteeStatisticsService {
	return &statisticsService{
		testeeRepo:       testeeRepo,
		assessmentReader: assessmentReader,
		scoreReader:      scoreReader,
		reportReader:     reportReader,
	}
}

// GetScaleAnalysis 获取受试者的量表趋势分析
// 场景：查看受试者在各个量表上的历史得分变化
// 用于绘制趋势图表，分析干预效果
func (s *statisticsService) GetScaleAnalysis(ctx context.Context, testeeID uint64) (*ScaleAnalysisResult, error) {
	testeeItem, err := s.loadTestee(ctx, testeeID)
	if err != nil {
		return nil, err
	}

	rows, err := s.loadAssessmentRows(ctx, testeeItem.ID().Uint64())
	if err != nil {
		return nil, err
	}

	scales := s.buildScaleAnalyses(ctx, filterScaleAnalysisRows(rows))

	return &ScaleAnalysisResult{
		TesteeID: testeeID,
		Scales:   scales,
	}, nil
}

func filterScaleAnalysisRows(items []evaluationreadmodel.AssessmentRow) []evaluationreadmodel.AssessmentRow {
	filtered := make([]evaluationreadmodel.AssessmentRow, 0, len(items))
	for _, item := range items {
		if assessment.Status(item.Status) != assessment.StatusInterpreted || item.MedicalScaleID == nil {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (s *statisticsService) buildScaleAnalyses(ctx context.Context, items []evaluationreadmodel.AssessmentRow) []ScaleTrendAnalysis {
	scaleMap := make(map[uint64]*ScaleTrendAnalysis)
	for _, item := range items {
		scaleTrend := ensureScaleTrendAnalysis(scaleMap, item)
		scaleTrend.Tests = append(scaleTrend.Tests, s.buildScaleTestRecord(ctx, item))
	}
	return finalizeScaleTrendAnalyses(scaleMap)
}

func ensureScaleTrendAnalysis(scaleMap map[uint64]*ScaleTrendAnalysis, item evaluationreadmodel.AssessmentRow) *ScaleTrendAnalysis {
	scaleID := uint64(0)
	if item.MedicalScaleID != nil {
		scaleID = *item.MedicalScaleID
	}
	if existing, ok := scaleMap[scaleID]; ok {
		return existing
	}

	scaleTrend := &ScaleTrendAnalysis{
		ScaleID:   scaleID,
		ScaleCode: derefString(item.MedicalScaleCode),
		ScaleName: derefString(item.MedicalScaleName),
		Tests:     make([]TestRecordData, 0),
	}
	scaleMap[scaleID] = scaleTrend
	return scaleTrend
}

func (s *statisticsService) buildScaleTestRecord(ctx context.Context, item evaluationreadmodel.AssessmentRow) TestRecordData {
	record := TestRecordData{
		AssessmentID: item.ID,
	}
	if item.InterpretedAt != nil {
		record.TestDate = *item.InterpretedAt
	} else if item.SubmittedAt != nil {
		record.TestDate = *item.SubmittedAt
	}
	if item.TotalScore != nil {
		record.TotalScore = *item.TotalScore
	}
	if item.RiskLevel != nil {
		record.RiskLevel = *item.RiskLevel
	}

	s.applyScaleReport(ctx, item, &record)
	s.appendScaleFactorScores(ctx, item, &record)
	return record
}

func (s *statisticsService) applyScaleReport(ctx context.Context, item evaluationreadmodel.AssessmentRow, record *TestRecordData) {
	if s.reportReader == nil {
		return
	}
	reportItem, err := s.reportReader.GetReportByAssessmentID(ctx, item.ID)
	if err == nil && reportItem != nil {
		record.Result = reportItem.Conclusion
	}
}

func (s *statisticsService) appendScaleFactorScores(ctx context.Context, item evaluationreadmodel.AssessmentRow, record *TestRecordData) {
	if s.scoreReader == nil {
		return
	}
	score, err := s.scoreReader.GetScoreByAssessmentID(ctx, item.ID)
	if err != nil || score == nil {
		return
	}

	for _, factorScore := range score.FactorScores {
		if factorScore.IsTotalScore {
			continue
		}
		record.Factors = append(record.Factors, FactorScoreData{
			FactorCode: factorScore.FactorCode,
			FactorName: factorScore.FactorName,
			RawScore:   factorScore.RawScore,
			RiskLevel:  factorScore.RiskLevel,
		})
	}
}

func finalizeScaleTrendAnalyses(scaleMap map[uint64]*ScaleTrendAnalysis) []ScaleTrendAnalysis {
	scales := make([]ScaleTrendAnalysis, 0, len(scaleMap))
	for _, scaleTrend := range scaleMap {
		sort.Slice(scaleTrend.Tests, func(i, j int) bool {
			return scaleTrend.Tests[i].TestDate.Before(scaleTrend.Tests[j].TestDate)
		})
		scales = append(scales, *scaleTrend)
	}
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].ScaleID < scales[j].ScaleID
	})
	return scales
}

// GetPeriodicStats 获取受试者参与的周期性测评项目统计
// 场景：查看受试者在周期性干预项目中的完成进度
// 用于监控长期干预计划的执行情况
func (s *statisticsService) GetPeriodicStats(ctx context.Context, testeeID uint64) (*PeriodicStatsResult, error) {
	testeeItem, err := s.loadTestee(ctx, testeeID)
	if err != nil {
		return nil, err
	}

	rows, err := s.loadAssessmentRows(ctx, testeeItem.ID().Uint64())
	if err != nil {
		return nil, err
	}

	projects, activeCount := buildPeriodicProjects(groupAssessmentRowsByPlan(rows))

	return &PeriodicStatsResult{
		TesteeID:       testeeID,
		Projects:       projects,
		TotalProjects:  len(projects),
		ActiveProjects: activeCount,
	}, nil
}

func (s *statisticsService) loadTestee(ctx context.Context, testeeID uint64) (*domain.Testee, error) {
	targetTesteeID, err := testeeIDFromUint64("testee_id", testeeID)
	if err != nil {
		return nil, err
	}
	testeeItem, err := s.testeeRepo.FindByID(ctx, targetTesteeID)
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee")
	}
	return testeeItem, nil
}

func (s *statisticsService) loadAssessmentRows(ctx context.Context, testeeID uint64) ([]evaluationreadmodel.AssessmentRow, error) {
	if s.assessmentReader == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "assessment read model is not configured")
	}
	rows, _, err := s.assessmentReader.ListAssessments(
		ctx,
		evaluationreadmodel.AssessmentFilter{TesteeID: &testeeID},
		evaluationreadmodel.PageRequest{Page: 1, PageSize: 1000},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find assessments")
	}
	return rows, nil
}

func groupAssessmentRowsByPlan(items []evaluationreadmodel.AssessmentRow) map[string][]evaluationreadmodel.AssessmentRow {
	planMap := make(map[string][]evaluationreadmodel.AssessmentRow)
	for _, item := range items {
		if assessment.OriginType(item.OriginType) != assessment.OriginPlan {
			continue
		}
		if item.OriginID == nil || *item.OriginID == "" {
			continue
		}
		planMap[*item.OriginID] = append(planMap[*item.OriginID], item)
	}
	return planMap
}

func buildPeriodicProjects(planMap map[string][]evaluationreadmodel.AssessmentRow) ([]PeriodicProjectStats, int) {
	projects := make([]PeriodicProjectStats, 0, len(planMap))
	activeCount := 0

	for planID, planAssessments := range planMap {
		project, isActive := buildPeriodicProject(planID, planAssessments)
		if isActive {
			activeCount++
		}
		projects = append(projects, project)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ProjectName < projects[j].ProjectName
	})
	return projects, activeCount
}

func buildPeriodicProject(planID string, planAssessments []evaluationreadmodel.AssessmentRow) (PeriodicProjectStats, bool) {
	sortAssessmentRowsBySubmittedAt(planAssessments)

	completedCount := countCompletedAssessmentRows(planAssessments)
	tasks, startDate, endDate := buildPeriodicTasks(planAssessments)
	totalWeeks := len(planAssessments)

	return PeriodicProjectStats{
		ProjectID:      0, // TODO: 需要从 plan 领域获取真实的项目ID
		ProjectName:    planID,
		ScaleName:      periodicScaleName(planAssessments),
		TotalWeeks:     totalWeeks,
		CompletedWeeks: completedCount,
		CompletionRate: calculateCompletionRate(completedCount, totalWeeks),
		CurrentWeek:    calculateCurrentWeek(completedCount, totalWeeks),
		Tasks:          tasks,
		StartDate:      startDate,
		EndDate:        endDate,
	}, completedCount < totalWeeks
}

func sortAssessmentRowsBySubmittedAt(items []evaluationreadmodel.AssessmentRow) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i].SubmittedAt
		right := items[j].SubmittedAt
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.Before(*right)
	})
}

func countCompletedAssessmentRows(items []evaluationreadmodel.AssessmentRow) int {
	count := 0
	for _, item := range items {
		if assessment.Status(item.Status) == assessment.StatusInterpreted {
			count++
		}
	}
	return count
}

func buildPeriodicTasks(items []evaluationreadmodel.AssessmentRow) ([]PeriodicTask, *time.Time, *time.Time) {
	tasks := make([]PeriodicTask, 0, len(items))
	var startDate *time.Time
	var endDate *time.Time

	for index, item := range items {
		task := buildPeriodicTask(index+1, item)
		startDate, endDate = expandPeriodicWindow(startDate, endDate, item.SubmittedAt)
		tasks = append(tasks, task)
	}

	return tasks, startDate, endDate
}

func buildPeriodicTask(week int, item evaluationreadmodel.AssessmentRow) PeriodicTask {
	task := PeriodicTask{
		Week:    week,
		DueDate: item.SubmittedAt,
	}

	switch assessment.Status(item.Status) {
	case assessment.StatusInterpreted:
		task.Status = "completed"
		task.CompletedAt = item.InterpretedAt
		assessmentID := item.ID
		task.AssessmentID = &assessmentID
	case assessment.StatusFailed:
		task.Status = "overdue"
	default:
		task.Status = "pending"
	}

	return task
}

func expandPeriodicWindow(startDate, endDate, submittedAt *time.Time) (*time.Time, *time.Time) {
	if submittedAt == nil {
		return startDate, endDate
	}
	if startDate == nil || submittedAt.Before(*startDate) {
		startDate = submittedAt
	}
	if endDate == nil || submittedAt.After(*endDate) {
		endDate = submittedAt
	}
	return startDate, endDate
}

func periodicScaleName(items []evaluationreadmodel.AssessmentRow) string {
	for _, item := range items {
		if item.MedicalScaleName != nil {
			return *item.MedicalScaleName
		}
	}
	return ""
}

func calculateCompletionRate(completedCount, totalWeeks int) float64 {
	if totalWeeks == 0 {
		return 0
	}
	return float64(completedCount) / float64(totalWeeks) * 100
}

func calculateCurrentWeek(completedCount, totalWeeks int) int {
	currentWeek := completedCount + 1
	if currentWeek > totalWeeks {
		return totalWeeks
	}
	return currentWeek
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
