package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
)

// AnswerSheetScoreHandler 答卷分数计算处理器
// 职责：根据答卷计算各因子的原始得分
// 输入：Assessment、MedicalScale、AnswerSheet
// 输出：填充 Context.FactorScores
type AnswerSheetScoreHandler struct {
	*BaseHandler
}

// NewAnswerSheetScoreHandler 创建答卷分数计算处理器
func NewAnswerSheetScoreHandler() *AnswerSheetScoreHandler {
	return &AnswerSheetScoreHandler{
		BaseHandler: NewBaseHandler("AnswerSheetScoreHandler"),
	}
}

// Handle 处理答卷分数计算
func (h *AnswerSheetScoreHandler) Handle(ctx context.Context, evalCtx *Context) error {
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
		rawScore := h.calculateFactorRawScore(factor, evalCtx.AnswerSheet)

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

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// calculateFactorRawScore 计算因子原始得分
// 根据因子关联的题目和计分策略计算原始分
func (h *AnswerSheetScoreHandler) calculateFactorRawScore(factor *scale.Factor, sheet *answersheet.AnswerSheet) float64 {
	// 如果没有答卷数据，使用模拟数据
	if sheet == nil {
		return h.simulateFactorScore(factor)
	}

	// 获取该因子关联的题目编码
	questionCodes := factor.GetQuestionCodes()
	if len(questionCodes) == 0 {
		return 0
	}

	// 收集该因子所有题目的得分
	var scores []float64
	scoreMap := h.buildScoreMap(sheet)
	for _, qCode := range questionCodes {
		if score, found := scoreMap[qCode.String()]; found {
			scores = append(scores, score)
		}
	}

	if len(scores) == 0 {
		return 0
	}

	// 根据计分策略计算最终得分
	return h.applyScoringStrategy(factor.GetScoringStrategy(), scores)
}

// buildScoreMap 构建题目得分映射
func (h *AnswerSheetScoreHandler) buildScoreMap(sheet *answersheet.AnswerSheet) map[string]float64 {
	scoreMap := make(map[string]float64)
	for _, ans := range sheet.Answers() {
		scoreMap[ans.QuestionCode()] = ans.Score()
	}
	return scoreMap
}

// applyScoringStrategy 应用计分策略
func (h *AnswerSheetScoreHandler) applyScoringStrategy(strategy scale.ScoringStrategyCode, scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}

	switch strategy {
	case scale.ScoringStrategySum:
		// 求和策略
		var total float64
		for _, s := range scores {
			total += s
		}
		return total

	case scale.ScoringStrategyAvg:
		// 平均策略
		var total float64
		for _, s := range scores {
			total += s
		}
		return total / float64(len(scores))

	case scale.ScoringStrategyCustom:
		// 自定义策略：当前使用求和作为默认实现
		var total float64
		for _, s := range scores {
			total += s
		}
		return total

	default:
		// 默认使用求和策略
		var total float64
		for _, s := range scores {
			total += s
		}
		return total
	}
}

// simulateFactorScore 模拟因子得分（当没有答卷数据时使用）
func (h *AnswerSheetScoreHandler) simulateFactorScore(factor *scale.Factor) float64 {
	// 模拟得分：基于因子包含的题目数量生成模拟值
	questionCount := factor.QuestionCount()
	if questionCount == 0 {
		return 50.0 // 默认模拟值
	}

	// 假设每题平均分为 2.5 分
	return float64(questionCount) * 2.5
}
