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
	// 1. 验证受试者是否存在
	testee, err := s.testeeRepo.FindByID(ctx, domain.ID(testeeID))
	if err != nil {
		if errors.IsCode(err, code.ErrUserNotFound) {
			return nil, errors.WithCode(code.ErrUserNotFound, "testee not found")
		}
		return nil, errors.Wrap(err, "failed to find testee")
	}

	// 2. 查询受试者的所有测评记录（不分页，全量查询）
	// 使用一个大的分页参数来获取所有记录
	pagination := assessment.NewPagination(1, 1000)
	assessments, _, err := s.assessmentRepo.FindByTesteeID(ctx, testee.ID(), pagination)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find assessments")
	}

	// 3. 过滤出已完成的测评（只有完成的测评才有有效结果）
	completedAssessments := make([]*assessment.Assessment, 0)
	for _, a := range assessments {
		if a.Status() == assessment.StatusInterpreted {
			completedAssessments = append(completedAssessments, a)
		}
	}

	// 4. 按量表分组
	scaleMap := make(map[uint64]*ScaleTrendAnalysis) // key: scaleID

	for _, a := range completedAssessments {
		// 只处理有量表的测评
		if !a.HasMedicalScale() {
			continue
		}

		scaleRef := a.MedicalScaleRef()
		scaleID := uint64(scaleRef.ID())

		// 初始化量表分析结构
		if _, exists := scaleMap[scaleID]; !exists {
			scaleMap[scaleID] = &ScaleTrendAnalysis{
				ScaleID:   scaleID,
				ScaleCode: string(scaleRef.Code()),
				ScaleName: scaleRef.Name(),
				Tests:     make([]TestRecordData, 0),
			}
		}

		// 构建测评记录数据
		testRecord := TestRecordData{
			AssessmentID: uint64(a.ID()),
			TestDate:     *a.SubmittedAt(), // 使用提交时间
		}

		// 获取总分和风险等级
		if a.TotalScore() != nil {
			testRecord.TotalScore = *a.TotalScore()
		}
		if a.RiskLevel() != nil {
			testRecord.RiskLevel = string(*a.RiskLevel())
		}

		// 获取解读报告（获取结果描述）
		rep, err := s.reportRepo.FindByAssessmentID(ctx, a.ID())
		if err == nil && rep != nil {
			testRecord.Result = rep.Conclusion()
		}

		// 获取因子得分
		scores, err := s.scoreRepo.FindByAssessmentID(ctx, a.ID())
		if err == nil && len(scores) > 0 {
			// 获取因子得分列表
			for _, score := range scores {
				for _, factorScore := range score.FactorScores() {
					// 跳过总分因子（总分已经在上面记录）
					if factorScore.IsTotalScore() {
						continue
					}

					factorData := FactorScoreData{
						FactorCode: string(factorScore.FactorCode()),
						FactorName: factorScore.FactorName(),
						RawScore:   factorScore.RawScore(),
						RiskLevel:  string(factorScore.RiskLevel()),
					}

					// T分和百分位（如果有）
					// 注意：当前设计中 FactorScore 只有 RawScore，T分和百分位可能在未来扩展
					// 这里预留字段，当前为 nil

					testRecord.Factors = append(testRecord.Factors, factorData)
				}
			}
		}

		scaleMap[scaleID].Tests = append(scaleMap[scaleID].Tests, testRecord)
	}

	// 5. 对每个量表的测评记录按时间排序（升序）
	for _, scaleTrend := range scaleMap {
		sort.Slice(scaleTrend.Tests, func(i, j int) bool {
			return scaleTrend.Tests[i].TestDate.Before(scaleTrend.Tests[j].TestDate)
		})
	}

	// 6. 转换为结果列表（按量表ID排序）
	scales := make([]ScaleTrendAnalysis, 0, len(scaleMap))
	for _, scaleTrend := range scaleMap {
		scales = append(scales, *scaleTrend)
	}

	// 按量表ID排序
	sort.Slice(scales, func(i, j int) bool {
		return scales[i].ScaleID < scales[j].ScaleID
	})

	return &ScaleAnalysisResult{
		TesteeID: testeeID,
		Scales:   scales,
	}, nil
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
	testeeItem, err := s.testeeRepo.FindByID(ctx, domain.ID(testeeID))
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
		assessmentID := uint64(item.ID())
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
