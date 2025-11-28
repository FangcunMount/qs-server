package assessment

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// evaluationService 评估服务实现
// 行为者：评估引擎 (qs-worker)
//
// 设计说明：
// 此服务负责执行测评的计分和解读流程，由 qs-worker 消费 AssessmentSubmittedEvent 后调用。
// 完整流程：
//  1. 加载 Assessment、MedicalScale、AnswerSheet
//  2. 调用 calculation 功能域计算各因子得分
//  3. 调用 interpretation 功能域生成解读
//  4. 组装 EvaluationResult
//  5. 应用评估结果到 Assessment
//  6. 保存 AssessmentScore
//  7. 生成并保存 InterpretReport
//
// 注意：当前实现为骨架代码，部分功能（如 survey 域集成）需要后续完善。
type evaluationService struct {
	// 仓储依赖
	assessmentRepo assessment.Repository
	scoreRepo      assessment.ScoreRepository
	reportRepo     report.ReportRepository
	scaleRepo      scale.Repository

	// 领域服务依赖
	reportBuilder report.ReportBuilder
}

// NewEvaluationService 创建评估服务
func NewEvaluationService(
	assessmentRepo assessment.Repository,
	scoreRepo assessment.ScoreRepository,
	reportRepo report.ReportRepository,
	scaleRepo scale.Repository,
	reportBuilder report.ReportBuilder,
) EvaluationService {
	return &evaluationService{
		assessmentRepo: assessmentRepo,
		scoreRepo:      scoreRepo,
		reportRepo:     reportRepo,
		scaleRepo:      scaleRepo,
		reportBuilder:  reportBuilder,
	}
}

// Evaluate 执行评估
func (s *evaluationService) Evaluate(ctx context.Context, assessmentID uint64) error {
	// 1. 加载 Assessment
	id := meta.FromUint64(assessmentID)
	a, err := s.assessmentRepo.FindByID(ctx, id)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrAssessmentNotFound, "测评不存在")
	}

	// 检查状态
	if !a.Status().IsSubmitted() {
		return errors.WithCode(errorCode.ErrAssessmentInvalidStatus, "测评状态不正确，无法评估")
	}

	// 检查是否有关联量表（纯问卷模式不需要评估）
	if a.MedicalScaleRef() == nil {
		// 纯问卷模式，直接标记为已解读（无需计分）
		// 注意：需要在领域层添加纯问卷模式的完成方法
		return nil
	}

	// 2. 加载 MedicalScale
	scaleCode := a.MedicalScaleRef().Code().String()
	medicalScale, err := s.scaleRepo.FindByCode(ctx, scaleCode)
	if err != nil {
		s.markAsFailed(ctx, a, "加载量表失败: "+err.Error())
		return errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "量表不存在")
	}

	// 3. 执行计分和解读
	// TODO: 加载 AnswerSheet 并执行实际的计分逻辑
	// 当前使用模拟数据进行测试
	evalResult := s.buildMockEvaluationResult(medicalScale)

	// 4. 应用评估结果到 Assessment
	if err := a.ApplyEvaluation(evalResult); err != nil {
		s.markAsFailed(ctx, a, "应用评估结果失败: "+err.Error())
		return errors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, "应用评估结果失败")
	}

	// 5. 保存 Assessment
	if err := s.assessmentRepo.Save(ctx, a); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败")
	}

	// 6. 创建并保存 AssessmentScore
	score := assessment.FromEvaluationResult(a.ID(), evalResult)
	if err := s.scoreRepo.SaveScores(ctx, []*assessment.AssessmentScore{score}); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存得分失败")
	}

	// 7. 生成并保存 InterpretReport
	report, err := s.reportBuilder.Build(a, medicalScale, evalResult)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, "生成报告失败")
	}
	if err := s.reportRepo.Save(ctx, report); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存报告失败")
	}

	return nil
}

// EvaluateBatch 批量评估
func (s *evaluationService) EvaluateBatch(ctx context.Context, assessmentIDs []uint64) (*BatchEvaluationResult, error) {
	result := &BatchEvaluationResult{
		TotalCount:   len(assessmentIDs),
		SuccessCount: 0,
		FailedCount:  0,
		FailedIDs:    make([]uint64, 0),
	}

	for _, id := range assessmentIDs {
		if err := s.Evaluate(ctx, id); err != nil {
			result.FailedCount++
			result.FailedIDs = append(result.FailedIDs, id)
		} else {
			result.SuccessCount++
		}
	}

	return result, nil
}

// buildMockEvaluationResult 构建模拟评估结果（用于测试）
// TODO: 替换为实际的计分和解读逻辑
func (s *evaluationService) buildMockEvaluationResult(medicalScale *scale.MedicalScale) *assessment.EvaluationResult {
	// 获取所有因子并构建模拟得分
	factors := medicalScale.GetFactors()
	factorScores := make([]assessment.FactorScoreResult, 0, len(factors))

	var totalScore float64
	for _, factor := range factors {
		// 模拟得分
		mockScore := 50.0
		totalScore += mockScore

		factorScore := assessment.NewFactorScoreResult(
			assessment.NewFactorCode(string(factor.GetCode())),
			factor.GetTitle(),
			mockScore,
			assessment.RiskLevelLow,
			"模拟结论",
			"模拟建议",
			factor.IsTotalScore(),
		)
		factorScores = append(factorScores, factorScore)
	}

	// 计算平均分作为总分
	if len(factors) > 0 {
		totalScore = totalScore / float64(len(factors))
	}

	return assessment.NewEvaluationResult(
		totalScore,
		assessment.RiskLevelLow,
		"测评已完成，整体情况良好",
		"保持健康的生活方式",
		factorScores,
	)
}

// markAsFailed 标记测评为失败
func (s *evaluationService) markAsFailed(ctx context.Context, a *assessment.Assessment, reason string) {
	_ = a.MarkAsFailed(reason)
	_ = s.assessmentRepo.Save(ctx, a)
}
