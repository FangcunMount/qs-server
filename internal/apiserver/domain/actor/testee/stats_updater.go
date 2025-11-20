package testee

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// StatsUpdater 测评统计更新器领域服务
// 负责更新 Testee 的测评统计快照
// 注意：此服务应该由测评完成的领域事件触发调用
type StatsUpdater interface {
	// UpdateAfterAssessment 测评完成后更新统计
	// 参数：
	//   - testee: 受试者
	//   - assessmentTime: 测评完成时间
	//   - riskLevel: 风险等级
	UpdateAfterAssessment(ctx context.Context, testee *Testee, assessmentTime time.Time, riskLevel string) error

	// RecalculateStats 重新计算统计（用于修复数据）
	// 从数据库查询该受试者的所有测评记录，重新计算统计
	RecalculateStats(ctx context.Context, testee *Testee) error

	// IncrementCount 仅增加计数（快速路径，不更新其他信息）
	IncrementCount(testee *Testee) error
}

// statsUpdater 统计更新器实现
type statsUpdater struct {
	repo Repository
	// assessmentRepo 可能需要查询测评记录
	// assessmentRepo assessment.Repository
}

// NewStatsUpdater 创建统计更新器
func NewStatsUpdater(repo Repository) StatsUpdater {
	return &statsUpdater{
		repo: repo,
	}
}

// UpdateAfterAssessment 测评完成后更新统计
func (s *statsUpdater) UpdateAfterAssessment(
	ctx context.Context,
	testee *Testee,
	assessmentTime time.Time,
	riskLevel string,
) error {
	// 验证参数
	if assessmentTime.IsZero() {
		return errors.WithCode(code.ErrInvalidArgument, "assessment time cannot be zero")
	}

	if riskLevel == "" {
		return errors.WithCode(code.ErrInvalidArgument, "risk level cannot be empty")
	}

	// 获取当前统计
	var totalCount int
	var lastTime time.Time

	if testee.assessmentStats != nil {
		totalCount = testee.assessmentStats.TotalCount()
		lastTime = testee.assessmentStats.LastAssessmentAt()
	}

	// 增加计数
	totalCount++

	// 更新最后测评时间（只记录最新的）
	if assessmentTime.After(lastTime) {
		lastTime = assessmentTime
	}

	// 创建新的统计快照
	newStats := NewAssessmentStats(lastTime, totalCount, riskLevel)
	testee.updateAssessmentStats(newStats)

	// TODO: 根据统计结果自动打标签
	// 例如：如果是高风险，自动添加 "high_risk" 标签
	if riskLevel == "high" && !testee.HasTag("high_risk") {
		testee.addTag("high_risk")
	}

	// TODO: 发布领域事件
	// events.Publish(NewTesteeStatsUpdatedEvent(testee.ID(), totalCount, riskLevel))

	return nil
}

// RecalculateStats 重新计算统计
func (s *statsUpdater) RecalculateStats(ctx context.Context, testee *Testee) error {
	// TODO: 实现从 assessment 仓储查询并重新计算
	// 这个方法用于数据修复或定期校验

	// 伪代码示例：
	// assessments, err := s.assessmentRepo.FindByTesteeID(ctx, testee.ID())
	// if err != nil {
	//     return err
	// }
	//
	// if len(assessments) == 0 {
	//     testee.UpdateAssessmentStats(nil)
	//     return nil
	// }
	//
	// // 找到最新的测评
	// var lastTime time.Time
	// var lastRiskLevel string
	// for _, a := range assessments {
	//     if a.CompletedAt().After(lastTime) {
	//         lastTime = a.CompletedAt()
	//         lastRiskLevel = a.RiskLevel()
	//     }
	// }
	//
	// newStats := NewAssessmentStats(lastTime, len(assessments), lastRiskLevel)
	// testee.UpdateAssessmentStats(newStats)

	return errors.New("not implemented yet")
}

// IncrementCount 仅增加计数
func (s *statsUpdater) IncrementCount(testee *Testee) error {
	if testee.assessmentStats == nil {
		return errors.WithCode(code.ErrValidation, "cannot increment count: no stats initialized")
	}

	currentStats := testee.assessmentStats
	newStats := NewAssessmentStats(
		currentStats.LastAssessmentAt(),
		currentStats.TotalCount()+1,
		currentStats.LastRiskLevel(),
	)

	testee.updateAssessmentStats(newStats)

	return nil
}
