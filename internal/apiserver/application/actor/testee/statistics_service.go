package testee

import (
	"context"
	"sort"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// statisticsService 受试者统计服务实现
// 行为者：管理员、数据分析系统
// 职责：提供受试者的测评数据统计和分析能力
type statisticsService struct {
	testeeRepo     domain.Repository
	assessmentRepo assessment.Repository
	scoreRepo      assessment.ScoreRepository
	reportRepo     report.ReportRepository
}

// NewStatisticsService 创建受试者统计服务
func NewStatisticsService(
	testeeRepo domain.Repository,
	assessmentRepo assessment.Repository,
	scoreRepo assessment.ScoreRepository,
	reportRepo report.ReportRepository,
) TesteeStatisticsService {
	return &statisticsService{
		testeeRepo:     testeeRepo,
		assessmentRepo: assessmentRepo,
		scoreRepo:      scoreRepo,
		reportRepo:     reportRepo,
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

	assessments, err := s.loadAssessments(ctx, testeeItem.ID())
	if err != nil {
		return nil, err
	}

	scales := s.buildScaleAnalyses(ctx, filterScaleAnalysisAssessments(assessments))

	return &ScaleAnalysisResult{
		TesteeID: testeeID,
		Scales:   scales,
	}, nil
}

func filterScaleAnalysisAssessments(items []*assessment.Assessment) []*assessment.Assessment {
	filtered := make([]*assessment.Assessment, 0, len(items))
	for _, item := range items {
		if item == nil || item.Status() != assessment.StatusInterpreted || !item.HasMedicalScale() {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (s *statisticsService) buildScaleAnalyses(ctx context.Context, items []*assessment.Assessment) []ScaleTrendAnalysis {
	scaleMap := make(map[uint64]*ScaleTrendAnalysis)
	for _, item := range items {
		scaleTrend := ensureScaleTrendAnalysis(scaleMap, item)
		scaleTrend.Tests = append(scaleTrend.Tests, s.buildScaleTestRecord(ctx, item))
	}
	return finalizeScaleTrendAnalyses(scaleMap)
}

func ensureScaleTrendAnalysis(scaleMap map[uint64]*ScaleTrendAnalysis, item *assessment.Assessment) *ScaleTrendAnalysis {
	scaleRef := item.MedicalScaleRef()
	scaleID := scaleRef.ID().Uint64()
	if existing, ok := scaleMap[scaleID]; ok {
		return existing
	}

	scaleTrend := &ScaleTrendAnalysis{
		ScaleID:   scaleID,
		ScaleCode: string(scaleRef.Code()),
		ScaleName: scaleRef.Name(),
		Tests:     make([]TestRecordData, 0),
	}
	scaleMap[scaleID] = scaleTrend
	return scaleTrend
}

func (s *statisticsService) buildScaleTestRecord(ctx context.Context, item *assessment.Assessment) TestRecordData {
	record := TestRecordData{
		AssessmentID: item.ID().Uint64(),
		TestDate:     *item.SubmittedAt(),
	}
	if item.TotalScore() != nil {
		record.TotalScore = *item.TotalScore()
	}
	if item.RiskLevel() != nil {
		record.RiskLevel = string(*item.RiskLevel())
	}

	s.applyScaleReport(ctx, item, &record)
	s.appendScaleFactorScores(ctx, item, &record)
	return record
}

func (s *statisticsService) applyScaleReport(ctx context.Context, item *assessment.Assessment, record *TestRecordData) {
	reportItem, err := s.reportRepo.FindByAssessmentID(ctx, item.ID())
	if err == nil && reportItem != nil {
		record.Result = reportItem.Conclusion()
	}
}

func (s *statisticsService) appendScaleFactorScores(ctx context.Context, item *assessment.Assessment, record *TestRecordData) {
	scores, err := s.scoreRepo.FindByAssessmentID(ctx, item.ID())
	if err != nil {
		return
	}

	for _, score := range scores {
		for _, factorScore := range score.FactorScores() {
			if factorScore.IsTotalScore() {
				continue
			}
			record.Factors = append(record.Factors, FactorScoreData{
				FactorCode: string(factorScore.FactorCode()),
				FactorName: factorScore.FactorName(),
				RawScore:   factorScore.RawScore(),
				RiskLevel:  string(factorScore.RiskLevel()),
			})
		}
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

	assessments, err := s.loadAssessments(ctx, testeeItem.ID())
	if err != nil {
		return nil, err
	}

	projects, activeCount := buildPeriodicProjects(groupAssessmentsByPlan(assessments))

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

func (s *statisticsService) loadAssessments(ctx context.Context, testeeID domain.ID) ([]*assessment.Assessment, error) {
	pagination := assessment.NewPagination(1, 1000)
	assessments, _, err := s.assessmentRepo.FindByTesteeID(ctx, testeeID, pagination)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find assessments")
	}
	return assessments, nil
}

func groupAssessmentsByPlan(items []*assessment.Assessment) map[string][]*assessment.Assessment {
	planMap := make(map[string][]*assessment.Assessment)
	for _, item := range items {
		if item == nil || item.Origin().Type() != assessment.OriginPlan {
			continue
		}
		planID := item.Origin().ID()
		if planID == nil || *planID == "" {
			continue
		}
		planMap[*planID] = append(planMap[*planID], item)
	}
	return planMap
}

func buildPeriodicProjects(planMap map[string][]*assessment.Assessment) ([]PeriodicProjectStats, int) {
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

func buildPeriodicProject(planID string, planAssessments []*assessment.Assessment) (PeriodicProjectStats, bool) {
	sortAssessmentsBySubmittedAt(planAssessments)

	completedCount := countCompletedAssessments(planAssessments)
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

func sortAssessmentsBySubmittedAt(items []*assessment.Assessment) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i].SubmittedAt()
		right := items[j].SubmittedAt()
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		return left.Before(*right)
	})
}

func countCompletedAssessments(items []*assessment.Assessment) int {
	count := 0
	for _, item := range items {
		if item.Status() == assessment.StatusInterpreted {
			count++
		}
	}
	return count
}

func buildPeriodicTasks(items []*assessment.Assessment) ([]PeriodicTask, *time.Time, *time.Time) {
	tasks := make([]PeriodicTask, 0, len(items))
	var startDate *time.Time
	var endDate *time.Time

	for index, item := range items {
		task := buildPeriodicTask(index+1, item)
		startDate, endDate = expandPeriodicWindow(startDate, endDate, item.SubmittedAt())
		tasks = append(tasks, task)
	}

	return tasks, startDate, endDate
}

func buildPeriodicTask(week int, item *assessment.Assessment) PeriodicTask {
	task := PeriodicTask{
		Week:    week,
		DueDate: item.SubmittedAt(),
	}

	switch item.Status() {
	case assessment.StatusInterpreted:
		task.Status = "completed"
		task.CompletedAt = item.InterpretedAt()
		assessmentID := item.ID().Uint64()
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

func periodicScaleName(items []*assessment.Assessment) string {
	for _, item := range items {
		if item != nil && item.HasMedicalScale() {
			return item.MedicalScaleRef().Name()
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
