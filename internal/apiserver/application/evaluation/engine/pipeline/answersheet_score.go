package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
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

	// TODO: 从 survey 域加载 AnswerSheet
	// answerSheet, err := h.answerSheetRepo.FindByID(ctx, evalCtx.Assessment.AnswerSheetRef().ID())
	// if err != nil {
	//     evalCtx.SetError(err)
	//     return err
	// }

	// 获取量表因子
	factors := evalCtx.MedicalScale.GetFactors()
	factorScores := make([]assessment.FactorScoreResult, 0, len(factors))

	// TODO: 实现实际的计分逻辑
	// 当前使用模拟数据
	for _, factor := range factors {
		// 模拟计算每个因子的原始得分
		// 实际实现时应该：
		// 1. 获取该因子关联的题目
		// 2. 从答卷中获取这些题目的答案
		// 3. 根据计分规则计算原始分

		rawScore := h.calculateFactorRawScore(factor)

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

// calculateFactorRawScore 计算因子原始得分（模拟实现）
// TODO: 替换为实际的计分逻辑
func (h *AnswerSheetScoreHandler) calculateFactorRawScore(factor interface{}) float64 {
	// 模拟得分：返回一个模拟值
	// 实际实现时应根据答卷和计分规则计算
	return 50.0
}
