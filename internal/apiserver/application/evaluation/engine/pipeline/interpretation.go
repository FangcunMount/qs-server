package pipeline

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// InterpretationHandler 测评分析解读处理器
// 职责：
// 1. 根据因子得分和风险等级生成解读结论和建议
// 2. 应用评估结果到 Assessment
// 3. 生成并保存 InterpretReport
// 输入：Context（包含因子得分、总分、风险等级）
// 输出：填充 Context.Conclusion, Suggestion, EvaluationResult
type InterpretationHandler struct {
	*BaseHandler
	assessmentRepo assessment.Repository
	reportRepo     report.ReportRepository
	reportBuilder  report.ReportBuilder
}

// NewInterpretationHandler 创建测评分析解读处理器
func NewInterpretationHandler(
	assessmentRepo assessment.Repository,
	reportRepo report.ReportRepository,
	reportBuilder report.ReportBuilder,
) *InterpretationHandler {
	return &InterpretationHandler{
		BaseHandler:    NewBaseHandler("InterpretationHandler"),
		assessmentRepo: assessmentRepo,
		reportRepo:     reportRepo,
		reportBuilder:  reportBuilder,
	}
}

// Handle 处理测评分析解读
func (h *InterpretationHandler) Handle(ctx context.Context, evalCtx *Context) error {
	// 1. 生成因子解读
	h.generateFactorInterpretations(evalCtx)

	// 2. 生成整体解读
	h.generateOverallInterpretation(evalCtx)

	// 3. 构建完整的评估结果
	evalResult := h.buildEvaluationResult(evalCtx)
	evalCtx.EvaluationResult = evalResult

	// 4. 应用评估结果到 Assessment
	if err := evalCtx.Assessment.ApplyEvaluation(evalResult); err != nil {
		evalCtx.SetError(errors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, "应用评估结果失败"))
		return evalCtx.Error
	}

	// 5. 保存 Assessment
	if err := h.assessmentRepo.Save(ctx, evalCtx.Assessment); err != nil {
		evalCtx.SetError(errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败"))
		return evalCtx.Error
	}

	// 6. 生成并保存报告
	if err := h.generateAndSaveReport(ctx, evalCtx); err != nil {
		evalCtx.SetError(err)
		return err
	}

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// generateFactorInterpretations 生成因子解读
func (h *InterpretationHandler) generateFactorInterpretations(evalCtx *Context) {
	updatedScores := make([]assessment.FactorScoreResult, 0, len(evalCtx.FactorScores))

	for _, fs := range evalCtx.FactorScores {
		// 根据因子的风险等级生成结论和建议
		conclusion, suggestion := h.interpretFactor(fs.FactorName, fs.RiskLevel, fs.RawScore)

		updatedScore := assessment.NewFactorScoreResult(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			conclusion,
			suggestion,
			fs.IsTotalScore,
		)
		updatedScores = append(updatedScores, updatedScore)
	}

	evalCtx.FactorScores = updatedScores
}

// interpretFactor 解读单个因子
func (h *InterpretationHandler) interpretFactor(factorName string, riskLevel assessment.RiskLevel, score float64) (conclusion, suggestion string) {
	// TODO: 根据因子类型和量表规则生成更精确的解读
	// 当前使用通用模板
	switch riskLevel {
	case assessment.RiskLevelSevere:
		conclusion = fmt.Sprintf("%s因子得分%.1f分，处于严重异常水平", factorName, score)
		suggestion = "建议立即寻求专业帮助，进行进一步评估"
	case assessment.RiskLevelHigh:
		conclusion = fmt.Sprintf("%s因子得分%.1f分，处于较高风险水平", factorName, score)
		suggestion = "建议尽快咨询专业人员，了解更多信息"
	case assessment.RiskLevelMedium:
		conclusion = fmt.Sprintf("%s因子得分%.1f分，处于中等水平", factorName, score)
		suggestion = "建议关注相关方面，适当调整生活方式"
	case assessment.RiskLevelLow:
		conclusion = fmt.Sprintf("%s因子得分%.1f分，处于正常偏低水平", factorName, score)
		suggestion = "整体情况良好，保持当前状态"
	default:
		conclusion = fmt.Sprintf("%s因子得分%.1f分，处于正常水平", factorName, score)
		suggestion = "状态良好，继续保持"
	}
	return
}

// generateOverallInterpretation 生成整体解读
func (h *InterpretationHandler) generateOverallInterpretation(evalCtx *Context) {
	// TODO: 根据量表类型和整体风险等级生成更精确的解读
	switch evalCtx.RiskLevel {
	case assessment.RiskLevelSevere:
		evalCtx.Conclusion = "测评结果显示存在严重问题，需要立即关注"
		evalCtx.Suggestion = "强烈建议尽快寻求专业帮助，进行全面评估和干预"
	case assessment.RiskLevelHigh:
		evalCtx.Conclusion = "测评结果显示存在较高风险，需要重点关注"
		evalCtx.Suggestion = "建议尽快咨询专业人员，获取更详细的评估和指导"
	case assessment.RiskLevelMedium:
		evalCtx.Conclusion = "测评结果显示存在一定风险，需要适度关注"
		evalCtx.Suggestion = "建议关注相关方面的变化，必要时寻求专业帮助"
	case assessment.RiskLevelLow:
		evalCtx.Conclusion = "测评结果显示整体情况良好，少数方面需要注意"
		evalCtx.Suggestion = "保持健康的生活方式，定期进行自我检查"
	default:
		evalCtx.Conclusion = "测评已完成，整体情况良好"
		evalCtx.Suggestion = "保持健康的生活方式"
	}
}

// buildEvaluationResult 构建评估结果
func (h *InterpretationHandler) buildEvaluationResult(evalCtx *Context) *assessment.EvaluationResult {
	return assessment.NewEvaluationResult(
		evalCtx.TotalScore,
		evalCtx.RiskLevel,
		evalCtx.Conclusion,
		evalCtx.Suggestion,
		evalCtx.FactorScores,
	)
}

// generateAndSaveReport 生成并保存报告
func (h *InterpretationHandler) generateAndSaveReport(ctx context.Context, evalCtx *Context) error {
	// 生成报告
	rpt, err := h.reportBuilder.Build(evalCtx.Assessment, evalCtx.MedicalScale, evalCtx.EvaluationResult)
	if err != nil {
		return errors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, "生成报告失败")
	}

	// 保存报告
	if err := h.reportRepo.Save(ctx, rpt); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存报告失败")
	}

	return nil
}
