package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

// FactorScoreHandler 因子分数计算处理器
// 职责：从答卷读取预计算分数，按因子聚合计算原始得分
// 输入：Assessment、MedicalScale、AnswerSheet（已包含每题分数）
// 输出：填充 Context.FactorScores
type FactorScoreHandler struct {
	*BaseHandler
	scoringService scale.ScoringService // 量表计分服务（领域服务）
}

// NewFactorScoreHandler 创建因子分数计算处理器
func NewFactorScoreHandler() *FactorScoreHandler {
	return &FactorScoreHandler{
		BaseHandler:    NewBaseHandler("FactorScoreHandler"),
		scoringService: scale.NewScoringService(),
	}
}

// Handle 处理因子分数计算
func (h *FactorScoreHandler) Handle(ctx context.Context, evalCtx *Context) error {
	// 检查前置条件
	if evalCtx.Assessment == nil {
		evalCtx.SetError(ErrAssessmentRequired)
		return evalCtx.Error
	}
	if evalCtx.MedicalScale == nil {
		evalCtx.SetError(ErrMedicalScaleRequired)
		return evalCtx.Error
	}

	// 获取量表因子
	factors := evalCtx.MedicalScale.GetFactors()
	factorScores := make([]assessment.FactorScoreResult, 0, len(factors))

	// 计算每个因子的原始得分
	for _, factor := range factors {
		rawScore := h.calculateFactorRawScore(factor, evalCtx.AnswerSheet, evalCtx.Questionnaire)

		factorScore := assessment.NewFactorScoreResult(
			assessment.NewFactorCode(string(factor.GetCode())),
			factor.GetTitle(),
			rawScore,
			assessment.RiskLevelNone, // 风险等级由后续处理器计算
			"",                       // 结论由后续处理器填充
			"",                       // 建议由后续处理器填充
			factor.IsTotalScore(),
		)
		factorScores = append(factorScores, factorScore)
	}

	// 填充评估上下文
	evalCtx.FactorScores = factorScores

	// 计算总分
	evalCtx.TotalScore = h.calculateTotalScore(factorScores)

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// calculateTotalScore 计算总分
// 如果有标记为总分的因子，直接使用；否则累加所有因子得分
func (h *FactorScoreHandler) calculateTotalScore(factorScores []assessment.FactorScoreResult) float64 {
	var totalScore float64

	for _, fs := range factorScores {
		// 如果有明确标记为总分的因子，直接使用
		if fs.IsTotalScore {
			return fs.RawScore
		}
		totalScore += fs.RawScore
	}

	// 如果没有总分因子，返回所有因子得分之和
	return totalScore
}

// calculateFactorRawScore 计算因子原始得分
// 委托给量表领域的计分服务
func (h *FactorScoreHandler) calculateFactorRawScore(
	factor *scale.Factor,
	sheet *answersheet.AnswerSheet,
	qnr *questionnaire.Questionnaire,
) float64 {
	// 如果没有答卷数据，使用模拟数据
	if sheet == nil {
		return h.simulateFactorScore(factor)
	}

	// 委托给领域服务计算因子得分
	score, err := h.scoringService.CalculateFactorScore(factor, sheet, qnr)
	if err != nil {
		// 计算失败，返回 0
		// TODO: 添加日志记录
		return 0
	}

	return score
}

// simulateFactorScore 模拟因子得分（当没有答卷数据时使用）
func (h *FactorScoreHandler) simulateFactorScore(factor *scale.Factor) float64 {
	// 模拟得分：基于因子包含的题目数量生成模拟值
	questionCount := factor.QuestionCount()
	if questionCount == 0 {
		return 50.0 // 默认模拟值
	}

	// 假设每题平均分为 2.5 分
	return float64(questionCount) * 2.5
}
