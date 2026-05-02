package pipeline

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// RiskLevelHandler 风险等级计算处理器
// 职责：
// 1. 根据因子得分和量表解读规则计算风险等级
// 2. 委托 writer 保存 AssessmentScore
// 输入：Context.FactorScores, Context.TotalScore
// 输出：填充 Context.RiskLevel，更新 Context.FactorScores 的风险等级
type RiskLevelHandler struct {
	*BaseHandler
	scoreWriter AssessmentScoreWriter
	classifier  RiskClassifier
}

// NewRiskLevelHandler 创建风险等级计算处理器
func NewRiskLevelHandler(scoreRepo assessment.ScoreRepository) *RiskLevelHandler {
	return NewRiskLevelHandlerWithWriter(NewAssessmentScoreWriter(scoreRepo))
}

func NewRiskLevelHandlerWithWriter(scoreWriter AssessmentScoreWriter) *RiskLevelHandler {
	return &RiskLevelHandler{
		BaseHandler: NewBaseHandler("RiskLevelHandler"),
		scoreWriter: scoreWriter,
		classifier:  NewRiskClassifier(),
	}
}

// Handle 处理风险等级计算
func (h *RiskLevelHandler) Handle(ctx context.Context, evalCtx *Context) error {
	// 检查前置条件
	if len(evalCtx.FactorScores) == 0 {
		evalCtx.SetError(ErrFactorScoresRequired)
		return evalCtx.Error
	}

	evalCtx.FactorScores, evalCtx.RiskLevel = h.classifier.Classify(evalCtx.MedicalScale, evalCtx.FactorScores)

	// 3. 保存 AssessmentScore
	if err := h.scoreWriter.SaveAssessmentScore(ctx, evalCtx); err != nil {
		evalCtx.SetError(err)
		return err
	}

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}
