package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// FactorScoreHandler 因子分数计算处理器
// 职责：从答卷读取预计算分数，按因子聚合计算原始得分
// 输入：Assessment、MedicalScale、AnswerSheet（已包含每题分数）
// 输出：填充 Context.FactorScores
type FactorScoreHandler struct {
	*BaseHandler
	calculator FactorScoreCalculator
}

// NewFactorScoreHandler 创建因子分数计算处理器
func NewFactorScoreHandler(scorer ruleengine.ScaleFactorScorer) *FactorScoreHandler {
	return &FactorScoreHandler{
		BaseHandler: NewBaseHandler("FactorScoreHandler"),
		calculator:  NewFactorScoreCalculator(scorer),
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

	evalCtx.FactorScores, evalCtx.TotalScore = h.calculator.Calculate(
		ctx,
		evalCtx.MedicalScale,
		evalCtx.AnswerSheet,
		evalCtx.Questionnaire,
	)

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}
